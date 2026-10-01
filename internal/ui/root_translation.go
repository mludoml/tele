package ui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/translation"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// Translation is display state and is deliberately kept here, in the client,
// and nowhere else: nothing is written back onto domain.Message, into the store
// or to Telegram, and nothing survives the process. Restarting tele restores
// every original, and the only thing that persists is the language it would
// translate into.
//
// Two kinds of intent produce a translation, and they are tracked apart:
//
//   - An explicit message intent is the person asking, once, for this message.
//   - Automatic chat mode is the person asking for every text message of a chat,
//     as it is loaded, backfilled, edited or received.
//
// Desired visibility is the union:
//
//	explicit[key] || (autoChat[chatID] && !originalOverride[key])
//
// Inside automatic mode "Show original" is an exemption rather than the removal
// of an intent, which is what makes turning the mode off give back exactly the
// messages that were individually translated rather than a chat full of
// translations nobody asked for.

// translationKey names one message's translation. The chat is half of it: a
// message ID is unique within its chat and means nothing outside it, so keying
// by ID alone would let two chats collide (#253).
type translationKey struct {
	ChatID int64
	MsgID  int
}

// translationSource is the exact content a request was made against. A
// translation answers one text, so an edit — the text or its formatting — makes
// any cached or in-flight answer about it stale, and the comparison is what
// detects that rather than a second event saying "this changed".
type translationSource struct {
	Text     string
	Entities []domain.MessageEntity
}

// sourceOf snapshots a message's content.
func sourceOf(msg domain.Message) translationSource {
	return translationSource{Text: msg.Text, Entities: msg.Entities}
}

// matches reports whether a message still carries exactly this content.
func (s translationSource) matches(msg domain.Message) bool {
	return s.Text == msg.Text && slices.Equal(s.Entities, msg.Entities)
}

// translationOrigin says what asked for a translation, which decides what
// happens when it fails.
type translationOrigin uint8

const (
	// originManual is one message the person asked for. A failure is reported
	// and the action that asked for it is rolled back exactly.
	originManual translationOrigin = iota
	// originAutoChat is automatic chat mode. A failure is the feature saying
	// no, so the mode is turned off rather than a toast per message.
	originAutoChat
)

// translationRequest is one in-flight ask: what it is about, what the content
// was, and everything needed to judge the answer and to undo the action if it
// is refused.
type translationRequest struct {
	key    translationKey
	source translationSource
	// serial is per key and monotonic within the process. A second ask about the
	// same message outruns the first, and only the newest answer may be applied.
	serial int
	// target is the language code this request asked for. A completion carrying
	// a code the running config no longer names is old news.
	target string
	origin translationOrigin
	// generation is the automatic-chat generation this request was made under,
	// for originAutoChat only. Turning the mode off or changing the language
	// advances it, which retires every request made under the old one.
	generation int
	// preExplicit and preOverride are what the action that started this request
	// changed, so a refusal can put them back exactly as they were: an explicit
	// intent that was recorded, or an override that was removed.
	preExplicit bool
	preOverride bool
}

// translationSuccess is one applied answer, carried in a completion.
type translationSuccess struct {
	key        translationKey
	source     translationSource
	value      domain.MessageTranslation
	language   string
	origin     translationOrigin
	generation int
	serial     int
}

// translationDoneMsg is one batch of asks coming back. It is deliberately one
// message for the whole batch: successes and the failure are judged together,
// so a batch that half-succeeded does not report and roll back in two passes.
type translationDoneMsg struct {
	successes []translationSuccess
	// requested is every key the batch asked about, success or not.
	requested []translationRequest
	target    string
	// err is the terminal failure of the batch, nil when every batch succeeded.
	err error
}

// translationState is the whole of the client's translation intent and
// bookkeeping. It is a value on RootModel rather than a pointer so a copied
// model cannot share the maps by accident.
type translationState struct {
	// explicit is the messages the person asked to see translated.
	explicit map[translationKey]bool
	// originalOverride is the messages exempted while their chat is in
	// automatic mode. Kept apart from explicit so leaving the mode restores
	// exactly the originals that were asked for.
	originalOverride map[translationKey]bool
	// autoChat is the chats in automatic translation, mapped to the generation
	// in force. The generation is what makes a change of intent retire the
	// requests made under the old one.
	autoChat map[int64]int
	// pending is the in-flight request per key, newest only.
	pending map[translationKey]translationRequest
	// failureGeneration records the automatic generation whose failure has
	// already been reported per chat, so one refusal produces one toast.
	failureGeneration map[int64]int
	// serial is the process-wide request counter.
	serial int
	// generation is the process-wide automatic-chat counter.
	generation int
}

