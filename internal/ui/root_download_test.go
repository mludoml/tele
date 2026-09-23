package ui_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// downloadCompletionText runs a command (possibly batched) and returns the
// fileDownloadDoneMsg text it produces, if any.
func downloadCompletionText(cmd tea.Cmd) (string, bool) {
	if cmd == nil {
		return "", false
	}
	for _, msg := range drainMsgs(cmd()) {
		if text, _, ok := ui.FileDownloadDoneTextForTest(msg); ok {
			return text, true
		}
	}
	return "", false
}

// stageSavableMedia registers a file the owner will "save" for the given slot,
// under the name the real owner would have picked.
func stageSavableMedia(t *testing.T, o *testOwner, chatID int64, msgID int, slot domain.MediaSlot, name string) {
	t.Helper()
	src := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(src, []byte("payload"), 0600))
	o.mediaPaths[mediaPathKey{chatID, msgID, slot, 0}] = src
}

func TestDownloadKey_OnPhoto_SavesFullQualityJpg(t *testing.T) {
	dir := t.TempDir()
	defer ui.SetDownloadsDirForTest(dir)()

	m, st := newRootOnChat(t)
	o := m.Owner().(*testOwner)
	stageSavableMedia(t, o, 1, 10, domain.PhotoFull, "photo_321.jpg")

	photo := domain.Message{ID: 10, ChatID: 1, Date: time.Now(),
		Media: &domain.MediaRef{Kind: domain.MediaPhoto},
		Photo: &domain.PhotoRef{ID: 321, FullThumbSize: "y"}}
	st.AppendMessage(photo)
	nm, _ := applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)
	m.View() // lay out the message list so the photo becomes the selection

	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	text, ok := downloadCompletionText(cmd)
	require.True(t, ok, "pressing s on a photo must start a download")
	assert.Contains(t, text, filepath.Join(dir, "photo_321.jpg"))
}

func TestDownloadKey_OnVideo_SavesSynthesizedName(t *testing.T) {
	dir := t.TempDir()
	defer ui.SetDownloadsDirForTest(dir)()

	m, st := newRootOnChat(t)
	o := m.Owner().(*testOwner)
	stageSavableMedia(t, o, 1, 11, domain.DocFull, "video_654.mp4")

	video := domain.Message{ID: 11, ChatID: 1, Date: time.Now(),
		Media:    &domain.MediaRef{Kind: domain.MediaVideo},
		Document: &domain.DocumentRef{ID: 654, MimeType: "video/mp4"}}
	st.AppendMessage(video)
	nm, _ := applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)
	m.View()

	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	text, ok := downloadCompletionText(cmd)
	require.True(t, ok, "pressing s on a video must start a download")
	assert.Contains(t, text, filepath.Join(dir, "video_654.mp4"))
}

// The context-menu Download request must be routed (it was previously dropped in
// the root update switch).
func TestDownloadFileRequest_RoutedForPhoto(t *testing.T) {
	dir := t.TempDir()
	defer ui.SetDownloadsDirForTest(dir)()

	m, st := newRootOnChat(t)
	o := m.Owner().(*testOwner)
	stageSavableMedia(t, o, 1, 12, domain.PhotoFull, "photo_999.jpg")

	photo := domain.Message{ID: 12, ChatID: 1, Date: time.Now(),
		Media: &domain.MediaRef{Kind: domain.MediaPhoto},
		Photo: &domain.PhotoRef{ID: 999}}
	st.AppendMessage(photo)
	nm, _ := applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)
	m.View()

	_, cmd := m.Update(components.DownloadFileRequest{})
	text, ok := downloadCompletionText(cmd)
	require.True(t, ok, "DownloadFileRequest must be routed to a download")
	assert.Contains(t, text, filepath.Join(dir, "photo_999.jpg"))
}

// Saving and opening media moved behind the owner in #196. Streaming to disk,
// the truncate-and-retry on an expired file reference, naming, collisions and
// partial-file cleanup are covered by TestSaveMedia_* and TestFetchMedia_* in
// internal/core; the client's half — reporting the saved path, reporting a
// failure, launching the file — by TestSaveFileCmd_* and TestOpenDocumentCmd_*
// in the in-package tests. The disk cache's hit/miss behaviour is covered by
// TestFetchMedia_* as well.
