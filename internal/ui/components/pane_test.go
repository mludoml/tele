package components_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func TestRenderPane_ExactFootprint(t *testing.T) {
	// Every row is exactly w wide and there are exactly h rows, whatever the
	// content: the pane is joined beside the list column by width, and a short
	// or long row would shear everything to its right.
	out := components.RenderPane("short\n"+strings.Repeat("x", 5), "Title", "", nil, nil, nil, 8, 5, nil)
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 5)
	for i, l := range lines {
		assert.Equal(t, 8, lipgloss.Width(l), "row %d", i)
	}
	// Header row, then content inset one column: no frame glyphs anywhere.
	assert.Equal(t, " Title  ", xansi.Strip(lines[0]))
	assert.Equal(t, " short  ", xansi.Strip(lines[1]))
	assert.NotContains(t, xansi.Strip(out), "│")
}

func TestRenderPane_ContentTrimmedToHeightMinusHeader(t *testing.T) {
	out := components.RenderPane("a\nb\nc\nd", "", "", nil, nil, nil, 6, 3, nil)
	lines := strings.Split(xansi.Strip(out), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, " a    ", lines[1])
	assert.Equal(t, " b    ", lines[2])
}

func TestRenderPane_ScrollbarThinLineInLastColumn(t *testing.T) {
	// 4 content rows, track covers rows 0..2; 10 total rows, 3 visible, at top.
	sb := &components.Scrollbar{Info: components.ScrollInfo{Total: 10, Visible: 3, Offset: 0}, TrackTop: 0, TrackLen: 3}
	out := components.RenderPane("a\nb\nc\nd", "T", "", nil, nil, nil, 6, 5, sb)
	lines := strings.Split(xansi.Strip(out), "\n")
	full := components.RenderPane("abc\nabc\nabc\nabc", "T", "", nil, nil, nil, 6, 5, sb)
	// Content filling its width is still one blank column away from the bar.
	assert.Equal(t, " abc ┃", strings.Split(xansi.Strip(full), "\n")[1])
	require.Len(t, lines, 5)
	assert.True(t, strings.HasSuffix(lines[1], "┃"), "thumb at the top of the track")
	assert.True(t, strings.HasSuffix(lines[2], "│"), "track below the thumb")
	assert.True(t, strings.HasSuffix(lines[3], "│"), "track below the thumb")
	assert.True(t, strings.HasSuffix(lines[4], " "), "row outside the track is blank")
	assert.NotContains(t, out, "█")
}

func TestRenderPane_NoScrollbarWhenContentFits(t *testing.T) {
	sb := &components.Scrollbar{Info: components.ScrollInfo{Total: 2, Visible: 4}, TrackTop: 0, TrackLen: 4}
	out := xansi.Strip(components.RenderPane("a\nb", "T", "", nil, nil, nil, 6, 5, sb))
	assert.NotContains(t, out, "│")
	assert.NotContains(t, out, "┃")
}

func TestRenderPane_SuffixDroppedWhenItDoesNotFit(t *testing.T) {
	wide := xansi.Strip(components.RenderPane("", "Name", "●", nil, nil, nil, 12, 2, nil))
	assert.Equal(t, " Name  ●    ", strings.Split(wide, "\n")[0])

	narrow := strings.Split(components.RenderPane("", "Name", "●", nil, nil, nil, 7, 2, nil), "\n")[0]
	assert.Equal(t, 7, lipgloss.Width(narrow))
	assert.NotContains(t, narrow, "●")
}

func TestRenderPane_LongTitleTruncatedToWidth(t *testing.T) {
	head := strings.Split(components.RenderPane("", "A very long chat title", "", nil, nil, nil, 10, 2, nil), "\n")[0]
	assert.Equal(t, 10, lipgloss.Width(head))
	assert.Contains(t, xansi.Strip(head), "…")
}
