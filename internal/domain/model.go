// Package domain holds the account's data model: the types a view renders and
// a command addresses. It is pure data — no persistence, no gotd — so a client
// can hold it without holding the store.
package domain

import (
	"strconv"
	"strings"
	"time"
)

type PeerType int

const (
	PeerUser PeerType = iota
	PeerGroup
	PeerChannel
	PeerSuperGroup
)

type Peer struct {
	ID         int64
	Type       PeerType
	AccessHash int64
}

func (p Peer) IsUser() bool       { return p.Type == PeerUser }
func (p Peer) IsGroup() bool      { return p.Type == PeerGroup || p.Type == PeerSuperGroup }
func (p Peer) IsChannel() bool    { return p.Type == PeerChannel }
func (p Peer) IsSuperGroup() bool { return p.Type == PeerSuperGroup }

type Reaction struct {
	Emoji    string
	Count    int
	IsChosen bool
}

type MessageEntity struct {
	Type   string // "bold", "italic", "code", "pre", "strike", "underline", "text_url", "url", "email", "phone", "bank_card", "mention", "mention_name", "hashtag", "cashtag", "bot_command" — UTF-16 offsets (Telegram encoding)
	Offset int
	Length int
	URL    string // for "text_url": the hidden target URL; empty otherwise
	// Language is the info string of a fenced code block ("```go"). Set only for
	// Type=="pre", and only on the send side for now (#152); the receive side
	// does not populate it yet.
	Language string
	// UserID/AccessHash are set only for Type=="mention_name" (name-based
	// mention of a user without a public username).
	UserID     int64
	AccessHash int64
}

// ChatMember is a group/channel participant offered by the @mention autocomplete.
//
// It is a membership rather than a lesser User: the same person is a member of
// many chats or of none, and a role in one chat belongs here rather than on the
// person (#222).
type ChatMember struct {
	UserID      int64
	Username    string // without leading '@'; empty if the user has no public username
	DisplayName string // First + Last, trimmed
	AccessHash  int64
}

// User is a person the account has an address for. It holds facts about the
// person and none about the conversation with them: a mute, an unread count and
// a draft belong to the Chat.
//
// A User may be partial. What the owner knows locally arrives first and the
// rest follows from users.getFullUser, so an empty Bio means "not known" as
// readily as "not set" — the caller that cares tracks which (#222).
type User struct {
	ID        int64
	Username  string // without leading '@'; empty if the user has no public username
	FirstName string
	LastName  string
	// Bio is the user's "about" text. Only ever set from the full response.
	Bio string
	// Phone is set only when Telegram's privacy settings return one, which is
	// mostly for mutual contacts. Empty otherwise.
	Phone string
	// Online is the coarse presence flag the dialog list already carries. It is
	// a boolean because that is all the client reads today; the full range of
	// last-seen states is #127.
	Online          bool
	IsBot           bool
	IsContact       bool
	IsMutualContact bool
	IsDeleted       bool
	// AvatarID names the person's current avatar, and is the whole of what makes
	// a cached avatar stale: a person who changes their picture gets a new id,
	// so nothing has to notice the change for the old file to stop being asked
	// for. Zero means there is no avatar to fetch — a person who set none and a
	// person whose privacy settings withhold it are the same answer here, and
	// both are drawn as a monogram (#223).
	//
	// Only ever set from the full response, like Bio: the dialog list carries no
	// avatar.
	AvatarID int64
}

// DisplayName is the name to draw for a user: First + Last, falling back to the
// id when the person has neither, which is what a deleted account looks like.
func (u User) DisplayName() string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		return "User " + strconv.FormatInt(u.ID, 10)
	}
	return name
}

type PhotoRef struct {
	ID            int64
	AccessHash    int64
	FileReference []byte
	DCID          int
	ThumbSize     string // inline: "m" (320px) or best available
	FullThumbSize string // full quality: best large size ("x"→800px, "y"→1280px, "w"→2560px)
	// Width/Height are the pixel dimensions of the size named by ThumbSize, as
	// Telegram reports them — known from the message's own metadata, before any
	// bytes are downloaded. The UI uses them to reserve the photo's exact
	// on-screen aspect the moment the chat opens, instead of guessing a generic
	// box that resizes once the thumbnail decodes. Zero on a message persisted
	// before this field existed; callers fall back to a generic box then.
	Width, Height int
}

