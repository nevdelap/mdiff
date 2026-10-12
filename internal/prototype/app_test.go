package prototype

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/compare"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/display"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func TestSimulationEditingAlignmentScrollResize(t *testing.T) {
	configDir := t.TempDir()
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(48, 6)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	if app.left.buf.Type.Kind != buffer.BTDefault.Kind || app.right.buf.Type.Kind != buffer.BTDefault.Kind {
		t.Fatal("both panels must use Micro default buffers")
	}
	if app.left.buf == app.right.buf {
		t.Fatal("panels unexpectedly share one Micro buffer")
	}
	if app.left.buf.LinesNum() != 1 || app.right.buf.LinesNum() != 1 {
		t.Fatal("new panes are not empty")
	}
	if app.left.buf.Settings["ruler"] != true || app.right.buf.Settings["ruler"] != true {
		t.Fatalf("panes do not preserve Micro's default line-number setting: left=%v right=%v", app.left.buf.Settings["ruler"], app.right.buf.Settings["ruler"])
	}

	leftText := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\ntwelve"
	rightText := "one\ninserted\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\ntwelve"
	app.handle(tcell.NewEventPaste(leftText, ""))
	app.handle(tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModCtrl, ""))
	app.handle(tcell.NewEventPaste(rightText, ""))
	flushDiffForTest(t, app)
	if app.versions.Left != 1 || app.versions.Right != 1 {
		t.Fatalf("bracketed paste versions = %#v, want one edit per pane", app.versions)
	}
	if app.left.buf.Line(1) != "two" || app.right.buf.Line(1) != "inserted" {
		t.Fatalf("paste did not update independent buffers: left=%q right=%q", app.left.buf.Line(1), app.right.buf.Line(1))
	}
	if !hasSpacer(app.layout.Rows, viewport.Left) {
		t.Fatal("inserted line has no left-side virtual spacer")
	}
	if app.left.buf.Line(1) == "inserted" {
		t.Fatal("editing right pane changed left buffer")
	}
	app.focus = viewport.Right
	app.setFocus()
	app.handle(tcell.NewEventKey(tcell.KeyCtrlZ, 0, tcell.ModCtrl, ""))
	flushDiffForTest(t, app)
	if app.versions.Left != 1 || app.versions.Right != 2 {
		t.Fatalf("undo advanced unexpected versions: %#v", app.versions)
	}
	if app.right.buf.LinesNum() != 1 || app.right.buf.Line(0) != "" {
		t.Fatalf("Micro undo did not revert only the right pane: line count=%d line0=%q", app.right.buf.LinesNum(), app.right.buf.Line(0))
	}
	if app.left.buf.Line(1) != "two" {
		t.Fatal("undo in the right pane changed the left buffer")
	}
	app.handle(tcell.NewEventKey(tcell.KeyCtrlY, 0, tcell.ModCtrl, ""))
	flushDiffForTest(t, app)
	if app.versions.Left != 1 || app.versions.Right != 3 {
		t.Fatalf("redo advanced unexpected versions: %#v", app.versions)
	}
	if app.right.buf.Line(1) != "inserted" {
		t.Fatalf("Micro redo did not restore the right pane: line count=%d line1=%q", app.right.buf.LinesNum(), app.right.buf.Line(1))
	}

	for _, side := range []viewport.Side{viewport.Left, viewport.Right} {
		app.focus = side
		app.setFocus()
		app.focusedPane().h.GotoLoc(buffer.Loc{X: 0, Y: 0})
		app.top = 0
		app.handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModAlt, ""))
		if app.focusedPane().h.Cursor.Y != 1 || app.top != 1 {
			t.Fatalf("diff jump from side %d did not land on the first hunk: cursor=%v top=%d", side, app.focusedPane().h.Cursor.Loc, app.top)
		}
	}

	app.top = 2
	app.clampTop()
	before := app.top
	app.handle(tcell.NewEventMouse(4, 4, tcell.WheelDown, tcell.ModNone, ""))
	if app.top <= before {
		t.Fatalf("wheel event did not advance shared viewport: before=%d after=%d", before, app.top)
	}
	app.handle(tcell.NewEventMouse(35, 4, tcell.WheelUp, tcell.ModNone, ""))
	if app.top >= before+3 {
		t.Fatalf("wheel event over right panel did not scroll the shared viewport: %d", app.top)
	}

	app.focus = viewport.Left
	app.handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModCtrl, ""))
	if app.top == 0 {
		t.Fatal("keyboard scrolling did not change shared viewport")
	}
	app.focus = viewport.Right
	app.handle(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModCtrl, ""))

	if err := app.left.buf.SetOptionNative("softwrap", true); err != nil {
		t.Fatal(err)
	}
	if err := app.right.buf.SetOptionNative("softwrap", true); err != nil {
		t.Fatal(err)
	}
	app.handle(tcell.NewEventResize(24, 8))
	if app.cols != 24 || app.rows != 4 {
		t.Fatalf("resize dimensions are cols=%d rows=%d", app.cols, app.rows)
	}
	app.draw()

	// Insert above the visible anchor and verify the same logical line remains
	// at the top when it still exists uniquely in the edited side.
	app.focus = viewport.Left
	app.left.h.GotoLoc(buffer.Loc{X: 0, Y: 4})
	app.top = app.layout.RowFor(viewport.Left, 4)
	anchor := app.captureAnchor()
	if !anchor.valid {
		t.Fatal("expected a real logical row to anchor the viewport")
	}
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "added above\n")
	flushDiffForTest(t, app)
	cell := app.layout.Rows[app.top].Left
	if anchor.side == viewport.Right {
		cell = app.layout.Rows[app.top].Right
	}
	anchoredBuf := app.left.buf
	if anchor.side == viewport.Right {
		anchoredBuf = app.right.buf
	}
	if cell.Line != anchorLine(anchoredBuf, anchor) || cell.WrapRow != anchor.wrapRow {
		t.Fatalf("viewport anchor jumped after insertion above: top=%d anchor=%#v row=%#v", app.top, anchor, app.layout.Rows[app.top])
	}
	app.focus = viewport.Left
	app.setFocus()
	app.left.h.GotoLoc(buffer.Loc{X: 0, Y: 0})
	app.handle(tcell.NewEventKey(tcell.KeyRune, 'X', tcell.ModNone, ""))
	if app.left.buf.Line(0) != "Xadded above" || app.right.buf.Line(0) != "one" {
		t.Fatalf("Micro key editing did not stay in the focused buffer: left=%q right=%q", app.left.buf.Line(0), app.right.buf.Line(0))
	}
	app.handle(tcell.NewEventKey(tcell.KeyTAB, 0, tcell.ModNone, ""))
	if app.left.buf.Line(0) == "Xadded above" {
		t.Fatal("Tab no longer runs Micro's indentation action")
	}

	app.Close()
	assertNoUnexpectedPersistedState(t, configDir)
}

