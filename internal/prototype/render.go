package prototype

import (
	"unicode"
	"unicode/utf8"

	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/compare"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/micro/util"
	"github.com/nevdelap/mdiff/internal/viewport"
)

type renderedCell struct {
	rune       rune
	combc      []rune
	style      tcell.Style
	width      int
	sourceRune int
}

type renderedLine struct {
	changed  []bool
	selected []bool
}

type renderedRowKey struct{ line, wrapRow int }

func (a *App) renderPane(p *pane, layout viewport.Layout, top, height int) {
	cache := make(map[renderedRowKey][]renderedCell)
	masks := make(map[int]renderedLine)
	cursorRow, cursorCol := -1, -1
	if a.focusedPane() == p {
		loc := p.h.Cursor.Loc
		visual := p.view.VLocFromLoc(loc)
		cursorRow = layout.CursorRow(a.focus, loc.Y, visual.Row) - top
		cursorCol = p.view.BufView().X + visual.VisualX - p.view.StartCol - p.x
		if cursorCol < 0 || cursorCol >= p.width {
			cursorCol = -1
		}
	}
	for screenY := 0; screenY < height; screenY++ {
		rowIndex := top + screenY
		if rowIndex >= len(layout.Rows) {
			clearRow(p, screenY, config.DefStyle)
			continue
		}
		row := layout.Rows[rowIndex]
		cell := row.Left
		if p.x != 0 {
			cell = row.Right
		}
		base := config.DefStyle
		different := row.Kind != compare.Equal
		if different {
			base = diffStyle(row.Kind, p.x != 0)
		}
		clearRow(p, screenY, base)
		if cell.Spacer || cell.Continuation || cell.Line < 0 {
			continue
		}
		key := renderedRowKey{line: cell.Line, wrapRow: cell.WrapRow}
		cells, ok := cache[key]
		if !ok {
			selected := paneHasSelection(p)
			annotate := len(row.Changes) > 0 || selected
			cells = a.renderMicroRow(p, cell.Line, cell.WrapRow, annotate)
			cache[key] = cells
			if _, exists := masks[cell.Line]; !exists {
				line := renderedLine{}
				if annotate {
					runes := []rune(p.buf.Line(cell.Line))
					if len(row.Changes) > 0 {
						line.changed = changedRuneMask(runes, row.Changes, p.x != 0)
					}
					if selected {
						line.selected = selectionRuneMask(p, cell.Line, runes)
					}
				}
				masks[cell.Line] = line
			}
		}
		visibleCursorCol := -1
		if cursorRow == screenY {
			visibleCursorCol = cursorCol
		}
		lineMasks := masks[cell.Line]
		copyLine(p, screenY, cells, base, row, p.x != 0, different, visibleCursorCol, lineMasks.changed, lineMasks.selected)
	}
}

// renderMicroLine asks Micro's BufWindow renderer to paint one logical line
// into the offscreen tcell surface. The caller maps those rendered soft-wrap
// rows onto the shared diff-aware visual row sequence.
func (a *App) renderMicroLine(p *pane, line int, annotate bool) [][]renderedCell {
	wraps := a.wrapHeight(p, line)
	rows := make([][]renderedCell, wraps)
	for wrapRow := range wraps {
		rows[wrapRow] = a.renderMicroRow(p, line, wrapRow, annotate)
	}
	return rows
}

func (a *App) renderMicroRow(p *pane, line, wrapRow int, annotate bool) []renderedCell {
	view := p.view
	x, y, width, height := view.X, view.Y, view.Width, view.Height
	startLine, startCol, active := view.StartLine, view.StartCol, view.IsActive()
	view.X, view.Y, view.Height = 0, 0, 2
	view.StartLine = display.SLoc{Line: line, Row: wrapRow}
	view.SetActive(false)
	compositionScreen := screen.Screen
	a.paintScreen.Clear()
	screen.Screen = a.paintScreen
	view.Display()

	row := make([]renderedCell, p.width)
	for col := 0; col < p.width; col++ {
		r, combc, style, cellWidth := a.paintScreen.GetContent(col, 0)
		row[col] = renderedCell{rune: r, combc: append([]rune(nil), combc...), style: style, width: cellWidth, sourceRune: -1}
	}
	rows := [][]renderedCell{row}
	if annotate {
		annotateSourceRunes(p, line, rows, wrapRow)
	}
	screen.Screen = compositionScreen

	view.X, view.Y, view.Width, view.Height = x, y, width, height
	view.StartLine, view.StartCol = startLine, startCol
	view.Resize(width, height)
	view.StartLine, view.StartCol = startLine, startCol
	view.SetActive(active)
	return row
}

func diffStyle(kind compare.Kind, right bool) tcell.Style {
	background := tcell.ColorDarkBlue
	if kind == compare.Spacer {
		background = tcell.ColorDarkRed
		if right {
			background = tcell.ColorDarkGreen
		}
	}
	return config.DefStyle.Background(background)
}

