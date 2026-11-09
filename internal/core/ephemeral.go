package core

import (
	"sync"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// EphemeralDraftTTL is Telegram's expiry after the last draft revision.
const EphemeralDraftTTL = 30 * time.Second

// EphemeralDraft is transient host-to-window content, like Typing. An empty
// Draft ends the stream. Neither revisions nor expiry ever enter the store.
type EphemeralDraft struct {
	ChatID int64
	Draft  domain.EphemeralDraft
}

type draftStream struct {
	mu      sync.Mutex
	live    map[int64]domain.EphemeralDraft
	timers  map[int64]*time.Timer
	publish func(EphemeralDraft)
	closed  bool
}

func newDraftStream() *draftStream {
	return &draftStream{
		live:   make(map[int64]domain.EphemeralDraft),
		timers: make(map[int64]*time.Timer),
	}
}

// handleEphemeral deliberately bypasses state.Apply and SQLite.
func (o *Owner) handleEphemeral(evt store.Event) {
	switch evt.Kind {
	case store.EventEphemeralDraft:
		o.draftSet.put(evt.Ephemeral)
	case store.EventEphemeralDelete:
		o.draftSet.drop(evt.ChatID)
	}
}

func (s *draftStream) put(draft domain.EphemeralDraft) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	chatID := draft.ChatID
	if t := s.timers[chatID]; t != nil {
		t.Stop()
	}
	s.live[chatID] = draft
	var timer *time.Timer
	timer = time.AfterFunc(EphemeralDraftTTL, func() {
		s.expire(chatID, timer)
	})
	s.timers[chatID] = timer
	s.emit(EphemeralDraft{ChatID: chatID, Draft: draft})
}

// expire ignores callbacks belonging to replaced or cancelled revisions.
func (s *draftStream) expire(chatID int64, timer *time.Timer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if timer != nil && s.timers[chatID] == timer {
		s.remove(chatID)
	}
}

func (s *draftStream) drop(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(chatID)
}

// remove runs under mu, serializing expiry with revisions and publication.
func (s *draftStream) remove(chatID int64) {
	if _, ok := s.live[chatID]; !ok {
		return
	}
	delete(s.live, chatID)
	if t := s.timers[chatID]; t != nil {
		t.Stop()
	}
	delete(s.timers, chatID)
	s.emit(EphemeralDraft{ChatID: chatID})
}

func (s *draftStream) emit(d EphemeralDraft) {
	if s.publish != nil {
		s.publish(d)
	}
}

func (s *draftStream) Draft(chatID int64) (domain.EphemeralDraft, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.live[chatID]
	return d, ok
}

func (s *draftStream) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for id := range s.live {
		s.remove(id)
	}
}

func (o *Owner) publishDrafts(d EphemeralDraft) { o.broadcast(d, false) }