// DocumentRef is the download-capable reference for document-backed media
// (video, round video, voice, audio, sticker, gif, file). The full file is
// fetched via DownloadDocument; ThumbSize (when present) names a PhotoSize in
// the document's thumbnail set for an inline preview.
type DocumentRef struct {
	ID            int64
	AccessHash    int64
	FileReference []byte
	DCID          int
	ThumbSize     string // best thumbnail PhotoSize type, "" if no thumbnail
	MimeType      string
	FileName      string
	Size          int64
}

// MediaKind classifies the media a message carries, for display purposes.
type MediaKind int

const (
	MediaPhoto MediaKind = iota
	MediaVideo
	MediaVideoNote // round video message (кружок)
	MediaVoice
	MediaAudio
	MediaSticker
	MediaGIF
	MediaFile
	MediaLocation
	MediaOther // generic fallback (contact, poll, dice, …)

	// MediaKindCount is not a kind. It bounds the enum so the canvas scan can
	// walk every one of them, which is what keeps a kind added above this line
	// from shipping a placeholder nobody ever rendered under a painted theme
	// (#227). Keep it last.
	MediaKindCount
)

// IsVideo reports whether the kind is a playable video (regular or round note),
// both of which render an inline thumbnail preview and open in an external player.
func (k MediaKind) IsVideo() bool {
	return k == MediaVideo || k == MediaVideoNote
}

// IsStaticSticker reports whether the media is a sticker whose document is a
// static WEBP image (renderable inline), as opposed to an animated .tgs or
// video .webm sticker.
func IsStaticSticker(m *MediaRef, d *DocumentRef) bool {
	return m != nil && m.Kind == MediaSticker &&
		d != nil && d.MimeType == "image/webp"
}

// MediaRef is the display-level description of a message's media. PhotoRef
// remains the download-capable reference and is set only for photos.
type MediaRef struct {
	Kind  MediaKind
	Emoji string // sticker's alt emoji; populated now for stickers
	// Audio/voice metadata.
	Duration  int    // seconds, for video/voice/audio/note
	Waveform  []byte // bitpacked 5-bit amplitude samples, for voice messages
	Title     string // song title, for audio
	Performer string // performer, for audio
	// File metadata (from the document), populated for document-backed media.
	FileName string // original file name
	Size     int64  // bytes
	// Width/Height are the video's/GIF's pixel dimensions as Telegram reports
	// them (DocumentAttributeVideo), known before any bytes download. See
	// PhotoRef.Width/Height for why the UI wants this upfront.
	Width, Height int
}

// BlockKind classifies one block of a rich message (Telegram's richMessage /
// rich page document). The set follows the MTProto `PageBlock` family, with the
// shapes that render the same folded together: the six heading constructors are
// one kind carrying a Level, and the two blockquote constructors are one kind
// carrying either text or nested blocks. Anything with no meaningful terminal
// rendering is BlockKindUnsupported, which is drawn as a named placeholder
// rather than dropped.
type BlockKind int

const (
	BlockKindParagraph BlockKind = iota
	// BlockKindHeading carries Level 1..6, largest first.
	BlockKindHeading
	BlockKindPreformatted
	BlockKindFooter
	BlockKindDivider
	// BlockKindAnchor is a link target, not content: it renders as nothing.
	BlockKindAnchor
	BlockKindKicker
	// BlockKindTitle/Subtitle/Header/Subheader are the legacy Instant View
	// masthead and heading constructors; they render as their nearest modern
	// equivalent (see docs/rich-messages.md).
	BlockKindTitle
	BlockKindSubtitle
	BlockKindAuthorDate
	BlockKindBlockquote
	BlockKindPullquote
	BlockKindList
	BlockKindOrderedList
	// BlockKindListItem is not a PageBlock constructor: it is the item of a
	// BlockKindList/BlockKindOrderedList, kept as a child block so the item may
	// itself hold a paragraph, code or nested list.
	BlockKindListItem
	BlockKindTable
	BlockKindDetails
	BlockKindCollage
	BlockKindSlideshow
	BlockKindMap
	BlockKindPhoto
	BlockKindVideo
	BlockKindAudio
	// BlockKindMath carries the raw expression: a terminal has no LaTeX engine.
	BlockKindMath
	// BlockKindThinking is the streaming draft placeholder. It only ever arrives
	// on the ephemeral draft path.
	BlockKindThinking
	BlockKindUnsupported

	// BlockKindCount is not a kind. It bounds the enum so a scan can walk every
	// one of them. Keep it last.
	BlockKindCount
)

