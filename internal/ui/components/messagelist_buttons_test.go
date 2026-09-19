package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// The keyboard is drawn as the bot arranged it: rows stay rows, because the
// arrangement is part of what the keyboard means.
func TestRenderReplyMarkup_KeepsTheBotsRows(t *testing.T) {
	ml := richList(40, 30)
	markup := &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{
		{{Text: "a"}, {Text: "b"}},
		{{Text: "c"}},
	}}
	lines := ml.renderReplyMarkup(markup, 40, -1)
	require.Len(t, lines, 2, "two rows, one line each")
	assert.Contains(t, lines[0], "a")
	assert.Contains(t, lines[0], "b")
	assert.Contains(t, lines[1], "c")
}

// A button's emphasis is a fill, which is the only button-shaped signalling a
// terminal has: the three Telegram styles must not come out identical.
func TestRenderReplyMarkup_StylesAreDistinct(t *testing.T) {
	ml := richList(40, 30)
	render := func(style domain.ButtonStyle) string {
		return ml.renderReplyMarkup(&domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{
			{{Text: "Go", Style: style, Action: domain.ButtonAction{Kind: domain.ButtonActionCallback}}},
		}}, 30, -1)[0]
	}
	primary := render(domain.ButtonStylePrimary)
	success := render(domain.ButtonStyleSuccess)
	danger := render(domain.ButtonStyleDanger)
	neutral := render(domain.ButtonStyleDefault)

	assert.NotEqual(t, primary, neutral)
	assert.NotEqual(t, success, neutral)
	assert.NotEqual(t, danger, neutral)
	assert.NotEqual(t, primary, danger)
	assert.NotEqual(t, primary, success)
	assert.NotEqual(t, success, danger)
}

// An unsupported button is drawn, disabled, with its reason: dropping it would
// leave the row with a hole and misdescribe what the bot offered.
func TestRenderReplyMarkup_UnsupportedStaysVisibleAndExplains(t *testing.T) {
	ml := richList(60, 30)
	msg := domain.Message{
		ID: 1, ChatID: 1, Date: time.Unix(0, 0), Text: "hi",
		ReplyMarkup: &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{{
			{Text: "Send phone", Action: domain.ButtonAction{Kind: domain.ButtonActionNone, Reason: "asks for your phone number"}},
		}}},
	}
	ml.SetMessages([]domain.Message{msg})
	out := strings.Join(ml.renderMessage(msg, false), "\n")
	assert.Contains(t, out, "Send phone", "the button is still drawn")
	assert.Contains(t, out, "asks for your phone number", "and says why it cannot be used")
}

// A row of buttons fills the content width, so the bubble's border never cuts
// through the keyboard.
func TestRenderReplyMarkup_FillsTheWidth(t *testing.T) {
	ml := richList(36, 30)
	markup := &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{
		{{Text: "one"}, {Text: "two"}, {Text: "three"}},
		{{Text: "a very long label that will not fit its cell"}},
	}}
	for i, line := range ml.renderReplyMarkup(markup, 36, -1) {
		assert.LessOrEqual(t, lineWidth(line), 36, "line %d", i)
	}
}

// Navigation wraps at both ends: a keyboard is a small closed set, and stopping
// at its edge would make the last button a dead end for the key that got there.
func TestButtonMode_NavigationWraps(t *testing.T) {
	ml := richList(40, 30)
	msg := domain.Message{ID: 5, ChatID: 1, Date: time.Unix(0, 0),
		ReplyMarkup: &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{
			{{Text: "a"}, {Text: "b"}},
			{{Text: "c"}},
		}}}
	ml.SetMessages([]domain.Message{msg})

	require.True(t, ml.EnterButtonMode())
	assert.Equal(t, 5, ml.ButtonModeMessageID())

	btn, ok := ml.SelectedButton()
	require.True(t, ok)
	assert.Equal(t, "a", btn.Text)

	ml.MoveButtonCursor(1)
	btn, _ = ml.SelectedButton()
	assert.Equal(t, "b", btn.Text, "reading order crosses the row boundary")

	ml.MoveButtonCursor(1)
	btn, _ = ml.SelectedButton()
	assert.Equal(t, "c", btn.Text)

	ml.MoveButtonCursor(1)
	btn, _ = ml.SelectedButton()
	assert.Equal(t, "a", btn.Text, "past the last button comes the first")

	ml.MoveButtonCursor(-1)
	btn, _ = ml.SelectedButton()
	assert.Equal(t, "c", btn.Text, "and backwards from the first comes the last")
}

