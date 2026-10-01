package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/mediacache"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// mediaStub counts downloads and can fail the first attempts with an expired
// file reference, which is the case the refresh retry exists for.
type mediaStub struct {
	stubClient

	mu             sync.Mutex
	photoCalls     int
	docCalls       int
	thumbCalls     int
	payload        string
	staleUntilCall int // attempts up to and including this one report StaleReference
	refreshed      domain.Message
	refreshCalls   int
	delay          time.Duration
	lastPhotoSize  string
}

func (s *mediaStub) stream(dst io.Writer, n *int) error {
	s.mu.Lock()
	*n++
	attempt := *n
	payload := s.payload
	stale := attempt <= s.staleUntilCall
	delay := s.delay
	s.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	if stale {
		// Write a partial body first: a reference expires mid-download, and the
		// retry must truncate these bytes away rather than prepend them.
		_, _ = dst.Write([]byte("stale-partial-bytes"))
		return &telerr.Error{Kind: telerr.StaleReference}
	}
	_, err := dst.Write([]byte(payload))
	return err
}

func (s *mediaStub) DownloadPhotoToFile(_ context.Context, ref domain.PhotoRef, dst io.Writer) error {
	s.mu.Lock()
	s.lastPhotoSize = ref.ThumbSize
	s.mu.Unlock()
	return s.stream(dst, &s.photoCalls)
}

func (s *mediaStub) DownloadDocumentToFile(_ context.Context, _ domain.DocumentRef, dst io.Writer) error {
	return s.stream(dst, &s.docCalls)
}

func (s *mediaStub) DownloadDocumentThumbToFile(_ context.Context, _ domain.DocumentRef, dst io.Writer) error {
	return s.stream(dst, &s.thumbCalls)
}

func (s *mediaStub) RefreshMessage(context.Context, domain.Peer, int) (domain.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshCalls++
	return s.refreshed, nil
}

func (s *mediaStub) calls() (photo, doc, thumb, refresh int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.photoCalls, s.docCalls, s.thumbCalls, s.refreshCalls
}

// newMediaOwner builds an owner with a real cache in a temp dir and one chat
// holding one message carrying both a photo and a document.
func newMediaOwner(t *testing.T, c *mediaStub) (*Owner, string) {
	t.Helper()
	o, s := newOwnerWithClient(t, c)
	dir := t.TempDir()
	cache, err := mediacache.New(dir, 1<<20)
	require.NoError(t, err)
	o.SetMediaCache(cache)
	st := s.Store()
	st.SetChat(domain.Chat{ID: 1, Title: "Ada", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	st.SetMessages(1, []domain.Message{{
		ID: 5, ChatID: 1, Date: time.Unix(1, 0),
		Photo:    &domain.PhotoRef{ID: 9, ThumbSize: "m", FullThumbSize: "x", FileReference: []byte("stale")},
		Document: &domain.DocumentRef{ID: 11, ThumbSize: "m", FileName: "clip.mp4", MimeType: "video/mp4", FileReference: []byte("stale")},
		Media:    &domain.MediaRef{Kind: domain.MediaVideo},
	}})
	return o, dir
}

func TestFetchMedia_StreamsAPhotoThumbIntoTheCache(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, dir := newMediaOwner(t, c)

	path, err := o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "photo_9_m"), path)
	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "bytes", string(got))
	photo, _, _, _ := c.calls()
	assert.Equal(t, 1, photo)
}

func TestFetchMedia_SecondFetchIsServedFromTheCache(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, _ := newMediaOwner(t, c)
	_, err := o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)
	require.NoError(t, err)

	_, err = o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)

	require.NoError(t, err)
	photo, _, _, _ := c.calls()
	assert.Equal(t, 1, photo, "a cache hit must not reach Telegram")
}

