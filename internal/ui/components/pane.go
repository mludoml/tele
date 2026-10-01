package components

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// paneTrackChar and paneThumbChar draw a frameless pane's scrollbar: a thin
// line, with the thumb one weight heavier than the track it runs on.
const (
	paneTrackChar = "│"
	paneThumbChar = "┃"
)

// RenderPane renders a pane with no frame, w columns by h rows. The first row
// is a header holding title and, two cells after it, suffix (pre-styled; dropped
// when it does not fit). The remaining h-1 rows hold content, inset one column
// on the left so it does not touch whatever is drawn beside the pane, followed
// by a one-column gap and the scrollbar column; the gap keeps a line drawn at
// the content's right edge (a bubble border) from merging with the scrollbar.
// The content area is therefore w-3 columns by h-1 rows.
//
// titleFg colours the title; nil leaves it body text. The scrollbar is drawn
// only while the content overflows: trackFg paints the track rows and thumbFg
// the thumb. Outside the track, and whenever the content fits, the column is
// blank canvas.
func RenderPane(content, title, suffix string, titleFg, trackFg, thumbFg color.Color, w, h int, sb *Scrollbar) string {
	innerW := w - 3
	innerH := h - 1
	if innerW < 0 || innerH < 0 {
		return ""
	}

	titleStyle := theme.S().Body
	if titleFg != nil {
		titleStyle = titleStyle.Foreground(titleFg)
	}
	title = xansi.Truncate(title, innerW, "…")
	header := theme.Pad(1) + titleStyle.Render(title)
	used := 1 + lipgloss.Width(title)
	if suffix != "" && used+2+lipgloss.Width(suffix) <= w {
		header += theme.Pad(2) + suffix
		used += 2 + lipgloss.Width(suffix)
	}
	header += theme.PadTo(used, w)

	thumbStart, thumbSize, showThumb := 0, 0, false
	if sb != nil {
		thumbStart, thumbSize, showThumb = sb.Info.Thumb(sb.TrackLen)
	}
	track := theme.NewStyle().Foreground(trackFg).Render(paneTrackChar)
	thumb := theme.NewStyle().Foreground(thumbFg).Render(paneThumbChar)

	lines := strings.Split(content, "\n")
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	lines = lines[:innerH]

	result := make([]string, 0, h)
	result = append(result, header)
	for ri, l := range lines {
		// As in RenderBox: the pad follows the line's own reset, the only place
		// from which the canvas survives. The trailing cell is the gap column.
		row := theme.Pad(1) + l + theme.PadTo(lipgloss.Width(l), innerW) + theme.Pad(1)
		switch {
		case !showThumb || ri < sb.TrackTop || ri >= sb.TrackTop+sb.TrackLen:
			row += theme.Pad(1)
		case ri-sb.TrackTop >= thumbStart && ri-sb.TrackTop < thumbStart+thumbSize:
			row += thumb
		default:
			row += track
		}
		result = append(result, row)
	}
	return strings.Join(result, "\n")
}
