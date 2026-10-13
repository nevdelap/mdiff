package prototype

import (
	"reflect"
	"strings"
	"testing"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func TestRenderCharacterDiffAndStylePrecedence(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "cat")
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "cut")
	app.focus = viewport.Left
	app.setFocus()
	app.rebuildWithAnchor(anchor{})

	colors := map[string]tcell.Style{
		"diff-added":   tcell.StyleDefault.Foreground(tcell.ColorGreen),
		"diff-deleted": tcell.StyleDefault.Foreground(tcell.ColorRed),
		"selection":    tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorBlue),
	}
	withColorscheme(t, colors)

	app.draw()
	leftTextX := app.left.view.BufView().X
	rightTextX := app.right.view.BufView().X
	assertCellStyle(t, app.surface, leftTextX+1, 0, tcell.ColorRed, tcell.ColorDarkBlue, 0)
	assertCellStyle(t, app.surface, rightTextX+1, 0, tcell.ColorGreen, tcell.ColorDarkBlue, 0)

	app.left.h.Cursor.CurSelection = [2]buffer.Loc{{X: 1, Y: 0}, {X: 2, Y: 0}}
	app.draw()
	assertCellStyle(t, app.surface, leftTextX+1, 0, tcell.ColorYellow, tcell.ColorBlue, 0)

	app.left.h.Cursor.ResetSelection()
	app.left.h.Cursor.GotoLoc(buffer.Loc{X: 1, Y: 0})
	leftCursor := app.left.h.Cursor.Loc
	leftSelection := app.left.h.Cursor.CurSelection
	leftUndo, leftRedo := app.left.buf.UndoStack.Len(), app.left.buf.RedoStack.Len()
	rightCursor := app.right.h.Cursor.Loc
	rightSelection := app.right.h.Cursor.CurSelection
	rightUndo, rightRedo := app.right.buf.UndoStack.Len(), app.right.buf.RedoStack.Len()
	app.draw()
	_, _, attrs := cellStyle(app.surface, leftTextX+1, 0)
	if attrs&tcell.AttrReverse == 0 {
		t.Fatalf("focused cursor did not override changed-character style: attrs=%b", attrs)
	}
	if got := app.left.buf.Line(0); got != "cat" {
		t.Fatalf("rendering changed the left buffer: %q", got)
	}
	if got := app.right.buf.Line(0); got != "cut" {
		t.Fatalf("rendering changed the right buffer: %q", got)
	}
	if app.left.h.Cursor.Loc != leftCursor || app.left.h.Cursor.CurSelection != leftSelection ||
		app.left.buf.UndoStack.Len() != leftUndo || app.left.buf.RedoStack.Len() != leftRedo {
		t.Fatal("rendering changed the left cursor, selection, or undo/redo state")
	}
	if app.right.h.Cursor.Loc != rightCursor || app.right.h.Cursor.CurSelection != rightSelection ||
		app.right.buf.UndoStack.Len() != rightUndo || app.right.buf.RedoStack.Len() != rightRedo {
		t.Fatal("rendering changed the right cursor, selection, or undo/redo state")
	}
}

func TestRenderCharacterDiffMapsTabsWideRunesAndCombiningMarks(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	left, right := "A\t界e\u0301Z", "A\t猫e\u0300Z"
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, left)
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, right)
	app.focus = viewport.Left
	app.setFocus()
	app.rebuildWithAnchor(anchor{})
	withColorscheme(t, map[string]tcell.Style{
		"diff-added":   tcell.StyleDefault.Foreground(tcell.ColorGreen),
		"diff-deleted": tcell.StyleDefault.Foreground(tcell.ColorRed),
	})

	leftCells := app.renderMicroLine(&app.left, 0, true)
	textStart := app.left.view.BufView().X - app.left.x
	if got := leftCells[0][textStart+1].sourceRune; got != 1 {
		t.Fatalf("first tab cell maps to rune %d, want tab rune 1", got)
	}
	if got := leftCells[0][textStart+3].sourceRune; got != 1 {
		t.Fatalf("tab expansion cell maps to rune %d, want tab rune 1", got)
	}
	if got := leftCells[0][textStart+4].sourceRune; got != 2 {
		t.Fatalf("wide rune maps to rune %d, want rune 2", got)
	}
	if got := leftCells[0][textStart+6].sourceRune; got != 3 {
		t.Fatalf("combining sequence base maps to rune %d, want rune 3", got)
	}

	app.draw()
	assertCellStyle(t, app.surface, textStart+4, 0, tcell.ColorRed, tcell.ColorDarkBlue, 0)
	assertCellStyle(t, app.surface, textStart+5, 0, tcell.ColorRed, tcell.ColorDarkBlue, 0)
	assertCellStyle(t, app.surface, textStart+6, 0, tcell.ColorRed, tcell.ColorDarkBlue, 0)
	_, _, tabAttrs := cellStyle(app.surface, textStart+1, 0)
	if tabAttrs&tcell.AttrReverse != 0 {
		t.Fatal("unchanged tab received a change style")
	}
}

