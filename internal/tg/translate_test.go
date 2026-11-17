package tg

import (
	"context"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// translateInvoker answers messages.translateText the way the wire does, from a
// canned result, and records what was asked. It stands in for gotd's RPC chain
// so the real method runs: the request is the one the method built, and the
// reply is decoded off a buffer exactly as a live one is.
type translateInvoker struct {
	// result is what the server answers with; nil answers Telegram's refusal.
	result *tg.MessagesTranslateResult
	err    error

	calls int
	req   *tg.MessagesTranslateTextRequest
}

func (f *translateInvoker) Invoke(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	f.calls++
	req, ok := input.(*tg.MessagesTranslateTextRequest)
	if !ok {
		return &tgerr.Error{Code: 400, Type: "UNEXPECTED_REQUEST"}
	}
	f.req = req
	if f.err != nil {
		return f.err
	}

	// Round-tripped through the wire format rather than handed over directly,
	// so the test exercises the decode path a real reply takes.
	var buf bin.Buffer
	if err := f.result.Encode(&buf); err != nil {
		return err
	}
	return output.Decode(&buf)
}

// translatingClient wires a GotdClient to the invoker above. It goes through
// the error middleware the connection installs, so a refusal is classified by
// the same path a live call takes.
func translatingClient(f *translateInvoker) *GotdClient {
	c := testClient()
	c.api = tg.NewClient(c.errorMiddleware().Handle(f))
	return c
}

func TestTranslateMessages_NoIDsIsNoRPC(t *testing.T) {
	f := &translateInvoker{result: &tg.MessagesTranslateResult{}}
	c := translatingClient(f)

	got, err := c.TranslateMessages(context.Background(), domain.Peer{ID: 10, Type: domain.PeerUser}, nil, "pl")

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Zero(t, f.calls, "nothing was asked for, so Telegram must not be asked")
}

// The pairing is positional, so the ids have to come back in the caller's order
// with the text Telegram returned in that position.
func TestTranslateMessages_ZipsResultsOntoIDsInOrder(t *testing.T) {
	f := &translateInvoker{result: &tg.MessagesTranslateResult{Result: []tg.TextWithEntities{
		{Text: "czesc", Entities: []tg.MessageEntityClass{&tg.MessageEntityBold{Offset: 0, Length: 5}}},
		{Text: "do widzenia"},
	}}}
	c := translatingClient(f)

	got, err := c.TranslateMessages(context.Background(), domain.Peer{ID: 10, Type: domain.PeerUser}, []int{7, 9}, "pl")

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, 7, got[0].MessageID)
	assert.Equal(t, "czesc", got[0].Text)
	assert.Equal(t, []domain.MessageEntity{{Type: "bold", Offset: 0, Length: 5}}, got[0].Entities)
	assert.Equal(t, 9, got[1].MessageID)
	assert.Equal(t, "do widzenia", got[1].Text)
	assert.Nil(t, got[1].Entities, "an unstyled translation carries no entities")
}

func TestTranslateMessages_NamesThePeerAndTheTargetLanguage(t *testing.T) {
	f := &translateInvoker{result: &tg.MessagesTranslateResult{Result: []tg.TextWithEntities{{Text: "hej"}}}}
	c := translatingClient(f)

	_, err := c.TranslateMessages(context.Background(),
		domain.Peer{ID: 11, AccessHash: 22, Type: domain.PeerChannel}, []int{5}, "de")

	require.NoError(t, err)
	require.NotNil(t, f.req)
	assert.Equal(t, "de", f.req.ToLang)
	assert.Equal(t, &tg.InputPeerChannel{ChannelID: 11, AccessHash: 22}, f.req.Peer)

	// The peer+ids form of the request, not the text form: gotd encodes which
	// of the conditional fields is present in a flag bit, and only the setters
	// set it.
	ids, ok := f.req.GetID()
	require.True(t, ok, "the id list must be flagged as present")
	assert.Equal(t, []int{5}, ids)
	_, ok = f.req.GetPeer()
	assert.True(t, ok, "the peer must be flagged as present")
	assert.Empty(t, f.req.Text, "the text field stays untouched")
	assert.False(t, f.req.Flags.Has(1), "the text flag must stay unset")
}

