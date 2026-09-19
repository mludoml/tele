package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// This file draws and navigates a message's inline keyboard.
//
// The keyboard is drawn under the bubble's content, inside the bubble: rows are
// the bot's rows, because re-flowing them would change what the keyboard means
// (the arrangement is the interface). A button's style comes from Telegram
// (primary/success/danger) and is rendered as a background fill, which is the
// only way a terminal can convey emphasis in a button-shaped cell.
//
// A button whose action this client cannot carry out is drawn disabled with its
// reason appended, never dropped: a keyboard with a hole in it misdescribes what
// the bot offered.

// richButtonPad is the blank cells framing a button's label inside its fill.
const richButtonPad = 1

// richButtonGap is the blank columns between two buttons of one row.
const richButtonGap = 1

// richButtonMinW is the narrowest a button cell may be drawn at, so a one-letter
// label still reads as a button rather than as a stray glyph.
const richButtonMinW = 5

// renderReplyMarkup draws the keyboard's rows at the given content width.
// selected is the flattened button index the cursor is on, or -1 when the
// keyboard is not being navigated.
func (ml *MessageList) renderReplyMarkup(markup *domain.ReplyMarkup, width, selected int) []string {
	if markup == nil || markup.Count() == 0 {
		return nil
	}
	if width < 1 {
		width = 1
	}
	var out []string
	index := 0
	for _, row := range markup.Rows {
		if len(row) == 0 {
			continue
		}
		out = append(out, ml.renderButtonRow(row, width, selected, index)...)
		index += len(row)
	}
	return out
}

// renderButtonRow draws one row of buttons, sizing the cells to fill the content
// width the way the bot's own client does: one button takes the whole width, and
// several share it. A row whose labels cannot fit its cells is drawn over as many
// lines as it needs, with the buttons of one line kept whole.
func (ml *MessageList) renderButtonRow(row []domain.KeyboardButton, width, selected, firstIndex int) []string {
	cells := buttonCellWidths(row, width)
	// A row of very long labels on a narrow pane cannot be split across lines
	// without changing the keyboard's shape, so the labels are clipped to their
	// cells and the fact is stated once.
	clipped := false
	lines := make([]string, 0, 1)
	var sb strings.Builder
	used := 0
	for i, btn := range row {
		cellW := cells[i]
		label := btn.Text
		if clippedLabel := clipLabel(label, cellW-2*richButtonPad); clippedLabel != label {
			clipped = true
			label = clippedLabel
		}
		sb.WriteString(ml.renderButtonCell(btn, label, cellW, selected >= 0 && selected == firstIndex+i))
		used += cellW
		if i < len(row)-1 {
			sb.WriteString(theme.Pad(richButtonGap))
			used += richButtonGap
		}
	}
	lines = append(lines, sb.String()+theme.PadTo(used, width))
	if clipped {
		lines = append(lines, theme.S().Timestamp.Render("(labels truncated to fit)"))
	}
	return lines
}

// buttonCellWidths splits the content width across a row's buttons. Every button
// gets the same share with the remainder spread left to right, so a row of two
// looks like two buttons rather than one wide and one narrow.
func buttonCellWidths(row []domain.KeyboardButton, width int) []int {
	n := len(row)
	avail := width - (n-1)*richButtonGap
	if avail < n*richButtonMinW {
		avail = n * richButtonMinW
	}
	base, rem := avail/n, avail%n
	out := make([]int, n)
	for i := range out {
		out[i] = base
		if i < rem {
			out[i]++
		}
	}
	return out
}

// renderButtonCell draws one button: a filled cell carrying its label, in the
// emphasis Telegram asked for (or a dim fill when the action is unsupported).
// A focused cell is drawn in reverse video: the one color change that reads on
// top of any emphasis, so the cursor is visible whichever style the bot chose.
func (ml *MessageList) renderButtonCell(btn domain.KeyboardButton, label string, cellW int, focused bool) string {
	style := buttonStyle(btn)
	if focused {
		style = style.Reverse(true)
	}
	inner := cellW - 2*richButtonPad
	if inner < 1 {
		inner = 1
	}
	text := label
	if w := lipgloss.Width(text); w > inner {
		text = xansi.Truncate(text, inner, "…")
	}
	// The cursor's marker replaces the cell's leading pad rather than being
	// added beside it, so a focused button does not shift the row it belongs
	// to and the row's width is the same either way.
	//
	// canvas:ok these spaces land inside body, which style.Render paints
	// whole a few lines down — theme.Pad's own background and reset would be
	// the base canvas colour, not this button's fill, and its reset would cut
	// that fill off partway through the cell (see internal/ui/theme/canvas.go).
	lead := strings.Repeat(" ", richButtonPad)
	if focused && richButtonPad > 0 {
		lead = "▸" + strings.Repeat(" ", richButtonPad-1) // canvas:ok same cell, same reason as above
	}
	trailing := inner + richButtonPad - lipgloss.Width(lead) - lipgloss.Width(text)
	if trailing < 0 {
		trailing = 0
	}
	body := lead + text + strings.Repeat(" ", trailing) // canvas:ok same cell, same reason as above
	return style.Render(body)
}

