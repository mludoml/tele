package components

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// This file draws Telegram's rich-message block document (a bot's
// richMessage). It is deliberately render-driven: the lines ARE the height, so
// the two can never drift the way a separate height formula would. The plan
// asked for renderRichBlocks and richBlocksHeight to share a wrap measurement;
// they share more than that — richBlocksHeight counts what renderRichBlocks
// produced, the same way an outbox item's height is measured (see
// computeItemHeight).
//
// The bubble a rich message is drawn in is laid out at the widest it may be
// rather than at its text's natural width. A table and a row of buttons are
// full-width by construction and have no narrower natural width, so measuring
// one would have to invent a number and the render would then have to agree
// with the invention. measureBubble takes the same decision for album grids.

// richBlockIndent is the left indent a nested block is drawn at, in cells.
const richBlockIndent = 2

// richBar is the vertical rule drawn beside a quotation.
const richBar = "▏"

// richDetailsClosed and richDetailsOpen mark a collapsible section's state. The
// arrow is the whole affordance: a terminal has no other way to say a section
// can be opened.
const (
	richDetailsClosed = "▸"
	richDetailsOpen   = "▾"
)

// renderRichBlocks draws a message's block document as bubble content lines at
// the given content width. It never returns nil-and-empty: a message that
// carried blocks always draws at least one row, so the bubble is never an empty
// box. An empty document is the caller's problem to avoid — a message with no
// blocks takes the ordinary text path.
func (ml *MessageList) renderRichBlocks(msgID int, blocks []domain.PageBlock, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	ml.appendRichBlocks(&out, msgID, "", blocks, width, 0)
	if len(out) == 0 {
		// Every block was skipped (an anchor, an empty document): one blank row,
		// so the bubble still has an inside and its height is one line rather
		// than zero.
		out = append(out, theme.Pad(width))
	}
	return out
}

// appendRichBlocks appends one level of blocks. path is the slash-joined index
// trail from the document root, which is what a details block's open state is
// keyed by: two collapsible sections in one message are two states.
func (ml *MessageList) appendRichBlocks(out *[]string, msgID int, path string, blocks []domain.PageBlock, width, indent int) {
	for i, b := range blocks {
		childPath := strconv.Itoa(i)
		if path != "" {
			childPath = path + "/" + childPath
		}
		ml.appendRichBlock(out, msgID, childPath, b, width, indent)
		if isRichMediaBlock(b.Kind) && i < len(blocks)-1 {
			// A photo, video, audio track, gallery or map reads as a distinct
			// object; content that follows it (a caption already belongs to
			// the block itself, drawn above) is set off the way a plain
			// message's media is set off from its own caption.
			ml.appendIndented(out, "", width, indent)
		}
	}
}

// isRichMediaBlock reports whether kind draws as a visual object that wants a
// blank row of separation from whatever block follows it.
func isRichMediaBlock(kind domain.BlockKind) bool {
	switch kind {
	case domain.BlockKindPhoto, domain.BlockKindVideo, domain.BlockKindAudio,
		domain.BlockKindCollage, domain.BlockKindSlideshow, domain.BlockKindMap:
		return true
	default:
		return false
	}
}

