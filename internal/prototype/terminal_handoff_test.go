package prototype

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func TestTerminalHandoffInstallsPendingDiffBeforeReaderRestart(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 10)
	defer app.Close()
	app.right.buf.Insert(buffer.Loc{}, "right content")
	app.rebuildWithAnchor(anchor{})
	app.focus = viewport.Right
	app.setFocus()
	app.right.h.Cursor.GotoLoc(buffer.Loc{X: 5})
	app.right.h.Cursor.CurSelection = [2]buffer.Loc{{X: 1}, {X: 5}}
	leftBuffer, rightBuffer := app.left.buf, app.right.buf
	rightCursor, rightSelection := app.right.h.Cursor.Loc, app.right.h.Cursor.CurSelection
	events := make(chan tcell.Event, 16)
	session := newTerminalSession(app, events)
	defer session.stopReader()
	next := tcell.NewSimulationScreen("")
	if err := next.Init(); err != nil {
		t.Fatal(err)
	}
	next.SetSize(60, 10)
	session.restart = func(wasNil bool) error {
		if wasNil {
			t.Fatal("handoff did not release an initialized screen")
		}
		screen.Screen = next
		screen.Unlock()
		return nil
	}
	if err := session.handoff(func() error {
		if screen.Screen != nil || app.terminal != nil {
			t.Fatal("handoff work ran while the editor screen was live")
		}
		app.left.buf.Insert(app.left.h.Cursor.Loc, "pending diff")
		left, right := app.snapshots()
		app.coordinator.DebounceElapsedOwned(diffSnapshot{left: left, right: right, versions: app.versions})
		select {
		case <-app.coordinator.activeDone:
		case <-time.After(3 * time.Second):
			t.Fatal("comparison did not finish during handoff")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if app.terminal != next || session.reader == nil || app.displayedLeft[0] != "pending diff" {
		t.Fatalf("handoff did not install pending comparison on the new screen: terminal=%v reader=%v displayed=%q", app.terminal == next, session.reader != nil, app.displayedLeft)
	}
	if app.left.buf != leftBuffer || app.right.buf != rightBuffer || app.focus != viewport.Right ||
		app.right.h.Cursor.Loc != rightCursor || app.right.h.Cursor.CurSelection != rightSelection ||
		!app.left.buf.Modified() || !app.right.buf.Modified() {
		t.Fatal("handoff changed buffer identity, focus, cursor, selection, or dirty state")
	}
	if app.isDiffStale() || app.layout.RowFor(viewport.Left, 0) != app.layout.RowFor(viewport.Right, 0) {
		t.Fatal("handoff did not resume with aligned current rows")
	}
	app.draw()
	if !strings.Contains(simulationText(next), "pending diff") || !strings.Contains(simulationText(next), "right content") {
		t.Fatal("resumed screen did not paint both preserved panes")
	}
}

func TestTerminalHandoffDiscardsStaleCompletedDiff(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 10)
	defer app.Close()
	session := newTerminalSession(app, make(chan tcell.Event, 16))
	defer session.stopReader()
	next := tcell.NewSimulationScreen("")
	if err := next.Init(); err != nil {
		t.Fatal(err)
	}
	next.SetSize(60, 10)
	session.restart = func(bool) error { screen.Screen = next; screen.Unlock(); return nil }
	var staleVersion DiffVersions
	if err := session.handoff(func() error {
		app.left.buf.Insert(buffer.Loc{}, "obsolete")
		staleVersion = app.versions
		left, right := app.snapshots()
		app.coordinator.DebounceElapsedOwned(diffSnapshot{left: left, right: right, versions: staleVersion})
		select {
		case <-app.coordinator.activeDone:
		case <-time.After(3 * time.Second):
			t.Fatal("stale comparison did not complete")
		}
		app.left.buf.Remove(buffer.Loc{}, buffer.Loc{X: 8})
		app.left.buf.Insert(buffer.Loc{}, "current")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if app.layoutVersions == staleVersion || strings.Contains(strings.Join(app.displayedLeft, ""), "obsolete") {
		t.Fatal("stale comparison was installed on resume")
	}
	if app.coordinator.activeDone == nil {
		t.Fatal("newest comparison was not started after stale result")
	}
	select {
	case <-app.coordinator.activeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("newest comparison did not complete")
	}
	session.drainDiff()
	if app.isDiffStale() || app.displayedLeft[0] != "current" {
		t.Fatalf("latest comparison not installed: stale=%t display=%q", app.isDiffStale(), app.displayedLeft)
	}
}

func simulationText(sim tcell.SimulationScreen) string {
	width, height := sim.Size()
	var out strings.Builder
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			main, _, _, _ := sim.GetContent(x, y)
			out.WriteRune(main)
		}
		out.WriteByte('\n')
	}
	return out.String()
}

func TestTerminalHandoffRestartFailureDoesNotReuseReleasedScreen(t *testing.T) {
	app, _ := newEditingTestApp(t, 60, 10)
	defer app.Close()
	session := newTerminalSession(app, make(chan tcell.Event, 16))
	session.restart = func(bool) error {
		screen.Unlock()
		return errors.New("forced initialization failure")
	}
	err := session.handoff(func() error { return nil })
	if err == nil || !strings.Contains(err.Error(), "forced initialization failure") {
		t.Fatalf("restart error = %v", err)
	}
	if session.reader != nil || app.terminal != nil || screen.Screen != nil {
		t.Fatal("failed restart retained an event reader or released screen")
	}
}
