package tg

import (
	"context"
	"fmt"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// TranslateMessages asks Telegram to translate several messages of one peer
// (messages.translateText) into targetLanguage, an ISO 639-1 code.
//
// The answer is a query result and nothing else. Translation is display state:
// the original text stays where it is — in the store, in the projections and on
// Telegram — and the caller renders whichever of the two it means to show, so
// nothing here writes anywhere (#253).
//
// Telegram documents one result per requested message, in the request's order,
// so the reply is paired up positionally with the caller's ids. A reply of any
// other length is refused rather than guessed at: attaching one message's
// translation to another message is worse than showing none.
func (c *GotdClient) TranslateMessages(ctx context.Context, peer domain.Peer, msgIDs []int, targetLanguage string) ([]domain.MessageTranslation, error) {
	if len(msgIDs) == 0 {
		return nil, nil
	}
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}

	var out []domain.MessageTranslation
	err = WithRetry(ctx, func() error {
		req := &tg.MessagesTranslateTextRequest{ToLang: targetLanguage}
		// The ID list, not the Text list: with a peer and ids the request names
		// messages. gotd's setters are also what set the flag bit each
		// conditional field's presence is encoded in, so the fields are filled
		// through them and Text is left alone entirely.
		req.SetPeer(peerToInput(peer))
		req.SetID(msgIDs)

		res, err := api.MessagesTranslateText(ctx, req)
		if err != nil {
			// Without the message text and without the translated text: the
			// first is the person's conversation, the second is that
			// conversation in a language they did not choose to have it
			// logged in (#253).
			c.log.Error("MessagesTranslateText failed",
				zap.Int64("peer_id", peer.ID),
				zap.Int("count", len(msgIDs)),
				zap.String("target_language", targetLanguage),
				zap.Error(err))
			return err
		}

		translated, err := zipTranslations(msgIDs, res.Result)
		if err != nil {
			return err
		}
		out = translated
		return nil
	})
	return out, err
}

// zipTranslations pairs each requested message id with the translation Telegram
// returned in its position.
//
// A reply whose length differs from the request is refused: the pairing would
// otherwise be a guess, and a guess here shows one message's translation under
// another message's bubble.
func zipTranslations(msgIDs []int, results []tg.TextWithEntities) ([]domain.MessageTranslation, error) {
	if len(results) != len(msgIDs) {
		return nil, &telerr.Error{
			Kind:   telerr.Internal,
			Op:     "translate messages",
			Detail: fmt.Sprintf("%d results for %d messages", len(results), len(msgIDs)),
		}
	}
	out := make([]domain.MessageTranslation, 0, len(msgIDs))
	for i, id := range msgIDs {
		out = append(out, domain.MessageTranslation{
			MessageID: id,
			Text:      results[i].Text,
			Entities:  convertEntities(results[i].Entities),
		})
	}
	return out, nil
}
