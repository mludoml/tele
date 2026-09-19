package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/mediacache"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// refreshCooldown is how long a refreshed file reference is trusted. A second
// expiry inside this window means refreshing did not help, so the next fetch
// fails instead of asking Telegram again. A variable so tests can shorten it.
var refreshCooldown = time.Minute

// mediaFetcher downloads media on behalf of clients. It is the only place a
// file reference is handled: a client names a message slot, this resolves the
// reference, streams the file to disk, refreshes an expired reference once, and
// answers with a path. Decoding stays with the client, because decoding serves
// rendering (#196).
type mediaFetcher struct {
	client internaltg.Client
	state  *state.State
	log    *zap.Logger

	// cache is nil until the app supplies one. A fetch without a cache has
	// nowhere to put the file and fails rather than inventing a location.
	cache *mediacache.Cache

	// inflight collapses concurrent fetches of one key into a single download.
	// Every repaint asks for the same thumbnails, so without this a slow
	// download starts again on each pass.
	inflight singleflight.Group

	// refreshedAt remembers when each key's file reference was last refreshed,
	// so a reference that keeps expiring costs one round trip rather than one
	// per repaint. Entries older than the cooldown are dropped as they are
	// passed, which keeps the map the size of what is currently failing.
	refreshMu sync.Mutex
	refreshed map[string]time.Time
}

func newMediaFetcher(client internaltg.Client, st *state.State, log *zap.Logger) *mediaFetcher {
	return &mediaFetcher{client: client, state: st, log: log, refreshed: make(map[string]time.Time)}
}

// claimRefresh reports whether key may be refreshed now, and records the
// attempt when it may. A refresh inside the cooldown is refused: the reference
// it produced expired again, so another one buys nothing.
func (f *mediaFetcher) claimRefresh(key string) bool {
	f.refreshMu.Lock()
	defer f.refreshMu.Unlock()
	now := time.Now()
	for k, at := range f.refreshed {
		if now.Sub(at) >= refreshCooldown {
			delete(f.refreshed, k)
		}
	}
	if at, ok := f.refreshed[key]; ok && now.Sub(at) < refreshCooldown {
		return false
	}
	f.refreshed[key] = now
	return true
}

// mediaRef is a resolved location: exactly one of photo or doc is meaningful,
// picked by slot.
type mediaRef struct {
	slot  domain.MediaSlot
	photo domain.PhotoRef
	doc   domain.DocumentRef
	kind  domain.MediaKind
}

// findRichMedia searches a rich message's block tree for the photo or
// document named by mediaID, recursing into container blocks (collage,
// slideshow, details, list items, blockquote) the way a bot composes a
// gallery. ok is false when no block names this id: the id is stale, or the
// message was edited and no longer carries it.
func findRichMedia(blocks []domain.PageBlock, mediaID int64) (photo *domain.PhotoRef, doc *domain.DocumentRef, kind domain.MediaKind, ok bool) {
	for _, b := range blocks {
		switch {
		case b.Photo != nil && b.Photo.ID == mediaID:
			k := domain.MediaPhoto
			if b.Media != nil {
				k = b.Media.Kind
			}
			return b.Photo, nil, k, true
		case b.Document != nil && b.Document.ID == mediaID:
			var k domain.MediaKind
			if b.Media != nil {
				k = b.Media.Kind
			}
			return nil, b.Document, k, true
		}
		if photo, doc, kind, ok := findRichMedia(b.Children, mediaID); ok {
			return photo, doc, kind, ok
		}
	}
	return nil, nil, 0, false
}

