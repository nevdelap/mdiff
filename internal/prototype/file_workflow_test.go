package prototype

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func TestStartupFilesUseIndependentBuffersEvenForSamePath(t *testing.T) {
	configDir := t.TempDir()
	path := filepath.Join(t.TempDir(), "shared.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(60, 12)
	app, err := NewWithFiles(sim, configDir, nil, path, path)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	if app.left.buf == app.right.buf || app.left.buf.Shared() || app.right.buf.Shared() {
		t.Fatal("the panes reused Micro's shared buffer for the same path")
	}
	if app.left.buf.Path != path || app.right.buf.Path != path {
		t.Fatalf("startup names differ from arguments: left=%q right=%q", app.left.buf.Path, app.right.buf.Path)
	}
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "left ")
	if got := string(app.left.buf.Bytes()); got != "left original\n" {
		t.Fatalf("left edit = %q", got)
	}
	if got := string(app.right.buf.Bytes()); got != "original\n" {
		t.Fatalf("left edit leaked into right buffer: %q", got)
	}
}

func TestSavingStaleDuplicatePathPanePromptsBeforeReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newWorkflowApp(t, path, path)
	defer app.Close()
	app.right.buf.Insert(buffer.Loc{X: 0, Y: 0}, "right ")
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "left ")
	app.handle(key(tcell.KeyCtrlS, 0, tcell.ModCtrl))
	if got := readWorkflowFile(t, path); got != "left original\n" {
		t.Fatalf("left pane save=%q", got)
	}

	app.focus = viewport.Right
	app.setFocus()
	app.handle(key(tcell.KeyCtrlS, 0, tcell.ModCtrl))
	if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "Reload file?") {
		t.Fatalf("stale pane save did not ask before reloading changed contents: %q", app.currentStatus())
	}
	if got := readWorkflowFile(t, path); got != "left original\n" {
		t.Fatalf("stale pane overwrote the first pane's saved changes before confirmation: %q", got)
	}
	app.handle(key(tcell.KeyRune, 'y', tcell.ModNone))
	if got := string(app.right.buf.Bytes()); got != "left original\n" || app.right.buf.Modified() {
		t.Fatalf("confirming reload did not adopt disk content: text=%q modified=%v", got, app.right.buf.Modified())
	}
	if got := readWorkflowFile(t, path); got != "left original\n" {
		t.Fatalf("reload confirmation changed the file: %q", got)
	}
}

func TestSavePromptsForExternalFileChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newWorkflowApp(t, path)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "local ")
	if err := os.WriteFile(path, []byte("external edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	app.handle(key(tcell.KeyCtrlS, 0, tcell.ModCtrl))
	if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "Reload file?") {
		t.Fatalf("save after external edit did not show Micro reload prompt: %q", app.currentStatus())
	}
	if got := readWorkflowFile(t, path); got != "external edit\n" {
		t.Fatalf("external file was overwritten before confirmation: %q", got)
	}
	app.handle(key(tcell.KeyRune, 'y', tcell.ModNone))
	if got := string(app.left.buf.Bytes()); got != "external edit\n" || app.left.buf.Modified() {
		t.Fatalf("confirming reload did not adopt external file: text=%q modified=%v", got, app.left.buf.Modified())
	}
}