func (ml *MessageList) appendRichBlock(out *[]string, msgID int, path string, b domain.PageBlock, width, indent int) {
	inner := width - indent
	if inner < 1 {
		inner = 1
	}
	appendWrapped := func(rt *domain.RichText, style lipgloss.Style) {
		for _, line := range wrapRichText(rt, inner) {
			ml.appendIndented(out, style.Render(line), width, indent)
		}
	}

	switch b.Kind {
	case domain.BlockKindAnchor:
		// An anchor is a link target, not content: it occupies no rows. Drawing
		// a placeholder for it would put a line in the message that the sender
		// never wrote.

	case domain.BlockKindDivider:
		*out = append(*out, theme.S().Separator.Render(strings.Repeat("─", width)))

	case domain.BlockKindParagraph:
		appendWrapped(b.Text, theme.S().Body)

	case domain.BlockKindHeading:
		// A terminal has no font size. The six levels are told apart by
		// attributes instead: 1-2 the loudest, 3-4 bold, 5-6 quiet.
		style := theme.S().BodyBold
		switch {
		case b.Level <= 2:
			style = theme.S().BodyBold.Underline(true)
		case b.Level >= 5:
			style = theme.S().BodyBold.Foreground(theme.T().TextDim)
		}
		appendWrapped(b.Text, style)

	case domain.BlockKindPreformatted:
		ml.appendPreformatted(out, b, width, indent)

	case domain.BlockKindFooter:
		appendWrapped(b.Text, theme.S().Timestamp)

	case domain.BlockKindKicker:
		// A kicker is a short all-caps label above a title; the caps are the
		// look Telegram gives it, and there is no styling that conveys one.
		appendWrapped(upperRichText(b.Text), theme.S().Timestamp)

	case domain.BlockKindTitle:
		appendWrapped(b.Text, theme.S().BodyBold.Underline(true))

	case domain.BlockKindSubtitle:
		appendWrapped(b.Text, theme.S().Quote)

	case domain.BlockKindAuthorDate:
		ml.appendAuthorDate(out, b, width, indent)

	case domain.BlockKindBlockquote:
		ml.appendBlockquote(out, msgID, path, b, width, indent)

	case domain.BlockKindPullquote:
		ml.appendPullquote(out, b, width, indent)

	case domain.BlockKindList:
		ml.appendList(out, msgID, path, b, width, indent, false)

	case domain.BlockKindOrderedList:
		ml.appendList(out, msgID, path, b, width, indent, true)

	case domain.BlockKindTable:
		*out = append(*out, ml.renderRichTable(b, width, indent)...)

	case domain.BlockKindDetails:
		ml.appendDetails(out, msgID, path, b, width, indent)

	case domain.BlockKindCollage, domain.BlockKindSlideshow:
		ml.appendMediaGrid(out, b, width, indent)

	case domain.BlockKindMap:
		ml.appendMapBlock(out, b, width, indent)

	case domain.BlockKindPhoto, domain.BlockKindVideo,
		domain.BlockKindAudio, domain.BlockKindThinking:
		ml.appendMediaBlock(out, b, width, indent)

	case domain.BlockKindMath:
		// No LaTeX engine in a terminal, so the expression is shown verbatim
		// behind a marker that says it is maths. Rendering it as anything else
		// would be inventing a reading of it. See docs/rich-messages.md.
		rt := b.Text
		if rt == nil {
			rt = &domain.RichText{}
		}
		marker := "∑ "
		lines := wrapRichText(&domain.RichText{Text: marker + rt.Text, Entities: shiftEntities(rt.Entities, marker)}, inner)
		for _, line := range lines {
			ml.appendIndented(out, theme.S().Body.Foreground(theme.T().TextCode).Render(line), width, indent)
		}

	case domain.BlockKindUnsupported:
		name := b.Label
		if name == "" {
			name = "unknown"
		}
		ml.appendIndented(out, theme.S().Timestamp.Render("[unsupported: "+name+"]"), width, indent)

	default:
		ml.appendIndented(out, theme.S().Timestamp.Render("[unsupported: "+b.Kind.String()+"]"), width, indent)
	}
}

// appendIndented writes one already-painted line at an indent, padded to the
// full content width. Padding is emitted raw rather than through the style, so a
// line carrying a reset in the middle of it keeps its own colours.
func (ml *MessageList) appendIndented(out *[]string, line string, width, indent int) {
	pad := theme.Pad(indent)
	line = pad + line
	*out = append(*out, line+theme.PadTo(lipgloss.Width(line), width))
}

