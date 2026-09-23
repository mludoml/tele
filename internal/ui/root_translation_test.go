package ui_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/translation"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// translationRoot opens a chat holding the given messages, wired to the
// stand-in owner, and lays the view out so a message is selected. The messages
// reach the window through the subscription's Reset, as production delivers it.
func translationRoot(t *testing.T, msgs []domain.Message) (ui.RootModel, store.Store, *testOwner) {
	t.Helper()
	m, st := newRootOnChat(t)
	st.SetMessages(1, msgs)
	nm, _ := applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)
	m.View()
	return m, st, m.Owner().(*testOwner)
}

// runCmd runs a command tree and feeds every message it produced back into the
// model, following the commands each Update returns — which is what the
// bubbletea program does with an owner reply and everything it triggers.
//
// A command that has not answered within commandPatience is a timer rather than
// a reply. An Update returns both a query and the tick that will retire a toast,
// and running the tick would make the test wait out the toast's lifetime and
// then assert on a screen the toast had already left.
func runCmd(t *testing.T, m ui.RootModel, cmd tea.Cmd) ui.RootModel {
	t.Helper()
	switch msg := runCmdOnce(cmd).(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = runCmd(t, m, c)
		}
		return m
	case nil:
		return m
	default:
		next, more := m.Update(msg)
		return runCmd(t, next.(ui.RootModel), more)
	}
}

// commandPatience is how long a test waits for a command to produce a message
// before treating it as a timer. Every command here answers from a stub and
// returns at once; the timers count seconds.
const commandPatience = 50 * time.Millisecond

func runCmdOnce(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		return msg
	case <-time.After(commandPatience):
		return nil
	}
}

// toastText is what the toast stack is currently saying, with its styling
// stripped. A toast wraps to the corner it is drawn in, so a phrase is asserted
// against the words rather than against one contiguous run.
func toastText(m ui.RootModel) string { return xansi.Strip(m.View().Content) }

// selectedText is what the selected message is currently displayed as, which is
// the thing Copy copies and the thing the person reads.
func selectedText(t *testing.T, m ui.RootModel) string {
	t.Helper()
	text, ok := m.Chat().SelectedMessageText()
	require.True(t, ok, "a text message must be selected")
	return text
}

// toggleTranslation sends the message menu's request and runs whatever it
// returns, as the menu's own action does.
func toggleTranslation(t *testing.T, m ui.RootModel, msgID int, enable bool) ui.RootModel {
	t.Helper()
	next, cmd := m.Update(components.TranslateMsgRequest{MsgID: msgID, Enable: enable})
	return runCmd(t, next.(ui.RootModel), cmd)
}

// toggleChatTranslation sends the chat menu's request and runs the result.
func toggleChatTranslation(t *testing.T, m ui.RootModel, chatID int64, enable bool) ui.RootModel {
	t.Helper()
	next, cmd := m.Update(components.TranslateChatRequest{
		Peer:   domain.Peer{ID: chatID, Type: domain.PeerUser},
		Enable: enable,
	})
	return runCmd(t, next.(ui.RootModel), cmd)
}

func twoTextMessages() []domain.Message {
	return []domain.Message{
		{ID: 10, ChatID: 1, Text: "hola", Date: time.Unix(1, 0)},
		{ID: 11, ChatID: 1, Text: "adios", Date: time.Unix(2, 0)},
	}
}

// A manual ask translates exactly the message it named, and the answer replaces
// the body.
func TestTranslation_ManualAsk_TranslatesThatMessageOnly(t *testing.T) {
	m, _, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{10: "hello", 11: "bye"}

	m = toggleTranslation(t, m, 11, true)

	require.Len(t, owner.translateReqs, 1, "one ask for one message")
	assert.Equal(t, []int{11}, owner.translateReqs[0].msgIDs)
	assert.Equal(t, translation.Default, owner.translateReqs[0].target)
	assert.Equal(t, "bye", selectedText(t, m))
}

// Show original gives back the exact original body.
func TestTranslation_ShowOriginal_RestoresTheBody(t *testing.T) {
	m, _, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{11: "bye"}
	m = toggleTranslation(t, m, 11, true)
	require.Equal(t, "bye", selectedText(t, m))

	m = toggleTranslation(t, m, 11, false)

	assert.Equal(t, "adios", selectedText(t, m))
	assert.Empty(t, m.TranslationIntentForTest(1), "the intent goes with the display")
}

