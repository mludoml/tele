package components

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plainRows is a rendered block with its escape sequences removed, one entry per
// row, so a test can ask what the screen says rather than what the cache holds.
func plainRows(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = xansi.Strip(l)
	}
	return out
}

func joinedPlain(lines []string) string { return strings.Join(plainRows(lines), "\n") }

func translatedMessage(id int, chatID int64, text string) domain.Message {
	return domain.Message{ID: id, ChatID: chatID, Date: time.Unix(100, 0), Text: text}
}

func translationOf(id int, text string) domain.MessageTranslation {
	return domain.MessageTranslation{MessageID: id, Text: text}
}

// The translation replaces the body: it is not appended to the original, and the
// original is not drawn anywhere in the bubble.
func TestTranslation_ReplacesTheOriginalText(t *testing.T) {
	ml := NewMessageList(20, 60)
	msg := translatedMessage(1, 7, "original wording")
	ml.SetMessages([]domain.Message{msg})
	ml.SetTranslation(7, msg, "pl", "Polish", translationOf(1, "przetłumaczony tekst"))

	got := joinedPlain(ml.renderMessage(msg, false))

	assert.Contains(t, got, "przetłumaczony tekst")
	assert.NotContains(t, got, "original wording", "the original must be replaced, not concatenated with the translation")
}

func TestTranslation_MarkerNamesTheLanguage(t *testing.T) {
	ml := NewMessageList(20, 60)
	msg := translatedMessage(1, 7, "hi")
	ml.SetMessages([]domain.Message{msg})
	ml.SetTranslation(7, msg, "pl", "Polish", translationOf(1, "cześć"))

	got := plainRows(ml.renderMessage(msg, false))

	markers := 0
	for _, line := range got {
		if strings.Contains(line, "Translated to Polish") {
			markers++
			// The marker is its own row: it reads exactly as the phrase, with
			// only the bubble's padding around it.
			assert.Contains(t, strings.TrimSpace(line), "Translated to Polish")
		}
	}
	assert.Equal(t, 1, markers, "exactly one marker row:\n%s", strings.Join(got, "\n"))
}

// Without a translation there is no marker at all: the bubble looks exactly as
// it did before the feature existed.
func TestTranslation_NoMarkerWhenUntranslated(t *testing.T) {
	ml := NewMessageList(20, 60)
	msg := translatedMessage(1, 7, "hi")
	ml.SetMessages([]domain.Message{msg})

	assert.NotContains(t, joinedPlain(ml.renderMessage(msg, false)), "Translated to")
}

// The entities that came back with the translation are the ones rendered: a bold
// span in the translated text is drawn bold, and the same text without that
// entity is not.
func TestTranslation_EntitiesAreRendered(t *testing.T) {
	const (
		original   = "plain original"
		translated = "bold word here"
	)
	boldStart := strings.Index(translated, "word")

	// The same text with no entity is the control: it must come out unbold, so
	// the bold run below is the entity and not the renderer's own doing.
	control := NewMessageList(20, 60)
	controlMsg := translatedMessage(1, 7, translated)
	control.SetMessages([]domain.Message{controlMsg})
	assert.NotContains(t, strings.Join(control.renderMessage(controlMsg, false), "\n"), "\x1b[1m")

	ml := NewMessageList(20, 60)
	msg := translatedMessage(1, 7, original)
	ml.SetMessages([]domain.Message{msg})
	ml.SetTranslation(7, msg, "pl", "Polish", domain.MessageTranslation{
		MessageID: 1,
		Text:      translated,
		Entities:  []domain.MessageEntity{{Type: "bold", Offset: boldStart, Length: len("word")}},
	})

	got := strings.Join(ml.renderMessage(msg, false), "\n")
	require.Contains(t, xansi.Strip(got), "bold word here")
	assert.Contains(t, got, "\x1b[1m", "the bold entity of the translation must reach the screen")
}

// Copy takes what is displayed: the translation while it is shown, the original
// once it is cleared, and the original again when the message's own content has
// moved on from what the translation answered.
func TestSelectedMessageText_FollowsWhatIsDisplayed(t *testing.T) {
	ml := NewMessageList(20, 60)
	msg := translatedMessage(1, 7, "original")
	ml.SetMessages([]domain.Message{msg})
	ml.SetTranslation(7, msg, "pl", "Polish", translationOf(1, "tłumaczenie"))

	text, ok := ml.SelectedMessageText()
	require.True(t, ok)
	assert.Equal(t, "tłumaczenie", text)

	ml.ClearTranslation(7, 1)
	text, ok = ml.SelectedMessageText()
	require.True(t, ok)
	assert.Equal(t, "original", text)

	// An edited message is a different question, so the answer to the old one is
	// no longer what is displayed — even while it is still cached.
	ml.SetTranslation(7, msg, "pl", "Polish", translationOf(1, "tłumaczenie"))
	edited := msg
	edited.Text = "original, edited"
	ml.SetMessages([]domain.Message{edited})

	text, ok = ml.SelectedMessageText()
	require.True(t, ok)
	assert.Equal(t, "original, edited", text)
}

