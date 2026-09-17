package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// translationModel builds a model with one chat open over the given messages,
// wired to the in-package stub owner. The messages are seeded straight into the
// window: what these tests exercise is the coordinator's own decisions, and the
// window is an input to them.
func translationModel(t *testing.T, msgs []domain.Message) (RootModel, *ownerStub, *config.Store) {
	t.Helper()
	m, store := applyModel(t, "")
	owner := &ownerStub{translations: map[int]string{}}
	m = m.WithOwner(owner).WithScreen(ScreenMain)
	m.currentChatID = 1
	m.chatMsgs = msgs
	m.chat.SetMessages(msgs)
	m.View()
	return m, owner, store
}

func translationPair() []domain.Message {
	return []domain.Message{
		{ID: 10, ChatID: 1, Text: "hola", Date: time.Unix(1, 0)},
		{ID: 11, ChatID: 1, Text: "adios", Date: time.Unix(2, 0)},
	}
}

// applyLangSetting writes the target language through the store and takes the
// path the settings overlay takes, which is the only way a change reaches a
// running tele (ADR 0009).
func applyLangSetting(t *testing.T, m RootModel, store *config.Store, code string) RootModel {
	t.Helper()
	require.NoError(t, store.Set("translation.target_language", code))
	next, cmd := m.applySettingChange()
	return runTranslationForTest(next, cmd)
}

// runTranslationForTest runs a command tree and feeds every message it produced
// back, following the commands each Update returns — what the bubbletea program
// does with an owner reply and everything it triggers. A command that does not
// answer within commandPatience is a timer rather than a reply, and is left
// alone: running it would only wait out a toast's lifetime.
func runTranslationForTest(m RootModel, cmd tea.Cmd) RootModel {
	if cmd == nil {
		return m
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		switch v := msg.(type) {
		case tea.BatchMsg:
			for _, c := range v {
				m = runTranslationForTest(m, c)
			}
			return m
		case nil:
			return m
		default:
			next, more := m.Update(v)
			return runTranslationForTest(next.(RootModel), more)
		}
	case <-time.After(50 * time.Millisecond):
		return m
	}
}

// displayedText is what a message is currently drawn as: the translation when
// one is current, the original otherwise.
func displayedText(m RootModel, chatID int64, msgID int) string {
	for _, msg := range m.chatMsgs {
		if msg.ID == msgID && msg.ChatID == chatID {
			text, _ := m.chat.DisplayedContentForTest(msg)
			return text
		}
	}
	return ""
}

// The target language is immediate: the open chat is emptied of the old
// language's answers and refilled in the new one, and the intent survives.
func TestTranslation_LanguageChange_RefillsInTheNewLanguage(t *testing.T) {
	m, owner, store := translationModel(t, translationPair())
	owner.translations = map[int]string{11: "bye"}

	next, cmd := m.Update(components.TranslateMsgRequest{MsgID: 11, Enable: true})
	m = runTranslationForTest(next.(RootModel), cmd)
	require.Equal(t, "bye", displayedText(m, 1, 11))

	// What is displayed goes with the language, and what wanted translating is
	// asked for again in the new one.
	owner.translations = map[int]string{11: "auf Wiedersehen"}
	m = applyLangSetting(t, m, store, "de")

	assert.Equal(t, "auf Wiedersehen", displayedText(m, 1, 11))
	assert.Equal(t, []int{11}, m.TranslationIntentForTest(1), "the intent is not the language")
}

// An answer in the old language arriving after the change is about a question
// nobody is asking any more and is ignored.
func TestTranslation_StaleAnswerAfterLanguageChange_IsIgnored(t *testing.T) {
	m, owner, store := translationModel(t, translationPair())
	owner.translations = map[int]string{11: "bye"}

	// Ask, and keep the command to deliver later.
	next, cmd := m.Update(components.TranslateMsgRequest{MsgID: 11, Enable: true})
	m = next.(RootModel)
	require.NotNil(t, cmd)
	require.NotZero(t, m.TranslationPendingForTest())

	owner.translations = map[int]string{11: "auf Wiedersehen"}
	m = applyLangSetting(t, m, store, "de")
	require.Equal(t, "auf Wiedersehen", displayedText(m, 1, 11))

	// The superseded ask finally answers, in the language it was made in.
	owner.translations = map[int]string{11: "bye"}
	m = runTranslationForTest(m, cmd)

	assert.Equal(t, "auf Wiedersehen", displayedText(m, 1, 11),
		"an answer in the old language is not displayed over the new one")
}