func newTranslationState() translationState {
	return translationState{
		explicit:          make(map[translationKey]bool),
		originalOverride:  make(map[translationKey]bool),
		autoChat:          make(map[int64]int),
		pending:           make(map[translationKey]translationRequest),
		failureGeneration: make(map[int64]int),
	}
}

// translationBatchSize bounds one RPC. Telegram takes a list of message IDs and
// documents nothing about a comfortable length, so it is bounded here rather
// than discovered with a refusal.
const translationBatchSize = 20

// targetLanguage is the language the running config names, and the empty string
// when there is no config at all.
func (m RootModel) targetLanguage() string {
	if m.cfg == nil || m.cfg.Translation.TargetLanguage == "" {
		return translation.Default
	}
	return m.cfg.Translation.TargetLanguage
}

// translationKeyFor names the message a translation would be kept under. The
// chat comes from the message itself: a message carries its own chat, and the
// window it arrived in is not needed to identify it.
func translationKeyFor(msg domain.Message) translationKey {
	return translationKey{ChatID: msg.ChatID, MsgID: msg.ID}
}

// messageTranslationDesired reports whether a message should be showing a
// translation, from the two kinds of intent and the one kind of exemption.
func (m RootModel) messageTranslationDesired(msg domain.Message) bool {
	key := translationKeyFor(msg)
	if m.translation.explicit[key] {
		return true
	}
	if _, auto := m.translation.autoChat[key.ChatID]; auto {
		return !m.translation.originalOverride[key]
	}
	return false
}

// autoChatTranslationEnabled reports whether a chat is in automatic
// translation. The chat-list menu reads it, so the row says what the next press
// will do.
func (m RootModel) autoChatTranslationEnabled(chatID int64) bool {
	_, ok := m.translation.autoChat[chatID]
	return ok
}

// translationMenuState answers what the message menu needs to know: which
// message the translation action addresses, and whether it is currently
// desired. The ID is the caption-bearing album part's, not the anchor's,
// because a collapsed album's caption lives on one part and the translation of
// that caption is keyed by it.
//
// Desired is taken from the coordinator's intent rather than from the cache:
// a request still in flight must already offer "Show original", or pressing the
// row twice would ask for the same thing twice.
func (m *RootModel) translationMenuState(msgID int) (int, bool) {
	msg := m.captionMessage(msgID)
	if msg == nil || msg.Text == "" {
		return 0, false
	}
	return msg.ID, m.messageTranslationDesired(*msg)
}

// captionMessage returns the message a translation action on the given
// selection addresses: the real caption-bearing album part when the selection
// is a collapsed album, the message itself otherwise.
//
// The selection names an album by its anchor — SelectedMessageID is the item's
// first part — so the album case is recognised by the selection matching rather
// than by the caption part matching the ID it was asked about.
func (m *RootModel) captionMessage(msgID int) *domain.Message {
	if m.chat == nil || msgID == 0 {
		return nil
	}
	if caption := m.chat.SelectedCaptionMessage(); caption != nil && m.chat.SelectedMessageID() == msgID {
		return caption
	}
	// The selection is something else: a message the current window holds.
	// Looked up here rather than held, because the window is the client's own
	// copy and the caller named an ID from it.
	for i := range m.chatMsgs {
		if m.chatMsgs[i].ID == msgID {
			return &m.chatMsgs[i]
		}
	}
	return nil
}

