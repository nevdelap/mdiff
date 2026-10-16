package prototype

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/compare"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func TestRenderCharacterDiffAndStylePrecedence(t *testing.T) {
	app, sim := newRenderTestApp(t)
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
	assertCellStyle(t, app.surface, rightTextX+1, 0, tcell.ColorGreen, tcell.NewHexColor(0x000046), 0)

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
	if attrs&tcell.AttrReverse != 0 {
		t.Fatalf("focused cursor painted a block over Micro's cursor: attrs=%b", attrs)
	}
	if x, y, visible := sim.GetCursor(); !visible || x != leftTextX+1 || y != 0 {
		t.Fatalf("Micro cursor location not carried to terminal: (%d,%d) visible=%t", x, y, visible)
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

func TestConfiguredScrollbarAndReplacementBuffer(t *testing.T) {
	app, sim := newRenderTestApp(t)
	defer app.Close()
	content := strings.Repeat("row\n", 40)
	app.left.buf.Insert(buffer.Loc{}, content)
	app.right.buf.Insert(buffer.Loc{}, content)
	app.left.buf.Settings["scrollbar"] = true
	app.right.buf.Settings["scrollbar"] = true
	config.GlobalSettings["scrollbarchar"] = "#"
	app.rebuildWithAnchor(anchor{})
	app.top = 15
	app.draw()
	for _, p := range app.panes() {
		found := false
		for y := 0; y < app.rows; y++ {
			r, _, _, _ := sim.GetContent(p.x+p.width-1, y)
			found = found || r == '#'
		}
		if !found {
			t.Fatalf("configured scrollbar missing in pane at x=%d", p.x)
		}
	}
	replacement := buffer.NewBufferFromString("replacement\ntext", "", buffer.BTDefault)
	replacement.Settings["scrollbar"] = true
	app.left.h.OpenBuffer(replacement)
	app.syncChangedBuffers()
	if app.left.buf.Settings["scrollbar"] != true {
		t.Fatal("file replacement disabled the configured scrollbar")
	}
}

func TestFakeCursorUsesOneTerminalCellAndNoPhysicalCursor(t *testing.T) {
	app, sim := newRenderTestApp(t)
	defer app.Close()
	previous := config.GlobalSettings["fakecursor"]
	config.GlobalSettings["fakecursor"] = true
	t.Cleanup(func() { config.GlobalSettings["fakecursor"] = previous })
	app.left.buf.Insert(buffer.Loc{}, "cat")
	app.left.h.Cursor.GotoLoc(buffer.Loc{X: 1})
	app.rebuildWithAnchor(anchor{})
	app.draw()
	x := app.left.view.BufView().X + 1
	if _, _, visible := sim.GetCursor(); visible {
		t.Fatal("physical cursor visible with fakecursor enabled")
	}
	_, _, surfaceAttrs := cellStyle(app.surface, x, 0)
	_, _, terminalAttrs := cellStyle(sim, x, 0)
	if surfaceAttrs&tcell.AttrReverse != 0 || terminalAttrs&tcell.AttrReverse == 0 {
		t.Fatalf("fake cursor should appear only on terminal: surface=%b terminal=%b", surfaceAttrs, terminalAttrs)
	}
	app.draw()
	_, _, terminalAttrs = cellStyle(sim, x, 0)
	if terminalAttrs&tcell.AttrReverse == 0 {
		t.Fatal("fake cursor disappeared after redraw")
	}
}

func TestPromptCursorUsesVisualColumnsAndClipsMixedWidthInput(t *testing.T) {
	app, sim := newEditingTestApp(t, 30, 10)
	defer app.Close()
	app.openCommandBar()
	for _, r := range "猫" {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.draw()
	if x, y, visible := sim.GetCursor(); !visible || x != 4 || y != app.statusRow {
		t.Fatalf("wide prompt cursor = (%d,%d) visible=%t, want (4,%d)", x, y, visible, app.statusRow)
	}
	for _, r := range "ab界cdefghijklmnopqrstuvwxyz" {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.draw()
	x, y, visible := sim.GetCursor()
	if !visible || x < 0 || x >= app.cols || y != app.statusRow {
		t.Fatalf("clipped prompt cursor = (%d,%d) visible=%t", x, y, visible)
	}
	if got := string(action.InfoBar.LineBytes(0)); got != "猫ab界cdefghijklmnopqrstuvwxyz" {
		t.Fatalf("prompt input changed during clipping: %q", got)
	}
	if got := screenLine(sim, app.statusRow, app.cols); !strings.Contains(got, "uvwxyz") {
		t.Fatalf("clipped prompt lost its visible tail: %q", got)
	}
}

func TestTwoRowTerminalShowsPromptAndRestoresEditorRows(t *testing.T) {
	app, sim := newEditingTestApp(t, 30, 2)
	defer app.Close()
	if app.rows != 2 || app.statusRow != -1 {
		t.Fatalf("initial two-row layout = content %d, status %d", app.rows, app.statusRow)
	}
	app.openCommandBar()
	app.handle(key(tcell.KeyRune, 'x', tcell.ModNone))
	app.draw()
	if app.rows != 1 || app.statusRow != 1 {
		t.Fatalf("prompt layout = content %d, status %d; want 1 and 1", app.rows, app.statusRow)
	}
	if got := screenLine(sim, 1, app.cols); !strings.HasPrefix(got, "> x") {
		t.Fatalf("two-row prompt not painted: %q", got)
	}
	if x, y, visible := sim.GetCursor(); !visible || x != 3 || y != 1 {
		t.Fatalf("two-row prompt cursor = (%d,%d) visible=%t", x, y, visible)
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	app.draw()
	if app.rows != 2 || app.statusRow != -1 {
		t.Fatalf("editor rows not restored: content %d, status %d", app.rows, app.statusRow)
	}
}

func TestStatusLineUsesVisualColumns(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	previous := screen.Screen
	screen.Screen = app.surface
	defer func() { screen.Screen = previous }()
	app.renderLine(app.statusRow, "猫e\u0301a\tZ", config.DefStyle)
	for _, tc := range []struct {
		x int
		r rune
	}{
		{0, '猫'}, {2, 'e'}, {3, 'a'}, {4, ' '}, {7, ' '}, {8, 'Z'},
	} {
		r, _, _, _ := app.surface.GetContent(tc.x, app.statusRow)
		if r != tc.r {
			t.Fatalf("status cell %d = %q, want %q", tc.x, r, tc.r)
		}
	}
	_, combc, _, _ := app.surface.GetContent(2, app.statusRow)
	if !reflect.DeepEqual(combc, []rune{'\u0301'}) {
		t.Fatalf("combining mark lost: %q", string(combc))
	}
}

func TestMenuLineAlignsWideTextByVisualColumns(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	app.cols = 12
	previous := screen.Screen
	screen.Screen = app.surface
	defer func() { screen.Screen = previous }()
	app.renderMenuLine(0, "猫A", "界B", config.DefStyle)
	for _, tc := range []struct {
		x int
		r rune
	}{
		{0, '猫'}, {2, 'A'}, {9, '界'}, {11, 'B'},
	} {
		r, _, _, _ := app.surface.GetContent(tc.x, 0)
		if r != tc.r {
			t.Fatalf("menu cell %d = %q, want %q", tc.x, r, tc.r)
		}
	}
}

func TestFocusedLineKeepsMicroLineNumberAndApprovedDiffPalette(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	withColorscheme(t, map[string]tcell.Style{
		"line-number":         tcell.StyleDefault.Foreground(tcell.ColorYellow),
		"current-line-number": tcell.StyleDefault.Foreground(tcell.ColorWhite),
		"cursor-line":         tcell.StyleDefault.Foreground(tcell.ColorGray),
	})
	app.left.buf.Insert(buffer.Loc{}, "same\nleft\nstill same")
	app.right.buf.Insert(buffer.Loc{}, "same\nright\nstill same")
	app.rebuildWithAnchor(anchor{})
	app.left.h.Cursor.GotoLoc(buffer.Loc{Y: 0})
	app.draw()
	leftTextX := app.left.view.BufView().X
	assertCellStyle(t, app.surface, 0, 0, tcell.ColorWhite, tcell.ColorGray, 0)
	_, bg, _ := cellStyle(app.surface, leftTextX, 0)
	if bg != tcell.ColorGray {
		t.Fatalf("equal current line background = %v, want grey", bg)
	}
	_, bg, _ = cellStyle(app.surface, leftTextX, 1)
	if bg != tcell.NewHexColor(0x000046) {
		t.Fatalf("noncurrent changed line background = %v, want half-brightness blue", bg)
	}
	_, bg, _ = cellStyle(app.surface, leftTextX, 2)
	_, normalBG, _ := config.DefStyle.Decompose()
	if bg != normalBG {
		t.Fatalf("noncurrent equal line background = %v, want Micro default %v", bg, normalBG)
	}
	app.left.h.Cursor.GotoLoc(buffer.Loc{Y: 1})
	app.draw()
	assertCellStyle(t, app.surface, 0, 1, tcell.ColorWhite, tcell.ColorDarkBlue, 0)
	_, bg, _ = cellStyle(app.surface, leftTextX, 1)
	if bg != tcell.ColorDarkBlue {
		t.Fatalf("changed current line background = %v, want approved dark blue", bg)
	}
	for _, tc := range []struct {
		kind       compare.Kind
		right      bool
		noncurrent tcell.Color
		current    tcell.Color
	}{
		{compare.Changed, false, tcell.NewHexColor(0x000046), tcell.ColorDarkBlue},
		{compare.Spacer, false, tcell.NewHexColor(0x460000), tcell.ColorDarkRed},
		{compare.Spacer, true, tcell.NewHexColor(0x003200), tcell.ColorDarkGreen},
	} {
		_, dark, _ := diffStyle(tc.kind, tc.right, false).Decompose()
		_, light, _ := diffStyle(tc.kind, tc.right, true).Decompose()
		if dark != tc.noncurrent || light != tc.current {
			t.Fatalf("diff colors = (%v,%v), want (%v,%v)", dark, light, tc.noncurrent, tc.current)
		}
	}
}

func TestMicroColorschemeOverridesDiffBackgroundsWithoutReplacingForegrounds(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	const theme = `color-link mdiff.diff.changed.current ",#102030"
color-link mdiff.diff.changed ",#405060"
color-link mdiff.diff.left-only.current ",#701010"
color-link mdiff.diff.left-only ",#401010"
color-link mdiff.diff.right-only.current ",#107010"
color-link mdiff.diff.right-only ",#104010"`
	colors, err := config.ParseColorscheme("mdiff-test", theme, nil)
	if err != nil {
		t.Fatal(err)
	}
	colors["diff-deleted"] = tcell.StyleDefault.Foreground(tcell.ColorYellow)
	colors["current-line-number"] = tcell.StyleDefault.Foreground(tcell.ColorWhite)
	colors["cursor-line"] = tcell.StyleDefault.Foreground(tcell.ColorGray)
	withColorscheme(t, colors)
	for _, tc := range []struct {
		kind    compare.Kind
		right   bool
		current bool
		want    string
	}{
		{compare.Changed, false, true, "#102030"},
		{compare.Changed, false, false, "#405060"},
		{compare.Spacer, false, true, "#701010"},
		{compare.Spacer, false, false, "#401010"},
		{compare.Spacer, true, true, "#107010"},
		{compare.Spacer, true, false, "#104010"},
	} {
		_, bg, _ := diffStyle(tc.kind, tc.right, tc.current).Decompose()
		if bg != tcell.GetColor(tc.want) {
			t.Fatalf("theme background for %+v = %v, want %s", tc, bg, tc.want)
		}
	}
	app.left.buf.Insert(buffer.Loc{}, "cat\nsame")
	app.right.buf.Insert(buffer.Loc{}, "cut\nsame")
	app.left.h.Cursor.GotoLoc(buffer.Loc{})
	app.rebuildWithAnchor(anchor{})
	app.draw()
	textX := app.left.view.BufView().X
	assertCellStyle(t, app.surface, textX+1, 0, tcell.ColorYellow, tcell.GetColor("#102030"), 0)
	fg, bg, _ := cellStyle(app.surface, 0, 0)
	if fg != tcell.ColorWhite || bg != tcell.GetColor("#102030") {
		t.Fatalf("current line number style = (%v,%v), want white on theme blue", fg, bg)
	}
	_, bg, _ = cellStyle(app.surface, textX, 1)
	_, normalBG, _ := config.DefStyle.Decompose()
	if bg != normalBG {
		t.Fatalf("equal noncurrent row background = %v, want Micro default %v", bg, normalBG)
	}
	app.left.h.Cursor.GotoLoc(buffer.Loc{Y: 1})
	app.draw()
	_, bg, _ = cellStyle(app.surface, textX, 1)
	if bg != tcell.ColorGray {
		t.Fatalf("equal current row background = %v, want Micro cursor-line grey", bg)
	}
}

func TestBundledLightThemeKeepsEqualRowsAndReadableDiffBackgrounds(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"colorscheme":"dukelight-tc"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 10)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	if _, ok := config.Colorscheme["mdiff.diff.changed"]; ok {
		t.Fatal("bundled light theme unexpectedly defines mdiff row colors")
	}
	app.left.buf.Insert(buffer.Loc{}, "same\ncat\nleft only\nend")
	app.right.buf.Insert(buffer.Loc{}, "same\ncut\nend")
	app.left.h.Cursor.GotoLoc(buffer.Loc{Y: 1})
	app.rebuildWithAnchor(anchor{})
	app.draw()
	leftX := app.left.view.BufView().X
	rightX := app.right.view.BufView().X
	const base = 0xf0f0f0
	if marker, _, _, _ := app.surface.GetContent(app.left.width, 1); marker != '│' {
		t.Fatalf("known-background theme divider = %q, want ordinary divider", marker)
	}
	assertCellStyle(t, app.surface, leftX, 0, tcell.GetColor("#000000"), tcell.NewHexColor(base), 0)
	assertCellStyle(t, app.surface, rightX, 0, tcell.GetColor("#000000"), tcell.NewHexColor(base), 0)
	_, equalBG, _ := cellStyle(app.surface, leftX, 3)
	if equalBG != tcell.NewHexColor(base) {
		t.Fatalf("equal row background = %v, want Micro default", equalBG)
	}
	assertCellStyle(t, app.surface, 0, 1, tcell.GetColor("#004080"), lightDiffBackground(t, compare.Changed, false, true), tcell.AttrBold)
	assertCellStyle(t, app.surface, leftX+1, 1, tcell.GetColor("#500000"), tcell.NewHexColor(base), 0)
	assertCellStyle(t, app.surface, rightX+1, 1, tcell.GetColor("#008040"), tcell.NewHexColor(base), 0)
	if len(app.layout.Rows) <= 2 || app.layout.Rows[2].Kind != compare.Spacer || !app.layout.Rows[2].Right.Spacer {
		t.Fatalf("fixture did not produce a right spacer: %#v", app.layout.Rows)
	}
	_, leftOnlyBG, _ := cellStyle(app.surface, leftX+20, 2)
	_, rightSpacerBG, _ := cellStyle(app.surface, rightX, 2)
	if leftOnlyBG != lightDiffBackground(t, compare.Spacer, false, false) ||
		rightSpacerBG != lightDiffBackground(t, compare.Spacer, true, false) {
		t.Fatalf("spacer backgrounds = (%v,%v), want readable light red/green", leftOnlyBG, rightSpacerBG)
	}
	app.left.h.Cursor.GotoLoc(buffer.Loc{Y: 0})
	app.draw()
	assertCellStyle(t, app.surface, leftX, 0, tcell.GetColor("#000000"), tcell.GetColor("#e6e6e6"), 0)
	config.Colorscheme["mdiff.diff.changed"] = tcell.StyleDefault.Background(tcell.GetColor("#abcdef"))
	_, overridden, _ := diffStyle(compare.Changed, false, false).Decompose()
	if overridden != tcell.GetColor("#abcdef") {
		t.Fatalf("explicit light-theme override = %v, want #abcdef", overridden)
	}
}

func TestBundledTerminalDefaultThemeKeepsTerminalBackground(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"colorscheme":"simple"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 10)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	_, terminalBG, _ := config.DefStyle.Decompose()
	if terminalBG.Hex() >= 0 {
		t.Fatalf("simple theme background = %v, want terminal default", terminalBG)
	}
	if _, ok := config.Colorscheme["mdiff.diff.changed"]; ok {
		t.Fatal("bundled simple theme unexpectedly defines mdiff row colors")
	}
	app.left.buf.Insert(buffer.Loc{}, "same\ncat\nleft only\nend")
	app.right.buf.Insert(buffer.Loc{}, "same\ncut\nend")
	app.left.h.Cursor.GotoLoc(buffer.Loc{Y: 1})
	app.rebuildWithAnchor(anchor{})
	app.draw()
	leftX := app.left.view.BufView().X
	rightX := app.right.view.BufView().X
	for _, tc := range []struct {
		x, y int
		name string
	}{
		{leftX, 0, "left equal"}, {rightX, 0, "right equal"},
		{leftX + 20, 1, "left current changed"}, {rightX + 20, 1, "right noncurrent changed"},
		{leftX + 20, 2, "left-only"}, {rightX, 2, "right spacer"},
	} {
		_, bg, _ := cellStyle(app.surface, tc.x, tc.y)
		if bg != terminalBG {
			t.Fatalf("%s background = %v, want terminal default %v", tc.name, bg, terminalBG)
		}
	}
	assertCellStyle(t, app.surface, leftX, 0, tcell.ColorDefault, terminalBG, 0)
	deletedFG, _, _ := config.Colorscheme["diff-deleted"].Decompose()
	addedFG, _, _ := config.Colorscheme["diff-added"].Decompose()
	assertCellStyle(t, app.surface, leftX+1, 1, deletedFG, terminalBG, 0)
	assertCellStyle(t, app.surface, rightX+1, 1, addedFG, terminalBG, 0)
	fg, bg, _ := cellStyle(app.surface, 0, 1)
	lineNumberFG, _, _ := config.Colorscheme["current-line-number"].Decompose()
	if fg != lineNumberFG || bg != terminalBG {
		t.Fatalf("current line number style = (%v,%v), want Micro style on terminal background", fg, bg)
	}
	config.Colorscheme["mdiff.diff.changed.current"] = tcell.StyleDefault.Background(tcell.GetColor("#123456"))
	config.Colorscheme["mdiff.diff.right-only"] = tcell.StyleDefault.Background(tcell.GetColor("#234567"))
	_, changedBG, _ := diffStyle(compare.Changed, false, true).Decompose()
	_, spacerBG, _ := diffStyle(compare.Spacer, true, false).Decompose()
	if changedBG != tcell.GetColor("#123456") || spacerBG != tcell.GetColor("#234567") {
		t.Fatalf("terminal-default theme overrides = (%v,%v), want explicit colors", changedBG, spacerBG)
	}
}

func TestTerminalDefaultDividerIdentifiesCoarseChangesAndSpacers(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"colorscheme":"simple"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 10)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	app.left.buf.Insert(buffer.Loc{}, "same\nold\nleft only\nend")
	app.right.buf.Insert(buffer.Loc{}, "same\nnew\nright only\nend")
	app.rebuildWithAnchor(anchor{})
	// A budget-expired comparison can pair changed lines without rune spans.
	// Include both spacer directions to check the terminal-default cue.
	app.pairs = []compare.Pair{
		{Left: 0, Right: 0, Kind: compare.Equal},
		{Left: 1, Right: 1, Kind: compare.Changed},
		{Left: 2, Right: -1, Kind: compare.Spacer},
		{Left: -1, Right: 2, Kind: compare.Spacer},
		{Left: 3, Right: 3, Kind: compare.Equal},
	}
	app.expandWithAnchor(anchor{})
	app.draw()
	dividerX := app.left.width
	for _, tc := range []struct {
		row    int
		marker rune
		group  string
	}{
		{0, '│', ""}, {1, '!', "diff-modified"},
		{2, '<', "diff-deleted"}, {3, '>', "diff-added"},
		{4, '│', ""},
	} {
		got, _, style, _ := app.surface.GetContent(dividerX, tc.row)
		fg, bg, _ := style.Decompose()
		_, terminalBG, _ := config.DefStyle.Decompose()
		if got != tc.marker || bg != terminalBG {
			t.Fatalf("divider row %d = (%q,%v), want (%q, terminal background)", tc.row, got, bg, tc.marker)
		}
		if tc.group != "" {
			wantFG, _, _ := config.Colorscheme[tc.group].Decompose()
			if fg != wantFG {
				t.Fatalf("divider row %d foreground = %v, want Micro %s %v", tc.row, fg, tc.group, wantFG)
			}
		}
	}
	leftX, rightX := app.left.view.BufView().X, app.right.view.BufView().X
	_, terminalBG, _ := config.DefStyle.Decompose()
	for _, x := range []int{leftX, rightX} {
		fg, bg, _ := cellStyle(app.surface, x, 1)
		if fg != tcell.ColorDefault || bg != terminalBG {
			t.Fatalf("coarse changed text style = (%v,%v), want Micro default", fg, bg)
		}
	}
	for _, tc := range []struct{ x, y int }{{leftX + 20, 2}, {rightX, 2}, {leftX, 3}, {rightX + 20, 3}} {
		_, bg, _ := cellStyle(app.surface, tc.x, tc.y)
		if bg != terminalBG {
			t.Fatalf("spacer row background at (%d,%d) = %v, want terminal default", tc.x, tc.y, bg)
		}
	}
}

func lightDiffBackground(t *testing.T, kind compare.Kind, right, current bool) tcell.Color {
	t.Helper()
	_, bg, _ := diffStyle(kind, right, current).Decompose()
	rgb := bg.Hex()
	r, g, b := (rgb>>16)&0xff, (rgb>>8)&0xff, rgb&0xff
	if (299*r+587*g+114*b)/1000 < 160 || bg == tcell.GetColor("#f0f0f0") {
		t.Fatalf("light-theme diff background = %v, want distinct light color", bg)
	}
	return bg
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
	view.SetActive(active)
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

func TestRuneBoundariesMatchMicroCharacterLocations(t *testing.T) {
	for _, tc := range []struct {
		name, line     string
		width          int
		wantCharacters []int
	}{
		{"tabs wide whitespace", "aa\t猫abcdefgh xyz界", 12, nil},
		{"multiple combining marks", "e\u0301\u0327猫\tz", 12, []int{0, 1, 1, 1, 2, 3, 4}},
		{"mark at narrow wrap", "ab e\u0301\u0327猫 xyz", 8, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, sim := newRenderTestApp(t)
			defer app.Close()
			sim.SetSize(tc.width, 8)
			app.resize(tc.width, 8)
			app.left.buf.Insert(buffer.Loc{}, tc.line)
			for _, softwrap := range []bool{false, true} {
				for _, wordwrap := range []bool{false, true} {
					if err := app.left.buf.SetOptionNative("softwrap", softwrap); err != nil {
						t.Fatal(err)
					}
					if err := app.left.buf.SetOptionNative("wordwrap", wordwrap); err != nil {
						t.Fatal(err)
					}
					mapped := app.left.view.RuneBoundaries(0)
					if len(mapped.Visual) != len([]rune(tc.line))+1 || len(mapped.Character) != len(mapped.Visual) {
						t.Fatalf("boundary lengths = %d/%d, want %d", len(mapped.Visual), len(mapped.Character), len([]rune(tc.line))+1)
					}
					if tc.wantCharacters != nil && !reflect.DeepEqual(mapped.Character, tc.wantCharacters) {
						t.Fatalf("rune-to-character boundaries = %v, want %v", mapped.Character, tc.wantCharacters)
					}
					for offset, char := range mapped.Character {
						want := app.left.view.VLocFromLoc(buffer.Loc{X: char, Y: 0})
						if mapped.Visual[offset] != want {
							t.Fatalf("softwrap=%t wordwrap=%t rune=%d char=%d: got %#v, Micro gives %#v", softwrap, wordwrap, offset, char, mapped.Visual[offset], want)
						}
					}
					if got, want := app.left.view.LineEndVLoc(0), mapped.Visual[len(mapped.Visual)-1]; got != want {
						t.Fatalf("line end = %#v, batch gives %#v", got, want)
					}
				}
			}
		})
	}
}

func TestSelectionAfterCombiningClusterUsesCharacterOffset(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "e\u0301\u0327X")
	app.left.h.Cursor.CurSelection = [2]buffer.Loc{{X: 1}, {X: 2}}
	mapped := app.left.view.RuneBoundaries(0)
	if got, want := selectionRuneMask(&app.left, 0, []rune("e\u0301\u0327X"), mapped.Character), []bool{false, false, false, true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selection mask = %v, want %v", got, want)
	}
}

