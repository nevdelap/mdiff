package prototype

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/clipboard"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func TestMicroEditingActionsRemainPaneLocalAndUndoable(t *testing.T) {
	oldMethod := clipboard.CurrentMethod
	clipboard.CurrentMethod = clipboard.Internal
	defer func() { clipboard.CurrentMethod = oldMethod }()

	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()

	text := "alpha\nbeta\n"
	app.handle(tcell.NewEventPaste(text, ""))
	if got := string(app.left.buf.Bytes()); got != text {
		t.Fatalf("bracketed paste bytes = %q, want %q", got, text)
	}
	if app.versions.Left != 1 {
		t.Fatalf("one paste produced %d mutation versions", app.versions.Left)
	}
	app.handle(key(tcell.KeyCtrlZ, 0, tcell.ModCtrl))
	if got := string(app.left.buf.Bytes()); got != "" {
		t.Fatalf("one undo did not revert bracketed paste: %q", got)
	}
	app.handle(key(tcell.KeyCtrlY, 0, tcell.ModCtrl))
	if got := string(app.left.buf.Bytes()); got != text {
		t.Fatalf("one redo did not restore bracketed paste: %q", got)
	}

	app.handle(key(tcell.KeyCtrlA, 0, tcell.ModCtrl))
	if !app.left.h.Cursor.HasSelection() {
		t.Fatal("Ctrl+A did not select the focused buffer")
	}
	if app.right.h.Cursor.HasSelection() {
		t.Fatal("selection state leaked to the other pane")
	}
	app.handle(key(tcell.KeyCtrlC, 0, tcell.ModCtrl))
	if got, err := clipboard.Read(clipboard.ClipboardReg); err != nil || got != text {
		t.Fatalf("copied selection = %q, %v; want %q", got, err, text)
	}
	app.handle(key(tcell.KeyCtrlX, 0, tcell.ModCtrl))
	if got := string(app.left.buf.Bytes()); got != "" {
		t.Fatalf("cut selection left %q in the buffer", got)
	}
	if got := string(app.right.buf.Bytes()); got != "" {
		t.Fatalf("left pane action changed right buffer: %q", got)
	}
	app.handle(key(tcell.KeyCtrlV, 0, tcell.ModCtrl))
	if got := string(app.left.buf.Bytes()); got != text {
		t.Fatalf("Micro paste did not preserve newlines: %q", got)
	}
	app.handle(key(tcell.KeyCtrlZ, 0, tcell.ModCtrl))
	if got := string(app.left.buf.Bytes()); got != "" {
		t.Fatalf("one undo did not revert Micro paste as one operation: %q", got)
	}
	if app.right.buf.ContentVersion() != 0 {
		t.Fatal("editing actions advanced the other pane's version")
	}
	if app.right.buf.UndoStack.Len() != 0 {
		t.Fatal("left pane edits entered the other pane's undo history")
	}
}

