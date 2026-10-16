package prototype

import (
	"context"
	"fmt"
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

func TestMicroHelpUsesBundledBufferMultipleTopicsAndBindings(t *testing.T) {
	app, sim := newEditingTestApp(t, 68, 12)
	defer app.Close()
	left, right := app.left.buf, app.right.buf
	app.runMicroCommand(app.left.h, "help commands options -vsplit")
	if !app.helpVisible || app.helpBuf == nil || app.helpBuf.Type != buffer.BTHelp ||
		!strings.Contains(app.helpText, "# commands") || !strings.Contains(app.helpText, "# options") {
		t.Fatalf("help pages did not use combined Micro help buffer: visible=%t topic=%q", app.helpVisible, app.helpTopic)
	}
	app.draw()
	if got := screenLine(sim, 0, 68); !strings.Contains(got, "commands") {
		t.Fatalf("Micro help buffer was not rendered: %q", got)
	}
	action.BindKey("F7", "Find", action.BufMapEvent)
	app.handle(key(tcell.KeyF7, 0, tcell.ModNone))
	if !action.InfoBar.HasPrompt || !app.helpVisible {
		t.Fatal("configured Micro Find action did not open its normal prompt in help")
	}
	for _, r := range "command" {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	if app.helpBuf.LastSearch != "command" || !app.helpPane.Cursor.HasSelection() {
		t.Fatalf("Micro Find did not search/select help text: query=%q selection=%t", app.helpBuf.LastSearch, app.helpPane.Cursor.HasSelection())
	}
	firstMatch := app.helpPane.Cursor.Loc
	action.BindKey("F8", "FindNext", action.BufMapEvent)
	app.handle(key(tcell.KeyF8, 0, tcell.ModNone))
	if app.helpPane.Cursor.Loc == firstMatch {
		t.Fatal("configured FindNext did not advance in help text")
	}
	app.handle(key(tcell.KeyCtrlG, 0, tcell.ModCtrl))
	if app.helpVisible || app.left.buf != left || app.right.buf != right || len(action.Tabs.List) != 1 {
		t.Fatal("closing help changed the paired editor")
	}
}

func TestMicroHelpSingleTopicPreservesBytesAndEscBinding(t *testing.T) {
	app, _ := newEditingTestApp(t, 68, 12)
	defer app.Close()
	file := config.FindRuntimeFile(config.RTHelp, "commands")
	if file == nil {
		t.Fatal("bundled commands help page not found")
	}
	want, err := file.Data()
	if err != nil {
		t.Fatal(err)
	}
	app.runMicroCommand(app.left.h, "help commands")
	if got := app.helpBuf.Bytes(); string(got) != string(want) {
		t.Fatalf("single-topic help changed bundled page bytes: got prefix %q, want prefix %q", got[:min(len(got), 40)], want[:min(len(want), 40)])
	}
	action.BindKey("Esc", "CursorEnd", action.BufMapEvent)
	app.helpPane.Cursor.GotoLoc(buffer.Loc{})
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	if !app.helpVisible || app.helpPane.Cursor.Loc != app.helpBuf.End() {
		t.Fatalf("configured Esc binding was intercepted: visible=%t cursor=%v", app.helpVisible, app.helpPane.Cursor.Loc)
	}
}

func TestHelpOverlayContainsQuitPasteAndRawActions(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "left document")
	app.right.buf.Insert(buffer.Loc{}, "right document")
	leftBefore, rightBefore := string(app.left.buf.Bytes()), string(app.right.buf.Bytes())
	action.BindKey("F10", "ToggleHelp", action.BufMapEvent)
	app.runMicroCommand(app.left.h, "help commands")
	helpBefore := string(app.helpBuf.Bytes())
	app.handle(tcell.NewEventPaste("pasted text", ""))
	if string(app.left.buf.Bytes()) != leftBefore || string(app.right.buf.Bytes()) != rightBefore || string(app.helpBuf.Bytes()) != helpBefore {
		t.Fatal("paste in help modified a hidden document or read-only help text")
	}
	const raw = "\x1b[?43~"
	action.BindKey(raw, "CursorEnd", action.BufMapEvent)
	app.helpPane.Cursor.GotoLoc(buffer.Loc{})
	app.handle(tcell.NewEventRaw(raw))
	if app.helpPane.Cursor.Loc != app.helpBuf.End() {
		t.Fatalf("raw help binding left cursor at %v", app.helpPane.Cursor.Loc)
	}
	for _, event := range []*tcell.EventKey{key(tcell.KeyCtrlG, 0, tcell.ModCtrl), key(tcell.KeyCtrlQ, 0, tcell.ModCtrl)} {
		app.helpVisible = true
		app.handle(event)
		if app.helpVisible || app.quitComplete || len(action.Tabs.List) != 1 || len(action.Tabs.List[0].Panes) != 2 ||
			app.left.buf != app.left.h.Buf || app.right.buf != app.right.h.Buf {
			t.Fatalf("help close action %v changed the fixed tab or quit mdiff", event)
		}
	}
	app.helpVisible = true
	app.runMicroCommand(app.helpPane, "help commands -vsplit -hsplit")
	if !strings.Contains(app.currentStatus(), "hsplit and vsplit are not allowed at the same time") {
		t.Fatalf("conflicting help split flags did not preserve Micro's error: %q", app.currentStatus())
	}
}

func TestQuitAllFromHelpExitsEventLoopForKeyMouseAndRaw(t *testing.T) {
	const raw = "\x1b[?44~"
	for _, tc := range []struct {
		name    string
		binding string
		event   func(*App) tcell.Event
	}{
		{"key", "F11", func(_ *App) tcell.Event { return key(tcell.KeyF11, 0, tcell.ModNone) }},
		{"mouse", "MouseRight", func(_ *App) tcell.Event {
			return tcell.NewEventMouse(2, 0, tcell.ButtonSecondary, tcell.ModNone, "")
		}},
		{"raw", raw, func(_ *App) tcell.Event { return tcell.NewEventRaw(raw) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newEditingTestApp(t, 60, 12)
			defer app.Close()
			action.BindKey(tc.binding, "QuitAll", action.BufMapEvent)
			app.runMicroCommand(app.left.h, "help commands")
			events := make(chan tcell.Event, 1)
			events <- tc.event(app)
			close(events)
			if err := RunWithSignals(context.Background(), app, events); err != nil || !app.quitComplete {
				t.Fatalf("clean QuitAll from help did not exit event loop: err=%v complete=%t", err, app.quitComplete)
			}
		})
	}
	t.Run("dirty-confirmation", func(t *testing.T) {
		app, _ := newEditingTestApp(t, 60, 12)
		defer app.Close()
		app.left.buf.Insert(buffer.Loc{}, "dirty")
		action.BindKey("F11", "QuitAll", action.BufMapEvent)
		app.runMicroCommand(app.left.h, "help commands")
		events := make(chan tcell.Event, 2)
		events <- key(tcell.KeyF11, 0, tcell.ModNone)
		events <- key(tcell.KeyRune, 'y', tcell.ModNone)
		close(events)
		if err := RunWithSignals(context.Background(), app, events); err != nil || !app.quitComplete {
			t.Fatalf("confirmed dirty QuitAll from help did not exit event loop: err=%v complete=%t", err, app.quitComplete)
		}
	})
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

func TestSaveAllMatchesMicroForUntitledAndExternallyChangedFiles(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "untitled")
	app.right.buf.Insert(buffer.Loc{}, "also untitled")
	action.BindKey("F8", "SaveAll", action.BufMapEvent)
	app.handle(key(tcell.KeyF8, 0, tcell.ModNone))
	if action.InfoBar.HasPrompt || !app.left.buf.Modified() || !app.right.buf.Modified() {
		t.Fatal("SaveAll prompted for untitled buffers instead of using Buffer.Save semantics")
	}
	app.Close()
	left, right := filepath.Join(t.TempDir(), "left"), filepath.Join(t.TempDir(), "right")
	for _, path := range []string{left, right} {
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app = newWorkflowAppWithConfig(t, t.TempDir(), left, right)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "L")
	app.right.buf.Insert(buffer.Loc{}, "R")
	if err := os.WriteFile(right, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.handle(key(tcell.KeyF8, 0, tcell.ModNone))
	if action.InfoBar.HasPrompt {
		t.Fatal("SaveAll added an external-change confirmation absent from Micro's SaveAll")
	}
}

func TestBoundQuitAllConfirmsOnceAndDiscardsBothBuffers(t *testing.T) {
	for _, dirty := range []string{"left", "right", "both"} {
		t.Run(dirty, func(t *testing.T) {
			app, _ := newEditingTestApp(t, 60, 12)
			defer app.Close()
			if dirty == "left" || dirty == "both" {
				app.left.buf.Insert(buffer.Loc{}, "L")
			}
			if dirty == "right" || dirty == "both" {
				app.right.buf.Insert(buffer.Loc{}, "R")
			}
			action.BindKey("F10", "QuitAll", action.BufMapEvent)
			app.handle(key(tcell.KeyF10, 0, tcell.ModNone))
			if !action.InfoBar.HasPrompt || !strings.Contains(action.InfoBar.Msg, "without saving") {
				t.Fatalf("QuitAll prompt = %q; want one discard confirmation", action.InfoBar.Msg)
			}
			app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
			if app.quitComplete || !app.left.buf.Modified() && dirty != "right" || !app.right.buf.Modified() && dirty != "left" {
				t.Fatal("canceling QuitAll discarded data or quit")
			}
		})
	}
}

func TestBoundQuitAllYesDiscardsDirtyPanesWithoutSaving(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "left")
	app.right.buf.Insert(buffer.Loc{}, "right")
	action.BindKey("F10", "QuitAll", action.BufMapEvent)
	app.handle(key(tcell.KeyF10, 0, tcell.ModNone))
	app.handle(key(tcell.KeyRune, 'y', tcell.ModNone))
	if !app.quitComplete || !app.left.buf.Modified() || !app.right.buf.Modified() {
		t.Fatal("QuitAll confirmation did not quit while retaining unsaved buffers")
	}
}

func TestCommandBridgePreservesMicroQuotingAndCase(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	var got []string
	action.MakeCommand("MyCommand", func(_ *action.BufPane, args []string) { got = args }, nil)
	app.runMicroCommand(app.left.h, `"MyCommand" 'argument with spaces'`)
	if !reflect.DeepEqual(got, []string{"argument with spaces"}) {
		t.Fatalf("parsed command args = %#v", got)
	}
	app.runMicroCommand(app.left.h, `"help" commands options`)
	if !app.helpVisible || !strings.Contains(app.helpText, "# commands") || !strings.Contains(app.helpText, "# options") {
		t.Fatal("quoted command or multiple help topics were rejected")
	}
	app.runMicroCommand(app.left.h, `help 'unterminated`)
	if !strings.Contains(app.currentStatus(), "Error parsing args") {
		t.Fatalf("malformed quote error = %q", app.currentStatus())
	}
}

func TestDefaultDiffKeysFallBackWhenThereAreNoDifferences(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "same\ntext")
	app.right.buf.Insert(buffer.Loc{}, "same\ntext")
	app.rebuildWithAnchor(anchor{})
	app.left.h.GotoLoc(buffer.Loc{X: 4, Y: 1})
	app.handle(key(tcell.KeyRune, '[', tcell.ModAlt))
	if app.left.h.Cursor.Loc != (buffer.Loc{X: 0, Y: 0}) {
		t.Fatalf("DiffPrevious fallback cursor = %v", app.left.h.Cursor.Loc)
	}
}