func TestFetchMedia_UsesTheThumbnailLocationForADocumentPoster(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, dir := newMediaOwner(t, c)

	path, err := o.FetchMedia(context.Background(), 1, 5, domain.DocThumb, 0)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "doc_11_thumb_m"), path)
	_, doc, thumb, _ := c.calls()
	assert.Equal(t, 1, thumb)
	assert.Equal(t, 0, doc, "a poster must not stream the whole document")
}

func TestFetchMedia_UsesTheFullDocumentForASticker(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, dir := newMediaOwner(t, c)

	path, err := o.FetchMedia(context.Background(), 1, 5, domain.DocFull, 0)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "doc_11"), path)
	_, doc, _, _ := c.calls()
	assert.Equal(t, 1, doc)
}

// An expired reference is refreshed once, the download retried, and the fresh
// reference recorded so the next fetch does not repeat the round trip.
func TestFetchMedia_RefreshesAnExpiredReferenceAndRecordsIt(t *testing.T) {
	c := &mediaStub{
		payload:        "bytes",
		staleUntilCall: 1,
		refreshed: domain.Message{
			ID: 5, ChatID: 1,
			Photo: &domain.PhotoRef{ID: 9, ThumbSize: "m", FullThumbSize: "x", FileReference: []byte("fresh")},
		},
	}
	o, _ := newMediaOwner(t, c)

	path, err := o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)

	require.NoError(t, err)
	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "bytes", string(got), "the partial first attempt must be truncated away")
	photo, _, _, refresh := c.calls()
	assert.Equal(t, 2, photo, "one failed attempt and one retry")
	assert.Equal(t, 1, refresh)
	stored := o.state.Store().Messages(1)
	require.Len(t, stored, 1)
	assert.Equal(t, []byte("fresh"), stored[0].Photo.FileReference)
}

func TestFetchMedia_GivesUpAfterOneRefresh(t *testing.T) {
	c := &mediaStub{
		payload:        "bytes",
		staleUntilCall: 2,
		refreshed: domain.Message{
			ID: 5, ChatID: 1,
			Photo: &domain.PhotoRef{ID: 9, ThumbSize: "m", FileReference: []byte("fresh")},
		},
	}
	o, _ := newMediaOwner(t, c)

	_, err := o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)

	assert.Equal(t, telerr.StaleReference, telerr.Of(err))
	photo, _, _, _ := c.calls()
	assert.Equal(t, 2, photo, "one attempt, one retry, then stop")
}

// A reference that expires again straight after a refresh must not start a
// second refresh round. A client re-fetches on every repaint, and the refresh
// itself publishes a change that provokes the next repaint, so an unrefreshable
// photo would otherwise turn into an endless stream of round trips.
func TestFetchMedia_DoesNotRefreshTwiceInARow(t *testing.T) {
	c := &mediaStub{
		payload:        "bytes",
		staleUntilCall: 99,
		refreshed: domain.Message{
			ID: 5, ChatID: 1,
			Photo: &domain.PhotoRef{ID: 9, ThumbSize: "m", FileReference: []byte("fresh")},
		},
	}
	o, _ := newMediaOwner(t, c)
	_, err := o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)
	require.Error(t, err)

	_, err = o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)

	assert.Equal(t, telerr.StaleReference, telerr.Of(err))
	photo, _, _, refresh := c.calls()
	assert.Equal(t, 1, refresh, "the second fetch must not refresh again")
	assert.Equal(t, 3, photo, "one attempt and one retry, then a single attempt")
}

// The cooldown bounds a burst, not the day: media untouched for long enough is
// refreshed again, because by then the expiry is a new one.
func TestFetchMedia_RefreshesAgainAfterTheCooldown(t *testing.T) {
	c := &mediaStub{
		payload:        "bytes",
		staleUntilCall: 99,
		refreshed: domain.Message{
			ID: 5, ChatID: 1,
			Photo: &domain.PhotoRef{ID: 9, ThumbSize: "m", FileReference: []byte("fresh")},
		},
	}
	o, _ := newMediaOwner(t, c)
	_, err := o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)
	require.Error(t, err)

	prev := refreshCooldown
	refreshCooldown = 0
	t.Cleanup(func() { refreshCooldown = prev })
	_, err = o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)

	require.Error(t, err)
	_, _, _, refresh := c.calls()
	assert.Equal(t, 2, refresh)
}

