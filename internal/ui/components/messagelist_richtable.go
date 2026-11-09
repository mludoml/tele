package components

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// This file draws a rich-message table. A terminal has no cell borders, so the
// table is a fixed-width text grid: columns are sized to their content within
// the pane's width, cells wrap rather than overflow, and a header row is set off
// by a rule. See docs/rich-messages.md for what colspan/rowspan do here.

// richTableMinColW is the narrowest a column may be squeezed to before the table
// gives up on fitting and clips instead.
const richTableMinColW = 3

// richTableMaxCols bounds how many columns are drawn. A table with more columns
// than this would be unreadable at any terminal width, so the extra columns are
// reported rather than silently dropped into a smear of one-cell columns.
const richTableMaxCols = 8

// renderRichTable draws a table's grid at the given width, indented. The rows
// are the cell text as the table's own lines; the caller pads and wraps.
func (ml *MessageList) renderRichTable(b domain.PageBlock, width, indent int) []string {
	tb := b.Table
	if tb == nil {
		if b.Text != nil {
			return []string{theme.S().Body.Render(b.Text.Text)}
		}
		return nil
	}
	inner := width - indent
	if inner < 1 {
		inner = 1
	}
	rows := richTableRows(tb)
	if len(rows) == 0 {
		return nil
	}

	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	truncatedCols := false
	if cols > richTableMaxCols {
		cols = richTableMaxCols
		truncatedCols = true
	}

	out := ml.appendTableTitle(nil, tb.Title, width, indent)

	widths := richTableColWidths(rows, cols, inner, tb)
	for r, row := range rows {
		out = append(out, ml.renderTableRow(row, cols, widths, width, indent, tb)...)
		if richRowIsHeader(row) && r+1 < len(rows) {
			out = append(out, ml.renderTableRule(widths, width, indent, tb))
		}
	}
	if truncatedCols {
		out = append(out, theme.S().Timestamp.Render("table truncated to "+strconv.Itoa(cols)+" columns"))
	}
	return out
}

// appendTableTitle writes the table's own title above the grid. It is a plain
// line rather than a block: Bot API tables carry one title, not a heading tree.
func (ml *MessageList) appendTableTitle(out []string, title *domain.RichText, width, indent int) []string {
	if title == nil {
		return out
	}
	inner := width - indent
	if inner < 1 {
		inner = 1
	}
	for _, line := range wrapRichText(title, inner) {
		row := theme.Pad(indent) + theme.S().BodyBold.Render(line)
		out = append(out, row+theme.PadTo(lipgloss.Width(row), width))
	}
	return out
}