func TestMarkOnlyChangeHighlightsBaseCell(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "e\u0301Z")
	app.right.buf.Insert(buffer.Loc{}, "e\u0300Z")
	app.rebuildWithAnchor(anchor{})
	withColorscheme(t, map[string]tcell.Style{"diff-deleted": tcell.StyleDefault.Foreground(tcell.ColorRed)})
	app.draw()
	textX := app.left.view.BufView().X
	assertCellStyle(t, app.surface, textX, 0, tcell.ColorRed, tcell.ColorDarkBlue, 0)
	_, _, attrs := cellStyle(app.surface, textX+1, 0)
	if attrs&tcell.AttrReverse != 0 {
		t.Fatal("unchanged cell after combining cluster received change style")
	}
}

func TestChangedTextSpanningVisualWrap(t *testing.T) {
	app, sim := newRenderTestApp(t)
	defer app.Close()
	sim.SetSize(30, 8)
	app.resize(30, 8)
	for _, p := range []*pane{&app.left, &app.right} {
		if err := p.buf.SetOptionNative("softwrap", true); err != nil {
			t.Fatal(err)
		}
		if err := p.buf.SetOptionNative("wordwrap", false); err != nil {
			t.Fatal(err)
		}
	}
	width := app.left.view.BufView().Width
	left := strings.Repeat("a", width-1) + "bc"
	right := strings.Repeat("a", width-1) + "XY"
	app.left.buf.Insert(buffer.Loc{}, left)
	app.right.buf.Insert(buffer.Loc{}, right)
	app.rebuildWithAnchor(anchor{})
	mapped := app.left.view.RuneBoundaries(0)
	if mapped.Visual[width-1].Row == mapped.Visual[width].Row {
		t.Fatalf("changed characters do not span wrap: %v", mapped.Visual)
	}
	withColorscheme(t, map[string]tcell.Style{"diff-deleted": tcell.StyleDefault.Foreground(tcell.ColorRed)})
	app.draw()
	textX := app.left.view.BufView().X
	assertCellStyle(t, app.surface, textX+mapped.Visual[width-1].VisualX, mapped.Visual[width-1].Row, tcell.ColorRed, tcell.ColorDarkBlue, 0)
	assertCellStyle(t, app.surface, textX+mapped.Visual[width].VisualX, mapped.Visual[width].Row, tcell.ColorRed, tcell.ColorDarkBlue, 0)
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
	got := baseCellStyle(syntax, row, true, true)
	fg, bg, attrs := got.Decompose()
	if fg != tcell.ColorAqua || bg != tcell.ColorPurple || attrs&tcell.AttrItalic == 0 {
		t.Fatalf("highlight/hunk composition = (%v,%v,%b), want Micro highlight bg, syntax fg, italic", fg, bg, attrs)
	}

	withColorscheme(t, map[string]tcell.Style{
		"diff-deleted": tcell.StyleDefault.Foreground(tcell.ColorRed).Bold(true),
	})
	got = characterDiffStyle(got, row, false, true)
	fg, bg, attrs = got.Decompose()
	if fg != tcell.ColorRed || bg != tcell.ColorPurple || attrs&tcell.AttrBold == 0 || attrs&tcell.AttrItalic != 0 {
		t.Fatalf("character diff composition = (%v,%v,%b), want configured diff fg with Micro highlight bg", fg, bg, attrs)
	}

	config.Colorscheme = map[string]tcell.Style{}
	got = characterDiffStyle(tcell.StyleDefault.Background(tcell.ColorDarkBlue), row, true, false)
	fg, bg, _ = got.Decompose()
	if fg != tcell.ColorGreen || bg != tcell.ColorDarkBlue {
		t.Fatalf("missing added style fallback = (%v,%v), want green on hunk background", fg, bg)
	}
}

