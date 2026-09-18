package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// rootOnRichMessage opens a chat holding one bot message with an inline keyboard
// (and optionally a block document), then drains the resulting projections.
func rootOnRichMessage(t *testing.T, msg domain.Message) (RootModel, *ownerStub) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Bot", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	msg.ChatID = 1
	if msg.Date.IsZero() {
		msg.Date = time.Unix(1000, 0)
	}
	st.SetMessages(1, []domain.Message{msg})

	m := newRootInternal(st, 50).WithScreen(ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(RootModel)

	newM, _ = m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Bot"})
	m = newM.(RootModel)
	newM, _ = applyEventInternal(t, m, st, store.Event{
		Kind: store.EventNewMessage,
		Message: domain.Message{ID: msg.ID, ChatID: 1, Text: msg.Text, Date: msg.Date,
			RichBlocks: msg.RichBlocks, ReplyMarkup: msg.ReplyMarkup},
	})
	m = newM.(RootModel)
	o, ok := m.owner.(*ownerStub)
	require.True(t, ok)
	return m, o
}

// view is the model's screen with every toast settled onto it, so a test reads
// what the person would see rather than a toast mid-slide.
func view(m RootModel) string {
	m.SettleToastsForTest()
	return m.View().Content
}

// key sends one keypress to the model and returns the result.
func key(t *testing.T, m RootModel, msg tea.KeyPressMsg) (RootModel, tea.Cmd) {
	t.Helper()
	return send(t, m, msg)
}

// send delivers any message to the model and returns the updated model.
func send(t *testing.T, m RootModel, msg tea.Msg) (RootModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	root, ok := next.(RootModel)
	require.True(t, ok)
	return root, cmd
}

func callbackMarkup(texts ...string) *domain.ReplyMarkup {
	row := make([]domain.KeyboardButton, 0, len(texts))
	for _, txt := range texts {
		row = append(row, domain.KeyboardButton{
			Text:   txt,
			Action: domain.ButtonAction{Kind: domain.ButtonActionCallback, Data: []byte(txt)},
		})
	}
	return &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{row}}
}

// b enters the keyboard focus mode and leaves it again: it is a door both ways.
func TestRoot_ButtonKeyTogglesTheMode(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi", ReplyMarkup: callbackMarkup("one", "two")})

	m, _ = key(t, m, tea.KeyPressMsg{Code: 'B', Text: "B"})
	assert.True(t, m.chat.InButtonMode(), "B enters the keyboard")

	m, _ = key(t, m, tea.KeyPressMsg{Code: 'B', Text: "B"})
	assert.False(t, m.chat.InButtonMode(), "B leaves it again")
}

// The mode is refused (rather than opened empty) when the selected message has no
// keyboard, and the refusal is visible.
func TestRoot_ButtonKeyOnAMessageWithoutAKeyboard(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "no buttons here"})

	m, cmd := key(t, m, tea.KeyPressMsg{Code: 'B', Text: "B"})
	assert.False(t, m.chat.InButtonMode())
	require.NotNil(t, cmd, "a toast is scheduled")
	assert.Contains(t, view(m), "no inline buttons")
}

// Tab and shift+tab move the cursor inside the mode; the press sends the button
// the cursor is on, with its own payload.
func TestRoot_ButtonNavigationAndPress(t *testing.T) {
	m, owner := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi", ReplyMarkup: callbackMarkup("one", "two")})

	m, _ = key(t, m, tea.KeyPressMsg{Code: 'B', Text: "B"})
	require.True(t, m.chat.InButtonMode())

	m, _ = key(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	btn, ok := m.chat.SelectedButton()
	require.True(t, ok)
	assert.Equal(t, "two", btn.Text, "tab moves to the next button")

	owner.callbackAnswer = domain.CallbackAnswer{Message: "chosen"}
	_, cmd := key(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	require.IsType(t, buttonPressedMsg{}, msg)
	_, cmd = send(t, m, msg.(buttonPressedMsg))
	assert.Equal(t, 5, owner.pressedMsgID)
	assert.Equal(t, []byte("two"), owner.pressedData, "the payload of the button under the cursor")
	require.NotNil(t, cmd, "the answer is shown and a timer retires it")
	assert.Contains(t, view(m), "chosen")
}

// A link button opens in the browser and never reaches the bot: nothing about it
// is a callback.
func TestRoot_PressLinkButtonOpensTheBrowser(t *testing.T) {
	restore := SetURLOpenerForTest(func(string) {})
	defer restore()

	m, owner := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi",
		ReplyMarkup: &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{
			{{Text: "Docs", Action: domain.ButtonAction{Kind: domain.ButtonActionURL, URL: "https://example.com"}}},
		}}})
	m, _ = key(t, m, tea.KeyPressMsg{Code: 'B', Text: "B"})

	_, cmd := key(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "opening a URL is a command")
	cmd()
	assert.Zero(t, owner.pressedMsgID, "a link button is not a callback press")
}

