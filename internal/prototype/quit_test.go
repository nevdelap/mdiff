package prototype

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
)

func TestCtrlQDoesNotBypassActivePrompts(t *testing.T) {
	app := newWorkflowApp(t)
	defer app.Close()

	for _, prompt := range []struct {
		name string
		open func()
	}{
		{"save filename", func() { app.handle(key(tcell.KeyCtrlS, 0, tcell.ModCtrl)) }},
		{"open filename", func() { app.handle(key(tcell.KeyCtrlO, 0, tcell.ModCtrl)) }},
		{"dirty buffer", func() {
			app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "dirty")
			app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl))
		}},
	} {
		t.Run(prompt.name, func(t *testing.T) {
			prompt.open()
			if !action.InfoBar.HasPrompt {
				t.Fatal("expected prompt was not opened")
			}
			if quit := app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl)); quit {
				t.Fatal("Ctrl+Q bypassed an active Micro prompt")
			}
			if action.InfoBar.HasPrompt || app.quitComplete {
				t.Fatal("Ctrl+Q did not cancel the prompt or quit the application")
			}
			if prompt.name == "dirty buffer" && (app.quitFlow != nil || !app.left.buf.Modified()) {
				t.Fatal("canceling dirty quit did not keep the modified document open")
			}
		})
	}
}

func TestQuitStagesBothPaneDecisionsBeforeSaving(t *testing.T) {
	configDir := t.TempDir()
	leftPath, rightPath := writeWorkflowFiles(t, "left.txt", "left original\n", "right.txt", "right original\n")
	app := newWorkflowAppWithConfig(t, configDir, leftPath, rightPath)
	defer app.Close()
	left, right := app.left.buf, app.right.buf
	left.Insert(buffer.Loc{X: 0, Y: 0}, "left edited ")
	right.Insert(buffer.Loc{X: 0, Y: 0}, "right edited ")
	leftText, rightText := string(left.Bytes()), string(right.Bytes())

	if app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl)) || !action.InfoBar.HasPrompt {
		t.Fatal("Ctrl+Q did not ask about the first dirty pane")
	}
	app.handle(key(tcell.KeyRune, 'y', tcell.ModNone))
	if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "right.txt") {
		t.Fatalf("first save choice did not advance to the right pane: %q", app.currentStatus())
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))

	if app.quitComplete || app.left.buf != left || app.right.buf != right || !left.Modified() || !right.Modified() {
		t.Fatal("canceling the second pane closed or cleaned either modified buffer")
	}
	if string(left.Bytes()) != leftText || string(right.Bytes()) != rightText {
		t.Fatal("canceling quit changed in-memory buffer contents")
	}
	if got := readWorkflowFile(t, leftPath); got != "left original\n" {
		t.Fatalf("staged save changed left file before cancel: %q", got)
	}
	if got := readWorkflowFile(t, rightPath); got != "right original\n" {
		t.Fatalf("cancel changed right file: %q", got)
	}
}

func TestQuitNamesEachUnnamedPaneInItsSavePrompt(t *testing.T) {
	app := newWorkflowApp(t)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "left edits")
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "right edits")

	if app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl)) {
		t.Fatal("quit completed before asking about modified panes")
	}
	if got := app.currentStatus(); !strings.Contains(got, "left pane (No name)") {
		t.Fatalf("first prompt does not identify the left pane: %q", got)
	}
	app.handle(key(tcell.KeyRune, 'n', tcell.ModNone))
	if got := app.currentStatus(); !strings.Contains(got, "right pane (No name)") {
		t.Fatalf("second prompt does not identify the right pane: %q", got)
	}
	if !app.handle(key(tcell.KeyRune, 'n', tcell.ModNone)) || !app.quitComplete {
		t.Fatal("discarding both unnamed panes did not complete quit")
	}
}