// richTableRows flattens a table's cells into positional rows of text. A cell
// spanning columns is emitted once and the columns it covers are left as
// continuations, which the width pass then merges — the visual simplification
// docs/rich-messages.md describes.
func richTableRows(tb *domain.TableBlock) [][]domain.TableCell {
	rows := make([][]domain.TableCell, 0, len(tb.Rows))
	for _, row := range tb.Rows {
		if len(row) == 0 {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func richRowIsHeader(row []domain.TableCell) bool {
	for _, c := range row {
		if c.IsHeader {
			return true
		}
	}
	return false
}

// richTableColWidths distributes the available width across columns, in two
// passes: every column gets at least its natural width if the table fits, and a
// table that does not fit is shrunk proportionally so the widest column gives up
// the most. Every column keeps at least richTableMinColW cells.
func richTableColWidths(rows [][]domain.TableCell, cols, inner int, tb *domain.TableBlock) []int {
	// Natural width of each column: the widest cell in it, capped so one long
	// prose cell cannot starve every other column.
	natural := make([]int, cols)
	const colCap = 40
	for _, row := range rows {
		position := 0
		for _, cell := range row {
			if position >= cols {
				break
			}
			w := lipgloss.Width(cell.Text.Text)
			if span := cell.Colspan; span > 1 && position+span <= cols {
				// A spanning cell shares its width over the columns it covers.
				w = (w + span - 1) / span
			}
			for i := 0; i < spanWidth(cell, cols-position); i++ {
				if position+i < cols && w > natural[position+i] {
					natural[position+i] = w
				}
			}
			position += spanWidth(cell, cols-position)
		}
	}
	for i := range natural {
		if natural[i] > colCap {
			natural[i] = colCap
		}
		if natural[i] < richTableMinColW {
			natural[i] = richTableMinColW
		}
	}

	// The separator between columns is drawn as a cell, so it costs width too.
	sep := 1
	if tb.Bordered {
		sep = 3 // " │ "
	}
	available := inner - (cols-1)*sep
	if available < cols*richTableMinColW {
		available = cols * richTableMinColW
	}

	total := 0
	for _, w := range natural {
		total += w
	}
	if total <= available {
		return natural
	}
	// Shrink the widest columns first: a table with one long column and three
	// short ones must not narrow the short ones to fit the long one.
	out := make([]int, cols)
	copy(out, natural)
	for {
		sum := 0
		widest, widestW := -1, 0
		for i, w := range out {
			sum += w
			if w > widestW {
				widest, widestW = i, w
			}
		}
		if sum <= available || widest < 0 || out[widest] <= richTableMinColW {
			return out
		}
		out[widest]--
	}
}

// spanWidth is how many columns a cell occupies, clamped to what remains.
func spanWidth(cell domain.TableCell, left int) int {
	n := cell.Colspan
	if n < 1 {
		n = 1
	}
	if n > left {
		n = left
	}
	if n < 1 {
		n = 1
	}
	return n
}

// renderTableRow draws one row: each cell wrapped and padded to its column
// width, the row's height being the tallest cell in it. Vertical alignment is
// honoured by padding the shorter cells above or below their text, so a "bottom"
// cell sits on the row's baseline rather than floating at its top.
func (ml *MessageList) renderTableRow(row []domain.TableCell, cols int, widths []int, width, indent int, tb *domain.TableBlock) []string {
	type cellRender struct {
		lines  []string
		align  string
		valign string
		header bool
		w      int
	}
	var cells []cellRender
	position := 0
	for _, cell := range row {
		if position >= cols {
			break
		}
		span := spanWidth(cell, cols-position)
		colW := 0
		for i := 0; i < span; i++ {
			colW += widths[position+i]
		}
		// The separators the spanned columns would have carried are part of this
		// cell's room, so a merged cell is exactly as wide as what it covers.
		colW += (span - 1) * sepWidth(tb)
		lines := wrapRichText(&cell.Text, max(1, colW))
		if len(lines) == 0 {
			lines = []string{""}
		}
		aligned := make([]string, len(lines))
		for k, line := range lines {
			aligned[k] = alignCell(line, lipgloss.Width(line), colW, cell.Align)
		}
		cells = append(cells, cellRender{
			lines:  aligned,
			align:  cell.Align,
			valign: cell.VAlign,
			header: cell.IsHeader,
			w:      colW,
		})
		position += span
	}
	if len(cells) == 0 {
		return nil
	}

	tallest := 0
	for _, c := range cells {
		if len(c.lines) > tallest {
			tallest = len(c.lines)
		}
	}

	// Vertical alignment: a cell's text is pushed down (top), centred (middle) or
	// pushed up (bottom) inside the row.
	padded := make([][]string, len(cells))
	for i, c := range cells {
		blank := theme.Pad(c.w)
		var lines []string
		switch c.valign {
		case "bottom":
			for n := tallest - len(c.lines); n > 0; n-- {
				lines = append(lines, blank)
			}
			lines = append(lines, c.lines...)
		case "middle":
			top := (tallest - len(c.lines)) / 2
			for n := 0; n < top; n++ {
				lines = append(lines, blank)
			}
			lines = append(lines, c.lines...)
			for len(lines) < tallest {
				lines = append(lines, blank)
			}
		default:
			lines = append(lines, c.lines...)
			for len(lines) < tallest {
				lines = append(lines, blank)
			}
		}
		padded[i] = lines
	}

	out := make([]string, 0, tallest)
	for rowIdx := 0; rowIdx < tallest; rowIdx++ {
		var sb strings.Builder
		for i, c := range cells {
			if i > 0 {
				sb.WriteString(richTableSeparator(tb))
			}
			text := padded[i][rowIdx]
			// The cell's text may already carry entity colours; the cell style is
			// layered around the painted run rather than through it, so alignment
			// and padding never split a colour.
			if c.header {
				text = theme.S().BodyBold.Render(text)
			}
			sb.WriteString(text)
		}
		line := theme.Pad(indent) + sb.String()
		out = append(out, line+theme.PadTo(lipgloss.Width(line), width))
	}
	return out
}

// alignCell places a wrapped, painted cell line inside its column: left by
// default, centered or right when the cell asked for it (Bot API cells carry an
// explicit align). The padding sits outside the painted run, so a cell's colours
// survive its alignment.
func alignCell(painted string, textW, colW int, align string) string {
	gap := colW - textW
	if gap <= 0 {
		return painted
	}
	switch align {
	case "right":
		return theme.Pad(gap) + painted
	case "center":
		left := gap / 2
		return theme.Pad(left) + painted + theme.Pad(gap-left)
	default:
		return painted + theme.Pad(gap)
	}
}

// sepWidth is the width a column separator occupies.
func sepWidth(tb *domain.TableBlock) int {
	if tb.Bordered {
		return 3
	}
	return 1
}

// richTableSeparator draws the rule between two columns. A bordered table draws
// a box-drawing vertical framed by spaces; a plain one is separated by a single
// column, which is what a compact table looks like. Its width must equal
// sepWidth — the column widths are computed against that number, so a separator
// drawn narrower than it is charged would leave every row short of the bubble.
func richTableSeparator(tb *domain.TableBlock) string {
	if tb.Bordered {
		return theme.Pad(1) + theme.S().Separator.Render("│") + theme.Pad(1)
	}
	return theme.Pad(1)
}

// renderTableRule draws the rule that sets a header row off from the body: one
// line of dashes exactly as wide as the row above it, which is what makes the
// header read as a header.
func (ml *MessageList) renderTableRule(widths []int, width, indent int, tb *domain.TableBlock) string {
	total := 0
	for _, w := range widths {
		total += w
	}
	total += (len(widths) - 1) * sepWidth(tb)
	if total > width-indent {
		total = width - indent
	}
	if total < 1 {
		total = 1
	}
	var rule string
	if tb.Bordered {
		// The rule joins the column verticals rather than cutting through them,
		// so the header reads as the top row of one grid. Each run is the dashes
		// the row above has between one join and the next, which is why the
		// first carries one cell of padding and a middle one carries two: this
		// is what puts every ┼ directly under the │ it replaces.
		parts := make([]string, 0, len(widths))
		for i, w := range widths {
			switch {
			case i == 0:
				parts = append(parts, strings.Repeat("─", w+1))
			case i == len(widths)-1:
				parts = append(parts, strings.Repeat("─", w+1))
			default:
				parts = append(parts, strings.Repeat("─", w+2))
			}
		}
		rule = theme.S().Separator.Render(strings.Join(parts, "┼"))
	} else {
		rule = theme.S().Separator.Render(strings.Repeat("─", total))
	}
	row := theme.Pad(indent) + rule
	return row + theme.PadTo(lipgloss.Width(row), width)
}
