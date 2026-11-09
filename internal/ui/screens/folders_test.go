package screens_test

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testFolders = []domain.FolderFilter{
	{ID: 1, Title: "Work"},
	{ID: 2, Title: "Personal"},
}

func TestFolders_ShowsAllChatsAndFolders(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders(testFolders)
	m.SetSize(16, 20)
	view := m.View()
	assert.Contains(t, view, "All Chats")
	assert.Contains(t, view, "Work")
	assert.Contains(t, view, "Personal")
}

func TestFolders_ShowsBadge(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders(testFolders)
	m.SetSize(16, 20)
	m.SetUnreadCounts(map[int]int{1: 3})
	view := m.View()
	assert.Contains(t, view, "[3]")
}

func TestFolders_DefaultSelectedIsAllChats(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders(testFolders)
	assert.Nil(t, m.SelectedFilter())
}

func TestFolders_Down_MovesCursor(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders(testFolders)
	m.SetFocused(true)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	fm := newPane.(*screens.FoldersModel)
	assert.Equal(t, 1, fm.Cursor())
}

func TestFolders_Scrolls_KeepsCursorVisible(t *testing.T) {
	many := make([]domain.FolderFilter, 10)
	for i := range many {
		many[i] = domain.FolderFilter{ID: i + 1, Title: fmt.Sprintf("Folder %d", i+1)}
	}
	m := screens.NewFoldersModel()
	m.SetFolders(many)
	m.SetSize(16, 3) // 3 visible rows
	m.SetFocused(true)

	// Walk the cursor to the last folder: the offset must follow so the
	// cursor row stays inside the pane.
	cur := m
	for range 11 {
		newPane, _ := cur.Update(keys.ActionMsg{Action: keys.ActionDown})
		cur = newPane.(*screens.FoldersModel)
	}
	view := cur.View()
	assert.Contains(t, view, "Folder 10", "cursor row must be visible after scrolling down")
	assert.NotContains(t, view, "All Chats", "scrolled-out rows must be clipped")

	// Walk back up: the offset follows the cursor to the top of the list.
	for range 11 {
		newPane, _ := cur.Update(keys.ActionMsg{Action: keys.ActionUp})
		cur = newPane.(*screens.FoldersModel)
	}
	view = cur.View()
	assert.Contains(t, view, "All Chats", "top of the list must be visible again")
}

func TestFolders_ShrunkPane_ClampsOffsetToCursor(t *testing.T) {
	many := make([]domain.FolderFilter, 10)
	for i := range many {
		many[i] = domain.FolderFilter{ID: i + 1, Title: fmt.Sprintf("Folder %d", i+1)}
	}
	m := screens.NewFoldersModel()
	m.SetFolders(many)
	m.SetSize(16, 10)
	m.SetFocused(true)

	// Scroll down, then shrink the pane: the cursor must stay visible.
	cur := m
	for range 8 {
		newPane, _ := cur.Update(keys.ActionMsg{Action: keys.ActionDown})
		cur = newPane.(*screens.FoldersModel)
	}
	cur.SetSize(16, 2)
	view := cur.View()
	assert.Contains(t, view, "Folder 8", "shrunk pane must keep the cursor row visible")
}

func TestFolders_Enter_EmitsFolderSelectedMsg(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders(testFolders)
	m.SetFocused(true)
	// Move to "Work" (index 1)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	fm := newPane.(*screens.FoldersModel)
	_, cmd := fm.Update(keys.ActionMsg{Action: keys.ActionConfirm})
	require.NotNil(t, cmd)
	msg := cmd()
	sel, ok := msg.(screens.FolderSelectedMsg)
	require.True(t, ok)
	require.NotNil(t, sel.Filter)
	assert.Equal(t, 1, sel.Filter.ID)
}

func TestFolders_Enter_AllChats_EmitsNilFilter(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders(testFolders)
	m.SetFocused(true)
	_, cmd := m.Update(keys.ActionMsg{Action: keys.ActionConfirm})
	require.NotNil(t, cmd)
	msg := cmd()
	sel, ok := msg.(screens.FolderSelectedMsg)
	require.True(t, ok)
	assert.Nil(t, sel.Filter, "All Chats returns nil filter")
}

