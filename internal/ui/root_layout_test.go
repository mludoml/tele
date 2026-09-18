package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func TestComputeLayout_TwoPane(t *testing.T) {
	// width=100, height=30, composerHeight=3, no folders.
	// SplitHorizontal(100, .30) -> left=30, right=70. contentH=height-3=27.
	lay := computeLayout(100, 30, 3, false)

	assert.False(t, lay.hasFolders)
	// Chat list content sits inside the left box (border inset 1 on each side).
	assert.Equal(t, 1, lay.chatList.Top)
	assert.Equal(t, 1, lay.chatList.Left)
	assert.Equal(t, 28, lay.chatList.Width) // leftW-2
	assert.Equal(t, 27, lay.chatList.Height)
	// Messages occupy the top of the right box, above the composer.
	assert.Equal(t, 1, lay.messages.Top)
	assert.Equal(t, 31, lay.messages.Left)   // leftW+1
	assert.Equal(t, 68, lay.messages.Width)  // rightW-2
	assert.Equal(t, 24, lay.messages.Height) // contentH-composerHeight
	// Composer is the bottom composerHeight rows of the right box.
	assert.Equal(t, 25, lay.composer.Top) // 1+messages.Height
	assert.Equal(t, 31, lay.composer.Left)
	assert.Equal(t, 68, lay.composer.Width)
	assert.Equal(t, 3, lay.composer.Height)
	// Status bar is the final row, full width.
	assert.Equal(t, 29, lay.statusBar.Top) // height-1
	assert.Equal(t, 0, lay.statusBar.Left)
	assert.Equal(t, 100, lay.statusBar.Width)
	assert.Equal(t, 1, lay.statusBar.Height)
}

func TestComputeLayout_StackedFolders(t *testing.T) {
	// width=120, height=40, composerHeight=3, folders on.
	// contentH=height-3=37. SplitVertical(37, .2): folders=int(37*.2)=7, chats=30.
	// SplitHorizontal(120, .30): left=36, chat=84.
	lay := computeLayout(120, 40, 3, true)
	assert.True(t, lay.hasFolders)
	// Folders bar: full left-column width, top of the column.
	assert.Equal(t, 1, lay.folders.Top)    // 0+1
	assert.Equal(t, 1, lay.folders.Left)   // 0+1
	assert.Equal(t, 34, lay.folders.Width) // leftW(36)-2
	assert.Equal(t, 5, lay.folders.Height) // foldersH(7)-2
	// Chat list: below the folders bar, same column.
	assert.Equal(t, 8, lay.chatList.Top)  // foldersH(7)+1
	assert.Equal(t, 1, lay.chatList.Left) // 0+1
	assert.Equal(t, 34, lay.chatList.Width)
	assert.Equal(t, 28, lay.chatList.Height) // chatsH(30)-2
	// Chat pane: right column, unchanged.
	assert.Equal(t, 37, lay.messages.Left)  // leftW(36)+1
	assert.Equal(t, 82, lay.messages.Width) // chatW(84)-2
}

func TestWindowSize_SetsPaneSizesFromLayout(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	m := NewRootModel(st, 50, false).WithScreen(ScreenMain)
	m.chatList.SetWindow(0, len(st.Chats()), rowsOf(st.Chats()))

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	rm := next.(RootModel)

	lay := computeLayout(100, 30, rm.chat.ComposerHeight(), false)
	// The chat list's content height must equal the layout it will be hit-tested against.
	assert.Equal(t, lay.chatList.Height, rm.chatList.Height())
}
