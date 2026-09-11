package layout

const minPaneWidth = 5

// SplitHorizontal divides totalWidth into (left, right).
// leftRatio is a fraction 0..1. Each pane is at least minPaneWidth.
func SplitHorizontal(totalWidth, _ int, leftRatio float64) (left, right int) {
	left = int(float64(totalWidth) * leftRatio)
	if left < minPaneWidth {
		left = minPaneWidth
	}
	right = totalWidth - left
	if right < minPaneWidth {
		right = minPaneWidth
		left = totalWidth - right
	}
	return left, right
}

// SplitVertical divides totalHeight into (top, bottom).
// topRatio is a fraction 0..1. Each pane is at least 2 rows, so that both
// halves can still carry a bordered box when totalHeight is small.
func SplitVertical(totalHeight int, topRatio float64) (top, bottom int) {
	minH := 2
	top = int(float64(totalHeight) * topRatio)
	if top < minH {
		top = minH
	}
	bottom = totalHeight - top
	if bottom < minH {
		bottom = minH
		top = totalHeight - bottom
	}
	return top, bottom
}

// SplitThree divides totalWidth into (sidebar, mid, right).
// sidebarW is fixed. mid gets midRatio of the remaining space.
// Each pane is at least minPaneWidth.
func SplitThree(totalWidth, sidebarW int, midRatio float64) (sidebar, mid, right int) {
	sidebar = sidebarW
	if sidebar < minPaneWidth {
		sidebar = minPaneWidth
	}
	remaining := totalWidth - sidebar
	if remaining < 2*minPaneWidth {
		remaining = 2 * minPaneWidth
		sidebar = totalWidth - remaining
	}
	mid = int(float64(remaining) * midRatio)
	if mid < minPaneWidth {
		mid = minPaneWidth
	}
	right = remaining - mid
	if right < minPaneWidth {
		right = minPaneWidth
		mid = remaining - right
	}
	return sidebar, mid, right
}
