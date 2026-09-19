package components

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// richList is a list at a size, with the rich renderer in the state the app
// ships in.
func richList(width, height int) *MessageList {
	return NewMessageList(height, width)
}

// The two things that must never disagree: msgHeight's estimate and what
// renderMessage draws. This is the same invariant TestMsgHeightMatchesRenderMessage
// guards for ordinary messages (issue #115), asked of every block kind.
func TestMsgHeightMatchesRenderMessage_RichBlocks(t *testing.T) {
	cases := map[string][]domain.PageBlock{
		"paragraph":      {richPara("hello")},
		"long paragraph": {richPara(strings.Repeat("word ", 40))},
		"headings": {
			{Kind: domain.BlockKindHeading, Level: 1, Text: &domain.RichText{Text: "one"}},
			{Kind: domain.BlockKindHeading, Level: 3, Text: &domain.RichText{Text: "three"}},
			{Kind: domain.BlockKindHeading, Level: 6, Text: &domain.RichText{Text: "six"}},
		},
		"pre with language": {
			{Kind: domain.BlockKindPreformatted, Language: "go", Text: &domain.RichText{Text: "x := 1\ny := 2"}},
		},
		"footer and divider": {
			{Kind: domain.BlockKindFooter, Text: &domain.RichText{Text: "the end"}},
			{Kind: domain.BlockKindDivider},
		},
		"anchor draws nothing": {
			{Kind: domain.BlockKindAnchor, Label: "intro"},
			richPara("after"),
		},
		"kicker and title": {
			{Kind: domain.BlockKindKicker, Text: &domain.RichText{Text: "news"}},
			{Kind: domain.BlockKindTitle, Text: &domain.RichText{Text: "Report"}},
		},
		"author date": {
			{Kind: domain.BlockKindAuthorDate, Text: &domain.RichText{Text: "Ada"}, Date: 1700000000},
		},
		"blockquote with credit": {
			{
				Kind:   domain.BlockKindBlockquote,
				Text:   &domain.RichText{Text: "quoted words here"},
				Credit: &domain.RichText{Text: "someone"},
			},
		},
		"blockquote blocks": {
			{
				Kind: domain.BlockKindBlockquote,
				Children: []domain.PageBlock{
					richPara("inner"),
					{Kind: domain.BlockKindPreformatted, Text: &domain.RichText{Text: "code"}},
				},
				Credit: &domain.RichText{Text: "src"},
			},
		},
		"pullquote": {
			{Kind: domain.BlockKindPullquote, Text: &domain.RichText{Text: "pulled"}},
		},
		"unordered list": {
			{
				Kind: domain.BlockKindList,
				Children: []domain.PageBlock{
					{Kind: domain.BlockKindListItem, Text: &domain.RichText{Text: "one"}},
					{Kind: domain.BlockKindListItem, Text: &domain.RichText{Text: "two"}, Checkbox: true, Checked: true},
				},
			},
		},
		"list with a wrapped item": {
			{
				Kind: domain.BlockKindList,
				Children: []domain.PageBlock{
					{Kind: domain.BlockKindListItem, Text: &domain.RichText{Text: strings.Repeat("long ", 30)}},
				},
			},
		},
		"ordered list markers": {
			{
				Kind:   domain.BlockKindOrderedList,
				Start:  3,
				Marker: "i",
				Children: []domain.PageBlock{
					{Kind: domain.BlockKindListItem, Text: &domain.RichText{Text: "third"}},
					{Kind: domain.BlockKindListItem, Text: &domain.RichText{Text: "fourth"}},
				},
			},
		},
		"list item holding nested blocks": {
			{
				Kind: domain.BlockKindList,
				Children: []domain.PageBlock{
					{Kind: domain.BlockKindListItem, Children: []domain.PageBlock{richPara("para")}},
				},
			},
		},
		"table": {
			{Kind: domain.BlockKindTable, Table: &domain.TableBlock{Rows: [][]domain.TableCell{
				{richHeaderCell("Item"), richHeaderCell("Value")},
				{{Text: domain.RichText{Text: "Sales"}}, {Text: domain.RichText{Text: "142"}, Align: "right"}},
			}}},
		},
		"table wrapping cells": {
			{Kind: domain.BlockKindTable, Table: &domain.TableBlock{
				Bordered: true,
				Title:    &domain.RichText{Text: "Numbers"},
				Rows: [][]domain.TableCell{
					{richHeaderCell(strings.Repeat("wide ", 25)), {Text: domain.RichText{Text: "b"}}},
					{{Text: domain.RichText{Text: "c"}}, {Text: domain.RichText{Text: "d"}}},
				},
			}},
		},
		"table too many columns": {
			{Kind: domain.BlockKindTable, Table: &domain.TableBlock{Rows: [][]domain.TableCell{{
				{Text: domain.RichText{Text: "a"}}, {Text: domain.RichText{Text: "b"}},
				{Text: domain.RichText{Text: "c"}}, {Text: domain.RichText{Text: "d"}},
				{Text: domain.RichText{Text: "e"}}, {Text: domain.RichText{Text: "f"}},
				{Text: domain.RichText{Text: "g"}}, {Text: domain.RichText{Text: "h"}},
				{Text: domain.RichText{Text: "i"}}, {Text: domain.RichText{Text: "j"}},
			}}}},
		},
		"table with spanning cells": {
			{Kind: domain.BlockKindTable, Table: &domain.TableBlock{Rows: [][]domain.TableCell{
				{{Text: domain.RichText{Text: "across"}, Colspan: 2}},
				{{Text: domain.RichText{Text: "a"}}, {Text: domain.RichText{Text: "b"}}},
			}}},
		},
		"details collapsed": {
			{
				Kind:     domain.BlockKindDetails,
				Text:     &domain.RichText{Text: "More"},
				Children: []domain.PageBlock{richPara("hidden")},
			},
		},
		"details open": {
			{
				Kind:        domain.BlockKindDetails,
				Text:        &domain.RichText{Text: "More"},
				DetailsOpen: true,
				Children:    []domain.PageBlock{richPara("shown")},
			},
		},
		"math": {
			{Kind: domain.BlockKindMath, Text: &domain.RichText{Text: `\frac{a}{b}`}},
		},
		"unsupported": {
			{Kind: domain.BlockKindUnsupported, Label: "pageBlockEmbed"},
		},
		"thinking": {
			{Kind: domain.BlockKindThinking, Text: &domain.RichText{Text: "working"}},
		},
		"map": {
			{
				Kind:    domain.BlockKindMap,
				Map:     &domain.MapBlock{Lat: 52.22977, Long: 21.01178, Zoom: 12},
				Caption: &domain.RichText{Text: "the office"},
			},
		},
		"media without bytes": {
			{
				Kind:    domain.BlockKindPhoto,
				Media:   &domain.MediaRef{Kind: domain.MediaPhoto},
				Photo:   &domain.PhotoRef{ID: 1, ThumbSize: "m"},
				Caption: &domain.RichText{Text: "a photo"},
			},
		},
		"empty document": nil,
	}
	for name, blocks := range cases {
		t.Run(name, func(t *testing.T) {
			ml := richList(40, 50)
			msg := domain.Message{ID: 1, ChatID: 1, Date: time.Unix(0, 0), RichBlocks: blocks}
			ml.SetMessages([]domain.Message{msg})
			assert.Equal(t, len(ml.renderMessage(msg, false)), ml.msgHeight(msg),
				"msgHeight must equal the rendered line count")
		})
	}
}