func (ml *MessageList) appendPreformatted(out *[]string, b domain.PageBlock, width, indent int) {
	inner := width - indent
	if inner < 1 {
		inner = 1
	}
	if b.Language != "" {
		label := theme.S().Timestamp.Render("```" + b.Language)
		ml.appendIndented(out, label, width, indent)
	}
	// A code block is drawn as a surface: every line is painted with the code
	// background so the block reads as one box rather than as ordinary text.
	code := theme.NewStyle().Foreground(theme.T().TextCode).Background(theme.T().SurfaceCode)
	for _, line := range wrapPlainRich(b.Text, inner, true) {
		row := code.Render(line)
		row += theme.PadTo(lipgloss.Width(row), inner)
		ml.appendIndented(out, row, width, indent)
	}
}

func (ml *MessageList) appendAuthorDate(out *[]string, b domain.PageBlock, width, indent int) {
	inner := width - indent
	if inner < 1 {
		inner = 1
	}
	author := plainRichText(b.Text)
	when := ""
	if b.Date > 0 {
		when = formattedRichDate(b.Date)
	}
	label := author
	if label != "" && when != "" {
		label += " · "
	}
	label += when
	if strings.TrimSpace(label) == "" {
		return
	}
	for _, line := range wrapTextLines(label, inner) {
		ml.appendIndented(out, theme.S().Timestamp.Render(line), width, indent)
	}
}

func (ml *MessageList) appendBlockquote(out *[]string, msgID int, path string, b domain.PageBlock, width, indent int) {
	inner := width - indent - richBlockIndent
	if inner < 1 {
		inner = 1
	}
	bar := theme.S().Quote.Render(richBar) + theme.Pad(richBlockIndent-1)
	appendQuoted := func(rt *domain.RichText) {
		for _, line := range wrapRichText(rt, inner) {
			content := theme.S().Quote.Render(line)
			row := theme.Pad(indent) + bar + content
			*out = append(*out, row+theme.PadTo(lipgloss.Width(row), width))
		}
	}
	if b.Text != nil {
		appendQuoted(b.Text)
	}
	if len(b.Children) > 0 {
		// A nested tree is indented under the same bar, so the quotation stays
		// one visual unit however deep it goes.
		before := len(*out)
		ml.appendRichBlocks(out, msgID, path, b.Children, width, indent+richBlockIndent)
		for i := before; i < len(*out); i++ {
			row := theme.Pad(indent) + bar + xansi.TruncateLeft((*out)[i], indent+richBlockIndent, "")
			(*out)[i] = row + theme.PadTo(lipgloss.Width(row), width)
		}
	}
	credit := b.Credit
	if credit != nil {
		for _, line := range wrapRichText(credit, inner) {
			row := theme.Pad(indent+richBlockIndent) + theme.S().Quote.Italic(true).Render("— "+line)
			*out = append(*out, row+theme.PadTo(lipgloss.Width(row), width))
		}
	}
}

func (ml *MessageList) appendPullquote(out *[]string, b domain.PageBlock, width, indent int) {
	inner := width - indent
	if inner < 1 {
		inner = 1
	}
	style := theme.S().Body.Italic(true)
	for _, line := range wrapRichText(b.Text, inner) {
		// Centered, because that is what makes a pull quote a pull quote.
		w := lipgloss.Width(line)
		left := (width - w) / 2
		if left < indent {
			left = indent
		}
		row := theme.Pad(left) + style.Render(line)
		*out = append(*out, row+theme.PadTo(lipgloss.Width(row), width))
	}
	if b.Credit != nil {
		for _, line := range wrapRichText(b.Credit, inner) {
			row := theme.Pad(indent) + theme.S().Timestamp.Render("— "+line)
			*out = append(*out, row+theme.PadTo(lipgloss.Width(row), width))
		}
	}
}

