package tg

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// text is a shorthand for a plain rich-text leaf, which is what most blocks
// carry.
func text(s string) *tg.TextPlain { return &tg.TextPlain{Text: s} }

func TestConvertRichBlocks_TextualKinds(t *testing.T) {
	cases := []struct {
		name  string
		in    tg.PageBlockClass
		kind  domain.BlockKind
		text  string
		level int
	}{
		{"paragraph", &tg.PageBlockParagraph{Text: text("hello")}, domain.BlockKindParagraph, "hello", 0},
		{"h1", &tg.PageBlockHeading1{Text: text("one")}, domain.BlockKindHeading, "one", 1},
		{"h2", &tg.PageBlockHeading2{Text: text("two")}, domain.BlockKindHeading, "two", 2},
		{"h3", &tg.PageBlockHeading3{Text: text("three")}, domain.BlockKindHeading, "three", 3},
		{"h4", &tg.PageBlockHeading4{Text: text("four")}, domain.BlockKindHeading, "four", 4},
		{"h5", &tg.PageBlockHeading5{Text: text("five")}, domain.BlockKindHeading, "five", 5},
		{"h6", &tg.PageBlockHeading6{Text: text("six")}, domain.BlockKindHeading, "six", 6},
		{"header", &tg.PageBlockHeader{Text: text("masthead")}, domain.BlockKindHeading, "masthead", 2},
		{"subheader", &tg.PageBlockSubheader{Text: text("sub")}, domain.BlockKindHeading, "sub", 3},
		{"footer", &tg.PageBlockFooter{Text: text("fin")}, domain.BlockKindFooter, "fin", 0},
		{"kicker", &tg.PageBlockKicker{Text: text("KICK")}, domain.BlockKindKicker, "KICK", 0},
		{"title", &tg.PageBlockTitle{Text: text("t")}, domain.BlockKindTitle, "t", 0},
		{"subtitle", &tg.PageBlockSubtitle{Text: text("s")}, domain.BlockKindSubtitle, "s", 0},
		{"divider", &tg.PageBlockDivider{}, domain.BlockKindDivider, "", 0},
		{"thinking", &tg.PageBlockThinking{Text: text("hmm")}, domain.BlockKindThinking, "hmm", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := convertRichBlocks([]tg.PageBlockClass{tc.in}, richFileRefs{})
			require.Len(t, got, 1)
			assert.Equal(t, tc.kind, got[0].Kind)
			assert.Equal(t, tc.level, got[0].Level)
			if tc.text == "" {
				assert.Nil(t, got[0].Text)
				return
			}
			require.NotNil(t, got[0].Text)
			assert.Equal(t, tc.text, got[0].Text.Text)
		})
	}
}

// A pre block keeps its language, because it is the only thing that says what
// the code is.
func TestConvertRichBlocks_PreformattedKeepsLanguage(t *testing.T) {
	got := convertRichBlocks([]tg.PageBlockClass{
		&tg.PageBlockPreformatted{Text: text("x := 1"), Language: "go"},
	}, richFileRefs{})
	require.Len(t, got, 1)
	assert.Equal(t, domain.BlockKindPreformatted, got[0].Kind)
	assert.Equal(t, "go", got[0].Language)
	assert.Equal(t, "x := 1", got[0].Text.Text)
}

// An anchor is a target, not content: it survives as a block so the tree keeps
// the name, and the renderer is what decides it draws nothing.
func TestConvertRichBlocks_AnchorKeepsName(t *testing.T) {
	got := convertRichBlocks([]tg.PageBlockClass{&tg.PageBlockAnchor{Name: "intro"}}, richFileRefs{})
	require.Len(t, got, 1)
	assert.Equal(t, domain.BlockKindAnchor, got[0].Kind)
	assert.Equal(t, "intro", got[0].Label)
}

// A constructor this client cannot draw must still arrive as a block: the
// placeholder names what came, and a dropped block would shorten the message.
func TestConvertRichBlocks_UnknownConstructorBecomesUnsupported(t *testing.T) {
	got := convertRichBlocks([]tg.PageBlockClass{
		&tg.PageBlockEmbed{URL: "https://example.com"},
		&tg.PageBlockUnsupported{},
	}, richFileRefs{})
	require.Len(t, got, 2)
	assert.Equal(t, domain.BlockKindUnsupported, got[0].Kind)
	assert.Equal(t, "pageBlockEmbed", got[0].Label)
	assert.Equal(t, "pageBlockUnsupported", got[1].Label)
}