// resolveMediaRef turns a stored message and a slot into a file location. With
// mediaID zero it reads the message's own top-level photo or document, as
// every message not composed of rich blocks does. A nonzero mediaID instead
// names one photo or document living inside the message's rich blocks — a
// rich message may carry several (a collage, a gallery of listings), so the
// slot alone cannot tell them apart the way it can for an ordinary message. A
// slot the resolved media has nothing for is NotFound: the client asked for
// something that is not there, which is a mistake, not an empty answer.
func resolveMediaRef(msg domain.Message, slot domain.MediaSlot, mediaID int64) (mediaRef, error) {
	photo, doc := msg.Photo, msg.Document
	var kind domain.MediaKind
	if msg.Media != nil {
		kind = msg.Media.Kind
	}
	if mediaID != 0 {
		var ok bool
		photo, doc, kind, ok = findRichMedia(msg.RichBlocks, mediaID)
		if !ok {
			return mediaRef{}, &telerr.Error{Kind: telerr.NotFound, Op: "fetch media", Detail: "no rich block names this media id"}
		}
	}
	switch slot {
	case domain.PhotoThumb:
		if photo == nil {
			return mediaRef{}, &telerr.Error{Kind: telerr.NotFound, Op: "fetch media", Detail: "message has no photo"}
		}
		return mediaRef{slot: slot, photo: *photo, kind: kind}, nil
	case domain.PhotoFull:
		if photo == nil {
			return mediaRef{}, &telerr.Error{Kind: telerr.NotFound, Op: "fetch media", Detail: "message has no photo"}
		}
		r := *photo
		if r.FullThumbSize != "" {
			r.ThumbSize = r.FullThumbSize
		}
		return mediaRef{slot: slot, photo: r, kind: kind}, nil
	case domain.DocThumb:
		if doc == nil || doc.ThumbSize == "" {
			return mediaRef{}, &telerr.Error{Kind: telerr.NotFound, Op: "fetch media", Detail: "message has no document thumbnail"}
		}
		return mediaRef{slot: slot, doc: *doc, kind: kind}, nil
	case domain.DocFull:
		if doc == nil {
			return mediaRef{}, &telerr.Error{Kind: telerr.NotFound, Op: "fetch media", Detail: "message has no document"}
		}
		return mediaRef{slot: slot, doc: *doc, kind: kind}, nil
	}
	return mediaRef{}, &telerr.Error{Kind: telerr.NotFound, Op: "fetch media", Detail: "unknown media slot"}
}

// cacheKey names the file in the cache. Filename-safe by construction: every
// part is a number or a Telegram size letter.
func (r mediaRef) cacheKey() string {
	switch r.slot {
	case domain.PhotoThumb, domain.PhotoFull:
		if r.photo.ThumbSize == "" {
			return "photo_" + strconv.FormatInt(r.photo.ID, 10)
		}
		return "photo_" + strconv.FormatInt(r.photo.ID, 10) + "_" + r.photo.ThumbSize
	case domain.DocThumb:
		return "doc_" + strconv.FormatInt(r.doc.ID, 10) + "_thumb_" + r.doc.ThumbSize
	}
	return "doc_" + strconv.FormatInt(r.doc.ID, 10)
}

// stream writes the located file into dst.
func (f *mediaFetcher) stream(ctx context.Context, r mediaRef, dst io.Writer) error {
	switch r.slot {
	case domain.PhotoThumb, domain.PhotoFull:
		return f.client.DownloadPhotoToFile(ctx, r.photo, dst)
	case domain.DocThumb:
		return f.client.DownloadDocumentThumbToFile(ctx, r.doc, dst)
	}
	return f.client.DownloadDocumentToFile(ctx, r.doc, dst)
}