func richPara(text string) domain.PageBlock {
	return domain.PageBlock{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: text}}
}

func richHeaderCell(text string) domain.TableCell {
	return domain.TableCell{Text: domain.RichText{Text: text}, IsHeader: true}
}

func TestMsgHeightMatchesRenderMessage_ReplyMarkup(t *testing.T) {
	now := time.Now()
	markups := []*domain.ReplyMarkup{
		{Rows: [][]domain.KeyboardButton{{{Text: "Yes", Style: domain.ButtonStylePrimary}}}},
		{Rows: [][]domain.KeyboardButton{
			{{Text: "a"}, {Text: "b"}, {Text: "c"}},
			{{Text: "d"}},
		}},
		{Rows: [][]domain.KeyboardButton{{
			{Text: "an unsupported action", Action: domain.ButtonAction{Kind: domain.ButtonActionNone, Reason: "asks for your location"}},
		}}},
		{Rows: [][]domain.KeyboardButton{{
			{Text: strings.Repeat("very long label ", 10)},
		}}},
	}
	for i, markup := range markups {
		ml := richList(30, 40)
		msg := domain.Message{ID: 1, ChatID: 1, Date: now, Text: "body", ReplyMarkup: markup}
		ml.SetMessages([]domain.Message{msg})
		assert.Equal(t, len(ml.renderMessage(msg, false)), ml.msgHeight(msg), "markup case %d", i)
	}
}