func flushDiffForTest(t *testing.T, app *App) {
	t.Helper()
	if !app.coordinator.Stale() {
		return
	}
	left, right := app.snapshots()
	app.coordinator.DebounceElapsed(diffSnapshot{left: left, right: right, versions: app.versions})
	select {
	case result := <-app.coordinator.Results():
		app.installDiff(result)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for comparison")
	}
}

func TestLargePasteStaysResponsiveAndInstallsFinalAlignment(t *testing.T) {
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(48, 8)
	app, err := New(sim, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	large := strings.Repeat("x", largePasteBytes)
	started := time.Now()
	app.handle(tcell.NewEventPaste(large, ""))
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("large paste blocked event handling for %s", elapsed)
	}
	if !app.isDiffStale() {
		t.Fatal("large paste did not mark the visible alignment stale")
	}
	app.draw()
	var footer strings.Builder
	for x := 0; x < app.cols; x++ {
		r, _, _, _ := sim.GetContent(x, app.statusRow)
		footer.WriteRune(r)
	}
	if strings.Contains(footer.String(), "Diff updating") {
		t.Fatalf("status row exposes the unhelpful stale-diff indicator: %q", footer.String())
	}
	if got := debounceDelay(app.coordinator.baseDelay, largePasteBytes, 0); got != app.coordinator.baseDelay+20*time.Millisecond {
		t.Fatalf("large paste delay = %s", got)
	}
	select {
	case <-app.coordinator.TimerC():
	case <-time.After(2 * time.Second):
		t.Fatal("large paste debounce did not expire")
	}
	left, right := app.snapshots()
	app.coordinator.DebounceElapsed(diffSnapshot{left: left, right: right, versions: app.versions})
	select {
	case result := <-app.coordinator.Results():
		app.installDiff(result)
	case <-time.After(2 * time.Second):
		t.Fatal("large paste comparison did not complete")
	}
	editStarted := time.Now()
	app.handle(tcell.NewEventKey(tcell.KeyRune, 'y', tcell.ModNone, ""))
	if elapsed := time.Since(editStarted); elapsed > time.Second {
		t.Fatalf("editing the large fixture blocked event handling for %s", elapsed)
	}
	flushDiffForTest(t, app)
	left, right = app.snapshots()
	if app.isDiffStale() || !reflect.DeepEqual(app.pairs, compare.Align(left, right)) {
		t.Fatal("alignment did not converge to the final buffer snapshots")
	}
}