// streamInto writes the media named by (chatID, msgID, slot) into file. On an
// expired file reference it refreshes the message once, records the fresh
// reference in state, rewinds the file and retries. The rewind matters: a
// partial first attempt would otherwise be prefixed to the retry's bytes.
func (f *mediaFetcher) streamInto(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot, mediaID int64, file *os.File) error {
	msg, err := messageByID(f.state, chatID, msgID)
	if err != nil {
		return err
	}
	ref, err := resolveMediaRef(msg, slot, mediaID)
	if err != nil {
		return err
	}

	err = f.attempt(ctx, ref, file)
	if telerr.Of(err) != telerr.StaleReference {
		return err
	}

	// Nothing below is visible on screen: a preview that fails to download is
	// drawn as nothing at all. Every way out of the recovery therefore says so,
	// at a level the default log keeps.
	where := []zap.Field{
		zap.Int64("chat_id", chatID), zap.Int("msg_id", msgID), zap.String("slot", slot.String()),
	}
	if !f.claimRefresh(ref.cacheKey()) {
		f.log.Warn("media: reference expired again within the refresh cooldown, giving up",
			append(where, zap.String("key", ref.cacheKey()))...)
		return err
	}

	chat, ok := f.state.Store().GetChat(chatID)
	if !ok {
		return &telerr.Error{Kind: telerr.PeerNotFound}
	}
	fresh, rerr := f.client.RefreshMessage(ctx, chat.Peer, msgID)
	if rerr != nil {
		f.log.Warn("media: could not refresh an expired file reference",
			append(where, zap.Error(rerr))...)
		return err // the original expiry is the more useful error
	}
	if mediaID != 0 {
		if freshPhoto, freshDoc, _, ok := findRichMedia(fresh.RichBlocks, mediaID); ok {
			f.state.ApplyRichMediaRef(chatID, msgID, mediaID, freshPhoto, freshDoc)
		}
	} else {
		f.state.ApplyMediaRef(chatID, msgID, fresh.Photo, fresh.Document)
	}
	f.log.Debug("media: refreshed an expired file reference", where...)

	freshRef, err := resolveMediaRef(fresh, slot, mediaID)
	if err != nil {
		f.log.Warn("media: the refreshed message no longer carries this media",
			append(where, zap.Error(err))...)
		return err
	}
	if err := f.attempt(ctx, freshRef, file); err != nil {
		f.log.Warn("media: download failed after refreshing the file reference",
			append(where, zap.Error(err))...)
		return err
	}
	return nil
}

// attempt rewinds file and streams one download into it.
func (f *mediaFetcher) attempt(ctx context.Context, ref mediaRef, file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	return f.stream(ctx, ref, file)
}

// Fetch returns a path to the media, downloading it into the cache on a miss.
//
// A fetch crosses four layers (client -> owner -> cache -> tg) and shows
// nothing at all when it fails, because the client renders no placeholder for
// media it does not have. Each step therefore says what it did, so "the preview
// did not appear" can be traced to the layer that gave up (#196).
func (f *mediaFetcher) Fetch(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot, mediaID int64) (string, error) {
	if f.cache == nil {
		return "", &telerr.Error{Kind: telerr.Internal, Op: "fetch media", Detail: "no media cache"}
	}
	key, err := f.key(chatID, msgID, slot, mediaID)
	if err != nil {
		f.log.Debug("media: fetch could not resolve the reference",
			zap.Int64("chat_id", chatID), zap.Int("msg_id", msgID),
			zap.String("slot", slot.String()), zap.Error(err))
		return "", err
	}
	if p, ok := f.cache.Path(key); ok {
		f.log.Debug("media: fetch served from the cache", zap.String("key", key))
		return p, nil
	}
	path, err, _ := f.inflight.Do(key, func() (any, error) {
		// Another caller may have finished while this one waited.
		if p, ok := f.cache.Path(key); ok {
			return p, nil
		}
		f.log.Debug("media: fetch downloading",
			zap.String("key", key), zap.String("slot", slot.String()))
		return f.cache.Put(key, func(file *os.File) error {
			return f.streamInto(ctx, chatID, msgID, slot, mediaID, file)
		})
	})
	if err != nil {
		f.log.Debug("media: fetch failed", zap.String("key", key), zap.Error(err))
		return "", err
	}
	f.log.Debug("media: fetch done", zap.String("key", key), zap.String("path", path.(string)))
	return path.(string), nil
}