func annotateSourceRunes(p *pane, line int, rows [][]renderedCell, rowOffset int) {
	if !p.buf.Settings["softwrap"].(bool) {
		annotateUnwrappedSourceRunes(p, line, rows)
		return
	}
	runes := []rune(p.buf.Line(line))
	locations := sourceRuneLocations(p, line, runes)
	for i, r := range runes {
		runeIndex := i
		if isCombining(r) {
			for runeIndex > 0 && isCombining(runes[runeIndex]) {
				runeIndex--
			}
			if runeIndex == i {
				continue
			}
		}
		start := locations[runeIndex]
		end := locations[runeIndex+1]
		markVisualRange(rows, start, end, runeIndex, p, rowOffset)
	}
}

func annotateUnwrappedSourceRunes(p *pane, line int, rows [][]renderedCell) {
	data := p.buf.LineBytes(line)
	tabSize := util.IntOpt(p.buf.Settings["tabsize"])
	startCol := p.view.StartCol
	endCol := startCol + p.view.BufView().Width
	visualX, runeIndex := 0, 0
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		data = data[size:]
		if isCombining(r) {
			runeIndex++
			continue
		}
		width := runewidth.RuneWidth(r)
		if r == '\t' {
			width = tabSize - visualX%tabSize
		}
		start := visualX
		visualX += width
		if visualX > startCol && start < endCol {
			markVisualRange(rows, display.VLoc{SLoc: display.SLoc{Line: line}, VisualX: start}, display.VLoc{SLoc: display.SLoc{Line: line}, VisualX: visualX}, runeIndex, p, 0)
		}
		runeIndex++
		if visualX >= endCol {
			break
		}
	}
}

func paneHasSelection(p *pane) bool {
	for _, cursor := range p.buf.GetCursors() {
		if cursor.HasSelection() {
			return true
		}
	}
	return false
}

// sourceRuneLocations mirrors Micro's width, word-wrap, and soft-wrap
// calculation in one pass. Calling VLocFromLoc for every rune would rescan the
// line prefix each time and make long-line rendering quadratic.
func sourceRuneLocations(p *pane, line int, runes []rune) []display.VLoc {
	locations := make([]display.VLoc, len(runes)+1)
	if !p.buf.Settings["softwrap"].(bool) {
		tabSize := util.IntOpt(p.buf.Settings["tabsize"])
		visualX := 0
		for i, r := range runes {
			locations[i] = display.VLoc{SLoc: display.SLoc{Line: line}, VisualX: visualX}
			width := runewidth.RuneWidth(r)
			if r == '\t' {
				width = tabSize - visualX%tabSize
			}
			visualX += width
		}
		locations[len(runes)] = display.VLoc{SLoc: display.SLoc{Line: line}, VisualX: visualX}
		return locations
	}

	bufView := p.view.BufView()
	bufWidth := bufView.Width
	if bufWidth <= 0 {
		return locations
	}
	tabSize := util.IntOpt(p.buf.Settings["tabsize"])
	wordwrap := p.buf.Settings["wordwrap"].(bool)
	row, visualX, totalWidth := 0, 0, 0
	widths := make([]int, 0, min(bufWidth, 256))
	for start := 0; start < len(runes); {
		wordStart := start
		widths = widths[:0]
		wordWidth := 0
		for start < len(runes) {
			r := runes[start]
			width := 0
			if r == '\t' {
				tabWidth := tabSize - totalWidth%tabSize
				width = min(tabWidth, bufWidth-visualX)
				totalWidth += tabWidth
			} else {
				width = runewidth.RuneWidth(r)
				totalWidth += width
			}
			widths = append(widths, width)
			wordWidth += width
			start++
			if wordwrap && !util.IsWhitespace(r) && start < len(runes) && wordWidth < bufWidth {
				continue
			}
			break
		}

		if visualX+wordWidth > bufWidth && visualX > 0 {
			row++
			visualX = 0
		}
		for i, width := range widths {
			locations[wordStart+i] = display.VLoc{SLoc: display.SLoc{Line: line, Row: row}, VisualX: visualX}
			visualX += width
		}
		if visualX >= bufWidth {
			row++
			visualX = 0
		}
	}
	locations[len(runes)] = display.VLoc{SLoc: display.SLoc{Line: line, Row: row}, VisualX: visualX}
	return locations
}