func TestVisibleWrappedRowRendererMatchesMicroViewportRows(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	text := strings.Repeat("wrapped words ", 12)
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, text)
	if err := app.left.buf.SetOptionNative("softwrap", true); err != nil {
		t.Fatal(err)
	}
	p := &app.left
	wraps := app.wrapHeight(p, 0)
	if wraps < 2 {
		t.Fatalf("fixture has only %d wrapped rows", wraps)
	}

	view := p.view
	x, y, width, height := view.X, view.Y, view.Width, view.Height
	startLine, startCol, active := view.StartLine, view.StartCol, view.IsActive()
	view.X, view.Y, view.Height = 0, 0, wraps+1
	view.StartLine = display.SLoc{Line: 0}
	view.SetActive(false)
	previous := screen.Screen
	app.paintScreen.Clear()
	screen.Screen = app.paintScreen
	view.Display()
	fullRows := make([][]renderedCell, wraps)
	for row := range fullRows {
		fullRows[row] = make([]renderedCell, p.width)
		for col := range fullRows[row] {
			r, combc, style, cellWidth := app.paintScreen.GetContent(col, row)
			fullRows[row][col] = renderedCell{rune: r, combc: append([]rune(nil), combc...), style: style, width: cellWidth, sourceRune: -1}
		}
	}
	screen.Screen = previous
	view.X, view.Y, view.Width, view.Height = x, y, width, height
	view.StartLine, view.StartCol = startLine, startCol
	view.Resize(width, height)
	view.StartLine, view.StartCol = startLine, startCol
	view.SetActive(active)

	for row := 0; row < wraps; row++ {
		if got := app.renderMicroRow(p, 0, row, false); !reflect.DeepEqual(got, fullRows[row]) {
			t.Fatalf("rendered wrapped row %d differs from Micro's full-line render", row)
		}
	}
}

func TestSourceRuneLocationsMatchMicroAcrossWrappingModes(t *testing.T) {
	app, sim := newRenderTestApp(t)
	defer app.Close()
	sim.SetSize(12, 8)
	app.resize(12, 8)
	line := "aa\t猫abcdefgh xyz界"
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, line)
	runes := []rune(line)
	for _, softwrap := range []bool{false, true} {
		for _, wordwrap := range []bool{false, true} {
			if err := app.left.buf.SetOptionNative("softwrap", softwrap); err != nil {
				t.Fatal(err)
			}
			if err := app.left.buf.SetOptionNative("wordwrap", wordwrap); err != nil {
				t.Fatal(err)
			}
			locations := sourceRuneLocations(&app.left, 0, runes)
			for offset := range locations {
				want := app.left.view.VLocFromLoc(buffer.Loc{X: offset, Y: 0})
				if locations[offset] != want {
					t.Fatalf("softwrap=%t wordwrap=%t offset=%d: got %#v, Micro gives %#v", softwrap, wordwrap, offset, locations[offset], want)
				}
			}
		}
	}
}

func TestVirtualSpacerRowsRenderNoSourceText(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "before\nafter")
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "before\ninserted\nafter")
	app.rebuildWithAnchor(anchor{})
	app.draw()

	spacerRow := -1
	for i, row := range app.layout.Rows {
		if row.Left.Spacer {
			spacerRow = i
			break
		}
	}
	if spacerRow < 0 {
		t.Fatal("left side has no virtual spacer for the inserted line")
	}
	for x := 0; x < app.left.width; x++ {
		r, _, _, _ := app.surface.GetContent(x, spacerRow)
		if r != ' ' {
			t.Fatalf("left virtual spacer rendered source text %q at x=%d", r, x)
		}
	}
	if got := app.left.buf.Line(1); got != "after" {
		t.Fatalf("virtual spacer modified left source buffer: line 1 = %q", got)
	}
}

func TestDiffStylePreservesSyntaxForegroundAndCompositionRules(t *testing.T) {
	syntax := tcell.StyleDefault.Foreground(tcell.ColorAqua).Background(tcell.ColorPurple).Italic(true)
	row := tcell.StyleDefault.Background(tcell.ColorDarkBlue)
	got := baseCellStyle(syntax, row)
	fg, bg, attrs := got.Decompose()
	if fg != tcell.ColorAqua || bg != tcell.ColorDarkBlue || attrs&tcell.AttrItalic == 0 {
		t.Fatalf("base/hunk composition = (%v,%v,%b), want syntax fg, hunk bg, italic", fg, bg, attrs)
	}

	withColorscheme(t, map[string]tcell.Style{
		"diff-deleted": tcell.StyleDefault.Foreground(tcell.ColorRed).Bold(true),
	})
	got = characterDiffStyle(row, false)
	fg, bg, attrs = got.Decompose()
	if fg != tcell.ColorRed || bg != tcell.ColorDarkBlue || attrs&tcell.AttrBold == 0 || attrs&tcell.AttrItalic != 0 {
		t.Fatalf("character diff composition = (%v,%v,%b), want configured style with inherited hunk bg", fg, bg, attrs)
	}

	config.Colorscheme = map[string]tcell.Style{}
	got = characterDiffStyle(row, true)
	fg, bg, _ = got.Decompose()
	if fg != tcell.ColorGreen || bg != tcell.ColorDarkBlue {
		t.Fatalf("missing added style fallback = (%v,%v), want green on hunk background", fg, bg)
	}
}

func newRenderTestApp(t *testing.T) (*App, tcell.SimulationScreen) {
	t.Helper()
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 10)
	app, err := New(sim, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return app, sim
}

func withColorscheme(t *testing.T, colors map[string]tcell.Style) {
	t.Helper()
	previous := config.Colorscheme
	config.Colorscheme = colors
	t.Cleanup(func() { config.Colorscheme = previous })
}

func assertCellStyle(t *testing.T, screen tcell.SimulationScreen, x, y int, wantFG, wantBG tcell.Color, wantAttrs tcell.AttrMask) {
	t.Helper()
	fg, bg, attrs := cellStyle(screen, x, y)
	if fg != wantFG || bg != wantBG || attrs != wantAttrs {
		t.Fatalf("cell (%d,%d) style=(%v,%v,%b), want (%v,%v,%b)", x, y, fg, bg, attrs, wantFG, wantBG, wantAttrs)
	}
}

func cellStyle(screen tcell.SimulationScreen, x, y int) (tcell.Color, tcell.Color, tcell.AttrMask) {
	_, _, style, _ := screen.GetContent(x, y)
	return style.Decompose()
}
