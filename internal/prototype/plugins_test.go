package prototype

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func TestConfiguredMicroPluginLoadsAndEditsFocusedPane(t *testing.T) {
	configDir := t.TempDir()
	pluginDir := filepath.Join(configDir, "plug", "fixture")
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(configDir, "plugin-side-effect.txt")
	initLua := `local config = import("micro/config")
function init()
    config.RegisterGlobalOption("mdiff.test", "initlua", true)
end
`
	if err := os.WriteFile(filepath.Join(configDir, "init.lua"), []byte(initLua), 0o600); err != nil {
		t.Fatal(err)
	}
	bindings := []byte(`{"buffer":{"Ctrl-r":"lua:fixture.onKey"}}`)
	if err := os.WriteFile(filepath.Join(configDir, "bindings.json"), bindings, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"colorscheme":"plugtheme"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	plugin := fmt.Sprintf(`local buffer = import("micro/buffer")
local micro = import("micro")
local config = import("micro/config")
local shell = import("micro/shell")
local ioutil = import("io/ioutil")
local initialized = false
local marker = %q

function init()
    initialized = true
    config.AddRuntimeFileFromMemory(config.RTColorscheme, "plugtheme", 'include "monokai"')
end

function onBufferOpen(buf)
    if not initialized then
        error("init did not run before onBufferOpen")
    end
    local output, err = shell.ExecCommand("go", "version")
    if err ~= nil then
        error("plugin process invocation failed: " .. tostring(err))
    end
    local writeErr = ioutil.WriteFile(marker, output, 384)
    if writeErr ~= nil then
        error("plugin file write failed: " .. tostring(writeErr))
    end
    buf:Insert(buffer.Loc(0, 0), "opened:")
end

function onRune(bp, r)
    if r == "!" then
        if micro.CurPane().Buf ~= bp.Buf then
            error("CurPane did not follow mdiff focus")
        end
        bp.Buf:Insert(-bp.Cursor.Loc, "plugin")
    end
    return true
end

function onKey(bp)
    if micro.CurPane().Buf ~= bp.Buf then
        error("configured Lua action did not target the focused pane")
    end
    bp.Buf:Insert(-bp.Cursor.Loc, "bound")
    return true
end
`, markerPath)
	if err := os.WriteFile(filepath.Join(pluginDir, "fixture.lua"), []byte(plugin), 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 12)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("plugin file/process side effects were blocked: %v", err)
	}
	pluginLoaded := false
	for _, p := range config.Plugins {
		if p.Name == "fixture" {
			if !p.Loaded {
				t.Fatalf("fixture plugin did not load: %#v", p)
			}
			pluginLoaded = true
		}
	}
	if !pluginLoaded {
		t.Fatal("configured fixture plugin was not discovered")
	}
	if config.GlobalSettings["mdiff.test.initlua"] != true {
		t.Fatalf("Micro init.lua did not run during plugin startup: setting=%v plugins=%+v diagnostics=%q", config.GlobalSettings["mdiff.test.initlua"], config.Plugins, app.diagnostic)
	}
	if config.GlobalSettings["colorscheme"] != "plugtheme" || len(config.Colorscheme) == 0 {
		t.Fatalf("plugin-provided colorscheme was not applied: setting=%v scheme=%v", config.GlobalSettings["colorscheme"], config.Colorscheme)
	}
	if config.FindPlugin("comment") == nil {
		t.Fatal("bundled Micro comment plugin was not loaded")
	}

	for side, p := range app.panes() {
		if got := p.buf.Line(0); got != "opened:" {
			t.Errorf("pane %d onBufferOpen content = %q, want %q", side, got, "opened:")
		}
	}
	app.focus = viewport.Right
	app.setFocus()
	versionBefore := app.versions.Right
	leftVersionBefore := app.versions.Left
	if quit := app.handle(key(tcell.KeyCtrlR, 0, tcell.ModCtrl)); quit {
		t.Fatal("configured plugin action terminated mdiff")
	}
	if got := app.right.buf.Line(0); !strings.Contains(got, "bound") {
		t.Fatalf("configured Lua action did not edit the focused pane: %q", got)
	}
	if quit := app.handle(key(tcell.KeyRune, '!', tcell.ModNone)); quit {
		t.Fatal("plugin rune callback terminated mdiff")
	}
	if got := app.right.buf.Line(0); !strings.Contains(got, "plugin") {
		t.Fatalf("focused pane after plugin callback = %q, want plugin edit", got)
	}
	if got := app.left.buf.Line(0); got != "opened:" {
		t.Fatalf("unfocused pane changed after plugin callback: %q", got)
	}
	if app.versions.Right <= versionBefore || app.versions.Left != leftVersionBefore {
		t.Fatalf("plugin edit versions = %+v, want right advanced and left unchanged", app.versions)
	}
	flushDiffForTest(t, app)
	if app.displayedLeft[0] == app.displayedRight[0] {
		t.Fatal("plugin edit did not update the installed diff snapshot")
	}
}

