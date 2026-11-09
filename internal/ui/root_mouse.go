package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// region identifies a clickable/scrollable area of the main screen.
type region int

const (
	regionNone region = iota
	regionFolders
	regionChatList
	regionMessages
	regionComposer
	regionStatusBar
)

func rectContains(r components.Rect, x, y int) bool {
	return x >= r.Left && x < r.Left+r.Width && y >= r.Top && y < r.Top+r.Height
}

// hitTest maps a screen cell (x, y) to the pane region under it and the
// coordinates local to that region's content rect. Returns regionNone for
// borders, the status bar gaps, or anywhere outside a pane.
func (m RootModel) hitTest(x, y int) (region, int, int) {
	lay := computeLayout(m.width, m.height, m.chat.ComposerHeight(),
		m.folderBar != nil && m.folderBar.HasFolders())
	switch {
	case lay.hasFolders && rectContains(lay.folders, x, y):
		return regionFolders, x - lay.folders.Left, y - lay.folders.Top
	case rectContains(lay.chatList, x, y):
		return regionChatList, x - lay.chatList.Left, y - lay.chatList.Top
	case rectContains(lay.messages, x, y):
		return regionMessages, x - lay.messages.Left, y - lay.messages.Top
	case rectContains(lay.composer, x, y):
		return regionComposer, x - lay.composer.Left, y - lay.composer.Top
	case rectContains(lay.statusBar, x, y):
		return regionStatusBar, x - lay.statusBar.Left, y - lay.statusBar.Top
	}
	return regionNone, 0, 0
}

// mouseBlocked reports whether mouse events should be ignored: on the login
// screen and while any overlay/modal owns input. Overlay-menu clicks are Phase B.
func (m RootModel) mouseBlocked() bool {
	if m.screen == ScreenLogin {
		return true
	}
	return m.searchModel != nil || m.filePicker != nil || m.contextMenu != nil ||
		m.chatMenu != nil || m.reactionPicker != nil || m.photoViewer != nil ||
		m.videoPlayer != nil || m.openPicker != nil || m.mentionPopup != nil
}

// handleMouse dispatches wheel and left-click events to the pane under the cursor.
func (m RootModel) handleMouse(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.mouseBlocked() {
		return m, nil
	}
	switch e := msg.(type) {
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(e.Mouse())
	case tea.MouseClickMsg:
		return m.handleMouseClick(e.Mouse())
	}
	return m, nil
}

// handleMouseWheel scrolls the pane under the cursor by reusing the existing
// up/down key actions, independent of which pane holds keyboard focus.
func (m RootModel) handleMouseWheel(mo tea.Mouse) (tea.Model, tea.Cmd) {
	var action keys.Action
	switch mo.Button {
	case tea.MouseWheelUp:
		action = keys.ActionUp
	case tea.MouseWheelDown:
		action = keys.ActionDown
	default:
		return m, nil
	}
	r, _, _ := m.hitTest(mo.X, mo.Y)
	switch r {
	case regionChatList:
		if m.forum != nil {
			// A forum's topic list fills the pane (#275).
			newPane, cmd := m.forum.Update(keys.ActionMsg{Action: action})
			m.forum = newPane.(*screens.TopicListModel)
			m.syncTopicListWindow()
			return m, cmd
		}
		newPane, cmd := m.chatList.Update(keys.ActionMsg{Action: action})
		m.chatList = newPane.(*screens.ChatListModel)
		// The wheel moves the cursor as surely as a key does, so it has to ask
		// for the next window just the same.
		m.syncChatListWindow()
		return m, cmd
	case regionMessages:
		newPane, cmd := m.chat.Update(keys.ActionMsg{Action: action})
		m.chat = newPane.(*screens.ChatModel)
		return m, tea.Batch(cmd, m.markReadCmd())
	case regionFolders:
		if m.folderBar != nil {
			newPane, cmd := m.folderBar.Update(keys.ActionMsg{Action: action})
			m.folderBar = newPane.(*screens.FoldersModel)
			return m, cmd
		}
	}
	return m, nil
}

