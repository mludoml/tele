package keys_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
)

func TestDescribe_DefaultFallback(t *testing.T) {
	// "up" has no per-context override; it resolves via the shared default.
	lbl, ok := keys.Describe(keys.ContextChatList, keys.ActionUp)
	assert.True(t, ok)
	assert.Equal(t, "up", lbl.Short)
}

func TestDescribe_ContextOverride(t *testing.T) {
	// down means "move" in a list but "scroll" in the chat pane.
	list, _ := keys.Describe(keys.ContextChatList, keys.ActionDown)
	chat, _ := keys.Describe(keys.ContextChat, keys.ActionDown)
	assert.Equal(t, "move", list.Short)
	assert.Equal(t, "scroll", chat.Short)
}

func TestDescribe_LongDefaultsToShort(t *testing.T) {
	lbl, _ := keys.Describe(keys.ContextChat, keys.ActionReply)
	assert.Equal(t, "reply", lbl.Short)
	assert.Equal(t, "reply", lbl.Long) // Long empty in table -> mirrors Short
}

func TestDescribe_Unknown(t *testing.T) {
	_, ok := keys.Describe(keys.ContextChat, keys.Action("nonexistent"))
	assert.False(t, ok)
}

// translate has a label in both menus: bound to "n" by default in the message
// context menu, and still reachable unbound (Space → j/k → Enter) in the chat
// menu's automatic-mode toggle.
func TestDescribe_TranslateHasLabelInBothMenus(t *testing.T) {
	for _, ctx := range []keys.Context{keys.ContextContextMenu, keys.ContextChatMenu} {
		lbl, ok := keys.Describe(ctx, keys.ActionTranslate)
		assert.Truef(t, ok, "no label for translate in context %q", ctx)
		assert.Equal(t, "translate", lbl.Short)
	}
}

// The chat-list automatic-mode toggle must not take a key away: no default
// binding in chat_menu. The message menu's translate binding is covered by
// TestDefaultKeyMap_ContextContextMenu instead.
func TestDefaultKeyMap_ChatMenuTranslateIsUnbound(t *testing.T) {
	for key, action := range keys.DefaultKeyMap()[keys.ContextChatMenu] {
		assert.NotEqualf(t, keys.ActionTranslate, action,
			"chat_menu translate shipped unbound but is bound to %q", key)
	}
}

// Drift guard: every action bound in DefaultKeyMap has a non-empty label in
// its context. A new binding without a label fails here.
func TestDescribe_EveryBoundActionHasLabel(t *testing.T) {
	for ctx, binds := range keys.DefaultKeyMap() {
		for key, action := range binds {
			lbl, ok := keys.Describe(ctx, action)
			assert.Truef(t, ok, "no label for action %q (context %q, key %q)", action, ctx, key)
			assert.NotEmptyf(t, lbl.Short, "empty Short for action %q in context %q", action, ctx)
		}
	}
}