// With no keyboard there is nothing to focus, so the key must not open an empty
// mode.
func TestButtonMode_NoKeyboard(t *testing.T) {
	ml := richList(40, 30)
	ml.SetMessages([]domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(0, 0), Text: "plain"}})
	assert.False(t, ml.EnterButtonMode())
	assert.False(t, ml.InButtonMode())
	_, ok := ml.SelectedButton()
	assert.False(t, ok)
}

// Entering the mode and leaving it again must clear the cursor, so a later
// selection does not inherit a keyboard it never opened.
func TestButtonMode_ExitClears(t *testing.T) {
	ml := richList(40, 30)
	ml.SetMessages([]domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(0, 0),
		ReplyMarkup: &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{{{Text: "a"}}}}}})
	require.True(t, ml.EnterButtonMode())
	ml.ExitButtonMode()
	assert.False(t, ml.InButtonMode())
	assert.Zero(t, ml.ButtonModeMessageID())
}

// An edit can replace the keyboard under the cursor: the press must send the
// button that is on screen now, not the one that was there when the mode opened.
func TestButtonMode_FollowsAnEditThatShortensTheKeyboard(t *testing.T) {
	ml := richList(40, 30)
	msg := domain.Message{ID: 5, ChatID: 1, Date: time.Unix(0, 0),
		ReplyMarkup: &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{
			{{Text: "a"}, {Text: "b"}, {Text: "c"}},
		}}}
	ml.SetMessages([]domain.Message{msg})
	require.True(t, ml.EnterButtonMode())
	ml.MoveButtonCursor(2)

	// The bot rewrites its keyboard with a single button: the cursor now indexes
	// past the end, and the press must not send a button that is gone.
	edited := msg
	edited.ReplyMarkup = &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{{{Text: "only"}}}}
	ml.SetMessages([]domain.Message{edited})

	_, ok := ml.SelectedButton()
	assert.False(t, ok, "the cursor no longer names a button that exists")
}

// The cursor's marker is drawn inside the button's fill, so a focused button does
// not shift the row it belongs to.
func TestRenderReplyMarkup_CursorDoesNotShiftTheRow(t *testing.T) {
	ml := richList(30, 30)
	markup := &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{{{Text: "one"}, {Text: "two"}}}}
	unfocused := ml.renderReplyMarkup(markup, 30, -1)
	focused := ml.renderReplyMarkup(markup, 30, 0)
	require.Len(t, unfocused, 1)
	require.Len(t, focused, 1)
	assert.Equal(t, lineWidth(unfocused[0]), lineWidth(focused[0]))
	assert.NotEqual(t, unfocused[0], focused[0], "the cursor is visibly somewhere")
}

// A focused cell is a color change (reverse video), not just a leading glyph
// swapped into an otherwise separately-styled substring: a pre-rendered
// substring embedded in the cell body would carry its own reset code and cut
// the fill off partway through the cell once the outer style wraps it.
func TestRenderButtonCell_FocusIsReverseVideoWithNoBrokenFillMidCell(t *testing.T) {
	ml := richList(30, 30)
	btn := domain.KeyboardButton{Text: "Go", Action: domain.ButtonAction{Kind: domain.ButtonActionCallback}}

	unfocused := ml.renderButtonCell(btn, "Go", 12, false)
	focused := ml.renderButtonCell(btn, "Go", 12, true)

	assert.Contains(t, focused, "\x1b[7", "a focused cell must carry the reverse-video SGR code")
	assert.NotContains(t, unfocused, "\x1b[7", "an unfocused cell must not")
	// A reset in the middle of the cell (from a substring styled and closed
	// before the outer wrap) would leave everything after it unstyled; the
	// only reset allowed is the one terminating the whole cell.
	assert.Equal(t, 1, strings.Count(focused, "\x1b[m"), "exactly one reset, at the very end of the cell")
}

