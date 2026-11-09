package core

import (
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestEphemeralDraft_IsPublishedAndNeverStored(t *testing.T) {
	o, st := newCmdOwner(t, &stubClient{})
	a, b := NewAttachment(), NewAttachment()
	a.Join(o)
	b.Join(o)
	defer a.Detach()
	defer b.Detach()
	draft := domain.EphemeralDraft{ChatID: 1, ID: 7, Text: "working", TextBlocks: []domain.PageBlock{{Kind: domain.BlockKindThinking}}}
	o.handleEvent(store.Event{Kind: store.EventEphemeralDraft, ChatID: 1, Ephemeral: draft})
	for _, client := range []*Attachment{a, b} {
		got, ok := recv[EphemeralDraft](t, client)
		require.True(t, ok)
		assert.Equal(t, draft, got.Draft)
	}
	assert.Empty(t, st.Messages(domain.HistoryKey{ChatID: 1}))
	o.draftSet.expire(1, o.draftSet.timers[1])
	for _, client := range []*Attachment{a, b} {
		got, ok := recv[EphemeralDraft](t, client)
		require.True(t, ok)
		assert.Equal(t, int64(1), got.ChatID)
		assert.Zero(t, got.Draft)
	}
	assert.Empty(t, st.Messages(domain.HistoryKey{ChatID: 1}))
}

func TestEphemeralDraft_DeleteClearsTheStream(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	a := NewAttachment()
	defer a.Detach()
	a.Join(o)
	o.handleEvent(store.Event{Kind: store.EventEphemeralDraft, Ephemeral: domain.EphemeralDraft{ChatID: 1, ID: 7}})
	_, ok := recv[EphemeralDraft](t, a)
	require.True(t, ok)
	o.handleEvent(store.Event{Kind: store.EventEphemeralDelete, ChatID: 1})
	got, ok := recv[EphemeralDraft](t, a)
	require.True(t, ok)
	assert.Zero(t, got.Draft)
	_, live := o.draftSet.Draft(1)
	assert.False(t, live)
}

func TestDraftStream_StaleExpiryCannotDeleteRevision(t *testing.T) {
	s := newDraftStream()
	defer s.close()
	s.put(domain.EphemeralDraft{ChatID: 1, ID: 7, Text: "first"})
	old := s.timers[1]
	s.put(domain.EphemeralDraft{ChatID: 1, ID: 7, Text: "second"})
	s.expire(1, old)
	got, ok := s.Draft(1)
	require.True(t, ok)
	assert.Equal(t, "second", got.Text)
	s.put(domain.EphemeralDraft{ChatID: 2, ID: 8})
	current := s.timers[1]
	s.expire(1, current)
	s.expire(1, current)
	_, ok = s.Draft(1)
	assert.False(t, ok)
	_, ok = s.Draft(2)
	assert.True(t, ok)
}

func TestDraftStream_CloseStopsTimersAndRejectsRevisions(t *testing.T) {
	s := newDraftStream()
	s.put(domain.EphemeralDraft{ChatID: 1, ID: 7})
	s.close()
	s.put(domain.EphemeralDraft{ChatID: 2, ID: 8})
	assert.Empty(t, s.live)
	assert.Empty(t, s.timers)
}

func TestEphemeralDraftTTL_MatchesTheProtocol(t *testing.T) {
	assert.Equal(t, 30*time.Second, EphemeralDraftTTL)
}