// Nesting is preserved for the kinds that hold a tree, because the renderer
// indents by depth rather than flattening.
func TestConvertRichBlocks_NestedContainers(t *testing.T) {
	got := convertRichBlocks([]tg.PageBlockClass{
		&tg.PageBlockDetails{
			Open:  true,
			Title: text("More"),
			Blocks: []tg.PageBlockClass{
				&tg.PageBlockParagraph{Text: text("body")},
			},
		},
		&tg.PageBlockBlockquoteBlocks{
			Caption: text("said someone"),
			Blocks: []tg.PageBlockClass{
				&tg.PageBlockParagraph{Text: text("quoted")},
			},
		},
		&tg.PageBlockCollage{
			Items: []tg.PageBlockClass{
				&tg.PageBlockPhoto{PhotoID: 7},
			},
			Caption: tg.PageCaption{Text: text("cap")},
		},
	}, richFileRefs{})

	require.Len(t, got, 3)
	assert.Equal(t, domain.BlockKindDetails, got[0].Kind)
	assert.True(t, got[0].DetailsOpen)
	assert.Equal(t, "More", got[0].Text.Text)
	require.Len(t, got[0].Children, 1)
	assert.Equal(t, "body", got[0].Children[0].Text.Text)

	assert.Equal(t, domain.BlockKindBlockquote, got[1].Kind)
	require.Len(t, got[1].Children, 1)
	assert.Equal(t, "quoted", got[1].Children[0].Text.Text)
	assert.Equal(t, "said someone", got[1].Credit.Text)

	assert.Equal(t, domain.BlockKindCollage, got[2].Kind)
	require.Len(t, got[2].Children, 1)
	assert.Equal(t, "cap", got[2].Caption.Text)
}

// A cover is a wrapper rather than a kind of its own: what it wraps is what the
// reader sees.
func TestConvertRichBlocks_CoverUnwraps(t *testing.T) {
	got := convertRichBlocks([]tg.PageBlockClass{
		&tg.PageBlockCover{Cover: &tg.PageBlockPhoto{PhotoID: 3}},
	}, richFileRefs{})
	require.Len(t, got, 1)
	assert.Equal(t, domain.BlockKindPhoto, got[0].Kind)
}

// A media block's file lives in the message's Photos/Documents list, not in the
// block, so an unjoined block would render as a caption with no picture.
func TestConvertRichBlocks_MediaJoinsDeclaredFiles(t *testing.T) {
	photo := &tg.Photo{
		ID: 42, AccessHash: 7, FileReference: []byte{1}, DCID: 2,
		Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "m", W: 320, H: 240}},
	}
	doc := &tg.Document{
		ID: 99, AccessHash: 8, FileReference: []byte{2}, DCID: 3,
		MimeType: "video/mp4", Size: 100,
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeVideo{Duration: 12, W: 640, H: 480},
		},
	}
	refs := newRichFileRefs([]tg.PhotoClass{photo}, []tg.DocumentClass{doc})

	got := convertRichBlocks([]tg.PageBlockClass{
		&tg.PageBlockPhoto{PhotoID: 42, Caption: tg.PageCaption{Text: text("a photo")}},
		&tg.PageBlockVideo{VideoID: 99, Caption: tg.PageCaption{Text: text("a video")}},
	}, refs)

	require.Len(t, got, 2)
	require.NotNil(t, got[0].Photo)
	assert.Equal(t, int64(42), got[0].Photo.ID)
	assert.Equal(t, "m", got[0].Photo.ThumbSize)
	assert.Equal(t, 320, got[0].Photo.Width, "the rich photo's chosen-thumb dims must reach the block's PhotoRef")
	assert.Equal(t, 240, got[0].Photo.Height)
	assert.Equal(t, domain.MediaPhoto, got[0].Media.Kind)

	require.NotNil(t, got[1].Document)
	assert.Equal(t, int64(99), got[1].Document.ID)
	assert.Equal(t, domain.MediaVideo, got[1].Media.Kind)
	assert.Equal(t, 12, got[1].Media.Duration)
	assert.Equal(t, 640, got[1].Media.Width, "the rich video's reported dims must reach MediaRef")
	assert.Equal(t, 480, got[1].Media.Height)
}