func TestEqualRowCopyPreservesMicroCellStyle(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	microCell := tcell.StyleDefault.Foreground(tcell.ColorAqua).Background(tcell.ColorPurple).Bold(true)
	rowStyle := tcell.StyleDefault.Background(tcell.ColorDarkBlue)
	cell := renderedCell{rune: 'x', width: 1, sourceRune: 0, style: microCell}
	previous := screen.Screen
	screen.Screen = app.surface
	defer func() { screen.Screen = previous }()
	copyLine(&app.left, 0, []renderedCell{cell}, rowStyle, viewport.Row{Kind: compare.Equal}, false, false, 0, nil, nil)
	_, _, got, _ := app.surface.GetContent(app.left.x, 0)
	fg, bg, attrs := got.Decompose()
	if fg != tcell.ColorAqua || bg != tcell.ColorPurple || attrs&tcell.AttrBold == 0 {
		t.Fatalf("equal row replaced Micro's rendered cell style: (%v,%v,%b)", fg, bg, attrs)
	}
}

func TestEqualRowSearchBackgroundSurvivesCursorLineColorCollision(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	withColorscheme(t, map[string]tcell.Style{
		"hlsearch":    tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorPurple),
		"cursor-line": tcell.StyleDefault.Foreground(tcell.ColorPurple),
	})
	contents := "preceding line\nneedle on equal row"
	app.left.buf.Insert(buffer.Loc{}, contents)
	app.right.buf.Insert(buffer.Loc{}, contents)
	app.left.buf.LastSearch = "needle"
	app.left.buf.HighlightSearch = true
	app.left.h.GotoLoc(buffer.Loc{})
	app.rebuildWithAnchor(anchor{})
	app.draw()
	textX := app.left.view.BufView().X
	_, bg, _ := cellStyle(app.surface, textX, 1)
	if bg != tcell.ColorPurple {
		t.Fatalf("search background matching cursor-line color was replaced: got %v, want purple", bg)
	}
}