// An edit arrives as a projection update. The translation answered the old text,
// so it is dropped; the message is still wanted, so it is asked for again under
// the new content — and displays the answer to the new question.
func TestTranslation_EditInvalidatesAndReasks(t *testing.T) {
	m, st, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{11: "bye"}
	m = toggleTranslation(t, m, 11, true)
	require.Equal(t, "bye", selectedText(t, m))
	owner.translateReqs = nil
	owner.translations = map[int]string{11: "goodbye"}

	nm, cmd := applyEvent(t, m, st, store.Event{Kind: store.EventEditMessage, ChatID: 1, MsgID: 11,
		Message: domain.Message{ID: 11, ChatID: 1, Text: "adios amigo", Date: time.Unix(2, 0)}})
	m = runCmd(t, nm.(ui.RootModel), cmd)
	m.View()

	require.Len(t, owner.translateReqs, 1, "the edited message is asked about again")
	assert.Equal(t, []int{11}, owner.translateReqs[0].msgIDs)
	assert.Equal(t, "goodbye", selectedText(t, m),
		"what is displayed answers the text the message carries now, not the one it did")
}

// A reaction is not a content change and the translation stands.
func TestTranslation_ReactionKeepsTheAnswer(t *testing.T) {
	m, st, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{11: "bye"}
	m = toggleTranslation(t, m, 11, true)

	nm, _ := applyEvent(t, m, st, store.Event{Kind: store.EventReactionsUpdate, ChatID: 1, MsgID: 11, Reactions: []domain.Reaction{{Emoji: "👍", Count: 1}}})
	m = nm.(ui.RootModel)
	m.View()

	assert.Equal(t, "bye", selectedText(t, m),
		"a reaction says nothing about the text, so the translation still answers it")
}

// Automatic mode translates the window that is already loaded.
func TestTranslation_ChatMode_TranslatesTheLoadedWindow(t *testing.T) {
	m, _, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{10: "hello", 11: "bye"}

	m = toggleChatTranslation(t, m, 1, true)

	assert.True(t, m.AutoChatTranslationEnabledForTest(1))
	require.Len(t, owner.translateReqs, 1, "the loaded window is asked for in one query")
	assert.ElementsMatch(t, []int{10, 11}, owner.translateReqs[0].msgIDs)
	assert.Contains(t, m.View().Content, "bye")
}

// A chat that is not open goes into automatic mode without asking Telegram for
// anything: the mode joins the window when it is next opened. That is what keeps
// the chat-list action from fetching history nobody asked to see.
func TestTranslation_ChatMode_OnAClosedChatAsksForNothing(t *testing.T) {
	m, _, owner := translationRoot(t, twoTextMessages())

	m = toggleChatTranslation(t, m, 2, true)

	assert.True(t, m.AutoChatTranslationEnabledForTest(2))
	assert.Empty(t, owner.translateReqs)
}

// An exemption inside automatic mode gives back that one message, and turning
// the mode off gives back the rest — without leaving an individual intent
// behind.
func TestTranslation_ChatModeOff_RestoresOriginalsAndKeepsNoIntent(t *testing.T) {
	m, _, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{10: "hello", 11: "bye"}

	m = toggleChatTranslation(t, m, 1, true)
	require.Contains(t, m.View().Content, "bye")

	m = toggleTranslation(t, m, 11, false)
	assert.Contains(t, m.View().Content, "adios")
	assert.Equal(t, []int{11}, m.TranslationOverridesForTest(1))
	assert.Empty(t, m.TranslationIntentForTest(1), "an exemption is not an intent")

	m = toggleChatTranslation(t, m, 1, false)

	m.SettleToastsForTest()

	assert.False(t, m.AutoChatTranslationEnabledForTest(1))
	assert.Contains(t, m.View().Content, "adios")
	assert.Contains(t, m.View().Content, "hola")
	assert.Empty(t, m.TranslationOverridesForTest(1))
}

// A message translated on its own is an intent of its own, and survives the
// chat's automatic mode being turned off.
func TestTranslation_ChatModeOff_KeepsAnIndependentTranslation(t *testing.T) {
	m, _, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{10: "hello", 11: "bye"}

	m = toggleTranslation(t, m, 11, true)
	require.Equal(t, "bye", selectedText(t, m))

	m = toggleChatTranslation(t, m, 1, true)
	m = toggleChatTranslation(t, m, 1, false)

	assert.Equal(t, []int{11}, m.TranslationIntentForTest(1))
	assert.Equal(t, "bye", selectedText(t, m), "the message somebody translated themselves stays translated")
}

// A refused manual ask is reported once and rolled back exactly.
func TestTranslation_ManualFailure_RollsBackAndReportsOnce(t *testing.T) {
	m, _, owner := translationRoot(t, twoTextMessages())
	owner.cmdErr = &telerr.Error{Kind: telerr.Rejected, Reason: telerr.ReasonTranslationUnavailable}

	m = toggleTranslation(t, m, 11, true)

	m.SettleToastsForTest()

	assert.Empty(t, m.TranslationIntentForTest(1))
	assert.Zero(t, m.TranslationPendingForTest())
	assert.Contains(t, toastText(m), "unavailable for this account")
	assert.Equal(t, 1, owner.translationsCalled)
}