func TestNavigationDoesNotAdvanceBufferVersions(t *testing.T) {
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	app, err := New(sim, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	before := app.versions
	app.handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone, ""))
	if app.versions != before || app.coordinator.Stale() {
		t.Fatalf("navigation scheduled a content comparison: versions=%#v stale=%t", app.versions, app.coordinator.Stale())
	}
}

func hasSpacer(rows []viewport.Row, side viewport.Side) bool {
	for _, row := range rows {
		cell := row.Left
		if side == viewport.Right {
			cell = row.Right
		}
		if cell.Spacer {
			return true
		}
	}
	return false
}

func anchorLine(b *buffer.Buffer, anchor anchor) int {
	occurrence := 0
	for line := 0; line < b.LinesNum(); line++ {
		if b.Line(line) == anchor.text {
			if occurrence == anchor.occurrence {
				return line
			}
			occurrence++
		}
	}
	return -1
}

func TestAlignedRowsUseMaximumWrapHeight(t *testing.T) {
	pairs := compare.Align([]string{"a", "long"}, []string{"a", "other"})
	layout := viewport.Expand(pairs, func(line int) int {
		if line == 1 {
			return 3
		}
		return 1
	}, func(line int) int {
		if line == 1 {
			return 2
		}
		return 1
	})
	if len(layout.Rows) != 4 {
		t.Fatalf("got %d aligned visual rows, want 4", len(layout.Rows))
	}
	if !layout.Rows[3].Right.Continuation {
		t.Fatalf("shorter wrapped side did not receive a continuation spacer: %#v", layout.Rows[3])
	}
}

func TestClickOnSpacerTargetsNearestLineAndFocusesPane(t *testing.T) {
	app, _ := newEditingTestApp(t, 48, 10)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "before\nafter")
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "before\ninserted\nafter")
	app.rebuildWithAnchor(anchor{})
	spacer := -1
	for row, aligned := range app.layout.Rows {
		if aligned.Left.Spacer {
			spacer = row
			break
		}
	}
	if spacer < 0 {
		t.Fatal("left side has no inserted-line spacer")
	}
	app.focus = viewport.Right
	app.handle(tcell.NewEventMouse(app.left.x+app.left.width-1, spacer, tcell.Button1, tcell.ModNone, ""))
	if app.focus != viewport.Left {
		t.Fatalf("spacer click focus = %v, want left pane", app.focus)
	}
	if app.left.h.Cursor.Y != 0 {
		t.Fatalf("spacer click cursor line = %d, want preceding line 0 on an equal-distance tie", app.left.h.Cursor.Y)
	}
}