func markVisualRange(rows [][]renderedCell, start, end display.VLoc, runeIndex int, p *pane, rowOffset int) {
	softwrap := p.buf.Settings["softwrap"].(bool)
	contentStart := p.view.BufView().X
	startX, endX := start.VisualX, end.VisualX
	if !softwrap {
		startX -= p.view.StartCol
		endX -= p.view.StartCol
	}
	for row := start.Row; row <= end.Row; row++ {
		localRow := row - rowOffset
		if localRow < 0 || localRow >= len(rows) {
			continue
		}
		from, to := contentStart, p.width
		if row == start.Row {
			from = contentStart + startX
		}
		if row == end.Row {
			to = contentStart + endX
		}
		from = max(0, from)
		to = min(p.width, to)
		for col := from; col < to; col++ {
			rows[localRow][col].sourceRune = runeIndex
		}
	}
}

func changedRuneMask(runes []rune, changes []compare.ChangeSpan, right bool) []bool {
	mask := make([]bool, len(runes))
	for _, change := range changes {
		span := change.Left
		if right {
			span = change.Right
		}
		start := max(0, min(len(mask), span.Start))
		end := max(start, min(len(mask), span.End))
		for i := start; i < end; i++ {
			mask[i] = true
		}
	}
	return expandCombiningMask(runes, mask)
}

func selectionRuneMask(p *pane, line int, runes []rune) []bool {
	mask := make([]bool, len(runes))
	for _, cursor := range p.buf.GetCursors() {
		if !cursor.HasSelection() {
			continue
		}
		start, end := cursor.CurSelection[0], cursor.CurSelection[1]
		if start.GreaterThan(end) {
			start, end = end, start
		}
		for i := range runes {
			loc := buffer.Loc{X: i, Y: line}
			if loc.GreaterEqual(start) && loc.LessThan(end) {
				mask[i] = true
			}
		}
	}
	return expandCombiningMask(runes, mask)
}

func expandCombiningMask(runes []rune, mask []bool) []bool {
	for i, marked := range mask {
		if !marked || !isCombining(runes[i]) {
			continue
		}
		base := i - 1
		for base >= 0 && isCombining(runes[base]) {
			base--
		}
		if base >= 0 {
			mask[base] = true
		}
	}
	return mask
}

func isCombining(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r)
}

func clearRow(p *pane, screenY int, style tcell.Style) {
	for x := 0; x < p.width; x++ {
		screen.SetContent(p.x+x, screenY, ' ', nil, style)
	}
}

func copyLine(p *pane, screenY int, cells []renderedCell, rowStyle tcell.Style, row viewport.Row, right, different bool, cursorCol int, changed, selected []bool) {
	for x, cell := range cells {
		if cell.width == 0 {
			continue
		}
		style := baseCellStyle(cell.style, rowStyle)
		if different {
			_, background, _ := rowStyle.Decompose()
			style = style.Background(background)
		}
		if different && cell.sourceRune >= 0 && cell.sourceRune < len(changed) && changed[cell.sourceRune] {
			style = characterDiffStyle(rowStyle, right)
		}
		if cell.sourceRune >= 0 && cell.sourceRune < len(selected) && selected[cell.sourceRune] {
			style = selectionStyle()
		}
		if cursorCol >= 0 && x == cursorCol && !p.h.Cursor.HasSelection() {
			style = style.Reverse(true)
		}
		screen.SetContent(p.x+x, screenY, cell.rune, cell.combc, style)
	}
}

func baseCellStyle(syntax, row tcell.Style) tcell.Style {
	fg, _, attrs := syntax.Decompose()
	_, bg, _ := row.Decompose()
	return applyAttributes(tcell.StyleDefault.Foreground(fg).Background(bg), attrs)
}

func characterDiffStyle(row tcell.Style, right bool) tcell.Style {
	name, fallback := "diff-deleted", tcell.ColorRed
	if right {
		name, fallback = "diff-added", tcell.ColorGreen
	}
	style, ok := config.Colorscheme[name]
	if !ok {
		style = tcell.StyleDefault.Foreground(fallback)
	}
	fg, bg, attrs := style.Decompose()
	if fg == tcell.ColorDefault {
		fg = fallback
	}
	if bg == tcell.ColorDefault {
		_, bg, _ = row.Decompose()
	}
	return applyAttributes(tcell.StyleDefault.Foreground(fg).Background(bg), attrs)
}

func selectionStyle() tcell.Style {
	if style, ok := config.Colorscheme["selection"]; ok {
		return style
	}
	return config.DefStyle.Reverse(true)
}

func applyAttributes(style tcell.Style, attrs tcell.AttrMask) tcell.Style {
	style = style.Normal()
	style = style.Bold(attrs&tcell.AttrBold != 0)
	style = style.Blink(attrs&tcell.AttrBlink != 0)
	style = style.Reverse(attrs&tcell.AttrReverse != 0)
	style = style.Underline(attrs&tcell.AttrUnderline != 0)
	style = style.Dim(attrs&tcell.AttrDim != 0)
	style = style.Italic(attrs&tcell.AttrItalic != 0)
	style = style.StrikeThrough(attrs&tcell.AttrStrikeThrough != 0)
	return style
}