// applyMessageTranslation handles the message menu's toggle. The two modes are
// deliberately different acts:
//
//   - Outside automatic mode, "Translate" records an explicit intent for the
//     message, and "Show original" removes it.
//   - Inside automatic mode, "Translate" only removes the message's exemption
//     and "Show original" only adds one. Neither leaves behind an individual
//     intent, so turning the chat's mode off gives back exactly what was asked
//     for rather than a chat full of translations.
func (m RootModel) applyMessageTranslation(req components.TranslateMsgRequest, msg domain.Message) (RootModel, tea.Cmd) {
	key := translationKeyFor(msg)
	preExplicit := m.translation.explicit[key]
	preOverride := m.translation.originalOverride[key]

	auto := m.autoChatTranslationEnabled(key.ChatID)
	if req.Enable {
		if auto {
			delete(m.translation.originalOverride, key)
		} else {
			m.translation.explicit[key] = true
		}
	} else {
		delete(m.translation.explicit, key)
		if auto {
			m.translation.originalOverride[key] = true
		}
	}

	// A translation of this message is now wanted or not wanted, and either way
	// the answer to the previous question is not the answer to this one.
	m.retireRequest(key)

	if !m.messageTranslationDesired(msg) {
		// Nothing to ask for. What was displayed is dropped, and a caption that
		// shrinks back has to be re-measured by the terminal.
		had := m.chat.HasCurrentTranslation(key.ChatID, msg, m.targetLanguage())
		m.chat.ClearTranslation(key.ChatID, key.MsgID)
		return m, m.afterTranslationChange(msg, had)
	}
	return m, m.translationCmdFor([]domain.Message{msg}, originManual, preExplicit, preOverride)
}

// applyChatTranslation handles the chat menu's toggle for one chat.
//
// Enabling is session-only and, for a chat that is not open, asks Telegram for
// nothing: the mode joins the messages already loaded when the chat is next
// opened, which is what ChatReset reconciles. That is deliberate — the chat
// list's action must not fetch a history page nobody asked to see.
//
// Disabling is the reverse, and it takes the displayed translations with it:
// the messages that were translated only because the chat was in automatic mode
// go back to their originals, while a message somebody translated on its own
// stays translated.
func (m RootModel) applyChatTranslation(chatID int64, enable bool) (RootModel, tea.Cmd) {
	if enable {
		if m.autoChatTranslationEnabled(chatID) {
			return m, nil
		}
		m.translation.generation++
		m.translation.autoChat[chatID] = m.translation.generation
		delete(m.translation.failureGeneration, chatID)
		if chatID != m.currentChatID {
			// Not on screen: nothing to reconcile now, and no fetch to start.
			return m, nil
		}
		return m.reconcileChatTranslations(m.chatMsgs)
	}
	cmds := m.disableChatTranslation(chatID)
	return m, cmds
}

// disableChatTranslation turns automatic mode off for a chat and clears exactly
// what it was responsible for.
//
// It is one function because three callers must agree on what turning the mode
// off means: the menu's "Show original", a refusal that takes the mode with it,
// and a change of target language (which does not turn the mode off, but does
// retire its requests through the same generation).
func (m RootModel) disableChatTranslation(chatID int64) tea.Cmd {
	if _, on := m.translation.autoChat[chatID]; !on {
		return nil
	}
	// The generation advances first: every request made under the old one is
	// retired by the comparison, including the one whose failure is running
	// this.
	m.translation.generation++
	delete(m.translation.autoChat, chatID)
	delete(m.translation.failureGeneration, chatID)
	for key := range m.translation.originalOverride {
		if key.ChatID == chatID {
			delete(m.translation.originalOverride, key)
		}
	}

	// Displayed translations go, except the ones carrying an intent of their
	// own. An exemption that was in place is dropped with the mode, so every
	// message without an explicit intent is back to its original.
	var cmds []tea.Cmd
	for key := range m.translation.pending {
		if key.ChatID == chatID {
			delete(m.translation.pending, key)
		}
	}
	if chatID == m.currentChatID {
		for _, msg := range m.chatMsgs {
			key := translationKeyFor(msg)
			if m.translation.explicit[key] {
				continue
			}
			if m.chat.HasCurrentTranslation(key.ChatID, msg, m.targetLanguage()) {
				m.chat.ClearTranslation(key.ChatID, key.MsgID)
				cmds = append(cmds, m.afterTranslationChange(msg, true))
			}
		}
		// The chat is on screen and its mode is off; whatever explicit intent
		// remains still wants its messages translated, and the requests that
		// were just retired have to be re-asked.
		var reconcile tea.Cmd
		m, reconcile = m.reconcileChatTranslations(m.chatMsgs)
		cmds = append(cmds, reconcile)
	}
	return tea.Batch(cmds...)
}

