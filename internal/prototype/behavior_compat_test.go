package prototype

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/clipboard"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func TestPromptOwnedBufferSyncSurvivesFocusChange(t *testing.T) {
	for _, operation := range []string{"command open", "command open after close prompt", "OpenFile"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "replacement.txt")
			if err := os.WriteFile(path, []byte("replacement\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			app, _ := newEditingTestApp(t, 80, 12)
			defer app.Close()
			oldRight := app.right.buf
			if operation == "command open after close prompt" {
				app.left.buf.Insert(buffer.Loc{}, "unsaved")
			}
			if strings.HasPrefix(operation, "command open") {
				app.openCommandBar()
			} else {
				app.handle(key(tcell.KeyCtrlO, 0, tcell.ModCtrl))
			}
			if !action.InfoBar.HasPrompt {
				t.Fatal("file prompt did not open")
			}
			x := app.right.view.BufView().X
			app.handle(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone, ""))
			app.handle(tcell.NewEventMouse(x, 0, tcell.ButtonNone, tcell.ModNone, ""))
			if app.focus != viewport.Right {
				t.Fatal("mouse did not move focus during prompt")
			}
			input := path
			if strings.HasPrefix(operation, "command open") {
				input = "open " + path
			}
			for _, r := range input {
				app.handle(key(tcell.KeyRune, r, tcell.ModNone))
			}
			app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
			if operation == "command open after close prompt" {
				if !action.InfoBar.HasPrompt {
					t.Fatal("dirty file replacement did not prompt")
				}
				app.handle(key(tcell.KeyRune, 'n', tcell.ModNone))
			}
			if app.left.buf != app.left.h.Buf || app.left.buf.Path != path {
				t.Fatalf("opened pane desynchronized: app=%q Micro=%q", app.left.buf.Path, app.left.h.Buf.Path)
			}
			if app.isDiffStale() || len(app.displayedLeft) == 0 || app.displayedLeft[0] != "replacement" || app.layout.RowFor(viewport.Left, 0) < 0 {
				t.Fatal("opened pane did not rebuild its aligned diff snapshot")
			}
			if app.right.buf != oldRight || app.focus != viewport.Right {
				t.Fatal("opening the original pane changed the focused pane")
			}
		})
	}
}

func TestFilePromptsKeepPaneOwnershipAcrossResize(t *testing.T) {
	for _, tc := range []struct {
		name, binding string
		dirty         bool
	}{
		{"OpenFile", "", false},
		{"command-edit open", "command-edit:open ", false},
		{"command-edit dirty open", "command-edit:open ", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "resized.txt")
			if err := os.WriteFile(path, []byte("resized contents\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			app, sim := newEditingTestApp(t, 80, 12)
			defer app.Close()
			originalPane := app.left.h
			if tc.dirty {
				app.left.buf.Insert(buffer.Loc{}, "unsaved")
			}
			if tc.binding == "" {
				app.handle(key(tcell.KeyCtrlO, 0, tcell.ModCtrl))
			} else {
				action.BindKey("F7", tc.binding, action.BufMapEvent)
				app.handle(key(tcell.KeyF7, 0, tcell.ModNone))
			}
			if !action.InfoBar.HasPrompt {
				t.Fatal("file prompt did not open")
			}
			sim.SetSize(90, 14)
			app.handle(tcell.NewEventResize(90, 14))
			if app.left.h != originalPane {
				t.Fatal("resize replaced the pane captured by the prompt")
			}
			for _, r := range path {
				app.handle(key(tcell.KeyRune, r, tcell.ModNone))
			}
			app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
			if tc.dirty {
				if !action.InfoBar.HasPrompt {
					t.Fatal("dirty replacement did not request a save decision")
				}
				sim.SetSize(96, 16)
				app.handle(tcell.NewEventResize(96, 16))
				app.handle(key(tcell.KeyRune, 'n', tcell.ModNone))
			}
			if app.left.h != originalPane || app.left.h.Buf != app.left.buf || app.left.buf.Path != path {
				t.Fatalf("resized open escaped pane ownership: app=%q Micro=%q", app.left.buf.Path, app.left.h.Buf.Path)
			}
			if app.isDiffStale() || app.displayedLeft[0] != "resized contents" {
				t.Fatal("resized open did not refresh the aligned snapshot")
			}
		})
	}
}

