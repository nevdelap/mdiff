package prototype

import (
	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/compare"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/viewport"
)

type renderedCell struct {
	rune  rune
	combc []rune
	style tcell.Style
	width int
}

func (a *App) renderPane(p *pane, layout viewport.Layout, top, height int) {
	cache := make(map[int][][]renderedCell)
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
		lines, ok := cache[cell.Line]
		if !ok {
			lines = a.renderMicroLine(p, cell.Line)
			cache[cell.Line] = lines
		}
		if cell.WrapRow < 0 || cell.WrapRow >= len(lines) {
			continue
		}
		copyLine(p, screenY, lines[cell.WrapRow], base, different)
	}
}

// renderMicroLine asks Micro's BufWindow renderer to paint one logical line
// into the offscreen tcell surface. The caller maps those rendered soft-wrap
// rows onto the shared diff-aware visual row sequence.
func (a *App) renderMicroLine(p *pane, line int) [][]renderedCell {
	wraps := a.wrapHeight(p, line)
	view := p.view
	x, y, width, height := view.X, view.Y, view.Width, view.Height
	startLine, startCol, active := view.StartLine, view.StartCol, view.IsActive()
	view.X, view.Y, view.Height = 0, 0, wraps+1
	view.StartLine = display.SLoc{Line: line}
	view.SetActive(false)
	compositionScreen := screen.Screen
	a.paintScreen.Clear()
	screen.Screen = a.paintScreen
	view.Display()

	rows := make([][]renderedCell, wraps)
	for row := 0; row < wraps; row++ {
		rows[row] = make([]renderedCell, p.width)
		for col := 0; col < p.width; col++ {
			r, combc, style, cellWidth := a.paintScreen.GetContent(col, row)
			rows[row][col] = renderedCell{rune: r, combc: append([]rune(nil), combc...), style: style, width: cellWidth}
		}
	}
	screen.Screen = compositionScreen

	view.X, view.Y, view.Width, view.Height = x, y, width, height
	view.StartLine, view.StartCol = startLine, startCol
	view.Resize(width, height)
	view.StartLine, view.StartCol = startLine, startCol
	view.SetActive(active)
	return rows
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

func clearRow(p *pane, screenY int, style tcell.Style) {
	for x := 0; x < p.width; x++ {
		screen.SetContent(p.x+x, screenY, ' ', nil, style)
	}
}

func copyLine(p *pane, screenY int, cells []renderedCell, rowStyle tcell.Style, different bool) {
	_, rowBackground, _ := rowStyle.Decompose()
	for x, cell := range cells {
		if cell.width == 0 {
			continue
		}
		style := cell.style
		if different {
			style = style.Background(rowBackground)
		}
		screen.SetContent(p.x+x, screenY, cell.rune, cell.combc, style)
	}
}