// A block naming a file the message did not declare keeps its caption: the text
// is still what the sender wrote, and a missing download is not a missing
// sentence.
func TestConvertRichBlocks_MissingFileKeepsCaption(t *testing.T) {
	got := convertRichBlocks([]tg.PageBlockClass{
		&tg.PageBlockPhoto{PhotoID: 404, Caption: tg.PageCaption{Text: text("gone")}},
	}, richFileRefs{})
	require.Len(t, got, 1)
	assert.Nil(t, got[0].Media)
	assert.Nil(t, got[0].Photo)
	assert.Equal(t, "gone", got[0].Caption.Text)
}

// A map block is drawn from its coordinates, and a geo point that is not a
// concrete location leaves no coordinates rather than a zero-valued pin in the
// Gulf of Guinea.
func TestConvertRichBlocks_Map(t *testing.T) {
	got := convertRichBlocks([]tg.PageBlockClass{
		&tg.PageBlockMap{Geo: &tg.GeoPoint{Lat: 52.23, Long: 21.01}, Zoom: 15},
		&tg.PageBlockMap{Geo: &tg.GeoPointEmpty{}},
	}, richFileRefs{})
	require.Len(t, got, 2)
	require.NotNil(t, got[0].Map)
	assert.Equal(t, 52.23, got[0].Map.Lat)
	assert.Equal(t, 21.01, got[0].Map.Long)
	assert.Equal(t, 15, got[0].Map.Zoom)
	assert.Nil(t, got[1].Map)
}

// Math arrives as source and is kept verbatim: a terminal has no LaTeX engine,
// and inventing a rendering would be worse than showing the expression.
func TestConvertRichBlocks_MathKeepsSource(t *testing.T) {
	got := convertRichBlocks([]tg.PageBlockClass{&tg.PageBlockMath{Source: `\frac{a}{b}`}}, richFileRefs{})
	require.Len(t, got, 1)
	assert.Equal(t, domain.BlockKindMath, got[0].Kind)
	assert.Equal(t, `\frac{a}{b}`, got[0].Text.Text)
}

func TestConvertRichBlocks_Table(t *testing.T) {
	got := convertRichBlocks([]tg.PageBlockClass{
		&tg.PageBlockTable{
			Bordered: true,
			Rows: []tg.PageTableRow{
				{Cells: []tg.PageTableCell{
					{Header: true, Text: text("Item")},
					{Header: true, AlignRight: true, Text: text("Value")},
				}},
				{Cells: []tg.PageTableCell{
					{Text: text("Sales"), Colspan: 2},
					{ValignBottom: true, AlignCenter: true, Text: text("142")},
				}},
			},
		},
	}, richFileRefs{})

	require.Len(t, got, 1)
	tb := got[0].Table
	require.NotNil(t, tb)
	assert.True(t, tb.Bordered)
	require.Len(t, tb.Rows, 2)
	assert.True(t, tb.Rows[0][0].IsHeader)
	assert.Equal(t, "left", tb.Rows[0][0].Align)
	assert.Equal(t, "right", tb.Rows[0][1].Align)
	assert.Equal(t, 2, tb.Rows[1][0].Colspan)
	assert.Equal(t, "center", tb.Rows[1][1].Align)
	assert.Equal(t, "bottom", tb.Rows[1][1].VAlign)
}

func TestConvertRichBlocks_ListCarriesMarkers(t *testing.T) {
	got := convertRichBlocks([]tg.PageBlockClass{
		&tg.PageBlockList{Items: []tg.PageListItemClass{
			&tg.PageListItemText{Checkbox: true, Checked: true, Text: text("done")},
			&tg.PageListItemBlocks{Blocks: []tg.PageBlockClass{
				&tg.PageBlockParagraph{Text: text("nested")},
			}},
		}},
		&tg.PageBlockOrderedList{
			Start: 3,
			Type:  "a",
			Items: []tg.PageListOrderedItemClass{
				&tg.PageListOrderedItemText{Num: "c.", Text: text("third")},
			},
		},
	}, richFileRefs{})

	require.Len(t, got, 2)
	assert.Equal(t, domain.BlockKindList, got[0].Kind)
	require.Len(t, got[0].Children, 2)
	assert.True(t, got[0].Children[0].Checkbox)
	assert.True(t, got[0].Children[0].Checked)
	assert.Equal(t, "done", got[0].Children[0].Text.Text)
	assert.Equal(t, "nested", got[0].Children[1].Children[0].Text.Text)

	assert.Equal(t, domain.BlockKindOrderedList, got[1].Kind)
	assert.Equal(t, 3, got[1].Start)
	assert.Equal(t, "a", got[1].Marker)
	require.Len(t, got[1].Children, 1)
	assert.Equal(t, "c.", got[1].Children[0].Label)
}