func TestCancelExternalReloadPromptDoesNotSaveStaleBuffer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := newWorkflowApp(t, path)
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "local ")
	if err := os.WriteFile(path, []byte("external edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	app.handle(key(tcell.KeyCtrlS, 0, tcell.ModCtrl))
	if !action.InfoBar.HasPrompt {
		t.Fatal("save after external edit did not prompt")
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	if got := readWorkflowFile(t, path); got != "external edit\n" {
		t.Fatalf("canceling the reload prompt overwrote the external file: %q", got)
	}
	if got := string(app.left.buf.Bytes()); got != "local original\n" || !app.left.buf.Modified() {
		t.Fatalf("canceling the prompt changed the local buffer: text=%q modified=%v", got, app.left.buf.Modified())
	}
}

func TestDirtyReplacementSaveResolvesExternalChange(t *testing.T) {
	for _, operation := range []string{"open", "new tab"} {
		for _, choice := range []rune{'y', 'n', 0} {
			choiceName := map[rune]string{'y': "reload", 'n': "overwrite", 0: "cancel"}[choice]
			t.Run(operation+"/"+choiceName, func(t *testing.T) {
				oldPath, replacementPath := writeWorkflowFiles(t,
					"old.txt", "original old\n", "replacement.txt", "replacement text\n")
				app := newWorkflowApp(t, oldPath)
				defer app.Close()
				oldBuffer := app.left.buf
				oldBuffer.Insert(buffer.Loc{X: 0, Y: 0}, "local ")
				if err := os.WriteFile(oldPath, []byte("external edit\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				future := time.Now().Add(time.Minute)
				if err := os.Chtimes(oldPath, future, future); err != nil {
					t.Fatal(err)
				}

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
					t.Fatalf("%s did not ask how to handle dirty content: %q", operation, app.currentStatus())
				}
				app.handle(key(tcell.KeyRune, 'y', tcell.ModNone))
				if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "Reload file?") {
					t.Fatalf("%s save choice bypassed external-change prompt: %q", operation, app.currentStatus())
				}
				if got := readWorkflowFile(t, oldPath); got != "external edit\n" {
					t.Fatalf("%s overwrote external contents before resolution: %q", operation, got)
				}

				if choice == 0 {
					app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
					if app.left.buf != oldBuffer || app.left.buf.Path != oldPath || !oldBuffer.Modified() {
						t.Fatal("cancel did not preserve the original dirty document")
					}
					if got := readWorkflowFile(t, oldPath); got != "external edit\n" {
						t.Fatalf("cancel changed external contents: %q", got)
					}
					return
				}
				app.handle(key(tcell.KeyRune, choice, tcell.ModNone))
				wantOld := "external edit\n"
				if choice == 'n' {
					wantOld = "local original old\n"
				}
				if got := readWorkflowFile(t, oldPath); got != wantOld {
					t.Fatalf("%s old file=%q, want %q", choiceName, got, wantOld)
				}
				if operation == "open" {
					if app.left.buf.Path != replacementPath || string(app.left.buf.Bytes()) != "replacement text\n" {
						t.Fatal("Open callback did not run after external-change resolution")
					}
				} else if app.left.buf.Path != "" || app.left.buf.GetName() != "No name" {
					t.Fatal("New Tab callback did not run after external-change resolution")
				}
			})
		}
	}
}

func TestStartupSingleFileMapsToLeftPane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "single.txt")
	if err := os.WriteFile(path, []byte("single"), 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(60, 12)
	app, err := NewWithFiles(sim, t.TempDir(), nil, path)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if app.left.buf.Path != path || string(app.left.buf.Bytes()) != "single" {
		t.Fatalf("single startup file did not open on the left: path=%q text=%q", app.left.buf.Path, app.left.buf.Bytes())
	}
	if app.right.buf.Path != "" || app.right.buf.GetName() != "No name" {
		t.Fatalf("right pane is not a new unnamed document: path=%q name=%q", app.right.buf.Path, app.right.buf.GetName())
	}
}

func TestOpenSaveAndNewDocumentStayWithinFocusedPane(t *testing.T) {
	configDir := t.TempDir()
	leftPath := filepath.Join(t.TempDir(), "left.txt")
	rightPath := filepath.Join(t.TempDir(), "right.txt")
	openPath := filepath.Join(t.TempDir(), "opened.txt")
	for path, contents := range map[string]string{leftPath: "left", rightPath: "right", openPath: "opened"} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(60, 12)
	app, err := NewWithFiles(sim, configDir, nil, leftPath, rightPath)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	leftBuf, rightBuf := app.left.buf, app.right.buf
	app.focus = viewport.Right
	app.setFocus()
	app.handle(key(tcell.KeyCtrlO, 0, tcell.ModCtrl))
	for _, r := range openPath {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	if app.right.buf.Path != openPath || string(app.right.buf.Bytes()) != "opened" {
		t.Fatalf("Open did not replace the focused pane: path=%q text=%q", app.right.buf.Path, app.right.buf.Bytes())
	}
	if app.left.buf != leftBuf || string(app.left.buf.Bytes()) != "left" {
		t.Fatal("Open changed the opposite pane")
	}

	app.focus = viewport.Left
	app.setFocus()
	app.left.buf.Insert(buffer.Loc{X: 4, Y: 0}, " saved")
	app.handle(key(tcell.KeyCtrlS, 0, tcell.ModCtrl))
	saved, err := os.ReadFile(leftPath)
	if err != nil || string(saved) != "left saved\n" {
		t.Fatalf("Save did not write focused file: text=%q err=%v", saved, err)
	}
	if got := string(app.right.buf.Bytes()); got != "opened" || app.right.buf.Path != openPath {
		t.Fatalf("saving left pane changed right pane: path=%q text=%q", app.right.buf.Path, got)
	}

	app.handle(key(tcell.KeyCtrlT, 0, tcell.ModCtrl))
	app.handle(key(tcell.KeyRune, 'n', tcell.ModNone)) // Discard the modified left buffer.
	if app.left.buf == leftBuf || app.left.buf.GetName() != "No name" || app.left.buf.LinesNum() != 1 {
		t.Fatalf("New Tab did not create a new unnamed left document: %q", app.left.buf.GetName())
	}
	if app.right.buf.Path != openPath || rightBuf == app.right.buf {
		t.Fatal("New Tab changed the right pane")
	}
}

func TestOpenDirectoryFailurePreservesFocusedBuffer(t *testing.T) {
	configDir := t.TempDir()
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(60, 12)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	before := app.left.buf
	directory := t.TempDir()
	app.handle(key(tcell.KeyCtrlO, 0, tcell.ModCtrl))
	for _, r := range directory {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	if app.left.buf != before || app.left.buf.Path != "" {
		t.Fatal("failed Open replaced or renamed the old buffer")
	}
	if !strings.Contains(strings.ToLower(app.currentStatus()), "directory") {
		t.Fatalf("Open error was not reported in Micro status UI: %q", app.currentStatus())
	}
	if action.InfoBar.HasPrompt {
		t.Fatal("failed Open left an unexpected prompt active")
	}
}

func TestOpenModifiedBufferPromptsAndCancelKeepsExistingDocument(t *testing.T) {
	configDir := t.TempDir()
	path := filepath.Join(t.TempDir(), "replacement.txt")
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(60, 12)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "keep")
	before := app.left.buf
	app.handle(key(tcell.KeyCtrlO, 0, tcell.ModCtrl))
	for _, r := range path {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	if !action.InfoBar.HasPrompt || !strings.Contains(app.currentStatus(), "Save changes to No name (left pane)") {
		t.Fatalf("modified Open did not prompt to save changes: %q", app.currentStatus())
	}
	app.handle(key(tcell.KeyEscape, 0, tcell.ModNone))
	if app.left.buf != before || string(app.left.buf.Bytes()) != "keep" {
		t.Fatal("canceling dirty Open did not preserve the current document")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("canceling Open changed the target file:", err)
	}
}

func TestSaveAsFromUnnamedBufferWritesAndSetsOnlyFocusedPath(t *testing.T) {
	configDir := t.TempDir()
	path := filepath.Join(t.TempDir(), "new-file.txt")
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(60, 12)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	left, right := app.left.buf, app.right.buf
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "new content")
	app.handle(key(tcell.KeyCtrlS, 0, tcell.ModCtrl))
	for _, r := range path {
		app.handle(key(tcell.KeyRune, r, tcell.ModNone))
	}
	app.handle(key(tcell.KeyEnter, 0, tcell.ModNone))
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "new content\n" {
		t.Fatalf("Save As content=%q err=%v", contents, err)
	}
	if app.left.buf != left || app.left.buf.Path != path || app.right.buf != right || app.right.buf.Path != "" {
		t.Fatal("Save As did not update only the focused pane's file identity")
	}
}

func TestNewWithFilesRejectsMoreThanTwoBeforeInitializingScreen(t *testing.T) {
	_, err := NewWithFiles(nil, t.TempDir(), nil, "left", "right", "extra")
	if err == nil || !strings.Contains(err.Error(), "at most two") {
		t.Fatalf("too many paths error = %v", err)
	}
}
