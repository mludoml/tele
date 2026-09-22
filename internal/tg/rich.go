package tg

import (
	"time"
	"unicode/utf16"

	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// richFileRefs holds the files a rich message declared alongside its blocks.
// A rich block names its media by id (pageBlockPhoto{photo_id:…}) and the
// message carries the matching Photo/Document in separate lists, so the two
// halves are joined here before the tree is built. Without the join every media
// block would be a caption with no picture.
type richFileRefs struct {
	photos map[int64]*domain.PhotoRef
	docs   map[int64]*domain.DocumentRef
	media  map[int64]*domain.MediaRef
}

// newRichFileRefs indexes a rich message's declared files by id, classifying
// each document the same way a document carried by a message is classified:
// the block says which group it is (a video block names a video), and the
// document's own attributes say what its player needs (duration, title).
func newRichFileRefs(photos []tg.PhotoClass, documents []tg.DocumentClass) richFileRefs {
	refs := richFileRefs{
		photos: make(map[int64]*domain.PhotoRef, len(photos)),
		docs:   make(map[int64]*domain.DocumentRef, len(documents)),
		media:  make(map[int64]*domain.MediaRef, len(documents)),
	}
	for _, p := range photos {
		photo, ok := p.(*tg.Photo)
		if !ok {
			continue
		}
		thumb := pickThumbSize(photo.Sizes)
		if thumb == "" {
			continue
		}
		w, h := photoSizeDims(photo.Sizes, thumb)
		refs.photos[photo.ID] = &domain.PhotoRef{
			ID:            photo.ID,
			AccessHash:    photo.AccessHash,
			FileReference: photo.FileReference,
			DCID:          photo.DCID,
			ThumbSize:     thumb,
			FullThumbSize: pickFullThumbSize(photo.Sizes),
			Width:         w,
			Height:        h,
		}
	}
	for _, d := range documents {
		doc, ok := d.(*tg.Document)
		if !ok {
			continue
		}
		ref := buildDocumentRef(doc)
		refs.docs[doc.ID] = ref
		refs.media[doc.ID] = classifyDocument(&tg.MessageMediaDocument{Document: doc})
		if ref.FileName != "" {
			// The bubble names a file by its name, and the document ref is where
			// the name lives for the media path.
			refs.media[doc.ID].FileName = ref.FileName
		}
		refs.media[doc.ID].Size = doc.Size
	}
	return refs
}

// mediaFor resolves the media a photo or document block names. The block's own
// kind is the fallback: a document with no attributes is still the video block
// that named it, not a plain file. A block whose file the message did not
// declare reports false, and the renderer falls back to the caption alone
// rather than dropping the text.
func (r richFileRefs) mediaFor(kind domain.MediaKind, id int64) (*domain.MediaRef, *domain.PhotoRef, *domain.DocumentRef, bool) {
	if kind == domain.MediaPhoto {
		photo, ok := r.photos[id]
		if !ok {
			return nil, nil, nil, false
		}
		return &domain.MediaRef{Kind: domain.MediaPhoto}, photo, nil, true
	}
	doc, ok := r.docs[id]
	if !ok {
		return nil, nil, nil, false
	}
	media := r.media[id]
	if media == nil || media.Kind == domain.MediaFile || media.Kind == domain.MediaOther {
		// Nothing in the file said what it is; the block that named it did.
		media = &domain.MediaRef{Kind: kind, Size: doc.Size, FileName: doc.FileName}
	}
	return media, nil, doc, true
}

// convertRichBlocks turns the raw block tree of a rich message into the domain
// tree. Unsupported constructors become BlockKindUnsupported carrying their TL
// type name, which is what the renderer prints: a block nobody can draw is
// still a fact about the message, and dropping it silently would shorten what
// the sender wrote.
func convertRichBlocks(blocks []tg.PageBlockClass, refs richFileRefs) []domain.PageBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]domain.PageBlock, 0, len(blocks))
	for _, raw := range blocks {
		if b, ok := convertRichBlock(raw, refs); ok {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func convertRichBlock(raw tg.PageBlockClass, refs richFileRefs) (domain.PageBlock, bool) {
	switch b := raw.(type) {
	case *tg.PageBlockParagraph:
		return domain.PageBlock{Kind: domain.BlockKindParagraph, Text: convertRichText(b.Text)}, true
	case *tg.PageBlockHeading1:
		return headingBlock(b.Text, 1), true
	case *tg.PageBlockHeading2:
		return headingBlock(b.Text, 2), true
	case *tg.PageBlockHeading3:
		return headingBlock(b.Text, 3), true
	case *tg.PageBlockHeading4:
		return headingBlock(b.Text, 4), true
	case *tg.PageBlockHeading5:
		return headingBlock(b.Text, 5), true
	case *tg.PageBlockHeading6:
		return headingBlock(b.Text, 6), true
	case *tg.PageBlockPreformatted:
		return domain.PageBlock{
			Kind:     domain.BlockKindPreformatted,
			Text:     convertRichText(b.Text),
			Language: b.Language,
		}, true
	case *tg.PageBlockFooter:
		return domain.PageBlock{Kind: domain.BlockKindFooter, Text: convertRichText(b.Text)}, true
	case *tg.PageBlockDivider:
		return domain.PageBlock{Kind: domain.BlockKindDivider}, true
	case *tg.PageBlockAnchor:
		// An anchor is a link target rather than content: the domain keeps the
		// name so the tree round-trips, and the renderer draws nothing for it.
		return domain.PageBlock{Kind: domain.BlockKindAnchor, Label: b.Name}, true
	case *tg.PageBlockKicker:
		return domain.PageBlock{Kind: domain.BlockKindKicker, Text: convertRichText(b.Text)}, true
	case *tg.PageBlockTitle:
		return domain.PageBlock{Kind: domain.BlockKindTitle, Text: convertRichText(b.Text)}, true
	case *tg.PageBlockSubtitle:
		return domain.PageBlock{Kind: domain.BlockKindSubtitle, Text: convertRichText(b.Text)}, true
	case *tg.PageBlockHeader:
		// The legacy masthead heading; drawn as a top-level heading would be.
		return headingBlock(b.Text, 2), true
	case *tg.PageBlockSubheader:
		return headingBlock(b.Text, 3), true
	case *tg.PageBlockAuthorDate:
		author := convertRichText(b.Author)
		return domain.PageBlock{
			Kind:  domain.BlockKindAuthorDate,
			Text:  author,
			Date:  b.PublishedDate,
			Label: "", // the date field carries it; Label stays for the anchor case
		}, true
	case *tg.PageBlockBlockquote:
		return domain.PageBlock{
			Kind:   domain.BlockKindBlockquote,
			Text:   convertRichText(b.Text),
			Credit: convertRichText(b.Caption),
		}, true
	case *tg.PageBlockBlockquoteBlocks:
		return domain.PageBlock{
			Kind:     domain.BlockKindBlockquote,
			Children: convertRichBlocks(b.Blocks, refs),
			Credit:   convertRichText(b.Caption),
		}, true
	case *tg.PageBlockPullquote:
		return domain.PageBlock{
			Kind:   domain.BlockKindPullquote,
			Text:   convertRichText(b.Text),
			Credit: convertRichText(b.Caption),
		}, true
	case *tg.PageBlockList:
		items := make([]domain.PageBlock, 0, len(b.Items))
		for _, item := range b.Items {
			if it, ok := convertListItem(item, refs); ok {
				items = append(items, it)
			}
		}
		return domain.PageBlock{Kind: domain.BlockKindList, Children: items}, true
	case *tg.PageBlockOrderedList:
		items := make([]domain.PageBlock, 0, len(b.Items))
		for _, item := range b.Items {
			if it, ok := convertOrderedListItem(item, refs); ok {
				items = append(items, it)
			}
		}
		return domain.PageBlock{
			Kind:     domain.BlockKindOrderedList,
			Start:    b.Start,
			Marker:   b.Type,
			Children: items,
		}, true
	case *tg.PageBlockTable:
		return convertTable(b), true
	case *tg.PageBlockDetails:
		return domain.PageBlock{
			Kind:        domain.BlockKindDetails,
			Text:        convertRichText(b.Title),
			Children:    convertRichBlocks(b.Blocks, refs),
			DetailsOpen: b.Open,
		}, true
	case *tg.PageBlockCollage:
		return domain.PageBlock{
			Kind:     domain.BlockKindCollage,
			Children: convertRichBlocks(b.Items, refs),
			Caption:  convertPageCaption(b.Caption),
			Credit:   convertPageCredit(b.Caption),
		}, true
	case *tg.PageBlockSlideshow:
		return domain.PageBlock{
			Kind:     domain.BlockKindSlideshow,
			Children: convertRichBlocks(b.Items, refs),
			Caption:  convertPageCaption(b.Caption),
			Credit:   convertPageCredit(b.Caption),
		}, true
	case *tg.PageBlockCover:
		// A cover wraps one block; dropping the wrapper's own identity keeps the
		// rendering honest — what is inside IS the cover.
		inner, ok := convertRichBlock(b.Cover, refs)
		if !ok {
			return domain.PageBlock{}, false
		}
		return inner, true
	case *tg.PageBlockPhoto:
		block := domain.PageBlock{
			Kind:    domain.BlockKindPhoto,
			Caption: convertPageCaption(b.Caption),
			Credit:  convertPageCredit(b.Caption),
		}
		if media, photo, doc, ok := refs.mediaFor(domain.MediaPhoto, b.PhotoID); ok {
			block.Media, block.Photo, block.Document = media, photo, doc
		}
		return block, true
	case *tg.PageBlockVideo:
		block := domain.PageBlock{
			Kind:    domain.BlockKindVideo,
			Caption: convertPageCaption(b.Caption),
			Credit:  convertPageCredit(b.Caption),
		}
		applyRichMedia(&block, refs, domain.MediaVideo, b.VideoID)
		return block, true
	case *tg.PageBlockAudio:
		block := domain.PageBlock{
			Kind:    domain.BlockKindAudio,
			Caption: convertPageCaption(b.Caption),
			Credit:  convertPageCredit(b.Caption),
		}
		applyRichMedia(&block, refs, domain.MediaAudio, b.AudioID)
		return block, true
	case *tg.PageBlockMap:
		block := domain.PageBlock{
			Kind:    domain.BlockKindMap,
			Caption: convertPageCaption(b.Caption),
			Credit:  convertPageCredit(b.Caption),
		}
		if geo, ok := b.Geo.(*tg.GeoPoint); ok {
			block.Map = &domain.MapBlock{Lat: geo.Lat, Long: geo.Long, Zoom: b.Zoom}
		}
		return block, true
	case *tg.PageBlockMath:
		return domain.PageBlock{Kind: domain.BlockKindMath, Text: &domain.RichText{Text: b.Source}}, true
	case *tg.PageBlockThinking:
		return domain.PageBlock{Kind: domain.BlockKindThinking, Text: convertRichText(b.Text)}, true
	case *tg.PageBlockUnsupported:
		return domain.PageBlock{Kind: domain.BlockKindUnsupported, Label: raw.TypeName()}, true
	default:
		// Constructors this client has no rendering for still name themselves,
		// so the placeholder can say what arrived rather than that something did.
		return domain.PageBlock{Kind: domain.BlockKindUnsupported, Label: raw.TypeName()}, true
	}
}

// applyRichMedia attaches a resolved file to a media block, leaving the block
// caption-only when the declared file is missing.
func applyRichMedia(block *domain.PageBlock, refs richFileRefs, kind domain.MediaKind, id int64) {
	if media, photo, doc, ok := refs.mediaFor(kind, id); ok {
		block.Media, block.Photo, block.Document = media, photo, doc
	}
}

func headingBlock(text tg.RichTextClass, level int) domain.PageBlock {
	return domain.PageBlock{
		Kind:  domain.BlockKindHeading,
		Level: level,
		Text:  convertRichText(text),
	}
}

// convertListItem turns one unordered list item into a BlockKindListItem. The
// item's content is either inline text or a block tree, and both are kept in the
// same shape so the renderer has one case to handle.
func convertListItem(raw tg.PageListItemClass, refs richFileRefs) (domain.PageBlock, bool) {
	switch item := raw.(type) {
	case *tg.PageListItemText:
		return domain.PageBlock{
			Kind:     domain.BlockKindListItem,
			Text:     convertRichText(item.Text),
			Checkbox: item.Checkbox,
			Checked:  item.Checked,
		}, true
	case *tg.PageListItemBlocks:
		return domain.PageBlock{
			Kind:     domain.BlockKindListItem,
			Children: convertRichBlocks(item.Blocks, refs),
			Checkbox: item.Checkbox,
			Checked:  item.Checked,
		}, true
	default:
		return domain.PageBlock{Kind: domain.BlockKindUnsupported, Label: raw.TypeName()}, true
	}
}

// convertOrderedListItem is convertListItem plus the item's own label: the
// server renders the marker ("3.", "c.") and hands it over as Num, which is the
// only way an ordered list survives a reversed or non-decimal numbering.
func convertOrderedListItem(raw tg.PageListOrderedItemClass, refs richFileRefs) (domain.PageBlock, bool) {
	switch item := raw.(type) {
	case *tg.PageListOrderedItemText:
		return domain.PageBlock{
			Kind:     domain.BlockKindListItem,
			Text:     convertRichText(item.Text),
			Checkbox: item.Checkbox,
			Checked:  item.Checked,
			Label:    item.Num,
			Marker:   item.Type,
		}, true
	case *tg.PageListOrderedItemBlocks:
		return domain.PageBlock{
			Kind:     domain.BlockKindListItem,
			Children: convertRichBlocks(item.Blocks, refs),
			Checkbox: item.Checkbox,
			Checked:  item.Checked,
			Label:    item.Num,
			Marker:   item.Type,
		}, true
	default:
		return domain.PageBlock{Kind: domain.BlockKindUnsupported, Label: raw.TypeName()}, true
	}
}

// richTextValue is convertRichText with the empty case folded to the zero
// value, for the places that hold a RichText rather than a pointer to one.
func richTextValue(raw tg.RichTextClass) domain.RichText {
	if rt := convertRichText(raw); rt != nil {
		return *rt
	}
	return domain.RichText{}
}

// convertReplyMarkup maps a message's inline keyboard onto the domain's rows.
//
// Only ReplyInlineMarkup is a rich message's buttons. The other variants —
// ReplyKeyboardMarkup, ReplyKeyboardForceReply, ReplyKeyboardHide — are the
// custom keyboard a bot replaces the phone's own with, which this client does
// not render, so they report nil rather than an empty markup that would claim
// the message has a keyboard.
func convertReplyMarkup(raw tg.ReplyMarkupClass) *domain.ReplyMarkup {
	inline, ok := raw.(*tg.ReplyInlineMarkup)
	if !ok {
		return nil
	}
	rows := make([][]domain.KeyboardButton, 0, len(inline.Rows))
	for _, rawRow := range inline.Rows {
		row := make([]domain.KeyboardButton, 0, len(rawRow.Buttons))
		for _, rawBtn := range rawRow.Buttons {
			if btn, ok := convertKeyboardButton(rawBtn); ok {
				row = append(row, btn)
			}
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return &domain.ReplyMarkup{Rows: rows}
}

// convertKeyboardButton maps one button. A button always keeps its text: an
// action this client cannot carry out is a disabled button carrying the reason,
// never a button that vanished from the keyboard and left a gap in the row.
func convertKeyboardButton(raw tg.KeyboardButtonClass) (domain.KeyboardButton, bool) {
	if raw == nil {
		return domain.KeyboardButton{}, false
	}
	btn := domain.KeyboardButton{
		Text:  raw.GetText(),
		Style: convertButtonStyle(raw),
	}
	switch b := raw.(type) {
	case *tg.KeyboardButtonCallback:
		btn.Action = domain.ButtonAction{
			Kind: domain.ButtonActionCallback,
			Data: b.Data,
		}
	case *tg.KeyboardButtonURL:
		btn.Action = domain.ButtonAction{
			Kind: domain.ButtonActionURL,
			URL:  b.URL,
		}
	default:
		btn.Action = domain.ButtonAction{
			Kind:   domain.ButtonActionNone,
			Reason: unsupportedButtonReason(raw),
		}
	}
	return btn, true
}

// unsupportedButtonReason names, in words, what a button asked for that this
// client cannot do. The TL type name would be accurate and useless: a reader
// looking at a disabled button wants to know it wanted their phone number, not
// that it was a keyboardButtonRequestPhone.
func unsupportedButtonReason(raw tg.KeyboardButtonClass) string {
	switch raw.(type) {
	case *tg.KeyboardButtonRequestPhone:
		return "asks for your phone number"
	case *tg.KeyboardButtonRequestGeoLocation:
		return "asks for your location"
	case *tg.KeyboardButtonRequestPoll:
		return "asks you to create a poll"
	case *tg.KeyboardButtonGame:
		return "opens a game"
	case *tg.KeyboardButtonBuy:
		return "opens a payment"
	case *tg.KeyboardButtonSwitchInline:
		return "switches to inline mode"
	case *tg.KeyboardButtonWebView, *tg.KeyboardButtonSimpleWebView:
		return "opens a web app"
	case *tg.KeyboardButtonURLAuth:
		return "signs in through the bot"
	case *tg.KeyboardButtonRequestPeer:
		return "asks you to pick a chat"
	case *tg.KeyboardButtonUserProfile:
		return "opens a profile"
	case *tg.KeyboardButtonCopy:
		return "copies text"
	default:
		return raw.TypeName()
	}
}

// convertButtonStyle maps Telegram's mutually-exclusive emphasis flags onto the
// one style a button has. The order is the priority Telegram gives them; a
// button with none of the flags is the app's neutral default.
func convertButtonStyle(raw tg.KeyboardButtonClass) domain.ButtonStyle {
	style, ok := raw.GetStyle()
	if !ok {
		return domain.ButtonStyleDefault
	}
	switch {
	case style.BgPrimary:
		return domain.ButtonStylePrimary
	case style.BgSuccess:
		return domain.ButtonStyleSuccess
	case style.BgDanger:
		return domain.ButtonStyleDanger
	default:
		return domain.ButtonStyleDefault
	}
}

// convertPageCaption maps a page caption onto the domain's caption text.
func convertPageCaption(c tg.PageCaption) *domain.RichText {
	return convertRichText(c.Text)
}

// convertPageCredit maps a page caption's credit line, the `<cite>` of a photo
// or a quoted block.
func convertPageCredit(c tg.PageCaption) *domain.RichText {
	return convertRichText(c.Credit)
}

func convertTable(b *tg.PageBlockTable) domain.PageBlock {
	rows := make([][]domain.TableCell, 0, len(b.Rows))
	for _, rawRow := range b.Rows {
		row := make([]domain.TableCell, 0, len(rawRow.Cells))
		for _, c := range rawRow.Cells {
			row = append(row, convertTableCell(c))
		}
		rows = append(rows, row)
	}
	return domain.PageBlock{
		Kind: domain.BlockKindTable,
		Table: &domain.TableBlock{
			Title:    convertRichText(b.Title),
			Bordered: b.Bordered,
			Striped:  b.Striped,
			Rows:     rows,
		},
	}
}

// convertTableCell flattens Telegram's three alignment booleans into the string
// the Bot API documents, so the renderer has one value to switch on. A cell with
// neither flag set is left-aligned, which is what the flags mean.
func convertTableCell(c tg.PageTableCell) domain.TableCell {
	align := "left"
	switch {
	case c.AlignCenter:
		align = "center"
	case c.AlignRight:
		align = "right"
	}
	valign := "top"
	switch {
	case c.ValignMiddle:
		valign = "middle"
	case c.ValignBottom:
		valign = "bottom"
	}
	return domain.TableCell{
		Text:     richTextValue(c.Text),
		IsHeader: c.Header,
		Colspan:  c.Colspan,
		Rowspan:  c.Rowspan,
		Align:    align,
		VAlign:   valign,
	}
}

// convertRichText maps a RichText tree onto one string plus entities. Nesting
// and concatenation are flattened here: the renderer styles runs, and it has no
// use for the tree once the styles are known.
func convertRichText(raw tg.RichTextClass) *domain.RichText {
	if raw == nil {
		return nil
	}
	var b richTextBuilder
	b.write(raw)
	if b.text == "" && len(b.entities) == 0 {
		return nil
	}
	return &domain.RichText{Text: b.text, Entities: b.entities}
}

// richTextBuilder accumulates a RichText tree into flat text with UTF-16
// entities, the same encoding a plain message's entities use.
type richTextBuilder struct {
	text     string
	entities []domain.MessageEntity
}

func (b *richTextBuilder) write(raw tg.RichTextClass) {
	switch t := raw.(type) {
	case nil:
		return
	case *tg.TextEmpty:
		return
	case *tg.TextPlain:
		b.text += t.Text
	case *tg.TextConcat:
		for _, part := range t.Texts {
			b.write(part)
		}
	case *tg.TextBold:
		b.styled(t.Text, "bold")
	case *tg.TextItalic:
		b.styled(t.Text, "italic")
	case *tg.TextUnderline:
		b.styled(t.Text, "underline")
	case *tg.TextStrike:
		b.styled(t.Text, "strike")
	case *tg.TextSpoiler:
		// The renderer has no spoiler style; the text is shown as written rather
		// than hidden, which is the same thing every other unknown entity does.
		b.write(t.Text)
	case *tg.TextFixed:
		b.styled(t.Text, "code")
	case *tg.TextSubscript, *tg.TextSuperscript:
		// No terminal attribute for either; the run keeps its text and loses the
		// distinction, deliberately (see docs/rich-messages.md).
		b.write(richTextInner(raw))
	case *tg.TextMarked:
		b.write(t.Text)
	case *tg.TextURL:
		b.linked(t.Text, "text_url", t.URL)
	case *tg.TextEmail:
		b.linked(t.Text, "email", t.Email)
	case *tg.TextPhone:
		b.linked(t.Text, "phone", t.Phone)
	case *tg.TextMention:
		b.write(t.Text)
		b.mark(len(b.text), 0, "mention", "")
	case *tg.TextHashtag:
		b.write(t.Text)
		b.mark(len(b.text), 0, "hashtag", "")
	case *tg.TextCashtag:
		b.write(t.Text)
		b.mark(len(b.text), 0, "cashtag", "")
	case *tg.TextBotCommand:
		b.write(t.Text)
		b.mark(len(b.text), 0, "bot_command", "")
	case *tg.TextAutoURL, *tg.TextAutoEmail, *tg.TextAutoPhone, *tg.TextBankCard:
		// Server-detected entities: the text is all the tree carries, and the
		// plain-message renderer detects the same shapes from the text itself.
		b.write(richTextInner(raw))
	case *tg.TextMentionName:
		b.linked(t.Text, "mention_name", "")
		if n := len(b.entities); n > 0 {
			b.entities[n-1].UserID = t.UserID
			b.entities[n-1].URL = ""
		}
	case *tg.TextAnchor:
		// An anchor link's target is a name inside this same document; the
		// terminal has no in-document scrolling, so the text is kept unstyled.
		b.write(t.Text)
	case *tg.TextMath:
		// Inline math arrives as its source: no LaTeX engine here, so the
		// expression is shown verbatim (see docs/rich-messages.md).
		b.text += t.Source
	case *tg.TextCustomEmoji:
		b.text += t.Alt
	case *tg.TextImage:
		// An inline image has no inline rendering: name it where it stood so the
		// paragraph does not silently lose a picture.
		b.text += "[image]"
	case *tg.TextDate, *tg.TextDiff:
		b.write(richTextInner(raw))
	default:
		b.write(richTextInner(raw))
	}
}

// richTextInner returns the Text field of a constructor that is otherwise
// unrenderable, so an unrecognised style loses its styling but not its words.
func richTextInner(raw tg.RichTextClass) tg.RichTextClass {
	type hasText interface {
		GetText() (tg.RichTextClass, bool)
	}
	if getter, ok := raw.(hasText); ok {
		if inner, ok := getter.GetText(); ok {
			return inner
		}
	}
	return nil
}

// styled appends the inner text and marks the run it added.
func (b *richTextBuilder) styled(raw tg.RichTextClass, typ string) {
	start := len(b.text)
	b.write(raw)
	b.mark(start, len(b.text)-start, typ, "")
}

// linked appends the inner text and marks it as a link with an explicit target.
func (b *richTextBuilder) linked(raw tg.RichTextClass, typ, target string) {
	start := len(b.text)
	b.write(raw)
	b.mark(start, len(b.text)-start, typ, target)
}

// utf16Len is the length of s in UTF-16 code units, which is the encoding
// Telegram counts entity offsets in.
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// mark records an entity over the byte range [start, start+n) of the accumulated
// text, converting that range to UTF-16 offsets — which is what the entity
// renderer expects, and the reason the offset cannot be counted in bytes.
func (b *richTextBuilder) mark(start, n int, typ, url string) {
	if n <= 0 {
		return
	}
	prefix := b.text[:start]
	run := b.text[start : start+n]
	b.entities = append(b.entities, domain.MessageEntity{
		Type:   typ,
		Offset: utf16Len(prefix),
		Length: utf16Len(run),
		URL:    url,
	})
}

// convertEphemeralDraft turns one streaming draft action into the domain's
// draft.
//
// A streaming rich message does not arrive as a message at all: Telegram
// carries it as a *typing action*, the same channel a "still typing" indicator
// uses, with the partial document inside. That is why it can be replaced
// silently and why it expires on its own — it is never a message that existed
// and was withdrawn.
//
// A draft carrying only text (a bot that streams a plain reply) reports false:
// plain streaming already reads as typing, which this client shows, and turning
// it into an overlay as well would say the same thing twice.
func convertEphemeralDraft(chatID int64, action tg.SendMessageActionClass) (domain.EphemeralDraft, bool) {
	draftAction, ok := action.(*tg.SendMessageRichMessageDraftAction)
	if !ok {
		return domain.EphemeralDraft{}, false
	}
	rich := draftAction.RichMessage
	refs := newRichFileRefs(rich.Photos, rich.Documents)
	blocks := convertRichBlocks(rich.Blocks, refs)
	draft := domain.EphemeralDraft{
		ChatID:     chatID,
		ID:         int(draftAction.RandomID),
		Date:       time.Now(),
		TextBlocks: blocks,
	}
	// The "thinking" stage is a document whose only block says so; marking it
	// lets the overlay draw the placeholder rather than an empty document.
	if len(blocks) == 1 && blocks[0].Kind == domain.BlockKindThinking {
		draft.Placeholder = true
		if blocks[0].Text != nil {
			draft.Text = blocks[0].Text.Text
		}
	}
	return draft, true
}