// A message with blocks draws the blocks, not the flattened text the server sent
// alongside them: drawing both would say everything twice.
func TestRenderRichBlocks_BlocksReplaceTheFlattenedText(t *testing.T) {
	ml := richList(40, 20)
	msg := domain.Message{
		ID: 1, ChatID: 1, Date: time.Unix(0, 0),
		Text:       "flattened rendering of the document",
		RichBlocks: []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "the actual block"}}},
	}
	ml.SetMessages([]domain.Message{msg})

	out := strings.Join(ml.renderMessage(msg, false), "\n")
	assert.Contains(t, out, "the actual block")
	assert.NotContains(t, out, "flattened rendering")
}

// With the flag off, the message falls back to its flattened text: that text is
// what the server sends for exactly this case, and it is why switching the
// renderer off never needs a re-fetch.
func TestRenderRichBlocks_DisabledFallsBackToText(t *testing.T) {
	ml := NewMessageList(20, 40)
	ml.SetRichMessages(false)
	msg := domain.Message{
		ID: 1, ChatID: 1, Date: time.Unix(0, 0),
		Text:       "flattened rendering",
		RichBlocks: []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "the actual block"}}},
	}
	ml.SetMessages([]domain.Message{msg})

	out := strings.Join(ml.renderMessage(msg, false), "\n")
	assert.Contains(t, out, "flattened rendering")
	assert.NotContains(t, out, "the actual block")
}

// An anchor is a link target, not content: it occupies no line at all, so a
// document whose first block is an anchor starts at its real first block.
func TestRenderRichBlocks_AnchorDrawsNothing(t *testing.T) {
	ml := richList(40, 20)
	blocks := []domain.PageBlock{
		{Kind: domain.BlockKindAnchor, Label: "intro"},
		{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "after"}},
	}
	msg := domain.Message{ID: 1, ChatID: 1, Date: time.Unix(0, 0), RichBlocks: blocks}
	ml.SetMessages([]domain.Message{msg})
	assert.Len(t, ml.renderRichBlocks(msg.ID, blocks, 40), 1)
}

// An unsupported block names itself, so a document the client cannot fully draw
// still says what arrived rather than quietly shortening.
func TestRenderRichBlocks_UnsupportedNamesItself(t *testing.T) {
	ml := richList(40, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{{Kind: domain.BlockKindUnsupported, Label: "pageBlockEmbed"}}, 40)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "pageBlockEmbed")
}

// A document that produced no lines at all still draws one row: an empty bubble
// would be indistinguishable from a message that failed to arrive.
func TestRenderRichBlocks_NeverEmpty(t *testing.T) {
	ml := richList(40, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{{Kind: domain.BlockKindAnchor, Label: "x"}}, 40)
	assert.Len(t, lines, 1)
	assert.Equal(t, 40, lineWidth(lines[0]))
}

// Every line of every block is exactly the content width: a short line would let
// the bubble border fall inside the bubble.
func TestRenderRichBlocks_EveryLineFillsTheWidth(t *testing.T) {
	blocks := []domain.PageBlock{
		{Kind: domain.BlockKindHeading, Level: 1, Text: &domain.RichText{Text: "R"}},
		{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "x"}},
		{Kind: domain.BlockKindDivider},
		{Kind: domain.BlockKindTable, Table: &domain.TableBlock{
			Rows: [][]domain.TableCell{{{Text: domain.RichText{Text: "a"}}}},
		}},
		{Kind: domain.BlockKindDetails, Text: &domain.RichText{Text: "d"}, DetailsOpen: true,
			Children: []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "y"}}}},
		{Kind: domain.BlockKindList, Children: []domain.PageBlock{
			{Kind: domain.BlockKindListItem, Text: &domain.RichText{Text: "i"}},
		}},
	}
	const width = 24
	ml := richList(width, 20)
	for i, line := range ml.renderRichBlocks(1, blocks, width) {
		assert.Equal(t, width, lineWidth(line), "line %d: %q", i, line)
	}
}

