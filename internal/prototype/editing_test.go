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

func TestTerminalCursorIncludesMicroGutterOffset(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 10)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "abcdef")
	app.left.h.GotoLoc(buffer.Loc{X: 3, Y: 0})
	app.expandWithAnchor(anchor{}, []string{"abcdef"}, []string{""})
	visual := app.left.view.VLocFromLoc(app.left.h.Cursor.Loc)
	wantX := app.left.view.BufView().X + visual.VisualX - app.left.view.StartCol
	if app.left.view.BufView().X <= app.left.x {
		t.Fatalf("test pane has no Micro gutter: pane x=%d content x=%d", app.left.x, app.left.view.BufView().X)
	}
	gotX, _, visible := app.cursorPosition()
	if !visible || gotX != wantX {
		t.Fatalf("cursor x=%d visible=%t, want content coordinate %d including gutter", gotX, visible, wantX)
	}

	if err := app.left.buf.SetOptionNative("softwrap", true); err != nil {
		t.Fatal(err)
	}
	app.refreshLayoutIfNeeded()
	visual = app.left.view.VLocFromLoc(app.left.h.Cursor.Loc)
	wantX = app.left.view.BufView().X + visual.VisualX
	gotX, _, visible = app.cursorPosition()
	if !visible || gotX != wantX {
		t.Fatalf("softwrapped cursor x=%d visible=%t, want content coordinate %d including gutter", gotX, visible, wantX)
	}
}

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
	wantRow2 := "^F Find, ^Z Undo, ^Y Redo, ^A Select All, ^D Duplicate Line, ^T New File"
	if !strings.HasPrefix(row1, wantRow1) || !strings.HasPrefix(row2, wantRow2) {
		t.Fatalf("Micro menu order/separators changed: row1=%q row2=%q", row1, row2)
	}
	for _, label := range []string{"^W Switch Panes", "^Q Quit", "^S Save", "^O Open", "^G Help", "^E Command Bar", "^K Cut Line"} {
		if !strings.Contains(row1, label) {
			t.Errorf("menu row %q does not contain %q", row1, label)
		}
	}
	for _, label := range []string{"^F Find", "^Z Undo", "^Y Redo", "^A Select All", "^D Duplicate Line", "^T New File"} {
		if !strings.Contains(row2, label) {
			t.Errorf("menu row %q does not contain %q", row2, label)
		}
	}
	if !strings.HasSuffix(strings.TrimRight(row1, " "), "Left") || !strings.HasSuffix(strings.TrimRight(row2, " "), "mdiff (wrapping micro) v0.0.2") {
		t.Fatalf("right-aligned menu indicators missing: row1=%q row2=%q", row1, row2)
	}
	app.handle(key(tcell.KeyCtrlW, 0, tcell.ModCtrl))
	app.draw()
	if row := strings.TrimRight(screenLine(sim, app.menuRow1, app.cols), " "); !strings.HasSuffix(row, "Right") {
		t.Fatalf("right pane focus indicator is not at the end of the shortcut row: %q", row)
	}
	if row := strings.TrimSpace(screenLine(sim, app.statusRow, app.cols)); row == "Left" || row == "Right" {
		t.Fatalf("right pane focus indicator is duplicated on the status row: %q", row)
	}
	app.handle(key(tcell.KeyCtrlW, 0, tcell.ModCtrl))
	app.draw()
	status := screenLine(sim, app.statusRow, app.cols)
	if strings.TrimSpace(status) == "Left" || strings.TrimSpace(status) == "Right" {
		t.Fatalf("focus indicator is duplicated on the status row: %q", status)
	}
	for _, legend := range []string{"F6 switch", "Ctrl-Up/Down", "Alt-Up/Down differences"} {
		if strings.Contains(status, legend) {
			t.Errorf("status row retains permanent control legend %q: %q", legend, status)
		}
	}
	if app.menuRow1 != app.rows+1 || app.menuRow2 != app.menuRow1+1 || app.statusRow != app.menuRow2+1 {
		t.Fatalf("Micro bottom-row order changed: content=%d menu=(%d,%d) infobar=%d", app.rows, app.menuRow1, app.menuRow2, app.statusRow)
	}
	leftStatus := screenLine(sim, app.left.statusView.Y, app.left.width)
	rightStatus := screenLine(sim, app.right.statusView.Y, app.right.width)
	separator, _, _, _ := sim.GetContent(app.left.width, app.rows)
	if separator != '│' {
		t.Fatalf("pane status separator = %q, want vertical divider", separator)
	}
	for side, line := range []string{leftStatus, rightStatus} {
		for _, field := range []string{"No name", "(1,1)", "ft:unknown", "unix", "utf-8"} {
			if !strings.Contains(strings.ToLower(line), strings.ToLower(field)) {
				t.Errorf("pane %d Micro status %q does not contain %q", side, line, field)
			}
		}
	}

	app.handle(key(tcell.KeyCtrlG, 0, tcell.ModCtrl))
	if !app.helpVisible || app.helpTopic != "help" {
		t.Fatalf("Ctrl+G did not show Micro help: visible=%t topic=%q", app.helpVisible, app.helpTopic)
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
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
		{tcell.KeyCtrlB, tcell.ModCtrl, "ShellMode is unavailable"},
	} {
		app.handle(key(rejected.key, 0, rejected.mods))
		if !strings.Contains(app.currentStatus(), rejected.want) {
			t.Errorf("rejected key %v did not report %q: %q", rejected.key, rejected.want, app.currentStatus())
		}
	}
	app.handle(key(tcell.KeyF2, 0, tcell.ModNone))
	if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "Filename:") {
		t.Fatalf("F2 did not route Micro save: %q", app.currentStatus())
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
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
		t.Fatalf("save command changed buffer: %q, before %q", got, commandBaseline)
	}
	if got, err := os.ReadFile(savePath); err != nil || string(got) != commandBaseline {
		t.Fatalf("save command wrote %q, %v; want %q", got, err, commandBaseline)
	}
	if app.left.buf.Path != savePath || app.right.buf.GetName() != rightName {
		t.Fatalf("new document did not preserve the opposite pane: left=%q right=%q", app.left.buf.GetName(), app.right.buf.GetName())
	}
	for _, unsafe := range []string{"vsplit " + savePath, "term touch " + savePath} {
		app.handle(key(tcell.KeyCtrlE, 0, tcell.ModCtrl))
		for _, r := range unsafe {
			app.handle(key(tcell.KeyRune, r, tcell.ModNone))
		}
		app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
		if !strings.Contains(app.currentStatus(), "Command unavailable") {
			t.Fatalf("unsafe command %q was not rejected visibly: %q", unsafe, app.currentStatus())
		}
	}
	if got, err := os.ReadFile(savePath); err != nil || string(got) != commandBaseline {
		t.Fatalf("blocked commands changed saved file: %q, %v", got, err)
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
		{tcell.KeyCtrlQ, `"<Ctrl-q><Ctrl-s>":"Unsplit"`, "Unsplit is unavailable"},
		{tcell.KeyCtrlG, `"<Ctrl-g><Ctrl-s>":"Unsplit"`, "Unsplit is unavailable"},
		{tcell.KeyCtrlE, `"<Ctrl-e><Ctrl-s>":"Unsplit"`, "Unsplit is unavailable"},
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

func TestReservedKeysHonorCompatibleRemaps(t *testing.T) {
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
	if row := app.shortcutBar(0); strings.Contains(row, "^Q Quit") || !strings.Contains(row, "^Q Save") {
		t.Fatalf("remapped footer advertises inactive binding: %q", row)
	}

	if app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl)) || !action.InfoBar.HasPrompt {
		t.Fatal("remapped Ctrl+Q did not invoke Save")
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	if app.handle(key(tcell.KeyCtrlG, 0, tcell.ModCtrl)) || !action.InfoBar.HasPrompt {
		t.Fatal("remapped Ctrl+G did not invoke OpenFile")
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	app.handle(key(tcell.KeyCtrlE, 0, tcell.ModCtrl))
	if !strings.Contains(app.currentStatus(), "ShellMode is unavailable") {
		t.Fatalf("unsupported shell mode was not explained: %q", app.currentStatus())
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
	bindings := []byte(`{"buffer":{"Ctrl-r":"Unsplit","<Ctrl-x><Ctrl-s>":"Unsplit"}}`)
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
	if !strings.Contains(app.currentStatus(), "Unsplit is unavailable") {
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
	if string(app.left.buf.Bytes()) != before || !strings.Contains(app.currentStatus(), "Unsplit is unavailable") {
		t.Fatalf("configured unsafe key sequence changed content or lacked explanation: content=%q status=%q", app.left.buf.Bytes(), app.currentStatus())
	}
}

func TestConfiguredUnsplitActionIsRejectedWithoutChangingPanes(t *testing.T) {
	configDir := t.TempDir()
	bindings := []byte(`{"buffer":{"Ctrl-r":"Unsplit"}}`)
	if err := os.WriteFile(filepath.Join(configDir, "bindings.json"), bindings, 0o600); err != nil {
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

	left, right := app.left.buf, app.right.buf
	rows := len(app.layout.Rows)
	if quit := app.handle(key(tcell.KeyCtrlR, 0, tcell.ModCtrl)); quit {
		t.Fatal("rejected Unsplit binding terminated mdiff")
	}
	if app.left.buf != left || app.right.buf != right || len(app.layout.Rows) != rows {
		t.Fatal("rejected Unsplit binding changed the two-pane layout")
	}
	if !strings.Contains(app.currentStatus(), "Unsplit is unavailable") {
		t.Fatalf("rejected Unsplit diagnostic = %q", app.currentStatus())
	}
}

func TestConfiguredPrivateViewportActionsUseSharedViewport(t *testing.T) {
	for _, actionName := range []string{
		"ScrollUp", "ScrollDown", "Center", "Start", "End", "PageUp", "PageDown", "HalfPageUp", "HalfPageDown",
	} {
		t.Run(actionName, func(t *testing.T) {
			configDir := t.TempDir()
			bindings := []byte(`{"buffer":{"Ctrl-r":"` + actionName + `"}}`)
			if err := os.WriteFile(filepath.Join(configDir, "bindings.json"), bindings, 0o600); err != nil {
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

			text := strings.Repeat("line\n", 20)
			app.handle(tcell.NewEventPaste(text, ""))
			app.focus = viewport.Right
			app.setFocus()
			app.handle(tcell.NewEventPaste(text, ""))
			flushDiffForTest(t, app)
			app.top = 2
			beforeTop := app.top
			if beforeTop <= 0 {
				t.Fatalf("test viewport top %d is not scrolled", beforeTop)
			}
			if quit := app.handle(key(tcell.KeyCtrlR, 0, tcell.ModCtrl)); quit {
				t.Fatal("rejected view action terminated mdiff")
			}
			if app.top == beforeTop {
				t.Fatalf("adapted view action did not move shared viewport from %d", beforeTop)
			}
			if status := app.currentStatus(); strings.Contains(status, actionName+" is unavailable") {
				t.Fatalf("adapted view action was rejected: %q", status)
			}
		})
	}
}

func newEditingTestApp(t *testing.T, width, height int) (*App, tcell.SimulationScreen) {
	t.Helper()
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"keymenu":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(width, height)
	app, err := New(sim, configDir)
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