func TestChangedRowSearchBackgroundSurvivesCursorLineColorCollision(t *testing.T) {
	app, _ := newRenderTestApp(t)
	defer app.Close()
	withColorscheme(t, map[string]tcell.Style{
		"hlsearch":    tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorPurple),
		"cursor-line": tcell.StyleDefault.Foreground(tcell.ColorPurple),
	})
	app.left.buf.Insert(buffer.Loc{}, "needle on changed row\ncommon")
	app.right.buf.Insert(buffer.Loc{}, "other on changed row\ncommon")
	app.left.buf.LastSearch = "needle"
	app.left.buf.HighlightSearch = true
	app.left.h.GotoLoc(buffer.Loc{})
	app.rebuildWithAnchor(anchor{})
	app.draw()
	textX := app.left.view.BufView().X
	_, bg, _ := cellStyle(app.surface, textX, 0)
	if bg != tcell.ColorPurple {
		t.Fatalf("changed-row search background matching cursor-line color was replaced: got %v, want purple", bg)
	}
}

func newRenderTestApp(t testing.TB) (*App, tcell.SimulationScreen) {
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

func TestUnwrappedHorizontalScrollHighlightsChangedLongLine(t *testing.T) {
	app, sim := newRenderTestApp(t)
	defer app.Close()
	sim.SetSize(40, 8)
	app.resize(40, 8)
	index := 5000
	left := strings.Repeat("a", index) + "X" + strings.Repeat("a", 5000)
	right := strings.Repeat("a", index) + "Y" + strings.Repeat("a", 5000)
	app.left.buf.Insert(buffer.Loc{}, left)
	app.right.buf.Insert(buffer.Loc{}, right)
	app.rebuildWithAnchor(anchor{})
	app.left.view.StartCol = index - 3
	withColorscheme(t, map[string]tcell.Style{"diff-deleted": tcell.StyleDefault.Foreground(tcell.ColorRed)})
	app.draw()
	textX := app.left.view.BufView().X
	assertCellStyle(t, app.surface, textX+3, 0, tcell.ColorRed, tcell.ColorDarkBlue, 0)
}

func BenchmarkRenderUnwrappedLongLineAnnotations(b *testing.B) {
	app, _ := newRenderTestApp(b)
	index := 1 << 19
	left := strings.Repeat("a", index) + "X" + strings.Repeat("a", index)
	right := strings.Repeat("a", index) + "Y" + strings.Repeat("a", index)
	app.left.buf.Insert(buffer.Loc{}, left)
	app.right.buf.Insert(buffer.Loc{}, right)
	app.rebuildWithAnchor(anchor{})
	app.left.view.StartCol = index - 20
	app.right.view.StartCol = index - 20
	b.ReportAllocs()
	b.SetBytes(int64(len(left) + len(right)))
	b.ResetTimer()
	for range b.N {
		app.draw()
	}
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