// A heading's level has to be visible in the attributes, because a terminal has
// no font size: the loudest level is underlined and bold, the quietest neither.
func TestRenderRichBlocks_HeadingLevelsAreDistinguishable(t *testing.T) {
	ml := richList(20, 20)
	one := ml.renderRichBlocks(1, []domain.PageBlock{{Kind: domain.BlockKindHeading, Level: 1, Text: &domain.RichText{Text: "H"}}}, 20)
	six := ml.renderRichBlocks(1, []domain.PageBlock{{Kind: domain.BlockKindHeading, Level: 6, Text: &domain.RichText{Text: "H"}}}, 20)
	require.Len(t, one, 1)
	require.Len(t, six, 1)
	assert.NotEqual(t, one[0], six[0], "level 1 and level 6 must not render identically")
}

// Code is drawn on the code surface, which is what makes a pre block read as a
// block rather than as ordinary prose.
func TestRenderRichBlocks_PreformattedUsesTheCodeSurface(t *testing.T) {
	ml := richList(30, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{
		{Kind: domain.BlockKindPreformatted, Language: "go", Text: &domain.RichText{Text: "x := 1"}},
	}, 30)
	require.GreaterOrEqual(t, len(lines), 2)
	assert.Contains(t, lines[0], "go")
	assert.Contains(t, lines[1], "x := 1")
}

// An ordered list's marker comes from the server when it sent one, because that
// is the only way a reversed or non-decimal list reads correctly.
func TestRenderRichBlocks_OrderedMarkers(t *testing.T) {
	ml := richList(30, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{{
		Kind:   domain.BlockKindOrderedList,
		Marker: "i",
		Start:  3,
		Children: []domain.PageBlock{
			{Kind: domain.BlockKindListItem, Text: &domain.RichText{Text: "third"}},
			{Kind: domain.BlockKindListItem, Text: &domain.RichText{Text: "fourth"}},
		},
	}}, 30)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "iii.")
	assert.Contains(t, lines[1], "iv.")
}

// A list item's checkbox state is part of what the item says, so it is drawn
// rather than inferred from the marker.
func TestRenderRichBlocks_ListCheckboxes(t *testing.T) {
	ml := richList(30, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{{
		Kind: domain.BlockKindList,
		Children: []domain.PageBlock{
			{Kind: domain.BlockKindListItem, Text: &domain.RichText{Text: "todo"}, Checkbox: true},
			{Kind: domain.BlockKindListItem, Text: &domain.RichText{Text: "done"}, Checkbox: true, Checked: true},
		},
	}}, 30)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "[ ]")
	assert.Contains(t, lines[1], "[x]")
}

// A table's header row is bold and separated from the body by a rule, which is
// the whole of what makes a terminal table readable.
func TestRenderRichTable_HeaderIsSetOff(t *testing.T) {
	ml := richList(40, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{{Kind: domain.BlockKindTable, Table: &domain.TableBlock{
		Rows: [][]domain.TableCell{
			{{Text: domain.RichText{Text: "Item"}, IsHeader: true}, {Text: domain.RichText{Text: "Value"}, IsHeader: true}},
			{{Text: domain.RichText{Text: "Sales"}}, {Text: domain.RichText{Text: "142"}}},
		},
	}}}, 40)
	require.Len(t, lines, 3, "header, rule, body")
	assert.Contains(t, lines[0], "Item")
	assert.Contains(t, lines[1], "─")
	assert.Contains(t, lines[2], "Sales")
}