// appendList draws an unordered or ordered list. Items are children of kind
// BlockKindListItem; an item's own content is either inline text or a nested
// block tree, and both are drawn under the item's marker.
func (ml *MessageList) appendList(out *[]string, msgID int, path string, b domain.PageBlock, width, indent int, ordered bool) {
	number := b.Start
	if number == 0 {
		number = 1
	}
	for i, item := range b.Children {
		itemPath := path + "/" + strconv.Itoa(i)
		marker := "- "
		if ordered {
			marker = orderedMarker(b.Marker, item, number)
			number++
		}
		if item.Checkbox {
			if item.Checked {
				marker = "[x] "
			} else {
				marker = "[ ] "
			}
		}
		markerW := lipgloss.Width(marker)
		inner := width - indent - markerW
		if inner < 1 {
			inner = 1
		}
		// The marker sits on the item's first line; continuation lines and any
		// nested block align under the text rather than under the bullet.
		prefix := theme.Pad(indent) + theme.S().Timestamp.Render(marker)
		switch {
		case item.Text != nil:
			first := true
			for _, line := range wrapRichText(item.Text, inner) {
				row := prefix + theme.S().Body.Render(line)
				if !first {
					row = theme.Pad(indent+markerW) + theme.S().Body.Render(line)
				}
				first = false
				*out = append(*out, row+theme.PadTo(lipgloss.Width(row), width))
			}
		case len(item.Children) > 0:
			before := len(*out)
			ml.appendRichBlocks(out, msgID, itemPath, item.Children, width, indent+markerW)
			ml.prefixLines(out, before, prefix, width)
		default:
			row := prefix
			*out = append(*out, row+theme.PadTo(lipgloss.Width(row), width))
		}
	}
}

// prefixLines rewrites the lines appended since before so they begin with prefix
// (used to hang a list marker beside a nested block). The prefix is re-applied
// by truncating the indent those lines already carry.
func (ml *MessageList) prefixLines(out *[]string, before int, prefix string, width int) {
	prefixW := lipgloss.Width(prefix)
	for i := before; i < len(*out); i++ {
		row := prefix + xansi.TruncateLeft((*out)[i], prefixW, "")
		(*out)[i] = row + theme.PadTo(lipgloss.Width(row), width)
	}
}

// orderedMarker is the label of one ordered item: the number the server
// rendered when it sent one (the only way a reversed or non-decimal list reads
// correctly), or a marker generated from the item's position.
func orderedMarker(listMarker string, item domain.PageBlock, n int) string {
	// The item's own label wins: the server rendered it, which is the only way a
	// reversed or explicitly-numbered list reads correctly. The item's Marker is
	// the per-item spelling (an individual item may override the list's), and the
	// list's is the fallback.
	label := item.Label
	if label != "" {
		return label + " "
	}
	marker := item.Marker
	if marker == "" {
		marker = listMarker
	}
	switch marker {
	case "a":
		return string(rune('a'+((n-1)%26))) + ". "
	case "A":
		return string(rune('A'+((n-1)%26))) + ". "
	case "i", "I":
		roman := romanNumeral(n)
		if marker == "I" {
			roman = strings.ToUpper(roman)
		}
		return roman + ". "
	default:
		return strconv.Itoa(n) + ". "
	}
}

func romanNumeral(n int) string {
	if n <= 0 || n > 3999 {
		return strconv.Itoa(n)
	}
	table := []struct {
		value int
		glyph string
	}{
		{1000, "m"}, {900, "cm"}, {500, "d"}, {400, "cd"},
		{100, "c"}, {90, "xc"}, {50, "l"}, {40, "xl"},
		{10, "x"}, {9, "ix"}, {5, "v"}, {4, "iv"}, {1, "i"},
	}
	var sb strings.Builder
	for _, e := range table {
		for n >= e.value {
			sb.WriteString(e.glyph)
			n -= e.value
		}
	}
	return sb.String()
}

func (ml *MessageList) appendDetails(out *[]string, msgID int, path string, b domain.PageBlock, width, indent int) {
	open := ml.richDetailsOpen(msgID, path, b.DetailsOpen)
	arrow := richDetailsClosed
	if open {
		arrow = richDetailsOpen
	}
	summary := plainRichText(b.Text)
	if summary == "" {
		summary = "Details"
	}
	head := theme.S().Timestamp.Render(arrow+" ") + theme.S().BodyBold.Render(summary)
	ml.appendIndented(out, head, width, indent)
	if open && len(b.Children) > 0 {
		ml.appendRichBlocks(out, msgID, path, b.Children, width, indent+richBlockIndent)
	}
}

