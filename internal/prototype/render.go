package prototype

import (
	"sort"
	"unicode"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/compare"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
	"github.com/nevdelap/mdiff/internal/micro/screen"
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
	changed   []bool
	selected  []bool
	runeStart int
}

type renderedRowKey struct{ line, wrapRow int }

func (a *App) renderPane(p *pane, layout viewport.Layout, top, height int) {
	cache := make(map[renderedRowKey][]renderedCell)
	masks := make(map[int]renderedLine)
	sourceMaps := make(map[int]display.RuneBoundaryMap)
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
		currentLine := p.view.IsActive() && cell.Line >= 0 &&
			p.buf.Settings["cursorline"].(bool) && p.h.Cursor.Y == cell.Line &&
			!p.h.Cursor.HasSelection()
		base := config.DefStyle
		different := row.Kind != compare.Equal
		if different {
			base = diffStyle(row.Kind, p.x != 0, currentLine)
		} else if currentLine {
			if style, ok := config.Colorscheme["cursor-line"]; ok {
				color, _, _ := style.Decompose()
				base = base.Background(color)
			}
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
			var positions *display.RuneBoundaryMap
			if annotate {
				mapped, exists := sourceMaps[cell.Line]
				if !exists {
					if p.buf.Settings["softwrap"].(bool) {
						mapped = p.view.RuneBoundaries(cell.Line)
					} else {
						mapped = p.view.VisibleRuneBoundaries(cell.Line, p.view.StartCol, p.view.BufView().Width)
					}
					sourceMaps[cell.Line] = mapped
				}
				positions = &mapped
			}
			cells = a.renderMicroRowMapped(p, cell.Line, cell.WrapRow, positions)
			cache[key] = cells
			if _, exists := masks[cell.Line]; !exists {
				line := renderedLine{}
				if annotate {
					runes := sourceMaps[cell.Line].Runes
					line.runeStart = sourceMaps[cell.Line].RuneOffset
					if len(row.Changes) > 0 {
						line.changed = changedRuneMaskAt(runes, line.runeStart, row.Changes, p.x != 0)
					}
					if selected {
						line.selected = selectionRuneMask(p, cell.Line, runes, sourceMaps[cell.Line].Character)
					}
				}
				masks[cell.Line] = line
			}
		}
		lineMasks := masks[cell.Line]
		copyLine(p, screenY, cells, base, row, p.x != 0, different, lineMasks.runeStart, lineMasks.changed, lineMasks.selected)
	}
}

// renderMicroLine asks Micro's BufWindow renderer to paint one logical line
// into the offscreen tcell surface. The caller maps those rendered soft-wrap
// rows onto the shared diff-aware visual row sequence.
func (a *App) renderMicroLine(p *pane, line int, annotate bool) [][]renderedCell {
	wraps := a.wrapHeight(p, line)
	rows := make([][]renderedCell, wraps)
	var positions *display.RuneBoundaryMap
	if annotate {
		mapped := p.view.RuneBoundaries(line)
		positions = &mapped
	}
	for wrapRow := range wraps {
		rows[wrapRow] = a.renderMicroRowMapped(p, line, wrapRow, positions)
	}
	return rows
}

func (a *App) renderMicroRow(p *pane, line, wrapRow int, annotate bool) []renderedCell {
	var positions *display.RuneBoundaryMap
	if annotate {
		mapped := p.view.RuneBoundaries(line)
		positions = &mapped
	}
	return a.renderMicroRowMapped(p, line, wrapRow, positions)
}

func (a *App) renderMicroRowMapped(p *pane, line, wrapRow int, positions *display.RuneBoundaryMap) []renderedCell {
	view := p.view
	x, y, width, height := view.X, view.Y, view.Width, view.Height
	startLine, startCol, active := view.StartLine, view.StartCol, view.IsActive()
	view.X, view.Y, view.Height = 0, 0, 2
	view.StartLine = display.SLoc{Line: line, Row: wrapRow}
	view.SetActive(active)
	compositionScreen := screen.Screen
	a.paintScreen.Clear()
	screen.Screen = a.paintScreen
	view.SuppressCursor = true
	view.Display()
	view.SuppressCursor = false

	row := make([]renderedCell, p.width)
	for col := 0; col < p.width; col++ {
		r, combc, style, cellWidth := a.paintScreen.GetContent(col, 0)
		row[col] = renderedCell{rune: r, combc: append([]rune(nil), combc...), style: style, width: cellWidth, sourceRune: -1}
	}
	rows := [][]renderedCell{row}
	if positions != nil {
		annotateSourceRunes(p, rows, wrapRow, *positions)
	}
	screen.Screen = compositionScreen

	view.X, view.Y, view.Width, view.Height = x, y, width, height
	view.StartLine, view.StartCol = startLine, startCol
	view.Resize(width, height)
	view.StartLine, view.StartCol = startLine, startCol
	view.SetActive(active)
	return row
}