// A cell's own alignment is honoured: the Bot API requires every cell to name
// one, and a right-aligned numeric column is most of what makes a table readable.
// The three alignments of one text in one column must land in three places.
func TestRenderRichTable_AlignsCells(t *testing.T) {
	ml := richList(40, 20)
	cell := func(text, align string) domain.TableCell {
		return domain.TableCell{Text: domain.RichText{Text: text}, Align: align}
	}
	lines := ml.renderRichBlocks(1, []domain.PageBlock{{Kind: domain.BlockKindTable, Table: &domain.TableBlock{
		Rows: [][]domain.TableCell{
			{cell("x", "left")},
			{cell("x", "center")},
			{cell("x", "right")},
		},
	}}}, 40)
	require.Len(t, lines, 3)

	left := strings.Index(stripRichANSI(lines[0]), "x")
	center := strings.Index(stripRichANSI(lines[1]), "x")
	right := strings.Index(stripRichANSI(lines[2]), "x")
	assert.Less(t, left, center, "a centred cell starts right of a left-aligned one")
	assert.Less(t, center, right, "a right-aligned cell starts right of a centred one")
}

// A cell's vertical alignment places its text inside a row taller than it: a
// "bottom" cell sits on the row's last line rather than floating at its top.
func TestRenderRichTable_AlignsCellsVertically(t *testing.T) {
	ml := richList(40, 20)
	// Three lines, so a middle cell has somewhere to land: in a two-line row
	// there is no room to centre anything.
	tall := func() domain.TableCell {
		return domain.TableCell{Text: domain.RichText{Text: "a\nb\nc"}}
	}
	lines := ml.renderRichBlocks(1, []domain.PageBlock{{Kind: domain.BlockKindTable, Table: &domain.TableBlock{
		Rows: [][]domain.TableCell{
			{tall(), {Text: domain.RichText{Text: "top"}, VAlign: "top"}},
			{tall(), {Text: domain.RichText{Text: "mid"}, VAlign: "middle"}},
			{tall(), {Text: domain.RichText{Text: "bot"}, VAlign: "bottom"}},
		},
	}}}, 40)
	require.Len(t, lines, 9)
	text := func(i int) string { return stripRichANSI(lines[i]) }
	assert.Contains(t, text(0), "top", "a top cell sits on the row's first line")
	assert.NotContains(t, text(1), "top")
	// Three rows of three lines each: the bottom row is 6..8.
	assert.Contains(t, text(8), "bot", "a bottom cell sits on the row's last line")
	assert.NotContains(t, text(6), "bot")
	assert.Contains(t, text(4), "mid", "a middle cell is pushed down from the top")
	assert.NotContains(t, text(3), "mid")
}

// A table too wide for the pane shrinks its columns rather than overflowing it.
func TestRenderRichTable_FitsTheWidth(t *testing.T) {
	ml := richList(20, 20)
	table := &domain.TableBlock{
		Rows: [][]domain.TableCell{{
			{Text: domain.RichText{Text: "aaaaaaaaaaaaaaaaaaaaaaaaaa"}, IsHeader: true},
			{Text: domain.RichText{Text: "bbbbbbbbbbbbbbbbbbbbbbbbbb"}},
		}},
	}
	for i, line := range ml.renderRichBlocks(1, []domain.PageBlock{{Kind: domain.BlockKindTable, Table: table}}, 20) {
		assert.LessOrEqual(t, lineWidth(line), 20, "line %d", i)
		assert.Equal(t, 20, lineWidth(line), "line %d must fill the width", i)
	}
}

// A details block's summary is always drawn; its children only when it is open.
func TestRenderRichBlocks_DetailsCollapseAndExpand(t *testing.T) {
	ml := richList(30, 20)
	blocks := []domain.PageBlock{{
		Kind: domain.BlockKindDetails,
		Text: &domain.RichText{Text: "More"},
		Children: []domain.PageBlock{
			{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "the body"}},
		},
	}}
	collapsed := ml.renderRichBlocks(7, blocks, 30)
	require.Len(t, collapsed, 1)
	assert.Contains(t, collapsed[0], "More")
	assert.NotContains(t, strings.Join(collapsed, "\n"), "the body")

	require.True(t, ml.ToggleDetails(7, "0", false))
	expanded := ml.renderRichBlocks(7, blocks, 30)
	require.Len(t, expanded, 2)
	assert.Contains(t, strings.Join(expanded, "\n"), "the body")

	require.True(t, ml.ToggleDetails(7, "0", false))
	assert.Len(t, ml.renderRichBlocks(7, blocks, 30), 1)
}