// Rich text is flattened into one string plus UTF-16 entities, which is the
// shape a plain message's body already has — so the same renderer paints both.
func TestConvertRichText_FlattensStyles(t *testing.T) {
	raw := &tg.TextConcat{Texts: []tg.RichTextClass{
		&tg.TextPlain{Text: "a "},
		&tg.TextBold{Text: text("bold")},
		&tg.TextPlain{Text: " z"},
	}}
	got := convertRichText(raw)
	require.NotNil(t, got)
	assert.Equal(t, "a bold z", got.Text)
	require.Len(t, got.Entities, 1)
	assert.Equal(t, "bold", got.Entities[0].Type)
	assert.Equal(t, 2, got.Entities[0].Offset)
	assert.Equal(t, 4, got.Entities[0].Length)
}

// Offsets are counted in UTF-16 units, not bytes: a run after an emoji would be
// styled in the wrong place otherwise, which is the classic Telegram-entity bug.
func TestConvertRichText_OffsetsAreUTF16(t *testing.T) {
	raw := &tg.TextConcat{Texts: []tg.RichTextClass{
		&tg.TextPlain{Text: "😀"},
		&tg.TextItalic{Text: text("it")},
	}}
	got := convertRichText(raw)
	require.NotNil(t, got)
	require.Len(t, got.Entities, 1)
	assert.Equal(t, 2, got.Entities[0].Offset, "an emoji is two UTF-16 units")
	assert.Equal(t, 2, got.Entities[0].Length)
}

func TestConvertRichText_LinkAndMentionTargets(t *testing.T) {
	got := convertRichText(&tg.TextConcat{Texts: []tg.RichTextClass{
		&tg.TextURL{Text: text("site"), URL: "https://example.com"},
		&tg.TextEmail{Text: text("me"), Email: "a@b.c"},
		&tg.TextMentionName{Text: text("Ada"), UserID: 77},
	}})
	require.NotNil(t, got)
	require.Len(t, got.Entities, 3)
	assert.Equal(t, "text_url", got.Entities[0].Type)
	assert.Equal(t, "https://example.com", got.Entities[0].URL)
	assert.Equal(t, "email", got.Entities[1].Type)
	assert.Equal(t, "a@b.c", got.Entities[1].URL)
	assert.Equal(t, "mention_name", got.Entities[2].Type)
	assert.Equal(t, int64(77), got.Entities[2].UserID)
}

// An empty tree is no text, not empty text: a renderer that receives an empty
// RichText would draw a line for it.
func TestConvertRichText_EmptyIsNil(t *testing.T) {
	assert.Nil(t, convertRichText(nil))
	assert.Nil(t, convertRichText(&tg.TextEmpty{}))
	assert.Nil(t, convertRichText(&tg.TextPlain{Text: ""}))
}

// A text-less custom emoji would otherwise drop the glyph the sender chose.
func TestConvertRichText_CustomEmojiKeepsAlt(t *testing.T) {
	got := convertRichText(&tg.TextCustomEmoji{DocumentID: 5, Alt: "⭐"})
	require.NotNil(t, got)
	assert.Equal(t, "⭐", got.Text)
}

func TestConvertReplyMarkup_InlineOnly(t *testing.T) {
	got := convertReplyMarkup(&tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{
		{Buttons: []tg.KeyboardButtonClass{
			styledButton(&tg.KeyboardButtonCallback{Text: "Yes", Data: []byte("yes")}, tg.KeyboardButtonStyle{BgPrimary: true}),
			&tg.KeyboardButtonURL{Text: "Docs", URL: "https://example.com"},
		}},
	}})
	require.NotNil(t, got)
	require.Len(t, got.Rows, 1)
	require.Len(t, got.Rows[0], 2)

	assert.Equal(t, "Yes", got.Rows[0][0].Text)
	assert.Equal(t, domain.ButtonStylePrimary, got.Rows[0][0].Style)
	assert.Equal(t, domain.ButtonActionCallback, got.Rows[0][0].Action.Kind)
	assert.Equal(t, []byte("yes"), got.Rows[0][0].Action.Data)

	assert.Equal(t, domain.ButtonStyleDefault, got.Rows[0][1].Style)
	assert.Equal(t, domain.ButtonActionURL, got.Rows[0][1].Action.Kind)
	assert.Equal(t, "https://example.com", got.Rows[0][1].Action.URL)
}