// A button whose action this client cannot carry out says so rather than sending
// a callback that would never be understood.
func TestRoot_PressUnsupportedButtonExplains(t *testing.T) {
	m, owner := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi",
		ReplyMarkup: &domain.ReplyMarkup{Rows: [][]domain.KeyboardButton{
			{{Text: "Phone", Action: domain.ButtonAction{Kind: domain.ButtonActionNone, Reason: "asks for your phone number"}}},
		}}})
	// The mode still opens: the keyboard exists and its buttons must be visible.
	m, _ = key(t, m, tea.KeyPressMsg{Code: 'B', Text: "B"})

	_, cmd := key(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	assert.Zero(t, owner.pressedMsgID, "an unsupported button sends nothing")
	// The button explains itself where it is drawn, and the keypress says the
	// same thing where the press would have reported.
	assert.Contains(t, view(m), "Phone: asks for your phone")
	assert.Contains(t, view(m), "button unavailable")
}

// A refused press is reported like any other command failure.
func TestRoot_PressButtonRefusedIsReported(t *testing.T) {
	m, owner := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi", ReplyMarkup: callbackMarkup("one")})
	owner.err = &telerr.Error{Kind: telerr.Forbidden}

	m, _ = key(t, m, tea.KeyPressMsg{Code: 'B', Text: "B"})
	_, cmd := key(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	require.IsType(t, StatusErrMsg{}, msg)
}

// Esc leaves the mode without leaving the chat: the mode is a cursor over
// buttons, not a screen.
func TestRoot_EscLeavesButtonMode(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi", ReplyMarkup: callbackMarkup("one")})
	m, _ = key(t, m, tea.KeyPressMsg{Code: 'B', Text: "B"})
	require.True(t, m.chat.InButtonMode())

	m, _ = key(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	assert.False(t, m.chat.InButtonMode())
	assert.NotZero(t, m.currentChatID, "the chat stays open")
}

// An answer that arrives after the chat was left must not be shown against
// whatever is on screen now.
func TestRoot_LateButtonAnswerIsDropped(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi", ReplyMarkup: callbackMarkup("one")})
	m2, _ := send(t, m, buttonPressedMsg{
		chatID: 999,
		msgID:  5,
		answer: domain.CallbackAnswer{Message: "stale"},
	})
	assert.NotContains(t, view(m2), "stale")
}

// A bot's alert wants the reader to stop and read: it is shown as a warning,
// while an ordinary answer is informational.
func TestRoot_ButtonAnswerAlertIsAWarning(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi", ReplyMarkup: callbackMarkup("one")})

	quiet, _ := send(t, m, buttonPressedMsg{chatID: 1, msgID: 5, answer: domain.CallbackAnswer{Message: "noted"}})
	require.Contains(t, view(quiet), "noted")

	loud, _ := send(t, m, buttonPressedMsg{chatID: 1, msgID: 5, answer: domain.CallbackAnswer{Message: "careful", Alert: true}})
	assert.Contains(t, view(loud), "careful")
}

// A silent answer is an answer, and showing a toast about nothing is noise.
func TestRoot_SilentButtonAnswerShowsNothing(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi", ReplyMarkup: callbackMarkup("one")})
	m2, cmd := send(t, m, buttonPressedMsg{chatID: 1, msgID: 5})
	assert.Nil(t, cmd)
	assert.NotContains(t, view(m2), "button")
}

// The details key expands and collapses the reachable section of the selected
// rich message, and the toggle is reflected in the drawn height.
func TestRoot_ToggleDetailsKey(t *testing.T) {
	blocks := []domain.PageBlock{{
		Kind:     domain.BlockKindDetails,
		Text:     &domain.RichText{Text: "More"},
		Children: []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "the body"}}},
	}}
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, RichBlocks: blocks})
	require.NotContains(t, m.View().Content, "the body", "a collapsed section hides its content")

	m, _ = key(t, m, tea.KeyPressMsg{Code: 'v', Text: "v"})
	assert.Contains(t, view(m), "the body", "v expands the section")

	m, _ = key(t, m, tea.KeyPressMsg{Code: 'v', Text: "v"})
	assert.NotContains(t, view(m), "the body", "v collapses it again")
}

// The details key is inert on a message with no collapsible section: it must not
// report anything or change what is drawn.
func TestRoot_ToggleDetailsOnAPlainMessage(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "plain"})
	before := view(m)
	m2, cmd := key(t, m, tea.KeyPressMsg{Code: 'v', Text: "v"})
	assert.Nil(t, cmd)
	assert.Equal(t, before, view(m2))
}