// String names the kind as the Bot API names it, for logs, tests and the docs
// coverage table. Unknown values are "unknown".
func (k BlockKind) String() string {
	switch k {
	case BlockKindParagraph:
		return "paragraph"
	case BlockKindHeading:
		return "heading"
	case BlockKindPreformatted:
		return "pre"
	case BlockKindFooter:
		return "footer"
	case BlockKindDivider:
		return "divider"
	case BlockKindAnchor:
		return "anchor"
	case BlockKindKicker:
		return "kicker"
	case BlockKindTitle:
		return "title"
	case BlockKindSubtitle:
		return "subtitle"
	case BlockKindAuthorDate:
		return "author_date"
	case BlockKindBlockquote:
		return "blockquote"
	case BlockKindPullquote:
		return "pullquote"
	case BlockKindList:
		return "list"
	case BlockKindOrderedList:
		return "ordered_list"
	case BlockKindListItem:
		return "list_item"
	case BlockKindTable:
		return "table"
	case BlockKindDetails:
		return "details"
	case BlockKindCollage:
		return "collage"
	case BlockKindSlideshow:
		return "slideshow"
	case BlockKindMap:
		return "map"
	case BlockKindPhoto:
		return "photo"
	case BlockKindVideo:
		return "video"
	case BlockKindAudio:
		return "audio"
	case BlockKindMath:
		return "mathematical_expression"
	case BlockKindThinking:
		return "thinking"
	case BlockKindUnsupported:
		return "unsupported"
	default:
		return "unknown"
	}
}

// RichText is a run of text with the inline entities that style it. It is the
// rich-message counterpart of a plain Message's Text+Entities pair, and carries
// the same UTF-16 offsets so the same renderer can paint it.
type RichText struct {
	Text     string
	Entities []MessageEntity
}

// TableBlock is a rich-message table. Rows are positional and already expanded
// by the server into a grid. A cell's Colspan is merged visually — it is drawn
// as wide as the columns it covers — while Rowspan is carried for fidelity and
// not laid out: a cell taller than its row would make the grid a different shape
// for every terminal width, and the row it belongs to already sizes to its
// tallest cell. See docs/rich-messages.md.
type TableBlock struct {
	Title    *RichText
	Bordered bool
	Striped  bool
	Rows     [][]TableCell
}

// TableCell is one cell of a TableBlock. Align is "left", "center" or "right";
// VAlign is "top", "middle" or "bottom". Both are applied to the drawn cell.
// IsHeader marks the cells a terminal draws bold, with a rule under the header
// row. Colspan and Rowspan are 0 or 1 when the cell does not span.
type TableCell struct {
	Text     RichText
	IsHeader bool
	Colspan  int
	Rowspan  int
	Align    string
	VAlign   string
}

// MapBlock is a rich-message map. A terminal cannot draw tiles, so the block is
// rendered from these numbers plus its caption and an "open in browser" button
// (see docs/rich-messages.md).
type MapBlock struct {
	Lat  float64
	Long float64
	Zoom int
}