// A reply that is not one result per message cannot be paired up. Refused
// rather than guessed at: the alternative is showing one message's translation
// under another message's bubble.
func TestTranslateMessages_RefusesAReplyThatDoesNotMatchTheRequest(t *testing.T) {
	tests := []struct {
		name  string
		ids   []int
		reply []tg.TextWithEntities
	}{
		{"too few", []int{1, 2}, []tg.TextWithEntities{{Text: "a"}}},
		{"too many", []int{1}, []tg.TextWithEntities{{Text: "a"}, {Text: "b"}}},
		{"none at all", []int{1, 2}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &translateInvoker{result: &tg.MessagesTranslateResult{Result: tt.reply}}
			c := translatingClient(f)

			got, err := c.TranslateMessages(context.Background(), domain.Peer{ID: 10, Type: domain.PeerUser}, tt.ids, "pl")

			assert.Nil(t, got, "a mismatched reply may not be zipped onto ids")
			assert.Equal(t, telerr.Internal, telerr.Of(err))
		})
	}
}

// Without a live connection there is no RPC to map, so the method refuses
// before asking — which is what keeps an offline translate from looking like a
// refusal of the content.
func TestTranslateMessages_NotConnectedIsTransientNetwork(t *testing.T) {
	c := testClient()

	_, err := c.TranslateMessages(context.Background(), domain.Peer{ID: 10, Type: domain.PeerUser}, []int{5}, "pl")

	assert.Equal(t, telerr.Network, telerr.Of(err))
}

// The translation refusals arrive as 400/406 and Telegram's code table would
// read the 406 as an expired session — sending the person into a login that
// cannot help. They are rejected content, each with its own remedy (#253).
func TestClassifyTgErr_TranslationRefusals(t *testing.T) {
	tests := []struct {
		name       string
		err        *tgerr.Error
		wantReason telerr.Reason
	}{
		{"unsupported target language", &tgerr.Error{Code: 400, Type: "TO_LANG_INVALID"}, telerr.ReasonTranslationLanguage},
		{"translations switched off", &tgerr.Error{Code: 406, Type: "TRANSLATIONS_DISABLED"}, telerr.ReasonTranslationUnavailable},
		{"the request failed", &tgerr.Error{Code: 500, Type: "TRANSLATE_REQ_FAILED"}, telerr.ReasonTranslationTemporary},
		{"the quota ran out", &tgerr.Error{Code: 400, Type: "TRANSLATE_REQ_QUOTA_EXCEEDED"}, telerr.ReasonTranslationTemporary},
		{"the translator timed out", &tgerr.Error{Code: 500, Type: "TRANSLATION_TIMEOUT"}, telerr.ReasonTranslationTemporary},
		{"nothing to translate", &tgerr.Error{Code: 400, Type: "INPUT_TEXT_EMPTY"}, telerr.ReasonTextEmpty},
		{"too much to translate", &tgerr.Error{Code: 400, Type: "INPUT_TEXT_TOO_LONG"}, telerr.ReasonTextTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, reason, _ := classifyTgErr(tt.err)
			assert.Equal(t, telerr.Rejected, kind)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}

// Through the whole path the method takes, so the 406 the translations-disabled
// refusal arrives with is pinned as Rejected rather than Unauthorized.
func TestTranslateMessages_ARefusalArrivesAsRejected(t *testing.T) {
	f := &translateInvoker{err: &tgerr.Error{Code: 406, Type: "TRANSLATIONS_DISABLED"}}
	c := translatingClient(f)

	_, err := c.TranslateMessages(context.Background(), domain.Peer{ID: 10, Type: domain.PeerUser}, []int{5}, "pl")

	assert.Equal(t, telerr.Rejected, telerr.Of(err))
	assert.NotEqual(t, telerr.Unauthorized, telerr.Of(err))
	e, ok := telerr.As(err)
	require.True(t, ok)
	assert.Equal(t, telerr.ReasonTranslationUnavailable, e.Reason)
	assert.Equal(t, "messages.translateText", e.Op, "the operation is named by gotd's method name")
}