// Most shipped themes claim the canvas (a base background + text pair set
// deliberately, see internal/ui/theme/canvas.go), and a button's fill must
// survive one under it. theme.Pad — right for a container's own gap — carries
// that base background and its own reset; used *inside* a cell a different
// style is about to paint, the reset cuts the cell's fill off before the
// label ever appears, and the button reads as plain text (reported live: "bez
// fokusu nie wygląda na button, jest sam tekst").
func TestRenderButtonCell_SurvivesARealCanvasTheme(t *testing.T) {
	bg, err := theme.ParseColor("#1a1b26")
	require.NoError(t, err)
	fg, err := theme.ParseColor("#c0caf5")
	require.NoError(t, err)
	th := theme.TeleDark
	th.Name = "canvas-probe"
	th.Background, th.Text = bg, fg
	t.Cleanup(func() { theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight}); theme.Apply(true) })
	theme.SetSlots(theme.Slots{Dark: th, Light: th})
	theme.Apply(true)

	ml := richList(30, 30)
	btn := domain.KeyboardButton{Text: "Go", Action: domain.ButtonAction{Kind: domain.ButtonActionCallback}}

	cell := ml.renderButtonCell(btn, "Go", 12, false)

	assert.Equal(t, 1, strings.Count(cell, "\x1b[m"),
		"the fill must be one continuous run ending in a single reset, not cut short by a canvas pad's own reset")
}

// A keyboard is drawn under ordinary message text too: it is not a rich-message
// feature, and a bot's plain message carries one just as often.
func TestReplyMarkup_DrawnUnderPlainText(t *testing.T) {
	ml := richList(40, 30)
	msg := domain.Message{ID: 5, ChatID: 1, Date: time.Unix(0, 0), Text: "press a button",
		ReplyMarkup: &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{{{Text: "Go"}}}}}
	ml.SetMessages([]domain.Message{msg})
	out := strings.Join(ml.renderMessage(msg, false), "\n")
	assert.Contains(t, out, "press a button")
	assert.Contains(t, out, "Go")
}

// cellsBefore is the display width of a line's text before the first occurrence
// of glyph, or -1 when it is absent.
func cellsBefore(line, glyph string) int {
	i := strings.Index(line, glyph)
	if i < 0 {
		return -1
	}
	return lipgloss.Width(line[:i])
}

// A bordered table's rows and its header rule must be exactly as wide as each
// other: the separator's width is charged to the columns, so one drawn narrower
// than it is charged leaves every row short of the bubble.
func TestRenderRichTable_BorderedGridAligns(t *testing.T) {
	ml := richList(40, 20)
	blocks := []domain.PageBlock{{Kind: domain.BlockKindTable, Table: &domain.TableBlock{
		Bordered: true,
		Rows: [][]domain.TableCell{
			{{Text: domain.RichText{Text: "aaaa"}, IsHeader: true}, {Text: domain.RichText{Text: "bb"}, IsHeader: true}},
			{{Text: domain.RichText{Text: "c"}}, {Text: domain.RichText{Text: "d"}}},
		},
	}}}
	lines := ml.renderRichBlocks(1, blocks, 40)
	require.Len(t, lines, 3)
	// The header rule joins the verticals, so its ┼ stands where the header's │
	// stands: the two must line up column for column. Measured in cells rather
	// than bytes, because a box-drawing glyph is three bytes wide.
	headerPipe := cellsBefore(stripRichANSI(lines[0]), "│")
	assert.Equal(t, headerPipe, cellsBefore(stripRichANSI(lines[1]), "┼"),
		"the rule's join must sit under the header's separator")
	assert.Equal(t, headerPipe, cellsBefore(stripRichANSI(lines[2]), "│"))
}