// The cache is current only when both the language and the source content match;
// either one moving on makes it stale.
func TestHasCurrentTranslation_NeverCurrentWhenLanguageOrSourceDiffers(t *testing.T) {
	ml := NewMessageList(20, 60)
	msg := translatedMessage(1, 7, "original")
	ml.SetMessages([]domain.Message{msg})
	ml.SetTranslation(7, msg, "pl", "Polish", translationOf(1, "tłumaczenie"))

	assert.True(t, ml.HasCurrentTranslation(7, msg, "pl"))
	assert.False(t, ml.HasCurrentTranslation(7, msg, "de"), "another language is not this answer")
	assert.False(t, ml.HasCurrentTranslation(9, msg, "pl"), "another chat is not this answer")

	edited := msg
	edited.Entities = []domain.MessageEntity{{Type: "bold", Offset: 0, Length: 3}}
	assert.False(t, ml.HasCurrentTranslation(7, edited, "pl"), "the source snapshot moved on")

	// Still displayed, though — which is why HasTranslation exists beside it.
	assert.True(t, ml.HasTranslation(7, 1))
	ml.ClearChatTranslations(7)
	assert.False(t, ml.HasTranslation(7, 1))
}

// An album's caption lives on one part, and not necessarily the anchor. The
// translation is keyed by that part's ID: looking it up under the anchor's would
// miss it, and the album would keep drawing the original caption.
func TestAlbum_CaptionOnNonAnchorPartIsTranslated(t *testing.T) {
	const chatID int64 = 7
	first := domain.Message{ID: 1, ChatID: chatID, GroupedID: 100, SenderID: 5,
		Date: time.Unix(100, 0), Photo: &domain.PhotoRef{ID: 1}}
	second := domain.Message{ID: 2, ChatID: chatID, GroupedID: 100, SenderID: 5,
		Date: time.Unix(100, 0), Photo: &domain.PhotoRef{ID: 2}, Text: "album caption"}
	parts := []domain.Message{first, second}

	ml := NewMessageList(24, 60)
	ml.SetMessages(parts)

	require.NotNil(t, ml.SelectedCaptionMessage())
	assert.Equal(t, 2, ml.SelectedCaptionMessage().ID, "the caption part, not the anchor")

	ml.SetTranslation(chatID, second, "pl", "Polish", translationOf(2, "podpis albumu"))

	assert.True(t, ml.HasTranslation(chatID, 2))
	assert.False(t, ml.HasTranslation(chatID, 1), "the anchor holds no caption, so it holds no translation")

	got := joinedPlain(ml.renderGroupBubble(parts, false))
	assert.Contains(t, got, "podpis albumu")
	assert.NotContains(t, got, "album caption")
	assert.Contains(t, got, "Translated to Polish")

	// Copy is the displayed caption too.
	text, ok := ml.SelectedMessageText()
	require.True(t, ok)
	assert.Equal(t, "podpis albumu", text)
}

// The album's height and render stay in lock-step once a caption is translated,
// which is the same invariant the untranslated album is held to — in both album
// layouts, the vertical stack and the mosaic grid.
func TestAlbum_HeightMatchesRenderWhenCaptionTranslated(t *testing.T) {
	const chatID int64 = 7
	photo := func(id int, text string) domain.Message {
		return domain.Message{ID: id, ChatID: chatID, GroupedID: 100, SenderID: 5,
			Date: time.Unix(100, 0), Photo: &domain.PhotoRef{ID: int64(id)}, Text: text}
	}
	// A generic file: media the album draws as a badge line, with no preview to
	// grid on.
	filePart := func(id int) domain.Message {
		return domain.Message{ID: id, ChatID: chatID, GroupedID: 100, SenderID: 5,
			Date:     time.Unix(100, 0),
			Media:    &domain.MediaRef{Kind: domain.MediaFile, FileName: "report.pdf", Size: 2048},
			Document: &domain.DocumentRef{ID: int64(id), FileName: "report.pdf"}}
	}

	stack := []domain.Message{filePart(1), photo(2, "cap")}
	// Two previewable parts grid; one previewable part plus a file does not, so
	// the same helper picks mosaic and stack respectively.
	grid := []domain.Message{photo(1, ""), photo(2, "cap"), photo(3, ""), photo(4, "")}

	for _, tc := range []struct {
		name  string
		parts []domain.Message
	}{
		{"stack", stack},
		{"mosaic", grid},
	} {
		ml := NewMessageList(30, 60)
		ml.SetMessages(tc.parts)
		_, _, _, _, _, grids := ml.mosaicPlan(tc.parts)
		require.Equal(t, tc.name == "mosaic", grids, "%s: the case must exercise the layout it names", tc.name)

		wantUntranslated := ml.groupHeight(tc.parts)
		require.Equal(t, wantUntranslated, len(ml.renderGroupBubble(tc.parts, false)),
			"%s: the untranslated album is in lock-step to begin with", tc.name)

		captionPart := tc.parts[1]
		ml.SetTranslation(chatID, captionPart, "pl", "Polish",
			translationOf(captionPart.ID, "podpis albumu, na tyle długi żeby się zawinął na kilka wierszy"))

		want := ml.groupHeight(tc.parts)
		got := len(ml.renderGroupBubble(tc.parts, false))
		assert.Equalf(t, want, got, "%s: album height must equal its render", tc.name)

		rows := joinedPlain(ml.renderGroupBubble(tc.parts, false))
		assert.Contains(t, rows, "Translated to Polish")
		assert.NotContains(t, rows, "\ncap\n", "the untranslated caption must not be drawn")
	}
}