func TestQuitSavesOrDiscardsBothModifiedPanes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		leftChoice  rune
		rightChoice rune
		leftSaved   bool
		rightSaved  bool
	}{
		{"save both", 'y', 'y', true, true},
		{"discard both", 'n', 'n', false, false},
		{"save left discard right", 'y', 'n', true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			leftPath, rightPath := writeWorkflowFiles(t, "left.txt", "left original\n", "right.txt", "right original\n")
			app := newWorkflowApp(t, leftPath, rightPath)
			defer app.Close()
			app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "left edited ")
			app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "right edited ")
			if app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl)) {
				t.Fatal("dirty quit completed before decisions")
			}
			app.handle(key(tcell.KeyRune, tc.leftChoice, tcell.ModNone))
			quit := app.handle(key(tcell.KeyRune, tc.rightChoice, tcell.ModNone))
			if !quit || !app.quitComplete {
				t.Fatal("quit did not complete after both pane decisions")
			}
			leftWant, rightWant := "left original\n", "right original\n"
			if tc.leftSaved {
				leftWant = "left edited left original\n"
			}
			if tc.rightSaved {
				rightWant = "right edited right original\n"
			}
			if got := readWorkflowFile(t, leftPath); got != leftWant {
				t.Errorf("left file=%q, want %q", got, leftWant)
			}
			if got := readWorkflowFile(t, rightPath); got != rightWant {
				t.Errorf("right file=%q, want %q", got, rightWant)
			}
		})
	}
}

func TestQuitUnnamedBufferSaveDiscardAndCancel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		choice rune
		save   bool
		cancel bool
	}{
		{"save as", 'y', true, false},
		{"discard", 'n', false, false},
		{"cancel", 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newWorkflowApp(t)
			defer app.Close()
			app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "unsaved\n")
			before := app.left.buf
			if app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl)) {
				t.Fatal("unnamed dirty quit completed without a choice")
			}
			if tc.cancel {
				app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
				if app.quitComplete || app.left.buf != before || !before.Modified() || string(before.Bytes()) != "unsaved\n" {
					t.Fatal("cancel did not leave the unnamed buffer intact")
				}
				return
			}
			quit := app.handle(key(tcell.KeyRune, tc.choice, tcell.ModNone))
			var target string
			if tc.save {
				target = filepath.Join(t.TempDir(), "quit-save.txt")
				if !action.InfoBar.HasPrompt {
					t.Fatal("Save did not prompt for the unnamed buffer path")
				}
				for _, r := range target {
					app.handle(key(tcell.KeyRune, r, tcell.ModNone))
				}
				quit = app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
				if !quit || !app.quitComplete {
					t.Fatal("quit did not finish after unnamed-buffer Save As")
				}
				if got := readWorkflowFile(t, target); got != "unsaved\n" {
					t.Fatalf("Save As content=%q", got)
				}
			} else {
				if !quit || !app.quitComplete || app.left.buf != before || app.left.buf.Path != "" {
					t.Fatal("discard did not complete quit while keeping the unnamed document in memory")
				}
			}
		})
	}
}

func TestCtrlQHonorsAutosaveForNamedModifiedBuffer(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"autosave": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "autosaved.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newWorkflowAppWithConfig(t, configDir, path)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "changed ")
	if !app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl)) || action.InfoBar.HasPrompt {
		t.Fatal("configured autosave did not save and quit without prompting")
	}
	if got := readWorkflowFile(t, path); got != "changed original\n" {
		t.Fatalf("autosaved content=%q", got)
	}
}

func TestAutosaveQuitChecksStaleDuplicatePathBuffer(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"autosave": 1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "shared.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newWorkflowAppWithConfig(t, configDir, path, path)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "left ")
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "right ")

	if app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl)) {
		t.Fatal("autosave quit completed before checking the stale second buffer")
	}
	if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "Reload file?") {
		t.Fatalf("autosave quit did not check the second buffer's external change: %q", app.currentStatus())
	}
	if got := readWorkflowFile(t, path); got != "left original\n" {
		t.Fatalf("stale second buffer overwrote the first save before a choice: %q", got)
	}
	app.handle(key(tcell.KeyRune, 'y', tcell.ModNone))
	if !app.quitComplete || app.left.buf.Modified() || app.right.buf.Modified() {
		t.Fatal("confirming reload did not finish autosave quit with clean buffers")
	}
	if got := readWorkflowFile(t, path); got != "left original\n" {
		t.Fatalf("reloading the latest disk version changed it: %q", got)
	}
}