func TestMouseWheelBindingTargetsPaneUnderPointerWithoutChangingFocus(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "left\nlast")
	app.right.buf.Insert(buffer.Loc{}, "right\nlast")
	app.left.h.GotoLoc(buffer.Loc{})
	app.right.h.GotoLoc(buffer.Loc{})
	app.focus = viewport.Left
	app.setFocus()
	action.BindKey("MouseWheelDown", "CursorEnd", action.BufMapEvent)
	x := app.left.width + 2
	app.handle(tcell.NewEventMouse(x, 1, tcell.WheelDown, tcell.ModNone, ""))
	if app.focus != viewport.Left || app.right.h.Cursor.Y != app.right.buf.LinesNum()-1 || app.left.h.Cursor.Y == app.left.buf.LinesNum()-1 {
		t.Fatalf("wheel target/focus = right cursor %v, left cursor %v, focus %v", app.right.h.Cursor.Loc, app.left.h.Cursor.Loc, app.focus)
	}
}

func TestMouseWheelOutsidePaneBoundsFallsBackToFocusedPane(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "left\nlast")
	app.right.buf.Insert(buffer.Loc{}, "right\nlast")
	app.left.h.GotoLoc(buffer.Loc{})
	app.right.h.GotoLoc(buffer.Loc{})
	app.focus = viewport.Left
	app.setFocus()
	action.BindKey("MouseWheelDown", "CursorEnd", action.BufMapEvent)
	cases := []struct {
		name string
		x, y int
	}{
		{"divider", app.left.x + app.left.width, 1},
		{"footer", app.right.x + 1, app.rows},
		{"outside", -1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app.left.h.GotoLoc(buffer.Loc{})
			app.right.h.GotoLoc(buffer.Loc{})
			app.handle(tcell.NewEventMouse(tc.x, tc.y, tcell.WheelDown, tcell.ModNone, ""))
			if app.left.h.Cursor.Y != app.left.buf.LinesNum()-1 || app.right.h.Cursor.Y != 0 || app.focus != viewport.Left {
				t.Fatalf("wheel fallback moved left=%v right=%v focus=%v", app.left.h.Cursor.Loc, app.right.h.Cursor.Loc, app.focus)
			}
		})
	}
}