// handleMouseClick handles a left-button press: it moves keyboard focus into the
// clicked pane (focus-follows-click), opens the clicked chat — leaving the
// focus on the list the click landed on instead of moving to the chat pane —
// focuses/blurs the composer, and otherwise selects the pane. Non-left buttons
// are ignored.
func (m RootModel) handleMouseClick(mo tea.Mouse) (tea.Model, tea.Cmd) {
	if mo.Button != tea.MouseLeft {
		return m, nil
	}
	// Toasts float above everything, so a click on a toast action wins over
	// focus-follows-click.
	if msg, ok := m.toasts.HitTest(mo.X, mo.Y); ok {
		return m, func() tea.Msg { return msg }
	}
	r, _, localY := m.hitTest(mo.X, mo.Y)
	// Folder borders focus the pane, but never confirm a row.
	if r == regionNone {
		lay := computeLayout(m.width, m.height, m.chat.ComposerHeight(), m.HasFolders())
		box := components.Rect{
			Left: lay.folders.Left - 1, Top: lay.folders.Top - 1,
			Width: lay.folders.Width + 2, Height: lay.folders.Height + 2,
		}
		if lay.hasFolders && rectContains(box, mo.X, mo.Y) {
			r, localY = regionFolders, -1
		}
	}

	// Any click outside the composer blurs it.
	var blurCmd tea.Cmd
	if r != regionComposer && m.chat.ComposerFocused() {
		m.vimState.Mode = keys.ModeNormal
		m.statusBar.SetMode(keys.ModeNormal)
		newPane, cmd := m.chat.Update(keys.ActionMsg{Action: keys.ActionNormal})
		m.chat = newPane.(*screens.ChatModel)
		blurCmd = cmd
	}

	switch r {
	case regionChatList:
		// The click itself still moves the focus onto the list it landed on;
		// only the opening must not carry it into the chat pane.
		res, fcmd := m.focusPane(FocusChatList)
		m = res.(RootModel)
		if m.forum != nil {
			idx, ok := m.forum.TopicIndexAtViewportRow(localY)
			if !ok {
				return m, tea.Batch(blurCmd, fcmd)
			}
			m.forum.SetCursor(idx)
			newPane, ocmd := m.forum.Update(keys.ActionMsg{Action: keys.ActionConfirm})
			m.forum = newPane.(*screens.TopicListModel)
			// A click's opening leaves the focus on the list the click landed
			// on; a confirm from the keyboard moves it to the chat, as before.
			return m, tea.Batch(blurCmd, fcmd, keepFocusCmd(ocmd))
		}
		idx, ok := m.chatList.ChatIndexAtViewportRow(localY)
		if !ok {
			return m, tea.Batch(blurCmd, fcmd)
		}
		m.chatList.SetCursor(idx)
		newPane, ocmd := m.chatList.Update(keys.ActionMsg{Action: keys.ActionConfirm})
		m.chatList = newPane.(*screens.ChatListModel)
		// A click's opening leaves the focus on the list the click landed on;
		// a confirm from the keyboard moves it to the chat, as before.
		return m, tea.Batch(blurCmd, fcmd, keepFocusCmd(ocmd))

	case regionMessages:
		res, fcmd := m.focusPane(FocusChat)
		return res, tea.Batch(blurCmd, fcmd)

	case regionComposer:
		res, fcmd := m.focusPane(FocusChat)
		m = res.(RootModel)
		if !m.chat.ComposerFocused() {
			m.vimState.Mode = keys.ModeInsert
			m.statusBar.SetMode(keys.ModeInsert)
			newPane, icmd := m.chat.Update(keys.ActionMsg{Action: keys.ActionInsert})
			m.chat = newPane.(*screens.ChatModel)
			return m, tea.Batch(fcmd, icmd)
		}
		return m, fcmd

	case regionFolders:
		res, fcmd := m.focusPane(FocusFolders)
		m = res.(RootModel)
		idx, ok := m.folderBar.FolderIndexAtViewportRow(localY)
		if !ok {
			return m, tea.Batch(blurCmd, fcmd)
		}
		m.folderBar.SetCursor(idx)
		newPane, cmd := m.folderBar.Update(keys.ActionMsg{Action: keys.ActionConfirm})
		m.folderBar = newPane.(*screens.FoldersModel)
		return m, tea.Batch(blurCmd, fcmd, keepFocusCmd(cmd))
	}
	return m, blurCmd
}

// keepFocusCmd tags list-opening and folder-selection messages produced by a
// mouse click, so the selection leaves focus on the pane the click landed on.
// Any other message passes through unchanged, so a click never swallows it.
// A nil cmd passes through: there may be nothing to open.
func keepFocusCmd(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		switch m := msg.(type) {
		case screens.OpenChatMsg:
			m.KeepFocus = true
			return m
		case screens.OpenTopicMsg:
			m.KeepFocus = true
			return m
		case screens.FolderSelectedMsg:
			m.KeepFocus = true
			return m
		}
		return msg
	}
}
