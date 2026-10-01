package components_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

func TestChatMenu_ReadLabelForUnreadChat(t *testing.T) {
	chat := domain.Chat{ID: 1, UnreadCount: 3, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}}
	cm := components.NewChatContextMenu(chat, nil, false, keys.DefaultKeyMap())
	assert.Contains(t, cm.View(), "Mark as read")
}

func TestChatMenu_UnreadLabelForReadChat(t *testing.T) {
	chat := domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}}
	cm := components.NewChatContextMenu(chat, nil, false, keys.DefaultKeyMap())
	assert.Contains(t, cm.View(), "Mark as unread")
}

func TestChatMenu_MuteToggleLabel(t *testing.T) {
	muted := components.NewChatContextMenu(domain.Chat{ID: 1, IsMuted: true}, nil, false, keys.DefaultKeyMap())
	assert.Contains(t, muted.View(), "Unmute")

	unmuted := components.NewChatContextMenu(domain.Chat{ID: 1}, nil, false, keys.DefaultKeyMap())
	v := unmuted.View()
	assert.Contains(t, v, "Mute")
	assert.NotContains(t, v, "Unmute")
}

func TestChatMenu_EmitsToggleMuteRequest(t *testing.T) {
	chat := domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}}
	cm := components.NewChatContextMenu(chat, nil, false, keys.DefaultKeyMap())
	_, cmd := cm.Update(keyMsg('m')) // direct key -> Mute
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ToggleMuteRequest)
	require.True(t, ok)
	assert.True(t, req.Muted)
	assert.Equal(t, int64(1), req.Peer.ID)
}

func TestChatMenu_FolderSubmenuToggle(t *testing.T) {
	folders := []domain.FolderFilter{{ID: 7, Title: "Work", IncludePeers: []int64{1}}}
	chat := domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}}
	cm := components.NewChatContextMenu(chat, folders, false, keys.DefaultKeyMap())

	// open the folder submenu via direct key 'f'
	_, cmd := cm.Update(keyMsg('f'))
	require.Nil(t, cmd) // opening a submenu does not emit
	assert.Contains(t, cm.View(), "✓ Work")

	// cursor starts at the first folder; Enter toggles membership (remove).
	_, cmd = cm.Update(pressEnter())
	require.NotNil(t, cmd)
	req, ok := cmd().(components.AddToFolderRequest)
	require.True(t, ok)
	assert.Equal(t, 7, req.FilterID)
	assert.False(t, req.Add)
}

func TestChatMenu_ArchiveEntry(t *testing.T) {
	cm := components.NewChatContextMenu(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1}}, nil, false, keys.DefaultKeyMap())
	assert.Contains(t, cm.View(), "Archive")

	cmA := components.NewChatContextMenu(domain.Chat{ID: 1, IsArchived: true, Peer: domain.Peer{ID: 1}}, nil, false, keys.DefaultKeyMap())
	assert.Contains(t, cmA.View(), "Unarchive")
}

func TestChatMenu_EmitsToggleArchiveRequest(t *testing.T) {
	cm := components.NewChatContextMenu(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}}, nil, false, keys.DefaultKeyMap())
	_, cmd := cm.Update(keyMsg('a')) // direct key -> Archive
	require.NotNil(t, cmd)
	req, ok := cmd().(components.ToggleArchiveRequest)
	require.True(t, ok)
	assert.True(t, req.Archived)
}

// --- translation toggle ---

func TestChatMenu_OffersTranslateChatWhenOff(t *testing.T) {
	chat := domain.Chat{ID: 7, Title: "Bob", Peer: domain.Peer{ID: 7, Type: domain.PeerUser}}
	cm := components.NewChatContextMenu(chat, nil, false, defaultKM())
	view := strip(cm.View())
	assert.Contains(t, view, "Translate chat")
	assert.NotContains(t, view, "Show original")
}

func TestChatMenu_OffersShowOriginalWhenOn(t *testing.T) {
	chat := domain.Chat{ID: 7, Title: "Bob", Peer: domain.Peer{ID: 7, Type: domain.PeerUser}}
	cm := components.NewChatContextMenu(chat, nil, true, defaultKM())
	view := strip(cm.View())
	assert.Contains(t, view, "Show original")
	assert.NotContains(t, view, "Translate chat")
}

// Rows: Mark as unread(0) Mute(1) Translate chat(2) Archive(3) Profile(4).
func TestChatMenu_TranslateChatEnables(t *testing.T) {
	chat := domain.Chat{ID: 7, Title: "Bob", Peer: domain.Peer{ID: 7, Type: domain.PeerUser}}
	cm := components.NewChatContextMenu(chat, nil, false, defaultKM())
	cm, _ = cm.Update(pressJ())
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.TranslateChatRequest)
	require.True(t, ok)
	assert.Equal(t, int64(7), req.Peer.ID)
	assert.True(t, req.Enable)
}

func TestChatMenu_ShowOriginalDisables(t *testing.T) {
	chat := domain.Chat{ID: 7, Title: "Bob", Peer: domain.Peer{ID: 7, Type: domain.PeerUser}}
	cm := components.NewChatContextMenu(chat, nil, true, defaultKM())
	cm, _ = cm.Update(pressJ())
	cm, _ = cm.Update(pressJ())
	require.NotNil(t, cm)
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.TranslateChatRequest)
	require.True(t, ok)
	assert.Equal(t, int64(7), req.Peer.ID)
	assert.False(t, req.Enable)
}

// The folder sub-menu round trip must not lose the toggle's state either.
func TestChatMenu_TranslateChatSurvivesFolderSubMenu(t *testing.T) {
	folders := []domain.FolderFilter{{ID: 5, Title: "Work"}}
	chat := domain.Chat{ID: 7, Title: "Bob", Peer: domain.Peer{ID: 7, Type: domain.PeerUser}}
	cm := components.NewChatContextMenu(chat, folders, true, defaultKM())
	cm, _ = cm.Update(keyMsg('f')) // into the folder sub-menu
	require.NotNil(t, cm)
	cm, _ = cm.Update(pressEsc()) // back to main
	require.NotNil(t, cm)
	// Rows with folders: Mark as unread(0) Mute(1) Add to folder(2)
	// Translate/Show original(3) Archive(4) Profile(5).
	for range 3 {
		cm, _ = cm.Update(pressJ())
		require.NotNil(t, cm)
	}
	newCM, cmd := cm.Update(pressEnter())
	assert.Nil(t, newCM)
	require.NotNil(t, cmd)
	req, ok := cmd().(components.TranslateChatRequest)
	require.True(t, ok)
	assert.False(t, req.Enable, "the state read back is the translated one")
}