func TestPageActionsMoveCursorAndSharedViewportTogether(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	var lines strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&lines, "line %02d\n", i)
	}
	app.left.buf.Insert(buffer.Loc{}, lines.String())
	app.right.buf.Insert(buffer.Loc{}, lines.String())
	app.rebuildWithAnchor(anchor{})
	app.left.h.GotoLoc(buffer.Loc{X: 0, Y: 20})
	app.ensureCursorVisible()
	before := app.left.h.Cursor.Y
	app.handle(key(tcell.KeyPgDn, 0, tcell.ModNone))
	if app.left.h.Cursor.Y <= before || app.top == 0 {
		t.Fatalf("PageDown cursor/top = %d/%d from %d", app.left.h.Cursor.Y, app.top, before)
	}
	afterDown := app.left.h.Cursor.Y
	app.handle(key(tcell.KeyPgUp, 0, tcell.ModNone))
	if app.left.h.Cursor.Y >= afterDown {
		t.Fatalf("PageUp cursor = %d after PageDown %d", app.left.h.Cursor.Y, afterDown)
	}
}

func TestPageActionAfterSharedWheelScrollPreservesCursorScreenRow(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	content := strings.Repeat("line\n", 80)
	app.left.buf.Insert(buffer.Loc{}, content)
	app.right.buf.Insert(buffer.Loc{}, content)
	app.left.buf.Settings["scrollspeed"] = float64(1)
	app.right.buf.Settings["scrollspeed"] = float64(1)
	app.rebuildWithAnchor(anchor{})
	app.left.h.GotoLoc(buffer.Loc{Y: 20})
	app.top = 15
	app.routeMicroAction("ScrollDown", app.left.h)
	beforeRow := app.layout.CursorRow(viewport.Left, app.left.h.Cursor.Y, app.left.view.VLocFromLoc(app.left.h.Cursor.Loc).Row) - app.top
	if beforeRow < 0 || beforeRow >= app.rows {
		t.Fatalf("shared scroll left cursor outside viewport at row %d (top=%d cursor=%d rows=%d)", beforeRow, app.top, app.left.h.Cursor.Y, app.rows)
	}
	if app.left.view.StartLine.Line == app.top {
		t.Fatal("test did not leave Micro's private view stale after shared scrolling")
	}
	app.handle(key(tcell.KeyPgDn, 0, tcell.ModNone))
	afterRow := app.layout.CursorRow(viewport.Left, app.left.h.Cursor.Y, app.left.view.VLocFromLoc(app.left.h.Cursor.Loc).Row) - app.top
	startRow := app.layout.CursorRow(viewport.Left, app.left.view.StartLine.Line, app.left.view.StartLine.Row)
	microRow := app.left.h.Cursor.Y - app.left.view.StartLine.Line
	if startRow != app.top || afterRow != microRow {
		t.Fatalf("PageDown shared position differs from Micro's view: cursor row=%d Micro row=%d top=%d Micro top=%d (previous cursor row=%d)", afterRow, microRow, app.top, startRow, beforeRow)
	}
}