// A refusal while automatic mode is on is the feature saying no: the mode goes
// off once, the automatic results go, an explicit intent is preserved, and later
// deltas produce no further query.
func TestTranslation_ChatModeFailure_TurnsTheModeOffOnce(t *testing.T) {
	m, st, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{10: "hello", 11: "bye"}
	m = toggleTranslation(t, m, 11, true)
	require.Equal(t, "bye", selectedText(t, m))

	owner.cmdErr = &telerr.Error{Kind: telerr.Rejected, Reason: telerr.ReasonTranslationUnavailable}
	m = toggleChatTranslation(t, m, 1, true)

	m.SettleToastsForTest()

	assert.False(t, m.AutoChatTranslationEnabledForTest(1))
	assert.Equal(t, []int{11}, m.TranslationIntentForTest(1), "the explicit intent survives")
	assert.Contains(t, toastText(m), "unavailable for this account")
	require.Equal(t, 2, owner.translationsCalled, "the manual ask and the chat-mode ask")

	owner.cmdErr = nil
	before := owner.translationsCalled
	nm, cmd := applyEvent(t, m, st, store.Event{Kind: store.EventNewMessage, Message: domain.Message{
		ID: 12, ChatID: 1, Text: "otra", Date: time.Unix(3, 0),
	}})
	runCmd(t, nm.(ui.RootModel), cmd)

	assert.Equal(t, before, owner.translationsCalled, "a mode that was turned off issues no further queries")
}

// A backfilled page joins automatic mode as it arrives.
func TestTranslation_Backfill_JoinsAutomaticMode(t *testing.T) {
	m, st, owner := translationRoot(t, twoTextMessages())
	m = toggleChatTranslation(t, m, 1, true)
	owner.translateReqs = nil
	owner.translations = map[int]string{9: "old"}

	st.SetMessages(1, append([]domain.Message{
		{ID: 9, ChatID: 1, Text: "viejo", Date: time.Unix(0, 0)},
	}, twoTextMessages()...))
	nm, cmd := applyHistory(t, m, st, 1)
	m = runCmd(t, nm.(ui.RootModel), cmd)

	require.NotEmpty(t, owner.translateReqs)
	var asked []int
	for _, r := range owner.translateReqs {
		asked = append(asked, r.msgIDs...)
	}
	assert.Contains(t, asked, 9)
	assert.Contains(t, m.View().Content, "old")
}

// A message arriving in an automatic chat is translated as it lands.
func TestTranslation_ArrivingMessage_JoinsAutomaticMode(t *testing.T) {
	m, st, owner := translationRoot(t, twoTextMessages())
	m = toggleChatTranslation(t, m, 1, true)
	owner.translateReqs = nil
	owner.translations = map[int]string{12: "new"}

	nm, cmd := applyEvent(t, m, st, store.Event{Kind: store.EventNewMessage, Message: domain.Message{
		ID: 12, ChatID: 1, Text: "nuevo", Date: time.Unix(3, 0),
	}})
	m = runCmd(t, nm.(ui.RootModel), cmd)

	require.NotEmpty(t, owner.translateReqs)
	assert.Equal(t, []int{12}, owner.translateReqs[0].msgIDs)
	assert.Contains(t, m.View().Content, "new")
}

// An edit inside automatic mode re-asks for the edited message only.
func TestTranslation_EditInsideAutomaticMode_ReasksThatMessage(t *testing.T) {
	m, st, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{10: "hello", 11: "bye"}
	m = toggleChatTranslation(t, m, 1, true)
	owner.translateReqs = nil
	owner.translations = map[int]string{11: "goodbye"}

	nm, cmd := applyEvent(t, m, st, store.Event{Kind: store.EventEditMessage, ChatID: 1, MsgID: 11, Message: domain.Message{ID: 11, ChatID: 1, Text: "adios amigo"}})
	m = runCmd(t, nm.(ui.RootModel), cmd)

	require.Len(t, owner.translateReqs, 1, "only the edited message is re-asked for")
	assert.Equal(t, []int{11}, owner.translateReqs[0].msgIDs)
	assert.Contains(t, m.View().Content, "goodbye")
	assert.Contains(t, m.View().Content, "hello", "the untouched message keeps its translation")
}

// A message scrolled out of the window and back is not asked for again: window
// eviction is not deletion, and the cached answer still answers the same text.
func TestTranslation_WindowEviction_ReusesTheAnswer(t *testing.T) {
	m, st, owner := translationRoot(t, twoTextMessages())

	// Translate the older message explicitly and scroll the window past it.
	m = toggleTranslation(t, m, 10, true)
	require.Equal(t, []int{10}, m.TranslationIntentForTest(1))
	owner.translateReqs = nil

	st.SetMessages(1, twoTextMessages()[1:])
	nm, _ := applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)

	st.SetMessages(1, twoTextMessages())
	nm, _ = applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)

	assert.Empty(t, owner.translateReqs, "the same content is answered from what is already held")
	assert.Equal(t, []int{10}, m.TranslationIntentForTest(1), "the intent outlives the window")
}

