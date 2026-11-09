package layout_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/stretchr/testify/assert"
)

func TestSplitHorizontal_Normal(t *testing.T) {
	left, right := layout.SplitHorizontal(100, 40, 0.3)
	assert.Equal(t, 30, left)
	assert.Equal(t, 70, right)
}

func TestSplitHorizontal_MinWidth(t *testing.T) {
	left, right := layout.SplitHorizontal(15, 40, 0.3)
	assert.GreaterOrEqual(t, left, 5)
	assert.GreaterOrEqual(t, right, 5)
	assert.Equal(t, 15, left+right)
}

func TestSplitVertical_Normal(t *testing.T) {
	top, bottom := layout.SplitVertical(37, 0.2)
	assert.Equal(t, 7, top)
	assert.Equal(t, 30, bottom)
	assert.Equal(t, 37, top+bottom)
}

func TestSplitVertical_MinHeights(t *testing.T) {
	// Small totals must still leave the bottom half able to carry a bordered
	// box: the chat list is the more important pane, so the top gives way.
	top, bottom := layout.SplitVertical(3, 0.2)
	assert.Equal(t, 1, top)
	assert.Equal(t, 2, bottom)
	assert.Equal(t, 3, top+bottom)
	// The smallest total that fits two bordered boxes splits evenly.
	top, bottom = layout.SplitVertical(4, 0.2)
	assert.Equal(t, 2, top)
	assert.Equal(t, 2, bottom)
}

func TestSplitThree_Normal(t *testing.T) {
	sidebar, mid, right := layout.SplitThree(100, 18, 0.30)
	assert.Equal(t, 18, sidebar)
	assert.Equal(t, 24, mid) // int(82 * 0.30) = 24
	assert.Equal(t, 58, right)
	assert.Equal(t, 100, sidebar+mid+right)
}

func TestSplitThree_TotalEqualsSum(t *testing.T) {
	sidebar, mid, right := layout.SplitThree(120, 18, 0.35)
	assert.Equal(t, 120, sidebar+mid+right)
}

func TestSplitThree_MinWidths(t *testing.T) {
	sidebar, mid, right := layout.SplitThree(25, 18, 0.30)
	assert.GreaterOrEqual(t, sidebar, 5)
	assert.GreaterOrEqual(t, mid, 5)
	assert.GreaterOrEqual(t, right, 5)
	assert.Equal(t, 25, sidebar+mid+right)
}