func TestFetchMedia_MissingMediaIsNotFound(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, _ := newMediaOwner(t, c)
	o.state.Store().SetMessages(1, []domain.Message{{ID: 6, ChatID: 1, Date: time.Unix(1, 0)}})

	_, err := o.FetchMedia(context.Background(), 1, 6, domain.PhotoThumb, 0)

	assert.Equal(t, telerr.NotFound, telerr.Of(err))
}

// A rich message's photo lives inside its blocks rather than at the top
// level, and a message can carry more than one — the fetch must be addressed
// by the block's own photo id, resolving through RichBlocks rather than the
// message's (absent) top-level Photo (#274).
func TestFetchMedia_RichBlockPhoto_ResolvesThroughRichBlocksByID(t *testing.T) {
	c := &mediaStub{payload: "block-bytes"}
	o, dir := newMediaOwner(t, c)
	o.state.Store().SetMessages(1, []domain.Message{{
		ID: 20, ChatID: 1, Date: time.Unix(1, 0),
		RichBlocks: []domain.PageBlock{
			{
				Kind:  domain.BlockKindPhoto,
				Media: &domain.MediaRef{Kind: domain.MediaPhoto},
				Photo: &domain.PhotoRef{ID: 77, ThumbSize: "m"},
			},
		},
	}})

	path, err := o.FetchMedia(context.Background(), 1, 20, domain.PhotoThumb, 77)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "photo_77_m"), path)
	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "block-bytes", string(got))
}

// mediaID 0 must still mean "the message's own top-level media", not "the
// first rich block", or an ordinary message's photo would stop resolving the
// moment resolveMediaRef learned to look at RichBlocks too.
func TestFetchMedia_ZeroMediaIDIgnoresRichBlocks(t *testing.T) {
	c := &mediaStub{payload: "top-level-bytes"}
	o, _ := newMediaOwner(t, c)
	o.state.Store().SetMessages(1, []domain.Message{{
		ID: 21, ChatID: 1, Date: time.Unix(1, 0),
		Photo: &domain.PhotoRef{ID: 9, ThumbSize: "m"},
		RichBlocks: []domain.PageBlock{
			{Kind: domain.BlockKindPhoto, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 77, ThumbSize: "m"}},
		},
	}})

	path, err := o.FetchMedia(context.Background(), 1, 21, domain.PhotoThumb, 0)

	require.NoError(t, err)
	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "top-level-bytes", string(got))
	assert.Contains(t, path, "photo_9", "mediaID 0 must resolve the message's own photo, not a block's")
}

// An expired reference inside a rich block is refreshed the same way a
// top-level one is, but the fresh reference is recorded back into the
// matching block rather than the message's (absent) top-level Photo.
func TestFetchMedia_RichBlockPhoto_RefreshesAnExpiredReferenceIntoTheBlock(t *testing.T) {
	c := &mediaStub{
		payload:        "bytes",
		staleUntilCall: 1,
		refreshed: domain.Message{
			ID: 22, ChatID: 1,
			RichBlocks: []domain.PageBlock{
				{
					Kind:  domain.BlockKindPhoto,
					Media: &domain.MediaRef{Kind: domain.MediaPhoto},
					Photo: &domain.PhotoRef{ID: 77, ThumbSize: "m", FileReference: []byte("fresh")},
				},
			},
		},
	}
	o, _ := newMediaOwner(t, c)
	o.state.Store().SetMessages(1, []domain.Message{{
		ID: 22, ChatID: 1, Date: time.Unix(1, 0),
		RichBlocks: []domain.PageBlock{
			{
				Kind:  domain.BlockKindPhoto,
				Media: &domain.MediaRef{Kind: domain.MediaPhoto},
				Photo: &domain.PhotoRef{ID: 77, ThumbSize: "m", FileReference: []byte("stale")},
			},
		},
	}})

	_, err := o.FetchMedia(context.Background(), 1, 22, domain.PhotoThumb, 77)

	require.NoError(t, err)
	photo, _, _, refresh := c.calls()
	assert.Equal(t, 2, photo, "one failed attempt and one retry")
	assert.Equal(t, 1, refresh)
	stored := o.state.Store().Messages(1)
	require.Len(t, stored, 1)
	require.Len(t, stored[0].RichBlocks, 1)
	assert.Equal(t, []byte("fresh"), stored[0].RichBlocks[0].Photo.FileReference,
		"the refresh must patch the block's own photo, not a top-level field the message never had")
}