func TestIndentationSearchAndSelectionStayWithFocusedBuffer(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()

	app.handle(tcell.NewEventPaste("alpha\nbeta alpha", ""))
	app.handle(key(tcell.KeyCtrlA, 0, tcell.ModCtrl))
	app.handle(key(tcell.KeyTAB, 0, tcell.ModNone))
	indent := string(app.left.buf.IndentString(4))
	if !strings.HasPrefix(app.left.buf.Line(0), indent) {
		t.Fatalf("Tab did not indent selected lines: %q", app.left.buf.Line(0))
	}
	app.handle(key(tcell.KeyBacktab, 0, tcell.ModNone))
	if strings.HasPrefix(app.left.buf.Line(0), indent) {
		t.Fatalf("Backtab did not outdent selected lines: %q", app.left.buf.Line(0))
	}
	app.left.h.GotoLoc(buffer.Loc{X: 0, Y: 0})
	app.left.h.Cursor.Deselect(true)
	app.handle(key(tcell.KeyCtrlF, 0, tcell.ModCtrl))
	for _, r := range "beta" {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	if actionSearch := app.left.buf.LastSearch; actionSearch != "beta" {
		t.Fatalf("left pane search state = %q, want beta", actionSearch)
	}
	if app.right.buf.LastSearch != "" || app.right.h.Cursor.HasSelection() {
		t.Fatal("search selection or search term leaked to the other pane")
	}
	if app.left.h.Cursor.Loc != (buffer.Loc{X: 4, Y: 1}) {
		t.Fatalf("search cursor = %#v, want end of second-line match", app.left.h.Cursor.Loc)
	}
	app.focus = viewport.Right
	app.setFocus()
	app.handle(key(tcell.KeyRune, 'R', tcell.ModNone))
	if string(app.right.buf.Bytes()) != "R" || !strings.HasPrefix(string(app.left.buf.Bytes()), "alpha\n") {
		t.Fatal("ordinary insertion did not stay in the focused pane")
	}
}

func TestDuplicateCutLineMovementAndFocusArePaneLocal(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 10)
	defer app.Close()

	app.handle(tcell.NewEventPaste("one\ntwo", ""))
	app.left.h.GotoLoc(buffer.Loc{X: 0, Y: 0})
	app.handle(key(tcell.KeyCtrlD, 0, tcell.ModCtrl))
	if app.left.buf.LinesNum() != 3 || app.left.buf.Line(0) != "one" || app.left.buf.Line(1) != "one" {
		t.Fatalf("Ctrl+D did not duplicate the current line: %#v", app.left.buf.Bytes())
	}
	app.left.h.GotoLoc(buffer.Loc{X: 0, Y: 0})
	app.handle(key(tcell.KeyCtrlK, 0, tcell.ModCtrl))
	if app.left.buf.LinesNum() != 2 || app.left.buf.Line(0) != "one" || app.left.buf.Line(1) != "two" {
		t.Fatalf("Ctrl+K did not cut the current line: %#v", app.left.buf.Bytes())
	}
	if app.right.buf.LinesNum() != 1 || app.right.buf.Line(0) != "" {
		t.Fatal("duplicate or cut line changed the unfocused buffer")
	}

	app.left.h.GotoLoc(buffer.Loc{X: 0, Y: 0})
	app.handle(key(tcell.KeyRight, 0, tcell.ModNone))
	if app.left.h.Cursor.X != 1 {
		t.Fatalf("right-arrow cursor location = %#v, want x=1", app.left.h.Cursor.Loc)
	}
	leftCursor := app.left.h.Cursor.Loc
	app.handle(key(tcell.KeyCtrlW, 0, tcell.ModCtrl))
	app.handle(key(tcell.KeyRune, 'R', tcell.ModNone))
	if app.left.h.Cursor.Loc != leftCursor || string(app.right.buf.Bytes()) != "R" {
		t.Fatal("focus switch moved the inactive cursor or edited the wrong buffer")
	}
}

func TestAutocompleteUsesTheFocusedBufferWhenSuggestionsExist(t *testing.T) {
	app, _ := newEditingTestApp(t, 48, 8)
	defer app.Close()

	app.handle(tcell.NewEventPaste("alphabet\nal", ""))
	app.left.h.GotoLoc(buffer.Loc{X: 2, Y: 1})
	app.handle(key(tcell.KeyTAB, 0, tcell.ModNone))
	if got := app.left.buf.Line(1); got != "alphabet" {
		t.Fatalf("Micro autocomplete = %q, want alphabet", got)
	}
	if app.right.buf.Line(0) != "" {
		t.Fatal("autocomplete read or changed the other pane")
	}
}