// The other ReplyMarkup variants are a phone keyboard, not a message's buttons.
// Reporting an empty markup would claim the message has a keyboard with no keys.
func TestConvertReplyMarkup_OtherVariantsAreNil(t *testing.T) {
	assert.Nil(t, convertReplyMarkup(&tg.ReplyKeyboardMarkup{}))
	assert.Nil(t, convertReplyMarkup(&tg.ReplyKeyboardForceReply{}))
	assert.Nil(t, convertReplyMarkup(&tg.ReplyKeyboardHide{}))
	assert.Nil(t, convertReplyMarkup(nil))
	assert.Nil(t, convertReplyMarkup(&tg.ReplyInlineMarkup{}))
}

// A button whose action this client cannot carry out keeps its text and says
// why: dropping it would leave the row with a hole in it.
func TestConvertReplyMarkup_UnsupportedActionIsNamed(t *testing.T) {
	got := convertReplyMarkup(&tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{
		{Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonRequestPhone{Text: "Send phone"},
			&tg.KeyboardButtonGame{Text: "Play"},
		}},
	}})
	require.NotNil(t, got)
	require.Len(t, got.Rows[0], 2)
	want := []struct{ text, reason string }{
		{"Send phone", "asks for your phone number"},
		{"Play", "opens a game"},
	}
	for i, w := range want {
		assert.Equal(t, w.text, got.Rows[0][i].Text)
		assert.Equal(t, domain.ButtonActionNone, got.Rows[0][i].Action.Kind)
		assert.Equal(t, w.reason, got.Rows[0][i].Action.Reason)
	}
}

// Telegram's emphasis flags are mutually exclusive in practice; the priority
// order is the one it documents, so a server that sets two still lands on one.
func TestConvertReplyMarkup_StylePriority(t *testing.T) {
	got := convertReplyMarkup(&tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{
		{Buttons: []tg.KeyboardButtonClass{
			styledButton(&tg.KeyboardButtonCallback{Text: "a"}, tg.KeyboardButtonStyle{BgSuccess: true}),
			styledButton(&tg.KeyboardButtonCallback{Text: "b"}, tg.KeyboardButtonStyle{BgDanger: true}),
			styledButton(&tg.KeyboardButtonCallback{Text: "c"}, tg.KeyboardButtonStyle{BgPrimary: true, BgDanger: true}),
		}},
	}})
	require.NotNil(t, got)
	assert.Equal(t, domain.ButtonStyleSuccess, got.Rows[0][0].Style)
	assert.Equal(t, domain.ButtonStyleDanger, got.Rows[0][1].Style)
	assert.Equal(t, domain.ButtonStylePrimary, got.Rows[0][2].Style)
}

// styledButton builds a callback button with an emphasis, the way the decoder
// produces one: the style is a conditional field, so setting the struct field
// alone would leave it unread.
func styledButton(b *tg.KeyboardButtonCallback, style tg.KeyboardButtonStyle) *tg.KeyboardButtonCallback {
	b.SetStyle(style)
	return b
}

// A row whose buttons all vanished would render as an empty row, so it is
// dropped rather than kept as a gap.
func TestConvertReplyMarkup_EmptyRowsDropped(t *testing.T) {
	got := convertReplyMarkup(&tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{
		{},
		{Buttons: []tg.KeyboardButtonClass{&tg.KeyboardButtonURL{Text: "x", URL: "u"}}},
	}})
	require.NotNil(t, got)
	assert.Len(t, got.Rows, 1)
}

// The end-to-end path: a raw Message carrying a rich document becomes a domain
// message whose blocks and keyboard are populated.
func TestConvertMessage_CarriesRichContent(t *testing.T) {
	raw := &tg.Message{
		ID: 12, Date: 1700000000, Message: "fallback text",
		ReplyMarkup: &tg.ReplyInlineMarkup{Rows: []tg.KeyboardButtonRow{
			{Buttons: []tg.KeyboardButtonClass{
				&tg.KeyboardButtonCallback{Text: "Go", Data: []byte{1, 2}},
			}},
		}},
	}
	raw.SetRichMessage(tg.RichMessage{
		Blocks: []tg.PageBlockClass{
			&tg.PageBlockHeading2{Text: text("Report")},
			&tg.PageBlockParagraph{Text: text("body")},
		},
	})

	got, ok := convertMessage(raw, 5)
	require.True(t, ok)
	require.Len(t, got.RichBlocks, 2)
	assert.Equal(t, domain.BlockKindHeading, got.RichBlocks[0].Kind)
	assert.Equal(t, 2, got.RichBlocks[0].Level)
	require.NotNil(t, got.ReplyMarkup)
	assert.Equal(t, 1, got.ReplyMarkup.Count())

	// The flattened text is kept alongside the blocks: it is what a client that
	// draws no blocks shows.
	assert.Equal(t, "fallback text", got.Text)
}

