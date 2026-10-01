package core

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// EphemeralDraftTTL is how long a streaming draft lives without being replaced.
//
// It is the protocol's own limit, not a preference: Telegram expires a rich
// message draft 30 seconds after the last revision, so a client that kept one
// longer would show a document that cannot become a message any more. Nothing
// arrives to say so — a draft just stops being updated.
const EphemeralDraftTTL = 30 * time.Second

// EphemeralDraft is a streaming draft as a client sees it: the document, and
// whether it is still being written.
//
// It is an event rather than a projection, like Typing: there is no persisted
// state behind it to be right about, and a draft that outlived its generation
// has nothing to clear it. The empty Draft means the stream ended.
type EphemeralDraft struct {
	ChatID int64
	Draft  domain.EphemeralDraft
}

// draftStream is the in-memory set of live drafts. It is the owner's, not the
// state's: a draft is never applied to domain state and never written to disk,
// because a draft is not a message.
type draftStream struct {
	mu      sync.Mutex
	live    map[int64]domain.EphemeralDraft
	timers  map[int64]*time.Timer
	nowFunc func() time.Time
}

func newDraftStream() *draftStream {
	return &draftStream{
		live:    make(map[int64]domain.EphemeralDraft),
		timers:  make(map[int64]*time.Timer),
		nowFunc: time.Now,
	}
}

// Drafts is the stream of streaming drafts: one entry per revision, and a final
// entry with an empty document when the generation ends (by cancellation, by a
// commit into a real message, or by the protocol's own expiry).
func (o *Owner) Drafts() <-chan EphemeralDraft { return o.drafts }

// handleEphemeral applies one ephemeral event. The drafts never reach
// state.Apply: they are not state, and applying them would make a document
// nobody sent part of the account's history.
func (o *Owner) handleEphemeral(evt store.Event) {
	switch evt.Kind {
	case store.EventEphemeralDraft:
		o.draftSet.put(evt.Ephemeral)
		o.publishDrafts(EphemeralDraft{ChatID: evt.Ephemeral.ChatID, Draft: evt.Ephemeral})
	case store.EventEphemeralDelete:
		o.draftSet.drop(evt.ChatID)
		o.publishDrafts(EphemeralDraft{ChatID: evt.ChatID})
	}
}

// put records a revision and arms (or re-arms) the expiry that ends the stream.
// A draft replaced before its timer fires must not be taken away by the older
// timer, so the timer is replaced with the revision rather than merely started
// once.
func (s *draftStream) put(draft domain.EphemeralDraft) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.live[draft.ChatID] = draft
	if t, ok := s.timers[draft.ChatID]; ok {
		t.Stop()
	}
	chatID := draft.ChatID
	s.timers[chatID] = time.AfterFunc(EphemeralDraftTTL, func() {
		s.expire(chatID)
	})
}

// drop removes a chat's draft because the generation ended.
func (s *draftStream) drop(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.live[chatID]; !ok {
		return
	}
	delete(s.live, chatID)
	if t, ok := s.timers[chatID]; ok {
		t.Stop()
		delete(s.timers, chatID)
	}
}

// expire removes a chat's draft because nothing replaced it in time. The caller
// is the revision's own timer, which is what makes a stale timer harmless: it
// is stopped when a newer revision replaces it.
func (s *draftStream) expire(chatID int64) {
	s.mu.Lock()
	if _, ok := s.live[chatID]; !ok {
		s.mu.Unlock()
		return
	}
	delete(s.live, chatID)
	delete(s.timers, chatID)
	s.mu.Unlock()
}

// Draft returns the live draft for a chat, ok being false when there is none.
// It is what a client asks when it opens a chat: a draft that began before the
// subscription existed is still on screen.
func (s *draftStream) Draft(chatID int64) (domain.EphemeralDraft, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.live[chatID]
	return d, ok
}

// publishDrafts forwards a revision to every attached client, dropping it when a
// client is not draining. A dropped revision costs one frame of a stream the
// next revision replaces anyway.
func (o *Owner) publishDrafts(d EphemeralDraft) {
	select {
	case o.drafts <- d:
	default:
		o.log.Warn("ephemeral draft dropped: client is not draining",
			zap.Int64("chat_id", d.ChatID))
	}
}