func TestRawAndMouseQuitExitEventLoop(t *testing.T) {
	const sequence = "\x1b[?42~"
	for _, tc := range []struct {
		name, binding, action string
		event                 func(*App) tcell.Event
	}{
		{"raw Quit", sequence, "Quit", func(_ *App) tcell.Event { return tcell.NewEventRaw(sequence) }},
		{"raw ForceQuit", sequence, "ForceQuit", func(_ *App) tcell.Event { return tcell.NewEventRaw(sequence) }},
		{"mouse QuitAll", "MouseRight", "QuitAll", func(a *App) tcell.Event {
			return tcell.NewEventMouse(a.left.view.BufView().X, 0, tcell.ButtonSecondary, tcell.ModNone, "")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newEditingTestApp(t, 60, 12)
			defer app.Close()
			action.BindKey(tc.binding, tc.action, action.BufMapEvent)
			events := make(chan tcell.Event, 1)
			events <- tc.event(app)
			close(events)
			if err := RunWithSignals(context.Background(), app, events); err != nil || !app.quitComplete {
				t.Fatalf("event loop did not exit on completed %s: err=%v complete=%t", tc.action, err, app.quitComplete)
			}
		})
	}
}

func TestMicroHelpTopicsUseOverlayWithoutReplacingBuffers(t *testing.T) {
	app, sim := newEditingTestApp(t, 68, 12)
	defer app.Close()
	left, right := app.left.buf, app.right.buf
	app.runMicroCommand(app.left.h, "help commands")
	if !app.helpVisible || app.helpTopic != "commands" || !strings.Contains(app.helpText, "# Commands") {
		t.Fatalf("Micro help topic missing: visible=%t topic=%q", app.helpVisible, app.helpTopic)
	}
	app.draw()
	if got := screenLine(sim, 0, 68); !strings.Contains(got, "Micro help: commands") {
		t.Fatalf("help title missing from render: %q", got)
	}
	app.handle(key(tcell.KeyPgDn, 0, tcell.ModNone))
	if app.helpTop == 0 {
		t.Fatal("help did not scroll")
	}
	app.handle(key(tcell.KeyRight, 0, tcell.ModNone))
	if app.helpTopic == "commands" || app.helpTop != 0 {
		t.Fatalf("help topic navigation failed: topic=%q top=%d", app.helpTopic, app.helpTop)
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	if app.helpVisible || app.left.buf != left || app.right.buf != right || len(action.Tabs.List) != 1 {
		t.Fatal("closing help changed two-pane editor layout")
	}
}

func TestMicroHelpFindAndRepeatKeepsOverlay(t *testing.T) {
	app, sim := newEditingTestApp(t, 68, 12)
	defer app.Close()
	left, right := app.left.buf, app.right.buf
	app.runMicroCommand(app.left.h, "help commands")
	app.handle(key(tcell.KeyCtrlF, 0, tcell.ModCtrl))
	if !action.InfoBar.HasPrompt || !app.helpVisible {
		t.Fatal("Ctrl+F did not open help search")
	}
	for _, r := range "command" {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	matches := app.helpMatches()
	if !app.helpVisible || len(matches) < 2 || app.helpMatch < 0 || app.helpTop != matches[app.helpMatch].first {
		t.Fatalf("help search did not select a match: query=%q matches=%v selected=%d top=%d", app.helpQuery, matches, app.helpMatch, app.helpTop)
	}
	first := app.helpMatch
	app.draw()
	_, _, attrs := cellStyle(sim, 0, 1)
	if attrs&tcell.AttrReverse == 0 {
		t.Fatal("selected help match is not highlighted")
	}
	app.handle(key(tcell.KeyRune, 'n', tcell.ModNone))
	if app.helpMatch == first || app.helpTop != matches[app.helpMatch].first {
		t.Fatal("next help match did not move the viewport")
	}
	app.handle(key(tcell.KeyRune, 'N', tcell.ModNone))
	if app.helpMatch != first || app.left.buf != left || app.right.buf != right {
		t.Fatal("previous help match or editor buffer ownership changed")
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	if app.helpVisible {
		t.Fatal("Escape did not close help")
	}
}

func TestHelpSearchPromptShowsCursorUntilClosed(t *testing.T) {
	app, sim := newEditingTestApp(t, 40, 10)
	defer app.Close()
	app.openHelpTopic("help")
	app.handle(key(tcell.KeyCtrlF, 0, tcell.ModCtrl))
	app.handle(key(tcell.KeyRune, '猫', tcell.ModNone))
	app.draw()
	if x, y, visible := sim.GetCursor(); !visible || y != app.statusRow || x != len("Find help (regex): ")+2 {
		t.Fatalf("help search cursor = (%d,%d) visible=%t", x, y, visible)
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	app.draw()
	if !app.helpVisible || action.InfoBar.HasPrompt {
		t.Fatal("closing help search also closed the help overlay")
	}
	if _, _, visible := sim.GetCursor(); visible {
		t.Fatal("editor cursor visible over help after search prompt closed")
	}
}

func TestUnicodeHelpWrapAndMatchRowsFollowVisualColumns(t *testing.T) {
	app, sim := newEditingTestApp(t, 8, 8)
	defer app.Close()
	app.openHelpTopic("help")
	app.helpText = "aaaaaa猫e\u0301X"
	if got := app.helpLines(); !reflect.DeepEqual(got, []string{"aaaaaa猫", "e\u0301X"}) {
		t.Fatalf("Unicode help rows = %#v", got)
	}
	app.findHelp("猫e\u0301")
	if got := app.helpMatches(); len(got) != 1 || got[0] != (helpMatchPosition{first: 0, last: 1}) {
		t.Fatalf("Unicode help match rows = %+v", got)
	}
	app.draw()
	for _, row := range []int{1, 2} {
		_, _, attrs := cellStyle(sim, 0, row)
		if attrs&tcell.AttrReverse == 0 {
			t.Fatalf("Unicode match row %d not highlighted", row)
		}
	}
	r, marks, _, _ := sim.GetContent(0, 2)
	if r != 'e' || !reflect.DeepEqual(marks, []rune{'\u0301'}) {
		t.Fatalf("combined character split or lost: %q + %q", r, string(marks))
	}
}

func TestHelpSearchHighlightsEveryRowOfWrappedTab(t *testing.T) {
	app, sim := newEditingTestApp(t, 3, 8)
	defer app.Close()
	app.openHelpTopic("help")
	app.helpText = "\tX"
	if got := app.helpLines(); !reflect.DeepEqual(got, []string{"   ", " X"}) {
		t.Fatalf("wrapped tab rows = %#v", got)
	}
	app.findHelp(`\t`)
	if got := app.helpMatches(); len(got) != 1 || got[0] != (helpMatchPosition{first: 0, last: 1}) {
		t.Fatalf("wrapped tab match rows = %+v", got)
	}
	app.draw()
	for _, y := range []int{1, 2} {
		_, _, attrs := cellStyle(sim, 0, y)
		if attrs&tcell.AttrReverse == 0 {
			t.Fatalf("tab's display row %d was not highlighted", y)
		}
	}
}

func TestMicroHelpFindCrossesVisualWrap(t *testing.T) {
	app, sim := newEditingTestApp(t, 12, 8)
	defer app.Close()
	app.openHelpTopic("help")
	app.helpText = "before longneedle after"
	app.handle(key(tcell.KeyCtrlF, 0, tcell.ModCtrl))
	for _, r := range "longneedle" {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	matches := app.helpMatches()
	if len(matches) != 1 || matches[0] != (helpMatchPosition{first: 0, last: 1}) || app.helpMatch != 0 {
		t.Fatalf("wrapped source match mapped incorrectly: %+v selected=%d", matches, app.helpMatch)
	}
	app.draw()
	for _, row := range []int{1, 2} {
		_, _, attrs := cellStyle(sim, 0, row)
		if attrs&tcell.AttrReverse == 0 {
			t.Fatalf("wrapped match row %d was not highlighted", row)
		}
	}
	if !app.helpVisible || action.InfoBar.HasError {
		t.Fatal("wrapped help search unexpectedly closed or reported no match")
	}
}

func TestMicroHelpFindUsesRegexAndIgnorecaseSetting(t *testing.T) {
	app, _ := newEditingTestApp(t, 40, 10)
	defer app.Close()
	app.openHelpTopic("help")
	app.helpText = "axb"
	app.findHelp("a.b")
	if got := app.helpMatches(); len(got) != 1 || app.helpMatch != 0 {
		t.Fatalf("regex Find did not match axb: %+v selected=%d", got, app.helpMatch)
	}
	app.runMicroCommand(app.left.h, "set ignorecase false")
	if config.GetGlobalOption("ignorecase") != false {
		t.Fatal("Micro ignorecase setting was not disabled")
	}
	app.helpText = "Alpha"
	app.findHelp("alpha")
	if app.helpMatch != -1 || len(app.helpMatches()) != 0 {
		t.Fatal("case-sensitive help search matched different case")
	}
	app.findHelp("Alpha")
	if app.helpMatch != 0 || len(app.helpMatches()) != 1 {
		t.Fatal("case-sensitive help search missed exact case")
	}
	app.findHelp("[")
	if !action.InfoBar.HasError || app.helpQuery != "Alpha" || app.helpMatch != 0 {
		t.Fatal("invalid regex did not report an error and preserve the previous search")
	}
}

func TestMicroMovementAndRemapsReachFocusedPane(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	app.handle(tcell.NewEventPaste("one\ntwo\nthree", ""))
	app.left.h.GotoLoc(buffer.Loc{})
	app.handle(key(tcell.KeyDown, 0, tcell.ModAlt))
	if got := app.left.buf.Line(0); got != "two" {
		t.Fatalf("default Alt+Down did not move a Micro line: %q", got)
	}
	app.handle(key(tcell.KeyDown, 0, tcell.ModCtrl))
	if app.left.h.Cursor.Y != app.left.buf.LinesNum()-1 {
		t.Fatalf("default Ctrl+Down did not move to buffer end: %v", app.left.h.Cursor.Loc)
	}
	action.BindKey("AltDown", "CursorStart", action.BufMapEvent)
	app.handle(key(tcell.KeyDown, 0, tcell.ModAlt))
	if app.left.h.Cursor.Loc != (buffer.Loc{}) {
		t.Fatalf("remapped Alt+Down did not run CursorStart: %v", app.left.h.Cursor.Loc)
	}
	app.handle(key(tcell.KeyRune, ']', tcell.ModAlt))
	if app.left.buf.Line(0) != "two" {
		t.Fatal("diff navigation changed buffer content")
	}
}

func TestMouseGesturesUseMicroOnAlignedRows(t *testing.T) {
	oldMethod := clipboard.CurrentMethod
	clipboard.CurrentMethod = clipboard.Internal
	defer func() { clipboard.CurrentMethod = oldMethod }()
	app, _ := newEditingTestApp(t, 70, 12)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "alpha beta\ngamma delta")
	app.right.buf.Insert(buffer.Loc{}, "alpha beta\ninserted\ngamma delta")
	app.rebuildWithAnchor(anchor{})
	x := app.left.view.BufView().X + 1
	app.handle(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone, ""))
	app.handle(tcell.NewEventMouse(x+4, 0, tcell.Button1, tcell.ModNone, ""))
	app.handle(tcell.NewEventMouse(x+4, 0, tcell.ButtonNone, tcell.ModNone, ""))
	if !app.left.h.Cursor.HasSelection() {
		t.Fatal("drag did not create Micro selection")
	}
	app.handle(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone, ""))
	if !app.left.h.DoubleClick {
		t.Fatal("second click did not use Micro double-click selection")
	}
	app.handle(tcell.NewEventMouse(x, 0, tcell.ButtonNone, tcell.ModNone, ""))
	app.handle(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone, ""))
	if !app.left.h.TripleClick || !app.left.h.Cursor.HasSelection() {
		t.Fatal("third click did not select a Micro line")
	}
	app.handle(tcell.NewEventMouse(x, 0, tcell.ButtonNone, tcell.ModNone, ""))
	app.handle(tcell.NewEventMouse(x+3, 0, tcell.Button1, tcell.ModCtrl, ""))
	if app.left.buf.NumCursors() < 2 {
		t.Fatal("Ctrl-click did not add a Micro cursor")
	}
	app.handle(tcell.NewEventMouse(x+3, 0, tcell.ButtonNone, tcell.ModCtrl, ""))
	app.left.buf.ClearCursors()
	if err := clipboard.Write("P", clipboard.PrimaryReg); err != nil {
		t.Fatal(err)
	}
	app.handle(tcell.NewEventMouse(x, 0, tcell.ButtonMiddle, tcell.ModNone, ""))
	if !strings.Contains(app.left.buf.Line(0), "P") {
		t.Fatal("middle click did not paste primary clipboard")
	}
	app.handle(tcell.NewEventMouse(x, 0, tcell.ButtonNone, tcell.ModNone, ""))
	spacer := app.layout.RowFor(viewport.Right, 1)
	if spacer < 0 {
		t.Fatal("expected inserted line")
	}
	app.handle(tcell.NewEventMouse(x, spacer, tcell.Button1, tcell.ModNone, ""))
	if app.focus != viewport.Left || app.left.h.Cursor.Y < 0 || app.left.h.Cursor.Y >= app.left.buf.LinesNum() {
		t.Fatalf("spacer click escaped left buffer: %v", app.left.h.Cursor.Loc)
	}
	app.handle(tcell.NewEventMouse(x, spacer, tcell.ButtonNone, tcell.ModNone, ""))
	action.BindKey("MouseLeft", "Unsplit", action.BufMapEvent)
	before := app.left.h.Cursor.Loc
	app.handle(tcell.NewEventMouse(x+4, 0, tcell.Button1, tcell.ModNone, ""))
	if app.left.h.Cursor.Loc != before || !strings.Contains(app.currentStatus(), "Unsplit is unavailable") {
		t.Fatal("unsafe mouse binding ran its former action or lacked an explanation")
	}
}

func TestWrappedRowMouseDragUsesSourceColumns(t *testing.T) {
	app, _ := newEditingTestApp(t, 36, 10)
	defer app.Close()
	if err := app.left.buf.SetOptionNative("softwrap", true); err != nil {
		t.Fatal(err)
	}
	app.left.buf.Insert(buffer.Loc{}, strings.Repeat("w", 65))
	app.rebuildWithAnchor(anchor{})
	if app.layout.WrapHeight(viewport.Left, 0) < 2 {
		t.Fatal("fixture did not wrap")
	}
	x := app.left.view.BufView().X + 1
	app.handle(tcell.NewEventMouse(x, 0, tcell.Button1, tcell.ModNone, ""))
	app.handle(tcell.NewEventMouse(x+3, 1, tcell.Button1, tcell.ModNone, ""))
	app.handle(tcell.NewEventMouse(x+3, 1, tcell.ButtonNone, tcell.ModNone, ""))
	if app.left.h.Cursor.X <= 3 || !app.left.h.Cursor.HasSelection() {
		t.Fatalf("wrapped drag did not map to later source column: %v", app.left.h.Cursor.Loc)
	}
}

func TestAlternateQuitAndRemappedSaveAll(t *testing.T) {
	dir := t.TempDir()
	left := filepath.Join(dir, "left.txt")
	right := filepath.Join(dir, "right.txt")
	for _, path := range []string{left, right} {
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := newWorkflowAppWithConfig(t, t.TempDir(), left, right)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "L")
	app.right.buf.Insert(buffer.Loc{}, "R")
	action.BindKey("F8", "SaveAll", action.BufMapEvent)
	app.handle(key(tcell.KeyF8, 0, tcell.ModNone))
	for path, want := range map[string]string{left: "Lold\n", right: "Rold\n"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("SaveAll %s = %q, %v; want %q", path, got, err, want)
		}
	}
	if !app.handle(key(tcell.KeyF4, 0, tcell.ModNone)) {
		t.Fatal("Micro F4 quit did not complete clean two-pane workflow")
	}
}