// retireRequest drops a pending request for a key, which is how a superseded
// ask stops being applied. The serial is advanced with it so an answer that
// arrives afterwards is refused by the serial check as well.
func (m *RootModel) retireRequest(key translationKey) {
	if _, ok := m.translation.pending[key]; ok {
		m.translation.serial++
		delete(m.translation.pending, key)
	}
}

// afterTranslationChange returns the command a translation change needs besides
// the repaint: a caption that changed size is a Kitty placement measured at the
// old one.
//
// Only media captions matter. A plain text translation changes the text of a
// bubble and nothing about an image, so resetting the terminal's placements
// would delete and retransmit every visible picture for nothing.
// It takes the model by pointer because the reset it requests is state the
// caller returns: a value receiver would set the flag on a copy that is thrown
// away, and the placements would go on being drawn at the old height.
func (m *RootModel) afterTranslationChange(msg domain.Message, changed bool) tea.Cmd {
	if !changed || !hasMediaCaption(msg) {
		return nil
	}
	m.requestKittyReset()
	return m.reconcileKittyCmd()
}

// hasMediaCaption reports whether a message's text is the caption of media,
// which is what makes a translation change its measured geometry.
func hasMediaCaption(msg domain.Message) bool {
	return msg.Media != nil || msg.Photo != nil
}

// reconcileChatTranslations brings the chat's loaded window in line with the
// intent. It asks for what is wanted and not yet showing, and drops what is
// showing and no longer wanted.
//
// Every path that changes either the window's contents or the intent goes
// through this: a reset, a backfill, an append, an edit, and the language
// change. One place that decides what is wanted is what keeps those paths from
// disagreeing.
func (m RootModel) reconcileChatTranslations(msgs []domain.Message) (RootModel, tea.Cmd) {
	target := m.targetLanguage()
	var cmds []tea.Cmd
	// Wanted messages are grouped by what asked for them, because a batch is
	// one question and a refusal answers it one way: a message the person asked
	// for is rolled back on its own, a message the mode asked for takes the mode
	// with it. A chat in automatic mode whose message also carries an explicit
	// intent is the mode's to answer for, which is what originFor says.
	wanted := map[translationOrigin][]domain.Message{
		originManual:   nil,
		originAutoChat: nil,
	}

	for _, msg := range msgs {
		if msg.Text == "" {
			continue
		}
		key := translationKeyFor(msg)
		desired := m.messageTranslationDesired(msg)
		has := m.chat.HasCurrentTranslation(key.ChatID, msg, target)
		pending, isPending := m.translation.pending[key]
		currentPending := isPending && pending.serial == m.translation.serial && pending.source.matches(msg) && pending.target == target

		switch {
		case desired && has:
			// Already right. Nothing to ask and nothing to clear.
		case desired:
			// Wanted and not showing, and not already on its way. An equivalent
			// ask in flight is left alone rather than repeated.
			if !currentPending {
				origin := m.originFor(key.ChatID)
				wanted[origin] = append(wanted[origin], msg)
			}
		case has:
			// No longer wanted: an override was added, the mode went off, or the
			// language changed. Whatever is displayed goes.
			m.chat.ClearTranslation(key.ChatID, key.MsgID)
			cmds = append(cmds, m.afterTranslationChange(msg, true))
		}
	}
	// The automatic asks go first: a refusal there turns the mode off and its
	// own reconcile re-asks what remains, so ordering them last would ask for
	// messages the mode had just given up on.
	for _, origin := range []translationOrigin{originAutoChat, originManual} {
		if len(wanted[origin]) == 0 {
			continue
		}
		cmds = append(cmds, m.translationCmdFor(wanted[origin], origin, false, false))
	}
	return m, tea.Batch(cmds...)
}