// appendMediaGrid draws a collage or a slideshow as one grid of preview tiles.
//
// A slideshow is drawn exactly like a collage: a terminal cannot swipe, and
// paginating a deck behind a key that exists only inside this block would be a
// mode nobody could discover. See docs/rich-messages.md.
func (ml *MessageList) appendMediaGrid(out *[]string, b domain.PageBlock, width, indent int) {
	inner := width - indent
	if inner < 1 {
		inner = 1
	}
	parts := richMediaParts(b.Children)
	// A file the grid cannot preview (a document with no thumbnail) is named in
	// place rather than dropped, and it keeps its position: the block tree is
	// what the sender wrote, and a grid that silently skipped one child would
	// misdescribe what arrived.
	var unpictured []domain.PageBlock
	for _, child := range b.Children {
		if !richBlockIsPreviewable(child) {
			unpictured = append(unpictured, child)
		}
	}

	if prev := ml.previewParts(parts); len(prev) > 0 {
		cols := mosaicCols(len(prev))
		widths := tileWidths(inner, cols)
		if mosaicUsesGrid(len(prev), minWidth(widths)) {
			rows := (len(prev) + cols - 1) / cols
			tileRows := ml.tileRowsFor(minWidth(widths))
			ml.appendTileGrid(out, prev, widths, cols, rows, tileRows, width, indent)
		} else {
			// Too narrow to grid (a sidebar-width pane): one tile per row, with
			// each tile's own badge, which is what the album stack does too.
			for _, gm := range prev {
				ml.appendMediaBlock(out, domain.PageBlock{
					Kind:     mediaKindOf(gm.Msg),
					Media:    gm.Msg.Media,
					Photo:    gm.Msg.Photo,
					Document: gm.Msg.Document,
				}, width, indent)
			}
		}
	}

	for _, child := range unpictured {
		if len(unpictured) > 0 && len(parts) > 0 {
			ml.appendIndented(out, "", width, indent)
		}
		ml.appendRichBlock(out, 0, "", child, width, indent)
	}

	if b.Caption != nil {
		ml.appendIndented(out, "", width, indent)
		for _, line := range wrapRichText(b.Caption, inner) {
			ml.appendIndented(out, theme.S().Body.Render(line), width, indent)
		}
	}
	ml.appendCredit(out, b.Credit, width, indent)
}

// richBlockIsPreviewable reports whether a collage child has an image the grid
// can draw now: a photo, or a document with a thumbnail.
func richBlockIsPreviewable(c domain.PageBlock) bool {
	if c.Media == nil {
		return false
	}
	if c.Media.Kind == domain.MediaPhoto && c.Photo != nil {
		return true
	}
	return c.Document != nil && c.Document.ThumbSize != ""
}

// appendTileGrid lays previewable parts out in a grid of tileRows-tall tiles,
// sharing the album mosaic's own tile geometry (coverWindow/renderMosaicTile) so
// there is one implementation of what a tile is.
func (ml *MessageList) appendTileGrid(out *[]string, prev []groupMedia, widths []int, cols, rows, tileRows, width, indent int) {
	for r := 0; r < rows; r++ {
		tiles := make([][]string, 0, cols)
		for c := 0; c < cols; c++ {
			idx := r*cols + c
			if idx >= len(prev) {
				tiles = append(tiles, nil) // blank slot in a partial last row
				continue
			}
			gm := prev[idx]
			iw, ih := ml.previewDims(gm.Msg)
			g := coverWindow(iw, ih, widths[c], tileRows)
			tiles = append(tiles, ml.renderMosaicTile(gm, g, albumBadgeLabel(gm.Index, gm.Msg)+" "))
		}
		for row := 0; row < tileRows; row++ {
			var sb strings.Builder
			for c, t := range tiles {
				if c > 0 {
					sb.WriteString(theme.Pad(mosaicGap))
				}
				cell := theme.Pad(widths[c])
				if row < len(t) {
					cell = t[row]
				}
				sb.WriteString(cell)
			}
			line := theme.Pad(indent) + sb.String()
			*out = append(*out, line+theme.PadTo(lipgloss.Width(line), width))
		}
		if r < rows-1 {
			ml.appendIndented(out, "", width, indent)
		}
	}
}