// A chat re-entered after a language change asks in the language in force now,
// from the intent it retained.
func TestTranslation_ReopenedChat_AsksInTheCurrentLanguage(t *testing.T) {
	m, owner, store := translationModel(t, translationPair())
	owner.translations = map[int]string{11: "bye"}
	next, cmd := m.Update(components.TranslateMsgRequest{MsgID: 11, Enable: true})
	m = runTranslationForTest(next.(RootModel), cmd)

	owner.translations = map[int]string{11: "auf Wiedersehen"}
	m = applyLangSetting(t, m, store, "de")

	next, cmd = m.reconcileChatTranslations(translationPair())
	m = runTranslationForTest(next.(RootModel), cmd)

	assert.Equal(t, "de", owner.lastTranslateTarget, "re-entry asks in the language in force now")
	assert.Equal(t, "auf Wiedersehen", displayedText(m, 1, 11))
}

// A translated caption changes the bubble's measured height, so the Kitty
// placements measured against the old height must be deleted and retransmitted.
// A plain text translation changes no image and must not touch them (#253).
func TestTranslation_MediaCaptionAsksForAKittyReset(t *testing.T) {
	msgs := []domain.Message{
		{ID: 10, ChatID: 1, Text: "hola", Date: time.Unix(1, 0)},
		{ID: 11, ChatID: 1, Text: "adios", Date: time.Unix(2, 0),
			Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 99}},
	}
	m, owner, _ := translationModel(t, msgs)
	m.imageMode = media.ModeKitty
	owner.translations = map[int]string{11: "bye"}

	// A placement is live on the terminal, measured against the caption's old
	// height. The reset is only observable when there is something to delete.
	const photoID = int64(99)
	cols := m.chat.PhotoContentCols()
	m.kittyStore.MarkTransmitted(photoID, cols)
	m.kittyLive[photoID] = true

	next, cmd := m.Update(components.TranslateMsgRequest{MsgID: 11, Enable: true})
	m = runTranslationForTest(next.(RootModel), cmd)

	assert.False(t, m.kittyLive[photoID],
		"a caption that changed size invalidates the placement drawn at the old one")
}

// A translation of a plain text message leaves the terminal's placements alone:
// nothing about an image changed.
func TestTranslation_PlainTextCaptionLeavesKittyAlone(t *testing.T) {
	m, owner, _ := translationModel(t, translationPair())
	m.imageMode = media.ModeKitty
	owner.translations = map[int]string{11: "bye"}

	const photoID = int64(99)
	m.kittyStore.MarkTransmitted(photoID, m.chat.PhotoContentCols())
	m.kittyLive[photoID] = true

	next, cmd := m.Update(components.TranslateMsgRequest{MsgID: 11, Enable: true})
	m = runTranslationForTest(next.(RootModel), cmd)

	assert.True(t, m.kittyLive[photoID], "a text-only translation resets nothing")
}

// The whole settings path, with no double in it: the real keymap opens the real
// overlay over a real config file, the real store writes the code, and the model
// the app is running on picks the new language up. What the overlay shows and
// what the app asks in must not be able to disagree (#253).
func TestTranslation_LanguageChosenInTheOverlayReachesTheApp(t *testing.T) {
	m, store := applyModel(t, "")
	m = m.WithScreen(ScreenMain).WithSettingsStore(store)
	require.Equal(t, "pl", m.targetLanguage(), "the default before anything is chosen")

	// `,` opens the settings overlay, as the real keymap has it.
	next, _ := m.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	m = next.(RootModel)
	require.True(t, m.SettingsOpen(), ", opens the overlay")

	// Walk to the translation row and move it along, as a person does.
	for i := 0; i < 200 && m.Settings().CursorLabelForTest() != "Target language"; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = next.(RootModel)
	}
	require.Equal(t, "Target language", m.Settings().CursorLabelForTest(), "the row is reachable")

	next, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = next.(RootModel)
	require.True(t, m.SettingsOpen(), "still open after choosing")

	// The file holds the code, and the app runs on it.
	assert.NotEqual(t, "pl", m.targetLanguage(), "the running app follows the file")
	assert.Contains(t, m.targetLanguage(), store.Current().Translation.TargetLanguage)
}