func TestCommandBindingsAndRawEventsUseDispatcher(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	app.handle(tcell.NewEventPaste("abc\ndef", ""))
	action.MakeCommand("mdiff_test", func(h *action.BufPane, _ []string) {
		h.Buf.Insert(buffer.Loc{}, "P")
	}, nil)
	action.BindKey("F7", "command:mdiff_test", action.BufMapEvent)
	app.handle(key(tcell.KeyF7, 0, tcell.ModNone))
	if app.left.buf.Line(0) != "Pabc" || app.right.buf.Line(0) != "" {
		t.Fatal("plugin-defined command did not edit only the focused pane")
	}
	action.BindKey("F8", "command:set keymenu false", action.BufMapEvent)
	app.handle(key(tcell.KeyF8, 0, tcell.ModNone))
	if app.menuRow1 != -1 {
		t.Fatal("bound setting command did not hide key menu")
	}
	action.BindKey("F9", "command:vsplit", action.BufMapEvent)
	app.handle(key(tcell.KeyF9, 0, tcell.ModNone))
	if !strings.Contains(app.currentStatus(), "Command unavailable") || len(action.Tabs.List) != 1 {
		t.Fatal("unsafe bound command changed two-pane layout")
	}
	app.handle(key(tcell.KeyCtrlL, 0, tcell.ModCtrl))
	if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "goto") {
		t.Fatal("Micro Ctrl+L command-edit binding did not open goto prompt")
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	const seq = "\x1b[?42~"
	action.BindKey(seq, "CursorEnd", action.BufMapEvent)
	app.left.h.GotoLoc(buffer.Loc{})
	app.handle(tcell.NewEventRaw(seq))
	if app.left.h.Cursor.Y != 1 {
		t.Fatalf("raw Micro binding did not move cursor: %v", app.left.h.Cursor.Loc)
	}
	action.BindKey(seq, "Unsplit", action.BufMapEvent)
	app.left.h.GotoLoc(buffer.Loc{})
	app.handle(tcell.NewEventRaw(seq))
	if app.left.h.Cursor.Loc != (buffer.Loc{}) || !strings.Contains(app.currentStatus(), "Unsplit is unavailable") {
		t.Fatal("unsafe raw binding ran its former action or lacked an explanation")
	}
}