// richMediaParts turns a collage's children into the album-shaped messages the
// mosaic geometry reads. The block tree is the domain's, but the grid's math is
// the album's, and feeding it the same shape is what keeps one implementation of
// tile geometry instead of two.
func richMediaParts(children []domain.PageBlock) []domain.Message {
	out := make([]domain.Message, 0, len(children))
	for _, c := range children {
		if c.Media == nil && c.Photo == nil && c.Document == nil {
			continue
		}
		out = append(out, domain.Message{Media: c.Media, Photo: c.Photo, Document: c.Document})
	}
	return out
}

func mediaKindOf(msg domain.Message) domain.BlockKind {
	if msg.Media == nil {
		return domain.BlockKindUnsupported
	}
	switch msg.Media.Kind {
	case domain.MediaPhoto:
		return domain.BlockKindPhoto
	case domain.MediaVideo, domain.MediaVideoNote, domain.MediaGIF:
		return domain.BlockKindVideo
	case domain.MediaAudio, domain.MediaVoice:
		return domain.BlockKindAudio
	default:
		return domain.BlockKindUnsupported
	}
}

// appendMediaBlock draws a single media block: the picture or its placeholder,
// then the caption and the credit. It goes through the same image path a
// message's media does, so a rich block's photo is fetched and cached exactly
// like a photo message's.
func (ml *MessageList) appendMediaBlock(out *[]string, b domain.PageBlock, width, indent int) {
	inner := width - indent
	if inner < 1 {
		inner = 1
	}
	if b.Kind == domain.BlockKindThinking {
		label := plainRichText(b.Text)
		if label == "" {
			label = "thinking…"
		}
		// The streaming draft's placeholder, drawn dim and italic so it reads as
		// unfinished rather than as content.
		ml.appendIndented(out, theme.S().Quote.Italic(true).Render("✻ "+label), width, indent)
		return
	}
	msg := domain.Message{Media: b.Media, Photo: b.Photo, Document: b.Document}
	switch {
	case b.Media == nil:
		// The file the block named was not in the message: its caption is all
		// there is to draw.
	case richBlockIsPreviewable(b):
		if id, ok := ml.PreviewImageID(msg); ok {
			if img, has := ml.cachedImage(id); has {
				bb := img.Bounds()
				cols, _ := ml.mediaBox(msg, bb.Dx(), bb.Dy())
				if cols > inner {
					cols = inner
				}
				for _, art := range ml.renderer.Render(id, img, cols) {
					ml.appendIndented(out, art, width, indent)
				}
				if overlay := videoOverlayLabel(b.Media); overlay != "" {
					ml.appendIndented(out, theme.S().Timestamp.Render(overlay), width, indent)
				}
				break
			}
		}
		// Bytes are not cached yet: the placeholder stands in their place and
		// the picture swaps in at the same row count once they arrive.
		ml.appendIndented(out, theme.S().Body.Render(placeholderFor(b.Media)), width, indent)
	default:
		ml.appendIndented(out, theme.S().Body.Render(placeholderFor(b.Media)), width, indent)
	}
	if b.Caption != nil {
		for _, line := range wrapRichText(b.Caption, inner) {
			ml.appendIndented(out, theme.S().Body.Render(line), width, indent)
		}
	}
	ml.appendCredit(out, b.Credit, width, indent)
}