func TestShortcutBarHelpCommandAndRejectedActions(t *testing.T) {
	app, sim := newEditingTestApp(t, 120, 9)
	defer app.Close()

	app.draw()
	row1 := screenLine(sim, app.menuRow1, app.cols)
	row2 := screenLine(sim, app.menuRow2, app.cols)
	wantRow1 := "^W Switch Panes, ^Q Quit, ^S Save, ^O Open, ^G Help, ^E Command Bar, ^K Cut Line"
	wantRow2 := "^F Find, ^Z Undo, ^Y Redo, ^A Select All, ^D Duplicate Line, ^T New Tab"
	if !strings.HasPrefix(row1, wantRow1) || !strings.HasPrefix(row2, wantRow2) {
		t.Fatalf("Micro menu order/separators changed: row1=%q row2=%q", row1, row2)
	}
	for _, label := range []string{"^W Switch Panes", "^Q Quit", "^S Save", "^O Open", "^G Help", "^E Command Bar", "^K Cut Line"} {
		if !strings.Contains(row1, label) {
			t.Errorf("menu row %q does not contain %q", row1, label)
		}
	}
	for _, label := range []string{"^F Find", "^Z Undo", "^Y Redo", "^A Select All", "^D Duplicate Line", "^T New Tab"} {
		if !strings.Contains(row2, label) {
			t.Errorf("menu row %q does not contain %q", row2, label)
		}
	}
	if !strings.HasSuffix(strings.TrimRight(row1, " "), "Left") || !strings.HasSuffix(strings.TrimRight(row2, " "), "mdiff (wrapping micro) v0.0.1") {
		t.Fatalf("right-aligned menu indicators missing: row1=%q row2=%q", row1, row2)
	}
	status := screenLine(sim, app.statusRow, app.cols)
	for _, legend := range []string{"F6 switch", "Ctrl-Up/Down", "Alt-Up/Down differences"} {
		if strings.Contains(status, legend) {
			t.Errorf("status row retains permanent control legend %q: %q", legend, status)
		}
	}
	if app.statusRow <= app.rows || app.menuRow1 != app.statusRow+1 || app.menuRow2 != app.menuRow1+1 {
		t.Fatalf("editor, status, and menu rows overlap: content=%d status=%d menu=(%d,%d)", app.rows, app.statusRow, app.menuRow1, app.menuRow2)
	}
	leftStatus := screenLine(sim, app.left.statusView.Y, app.left.width)
	rightStatus := screenLine(sim, app.right.statusView.Y, app.right.width)
	for side, line := range []string{leftStatus, rightStatus} {
		for _, field := range []string{"No name", "(1,1)", "ft:unknown", "unix", "utf-8"} {
			if !strings.Contains(strings.ToLower(line), strings.ToLower(field)) {
				t.Errorf("pane %d Micro status %q does not contain %q", side, line, field)
			}
		}
	}

	app.handle(key(tcell.KeyCtrlG, 0, tcell.ModCtrl))
	if actionMessage := app.currentStatus(); !strings.Contains(actionMessage, "mdiff:") {
		t.Fatalf("Ctrl+G did not show help: %q", actionMessage)
	}
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "memory")
	before := string(app.left.buf.Bytes())
	rightName := app.right.buf.GetName()
	savePath := filepath.Join(t.TempDir(), "must-not-exist")
	app.handle(key(tcell.KeyCtrlS, 0, tcell.ModCtrl))
	if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "Filename:") {
		t.Fatalf("Ctrl+S did not ask for an unnamed buffer's filename: %q", app.currentStatus())
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	app.handle(key(tcell.KeyCtrlO, 0, tcell.ModCtrl))
	if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "open ") {
		t.Fatalf("Ctrl+O did not prompt for a path: %q", app.currentStatus())
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	for _, rejected := range []struct {
		key  tcell.Key
		mods tcell.ModMask
		want string
	}{
		{tcell.KeyF2, tcell.ModNone, "Save is unavailable"},
		{tcell.KeyCtrlB, tcell.ModCtrl, "ShellMode is unavailable"},
	} {
		app.handle(key(rejected.key, 0, rejected.mods))
		if !strings.Contains(app.currentStatus(), rejected.want) {
			t.Errorf("rejected key %v did not report %q: %q", rejected.key, rejected.want, app.currentStatus())
		}
	}
	if got := string(app.left.buf.Bytes()); got != before {
		t.Fatalf("rejected file/shell action changed buffer: %q", got)
	}
	oldRight := app.right.buf
	app.handle(key(tcell.KeyCtrlT, 0, tcell.ModCtrl))
	app.handle(key(tcell.KeyRune, 'n', tcell.ModNone))
	if app.right.buf != oldRight || app.left.buf == oldRight || app.left.buf.GetName() != "No name" || app.left.buf.LinesNum() != 1 {
		t.Fatal("Ctrl+T did not replace only the focused pane with a new unnamed buffer")
	}
	commandBaseline := string(app.left.buf.Bytes())
	app.handle(key(tcell.KeyCtrlE, 0, tcell.ModCtrl))
	if action.InfoBar == nil || !action.InfoBar.HasPrompt {
		t.Fatal("Ctrl+E did not open the command bar")
	}
	_, promptY, promptCursorVisible := app.cursorPosition()
	if promptY != app.statusRow || !promptCursorVisible {
		t.Fatalf("command prompt cursor=(y=%d, visible=%t), want visible on status row %d", promptY, promptCursorVisible, app.statusRow)
	}
	for _, r := range "save " + savePath {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	if got := string(app.left.buf.Bytes()); got != commandBaseline {
		t.Fatalf("rejected save changed buffer: %q, before %q", got, commandBaseline)
	}
	if _, err := os.Stat(savePath); !os.IsNotExist(err) {
		t.Fatalf("rejected save created a file, stat error=%v", err)
	}
	if app.left.buf.GetName() != "No name" || app.right.buf.GetName() != rightName {
		t.Fatalf("new document did not preserve the opposite pane: left=%q right=%q", app.left.buf.GetName(), app.right.buf.GetName())
	}
	if !strings.Contains(app.currentStatus(), "Command unavailable") {
		t.Fatalf("unsafe command was not rejected visibly: %q", app.currentStatus())
	}
	for _, unsafe := range []string{"open " + savePath, "run touch " + savePath} {
		app.handle(key(tcell.KeyCtrlE, 0, tcell.ModCtrl))
		for _, r := range unsafe {
			app.handle(key(tcell.KeyRune, r, tcell.ModNone))
		}
		app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
		if !strings.Contains(app.currentStatus(), "Command unavailable") {
			t.Fatalf("unsafe command %q was not rejected visibly: %q", unsafe, app.currentStatus())
		}
	}
	if _, err := os.Stat(savePath); !os.IsNotExist(err) {
		t.Fatalf("rejected open/external command created a file, stat error=%v", err)
	}

	narrow, narrowScreen := newEditingTestApp(t, 14, 5)
	defer narrow.Close()
	narrow.draw()
	narrowRow1 := screenLine(narrowScreen, narrow.menuRow1, narrow.cols)
	narrowRow2 := screenLine(narrowScreen, narrow.menuRow2, narrow.cols)
	if len([]rune(narrowRow1)) != narrow.cols || len([]rune(narrowRow2)) != narrow.cols || narrow.menuRow1 == narrow.menuRow2 {
		t.Fatalf("narrow menu wrapped or collided: row1=%q row2=%q", narrowRow1, narrowRow2)
	}
}

func TestRejectedSequencePrefixesOnReservedKeysAreBlocked(t *testing.T) {
	for _, tc := range []struct {
		key    tcell.Key
		config string
		want   string
	}{
		{tcell.KeyCtrlQ, `"<Ctrl-q><Ctrl-s>":"Save"`, "Save is unavailable"},
		{tcell.KeyCtrlG, `"<Ctrl-g><Ctrl-s>":"Save"`, "Save is unavailable"},
		{tcell.KeyCtrlE, `"<Ctrl-e><Ctrl-s>":"Save"`, "Save is unavailable"},
	} {
		t.Run(tc.want+tc.config[:8], func(t *testing.T) {
			configDir := t.TempDir()
			bindings := `{"buffer":{` + tc.config + `}}`
			if err := os.WriteFile(filepath.Join(configDir, "bindings.json"), []byte(bindings), 0o600); err != nil {
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
			defer app.Close()
			if quit := app.handle(key(tc.key, 0, tcell.ModCtrl)); quit {
				t.Fatal("rejected sequence prefix triggered mdiff quit")
			}
			if action.InfoBar != nil && action.InfoBar.HasPrompt {
				t.Fatal("rejected sequence prefix triggered an mdiff prompt")
			}
			if !strings.Contains(app.currentStatus(), tc.want) {
				t.Fatalf("prefix status = %q, want %q", app.currentStatus(), tc.want)
			}
		})
	}
}

func TestReservedKeysRejectConfiguredUnsafeActions(t *testing.T) {
	configDir := t.TempDir()
	bindings := `{"buffer":{"Ctrl-q":"Save","Ctrl-g":"OpenFile","Ctrl-e":"ShellMode"}}`
	if err := os.WriteFile(filepath.Join(configDir, "bindings.json"), []byte(bindings), 0o600); err != nil {
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
	defer app.Close()

	for _, tc := range []struct {
		key  tcell.Key
		want string
	}{
		{tcell.KeyCtrlQ, "Save is unavailable"},
		{tcell.KeyCtrlG, "OpenFile is unavailable"},
		{tcell.KeyCtrlE, "ShellMode is unavailable"},
	} {
		if quit := app.handle(key(tc.key, 0, tcell.ModCtrl)); quit {
			t.Errorf("configured unsafe action on key %v caused mdiff to quit", tc.key)
		}
		if status := app.currentStatus(); !strings.Contains(status, tc.want) {
			t.Errorf("configured action on key %v status = %q, want %q", tc.key, status, tc.want)
		}
		if action.InfoBar != nil && action.InfoBar.HasPrompt {
			t.Errorf("configured action on key %v opened an app prompt", tc.key)
		}
	}
}

func TestDefaultCtrlQStillQuits(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 10)
	defer app.Close()
	if quit := app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl)); !quit {
		t.Fatal("default Ctrl+Q did not quit")
	}
}