func TestFolders_NotFocused_IgnoresKeys(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders(testFolders)
	m.SetFocused(false)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	fm := newPane.(*screens.FoldersModel)
	assert.Equal(t, 0, fm.Cursor())
}

func TestFolders_ExactFitNameHasSpaceBeforeBadge(t *testing.T) {
	// "Russia news" = 11 visual cols; prefix "  " = 2; badge "[3]" = 3; separator = 1
	// exact fit: width = 11+2+3+1 = 17 → nameWidth = 17-2-3-1 = 11 = exact fit, no padRight padding
	// space must come from the explicit separator, not from padRight
	f := domain.FolderFilter{ID: 1, Title: "Russia news"}
	m := screens.NewFoldersModel()
	m.SetFolders([]domain.FolderFilter{f})
	m.SetSize(17, 10)
	m.SetUnreadCounts(map[int]int{1: 3})
	view := m.View()
	assert.Contains(t, view, "Russia news [3]", "non-truncated name must have space before badge")
}

func TestFolders_View_ShowsFilledArrowOnActiveItem(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders(testFolders)
	m.SetSize(16, 20)
	view := m.View()
	lines := strings.Split(view, "\n")
	require.NotEmpty(t, lines)
	assert.Contains(t, lines[0], "▶", "active item must show ▶")
	assert.NotContains(t, lines[0], "▸", "old icon ▸ must not appear")
}

func TestFolders_TruncatedBadgeFitsWidth(t *testing.T) {
	long := domain.FolderFilter{ID: 3, Title: "Russia newsletter"}
	m := screens.NewFoldersModel()
	m.SetFolders([]domain.FolderFilter{long})
	m.SetSize(16, 20) // inner width: outer(18) - 2 borders
	m.SetUnreadCounts(map[int]int{3: 24})
	view := m.View()
	// The title must be truncated (with ellipsis) and badge must fit
	assert.Contains(t, view, "… [24]", "truncated name + badge must be separated by a space")
	assert.NotContains(t, view, "newsletter", "full title must not be visible")
	// Each rendered line must fit in the inner width
	for _, line := range strings.Split(view, "\n") {
		w := lipgloss.Width(line)
		assert.LessOrEqual(t, w, 16, "line %q must fit in inner width 16", line)
	}
}

func TestFolders_ArchiveAppearsOnlyWhenPresent(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders([]domain.FolderFilter{{ID: 7, Title: "Work"}})

	// No archived chats yet: no Archive entry.
	for _, f := range m.Folders() {
		require.NotEqual(t, domain.ArchiveFolderID, f.ID, "archive must be hidden when empty")
	}

	// Archived chat exists: Archive entry appears last.
	m.SetArchivePresent(true)
	folders := m.Folders()
	last := folders[len(folders)-1]
	assert.Equal(t, domain.ArchiveFolderID, last.ID)
	assert.Equal(t, "Archive", last.Title)

	// Becomes empty again: Archive entry disappears.
	m.SetArchivePresent(false)
	for _, f := range m.Folders() {
		require.NotEqual(t, domain.ArchiveFolderID, f.ID)
	}
}

func TestFolders_ArchivePreservesSelectionByID(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders([]domain.FolderFilter{{ID: 7, Title: "Work"}})
	m.SetArchivePresent(true)

	// Move cursor to the Work folder (index 1). Update returns a new pane.
	m.SetFocused(true)
	p, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	m = p.(*screens.FoldersModel)
	require.Equal(t, 1, m.Cursor())

	// Toggling archive presence must not move the cursor off Work.
	m.SetArchivePresent(false)
	assert.Equal(t, 1, m.Cursor())
}

func TestFoldersModel_ScrollInfo(t *testing.T) {
	m := screens.NewFoldersModel()
	m.SetFolders([]domain.FolderFilter{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}})
	m.SetSize(18, 10)
	info := m.ScrollInfo()
	assert.Equal(t, 10, info.Visible)
	assert.Equal(t, 0, info.Offset)
	assert.LessOrEqual(t, info.Total, info.Visible) // fits => no thumb
}
