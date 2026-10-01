package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// The kind names are the Bot API's own, because the docs coverage table and the
// unsupported-block placeholder both print them, and a name that drifts from
// the server's vocabulary makes the table unreadable.
func TestBlockKind_NamesEveryKind(t *testing.T) {
	want := map[domain.BlockKind]string{
		domain.BlockKindParagraph:    "paragraph",
		domain.BlockKindHeading:      "heading",
		domain.BlockKindPreformatted: "pre",
		domain.BlockKindFooter:       "footer",
		domain.BlockKindDivider:      "divider",
		domain.BlockKindAnchor:       "anchor",
		domain.BlockKindKicker:       "kicker",
		domain.BlockKindTitle:        "title",
		domain.BlockKindSubtitle:     "subtitle",
		domain.BlockKindAuthorDate:   "author_date",
		domain.BlockKindBlockquote:   "blockquote",
		domain.BlockKindPullquote:    "pullquote",
		domain.BlockKindList:         "list",
		domain.BlockKindOrderedList:  "ordered_list",
		domain.BlockKindListItem:     "list_item",
		domain.BlockKindTable:        "table",
		domain.BlockKindDetails:      "details",
		domain.BlockKindCollage:      "collage",
		domain.BlockKindSlideshow:    "slideshow",
		domain.BlockKindMap:          "map",
		domain.BlockKindPhoto:        "photo",
		domain.BlockKindVideo:        "video",
		domain.BlockKindAudio:        "audio",
		domain.BlockKindMath:         "mathematical_expression",
		domain.BlockKindThinking:     "thinking",
		domain.BlockKindUnsupported:  "unsupported",
	}
	// Every declared kind is named here, so a kind added without a name is a
	// failure rather than an "unknown" nobody notices.
	assert.Len(t, want, int(domain.BlockKindCount))
	for kind, name := range want {
		assert.Equal(t, name, kind.String(), "kind %d", kind)
	}
	assert.Equal(t, "unknown", domain.BlockKind(99).String())
}

func TestButtonStyle_NamesEachStyle(t *testing.T) {
	assert.Equal(t, "default", domain.ButtonStyleDefault.String())
	assert.Equal(t, "primary", domain.ButtonStylePrimary.String())
	assert.Equal(t, "success", domain.ButtonStyleSuccess.String())
	assert.Equal(t, "danger", domain.ButtonStyleDanger.String())
	assert.Equal(t, "default", domain.ButtonStyle(99).String())
}

// A markup's button order is row-major: the keyboard is read the way it is
// drawn, and a caller that walks Buttons() to move a cursor depends on it.
func TestReplyMarkup_FlattensInReadingOrder(t *testing.T) {
	rm := &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{
		{{Text: "a"}, {Text: "b"}},
		{{Text: "c"}},
	}}
	got := rm.Buttons()
	assert.Len(t, got, 3)
	assert.Equal(t, []string{"a", "b", "c"}, []string{got[0].Text, got[1].Text, got[2].Text})
	assert.Equal(t, 3, rm.Count())
}

// A message with no keyboard is the ordinary case, and both accessors must
// answer it without the caller nil-checking first.
func TestReplyMarkup_NilIsEmpty(t *testing.T) {
	var rm *domain.ReplyMarkup
	assert.Nil(t, rm.Buttons())
	assert.Zero(t, rm.Count())
	assert.Zero(t, (&domain.ReplyMarkup{}).Count())
}