// translationCmdFor builds the command that asks Telegram for the wanted
// messages and reports back once, for the whole batch.
//
// The asks are split into stable batches and issued one after another: the RPC
// takes a list, and a window is not a reason to issue one request per bubble.
// A failure in any batch is carried alongside the successes rather than
// replacing them, because a half-filled chat is a better answer than none.
func (m RootModel) translationCmdFor(msgs []domain.Message, origin translationOrigin, preExplicit, preOverride bool) tea.Cmd {
	if m.owner == nil || len(msgs) == 0 {
		return nil
	}
	target := m.targetLanguage()
	generation := 0
	if origin == originAutoChat && len(msgs) > 0 {
		generation = m.translation.autoChat[msgs[0].ChatID]
	}
	chatID := msgs[0].ChatID

	requests := make([]translationRequest, 0, len(msgs))
	for _, msg := range msgs {
		key := translationKeyFor(msg)
		if m.chat.HasCurrentTranslation(key.ChatID, msg, target) {
			continue
		}
		m.translation.serial++
		requests = append(requests, translationRequest{
			key:         key,
			source:      sourceOf(msg),
			serial:      m.translation.serial,
			target:      target,
			origin:      origin,
			generation:  generation,
			preExplicit: preExplicit,
			preOverride: preOverride,
		})
		m.translation.pending[key] = requests[len(requests)-1]
	}
	if len(requests) == 0 {
		return nil
	}

	ctx, owner, chatID := m.ctx, m.owner, msgs[0].ChatID
	return func() tea.Msg {
		done := translationDoneMsg{requested: requests, target: target}
		for start := 0; start < len(requests); start += translationBatchSize {
			end := min(start+translationBatchSize, len(requests))
			batch := requests[start:end]
			ids := make([]int, 0, len(batch))
			for _, r := range batch {
				ids = append(ids, r.key.MsgID)
			}
			values, err := owner.TranslateMessages(ctx, chatID, ids, target)
			if err != nil {
				done.err = err
				return done
			}
			byID := make(map[int]domain.MessageTranslation, len(values))
			for _, v := range values {
				byID[v.MessageID] = v
			}
			for _, r := range batch {
				v, ok := byID[r.key.MsgID]
				if !ok {
					continue
				}
				done.successes = append(done.successes, translationSuccess{
					key:        r.key,
					source:     r.source,
					value:      v,
					language:   translation.Name(target),
					origin:     r.origin,
					generation: r.generation,
					serial:     r.serial,
				})
			}
		}
		return done
	}
}

// handleTranslationDone applies one batch's answer.
//
// Provenance is checked before anything is written, cleared or reported: an
// answer is about a particular ask, and if the ask was superseded — a newer
// request for the same message, a different language, a chat whose mode went
// off, an edit — the answer has no effects at all. Nothing here can be right
// about a question that is no longer being asked (#253).
func (m RootModel) handleTranslationDone(msg translationDoneMsg) (RootModel, tea.Cmd) {
	target := m.targetLanguage()
	applied := make(map[translationKey]bool)
	var cmds []tea.Cmd

	// Successes first, so a batch that half-succeeded keeps the half that worked
	// before any failure decides what to do with the rest.
	for _, s := range msg.successes {
		req, ok := m.translation.pending[s.key]
		if !ok || req.serial != s.serial {
			continue
		}
		if req.target != target || s.language != translation.Name(target) {
			continue
		}
		if req.origin == originAutoChat {
			if gen, on := m.translation.autoChat[s.key.ChatID]; !on || gen != req.generation {
				continue
			}
		}
		display := m.displayedMessage(s.key)
		if display == nil || !s.source.matches(*display) {
			// The message left the window, or its content moved on. The answer
			// is still an answer to the question that was asked, so the pending
			// entry goes — but nothing is displayed from an unknown source.
			delete(m.translation.pending, s.key)
			continue
		}
		if !m.messageTranslationDesired(*display) {
			delete(m.translation.pending, s.key)
			continue
		}
		m.chat.SetTranslation(s.key.ChatID, *display, target, s.language, s.value)
		delete(m.translation.pending, s.key)
		applied[s.key] = true
		cmds = append(cmds, m.afterTranslationChange(*display, true))
	}

	if msg.err == nil {
		return m, tea.Batch(cmds...)
	}

	// The failure is only news if some part of the batch is still current.
	current := m.currentRequests(msg.requested, target)
	if len(current) == 0 {
		return m, tea.Batch(cmds...)
	}
	if current[0].origin == originAutoChat {
		chatID := current[0].key.ChatID
		if m.translation.failureGeneration[chatID] == current[0].generation {
			// Already reported for this generation: a second batch of the same
			// failed mode says nothing and does not turn it off twice.
			return m, tea.Batch(cmds...)
		}
		m.translation.failureGeneration[chatID] = current[0].generation
		// One refusal is the feature saying no, not one message's problem: the
		// mode goes off through the ordinary path, which preserves every
		// explicit intent and reports once.
		cmds = append(cmds, m.disableChatTranslation(chatID))
		cmds = append(cmds, statusErrCmd("translate", msg.err))
		return m, tea.Batch(cmds...)
	}

	// Manual: the action that asked for it is put back exactly as it was, and
	// the refusal is reported once. This covers both directions — asking for a
	// translation and reversing an exemption inside automatic mode — because
	// both record pre-action state before they start.
	for _, req := range current {
		m.restoreIntent(req)
		delete(m.translation.pending, req.key)
	}
	if len(applied) == 0 {
		cmds = append(cmds, statusErrCmd("translate", msg.err))
	}
	return m, tea.Batch(cmds...)
}

