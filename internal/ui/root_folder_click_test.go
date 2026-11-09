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
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

func clickFolder(t *testing.T, m RootModel, x, y int) RootModel {
	t.Helper()
	next, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	m = next.(RootModel)
	if cmd != nil {
		var apply func(tea.Cmd)
		apply = func(cmd tea.Cmd) {
			if cmd == nil {
				return
			}
			switch msg := cmd().(type) {
			case tea.BatchMsg:
				for _, child := range msg {
					apply(child)
				}
			case screens.FolderSelectedMsg:
				require.True(t, msg.KeepFocus)
				next, _ := m.Update(msg)
				m = next.(RootModel)
			}
		}
		apply(cmd)
	}
	assert.Equal(t, FocusFolders, m.focus)
	assert.True(t, m.folderBar.Focused())
	return m
}

func TestClick_FolderSelectionAndEmptyRows(t *testing.T) {
	m := newSizedRoot(t, 100, 40)
	m.folderBar.SetFolders([]domain.FolderFilter{{ID: 11, Title: "FolderOne"}, {ID: 12, Title: "FolderTwo"}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(RootModel)
	lay := computeLayout(m.width, m.height, m.chat.ComposerHeight(), true)
	for row, id := range []int{0, 11, 12, 0} {
		if row == 3 {
			row = 0
		}
		res, _ := m.focusPane(FocusChat)
		m = clickFolder(t, res.(RootModel), lay.folders.Left, lay.folders.Top+row)
		assert.Equal(t, id, m.activeFolder)
		assert.Equal(t, row, m.folderBar.Cursor())
	}
	// Blank content and every edge of the frame only focus the folders pane.
	for _, p := range [][2]int{
		{lay.folders.Left, lay.folders.Top + len(m.folderBar.Folders())},
		{lay.folders.Left, lay.folders.Top - 1},
		{lay.folders.Left, lay.folders.Top + lay.folders.Height},
		{lay.folders.Left - 1, lay.folders.Top + 1},
		{lay.folders.Left + lay.folders.Width, lay.folders.Top + 1},
	} {
		res, _ := m.focusPane(FocusChat)
		m = clickFolder(t, res.(RootModel), p[0], p[1])
		assert.Zero(t, m.activeFolder)
		assert.Zero(t, m.folderBar.Cursor())
	}
	// Keyboard confirm still transfers focus to the chat list.
	pane, cmd := m.folderBar.Update(keys.ActionMsg{Action: keys.ActionConfirm})
	m.folderBar = pane.(*screens.FoldersModel)
	next, _ = m.Update(cmd())
	assert.Equal(t, FocusChatList, next.(RootModel).focus)
}

// This exercises the scrolling viewport when composed with feat/ui-v2. The
// mouse topic alone deliberately retains upstream's unscrolled folder view.
func TestClick_ScrolledFoldersMatchRenderedRows(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {100, 25}, {100, 26}, {100, 27}, {100, 28}, {200, 60}, {100, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := newSizedRoot(t, size[0], size[1])
			folders := make([]domain.FolderFilter, 40)
			for i := range folders {
				folders[i] = domain.FolderFilter{ID: i + 1, Title: fmt.Sprintf("Folder%02d", i+1)}
			}
			m.folderBar.SetFolders(folders)
			next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = next.(RootModel)
			lay := computeLayout(m.width, m.height, m.chat.ComposerHeight(), true)
			for range 35 {
				next, _ = m.Update(tea.MouseWheelMsg(tea.Mouse{X: lay.folders.Left, Y: lay.folders.Top, Button: tea.MouseWheelDown}))
				m = next.(RootModel)
			}
			info := m.folderBar.ScrollInfo()
			if info.Offset == 0 {
				t.Skip("upstream folders do not scroll without feat/ui-v2")
			}
			require.Greater(t, info.Offset, 0)
			for index := info.Offset; index < min(info.Offset+info.Visible, len(m.folderBar.Folders())); index++ {
				title := m.folderBar.Folders()[index].Title
				row := -1
				for y, line := range strings.Split(m.View().Content, "\n") {
					if strings.Contains(ansi.Strip(line), title) {
						row = y
						break
					}
				}
				require.GreaterOrEqual(t, row, lay.folders.Top, "%s must be drawn", title)
				require.Less(t, row, lay.folders.Top+lay.folders.Height)
				res, _ := m.focusPane(FocusChat)
				m = clickFolder(t, res.(RootModel), lay.folders.Left, row)
				assert.Equal(t, m.folderBar.Folders()[index].ID, m.activeFolder, "drawn row %d: %s", row, title)
				assert.Equal(t, index, m.folderBar.Cursor())
			}
		})
	}
}