// The server's default is what a section starts at; a reader's toggle is what
// overrides it, and it is per message so two messages do not share a state.
func TestRichDetailsOpen_ServerDefaultAndPerMessageState(t *testing.T) {
	ml := richList(30, 20)
	assert.True(t, ml.richDetailsOpen(1, "0", true), "an untouched section follows the server")
	assert.False(t, ml.richDetailsOpen(1, "0", false))

	require.True(t, ml.ToggleDetails(1, "0", false))
	assert.True(t, ml.richDetailsOpen(1, "0", false))
	// A different message's identically-shaped section is untouched.
	assert.False(t, ml.richDetailsOpen(2, "0", false))
}

// The toggle has to find the section the reader is looking at, which is the
// outermost one: a nested section inside a collapsed parent is not on screen.
func TestSelectedMessageDetailsPath_SkipsHiddenSections(t *testing.T) {
	ml := richList(30, 40)
	msg := domain.Message{
		ID: 9, ChatID: 1, Date: time.Unix(0, 0),
		RichBlocks: []domain.PageBlock{{
			Kind: domain.BlockKindDetails,
			Text: &domain.RichText{Text: "outer"},
			Children: []domain.PageBlock{{
				Kind: domain.BlockKindDetails,
				Text: &domain.RichText{Text: "inner"},
			}},
		}},
	}
	ml.SetMessages([]domain.Message{msg})

	id, path, defaultOpen, ok := ml.SelectedMessageDetailsPath()
	require.True(t, ok)
	assert.Equal(t, 9, id)
	assert.Equal(t, "0", path, "the collapsed outer section is the reachable one")
	assert.False(t, defaultOpen)

	// Once the outer one is open, the inner becomes the target.
	require.True(t, ml.ToggleDetails(9, "0", false))
	_, path, _, ok = ml.SelectedMessageDetailsPath()
	require.True(t, ok)
	assert.Equal(t, "0/0", path)
}

func TestSelectedMessageDetailsPath_NoDetails(t *testing.T) {
	ml := richList(30, 40)
	msg := domain.Message{ID: 9, ChatID: 1, Date: time.Unix(0, 0),
		RichBlocks: []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "plain"}}}}
	ml.SetMessages([]domain.Message{msg})
	_, _, _, ok := ml.SelectedMessageDetailsPath()
	assert.False(t, ok)
}

// Math has no engine here, so the expression is drawn verbatim behind a marker
// that says it is maths rather than prose.
func TestRenderRichBlocks_MathIsMarkedAndVerbatim(t *testing.T) {
	ml := richList(40, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{
		{Kind: domain.BlockKindMath, Text: &domain.RichText{Text: `\frac{a}{b}`}},
	}, 40)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "∑")
	assert.Contains(t, lines[0], `\frac{a}{b}`)
}

// A map has no tiles in a terminal, so its coordinates and zoom are what is
// drawn: the honest rendering of what the block points at.
func TestRenderRichBlocks_MapShowsCoordinates(t *testing.T) {
	ml := richList(60, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{{
		Kind:    domain.BlockKindMap,
		Map:     &domain.MapBlock{Lat: 52.22977, Long: 21.01178, Zoom: 12},
		Caption: &domain.RichText{Text: "the office"},
	}}, 60)
	out := strings.Join(lines, "\n")
	assert.Contains(t, out, "52.22977")
	assert.Contains(t, out, "21.01178")
	assert.Contains(t, out, "the office")
}

// A collage is drawn as one grid, sharing the album mosaic's tile geometry.
func TestRenderRichBlocks_CollageGrids(t *testing.T) {
	ml := richList(60, 40)
	blocks := []domain.PageBlock{{
		Kind: domain.BlockKindCollage,
		Children: []domain.PageBlock{
			{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 1, ThumbSize: "m"}},
			{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 2, ThumbSize: "m"}},
		},
		Caption: &domain.RichText{Text: "two photos"},
	}}
	lines := ml.renderRichBlocks(1, blocks, 60)
	require.Greater(t, len(lines), 3, "a grid plus a caption")
	assert.Contains(t, strings.Join(lines, "\n"), "two photos")
	for i, line := range lines {
		assert.Equal(t, 60, lineWidth(line), "line %d", i)
	}
}