// statusErrCmd turns a message-shaped status into the command that delivers it,
// which is what a command tree needs.
func statusErrCmd(action string, err error) tea.Cmd {
	msg := errStatus(action, err)
	if msg == nil {
		return nil
	}
	return func() tea.Msg { return msg }
}

// currentRequests filters a batch's requests down to the ones still being
// asked: same target, same generation where it applies, same serial.
func (m RootModel) currentRequests(reqs []translationRequest, target string) []translationRequest {
	out := make([]translationRequest, 0, len(reqs))
	for _, req := range reqs {
		pending, ok := m.translation.pending[req.key]
		if !ok || pending.serial != req.serial {
			continue
		}
		if req.target != target {
			continue
		}
		if req.origin == originAutoChat {
			if gen, on := m.translation.autoChat[req.key.ChatID]; !on || gen != req.generation {
				continue
			}
		}
		out = append(out, req)
	}
	return out
}

// restoreIntent puts back what the action that started a request changed.
func (m *RootModel) restoreIntent(req translationRequest) {
	if req.preExplicit {
		m.translation.explicit[req.key] = true
	} else {
		delete(m.translation.explicit, req.key)
	}
	if req.preOverride {
		m.translation.originalOverride[req.key] = true
	} else {
		delete(m.translation.originalOverride, req.key)
	}
}

// displayedMessage returns the current copy of a message, when the open chat
// holds it. A message that left the window has no copy to apply an answer to;
// the intent is kept, so it is translated again if it comes back.
func (m *RootModel) displayedMessage(key translationKey) *domain.Message {
	if key.ChatID != m.currentChatID {
		return nil
	}
	for i := range m.chatMsgs {
		if m.chatMsgs[i].ID == key.MsgID {
			return &m.chatMsgs[i]
		}
	}
	return nil
}

// invalidateTranslationsForLanguage is the target-language change: the language
// is immediate, so what is on screen is in the wrong one the moment it changes.
//
// Every displayed translation goes, every request in flight is retired, and the
// intent is kept — explicit messages and chats in automatic mode alike, because
// the question they answer ("show me this in my language") is unchanged. What
// wanted a translation still wants one; it is only the language that moved.
func (m RootModel) invalidateTranslationsForLanguage() (RootModel, tea.Cmd) {
	if len(m.translation.pending) > 0 {
		m.translation.serial++
		clear(m.translation.pending)
	}
	// A new language is a new generation for every chat in automatic mode, so
	// even the answers that arrive without a pending entry are refused.
	for chatID := range m.translation.autoChat {
		m.translation.generation++
		m.translation.autoChat[chatID] = m.translation.generation
	}
	clear(m.translation.failureGeneration)

	m.chat.ClearTranslations()
	cmds := []tea.Cmd{m.reconcileKittyCmd()}
	reconciled, cmd := m.reconcileChatTranslations(m.chatMsgs)
	cmds = append(cmds, cmd)
	return reconciled, tea.Batch(cmds...)
}