// The height the scroll clamp is given equals the rows the bubble actually
// occupies, even when the translation wraps to more lines than the original.
// Without this the frame is anchored above content that is a different number of
// lines tall, and the tail message is clipped (issue #115).
func TestTranslation_HeightMatchesRenderWhenTranslationWraps(t *testing.T) {
	const chatID int64 = 7
	original := "short"
	translation := strings.Repeat("długie tłumaczenie ", 8)

	ml := NewMessageList(30, 60)
	msg := translatedMessage(1, chatID, original)
	ml.SetMessages([]domain.Message{msg})

	msgIdx := msgItemIndex(ml, 1)
	require.GreaterOrEqual(t, msgIdx, 0)

	// Prime the height cache, so the assertions after the translation prove the
	// cache was invalidated rather than merely refilled.
	before := ml.itemHeight(msgIdx)
	require.Equal(t, before, ml.msgHeight(msg), "the untranslated bubble is measured correctly to begin with")
	require.Equal(t, before, len(ml.renderItem(msgIdx, false)))

	ml.SetTranslation(chatID, msg, "pl", "Polish", translationOf(1, translation))

	after := ml.msgHeight(msg)
	if after <= before {
		t.Fatalf("the translation wraps to more lines than the original: before=%d after=%d", before, after)
	}
	assert.Equal(t, after, ml.itemHeight(msgIdx),
		"itemHeight must report the new height: applying a translation invalidates the measured heights")
	assert.Equal(t, after, len(ml.renderItem(msgIdx, false)), "the reserved height must equal the rendered rows")
}

// Applying a translation is a display change, not a scroll change: the viewport
// stays where the reader left it.
func TestTranslation_LeavesTheViewportWhereItWas(t *testing.T) {
	const chatID int64 = 7
	msgs := make([]domain.Message, 0, 12)
	for i := 1; i <= 12; i++ {
		msgs = append(msgs, translatedMessage(i, chatID, strings.Repeat("treść ", 12)))
	}

	ml := NewMessageList(12, 60)
	ml.SetMessages(msgs)
	for i := 0; i < 4; i++ {
		ml.ScrollUp()
	}
	start, offset := ml.ViewStart(), ml.LineOffset()
	require.False(t, start == 0 && offset == 0, "the test must be scrolled away from the top to mean anything")

	target := msgs[2]
	ml.SetTranslation(chatID, target, "pl", "Polish", translationOf(target.ID, strings.Repeat("tłumaczenie ", 10)))

	assert.Equal(t, start, ml.ViewStart(), "viewStart must not move")
	assert.Equal(t, offset, ml.LineOffset(), "the line offset must not move")

	// The frame drawn from that position is still the height the position was
	// computed against.
	assert.Equal(t, ml.msgHeight(target), len(ml.renderItem(msgItemIndex(ml, target.ID), false)))
}

// A reply quotes what is displayed for the message it answers, so a reply to a
// translated message quotes the translation rather than the text behind it. The
// quote is measured from the same content, so the bubble it widens is sized for
// what it will actually draw.
func TestTranslation_ReplyQuotesTheDisplayedText(t *testing.T) {
	const chatID int64 = 7
	orig := translatedMessage(1, chatID, "the original")
	reply := domain.Message{ID: 2, ChatID: chatID, Date: time.Unix(200, 0),
		Text: "my reply", ReplyToMsgID: 1}

	ml := NewMessageList(20, 60)
	ml.SetMessages([]domain.Message{orig, reply})

	before := joinedPlain(ml.renderMessage(reply, false))
	require.Contains(t, before, "the original")

	ml.SetTranslation(chatID, orig, "pl", "Polish", translationOf(1, "oryginał"))

	after := joinedPlain(ml.renderMessage(reply, false))
	assert.Contains(t, after, "oryginał", "the quote must show the translation")
	assert.NotContains(t, after, "the original", "the quote must not quote the text behind the translation")
	assert.Equal(t, ml.msgHeight(reply), len(ml.renderMessage(reply, false)),
		"the reply's height must still match its render")
}

// msgItemIndex is the list item index holding a message, for tests that need to
// ask the list about one item rather than about a message value.
func msgItemIndex(ml *MessageList, msgID int) int {
	for i := range ml.items {
		if ml.items[i].kind == itemMessage && ml.items[i].msg.ID == msgID {
			return i
		}
	}
	return -1
}