func TestDirtyQuitResolvesStaleDuplicatePathSave(t *testing.T) {
	for _, choice := range []rune{'y', 'n', 0} {
		choiceName := map[rune]string{'y': "reload", 'n': "overwrite", 0: "cancel"}[choice]
		t.Run(choiceName, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "shared.txt")
			if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			app := newWorkflowApp(t, path, path)
			defer app.Close()
			app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "left ")
			app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "right ")

			app.handle(key(tcell.KeyCtrlQ, 0, tcell.ModCtrl))
			app.handle(key(tcell.KeyRune, 'y', tcell.ModNone)) // Save left.
			app.handle(key(tcell.KeyRune, 'y', tcell.ModNone)) // Save right.
			if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "Reload file?") {
				t.Fatalf("second quit save bypassed external-change prompt: %q", app.currentStatus())
			}
			if got := readWorkflowFile(t, path); got != "left original\n" {
				t.Fatalf("stale second quit save overwrote first before confirmation: %q", got)
			}

			if choice == 0 {
				app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
				if app.quitComplete || app.quitFlow != nil || !app.right.buf.Modified() {
					t.Fatal("canceling external-change prompt did not abort quit and preserve the second buffer")
				}
				if got := readWorkflowFile(t, path); got != "left original\n" {
					t.Fatalf("cancel changed the first save: %q", got)
				}
				return
			}

			app.handle(key(tcell.KeyRune, choice, tcell.ModNone))
			if !app.quitComplete {
				t.Fatal("quit did not complete after resolving the stale second save")
			}
			want := "left original\n"
			if choice == 'n' {
				want = "right original\n"
			}
			if got := readWorkflowFile(t, path); got != want {
				t.Fatalf("resolved quit save=%q, want %q", got, want)
			}
		})
	}
}

func TestDirtyOpenAndNewTabSaveDiscardAndCancel(t *testing.T) {
	for _, operation := range []string{"open", "new tab"} {
		for _, choice := range []rune{'y', 'n', 0} {
			name := map[rune]string{'y': "save", 'n': "discard", 0: "cancel"}[choice]
			t.Run(operation+"/"+name, func(t *testing.T) {
				oldPath, replacementPath := writeWorkflowFiles(t,
					"old.txt", "original old\n", "replacement.txt", "replacement text\n")
				app := newWorkflowApp(t, oldPath)
				defer app.Close()
				oldBuffer := app.left.buf
				oldBuffer.Insert(buffer.Loc{X: 0, Y: 0}, "edited ")
				oldText := string(oldBuffer.Bytes())

				if operation == "open" {
					app.handle(key(tcell.KeyCtrlO, 0, tcell.ModCtrl))
					for _, r := range replacementPath {
						app.handle(key(tcell.KeyRune, r, tcell.ModNone))
					}
					app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
				} else {
					app.handle(key(tcell.KeyCtrlT, 0, tcell.ModCtrl))
				}
				if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "Save changes") {
					t.Fatalf("%s did not prompt before replacing dirty content: %q", operation, app.currentStatus())
				}
				if choice == 0 {
					app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
					if app.left.buf != oldBuffer || app.left.buf.Path != oldPath || string(oldBuffer.Bytes()) != oldText {
						t.Fatal("Cancel changed the original buffer or path")
					}
					if got := readWorkflowFile(t, oldPath); got != "original old\n" {
						t.Fatalf("Cancel changed old file: %q", got)
					}
					return
				}
				app.handle(key(tcell.KeyRune, choice, tcell.ModNone))
				if choice == 'y' {
					if got := readWorkflowFile(t, oldPath); got != "edited original old\n" {
						t.Fatalf("Save did not persist the old buffer: %q", got)
					}
				} else if got := readWorkflowFile(t, oldPath); got != "original old\n" {
					t.Fatalf("Discard changed old file: %q", got)
				}
				if operation == "open" {
					if app.left.buf == oldBuffer || app.left.buf.Path != replacementPath || string(app.left.buf.Bytes()) != "replacement text\n" {
						t.Fatal("Open did not replace only the focused pane with the requested file")
					}
				} else if app.left.buf == oldBuffer || app.left.buf.Path != "" || app.left.buf.GetName() != "No name" {
					t.Fatal("New Tab did not replace the focused pane with a new unnamed document")
				}
			})
		}
	}
}

