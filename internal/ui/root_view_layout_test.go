package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// newRootForLayout builds a root model with n chats (unique, findable titles)
// and, when withFolders, two extra folders, at the given terminal size.
func newRootForLayout(t *testing.T, w, h int, withFolders bool) (RootModel, []domain.Chat) {
	t.Helper()
	st := store.NewMemory()
	for i := int64(1); i <= 12; i++ {
		st.SetChat(domain.Chat{ID: i, Peer: domain.Peer{ID: i, Type: domain.PeerUser}, Title: fmt.Sprintf("Chat%02d", i)})
	}
	chats := st.Chats()
	m := NewRootModel(false).WithScreen(ScreenMain)
	m.chatList.SetWindow(0, len(chats), rowsOf(chats))
	if withFolders {
		m.folderBar.SetFolders([]domain.FolderFilter{{ID: 11, Title: "Work"}, {ID: 12, Title: "Home"}})
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	require.Greater(t, next.(RootModel).width, 0)
	return next.(RootModel), chats
}

// drawnRow asserts chat i's title is rendered on screen row screenRow.
func drawnRow(t *testing.T, view string, title string) int {
	t.Helper()
	for row, line := range strings.Split(view, "\n") {
		if strings.Contains(ansi.Strip(line), title) {
			return row
		}
	}
	t.Fatalf("title %q not found in the drawn view", title)
	return -1
}

// TestMouseClick_RowsMatchDrawnView pins the mouse hit-testing to what View()
// draws: the screen row a chat's title is rendered on must be the row whose
// click selects that chat, and the click must move focus into the chat list.
// Exercise every remainder of the folders split: at heights 26 and 27 a
// two-row discrepancy in its input changes the rounded folder height by one.
func TestMouseClick_RowsMatchDrawnView(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {100, 25}, {100, 26}, {100, 27}, {100, 28}, {200, 60}, {100, 30}} {
		w, h := size[0], size[1]
		for _, withFolders := range []bool{true, false} {
			name := fmt.Sprintf("%dx%d/folders=%v", w, h, withFolders)
			t.Run(name, func(t *testing.T) {
				m, chats := newRootForLayout(t, w, h, withFolders)
				viewTop := m.View().Content
				require.Equal(t, h, len(strings.Split(viewTop, "\n")),
					"the terminal must not clip or scroll the rendered frame")
				lay := computeLayout(w, h, m.chat.ComposerHeight(), m.HasFolders())
				require.False(t, lay.chatListBox.Top == 0 && lay.chatListBox.Height == 0,
					"layout must describe where the chat-list box is drawn")

				// The chat list is freshly built, so its first window rows are
				// chats 0..n in order. Row 0 of the list's content sits at the
				// box's drawn top edge plus the border; chat 0's title must be
				// drawn exactly there.
				firstContentRow := lay.chatListBox.Top + 1
				assert.Equal(t, firstContentRow, drawnRow(t, viewTop, chats[0].Title),
					"chat 0 must be drawn on the first row inside the chat-list box")

				// Clicking the row a chat is drawn on selects that chat.
				for i := 0; i < 3 && i < len(chats); i++ {
					title := chats[i].Title
					view := m.View().Content
					row := drawnRow(t, view, title)
					// Sanity: the drawn row must lie inside the drawn box, and
					// hit-testing must agree that this cell is the chat list.
					assert.GreaterOrEqual(t, row, lay.chatListBox.Top+1)
					assert.Less(t, row, lay.chatListBox.Top+lay.chatListBox.Height)
					region, _, _ := m.hitTest(3, row)
					require.Equal(t, regionChatList, region, "row %d for chat %d", row, i)

					next, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: row, Button: tea.MouseLeft}))
					rm := next.(RootModel)
					assert.Equal(t, i, rm.chatList.Cursor(),
						"clicking the row where %q is drawn must select chat %d", title, i)
					assert.Equal(t, FocusChatList, rm.focus, "click must focus the chat list")
					if i == 0 {
						// Chat 0 was already the cursor's target; selecting it
						// can legitimately emit a confirm for the same chat.
						_ = cmd
					}
					// Re-focus the chat pane so the next click has a focus
					// transition to prove again.
					res, _ := rm.focusPane(FocusChat)
					m = res.(RootModel)
				}
			})
		}
	}
}

// TestMouseClick_FoldersMatchDrawnView pins the folders bar the same way: the
// row a folder's title is drawn on selects that folder and focuses the bar.
func TestMouseClick_FoldersMatchDrawnView(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {100, 25}, {100, 26}, {100, 27}, {100, 28}, {200, 60}} {
		w, h := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			m, _ := newRootForLayout(t, w, h, true)
			require.True(t, m.HasFolders())
			folders := m.folderBar.Folders()
			lay := computeLayout(w, h, m.chat.ComposerHeight(), m.HasFolders())
			require.True(t, lay.hasFolders)
			view := m.View().Content
			require.True(t, m.folderBar.ScrollInfo().Visible > 0,
				"the folders pane must show at least one row")
			for i, f := range folders {
				si := m.folderBar.ScrollInfo()
				if i < si.Offset || i >= si.Offset+si.Visible {
					continue // scrolled out of the tiny folders pane viewport
				}
				row := drawnRow(t, view, f.Title)
				assert.GreaterOrEqual(t, row, lay.folders.Top)
				assert.Less(t, row, lay.folders.Top+lay.folders.Height,
					"folder %d must be drawn inside the folders pane", i)
				next, _ := m.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: row, Button: tea.MouseLeft}))
				rm := next.(RootModel)
				assert.Equal(t, FocusFolders, rm.focus, "click must focus the folders bar")
			}
		})
	}
}