// The config gate is a rendering decision and nothing more: blocks and keyboards
// stay parsed and stored, and switching it off is a repaint.
func TestRoot_RichMessagesDisabledFallsBackToText(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Bot", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	st.SetMessages(1, []domain.Message{{
		ID: 5, ChatID: 1, Date: time.Unix(1000, 0),
		Text:        "flattened text",
		RichBlocks:  []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "a block"}}},
		ReplyMarkup: callbackMarkup("one"),
	}})

	m := newRootInternal(st, 50).WithScreen(ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(RootModel)
	m = m.WithConfig(&config.Config{RichMessages: config.RichMessagesConfig{Enabled: true}})
	newM, _ = m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Bot"})
	m = newM.(RootModel)
	o, _ := m.owner.(*ownerStub)
	drained, _ := o.drain(m)
	m = drained.(RootModel)

	cfg := &config.Config{RichMessages: config.RichMessagesConfig{Enabled: true}}
	m = m.WithConfig(cfg)
	require.Contains(t, view(m), "a block", "the default is on and the block is drawn")

	cfg.RichMessages.Enabled = false
	m = m.WithConfig(cfg)

	off := view(m)
	assert.Contains(t, off, "flattened text", "the server's flattened text is what a client without blocks shows")
	assert.NotContains(t, off, "a block")
}

// A streaming draft is drawn over the open chat, and the pane says so.
func TestRoot_EphemeralDraftShowsOverlay(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi"})

	m, cmd := send(t, m, core.EphemeralDraft{
		ChatID: 1,
		Draft: domain.EphemeralDraft{ChatID: 1, ID: 7,
			TextBlocks: []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "streaming now"}}}},
	})
	require.True(t, m.chat.DraftActive())
	require.NotNil(t, cmd, "the marker starts animating")
	assert.Contains(t, view(m), "streaming now")
	assert.Contains(t, view(m), "generating")
}

// A revision replaces what is on screen: one document is being written, not a
// scrollback of its drafts.
func TestRoot_EphemeralDraftRevisionReplaces(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi"})

	m, _ = send(t, m, core.EphemeralDraft{ChatID: 1, Draft: domain.EphemeralDraft{
		ChatID: 1, ID: 7,
		TextBlocks: []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "first half"}}},
	}})
	m, _ = send(t, m, core.EphemeralDraft{ChatID: 1, Draft: domain.EphemeralDraft{
		ChatID: 1, ID: 7,
		TextBlocks: []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "second half"}}},
	}})

	out := view(m)
	assert.Contains(t, out, "second half")
	assert.NotContains(t, out, "first half")
}

// The end of the stream takes the overlay away.
func TestRoot_EphemeralDraftEndClearsOverlay(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi"})

	m, _ = send(t, m, core.EphemeralDraft{ChatID: 1, Draft: domain.EphemeralDraft{
		ChatID: 1, ID: 7,
		TextBlocks: []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "streaming"}}},
	}})
	require.True(t, m.chat.DraftActive())

	m, _ = send(t, m, core.EphemeralDraft{ChatID: 1})
	assert.False(t, m.chat.DraftActive())
	assert.NotContains(t, view(m), "streaming")
}

// A draft for a chat nobody is looking at is dropped rather than kept: what the
// overlay shows is a document being written now.
func TestRoot_EphemeralDraftForAnotherChatIsDropped(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "hi"})

	m, cmd := send(t, m, core.EphemeralDraft{ChatID: 999, Draft: domain.EphemeralDraft{
		ChatID: 999, ID: 7,
		TextBlocks: []domain.PageBlock{{Kind: domain.BlockKindParagraph, Text: &domain.RichText{Text: "elsewhere"}}},
	}})
	assert.False(t, m.chat.DraftActive())
	assert.Nil(t, cmd)
	assert.NotContains(t, view(m), "elsewhere")
}

// The overlay takes its rows out of the history rather than overdrawing it, and
// the composer stays where it was.
func TestRoot_EphemeralDraftKeepsTheHistoryAndComposer(t *testing.T) {
	m, _ := rootOnRichMessage(t, domain.Message{ID: 5, Text: "a real message"})

	before := strings.Count(view(m), "\n")

	m, _ = send(t, m, core.EphemeralDraft{ChatID: 1, Draft: domain.EphemeralDraft{
		ChatID: 1, ID: 7, Placeholder: true, Text: "thinking",
		TextBlocks: []domain.PageBlock{{Kind: domain.BlockKindThinking, Text: &domain.RichText{Text: "thinking"}}},
	}})

	out := view(m)
	assert.Contains(t, out, "thinking")
	assert.Contains(t, out, "a real message", "the history above the overlay is still drawn")
	assert.Equal(t, before, strings.Count(out, "\n"), "the overlay takes room rather than adding it")
}