func TestFailedSaveAsKeepsUnnamedBufferAndPath(t *testing.T) {
	app := newWorkflowApp(t)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "unsaved\n")
	before := app.left.buf
	content := string(before.Bytes())
	missingParent := filepath.Join(t.TempDir(), "missing", "failed-save.txt")
	app.handle(key(tcell.KeyCtrlS, 0, tcell.ModCtrl))
	for _, r := range missingParent {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	if app.left.buf != before || app.left.buf.Path != "" || string(before.Bytes()) != content || !before.Modified() {
		t.Fatal("failed Save As changed the buffer, dirty state, or path")
	}
	if !strings.Contains(app.currentStatus(), "Parent dirs don't exist") {
		t.Fatalf("failed Save As error was not reported in Micro status UI: %q", app.currentStatus())
	}
	if _, err := os.Stat(missingParent); !os.IsNotExist(err) {
		t.Fatalf("failed Save As created its target, stat error=%v", err)
	}
}

func TestFailedSaveDuringDirtyOpenKeepsOriginalBufferAndTarget(t *testing.T) {
	oldPath, replacementPath := writeWorkflowFiles(t,
		"old.txt", "original old\n", "replacement.txt", "replacement text\n")
	app := newWorkflowApp(t, oldPath)
	defer app.Close()
	oldBuffer := app.left.buf
	oldBuffer.Insert(buffer.Loc{X: 0, Y: 0}, "edited ")
	oldText := string(oldBuffer.Bytes())
	if err := os.Remove(oldPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(oldPath, 0o700); err != nil {
		t.Fatal(err)
	}
	app.handle(key(tcell.KeyCtrlO, 0, tcell.ModCtrl))
	for _, r := range replacementPath {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	app.handle(key(tcell.KeyRune, 'y', tcell.ModNone))
	if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "Reload file?") {
		t.Fatalf("failed-save fixture did not expose the external-change decision: %q", app.currentStatus())
	}
	app.handle(key(tcell.KeyRune, 'n', tcell.ModNone)) // Keep the buffer and attempt the failing write.
	if app.left.buf != oldBuffer || app.left.buf.Path != oldPath || string(oldBuffer.Bytes()) != oldText {
		t.Fatal("failed prompted save replaced or changed the original buffer")
	}
	if !strings.Contains(strings.ToLower(app.currentStatus()), "directory and cannot be saved") {
		t.Fatalf("write error was not reported in Micro status UI: %q", app.currentStatus())
	}
	if got := readWorkflowFile(t, replacementPath); got != "replacement text\n" {
		t.Fatalf("failed save changed the requested Open target: %q", got)
	}
}

func newWorkflowApp(t *testing.T, paths ...string) *App {
	t.Helper()
	return newWorkflowAppWithConfig(t, t.TempDir(), paths...)
}

func newWorkflowAppWithConfig(t *testing.T, configDir string, paths ...string) *App {
	t.Helper()
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(72, 14)
	app, err := NewWithFiles(sim, configDir, nil, paths...)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func writeWorkflowFiles(t *testing.T, leftName, leftText, rightName, rightText string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	leftPath, rightPath := filepath.Join(dir, leftName), filepath.Join(dir, rightName)
	for path, text := range map[string]string{leftPath: leftText, rightPath: rightText} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return leftPath, rightPath
}

func readWorkflowFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
