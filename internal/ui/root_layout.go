package ui

import (
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
)

// foldersRatio is the share of the left column's height given to the folders
// bar when the account has folders; the chat list gets the rest (1:4 with the
// default 0.2).
const foldersRatio = 0.2

// paneLayout holds the on-screen content rectangles (inside the pane borders)
// of the main-screen panes. It is the single source of truth for pane geometry,
// shared by the resize plumbing and mouse hit-testing.
type paneLayout struct {
	hasFolders bool
	folders    components.Rect // valid only when hasFolders is true
	chatList   components.Rect
	messages   components.Rect // message-list sub-rect of the chat pane
	composer   components.Rect // composer sub-rect of the chat pane
	statusBar  components.Rect
}

// computeLayout returns the content rectangles of the main-screen panes for a
// terminal of the given size. composerHeight is the composer's visual height in
// rows. Each pane is drawn in a bordered box, so its content rect is inset by
// one cell on every side; the pane content height is height-3 (one status-bar
// row plus a top and bottom border).
//
// With folders, the screen is two columns: the left one stacks the folders bar
// on top of the chat list (1:4), the right one holds the chat pane. Without
// folders, the chat list takes the whole left column.
func computeLayout(width, height, composerHeight int, folderBarVisible bool) paneLayout {
	contentH := height - 3
	if contentH < 0 {
		contentH = 0
	}
	msgH := contentH - composerHeight
	if msgH < 0 {
		msgH = 0
	}
	lay := paneLayout{
		hasFolders: folderBarVisible,
		statusBar:  components.Rect{Top: height - 1, Left: 0, Height: 1, Width: width},
	}
	if folderBarVisible {
		foldersH, chatsH := layout.SplitVertical(contentH, foldersRatio)
		leftW, chatW := layout.SplitHorizontal(width, height, 0.30)
		lay.folders = components.Rect{Top: 1, Left: 1, Height: foldersH - 2, Width: leftW - 2}
		lay.chatList = components.Rect{Top: foldersH + 1, Left: 1, Height: chatsH - 2, Width: leftW - 2}
		lay.messages = components.Rect{Top: 1, Left: leftW + 1, Height: msgH, Width: chatW - 2}
		lay.composer = components.Rect{Top: 1 + msgH, Left: leftW + 1, Height: composerHeight, Width: chatW - 2}
	} else {
		leftW, rightW := layout.SplitHorizontal(width, height, 0.30)
		lay.chatList = components.Rect{Top: 1, Left: 1, Height: contentH, Width: leftW - 2}
		lay.messages = components.Rect{Top: 1, Left: leftW + 1, Height: msgH, Width: rightW - 2}
		lay.composer = components.Rect{Top: 1 + msgH, Left: leftW + 1, Height: composerHeight, Width: rightW - 2}
	}
	return lay
}