// reconcileTranslationSource is the edit path: a message's content changed, so
// any translation of the old content is stale and any request for it is
// answering a question nobody is asking any more. When the message is still
// wanted, the new content is asked for now.
func (m RootModel) reconcileTranslationSource(msg domain.Message) (RootModel, tea.Cmd) {
	key := translationKeyFor(msg)
	pending, isPending := m.translation.pending[key]
	staleRequest := isPending && !pending.source.matches(msg)
	staleDisplay := m.chat.HasTranslation(key.ChatID, key.MsgID) &&
		!m.chat.HasCurrentTranslation(key.ChatID, msg, m.targetLanguage())

	if !staleRequest && !staleDisplay {
		return m, nil
	}
	if staleRequest {
		m.retireRequest(key)
	}
	if staleDisplay {
		m.chat.ClearTranslation(key.ChatID, key.MsgID)
	}
	cmd := m.afterTranslationChange(msg, staleDisplay)
	if !m.messageTranslationDesired(msg) || msg.Text == "" {
		return m, cmd
	}
	return m, tea.Batch(cmd, m.translationCmdFor([]domain.Message{msg}, m.originFor(key.ChatID), false, false))
}

// originFor says which kind of intent a chat's messages are being asked under,
// which decides what a failure does.
func (m RootModel) originFor(chatID int64) translationOrigin {
	if m.autoChatTranslationEnabled(chatID) {
		return originAutoChat
	}
	return originManual
}

// TranslationMenuStateForTest answers what the message menu would offer for a
// selection: the id the translation row addresses, and whether it reads
// "Show original" — which is true while an ask is still in flight, not only once
// an answer is cached.
func (m RootModel) TranslationMenuStateForTest(msgID int) (int, bool) {
	return m.translationMenuState(msgID)
}

// TranslationIntentForTest exposes which messages a chat would want translated,
// so tests can assert on intent rather than on rendering.
func (m RootModel) TranslationIntentForTest(chatID int64) []int {
	return m.translationIntentSnapshotForTest(chatID)
}

// TranslationOverridesForTest exposes the per-message exemptions inside
// automatic mode.
func (m RootModel) TranslationOverridesForTest(chatID int64) []int {
	return m.translationOverrideSnapshotForTest(chatID)
}

// TranslationPendingForTest exposes how many requests are in flight.
func (m RootModel) TranslationPendingForTest() int { return len(m.translation.pending) }

// AutoChatTranslationEnabledForTest reports whether a chat is in automatic
// translation.
func (m RootModel) AutoChatTranslationEnabledForTest(chatID int64) bool {
	return m.autoChatTranslationEnabled(chatID)
}

// translationIntentSnapshotForTest exposes the messages a chat wants translated,
// for tests that assert on intent rather than on rendering.
func (m RootModel) translationIntentSnapshotForTest(chatID int64) []int {
	var out []int
	for key, on := range m.translation.explicit {
		if on && key.ChatID == chatID {
			out = append(out, key.MsgID)
		}
	}
	slices.Sort(out)
	return out
}

// translationOverrideSnapshotForTest exposes the per-message exemptions inside
// automatic mode.
func (m RootModel) translationOverrideSnapshotForTest(chatID int64) []int {
	var out []int
	for key, on := range m.translation.originalOverride {
		if on && key.ChatID == chatID {
			out = append(out, key.MsgID)
		}
	}
	slices.Sort(out)
	return out
}

// handleTranslateMsgRequest is the message menu's toggle: the menu named a
// message, and the coordinator decides what the press means from the intent it
// holds rather than from what happens to be cached.
func (m RootModel) handleTranslateMsgRequest(req components.TranslateMsgRequest) (RootModel, tea.Cmd) {
	m.contextMenu = nil
	msg := m.captionMessage(req.MsgID)
	if msg == nil || msg.Text == "" {
		return m, nil
	}
	return m.applyMessageTranslation(req, *msg)
}

// handleTranslateChatRequest is the chat menu's toggle, which is session-only
// state and nothing Telegram is told about.
func (m RootModel) handleTranslateChatRequest(req components.TranslateChatRequest) (RootModel, tea.Cmd) {
	m.chatMenu = nil
	return m.applyChatTranslation(req.Peer.ID, req.Enable)
}