func TestSelectionPageActionTracksWrappedAlignedRowsOnRightPane(t *testing.T) {
	app, _ := newEditingTestApp(t, 40, 10)
	defer app.Close()
	var left, right strings.Builder
	for i := 0; i < 40; i++ {
		line := fmt.Sprintf("line %02d with a long wrapped tail that continues", i)
		fmt.Fprintln(&left, line)
		if i == 4 {
			fmt.Fprintln(&right, "right-only inserted row")
		}
		fmt.Fprintln(&right, line)
	}
	for _, p := range app.panes() {
		p.buf.Settings["softwrap"] = true
	}
	app.left.buf.Insert(buffer.Loc{}, left.String())
	app.right.buf.Insert(buffer.Loc{}, right.String())
	app.rebuildWithAnchor(anchor{})
	app.focus = viewport.Right
	app.setFocus()
	app.right.h.GotoLoc(buffer.Loc{X: 0, Y: 18})
	app.ensureCursorVisible()
	app.handle(key(tcell.KeyPgDn, 0, tcell.ModShift))
	if !app.right.h.Cursor.HasSelection() || app.top == 0 {
		t.Fatalf("ShiftPageDown selection/viewport = %t/%d", app.right.h.Cursor.HasSelection(), app.top)
	}
	loc := app.right.h.Cursor.Loc
	visual := app.right.view.VLocFromLoc(loc)
	row := app.layout.CursorRow(viewport.Right, loc.Y, visual.Row)
	if row < app.top || row >= app.top+app.rows {
		t.Fatalf("selected cursor row %d is outside shared viewport [%d,%d)", row, app.top, app.top+app.rows)
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