// PageBlock is one node of a rich message's block tree. Which fields are set
// follows from Kind; the alternative is one struct per kind, which would make
// the tree untypeable in a domain that must stay serialisable as plain JSON.
type PageBlock struct {
	Kind BlockKind
	// Text is the block's inline content: paragraph, heading, pre, footer,
	// kicker, title, subtitle, blockquote, pullquote, the summary of a details
	// block, the author of an author_date block, and the raw expression of a
	// math block.
	Text *RichText
	// Level is the heading size, 1 (largest) to 6, for BlockKindHeading.
	Level int
	// Start is an ordered list's first number, for BlockKindOrderedList. Zero
	// means the list starts at one, which is what Telegram omits.
	Start int
	// Marker is the label style of an ordered list ("1", "a", "A", "i", "I"),
	// for BlockKindOrderedList. Empty when the server named none.
	Marker string
	// Children are the nested blocks: list items, the body of a details and
	// blockquote-blocks block, the parts of a collage/slideshow, and the one
	// block a cover wraps.
	Children []PageBlock
	// Table, Media and Map are the payloads of their respective kinds.
	Table *TableBlock
	Media *MediaRef
	// Photo and Document are the downloadable files of a media block, so the
	// block can be drawn by the same media path a photo message uses.
	Photo    *PhotoRef
	Document *DocumentRef
	// Caption and Credit are the block's caption and its `<cite>`-style credit.
	Caption *RichText
	Credit  *RichText
	Map     *MapBlock
	// DetailsOpen is the server's default expansion state for a details block.
	// The UI keeps the *current* state separately (see docs/rich-messages.md):
	// expanding a section is a view state, not a fact about the message.
	DetailsOpen bool
	// Language labels a preformatted block ("go", "json"); empty when unnamed.
	Language string
	// Label carries the one piece of metadata a kind has that has no inline
	// form: the name of an anchor, the server-rendered marker of an ordered list
	// item, or the reason an unsupported block was not drawn.
	Label string
	// Date is the publication date of an author_date block, unix seconds; zero
	// when absent.
	Date int
	// Checkbox and Checked are a list item's checkbox state.
	Checkbox bool
	Checked  bool
}

// ButtonStyle is a button's emphasis, as Telegram asks for it. The zero value is
// the app's neutral default.
type ButtonStyle int

const (
	ButtonStyleDefault ButtonStyle = iota
	ButtonStylePrimary
	ButtonStyleSuccess
	ButtonStyleDanger
)

// String names the style as the Bot API names it.
func (s ButtonStyle) String() string {
	switch s {
	case ButtonStylePrimary:
		return "primary"
	case ButtonStyleSuccess:
		return "success"
	case ButtonStyleDanger:
		return "danger"
	default:
		return "default"
	}
}

// ButtonActionKind says what pressing an inline button does.
type ButtonActionKind int

const (
	// ButtonActionNone is a button with no action we can carry out. It renders
	// as a visibly disabled button; Reason says what it asked for.
	ButtonActionNone ButtonActionKind = iota
	ButtonActionCallback
	ButtonActionURL
)

// ButtonAction is what a button does when pressed. It is one struct with a Kind
// rather than a union of types because a message's buttons are stored as JSON:
// an interface field would marshal fine and unmarshal into nothing, losing every
// button on restart.
type ButtonAction struct {
	Kind ButtonActionKind
	// Data is the callback payload, sent back to the bot verbatim.
	Data []byte
	// URL is the target of a link button.
	URL string
	// Reason names the action a button asked for that this client cannot carry
	// out (a phone number, a location, an in-app web view, a payment …). It is
	// why the button is shown disabled rather than silently dropped.
	Reason string
}

// KeyboardButton is one inline button under a message.
type KeyboardButton struct {
	Text   string
	Style  ButtonStyle
	Action ButtonAction
}

// ReplyMarkup is the row structure of a message's inline keyboard. Rows are
// kept as rows because the bot chose the grouping, and re-flowing them would
// change what the screen means.
type ReplyMarkup struct {
	Rows [][]KeyboardButton
}

// Buttons returns the markup's buttons in reading order, rows flattened.
func (r *ReplyMarkup) Buttons() []KeyboardButton {
	if r == nil {
		return nil
	}
	var out []KeyboardButton
	for _, row := range r.Rows {
		out = append(out, row...)
	}
	return out
}

// Count returns how many buttons the markup holds.
func (r *ReplyMarkup) Count() int {
	if r == nil {
		return 0
	}
	n := 0
	for _, row := range r.Rows {
		n += len(row)
	}
	return n
}