// buttonStyle is the fill a button is drawn in. The three emphases are the
// semantic colours the theme already owns for their meaning: primary is the
// accent, success the online/success green, danger the error red. An unsupported
// action is drawn in the quote tone: visibly inert rather than inviting.
func buttonStyle(btn domain.KeyboardButton) lipgloss.Style {
	if btn.Action.Kind == domain.ButtonActionNone {
		return theme.NewStyle().
			Foreground(theme.T().TextDim).
			Background(theme.T().SurfaceCode)
	}
	switch btn.Style {
	case domain.ButtonStylePrimary:
		return theme.NewStyle().
			Foreground(theme.T().TextOnSelected).
			Background(theme.T().Accent)
	case domain.ButtonStyleSuccess:
		return theme.NewStyle().
			Foreground(theme.T().TextOnSelected).
			Background(theme.T().StatusOnline)
	case domain.ButtonStyleDanger:
		return theme.NewStyle().
			Foreground(theme.T().TextOnSelected).
			Background(theme.T().StatusError)
	default:
		return theme.NewStyle().
			Foreground(theme.T().TextOnSelected).
			Background(theme.T().SurfaceSelected)
	}
}

// clipLabel trims a label to a cell width, keeping the last column for the
// ellipsis. A label that already fits is returned unchanged, which is how the
// caller learns that nothing was cut.
func clipLabel(label string, max int) string {
	if max < 1 {
		return ""
	}
	if lipgloss.Width(label) <= max {
		return label
	}
	return xansi.Truncate(label, max, "…")
}

// SelectedMessageMarkup returns the inline keyboard of the selected message, ok
// being false when it has none.
func (ml *MessageList) SelectedMessageMarkup() (*domain.ReplyMarkup, bool) {
	msg := ml.computeSelectedMsg()
	if msg == nil || msg.ReplyMarkup == nil || msg.ReplyMarkup.Count() == 0 {
		return nil, false
	}
	return msg.ReplyMarkup, true
}

// EnterButtonMode focuses the selected message's inline keyboard and puts the
// cursor on its first button. It reports whether there was a keyboard to focus:
// with none, the mode's keys stay inert rather than opening an empty mode.
func (ml *MessageList) EnterButtonMode() bool {
	msg := ml.computeSelectedMsg()
	if msg == nil || msg.ReplyMarkup == nil || msg.ReplyMarkup.Count() == 0 {
		return false
	}
	ml.buttonMsgID = msg.ID
	ml.buttonIndex = 0
	ml.invalidateHeights()
	return true
}

// ExitButtonMode leaves the keyboard and clears the cursor.
func (ml *MessageList) ExitButtonMode() {
	if ml.buttonMsgID == 0 {
		return
	}
	ml.buttonMsgID = 0
	ml.buttonIndex = 0
	ml.invalidateHeights()
}

// InButtonMode reports whether a keyboard currently has focus.
func (ml *MessageList) InButtonMode() bool { return ml.buttonMsgID != 0 }

// ButtonModeMessageID is the message whose keyboard is focused, 0 when none is.
func (ml *MessageList) ButtonModeMessageID() int { return ml.buttonMsgID }

// MoveButtonCursor moves the keyboard cursor by delta buttons, wrapping at both
// ends. Wrapping is deliberate: a keyboard is a small closed set and stopping at
// its edge would make the last button a dead end for the key that got there.
func (ml *MessageList) MoveButtonCursor(delta int) {
	if ml.buttonMsgID == 0 {
		return
	}
	markup, ok := ml.markupFor(ml.buttonMsgID)
	if !ok {
		ml.ExitButtonMode()
		return
	}
	n := markup.Count()
	next := (ml.buttonIndex + delta) % n
	if next < 0 {
		next += n
	}
	if next == ml.buttonIndex {
		return
	}
	ml.buttonIndex = next
	ml.invalidateHeights()
}

// SelectedButton returns the button the keyboard cursor is on, ok being false
// when no keyboard is focused or the message no longer holds one.
func (ml *MessageList) SelectedButton() (domain.KeyboardButton, bool) {
	if ml.buttonMsgID == 0 {
		return domain.KeyboardButton{}, false
	}
	markup, ok := ml.markupFor(ml.buttonMsgID)
	if !ok {
		return domain.KeyboardButton{}, false
	}
	buttons := markup.Buttons()
	if ml.buttonIndex < 0 || ml.buttonIndex >= len(buttons) {
		return domain.KeyboardButton{}, false
	}
	return buttons[ml.buttonIndex], true
}

// markupFor finds a message's current keyboard by id, wherever it sits in the
// loaded window. It is a lookup rather than a cached pointer because an edit can
// replace the keyboard under the cursor: the button a press sends must be the
// one on screen now, not the one that was there when the mode was entered.
func (ml *MessageList) markupFor(msgID int) (*domain.ReplyMarkup, bool) {
	for i := range ml.items {
		if ml.items[i].kind != itemMessage {
			continue
		}
		for _, m := range ml.items[i].parts {
			if m.ID == msgID {
				if m.ReplyMarkup == nil || m.ReplyMarkup.Count() == 0 {
					return nil, false
				}
				return m.ReplyMarkup, true
			}
		}
	}
	return nil, false
}