func TestConfiguredUnsafeActionIsFilteredWithExplanation(t *testing.T) {
	configDir := t.TempDir()
	bindings := []byte(`{"buffer":{"Ctrl-r":"Save","<Ctrl-x><Ctrl-s>":"Save"}}`)
	if err := os.WriteFile(filepath.Join(configDir, "bindings.json"), bindings, 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(48, 8)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	ruler := app.left.buf.Settings["ruler"]
	app.handle(tcell.NewEventPaste("safe", ""))
	before := string(app.left.buf.Bytes())
	app.handle(key(tcell.KeyCtrlR, 0, tcell.ModCtrl))
	if app.left.buf.Settings["ruler"] != ruler {
		t.Fatal("configured Save binding ran the built-in ruler action")
	}
	if !strings.Contains(app.currentStatus(), "Save is unavailable") {
		t.Fatalf("configured unsafe action was not explained: %q", app.currentStatus())
	}
	if app.closed {
		t.Fatal("rejected configured action terminated the application")
	}
	app.handle(key(tcell.KeyCtrlX, 0, tcell.ModCtrl))
	if app.left.h.Cursor.HasSelection() {
		t.Fatal("blocked key-sequence prefix ran Micro's single-key cut action")
	}
	app.handle(key(tcell.KeyCtrlS, 0, tcell.ModCtrl))
	if string(app.left.buf.Bytes()) != before || !strings.Contains(app.currentStatus(), "Save is unavailable") {
		t.Fatalf("configured unsafe key sequence changed content or lacked explanation: content=%q status=%q", app.left.buf.Bytes(), app.currentStatus())
	}
}

func newEditingTestApp(t *testing.T, width, height int) (*App, tcell.SimulationScreen) {
	t.Helper()
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(width, height)
	app, err := New(sim, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return app, sim
}

func key(keyCode tcell.Key, r rune, modifiers tcell.ModMask) *tcell.EventKey {
	return tcell.NewEventKey(keyCode, r, modifiers, "")
}

func screenLine(screen tcell.SimulationScreen, y, width int) string {
	var line strings.Builder
	for x := 0; x < width; x++ {
		r, _, _, _ := screen.GetContent(x, y)
		line.WriteRune(r)
	}
	return line.String()
}

func (a *App) currentStatus() string {
	if action.InfoBar == nil {
		return ""
	}
	if action.InfoBar.HasPrompt {
		return action.InfoBar.Msg + string(action.InfoBar.LineBytes(0))
	}
	return action.InfoBar.Msg
}