func diffStyle(kind compare.Kind, right, currentLine bool) tcell.Style {
	group := "mdiff.diff.changed"
	background := tcell.NewHexColor(0x000046)
	currentBackground := tcell.ColorDarkBlue
	lightAccent := int32(0x2680D9)
	if kind == compare.Spacer {
		group = "mdiff.diff.left-only"
		background = tcell.NewHexColor(0x460000)
		currentBackground = tcell.ColorDarkRed
		lightAccent = 0xD64A4A
		if right {
			group = "mdiff.diff.right-only"
			background = tcell.NewHexColor(0x003200)
			currentBackground = tcell.ColorDarkGreen
			lightAccent = 0x2F9E5B
		}
	}
	_, themeBackground, _ := config.DefStyle.Decompose()
	if themeBackground.Hex() < 0 {
		// A terminal-default background may be light or dark. Keep its colors
		// unless the selected Micro theme explicitly sets an mdiff row color.
		background = themeBackground
		currentBackground = themeBackground
	} else if lightBackground, ok := lightThemeBackground(); ok {
		background = blendColor(lightBackground, lightAccent, 15)
		currentBackground = blendColor(lightBackground, lightAccent, 35)
	}
	if currentLine {
		return themedRowBackground(group+".current", currentBackground)
	}
	return themedRowBackground(group, background)
}

func lightThemeBackground() (tcell.Color, bool) {
	_, background, _ := config.DefStyle.Decompose()
	rgb := background.Hex()
	if rgb < 0 {
		return 0, false
	}
	r, g, b := (rgb>>16)&0xff, (rgb>>8)&0xff, rgb&0xff
	return background, (299*r+587*g+114*b)/1000 >= 160
}

func blendColor(background tcell.Color, accent int32, percent int32) tcell.Color {
	base := background.Hex()
	channel := func(shift uint) int32 {
		b := (base >> shift) & 0xff
		a := (accent >> shift) & 0xff
		return (b*(100-percent) + a*percent + 50) / 100
	}
	return tcell.NewRGBColor(channel(16), channel(8), channel(0))
}

func themedRowBackground(group string, fallback tcell.Color) tcell.Style {
	if style, ok := config.Colorscheme[group]; ok {
		_, background, _ := style.Decompose()
		return config.DefStyle.Background(background)
	}
	return config.DefStyle.Background(fallback)
}

func paneHasSelection(p *pane) bool {
	for _, cursor := range p.buf.GetCursors() {
		if cursor.HasSelection() {
			return true
		}
	}
	return false
}

// annotateSourceRunes consumes Micro's rune boundaries. The binary search
// skips characters above this visual row, so a long wrapped line is not
// rescanned from its beginning for every rendered row.
func annotateSourceRunes(p *pane, rows [][]renderedCell, rowOffset int, positions display.RuneBoundaryMap) {
	count := len(positions.Visual) - 1
	if count <= 0 || positions.RuneOffset < 0 {
		return
	}
	first := sort.Search(count, func(i int) bool {
		return positions.Visual[i+1].Row >= rowOffset
	})
	for i := first; i < count; i++ {
		start := positions.Visual[i]
		if start.Row > rowOffset {
			break
		}
		if positions.Character[i] == positions.Character[i+1] {
			continue // combining mark; its base owns the visual cells
		}
		markVisualRange(rows, start, positions.Visual[i+1], positions.RuneOffset+i, p, rowOffset)
	}
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

func changedRuneMaskAt(runes []rune, runeStart int, changes []compare.ChangeSpan, right bool) []bool {
	mask := make([]bool, len(runes))
	for _, change := range changes {
		span := change.Left
		if right {
			span = change.Right
		}
		start := max(0, min(len(mask), span.Start-runeStart))
		end := max(start, min(len(mask), span.End-runeStart))
		for i := start; i < end; i++ {
			mask[i] = true
		}
	}
	return expandCombiningMask(runes, mask)
}

func selectionRuneMask(p *pane, line int, runes []rune, runeToCharacter []int) []bool {
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
			loc := buffer.Loc{X: runeToCharacter[i+1] - 1, Y: line}
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

func copyLine(p *pane, screenY int, cells []renderedCell, rowStyle tcell.Style, row viewport.Row, right, different bool, runeStart int, changed, selected []bool) {
	for x, cell := range cells {
		if cell.width == 0 {
			continue
		}
		style := baseCellStyle(cell.style, rowStyle)
		localRune := cell.sourceRune - runeStart
		if different && localRune >= 0 && localRune < len(changed) && changed[localRune] {
			style = characterDiffStyle(rowStyle, right)
		}
		if localRune >= 0 && localRune < len(selected) && selected[localRune] {
			style = selectionStyle()
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
