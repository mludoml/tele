package components

import (
	"slices"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// Translated display lives here, in the message list, and nowhere else: the
// message the store holds is never rewritten, so the original is always what
// the next render falls back to, and nothing about a translation survives the
// process. The list is where it lives because the list is what has to agree
// with itself about it — the bubble's width, its height, its text, its reply
// quotes, its album caption and what Copy hands over are all read from one
// place, and that place must not be able to disagree with the render.
//
// The cache is keyed by the message's chat as well as its ID. A message ID is
// unique within its chat and means nothing outside it, so keying by ID alone
// would let a translation of one chat's message appear on another's.

// translationKey names one message's translation.
type translationKey struct {
	chatID int64
	msgID  int
}

// translationEntry is one message's translation plus the content it answers.
//
// The source snapshot is what makes an entry current or stale: a translation
// answers one text, so an edit — the words or their formatting — leaves the
// entry describing something the message no longer says, and comparing against
// the snapshot is how that is noticed rather than a second event having to
// announce it.
type translationEntry struct {
	targetCode   string
	languageName string
	text         string
	entities     []domain.MessageEntity

	sourceText     string
	sourceEntities []domain.MessageEntity
}

// answers reports whether the entry is about exactly the content the message
// carries now.
func (e translationEntry) answers(msg domain.Message) bool {
	return e.sourceText == msg.Text && slices.Equal(e.sourceEntities, msg.Entities)
}

// SetTranslation records the translation of one message. source is the message
// as it was when the translation was asked for; value is what came back. The
// chat ID is passed explicitly because it is the caller's key, and the message
// it belongs to is not always in this list (a caption part of a collapsed album
// is, but the caller should not have to prove it).
func (ml *MessageList) SetTranslation(chatID int64, source domain.Message, targetCode, languageName string, value domain.MessageTranslation) {
	if ml.translations == nil {
		ml.translations = make(map[translationKey]translationEntry)
	}
	ml.translations[translationKey{chatID: chatID, msgID: source.ID}] = translationEntry{
		targetCode:     targetCode,
		languageName:   languageName,
		text:           value.Text,
		entities:       value.Entities,
		sourceText:     source.Text,
		sourceEntities: source.Entities,
	}
	ml.invalidateHeights()
}

// ClearTranslation drops one message's translation, if it has one.
func (ml *MessageList) ClearTranslation(chatID int64, msgID int) {
	key := translationKey{chatID: chatID, msgID: msgID}
	if _, ok := ml.translations[key]; !ok {
		return
	}
	delete(ml.translations, key)
	ml.invalidateHeights()
}

// ClearChatTranslations drops every translation of one chat, leaving other
// chats' alone. Called when a chat is forgotten rather than merely scrolled
// away from.
func (ml *MessageList) ClearChatTranslations(chatID int64) {
	cleared := false
	for key := range ml.translations {
		if key.chatID == chatID {
			delete(ml.translations, key)
			cleared = true
		}
	}
	if cleared {
		ml.invalidateHeights()
	}
}

// ClearTranslations drops every translation. It is the target-language change:
// what is displayed is in the wrong language the moment the setting moves.
func (ml *MessageList) ClearTranslations() {
	if len(ml.translations) == 0 {
		return
	}
	clear(ml.translations)
	ml.invalidateHeights()
}

// HasTranslation reports whether any translation is displayed for a message,
// whatever language it is in and whatever content it was made from. It is the
// question the caller asks to tell "nothing translated here" from "something
// translated, but out of date" — the second has to be cleared and its geometry
// re-measured, and HasCurrentTranslation cannot say which it is.
func (ml *MessageList) HasTranslation(chatID int64, msgID int) bool {
	_, ok := ml.translations[translationKey{chatID: chatID, msgID: msgID}]
	return ok
}

// HasCurrentTranslation reports whether the displayed translation of this
// message is in the language asked about and still answers its current content.
// A language that is no longer the target, or a snapshot that no longer matches
// an edited message, reports false: the answer would be about a question that
// is no longer being asked.
func (ml *MessageList) HasCurrentTranslation(chatID int64, source domain.Message, targetCode string) bool {
	entry, ok := ml.translations[translationKey{chatID: chatID, msgID: source.ID}]
	if !ok || entry.targetCode != targetCode {
		return false
	}
	return entry.answers(source)
}

// currentTranslation returns the entry to draw this message with: the one held
// for it when its source snapshot still matches what the message says, nothing
// otherwise.
//
// The language is deliberately not compared here. The entry is the only place
// the displayed language is recorded, so "which language is this in" is
// answered by the entry itself; a caller that needs to know whether that
// language is the one currently configured asks HasCurrentTranslation, which
// does compare it.
//
// The chat comes from the message: a message carries its own chat, and an album
// caption is looked up under the id of the part that holds it, not the anchor's.
func (ml *MessageList) currentTranslation(msg domain.Message) (translationEntry, bool) {
	entry, ok := ml.translations[translationKey{chatID: msg.ChatID, msgID: msg.ID}]
	if !ok || !entry.answers(msg) {
		return translationEntry{}, false
	}
	return entry, true
}

// effectiveContent is the single decision point for what a message is drawn
// with: the translation while one is current for this message, the message's
// own text and entities otherwise, plus the marker line the bubble carries when
// something is translated.
//
// Everything that depends on the text goes through here — the width measured
// for the bubble, the wrapped line count, the rendered body, a reply quote of
// this message, an album's caption, and what Copy hands over. Patching only the
// render would leave the height, the hit testing, the scroll clamp and Copy
// describing the original while the screen showed the translation.
func (ml *MessageList) effectiveContent(msg domain.Message) (text string, entities []domain.MessageEntity, marker string) {
	entry, ok := ml.currentTranslation(msg)
	if !ok {
		return msg.Text, msg.Entities, ""
	}
	return entry.text, entry.entities, "Translated to " + entry.languageName
}

// DisplayedContentForTest reports the text a message is currently drawn with —
// the translation while one is current, the message's own otherwise — for tests
// that assert on what the screen shows rather than on the cache behind it.
func (ml *MessageList) DisplayedContentForTest(msg domain.Message) (string, bool) {
	text, _, _ := ml.effectiveContent(msg)
	return text, text != ""
}

// widenForMarker grows a content width so it can hold the translation marker,
// capped at the widest the bubble is allowed to be. The marker is a row of the
// bubble like any other, so the bubble has to be measured for it in the same
// place the text is — a width that ignored it would render a border torn by a
// line nobody measured.
func widenForMarker(actualW, maxContentW int, marker string) int {
	if marker == "" {
		return actualW
	}
	w := lipgloss.Width(marker)
	if w > maxContentW {
		w = maxContentW
	}
	if w > actualW {
		return w
	}
	return actualW
}

// translationMarkerLine renders the one dim marker row a translated bubble
// carries after its body, inside the bubble borders like every other label
// line. A marker wider than the content width is truncated rather than allowed
// to tear the right border.
//
// The style is the timestamp's dim: a note about the bubble rather than part of
// it, so it reads as being about the text above instead of as more of it. It is
// an existing token; translation does not get one of its own.
func translationMarkerLine(marker string, actualW int, b lipgloss.Border, bs lipgloss.Style) string {
	label := marker
	if w := lipgloss.Width(label); w > actualW {
		label = xansi.Truncate(label, actualW, "")
	}
	return paintedLabelLine(theme.S().Timestamp.Render(label), lipgloss.Width(label), actualW, b, bs)
}
