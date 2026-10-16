package prototype

import (
	"strings"
	"testing"

	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func TestBlockedMicroCommandsPreservePairedPanes(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{}, "left")
	app.right.buf.Insert(buffer.Loc{}, "right")
	app.focus = viewport.Right
	app.setFocus()
	// A resize replaces mdiff's Micro tab. A following tabswitch must not
	// activate that new tab or call a plugin's onSetActive hook.
	beforeResize := action.Tabs.List[0]
	app.resize(60, 12)
	if action.Tabs.List[0] == beforeResize {
		t.Fatal("resize did not replace the Micro tab")
	}

	left, right := app.left.buf, app.right.buf
	leftPane, rightPane := app.left.h, app.right.h
	tab := action.Tabs.List[0]
	top := app.top
	for _, command := range []string{
		"vsplit", "hsplit", "tab", "tabmove 1", "tabswitch 1", "term", "log", "plugin list", "reload", "raw",
	} {
		name := strings.Fields(command)[0]
		if !action.IsRegisteredCommand(name) || safeCommand(name) {
			t.Fatalf("%q is not a guarded registered command", name)
		}
		app.runMicroCommand(app.right.h, command)
		if status := app.currentStatus(); !strings.Contains(status, "Command unavailable in mdiff: "+name) {
			t.Fatalf("%q was not rejected visibly: %q", command, status)
		}
		if len(action.Tabs.List) != 1 || action.Tabs.List[0] != tab || len(tab.Panes) != 2 ||
			app.left.buf != left || app.right.buf != right || app.left.h != leftPane || app.right.h != rightPane ||
			app.focus != viewport.Right || app.top != top || string(left.Bytes()) != "left" || string(right.Bytes()) != "right" {
			t.Fatalf("%q changed the fixed panes or shared viewport", command)
		}
	}
}

func TestEvalCommandReachesMicro(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 12)
	defer app.Close()
	app.focus = viewport.Right
	app.setFocus()
	left, right := app.left.buf, app.right.buf
	tab := action.Tabs.List[0]
	top := app.top

	if !action.IsRegisteredCommand("eval") || !safeCommand("eval") {
		t.Fatal("eval is not an allowed registered command")
	}
	app.runMicroCommand(app.right.h, "eval 1 + 1")
	if status := app.currentStatus(); !strings.Contains(status, "Eval unsupported") {
		t.Fatalf("eval did not reach bundled Micro: %q", status)
	}
	if len(action.Tabs.List) != 1 || action.Tabs.List[0] != tab || len(tab.Panes) != 2 ||
		app.left.buf != left || app.right.buf != right || app.focus != viewport.Right || app.top != top {
		t.Fatal("allowed commands changed the fixed panes")
	}
}