func TestConfiguredFooterAndSamePathStatePersistence(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"savecursor":true,"savehistory":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "same.txt")
	if err := os.WriteFile(path, []byte("abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newWorkflowAppWithConfig(t, configDir, path, path)
	if app.menuRow1 != -1 || app.statusRow != app.terminalHeight()-1 {
		t.Fatalf("Micro footer defaults ignored: menu=%d info=%d", app.menuRow1, app.statusRow)
	}
	app.runMicroCommand(app.left.h, "set statusline false")
	if app.rows != app.terminalHeight()-1 {
		t.Fatalf("disabled status line did not return row to viewport: %d", app.rows)
	}
	app.runMicroCommand(app.left.h, "set keymenu true")
	if app.menuRow1 < 0 {
		t.Fatal("enabled key menu was not displayed")
	}
	app.left.h.GotoLoc(buffer.Loc{X: 1, Y: 0})
	app.right.h.GotoLoc(buffer.Loc{X: 4, Y: 0})
	app.focus = viewport.Right
	app.setFocus()
	app.openCommandBar()
	for _, r := range "pwd" {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	app.Close()
	if _, err := os.Stat(filepath.Join(configDir, "buffers", "history")); err != nil {
		t.Fatalf("command history was not saved: %v", err)
	}
	reopened := newWorkflowAppWithConfig(t, configDir, path, path)
	defer reopened.Close()
	if reopened.left.buf.StartCursor.X != 4 || reopened.right.buf.StartCursor.X != 4 {
		t.Fatalf("focused pane did not own same-path state: left=%v right=%v", reopened.left.buf.StartCursor, reopened.right.buf.StartCursor)
	}
	if len(action.InfoBar.History["Command"]) == 0 {
		t.Fatal("Micro command history was not restored")
	}
}
