package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// Clicking a chat or a topic opens it and leaves the focus where the click
// landed: on the list. A confirm from the keyboard keeps moving the focus into
// the chat pane, as before.

func TestClick_ChatList_OpensChatWithoutMovingFocus(t *testing.T) {
	m := newSizedRoot(t, 100, 30)
	m.focus = FocusChat // start focused elsewhere

	// Click the third visible chat row (content row 2 -> screen y = 3).
	next, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: 3, Button: tea.MouseLeft}))
	rm := next.(RootModel)

	assert.Equal(t, FocusChatList, rm.focus) // focus stays where the click landed
	assert.Equal(t, 2, rm.chatList.Cursor()) // cursor moved to clicked row
	require.NotNil(t, cmd)                   // OpenChatMsg command emitted
	open, ok := cmd().(screens.OpenChatMsg)
	require.True(t, ok, "expected OpenChatMsg, got %T", open)
	require.False(t, open.IsForum)
	require.True(t, open.KeepFocus, "a click's opening keeps the focus on the list")
	clicked, ok := rm.chatList.CursorChat()
	require.True(t, ok)
	require.Equal(t, clicked.ID, open.ChatID)

	// The opening itself does not move the focus to the chat pane.
	next, ocmd := rm.Update(open)
	runCmdTree(ocmd)
	rm = next.(RootModel)
	assert.Equal(t, FocusChatList, rm.focus)
	assert.Equal(t, open.ChatID, rm.currentChatID)
	// The panes agree with the focus: the list is focused, the chat is not.
	assert.True(t, rm.chatList.Focused())
	assert.False(t, rm.chat.Focused())
}

// TestEnter_KeepsMovingFocusAfterAClick reproduces the click-then-keyboard
// sequence: the click left the focus on the list, and a confirm from the
// keyboard still moves it into the chat pane.
func TestEnter_KeepsMovingFocusAfterAClick(t *testing.T) {
	m := newSizedRoot(t, 100, 30)
	m.focus = FocusChat

	// Click the third visible chat row; the opening leaves the focus on the list.
	next, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: 3, Button: tea.MouseLeft}))
	rm := next.(RootModel)
	require.Equal(t, FocusChatList, rm.focus)
	open, ok := cmd().(screens.OpenChatMsg)
	require.True(t, ok)
	require.True(t, open.KeepFocus)
	next, ocmd := rm.Update(open)
	runCmdTree(ocmd)
	rm = next.(RootModel)

	// A confirm from the keyboard opens too, and moves the focus: its message
	// carries no KeepFocus.
	nm, kcmd := rm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, kcmd)
	kopen, ok := kcmd().(screens.OpenChatMsg)
	require.True(t, ok, "expected OpenChatMsg, got %T", kcmd())
	require.False(t, kopen.KeepFocus, "a keyboard confirm sets no KeepFocus")
	next, ocmd = nm.(RootModel).Update(kopen)
	runCmdTree(ocmd)
	rm2 := next.(RootModel)
	assert.Equal(t, FocusChat, rm2.focus)
	assert.Equal(t, kopen.ChatID, rm2.currentChatID)
	assert.True(t, rm2.chat.Focused())
	assert.False(t, rm2.chatList.Focused())
}

func TestEnter_ChatList_FocusesChatPane(t *testing.T) {
	m := newSizedRoot(t, 100, 30)
	m.focus = FocusChatList
	m.chatList.SetCursor(1)

	nm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	open, ok := cmd().(screens.OpenChatMsg)
	require.True(t, ok, "expected OpenChatMsg, got %T", open)
	require.False(t, open.KeepFocus, "a keyboard confirm sets no KeepFocus")

	// The opening itself moves the focus to the chat pane, as before.
	next, ocmd := rm(nm).Update(open)
	runCmdTree(ocmd)
	rm2 := rm(next)
	assert.Equal(t, FocusChat, rm2.focus)
	assert.Equal(t, open.ChatID, rm2.currentChatID)
	assert.True(t, rm2.chat.Focused())
	assert.False(t, rm2.chatList.Focused())
}

func TestClick_TopicList_OpensTopicWithoutMovingFocus(t *testing.T) {
	o, m := forumRoot(t)
	m = enterForum(t, o, m)
	require.NotNil(t, m.forum)
	m.focus = FocusChatList
	require.True(t, m.forum.Focused())

	// Click a topic row in the middle pane (content row 1 -> screen y = 2).
	r, _, _ := m.hitTest(10, 2)
	require.Equal(t, regionChatList, r, "the topic pane must own the click")
	next, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: 10, Y: 2, Button: tea.MouseLeft}))
	rm := next.(RootModel)

	assert.Equal(t, FocusChatList, rm.focus)
	require.NotNil(t, cmd)
	open, ok := cmd().(screens.OpenTopicMsg)
	require.True(t, ok, "expected OpenTopicMsg, got %T", open)
	require.True(t, open.KeepFocus, "a click's opening keeps the focus on the list")
	topicID := open.TopicID

	// The opening itself leaves the focus on the topic list.
	nm, ocmd := rm.Update(open)
	runCmdTree(ocmd)
	r2, _ := o.drain(nm.(RootModel))
	rm2 := r2.(RootModel)
	assert.Equal(t, FocusChatList, rm2.focus)
	assert.Equal(t, domain.HistoryKey{ChatID: testForum, TopicID: topicID}, rm2.currentHistory())
	assert.True(t, rm2.forum.Focused())
	assert.False(t, rm2.chat.Focused())
}

func TestClick_ChatPane_FocusesChat(t *testing.T) {
	m := newSizedRoot(t, 100, 30)
	m.focus = FocusChatList

	// Click the messages region.
	next, _ := m.Update(tea.MouseClickMsg(tea.Mouse{X: 40, Y: 2, Button: tea.MouseLeft}))
	rm := next.(RootModel)

	assert.Equal(t, FocusChat, rm.focus)
	assert.True(t, rm.chat.Focused())
	assert.False(t, rm.chatList.Focused())
}

func TestOpenHistory_KeepFocusIsReadFromTheMessage(t *testing.T) {
	// KeepFocus rides on the message, not on the model: even when the history
	// is already open and openHistory takes its shortcut, a click's opening
	// leaves the focus on the list, and a keyboard confirm moves it.
	m := newSizedRoot(t, 100, 30)
	m.focus = FocusChatList
	m.currentChatID = 2

	// The click "opens" the already open chat: the focus stays on the list.
	next, _ := m.Update(screens.OpenChatMsg{ChatID: 2, KeepFocus: true})
	rm := next.(RootModel)
	assert.Equal(t, FocusChatList, rm.focus)

	// A keyboard confirm on the same history moves the focus to the chat pane.
	next, _ = rm.Update(screens.OpenChatMsg{ChatID: 2})
	rm = next.(RootModel)
	assert.Equal(t, FocusChat, rm.focus)
}

// rm is a convenience for the model dance Update forces on every test.
func rm(next tea.Model) RootModel { return next.(RootModel) }
