package components

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// EphemeralDraftOverlay draws a bot's streaming rich message: the partial
// document it is writing, with a marker that says it is not finished.
//
// It is an overlay rather than a row of the message list, and that is the whole
// design. A draft is not a message: it has no place in the history, no id any
// other message addresses, and it disappears without an update that says so
// (Telegram expires it 30 seconds after the last revision). Putting it in the
// list would make it a message-shaped thing that could be selected, replied to
// and scrolled past — none of which a draft can be the subject of.
//
// The document is rendered by the same block renderer the history uses, at the
// same width, so a streamed table or collage looks the way it will look once it
// lands.
type EphemeralDraftOverlay struct {
	// Blocks is the partial document. Empty with Placeholder set is the
	// "thinking" stage; empty without it means the bot has said nothing yet.
	Blocks []domain.PageBlock
	// Text is the flattened text of a draft that carried no blocks.
	Text string
	// Placeholder marks a draft whose document is only the thinking stage.
	Placeholder bool
	// Spinner is the animation frame drawn beside the marker.
	Spinner string
	// Height bounds the overlay. A streamed document can be longer than the
	// pane, and an overlay that grew without bound would push the history off
	// screen; the tail is trimmed and the trim is stated.
	Height int
}

// NewEphemeralDraftOverlay builds the overlay for a draft.
func NewEphemeralDraftOverlay(draft domain.EphemeralDraft, spinner string, height int) *EphemeralDraftOverlay {
	return &EphemeralDraftOverlay{
		Blocks:      draft.TextBlocks,
		Text:        draft.Text,
		Placeholder: draft.Placeholder,
		Spinner:     spinner,
		Height:      height,
	}
}

// View renders the overlay at the given width.
func (o *EphemeralDraftOverlay) View(width int) string {
	if width < 8 {
		width = 8
	}
	// The overlay is drawn under the history, so it keeps the same three-quarter
	// budget every other block does and is aligned with the messages above it.
	inner := width*3/4 - 4
	if inner < 8 {
		inner = 8
	}

	marker := o.Spinner
	if marker == "" {
		marker = "·"
	}
	head := theme.S().Timestamp.Render(marker + " generating…")

	var body []string
	switch {
	case o.Placeholder:
		label := strings.TrimSpace(o.Text)
		if label == "" {
			label = "working on a rich message"
		}
		body = append(body, theme.S().Quote.Italic(true).Render(label))
	case len(o.Blocks) > 0:
		body = renderDraftBlocks(o.Blocks, inner)
	case strings.TrimSpace(o.Text) != "":
		for _, line := range wrapTextLines(o.Text, inner) {
			body = append(body, theme.S().Body.Render(line))
		}
	default:
		body = append(body, theme.S().Quote.Italic(true).Render("nothing yet"))
	}

	lines := append([]string{head}, body...)
	// Trim from the top when the document outgrows the budget: what a stream is
	// currently writing is its tail, and the beginning has already been seen.
	trimmed := false
	if o.Height > 0 && len(lines) > o.Height {
		lines = lines[len(lines)-o.Height:]
		trimmed = true
	}
	if trimmed {
		// Say that something was cut, so a document that appears to start
		// mid-sentence is explained rather than looking broken.
		lines[0] = theme.S().Timestamp.Render("… (earlier lines scrolled past)")
	}

	// The overlay sits on its own surface, so it reads as something in progress
	// rather than as another message.
	style := theme.NewStyle().
		Background(theme.T().SurfaceToast).
		Foreground(theme.T().TextOnToast).
		Padding(0, 1)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		row := style.Render(line + theme.PadTo(lipgloss.Width(line), inner))
		out = append(out, row)
	}
	return strings.Join(out, "\n")
}

// Rows is how many rows the overlay occupies at a width, so the chat pane can
// take them out of the history's budget before it renders.
func (o *EphemeralDraftOverlay) Rows(width int) int {
	if o == nil {
		return 0
	}
	return strings.Count(o.View(width), "\n") + 1
}

// renderDraftBlocks draws a partial document with the same renderer the history
// uses, so a streamed block looks the way it will look once it lands.
func renderDraftBlocks(blocks []domain.PageBlock, width int) []string {
	ml := NewMessageList(0, width)
	lines := ml.renderRichBlocks(0, blocks, width)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimRight(line, " "))
	}
	return out
}