func (ml *MessageList) appendCredit(out *[]string, credit *domain.RichText, width, indent int) {
	if credit == nil {
		return
	}
	inner := width - indent
	if inner < 1 {
		inner = 1
	}
	for _, line := range wrapRichText(credit, inner) {
		ml.appendIndented(out, theme.S().Timestamp.Render("— "+line), width, indent)
	}
}

// appendMapBlock draws a map as what a terminal can actually show: where it
// points and how far in, plus a button that opens the same place in a browser.
// Tile rendering needs a graphics protocol and a tile server; the coordinates
// are the honest rendering of the rest. See docs/rich-messages.md.
func (ml *MessageList) appendMapBlock(out *[]string, b domain.PageBlock, width, indent int) {
	inner := width - indent
	if inner < 1 {
		inner = 1
	}
	if b.Map != nil {
		label := fmt.Sprintf("📍 %.5f, %.5f · zoom %d", b.Map.Lat, b.Map.Long, b.Map.Zoom)
		for _, line := range wrapTextLines(label, inner) {
			ml.appendIndented(out, theme.S().Body.Render(line), width, indent)
		}
	}
	if b.Caption != nil {
		for _, line := range wrapRichText(b.Caption, inner) {
			ml.appendIndented(out, theme.S().Body.Render(line), width, indent)
		}
	}
	ml.appendCredit(out, b.Credit, width, indent)
}

// wrapRichText renders a rich text run and wraps it to width, using the same
// lipgloss wrap the bubble body uses so a rich block's text breaks in the same
// places ordinary message text would.
func wrapRichText(rt *domain.RichText, width int) []string {
	if rt == nil {
		return nil
	}
	return wrapPainted(RenderEntities(rt.Text, rt.Entities), width)
}

// wrapPlainRich is wrapRichText for a block drawn without entity styling (a code
// block paints its own), preserving every line including empty ones.
func wrapPlainRich(rt *domain.RichText, width int, keepEmpty bool) []string {
	if rt == nil {
		return nil
	}
	lines := wrapPainted(rt.Text, width)
	if keepEmpty && len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// wrapPainted wraps an already-painted string to width, one line at a time. A
// blank source line stays one blank row, matching the bubble body's handling of
// paragraph breaks.
func wrapPainted(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	// canvas:ok measurement only, as in wrappedLineCount: this style only breaks
	// lines, and the runs arriving here are already painted.
	wrapStyle := lipgloss.NewStyle().Width(width)
	for _, part := range strings.Split(s, "\n") {
		if part == "" {
			out = append(out, "")
			continue
		}
		for _, wl := range strings.Split(wrapStyle.Render(part), "\n") {
			out = append(out, strings.TrimRight(wl, " "))
		}
	}
	return out
}

// wrapTextLines wraps plain unrendered text, for the labels this file composes
// itself (a map's coordinates, an author and date).
func wrapTextLines(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	// canvas:ok measurement only, the same reason as wrapPainted.
	wrapStyle := lipgloss.NewStyle().Width(width)
	var out []string
	for _, wl := range strings.Split(wrapStyle.Render(s), "\n") {
		out = append(out, strings.TrimRight(wl, " "))
	}
	return out
}

// plainRichText is a rich run's text without its styling, for labels that are
// composed and then painted by the caller.
func plainRichText(rt *domain.RichText) string {
	if rt == nil {
		return ""
	}
	return rt.Text
}

// upperRichText uppercases a run for a kicker, leaving its entities' offsets
// alone: the mapping rune-to-rune is identity for the scripts Telegram uses in
// labels, and an entity that shifted would style the wrong word.
func upperRichText(rt *domain.RichText) *domain.RichText {
	if rt == nil {
		return nil
	}
	return &domain.RichText{Text: strings.ToUpper(rt.Text), Entities: shiftEntities(rt.Entities, "")}
}

// shiftEntities returns entities unchanged. It exists as the named place where a
// prefixed run would have to move its offsets: prefixing text shifts every
// entity by the prefix's UTF-16 length, and getting it wrong styles the wrong
// word. Callers that prefix text must pass the prefix here.
func shiftEntities(entities []domain.MessageEntity, prefix string) []domain.MessageEntity {
	if prefix == "" || len(entities) == 0 {
		return entities
	}
	shift := len([]rune(prefix))
	if !isASCII(prefix) {
		// A non-ASCII prefix needs the UTF-16 length, not the rune count.
		shift = utf16Length(prefix)
	}
	out := make([]domain.MessageEntity, len(entities))
	for i, e := range entities {
		e.Offset += shift
		out[i] = e
	}
	return out
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
			continue
		}
		n++
	}
	return n
}