func TestClickMapsPaneCoordinatesThroughGutterAndHorizontalOffset(t *testing.T) {
	app, _ := newEditingTestApp(t, 48, 10)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, strings.Repeat("x", 80))
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, strings.Repeat("y", 80))
	app.rebuildWithAnchor(anchor{})
	app.left.h.GotoLoc(buffer.Loc{X: 30, Y: 0})
	app.ensureCursorVisible()
	textStart := app.left.view.BufView().X
	x := textStart + 4
	app.handle(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone, ""))
	if app.focus != viewport.Left {
		t.Fatalf("click focus = %v, want left pane", app.focus)
	}
	if app.left.h.Cursor.Y != 0 || app.left.h.Cursor.X != app.left.view.LocFromVLoc(display.VLoc{
		SLoc: display.SLoc{Line: 0, Row: 0}, VisualX: app.left.view.StartCol + 4,
	}).X {
		t.Fatalf("click mapped to cursor %v at text start %d and horizontal offset %d", app.left.h.Cursor.Loc, textStart, app.left.view.StartCol)
	}
}

func TestDiffNavigationMovesBetweenDistinctHunksInBothDirections(t *testing.T) {
	app, _ := newEditingTestApp(t, 48, 10)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn\no")
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "a\nB\nc\nd\ne\nf\ng\nh\nI\nj\nk\nl\nm\nn\no")
	app.rebuildWithAnchor(anchor{})
	app.focus = viewport.Left
	app.setFocus()
	app.top = 0
	app.handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModAlt, ""))
	first := app.top
	app.handle(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModAlt, ""))
	second := app.top
	if first != 1 || second != 8 {
		t.Fatalf("next-difference viewport positions = %d, %d; want distinct hunks at 1, 8", first, second)
	}
	app.handle(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt, ""))
	if app.top != first {
		t.Fatalf("previous-difference viewport = %d, want first hunk at %d", app.top, first)
	}
	app.handle(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt, ""))
	if app.top != first {
		t.Fatalf("previous-difference at first hunk moved to %d, want stable position %d", app.top, first)
	}
}

func TestRepeatedLineAnchorUsesSurroundingContextAcrossInsertion(t *testing.T) {
	app, _ := newEditingTestApp(t, 48, 10)
	defer app.Close()
	contents := "head\np1\np2\np3\np4\nrepeat\nrepeat\ntail\na\nb\nc\nd"
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, contents)
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, contents)
	app.rebuildWithAnchor(anchor{})
	app.top = app.layout.RowFor(viewport.Left, 6)
	app.left.h.GotoLoc(buffer.Loc{X: 0, Y: 6})
	anchor := app.captureAnchor()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "prefix\n")
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "prefix\n")
	app.left.h.GotoLoc(buffer.Loc{X: 0, Y: 7})
	app.rebuildWithAnchor(anchor)
	cell := app.layout.CellAt(viewport.Left, app.top)
	if cell.Line != 7 || app.left.buf.Line(cell.Line) != "repeat" {
		t.Fatalf("repeated-line anchor resolved to row %#v at top=%d; want second repeat line 7", cell, app.top)
	}
}

func TestRepeatedBlockAnchorTracksOccurrenceWhenIdenticalBlockInsertedAbove(t *testing.T) {
	app, _ := newEditingTestApp(t, 48, 10)
	defer app.Close()
	block := []string{"a", "b", "c", "repeat", "d", "e", "f"}
	lines := append(append(append([]string(nil), block...), "gap-1", "gap-2", "gap-3"), block...)
	lines = append(lines, "tail-1", "tail-2", "tail-3", "tail-4", "tail-5", "tail-6", "tail-7", "tail-8")
	contents := strings.Join(lines, "\n")
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, contents)
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, contents)
	app.rebuildWithAnchor(anchor{})
	anchorLine := 13
	app.top = app.layout.RowFor(viewport.Left, anchorLine)
	app.left.h.GotoLoc(buffer.Loc{X: 0, Y: anchorLine})
	previous := app.captureAnchor()
	if previous.occurrence != 1 {
		t.Fatalf("captured repeat occurrence = %d, want 1", previous.occurrence)
	}
	if !reflect.DeepEqual(previous.before, []string{"c", "b", "a"}) || !reflect.DeepEqual(previous.after, []string{"d", "e", "f"}) {
		t.Fatalf("test fixture does not have identical three-line context: before=%v after=%v", previous.before, previous.after)
	}

	insertedBlock := strings.Join(block, "\n") + "\n"
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 10}, insertedBlock)
	app.rebuildWithAnchor(previous)
	cell := app.layout.CellAt(viewport.Left, app.top)
	if cell.Line != 20 || app.left.buf.Line(cell.Line) != "repeat" {
		t.Fatalf("identical-block anchor resolved to row %#v at top=%d; want original repeat at line 20", cell, app.top)
	}
}