// EphemeralDraft is a rich message a bot is still writing: the partial document
// it streams into a chat before it commits to sending one. It is not a message —
// Telegram expires it after 30 seconds of silence, it is never stored, and the
// finished article arrives later as an ordinary message.
//
// It is deliberately not a Message. Nothing about a draft is addressable: it has
// an id only so an update can replace the right one, it has no reactions, no
// replies and no place in the read pointer, and giving it a Message's shape
// would invite every message-shaped code path to try.
type EphemeralDraft struct {
	ChatID int64
	// ID identifies one draft stream. Updates with the same id replace each other.
	ID int
	// Date is when this revision was written, unix seconds.
	Date time.Time
	// Text is the flattened rendering of the partial document, and TextBlocks the
	// document itself when the bot streamed one.
	Text       string
	TextBlocks []PageBlock
	// Placeholder marks the "thinking" stage, where the bot has said it is
	// working and produced nothing yet.
	Placeholder bool
}

// CallbackAnswer is what a bot answered when one of its inline buttons was
// pressed. It is the reply to one press, not state about the message: it is
// shown once and forgotten, which is why it is neither stored nor a projection.
type CallbackAnswer struct {
	// Alert asks for something the reader has to dismiss rather than a line
	// that ages out on its own. Telegram's own clients draw a dialog for it;
	// this one shows the same text for longer (see docs/rich-messages.md).
	Alert bool
	// Message is the text of the answer, empty when the bot answered silently.
	Message string
	// URL is the target of a "press to open" answer, empty when there is none.
	// A non-empty URL is a press that turned into a navigation.
	URL string
}

type Chat struct {
	ID              int64
	Title           string
	Peer            Peer
	Pinned          bool
	UnreadCount     int
	ReadInboxMaxID  int
	ReadOutboxMaxID int
	LastMessage     *Message
	// TopMessageID is the id of the newest message the server has for this
	// chat, as of the last dialog list. It is where the history ends on
	// Telegram's side, which is what a repair compares its progress against;
	// LastMessage is the preview of that message and carries no id of its own.
	//
	// Held in memory only. Every connection reloads the dialog list before
	// anything reads this, so a value from disk would always be overwritten
	// before it was used, and a second place to be wrong about where the server
	// is buys nothing.
	TopMessageID int
	IsContact    bool
	IsBot        bool
	IsMuted      bool
	Online       bool
	// UnreadMark is the Telegram dialog `unread_mark` flag: a manual
	// "mark as unread" that is independent of UnreadCount.
	UnreadMark bool
	// UnreadReactionsCount is the Telegram dialog `unread_reactions_count`:
	// reactions on your messages that you have not yet viewed. Persisted like
	// UnreadCount; cleared by messages.readReactions when the chat is opened.
	UnreadReactionsCount int
	// UnreadMentionsCount is the Telegram dialog `unread_mentions_count`:
	// messages that mention you (or reply to you) that you have not yet viewed.
	// Persisted like UnreadCount; cleared by messages.readMentions on open.
	UnreadMentionsCount int
	// IsArchived reports whether the chat lives in the built-in Archive
	// folder (folder_id 1).
	IsArchived bool
	// Draft is the unsent message draft synced with Telegram (#62). It is
	// loaded from the dialog list and kept current via updateDraftMessage; it
	// is not persisted to disk (the server is the source of truth on restart).
	Draft string
}

type FolderFilter struct {
	ID    int // Telegram filter ID; 0 = "All Chats" sentinel
	Title string
	Emoji string

	PinnedPeers  []int64
	IncludePeers []int64
	ExcludePeers []int64

	// Category flags
	Contacts    bool
	NonContacts bool
	Groups      bool
	Broadcasts  bool
	Bots        bool

	// Exclusion flags
	ExcludeMuted    bool
	ExcludeRead     bool
	ExcludeArchived bool
}