// A message with no rich document carries neither field, so every existing
// rendering path sees exactly what it saw before this feature existed.
func TestConvertMessage_PlainMessageHasNoRichContent(t *testing.T) {
	got, ok := convertMessage(&tg.Message{ID: 1, Date: 1, Message: "hi"}, 5)
	require.True(t, ok)
	assert.Nil(t, got.RichBlocks)
	assert.Nil(t, got.ReplyMarkup)
}

// An empty rich document is not rich content: an empty block list would send
// the renderer down the rich path to draw nothing at all.
func TestConvertMessage_EmptyRichDocumentIsNil(t *testing.T) {
	raw := &tg.Message{ID: 1, Date: 1, Message: "hi"}
	raw.SetRichMessage(tg.RichMessage{})
	got, ok := convertMessage(raw, 5)
	require.True(t, ok)
	assert.Nil(t, got.RichBlocks)
}

// A streaming rich message does not arrive as a message: Telegram carries it as
// a typing action with the partial document inside. That is the whole of how a
// draft reaches a client, and the conversion is what makes it drawable.
func TestConvertEphemeralDraft_CarriesThePartialDocument(t *testing.T) {
	photo := &tg.Photo{
		ID: 42, AccessHash: 7, FileReference: []byte{1}, DCID: 2,
		Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "m", W: 320, H: 240}},
	}
	action := &tg.SendMessageRichMessageDraftAction{
		RandomID: 77,
		RichMessage: tg.RichMessage{
			Blocks: []tg.PageBlockClass{
				&tg.PageBlockHeading2{Text: text("Draft")},
				&tg.PageBlockPhoto{PhotoID: 42, Caption: tg.PageCaption{Text: text("preview")}},
			},
			Photos: []tg.PhotoClass{photo},
		},
	}

	got, ok := convertEphemeralDraft(9, action)
	require.True(t, ok)
	assert.Equal(t, int64(9), got.ChatID)
	assert.Equal(t, 77, got.ID)
	assert.False(t, got.Placeholder)
	require.Len(t, got.TextBlocks, 2)
	assert.Equal(t, domain.BlockKindHeading, got.TextBlocks[0].Kind)
	require.NotNil(t, got.TextBlocks[1].Photo, "the draft's file list is joined like a message's")
	assert.Equal(t, "preview", got.TextBlocks[1].Caption.Text)
}

// The thinking stage is a document whose only block says the bot is working: it
// is marked so the overlay draws the placeholder rather than an empty document.
func TestConvertEphemeralDraft_MarksTheThinkingStage(t *testing.T) {
	got, ok := convertEphemeralDraft(9, &tg.SendMessageRichMessageDraftAction{
		RandomID: 1,
		RichMessage: tg.RichMessage{Blocks: []tg.PageBlockClass{
			&tg.PageBlockThinking{Text: text("considering")},
		}},
	})
	require.True(t, ok)
	assert.True(t, got.Placeholder)
	assert.Equal(t, "considering", got.Text)
}

// A plain-text stream is not a draft: it already reads as typing, which this
// client shows, and an overlay as well would say the same thing twice.
func TestConvertEphemeralDraft_PlainTextIsNotADraft(t *testing.T) {
	_, ok := convertEphemeralDraft(9, &tg.SendMessageTextDraftAction{Text: tg.TextWithEntities{Text: "typing…"}})
	assert.False(t, ok)
	_, ok = convertEphemeralDraft(9, &tg.SendMessageTypingAction{})
	assert.False(t, ok)
}

// The draft's id is what makes a revision replace the right stream: two streams
// in the same chat are two drafts, and the id is the only thing telling them
// apart.
func TestConvertEphemeralDraft_KeepsTheStreamID(t *testing.T) {
	first, ok := convertEphemeralDraft(1, &tg.SendMessageRichMessageDraftAction{RandomID: 5,
		RichMessage: tg.RichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: text("a")}}}})
	require.True(t, ok)
	second, ok := convertEphemeralDraft(1, &tg.SendMessageRichMessageDraftAction{RandomID: 6,
		RichMessage: tg.RichMessage{Blocks: []tg.PageBlockClass{&tg.PageBlockParagraph{Text: text("b")}}}})
	require.True(t, ok)
	assert.NotEqual(t, first.ID, second.ID)
}