func TestResizePreservesPaneLocalHorizontalScrollAndSoftwrapRebuildsRows(t *testing.T) {
	app, sim := newEditingTestApp(t, 48, 10)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, strings.Repeat("x", 80))
	app.left.h.GotoLoc(buffer.Loc{X: 70, Y: 0})
	app.ensureCursorVisible()
	if app.left.view.StartCol == 0 {
		t.Fatal("Micro did not scroll the unwrapped pane horizontally to its cursor")
	}
	leftStartCol := app.left.view.StartCol
	app.handle(tcell.NewEventResize(30, 8))
	if app.left.view.StartCol == 0 || app.left.view.StartCol < leftStartCol {
		t.Fatalf("resize lost left horizontal scroll: before=%d after=%d", leftStartCol, app.left.view.StartCol)
	}
	if err := app.left.buf.SetOptionNative("softwrap", true); err != nil {
		t.Fatal(err)
	}
	if app.left.view.StartCol != 0 {
		t.Fatalf("Micro softwrap callback did not reset horizontal offset: %d", app.left.view.StartCol)
	}
	app.ensureCursorVisible()
	if app.layoutKey != app.currentLayoutKey() {
		t.Fatal("softwrap setting change left the shared row layout stale")
	}
	if app.left.view.StartCol != 0 {
		t.Fatalf("softwrapped pane retained horizontal offset %d", app.left.view.StartCol)
	}
	if len(app.layout.Rows) <= 1 {
		t.Fatalf("softwrapped long line was not expanded into aligned visual rows: %d", len(app.layout.Rows))
	}
	sim.SetSize(30, 8)
}

func TestSubMinimumTerminalSizeKeepsCoordinatesClamped(t *testing.T) {
	app, sim := newEditingTestApp(t, 3, 2)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "left\nsecond")
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "right\nsecond")
	app.rebuildWithAnchor(anchor{})
	sim.SetSize(2, 1)
	app.handle(tcell.NewEventResize(2, 1))
	app.draw()
	if app.cols != 3 || app.rows != 2 {
		t.Fatalf("sub-minimum layout dimensions = %dx%d, want deterministic 3x2 logical minimum", app.cols, app.rows)
	}
	if app.top < 0 || app.top > max(0, len(app.layout.Rows)-app.rows) {
		t.Fatalf("shared visual offset %d escaped clamped range for %d rows", app.top, len(app.layout.Rows))
	}
}

func TestDrawAppliesFocusedCursorToTerminalScreen(t *testing.T) {
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(48, 10)
	app, err := New(sim, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	app.focus = viewport.Right
	app.setFocus()
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "cursor")
	app.right.h.GotoLoc(buffer.Loc{X: 3, Y: 0})
	app.draw()
	visual := app.right.view.VLocFromLoc(app.right.h.Cursor.Loc)
	assertSimulationCursor(t, sim, app.right.view.BufView().X+visual.VisualX-app.right.view.StartCol, 0, true)

	app.top = len(app.layout.Rows) // Deliberately place the cursor off-screen.
	app.draw()
	assertSimulationCursor(t, sim, -1, -1, false)

	sim.SetSize(60, 12)
	app.handle(tcell.NewEventResize(60, 12))
	app.draw()
	visual = app.right.view.VLocFromLoc(app.right.h.Cursor.Loc)
	row := app.layout.CursorRow(viewport.Right, app.right.h.Cursor.Y, visual.Row)
	assertSimulationCursor(t, sim, app.right.view.BufView().X+visual.VisualX-app.right.view.StartCol, row-app.top, true)
}

func assertSimulationCursor(t *testing.T, sim tcell.SimulationScreen, wantX, wantY int, wantVisible bool) {
	t.Helper()
	x, y, visible := sim.GetCursor()
	if x != wantX || y != wantY || visible != wantVisible {
		t.Fatalf("terminal cursor=(%d,%d,%t), want (%d,%d,%t)", x, y, visible, wantX, wantY, wantVisible)
	}
}