// Every repaint asks for the same thumbnails; without deduplication a slow
// download is started once per pass.
func TestFetchMedia_CollapsesConcurrentFetchesOfTheSameFile(t *testing.T) {
	c := &mediaStub{payload: "bytes", delay: 50 * time.Millisecond}
	o, _ := newMediaOwner(t, c)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	photo, _, _, _ := c.calls()
	assert.Equal(t, 1, photo)
}

func TestInvalidateMedia_DropsTheEntrySoTheNextFetchDownloadsAgain(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, _ := newMediaOwner(t, c)
	_, err := o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)
	require.NoError(t, err)

	o.InvalidateMedia(1, 5, domain.PhotoThumb, 0)
	_, err = o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)

	require.NoError(t, err)
	photo, _, _, _ := c.calls()
	assert.Equal(t, 2, photo)
}

func TestSaveMedia_WritesTheDocumentUnderItsOwnNameOutsideTheCache(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, cacheDir := newMediaOwner(t, c)
	dest := t.TempDir()

	path, err := o.SaveMedia(context.Background(), 1, 5, domain.DocFull, 0, dest)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dest, "clip.mp4"), path)
	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, "bytes", string(got))
	ents, derr := os.ReadDir(cacheDir)
	require.NoError(t, derr)
	assert.Empty(t, ents, "a save must not populate the cache")
}

func TestSaveMedia_ResolvesANameCollision(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, _ := newMediaOwner(t, c)
	dest := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dest, "clip.mp4"), []byte("existing"), 0600))

	path, err := o.SaveMedia(context.Background(), 1, 5, domain.DocFull, 0, dest)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dest, "clip (1).mp4"), path)
	got, rerr := os.ReadFile(filepath.Join(dest, "clip.mp4"))
	require.NoError(t, rerr)
	assert.Equal(t, "existing", string(got), "an existing file must not be overwritten")
}

func TestSaveMedia_NamesAPhotoFromItsID(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, _ := newMediaOwner(t, c)
	dest := t.TempDir()

	path, err := o.SaveMedia(context.Background(), 1, 5, domain.PhotoFull, 0, dest)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dest, "photo_9.jpg"), path)
}

// PhotoFull addresses the largest size, not the inline preview.
func TestSaveMedia_AsksForTheFullPhotoSize(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, _ := newMediaOwner(t, c)

	_, err := o.SaveMedia(context.Background(), 1, 5, domain.PhotoFull, 0, t.TempDir())

	require.NoError(t, err)
	c.mu.Lock()
	defer c.mu.Unlock()
	assert.Equal(t, "x", c.lastPhotoSize)
}

func TestFetchMedia_AsksForTheInlinePhotoSize(t *testing.T) {
	c := &mediaStub{payload: "bytes"}
	o, _ := newMediaOwner(t, c)

	_, err := o.FetchMedia(context.Background(), 1, 5, domain.PhotoThumb, 0)

	require.NoError(t, err)
	c.mu.Lock()
	defer c.mu.Unlock()
	assert.Equal(t, "m", c.lastPhotoSize)
}

