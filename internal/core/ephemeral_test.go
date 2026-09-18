package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// A streaming draft is published, not stored: it must leave the account's
// history exactly as it found it (#192's rule about state, applied to a stream
// that has none).
func TestEphemeralDraft_IsPublishedAndNeverStored(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	before := len(st.Messages(1))
	require.Zero(t, before)

	o.handleEvent(store.Event{
		Kind:   store.EventEphemeralDraft,
		ChatID: 1,
		Ephemeral: domain.EphemeralDraft{
			ChatID: 1, ID: 7, Text: "generating",
			TextBlocks: []domain.PageBlock{{Kind: domain.BlockKindThinking, Text: &domain.RichText{Text: "working"}}},
		},
	})

	select {
	case got := <-o.Drafts():
		assert.Equal(t, int64(1), got.ChatID)
		assert.Equal(t, 7, got.Draft.ID)
		require.Len(t, got.Draft.TextBlocks, 1)
		assert.Equal(t, domain.BlockKindThinking, got.Draft.TextBlocks[0].Kind)
	default:
		t.Fatal("the revision was not published")
	}
	assert.Len(t, st.Messages(1), before, "a draft is not a message")
}

// The stream ends with an empty document, which is what tells a client to take
// the overlay away.
func TestEphemeralDraft_DeleteClearsTheStream(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)

	o.handleEvent(store.Event{Kind: store.EventEphemeralDraft, ChatID: 1,
		Ephemeral: domain.EphemeralDraft{ChatID: 1, ID: 7}})
	<-o.Drafts()

	o.handleEvent(store.Event{Kind: store.EventEphemeralDelete, ChatID: 1})

	select {
	case got := <-o.Drafts():
		assert.Equal(t, int64(1), got.ChatID)
		assert.Zero(t, got.Draft.ID, "an empty draft is the end of the stream")
	default:
		t.Fatal("the end of the stream was not published")
	}
	_, live := o.draftSet.Draft(1)
	assert.False(t, live)
}

// A revision replaces the previous one rather than queueing behind it: a stream
// is a sequence of revisions of one document, not a scrollback of them.
func TestEphemeralDraft_RevisionReplacesTheLiveOne(t *testing.T) {
	c := &stubClient{}
	o, _ := newCmdOwner(t, c)

	o.handleEvent(store.Event{Kind: store.EventEphemeralDraft, ChatID: 1,
		Ephemeral: domain.EphemeralDraft{ChatID: 1, ID: 7, Text: "first"}})
	<-o.Drafts()
	o.handleEvent(store.Event{Kind: store.EventEphemeralDraft, ChatID: 1,
		Ephemeral: domain.EphemeralDraft{ChatID: 1, ID: 7, Text: "second"}})
	<-o.Drafts()

	got, ok := o.draftSet.Draft(1)
	require.True(t, ok)
	assert.Equal(t, "second", got.Text)
}

// A draft that nothing replaces expires on the protocol's own clock, which is
// what keeps a dead stream's document off the screen forever.
func TestEphemeralDraft_Expires(t *testing.T) {
	s := newDraftStream()
	s.put(domain.EphemeralDraft{ChatID: 1, ID: 7, Text: "streaming"})
	_, ok := s.Draft(1)
	require.True(t, ok)

	// Expiry is the timer's job; firing it directly is the same code path the
	// timer takes, without a 30-second test.
	s.expire(1)
	_, ok = s.Draft(1)
	assert.False(t, ok)
}

// The draft stream is per chat: two chats streaming at once are two documents,
// and one ending does not take the other away.
func TestDraftStream_IsPerChat(t *testing.T) {
	s := newDraftStream()
	s.put(domain.EphemeralDraft{ChatID: 1, ID: 1, Text: "one"})
	s.put(domain.EphemeralDraft{ChatID: 2, ID: 2, Text: "two"})

	s.drop(1)

	_, ok := s.Draft(1)
	assert.False(t, ok)
	two, ok := s.Draft(2)
	require.True(t, ok)
	assert.Equal(t, "two", two.Text)
}

// Expiring a chat that has no live draft is a no-op: a stopped timer's callback
// can still run, and it must not take away a newer revision.
func TestDraftStream_ExpireIsIdempotent(t *testing.T) {
	s := newDraftStream()
	s.expire(9)
	_, ok := s.Draft(9)
	assert.False(t, ok)

	s.put(domain.EphemeralDraft{ChatID: 9, ID: 1})
	s.expire(9)
	s.expire(9)
	_, ok = s.Draft(9)
	assert.False(t, ok)
}

func TestEphemeralDraftTTL_MatchesTheProtocol(t *testing.T) {
	// Telegram expires a rich-message draft 30 seconds after the last revision.
	// A longer window would show a document that can no longer become a message.
	assert.Equal(t, 30*time.Second, EphemeralDraftTTL)
}