// Save streams the media into destDir under a name derived from the media
// itself, bypassing the cache: what the user saved must not be evicted, and a
// large file must not push thumbnails out of a budget sized for thumbnails.
func (f *mediaFetcher) Save(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot, mediaID int64, destDir string) (string, error) {
	msg, err := messageByID(f.state, chatID, msgID)
	if err != nil {
		return "", err
	}
	ref, err := resolveMediaRef(msg, slot, mediaID)
	if err != nil {
		return "", err
	}
	file, err := createUniqueFile(destDir, savedFileName(ref))
	if err != nil {
		return "", err
	}
	name := file.Name()
	if err := f.streamInto(ctx, chatID, msgID, slot, mediaID, file); err != nil {
		_ = file.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// Invalidate drops a cached entry. A client calls it when the bytes turn out to
// be undecodable, so the next fetch downloads the file again instead of
// returning the same broken entry forever.
func (f *mediaFetcher) Invalidate(chatID int64, msgID int, slot domain.MediaSlot, mediaID int64) {
	if f.cache == nil {
		return
	}
	if key, err := f.key(chatID, msgID, slot, mediaID); err == nil {
		// The client could not decode these bytes. That is invisible on screen —
		// the media simply never appears — so say it here.
		f.log.Debug("media: dropped an entry the client could not decode", zap.String("key", key))
		f.cache.Remove(key)
	}
}

func (f *mediaFetcher) key(chatID int64, msgID int, slot domain.MediaSlot, mediaID int64) (string, error) {
	msg, err := messageByID(f.state, chatID, msgID)
	if err != nil {
		return "", err
	}
	ref, err := resolveMediaRef(msg, slot, mediaID)
	if err != nil {
		return "", err
	}
	return ref.cacheKey(), nil
}

// savedFileName is the on-disk name for saved media: a document's own name when
// it has one, otherwise "<prefix>_<id><ext>" with the prefix reflecting the
// media kind and the extension derived from the MIME type. Telegram photos,
// voice messages and round notes often carry no file name.
func savedFileName(r mediaRef) string {
	if r.slot == domain.PhotoThumb || r.slot == domain.PhotoFull {
		return "photo_" + strconv.FormatInt(r.photo.ID, 10) + ".jpg"
	}
	if r.doc.FileName != "" {
		return r.doc.FileName
	}
	prefix := "file"
	switch r.kind {
	case domain.MediaVideo:
		prefix = "video"
	case domain.MediaVideoNote:
		prefix = "video_note"
	case domain.MediaVoice:
		prefix = "voice"
	case domain.MediaAudio:
		prefix = "audio"
	case domain.MediaGIF:
		prefix = "gif"
	}
	return prefix + "_" + strconv.FormatInt(r.doc.ID, 10) + extFromMime(r.doc.MimeType, r.kind)
}

// extFromMime derives a file extension from a MIME type via the mimetype
// library rather than a hand-rolled table, so the OS opens saved media in the
// right application. An unknown type yields no extension, because a wrong one
// is worse than none — except for video, which is opened in an external player
// that picks the application by extension, and where .mp4 is the usual Telegram
// container.
func extFromMime(mimeType string, kind domain.MediaKind) string {
	if mt := mimetype.Lookup(mimeType); mt != nil {
		return mt.Extension()
	}
	if kind.IsVideo() || kind == domain.MediaGIF {
		return ".mp4"
	}
	return ""
}

// createUniqueFile creates a new file in dir under name's base name, resolving
// collisions as "name (1).ext", "name (2).ext" and so on. It uses O_EXCL so the
// name is claimed atomically: no overwrite, no TOCTOU race. The caller owns the
// returned file and must Close it.
func createUniqueFile(dir, name string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	base := filepath.Base(filepath.Clean(name))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "file"
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 0; ; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		f, err := os.OpenFile(filepath.Join(dir, candidate), os.O_CREATE|os.O_RDWR|os.O_EXCL, 0644)
		if err == nil {
			return f, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
	}
}