type Message struct {
	ID         int
	ChatID     int64
	SenderID   int64
	SenderName string
	Text       string
	Date       time.Time
	IsOut      bool
	Entities   []MessageEntity
	Media      *MediaRef    // nil if message has no media
	Photo      *PhotoRef    // nil if message has no photo
	Document   *DocumentRef // nil if message has no document-backed media
	// GroupedID is Telegram's album key: album parts share the same non-zero
	// grouped_id. 0 means the message is not part of an album.
	GroupedID    int64
	ReplyToMsgID int        // 0 if not a reply
	EditDate     *time.Time // nil if not edited
	// EditHidden is Telegram's edit_hide: the message must be shown as
	// unmodified even though it carries an edit date. It is a statement about
	// the label alone and never about the content, which may well have changed
	// under it - a bot streaming a reply rewrites one message this way (#269).
	EditHidden bool
	// AppliedPosition is how far through the account's events this copy of the
	// message has been brought. On a copy that arrives it is the position of the
	// update carrying it; on a stored copy it is the last one applied, which is
	// what makes a late copy recognisable as late. Zero means the source carries
	// no position at all - a fetched history page, or our own edit before the
	// server answers - and orders against nothing (ADR 0016).
	AppliedPosition int
	Reactions       []Reaction
	// HasUnreadReactions is true when the raw message carried at least one recent
	// reaction flagged unread (a not-yet-viewed reaction on one of our messages).
	HasUnreadReactions bool
	// Mentioned is the raw message `mentioned` flag: the message mentions us
	// (@username, or a reply to one of our messages). Drives the chat-list
	// unread-mention indicator when the message is incoming and unread (#155).
	Mentioned bool
	Forward   *ForwardInfo // nil if not forwarded
	// RichBlocks is the block document a bot sent in place of plain text
	// (Telegram's richMessage). nil when the message carries no rich content;
	// when it is set, Text holds the server's flattened rendering of the same
	// document and is what a client that draws no blocks shows.
	RichBlocks []PageBlock
	// ReplyMarkup is the message's inline keyboard. nil when it has none.
	// Carried as rows because the bot chose the grouping, and only reachable on
	// a bot's message: it is what a press is addressed to.
	ReplyMarkup *ReplyMarkup
	// LocalMedia describes the files of a queued media send. Set only on the
	// bubble a client draws for an outbox entry; nil for real messages.
	LocalMedia *LocalMedia
}

// ShowsEdited reports whether the message carries the "edited" mark. Being
// edited and being labelled as edited are two different facts: Telegram hides
// the label on a reaction bump and on a bot rewriting its own message, and asks
// every client to hide it the same way.
func (m Message) ShowsEdited() bool { return m.EditDate != nil && !m.EditHidden }

// LocalMedia describes the files of a queued media send so the pending bubble
// can name them and show upload progress.
//
// It is built from the outbox entry when the bubble is drawn and never stored:
// what is in flight lives in the queue, not in a client's memory, so every
// attached client sees the same send rather than only the one that started it
// (#195).
type LocalMedia struct {
	Kind MediaKind
	// FileName names a lone file. Empty for a group, which is named by its count.
	FileName string
	Size     int64
	// Part is the 1-based file being uploaded now, Parts the total in the send.
	// Part is zero until the upload starts.
	Part  int
	Parts int
	// UploadProgress is the fraction uploaded (0..1) across the whole send.
	UploadProgress float64
}

// ForwardInfo describes the origin of a forwarded message.
type ForwardInfo struct {
	From string // display name of the original sender; empty if hidden
}

// MessageTranslation is one message's text in another language, as Telegram
// returned it. It is a query result and nothing more: translation is display
// state, so a translated body is never written back onto the Message, into the
// store, or to Telegram. What is rendered is chosen at render time from the
// message and this.
type MessageTranslation struct {
	MessageID int
	Text      string
	Entities  []MessageEntity
}

type TypingAction int

const (
	TypingActionUnknown TypingAction = iota
	TypingActionTyping
	TypingActionRecordAudio
	TypingActionUploadAudio
	TypingActionRecordVideo
	TypingActionUploadVideo
	TypingActionUploadPhoto
	TypingActionUploadDocument
	TypingActionChooseSticker
	TypingActionRecordRound
	TypingActionCancel
)

func (a TypingAction) Label() string {
	switch a {
	case TypingActionTyping:
		return "typing"
	case TypingActionRecordAudio:
		return "recording audio"
	case TypingActionUploadAudio:
		return "sending audio"
	case TypingActionRecordVideo:
		return "recording video"
	case TypingActionUploadVideo:
		return "sending video"
	case TypingActionUploadPhoto:
		return "sending a photo"
	case TypingActionUploadDocument:
		return "sending a file"
	case TypingActionChooseSticker:
		return "choosing a sticker"
	case TypingActionRecordRound:
		return "recording a video message"
	default:
		return ""
	}
}
