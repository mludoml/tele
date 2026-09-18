package components

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// A streamed document is drawn by the same block renderer the history uses, so a
// table or a heading looks the way it will look once the message lands.
func TestEphemeralDraftOverlay_RendersTheDocument(t *testing.T) {
	overlay := NewEphemeralDraftOverlay(domain.EphemeralDraft{
		ChatID: 1, ID: 7,
		TextBlocks: []domain.PageBlock{
			{Kind: domain.BlockKindHeading, Level: 2, Text: &domain.RichText{Text: "Streaming"}},
			{Kind: domain.BlockKindTable, Table: &domain.TableBlock{Rows: [][]domain.TableCell{
				{{Text: domain.RichText{Text: "a"}, IsHeader: true}, {Text: domain.RichText{Text: "b"}}},
			}}},
		},
	}, "·", 10)

	out := stripRichANSI(overlay.View(60))
	assert.Contains(t, out, "Streaming")
	assert.Contains(t, out, "generating…")
	assert.Contains(t, out, "a")
	assert.Contains(t, out, "b")
}

// The thinking stage is what the bot says before it has anything: it is drawn as
// a placeholder rather than as an empty document.
func TestEphemeralDraftOverlay_ThinkingStage(t *testing.T) {
	overlay := NewEphemeralDraftOverlay(domain.EphemeralDraft{
		ChatID: 1, ID: 7, Placeholder: true, Text: "considering",
		TextBlocks: []domain.PageBlock{{Kind: domain.BlockKindThinking, Text: &domain.RichText{Text: "considering"}}},
	}, "·", 10)

	out := stripRichANSI(overlay.View(60))
	assert.Contains(t, out, "considering")
	assert.Contains(t, out, "generating…")
}

// A draft that carried only flattened text (no blocks) is still shown: the text
// is what the bot wrote, whatever form it arrived in.
func TestEphemeralDraftOverlay_TextOnly(t *testing.T) {
	overlay := NewEphemeralDraftOverlay(domain.EphemeralDraft{ChatID: 1, ID: 7, Text: "partial sentence"}, "", 10)
	out := stripRichANSI(overlay.View(60))
	assert.Contains(t, out, "partial sentence")
}

// A draft longer than its budget keeps its tail and says that something was cut:
// a document that appears to start mid-sentence must be explained rather than
// looking broken.
func TestEphemeralDraftOverlay_TrimsFromTheTop(t *testing.T) {
	blocks := make([]domain.PageBlock, 0, 30)
	for i := 0; i < 30; i++ {
		blocks = append(blocks, domain.PageBlock{
			Kind: domain.BlockKindParagraph,
			Text: &domain.RichText{Text: "line " + string(rune('a'+i%26))},
		})
	}
	overlay := NewEphemeralDraftOverlay(domain.EphemeralDraft{ChatID: 1, ID: 7, TextBlocks: blocks}, "·", 5)

	out := stripRichANSI(overlay.View(60))
	assert.LessOrEqual(t, strings.Count(out, "\n")+1, 5)
	assert.Contains(t, out, "scrolled past", "the trim is stated")
	assert.Equal(t, overlay.Rows(60), strings.Count(out, "\n")+1)
}

// The overlay's row count and its render agree, which is what lets the chat pane
// take the room out of the history before drawing it.
func TestEphemeralDraftOverlay_RowsMatchesTheRender(t *testing.T) {
	cases := []domain.EphemeralDraft{
		{ChatID: 1, ID: 1, Placeholder: true},
		{ChatID: 1, ID: 1, Text: "one line"},
		{ChatID: 1, ID: 1, TextBlocks: []domain.PageBlock{
			{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: strings.Repeat("word ", 60)}},
		}},
	}
	for _, d := range cases {
		overlay := NewEphemeralDraftOverlay(d, "·", 12)
		assert.Equal(t, strings.Count(overlay.View(50), "\n")+1, overlay.Rows(50))
	}
}

// A nil overlay occupies nothing, so the chat pane's arithmetic needs no special
// case for "no draft".
func TestEphemeralDraftOverlay_NilRows(t *testing.T) {
	var overlay *EphemeralDraftOverlay
	assert.Zero(t, overlay.Rows(60))
}