// A slideshow is drawn as a grid too: there is no swipe in a terminal, and the
// deck's parts are all worth showing.
func TestRenderRichBlocks_SlideshowDrawsAsGrid(t *testing.T) {
	ml := richList(60, 40)
	blocks := []domain.PageBlock{{
		Kind: domain.BlockKindSlideshow,
		Children: []domain.PageBlock{
			{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 1, ThumbSize: "m"}},
			{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 2, ThumbSize: "m"}},
		},
	}}
	collage := ml.renderRichBlocks(1, []domain.PageBlock{{
		Kind:     domain.BlockKindCollage,
		Children: blocks[0].Children,
	}}, 60)
	assert.Equal(t, collapseIDs(collage), collapseIDs(ml.renderRichBlocks(1, blocks, 60)),
		"a slideshow and a collage of the same parts draw the same grid")
}

// collapseIDs strips escapes and trailing padding so two renders can be compared
// for shape without depending on colour or on their fill.
func collapseIDs(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimRight(stripRichANSI(l), " ")
	}
	return out
}

// stripRichANSI removes SGR escapes so a test can read a line's text.
func stripRichANSI(s string) string {
	return richSGR.ReplaceAllString(s, "")
}

var richSGR = regexp.MustCompile("\x1b\\[[0-9;]*[mKJH]")

// A media block whose file the message did not carry still draws its caption:
// the text is what the sender wrote, and a missing download is not a missing
// sentence.
func TestRenderRichBlocks_MediaWithoutFileKeepsCaption(t *testing.T) {
	ml := richList(40, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{{
		Kind:    domain.BlockKindPhoto,
		Caption: &domain.RichText{Text: "caption only"},
	}}, 40)
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], "caption only")
}

// A photo (or video, audio, gallery, map) reads as a distinct object; content
// that follows it must be set off by a blank row rather than touching it, the
// way a plain message's media is set off from its own caption (reported live:
// "brakuje mi jeszcze odstępu między zdjęciem a tekstem").
func TestRenderRichBlocks_MediaIsSeparatedFromWhatFollows(t *testing.T) {
	ml := richList(40, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{
		{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 1}},
		{Kind: domain.BlockKindHeading, Level: 3, Text: &domain.RichText{Text: "Heading"}},
	}, 40)
	require.Len(t, lines, 3, "photo row, blank separator, heading row")
	assert.Empty(t, strings.TrimSpace(stripRichANSI(lines[1])), "the separator row must be blank")
	assert.Contains(t, lines[2], "Heading")
}

// The separator is between blocks, not after the last one: a trailing blank
// row would be an empty line at the bottom of every media-ending message.
func TestRenderRichBlocks_NoTrailingSeparatorAfterLastMedia(t *testing.T) {
	ml := richList(40, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{
		{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 1}},
	}, 40)
	require.Len(t, lines, 1)
}

// Two ordinary text blocks in a row need no separator: the gap is specific to
// a media block, not general spacing between every pair of blocks.
func TestRenderRichBlocks_NoSeparatorBetweenTextBlocks(t *testing.T) {
	ml := richList(40, 20)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{
		{Kind: domain.BlockKindHeading, Level: 3, Text: &domain.RichText{Text: "One"}},
		{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "Two"}},
	}, 40)
	require.Len(t, lines, 2)
}

// A collage child with no preview is named in place rather than dropped, which
// is what keeps the block tree described honestly.
func TestRenderRichBlocks_CollageWithoutPreviewNamesTheFile(t *testing.T) {
	ml := richList(50, 30)
	lines := ml.renderRichBlocks(1, []domain.PageBlock{{
		Kind: domain.BlockKindCollage,
		Children: []domain.PageBlock{{
			Kind:     domain.BlockKindAudio,
			Media:    &domain.MediaRef{Kind: domain.MediaAudio, Title: "Track"},
			Document: &domain.DocumentRef{ID: 3, MimeType: "audio/mpeg"},
		}},
	}}, 50)
	assert.Contains(t, strings.Join(lines, "\n"), "Track")
}