func TestMicroPluginLayoutMutationIsReportedWithoutChangingPairedPanes(t *testing.T) {
	app, _ := newEditingTestApp(t, 80, 12)
	defer app.Close()

	newBuffer := buffer.NewBufferFromString("extra", "", buffer.BTDefault)
	defer newBuffer.Close()
	if got := app.left.h.VSplitBuf(newBuffer); got != nil {
		t.Fatal("plugin split unexpectedly created an independent pane")
	}
	app.microTab.AddPane(nil, len(app.microTab.Panes))
	action.Tabs.AddTab(&action.Tab{})
	if got := len(app.microTab.Panes); got != 2 {
		t.Fatalf("plugin changed paired pane count to %d, want 2", got)
	}
	if got := len(action.Tabs.List); got != 1 {
		t.Fatalf("plugin changed Micro tab count to %d, want one paired tab", got)
	}
	if status := app.currentStatus(); !strings.Contains(status, "mdiff: plugins cannot") {
		t.Fatalf("unavailable layout operation was not reported: %q", status)
	}
}

func TestPluginTryBindKeyPersistsOnlyWhenRequested(t *testing.T) {
	configDir := t.TempDir()
	pluginDir := filepath.Join(configDir, "plug", "bindingfixture")
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	plugin := `local config = import("micro/config")
local ioutil = import("io/ioutil")
function init()
    local _, readErr = ioutil.ReadFile(config.ConfigDir .. "/bindings.json")
    if readErr == nil then
        error("mdiff created bindings.json before an explicit plugin request")
    end
    local ok, bindErr = config.TryBindKey("Ctrl-Alt-z", "lua:bindingfixture.onKey", true)
    if bindErr ~= nil or not ok then
        error("TryBindKey failed: " .. tostring(bindErr))
    end
end
function onKey()
    return true
end
`
	if err := os.WriteFile(filepath.Join(pluginDir, "bindingfixture.lua"), []byte(plugin), 0o600); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 12)
	app, err := New(sim, configDir)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	active := false
	for _, got := range config.Bindings["buffer"] {
		if got == "lua:bindingfixture.onKey" {
			active = true
			break
		}
	}
	if !active {
		t.Fatalf("plugin binding is not active: bindings=%v", config.Bindings["buffer"])
	}
	bindingsPath := filepath.Join(configDir, "bindings.json")
	contents, err := os.ReadFile(bindingsPath)
	if err != nil {
		t.Fatalf("plugin binding was not persisted: %v", err)
	}
	if !strings.Contains(string(contents), "Ctrl-Alt-z") || !strings.Contains(string(contents), "lua:bindingfixture.onKey") {
		t.Fatalf("persisted bindings do not contain requested plugin binding: %s", contents)
	}
}

func TestPluginDeinitRunsWhenInitialPaneOpenFails(t *testing.T) {
	configDir := t.TempDir()
	pluginDir := filepath.Join(configDir, "plug", "deinitfixture")
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(configDir, "deinit-ran.txt")
	plugin := fmt.Sprintf(`local ioutil = import("io/ioutil")
local marker = %q
function deinit()
    local err = ioutil.WriteFile(marker, "deinit", 384)
    if err ~= nil then
        error("deinit marker write failed: " .. tostring(err))
    end
end
`, markerPath)
	if err := os.WriteFile(filepath.Join(pluginDir, "deinitfixture.lua"), []byte(plugin), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(configDir, "not-a-file"), 0o700); err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 12)
	if _, err := NewWithFiles(sim, configDir, nil, filepath.Join(configDir, "not-a-file")); err == nil {
		t.Fatal("opening a directory as the initial pane unexpectedly succeeded")
	}
	contents, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("plugin deinit was not run after pane-open failure: %v", err)
	}
	if string(contents) != "deinit" {
		t.Fatalf("deinit marker = %q, want deinit", contents)
	}
}