func TestSaveMedia_RemovesThePartialFileWhenTheDownloadFails(t *testing.T) {
	c := &mediaStub{payload: "bytes", staleUntilCall: 99}
	o, _ := newMediaOwner(t, c)
	dest := t.TempDir()

	_, err := o.SaveMedia(context.Background(), 1, 5, domain.DocFull, 0, dest)

	require.Error(t, err)
	ents, derr := os.ReadDir(dest)
	require.NoError(t, derr)
	assert.Empty(t, ents, "a failed save must leave nothing behind")
}

func TestSavedFileName_UsesTheDocumentsOwnNameWhenPresent(t *testing.T) {
	r := mediaRef{slot: domain.DocFull, doc: domain.DocumentRef{ID: 7, FileName: "clip.mp4", MimeType: "video/mp4"}, kind: domain.MediaVideo}

	assert.Equal(t, "clip.mp4", savedFileName(r))
}

func TestSavedFileName_SynthesizesForUnnamedDocuments(t *testing.T) {
	cases := []struct {
		name string
		doc  domain.DocumentRef
		kind domain.MediaKind
		want string
	}{
		{"video", domain.DocumentRef{ID: 7, MimeType: "video/mp4"}, domain.MediaVideo, "video_7.mp4"},
		{"video note", domain.DocumentRef{ID: 7, MimeType: "video/mp4"}, domain.MediaVideoNote, "video_note_7.mp4"},
		{"voice", domain.DocumentRef{ID: 7, MimeType: "audio/ogg"}, domain.MediaVoice, "voice_7.oga"},
		{"audio", domain.DocumentRef{ID: 7, MimeType: "audio/mpeg"}, domain.MediaAudio, "audio_7.mp3"},
		{"gif", domain.DocumentRef{ID: 7, MimeType: "video/mp4"}, domain.MediaGIF, "gif_7.mp4"},
		{"file fallback", domain.DocumentRef{ID: 7, MimeType: "application/pdf"}, domain.MediaFile, "file_7.pdf"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := mediaRef{slot: domain.DocFull, doc: c.doc, kind: c.kind}
			assert.Equal(t, c.want, savedFileName(r))
		})
	}
}

func TestCreateUniqueFile_ResolvesCollision(t *testing.T) {
	dir := t.TempDir()

	f1, err := createUniqueFile(dir, "report.pdf")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "report.pdf"), f1.Name())
	require.NoError(t, f1.Close())

	f2, err := createUniqueFile(dir, "report.pdf")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "report (1).pdf"), f2.Name())
	require.NoError(t, f2.Close())
}

// A file name comes from Telegram, so it is untrusted: it must never escape the
// destination directory.
func TestCreateUniqueFile_SanitizesTheName(t *testing.T) {
	dir := t.TempDir()

	f, err := createUniqueFile(dir, "/etc/passwd")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "passwd"), f.Name())
	require.NoError(t, f.Close())
}

func TestCreateUniqueFile_EmptyNameFallsBack(t *testing.T) {
	dir := t.TempDir()

	f, err := createUniqueFile(dir, "")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "file"), f.Name())
	require.NoError(t, f.Close())
}

func TestExtFromMime(t *testing.T) {
	cases := []struct {
		mime string
		kind domain.MediaKind
		want string
	}{
		{"video/quicktime", domain.MediaVideo, ".mov"},
		{"video/webm", domain.MediaVideo, ".webm"},
		{"video/x-matroska", domain.MediaVideo, ".mkv"},
		{"video/mp4", domain.MediaVideo, ".mp4"},
		{"application/pdf", domain.MediaFile, ".pdf"},
		{"image/png", domain.MediaFile, ".png"},
		// An unknown type gets no extension, because a wrong one is worse than
		// none — unless it is a video, which an external player selects by
		// extension.
		{"application/weird", domain.MediaFile, ""},
		{"application/weird", domain.MediaVideo, ".mp4"},
		{"application/weird", domain.MediaGIF, ".mp4"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, extFromMime(c.mime, c.kind), "%s as %v", c.mime, c.kind)
	}
}