// The menu says "Show original" while the ask is still in flight, so the row
// names what the next press will do rather than what happens to be cached.
func TestTranslation_MenuSaysShowOriginalWhilePending(t *testing.T) {
	m, _, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{11: "bye"}

	next, cmd := m.Update(components.TranslateMsgRequest{MsgID: 11, Enable: true})
	m = next.(ui.RootModel)
	require.NotNil(t, cmd)
	require.NotZero(t, m.TranslationPendingForTest(), "the ask is in flight")

	translationID, desired := m.TranslationMenuStateForTest(11)

	assert.Equal(t, 11, translationID)
	assert.True(t, desired, "a pending ask already offers Show original")
}

// The same question after the answer landed still says "Show original".
func TestTranslation_MenuSaysShowOriginalAfterTheAnswer(t *testing.T) {
	m, _, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{11: "bye"}

	m = toggleTranslation(t, m, 11, true)

	translationID, desired := m.TranslationMenuStateForTest(11)

	assert.Equal(t, 11, translationID)
	assert.True(t, desired)
}

// An album's caption lives on one part, and translation follows that part
// rather than the anchor the other menu rows address.
func TestTranslation_MenuTargetsTheCaptionBearingAlbumPart(t *testing.T) {
	msgs := []domain.Message{
		{ID: 20, ChatID: 1, GroupedID: 7, Date: time.Unix(1, 0),
			Photo: &domain.PhotoRef{ID: 100}, Media: &domain.MediaRef{Kind: domain.MediaPhoto}},
		{ID: 21, ChatID: 1, GroupedID: 7, Date: time.Unix(2, 0), Text: "album caption",
			Photo: &domain.PhotoRef{ID: 101}, Media: &domain.MediaRef{Kind: domain.MediaPhoto}},
	}
	m, _, _ := translationRoot(t, msgs)

	translationID, _ := m.TranslationMenuStateForTest(20)

	assert.Equal(t, 21, translationID,
		"the caption is on part 2, so that is the message translation addresses")
}

// The whole way a person does it, through the frame the terminal draws: select
// a message, press Space, pick Translate, read the translated body with its
// marker, and put the original back. Every other test here drives one step; this
// one is the sequence, and it asserts on the rendered frame rather than on the
// model's fields, so a break in the menu-to-render path cannot hide behind them.
func TestTranslation_SpaceMenuTranslateThenShowOriginal(t *testing.T) {
	m, _, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{11: "bye"}

	// Space opens the message menu.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = next.(ui.RootModel)
	require.True(t, m.ContextMenuOpen(), "space opens the message menu")

	// The Translate row is offered, and choosing it translates.
	next, cmd := m.Update(components.TranslateMsgRequest{MsgID: 11, Enable: true})
	m = runCmd(t, next.(ui.RootModel), cmd)
	frame := xansi.Strip(m.View().Content)

	assert.Contains(t, frame, "bye", "the body is the translation")
	assert.NotContains(t, frame, "adios", "the original is replaced, not kept beside it")
	assert.Contains(t, frame, "Translated to Polish", "and the bubble says which language it is in")

	// Show original gives the exact original back.
	next, cmd = m.Update(components.TranslateMsgRequest{MsgID: 11, Enable: false})
	m = runCmd(t, next.(ui.RootModel), cmd)
	frame = xansi.Strip(m.View().Content)

	assert.Contains(t, frame, "adios", "the original body is back")
	assert.NotContains(t, frame, "Translated to Polish", "and the marker is gone with it")
}

// Copy hands over what is displayed, which is the rule a person relies on
// without thinking about it: what they see is what they get.
func TestTranslation_CopyFollowsTheDisplay(t *testing.T) {
	var copied string
	defer ui.SetClipboardWriterForTest(func(s string) error { copied = s; return nil })()

	m, _, owner := translationRoot(t, twoTextMessages())
	owner.translations = map[int]string{11: "bye"}
	m = toggleTranslation(t, m, 11, true)

	next, cmd := m.Update(components.CopyMsgRequest{})
	m = runCmd(t, next.(ui.RootModel), cmd)
	assert.Equal(t, "bye", copied, "Copy copies the translation while it is on screen")

	m = toggleTranslation(t, m, 11, false)
	next, cmd = m.Update(components.CopyMsgRequest{})
	runCmd(t, next.(ui.RootModel), cmd)
	assert.Equal(t, "adios", copied, "and the original once Show original has restored it")
}