// formattedRichDate renders the publishing date of an author_date block. It uses
// a fixed layout rather than a locale: a theme cannot supply one, and the date
// is a label rather than a message.
func formattedRichDate(unix int) string {
	return time.Unix(int64(unix), 0).Format("2006-01-02")
}

// richActive reports whether the message's block document is what gets drawn.
// The config gate lives here rather than at each call site, so "is this a rich
// message" has one answer for the renderer and the height alike.
func (ml *MessageList) richActive(msg domain.Message) bool {
	return ml.richEnabled && len(msg.RichBlocks) > 0
}

// buttonsActive reports whether a message's inline keyboard is drawn. A keyboard
// is not a rich-message feature: an ordinary bot message carries one too, so it
// is drawn for any message that has one.
func (ml *MessageList) buttonsActive(msg domain.Message) bool {
	return ml.richEnabled && msg.ReplyMarkup != nil && msg.ReplyMarkup.Count() > 0
}

// richDrawLines builds the bordered bubble interior rows a rich message adds
// beyond its ordinary content: the block document (when the message has one) and
// the inline keyboard (when it has one).
//
// Both the render and the height measurement call this one function, which is
// what makes the two impossible to drift: there is no height arithmetic for a
// rich message anywhere, only a count of the lines this produced.
func (ml *MessageList) richDrawLines(msg domain.Message, actualW, innerW int, b lipgloss.Border, bs lipgloss.Style) []string {
	if actualW < 1 {
		actualW = 1
	}
	blank := bs.Render(b.Left) + theme.Pad(innerW) + bs.Render(b.Right)
	var sideLines []string

	if ml.richActive(msg) {
		for _, line := range ml.renderRichBlocks(msg.ID, msg.RichBlocks, actualW) {
			sideLines = append(sideLines, bs.Render(b.Left)+theme.Pad(1)+line+theme.Pad(1)+bs.Render(b.Right))
		}
	}
	if ml.buttonsActive(msg) {
		if len(sideLines) > 0 {
			// The keyboard is set off from the content above it, so a row of
			// buttons does not read as part of the paragraph it follows.
			sideLines = append(sideLines, blank)
		}
		cursor := -1
		if ml.buttonMsgID == msg.ID {
			cursor = ml.buttonIndex
		}
		for _, line := range ml.renderReplyMarkup(msg.ReplyMarkup, actualW, cursor) {
			sideLines = append(sideLines, bs.Render(b.Left)+theme.Pad(1)+line+theme.Pad(1)+bs.Render(b.Right))
		}
		// Why a button cannot be used is spelled out under the keyboard rather
		// than inside its cell: a reason is a sentence, and a cell is a button.
		for _, btn := range msg.ReplyMarkup.Buttons() {
			if btn.Action.Kind != domain.ButtonActionNone || btn.Action.Reason == "" {
				continue
			}
			for _, line := range wrapTextLines("⚠ "+btn.Text+": "+btn.Action.Reason, actualW) {
				painted := theme.S().Timestamp.Render(line)
				sideLines = append(sideLines, bs.Render(b.Left)+theme.Pad(1)+painted+
					theme.PadTo(lipgloss.Width(line), actualW)+theme.Pad(1)+bs.Render(b.Right))
			}
		}
	}
	return sideLines
}
