package prototype

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
)

func TestMicroSettingsApplyToBothPanesAndRemainReadOnly(t *testing.T) {
	configDir := t.TempDir()
	settings := []byte(`{
  "autoindent": false,
  "tabsize": 8,
  "tabstospaces": true,
  "softwrap": true,
  "ruler": false,
  "relativeruler": true,
  "savecursor": true,
  "saveundo": true,
  "filetype": "go",
  "colorscheme": "monokai",
  "mdiff": {"diff-debounce-ms": 120}
}`)
	bindings := []byte(`{"buffer":{"Ctrl-u":"Undo"}}`)
	settingsPath, bindingsPath := filepath.Join(configDir, "settings.json"), filepath.Join(configDir, "bindings.json")
	if err := os.WriteFile(settingsPath, settings, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bindingsPath, bindings, 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(100, 10)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := app.left.buf.Settings["tabsize"]; got != float64(8) {
		t.Fatalf("left tabsize = %v", got)
	}
	for _, pane := range []*pane{&app.left, &app.right} {
		for name, want := range map[string]any{
			"autoindent": false, "tabsize": float64(8), "tabstospaces": true,
			"softwrap": true, "ruler": false, "relativeruler": true,
		} {
			if got := pane.buf.Settings[name]; got != want {
				t.Errorf("%s pane setting %s = %v, want %v", pane.buf.Path, name, got, want)
			}
		}
		if pane.buf.FileType() != "go" || pane.buf.SyntaxDef == nil {
			t.Errorf("synthetic buffer filetype/syntax = %q / %v", pane.buf.FileType(), pane.buf.SyntaxDef)
		}
	}
	if got, err := configuredDebounce(nil); err != nil || got != 120000000 {
		t.Fatalf("configured mdiff debounce = %v, %v", got, err)
	}
	if labels := app.shortcutBar(1); !strings.Contains(labels, "^U Undo") {
		t.Fatalf("remapped undo shortcut missing: %q", labels)
	}
	if got := microMenuStyle(); got != config.Colorscheme["statusline"] {
		t.Fatal("key menu does not use the active Micro statusline colors")
	}
	app.draw()
	_, _, renderedStyle, _ := sim.GetContent(0, app.menuRow1)
	if renderedStyle != config.Colorscheme["statusline"] {
		t.Fatal("rendered shortcut row does not use the Micro statusline colors")
	}
	app.handle(tcell.NewEventPaste("edited in memory", ""))
	app.Close()
	for path, want := range map[string][]byte{settingsPath: settings, bindingsPath: bindings} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Errorf("config file %s changed: %q (err %v)", path, got, err)
		}
	}
	entries, err := os.ReadDir(configDir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("startup created config artifacts: entries=%v err=%v", entries, err)
	}
}

func TestMalformedCustomSyntaxReportsDiagnosticAndFallsBack(t *testing.T) {
	configDir := t.TempDir()
	settings := []byte(`{"filetype":"mdiff-broken"}`)
	syntax := []byte("filetype: [this is not valid YAML")
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), settings, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(configDir, "syntax"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "syntax", "broken.yaml"), syntax, 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(100, 10)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if !strings.Contains(app.diagnostic, "syntax file") {
		t.Fatalf("syntax error was not reported: %q", app.diagnostic)
	}
	if app.left.buf.FileType() == "" || app.left.buf.SyntaxDef == nil {
		t.Fatal("invalid user syntax prevented the built-in default syntax fallback")
	}
}

func TestMissingColorschemeFailsBeforeTerminalInitialization(t *testing.T) {
	configDir := t.TempDir()
	contents := []byte(`{"colorscheme":"does-not-exist"}`)
	path := filepath.Join(configDir, "settings.json")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 10)
	if _, err := New(sim, configDir); err == nil || !strings.Contains(err.Error(), "colorscheme") {
		t.Fatalf("missing colorscheme error = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(contents) {
		t.Fatalf("invalid colorscheme settings were changed: %q, %v", got, err)
	}
}

func TestShortcutMenuFallsBackWhenColorschemeHasNoStatusline(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(configDir, "colorschemes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "colorschemes", "empty.micro"), []byte("# valid scheme without overrides\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"colorscheme":"empty"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(100, 10)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if got, want := microMenuStyle(), config.DefStyle.Reverse(true); got != want {
		t.Fatalf("menu fallback style = %v, want %v", got, want)
	}
}

func TestMalformedSettingsFailBeforeTerminalInitialization(t *testing.T) {
	configDir := t.TempDir()
	path := filepath.Join(configDir, "settings.json")
	contents := []byte(`{"tabsize":`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 10)
	if _, err := New(sim, configDir); err == nil || !strings.Contains(err.Error(), "settings.json") {
		t.Fatalf("malformed settings error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(contents) {
		t.Fatalf("malformed settings file changed: %q, %v", got, err)
	}
}

func TestMalformedBindingsAreReportedWithoutBlockingStartup(t *testing.T) {
	configDir := t.TempDir()
	path := filepath.Join(configDir, "bindings.json")
	contents := []byte(`{"buffer":`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(100, 10)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if !strings.Contains(app.diagnostic, "Error reading bindings.json") {
		t.Fatalf("startup diagnostic = %q", app.diagnostic)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(contents) {
		t.Fatalf("malformed bindings file changed: %q, %v", got, err)
	}
}

func TestShortcutLabelsFollowRemovedAndRemappedActions(t *testing.T) {
	configDir := t.TempDir()
	bindings := []byte(`{"buffer":{"Ctrl-z":"","Ctrl-u":"Undo","Ctrl-r":"ShellMode"}}`)
	if err := os.WriteFile(filepath.Join(configDir, "bindings.json"), bindings, 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(100, 10)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	labels := app.shortcutBar(1)
	if !strings.Contains(labels, "^U Undo") || strings.Contains(labels, "^Z Undo") || strings.Contains(labels, "ShellMode") {
		t.Fatalf("shortcut labels did not follow binding configuration: %q", labels)
	}
}

func TestConfiguredBackupOptionControlsDocumentArtifacts(t *testing.T) {
	for _, backup := range []string{"false", "true"} {
		t.Run(backup, func(t *testing.T) {
			configDir := t.TempDir()
			settings := []byte(`{"backup":` + backup + `}`)
			if err := os.WriteFile(filepath.Join(configDir, "settings.json"), settings, 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "document.txt")
			if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			sim := tcell.NewSimulationScreen("")
			if err := sim.Init(); err != nil {
				t.Fatal(err)
			}
			sim.SetSize(80, 10)
			app, err := NewWithFiles(sim, configDir, nil, path)
			if err != nil {
				t.Fatal(err)
			}
			app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, "after ")
			if err := app.left.buf.Backup(); err != nil {
				t.Fatal(err)
			}
			backups := filepath.Join(configDir, "backups")
			entries, err := os.ReadDir(backups)
			if backup == "true" && (err != nil || len(entries) == 0) {
				t.Fatalf("enabled backup was not created: entries=%v err=%v", entries, err)
			}
			if backup == "false" && !os.IsNotExist(err) {
				t.Fatalf("disabled backup created artifacts: entries=%v err=%v", entries, err)
			}
			app.Close()
			if got, err := os.ReadFile(filepath.Join(configDir, "settings.json")); err != nil || string(got) != string(settings) {
				t.Fatalf("backup workflow changed Micro settings: %q, %v", got, err)
			}
		})
	}
}
