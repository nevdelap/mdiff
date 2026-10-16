// Package prototype contains mdiff's production Micro-backed editor.
package prototype

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/compare"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
	ulua "github.com/nevdelap/mdiff/internal/micro/lua"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/micro/shell"
	"github.com/nevdelap/mdiff/internal/viewport"
	lua "github.com/yuin/gopher-lua"
)

type pane struct {
	buf        *buffer.Buffer
	view       *display.BufWindow
	statusView *display.BufWindow
	h          *action.BufPane
	x          int
	width      int
}

type paneLayoutKey struct {
	width    int
	buffer   int
	softwrap bool
	wordwrap bool
	tabsize  int
}

type appLayoutKey struct {
	left, right paneLayoutKey
}

type quitFlow struct {
	panes        []*pane
	paneIndex    int
	saves        []*pane
	saveIndex    int
	awaitingSave bool
}

// App owns the two Micro buffers, panes, alignment, and shared viewport.
type App struct {
	left, right      pane
	layout           viewport.Layout
	layoutKey        appLayoutKey
	terminal         tcell.Screen
	surface          tcell.SimulationScreen
	paintScreen      tcell.SimulationScreen
	top              int
	rows             int
	cols             int
	statusRow        int
	menuRow1         int
	menuRow2         int
	focus            viewport.Side
	closed           bool
	coordinator      *diffCoordinator
	lineHeights      *lineHeightCache
	versions         DiffVersions
	layoutVersions   DiffVersions
	pairs            []compare.Pair
	displayedLeft    []string
	displayedRight   []string
	pendingAnchor    anchor
	hasPendingAnchor bool
	diffErr          error
	diffWarning      string
	handlingPaste    bool
	blockedSequence  string
	quitFlow         *quitFlow
	quitComplete     bool
	diagnostic       string
	microTab         *action.Tab
	pluginTimers     chan func()
	pluginStop       chan struct{}
	mousePane        *pane
	preserveViewport bool
	helpVisible      bool
	helpTopic        string
	helpText         string
	helpTop          int
	helpQuery        string
	helpMatch        int
	handoff          func(func() error) error
	terminalErr      error
}

// New initializes Micro's in-memory settings and real buffers/actions. Passing
// nil as the screen initializes the real tcell terminal; tests pass a
// SimulationScreen instead.
func New(s tcell.Screen, configDir string) (*App, error) {
	return NewWithDebounce(s, configDir, nil)
}

// NewWithDebounce constructs the app with an optional CLI debounce override.
func NewWithDebounce(s tcell.Screen, configDir string, cliDebounceMS *int) (*App, error) {
	return NewWithFiles(s, configDir, cliDebounceMS)
}

// NewWithFiles constructs the app and optionally loads one file into the left
// pane and a second file into the right pane.
func NewWithFiles(s tcell.Screen, configDir string, cliDebounceMS *int, paths ...string) (*App, error) {
	if len(paths) > 2 {
		return nil, errors.New("mdiff accepts at most two file paths")
	}
	if err := setConfigDir(configDir); err != nil {
		return nil, err
	}
	config.Diagnostics = nil
	config.DisableBufferStateWrites = false
	config.InitRuntimeFiles(true)
	config.InitPlugins()
	if err := config.ReadSettings(); err != nil {
		return nil, err
	}
	if err := config.InitGlobalSettings(); err != nil {
		return nil, err
	}
	debounce, err := configuredDebounce(cliDebounceMS)
	if err != nil {
		return nil, err
	}
	if ulua.L != nil {
		ulua.L.Close()
	}
	ulua.L = lua.NewState()

	if s == nil {
		if err := screen.Init(); err != nil {
			ulua.L.Close()
			ulua.L = nil
			return nil, err
		}
		s = screen.Screen
	} else {
		screen.Screen = s
	}
	paintScreen := tcell.NewSimulationScreen("")
	if err := paintScreen.Init(); err != nil {
		if s != nil {
			s.Fini()
		}
		return nil, fmt.Errorf("initialize Micro render surface: %w", err)
	}
	surface := tcell.NewSimulationScreen("")
	if err := surface.Init(); err != nil {
		paintScreen.Fini()
		if s != nil {
			s.Fini()
		}
		return nil, fmt.Errorf("initialize diff composition surface: %w", err)
	}
	app := &App{
		focus:        viewport.Left,
		terminal:     screen.Screen,
		paintScreen:  paintScreen,
		surface:      surface,
		coordinator:  newDiffCoordinator(debounce, nil, nil),
		pluginTimers: make(chan func(), 128),
		pluginStop:   make(chan struct{}),
	}
	screen.Events = make(chan tcell.Event, 16)
	action.Tabs = nil
	action.InitGlobals()
	screen.SetMessageHandler(func(message string) {
		config.AddDiagnostic(message)
		if action.InfoBar != nil {
			action.InfoBar.Error(message)
		}
	})
	initMicroLua(app)
	pluginErr := config.LoadAllPlugins()
	action.InitBindings() // Patched Micro helper reads bindings without creating one.
	action.InitCommands()
	action.MdiffAction = app.routeMicroAction
	action.MdiffCommand = app.runMicroCommand
	shell.InteractiveRunner = app.runInteractiveShell
	if pluginErr != nil {
		config.AddDiagnostic(pluginErr.Error())
	}
	for _, callback := range []string{"preinit", "init", "postinit"} {
		if err := config.RunPluginFn(callback); err != nil {
			config.AddDiagnostic(err.Error())
		}
	}
	if err := config.InitColorscheme(); err != nil {
		app.Close()
		return nil, fmt.Errorf("load Micro colorscheme after plugin initialization: %w", err)
	}

	leftBuf, err := newPaneBuffer(paths, 0)
	if err != nil {
		app.Close()
		return nil, fmt.Errorf("open left pane: %w", err)
	}
	rightBuf, err := newPaneBuffer(paths, 1)
	if err != nil {
		leftBuf.Close()
		app.Close()
		return nil, fmt.Errorf("open right pane: %w", err)
	}
	for _, b := range []*buffer.Buffer{leftBuf, rightBuf} {
		b.Settings["scrollbar"] = false
		b.Settings["diffgutter"] = false
	}

	app.left.buf = leftBuf
	app.right.buf = rightBuf
	app.lineHeights = newLineHeightCache(leftBuf.LinesNum(), rightBuf.LinesNum())
	app.diagnostic = strings.Join(config.Diagnostics, "; ")
	app.versions = DiffVersions{Left: leftBuf.ContentVersion(), Right: rightBuf.ContentVersion()}
	app.displayedLeft, app.displayedRight = app.snapshots()
	app.pairs = compare.Align(app.displayedLeft, app.displayedRight)
	app.layoutVersions = app.versions
	app.coordinator.AcceptCurrent(app.versions)
	leftBuf.SetChangeHandler(func(change buffer.TextChange) { app.textChanged(viewport.Left, change) })
	rightBuf.SetChangeHandler(func(change buffer.TextChange) { app.textChanged(viewport.Right, change) })
	app.resize(s.Size())
	return app, nil
}

func newPaneBuffer(paths []string, side int) (*buffer.Buffer, error) {
	if side < len(paths) {
		return buffer.NewBufferFromFileUnshared(paths[side], buffer.BTDefault)
	}
	return buffer.NewBufferFromString("", "", buffer.BTDefault), nil
}

func configuredDebounce(cliMS *int) (time.Duration, error) {
	return resolveDebounce(config.ParsedSettings(), cliMS)
}

func resolveDebounce(settings map[string]any, cliMS *int) (time.Duration, error) {
	ms := int(defaultDiffDebounce / time.Millisecond)
	if cliMS != nil {
		if *cliMS < 40 || *cliMS > 180 {
			return 0, fmt.Errorf("--diff-debounce must be between 40 and 180 milliseconds")
		}
		return time.Duration(*cliMS) * time.Millisecond, nil
	}
	if value, ok := settings["mdiff"]; ok {
		values, ok := value.(map[string]any)
		if !ok {
			return 0, fmt.Errorf("mdiff settings must be an object")
		}
		if setting, found := values["diff-debounce-ms"]; found {
			switch value := setting.(type) {
			case float64:
				if value != float64(int(value)) {
					return 0, fmt.Errorf("mdiff.diff-debounce-ms must be an integer from 40 through 180")
				}
				ms = int(value)
			case int:
				ms = value
			default:
				return 0, fmt.Errorf("mdiff.diff-debounce-ms must be an integer from 40 through 180")
			}
		}
	}
	if ms < 40 || ms > 180 {
		return 0, fmt.Errorf("mdiff.diff-debounce-ms must be between 40 and 180 milliseconds")
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func (a *App) textChanged(side viewport.Side, change buffer.TextChange) {
	lineCount := a.left.buf.LinesNum()
	if side == viewport.Right {
		lineCount = a.right.buf.LinesNum()
	}
	a.lineHeights.change(side, change.Ranges, lineCount)
	if !a.hasPendingAnchor {
		a.pendingAnchor = a.captureAnchor()
		a.hasPendingAnchor = a.pendingAnchor.valid
	}
	if a.hasPendingAnchor && a.pendingAnchor.side == side {
		a.pendingAnchor.line = transformAnchorLine(a.pendingAnchor.line, change.Ranges)
	}
	if side == viewport.Left {
		a.versions.Left = change.Version
	} else {
		a.versions.Right = change.Version
	}
	pasteBytes, pasteNewlines := 0, 0
	if a.handlingPaste {
		pasteBytes, pasteNewlines = change.InsertedBytes, change.InsertedNewlines
	}
	a.coordinator.MarkDirty(a.versions, pasteBytes, pasteNewlines)
}

func transformAnchorLine(line int, ranges []buffer.TextChangeRange) int {
	ordered := append([]buffer.TextChangeRange(nil), ranges...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].StartLine < ordered[j].StartLine
	})
	shift := 0
	for _, change := range ordered {
		if change.EndLine < line || (change.EndLine == line && change.StartLine < line) {
			shift += change.LineDelta
			continue
		}
		if change.StartLine < line && change.EndLine >= line {
			return max(0, change.StartLine+shift)
		}
		if change.StartLine == line && change.EndLine == line && change.StartX == 0 && change.LineDelta > 0 {
			shift += change.LineDelta
		}
	}
	return max(0, line+shift)
}

func (a *App) isDiffStale() bool {
	return a.layoutVersions != a.versions || a.coordinator.Stale()
}

func (a *App) installDiff(result diffResult) {
	completed, current, restartNow := a.coordinator.Complete(result)
	if !current {
		if restartNow {
			left, right := a.snapshots()
			a.coordinator.DebounceElapsedOwned(diffSnapshot{left: left, right: right, versions: a.versions})
		}
		return
	}
	if completed.err != nil {
		a.diffErr = completed.err
		return
	}
	currentAnchor := a.captureAnchorForSnapshot(completed.snapshot.left, completed.snapshot.right)
	a.pairs = completed.pairs
	// The result snapshot is detached and immutable; transfer its slices into
	// the displayed state instead of copying every line header again.
	a.displayedLeft = completed.snapshot.left
	a.displayedRight = completed.snapshot.right
	a.expandWithAnchor(currentAnchor, completed.snapshot.left, completed.snapshot.right)
	a.diffWarning = ""
	if completed.coarse {
		a.diffWarning = "Diff budget reached; showing coarse changed region"
	}
	a.hasPendingAnchor = false
	a.pendingAnchor = anchor{}
	a.layoutVersions = completed.snapshot.versions
	a.diffErr = nil
}

func setConfigDir(flagDir string) error {
	if flagDir != "" {
		info, err := os.Stat(flagDir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("config directory %q must already exist", flagDir)
		}
		config.ConfigDir = flagDir
		return nil
	}
	if p := os.Getenv("MICRO_CONFIG_HOME"); p != "" {
		config.ConfigDir = p
	} else if p := os.Getenv("XDG_CONFIG_HOME"); p != "" {
		config.ConfigDir = filepath.Join(p, "micro")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("find home directory: %w", err)
		}
		config.ConfigDir = filepath.Join(home, ".config", "micro")
	}
	if err := os.MkdirAll(config.ConfigDir, os.ModePerm); err != nil {
		return fmt.Errorf("create Micro config directory %q: %w", config.ConfigDir, err)
	}
	return nil
}

// Run drives tcell until a quit key, interrupt, or terminal termination.
func Run(configDir string) error {
	return RunWithDebounce(configDir, nil)
}

// RunWithDebounce uses a validated CLI override when present.
func RunWithDebounce(configDir string, debounceMS *int) error {
	return RunWithFiles(configDir, debounceMS)
}

// RunWithFiles starts mdiff with zero to two initial file paths.
func RunWithFiles(configDir string, debounceMS *int, paths ...string) error {
	if len(paths) > 2 {
		return errors.New("mdiff accepts at most two file paths")
	}
	app, err := NewWithFiles(nil, configDir, debounceMS, paths...)
	if err != nil {
		return err
	}
	defer app.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	events := make(chan tcell.Event, 16)
	session := newTerminalSession(app, events)
	app.handoff = session.handoff
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case sig := <-signals:
				if sig == os.Interrupt && session.inCommand.Load() {
					continue
				}
				cancel()
				return
			}
		}
	}()

	err = RunWithSignals(ctx, app, events)
	session.stopReader()
	return err
}

// Close releases both Micro buffers and restores the terminal exactly once.
func (a *App) Close() {
	if a.closed {
		return
	}
	a.closed = true
	if ulua.L != nil {
		if err := config.RunPluginFn("deinit"); err != nil {
			config.AddDiagnostic(err.Error())
		}
	}
	screen.SetMessageHandler(nil)
	action.Tabs = nil
	action.MdiffAction = nil
	action.MdiffCommand = nil
	shell.InteractiveRunner = nil
	screen.RestartCallback = nil
	if a.pluginStop != nil {
		close(a.pluginStop)
		a.pluginStop = nil
	}
	if a.coordinator != nil {
		a.coordinator.Close()
	}
	if screen.Screen == a.terminal || screen.Screen == a.surface || screen.Screen == a.paintScreen {
		screen.Screen = nil
	}
	if a.left.buf != nil && a.right.buf != nil && a.left.buf.AbsPath != "" && a.left.buf.AbsPath == a.right.buf.AbsPath {
		// The focused pane owns the one state file for a shared pathname.
		if a.focus == viewport.Left {
			a.right.buf.DisableStateWrite = true
		} else {
			a.left.buf.DisableStateWrite = true
		}
	}
	if a.left.buf != nil {
		a.left.buf.Close()
	}
	if a.right.buf != nil {
		a.right.buf.Close()
	}
	if action.InfoBar != nil {
		action.InfoBar.Close()
		action.InfoBar = nil
	}
	if buffer.LogBuf != nil {
		buffer.LogBuf.Close()
		buffer.LogBuf = nil
	}
	if a.paintScreen != nil {
		a.paintScreen.Fini()
		a.paintScreen = nil
	}
	if a.surface != nil {
		a.surface.Fini()
		a.surface = nil
	}
	if a.terminal != nil {
		a.terminal.Fini()
	}
	if ulua.L != nil {
		ulua.L.Close()
		ulua.L = nil
	}
}

func (a *App) panes() []*pane { return []*pane{&a.left, &a.right} }

func (a *App) resize(cols, rows int) {
	if cols < 3 {
		cols = 3
	}
	if rows < 2 {
		rows = 2
	}
	anchor := a.captureAnchor()
	a.cols = cols
	// Reserve the InfoBar and only the chrome rows enabled by Micro settings.
	a.statusRow, a.menuRow1, a.menuRow2 = -1, -1, -1
	a.rows = rows
	if rows >= 3 || action.InfoBar != nil && action.InfoBar.HasPrompt {
		a.rows--
		a.statusRow = rows - 1
		if enabled, _ := config.GetGlobalOption("statusline").(bool); enabled && a.rows > 1 {
			a.rows--
		}
		if enabled, _ := config.GetGlobalOption("keymenu").(bool); enabled && a.rows > 2 {
			a.rows -= 2
			a.menuRow1 = a.statusRow - 2
			a.menuRow2 = a.statusRow - 1
		}
	}
	leftWidth := cols / 2
	rightWidth := cols - leftWidth - 1
	if rightWidth < 1 {
		rightWidth = 1
	}
	startCols := [2]int{}
	if a.left.view != nil {
		startCols[0] = a.left.view.StartCol
	}
	if a.right.view != nil {
		startCols[1] = a.right.view.StartCol
	}
	for i, p := range a.panes() {
		p.x = 0
		p.width = leftWidth
		if i == 1 {
			p.x = leftWidth + 1
			p.width = rightWidth
		}
		if p.h == nil {
			p.h = action.NewBufPaneFromBuf(p.buf, nil)
			p.h.MouseLoc = func(e *tcell.EventMouse) buffer.Loc { return a.mouseLocation(p, e) }
			p.h.PromptBufferName = func() string { return a.panePromptName(p) }
			p.view = p.h.BWindow.(*display.BufWindow)
		}
		p.view.X, p.view.Y = p.x, 0
		if !p.buf.Settings["softwrap"].(bool) {
			p.view.StartCol = startCols[i]
		}
		p.view.SetActive((i == 0 && a.focus == viewport.Left) || (i == 1 && a.focus == viewport.Right))
		p.statusView = &display.BufWindow{
			View: &display.View{X: p.x, Y: a.rows, Width: p.width, Height: 1},
			Buf:  p.buf,
		}
		p.statusView.SetActive((i == 0 && a.focus == viewport.Left) || (i == 1 && a.focus == viewport.Right))
	}
	a.microTab = &action.Tab{}
	a.microTab.Panes = []action.Pane{a.left.h, a.right.h}
	a.left.h.SetTab(a.microTab)
	a.right.h.SetTab(a.microTab)
	a.microTab.SetActive(int(a.focus))
	action.Tabs = &action.TabList{
		TabWindow: display.NewTabWindow(cols, 0),
		List:      []*action.Tab{a.microTab},
	}
	screen.RestartCallback = action.Tabs.ResetMouse
	a.left.h.Resize(a.left.width, a.rows)
	a.right.h.Resize(a.right.width, a.rows)
	a.expandWithAnchor(anchor, a.displayedLeft, a.displayedRight)
}

func (a *App) snapshots() ([]string, []string) {
	left := make([]string, a.left.buf.LinesNum())
	right := make([]string, a.right.buf.LinesNum())
	for i := range left {
		left[i] = a.left.buf.Line(i)
	}
	for i := range right {
		right[i] = a.right.buf.Line(i)
	}
	return left, right
}

func (a *App) wrapHeight(p *pane, line int) int {
	if line < 0 || line >= p.buf.LinesNum() || p.view == nil {
		return 1
	}
	tabSize, _ := p.buf.Settings["tabsize"].(float64)
	softwrap, _ := p.buf.Settings["softwrap"].(bool)
	wordwrap, _ := p.buf.Settings["wordwrap"].(bool)
	side := viewport.Left
	if p.x != 0 {
		side = viewport.Right
	}
	return a.lineHeights.height(side, line, p.view.Width, int(tabSize), softwrap, wordwrap, func() int {
		end := len([]rune(p.buf.Line(line)))
		return p.view.VLocFromLoc(buffer.Loc{X: end, Y: line}).Row + 1
	})
}

type anchor struct {
	side       viewport.Side
	text       string
	occurrence int
	line       int
	before     []string
	after      []string
	wrapRow    int
	fallback   int
	valid      bool
}

func (a *App) captureAnchor() anchor {
	if len(a.layout.Rows) == 0 {
		return anchor{}
	}
	rowIndex := min(max(a.top, 0), len(a.layout.Rows)-1)
	row := a.layout.Rows[rowIndex]
	side, cell := viewport.Left, row.Left
	if cell.Spacer {
		side, cell = viewport.Right, row.Right
	}
	if cell.Spacer || cell.Line < 0 {
		return anchor{fallback: rowIndex}
	}
	lines := a.displayedLeft
	if side == viewport.Right {
		lines = a.displayedRight
	}
	if cell.Line < 0 || cell.Line >= len(lines) {
		return anchor{fallback: rowIndex}
	}
	text := lines[cell.Line]
	occurrence := 0
	for i := 0; i < cell.Line; i++ {
		if lines[i] == text {
			occurrence++
		}
	}
	return anchor{
		side: side, text: text, occurrence: occurrence, line: cell.Line,
		before: surroundingLines(lines, cell.Line, -1), after: surroundingLines(lines, cell.Line, 1),
		wrapRow: cell.WrapRow, fallback: rowIndex, valid: true,
	}
}

func (a *App) captureAnchorForSnapshot(left, right []string) anchor {
	if !a.hasPendingAnchor {
		return a.captureAnchor()
	}
	previous := a.pendingAnchor
	lines := left
	if previous.side == viewport.Right {
		lines = right
	}
	if previous.line >= 0 && previous.line < len(lines) && lines[previous.line] == previous.text {
		previous.occurrence = 0
		for line := 0; line < previous.line; line++ {
			if lines[line] == previous.text {
				previous.occurrence++
			}
		}
	}
	return previous
}

func anchorMatchesSnapshot(previous anchor, left, right []string) bool {
	if !previous.valid {
		return false
	}
	lines := left
	if previous.side == viewport.Right {
		lines = right
	}
	return previous.line >= 0 && previous.line < len(lines) && lines[previous.line] == previous.text
}

func surroundingLines(lines []string, line, direction int) []string {
	context := make([]string, 0, 3)
	for distance := 1; distance <= 3; distance++ {
		index := line + direction*distance
		if index < 0 || index >= len(lines) {
			break
		}
		context = append(context, lines[index])
	}
	return context
}

func (a *App) ensureCursorVisible() {
	a.refreshLayoutIfNeeded()
	if len(a.layout.Rows) == 0 || a.rows <= 0 {
		a.top = 0
		return
	}
	p, side := &a.left, viewport.Left
	if a.focus == viewport.Right {
		p, side = &a.right, viewport.Right
	}
	loc := p.h.Cursor.Loc
	p.view.Relocate()
	wrapRow := p.view.VLocFromLoc(loc).Row
	row := a.layout.CursorRow(side, loc.Y, wrapRow)
	if row < 0 {
		return
	}
	if row < a.top {
		a.top = row
	} else if row >= a.top+a.rows {
		a.top = row - a.rows + 1
	}
	a.clampTop()
}

func (a *App) clampTop() {
	a.top = min(max(a.top, 0), max(0, len(a.layout.Rows)-a.rows))
}

func (a *App) handle(event tcell.Event) bool {
	switch e := event.(type) {
	case *tcell.EventResize:
		a.resize(e.Size())
		return false
	case *tcell.EventMouse:
		if a.helpVisible {
			return a.handleHelpMouse(e)
		}
		a.blockedSequence = ""
		a.handleMouse(e)
		return a.quitComplete
	case *tcell.EventKey:
		if a.helpVisible {
			if action.InfoBar != nil && action.InfoBar.HasPrompt {
				a.handlePromptEvent(e)
				return a.quitComplete
			}
			return a.handleHelpKey(e)
		}
		if e.Key() == tcell.KeyCtrlQ && action.InfoBar != nil && action.InfoBar.HasPrompt {
			a.blockedSequence = ""
			a.handlePromptEvent(e)
			return a.quitComplete
		}
		if a.rejectConflictingReservedBinding(e) {
			return false
		}
		if action.InfoBar != nil && action.InfoBar.HasPrompt {
			a.blockedSequence = ""
			a.handlePromptEvent(e)
			return a.quitComplete
		}
		if a.blockedSequence != "" && a.handleRejectedSequence(e) {
			return false
		}
		if a.handleRejectedSequence(e) {
			return false
		}
		p := a.focusedPane()
		p.h.HandleEvent(e)
		if a.preserveViewport {
			a.preserveViewport = false
		} else {
			a.ensureCursorVisible()
		}
		return a.quitComplete
	case *tcell.EventRaw:
		if a.helpVisible {
			a.helpVisible = false
			return false
		}
		a.blockedSequence = ""
		if rejected := action.RejectedBufferBindingForRaw(e); rejected != "" {
			a.reportRejectedAction(rejected)
			return false
		}
		a.focusedPane().h.HandleEvent(e)
		a.ensureCursorVisible()
		return a.quitComplete
	case *tcell.EventPaste:
		a.blockedSequence = ""
		if action.InfoBar != nil && action.InfoBar.HasPrompt {
			a.handlePromptEvent(e)
			return a.quitComplete
		}
		p := a.focusedPane()
		a.handlingPaste = true
		p.h.HandleEvent(e)
		a.handlingPaste = false
		a.ensureCursorVisible()
		return a.quitComplete
	}
	return false
}

func (a *App) showHelp() {
	a.openHelpTopic("help")
}

// rejectConflictingReservedBinding consumes unsafe sequence prefixes before
// Micro can execute the default single-key action on the same prefix.
func (a *App) rejectConflictingReservedBinding(event *tcell.EventKey) bool {
	keyName := action.BufferKeyName(event)
	if rejected, _, prefix := action.RejectedBufferSequenceState("<" + keyName + ">"); prefix {
		a.blockedSequence = "<" + keyName + ">"
		if rejected == "" {
			rejected = "configured key sequence"
		}
		a.reportRejectedAction(rejected)
		return true
	}
	return false
}

func (a *App) openCommandBar() {
	if action.InfoBar == nil {
		return
	}
	p := a.focusedPane()
	action.InfoBar.Prompt("> ", "", "Command", nil, func(input string, canceled bool) {
		if canceled || strings.TrimSpace(input) == "" {
			return
		}
		a.runMicroCommand(p.h, input)
	})
}

func safeCommand(command string) bool {
	command = strings.ToLower(command)
	if !action.IsRegisteredCommand(command) {
		return false
	}
	switch command {
	case "vsplit", "hsplit", "tab", "tabmove", "tabswitch", "term", "eval", "log", "plugin", "reload", "raw":
		return false
	}
	return true
}

func (a *App) reportRejectedAction(binding string) {
	if action.InfoBar == nil {
		return
	}
	name := strings.TrimSpace(strings.NewReplacer("|", ", ", "&", ", ", ",", ", ").Replace(binding))
	action.InfoBar.Error(name + " is unavailable in mdiff")
}

func (a *App) handleRejectedSequence(event *tcell.EventKey) bool {
	keyName := action.BufferKeyName(event)
	if a.blockedSequence == "" {
		if rejected := action.RejectedBufferBindingForKey(event); rejected != "" {
			a.reportRejectedAction(rejected)
			return true
		}
	}
	candidate := a.blockedSequence + "<" + keyName + ">"
	rejected, exact, prefix := action.RejectedBufferSequenceState(candidate)
	if exact {
		a.blockedSequence = ""
		a.reportRejectedAction(rejected)
		return true
	}
	if prefix {
		a.blockedSequence = candidate
		return true
	}
	a.blockedSequence = ""
	if rejected := action.RejectedBufferBindingForKey(event); rejected != "" {
		a.reportRejectedAction(rejected)
		return true
	}
	return false
}

func (a *App) rebuildWithAnchor(previous anchor) {
	left, right := a.snapshots()
	if a.hasPendingAnchor {
		pending := a.captureAnchorForSnapshot(left, right)
		if anchorMatchesSnapshot(pending, left, right) {
			previous = pending
		}
	}
	a.pairs = compare.Align(left, right)
	a.displayedLeft = left
	a.displayedRight = right
	a.expandWithAnchor(previous, left, right)
	a.hasPendingAnchor = false
	a.pendingAnchor = anchor{}
	if a.coordinator != nil {
		a.versions = DiffVersions{Left: a.left.buf.ContentVersion(), Right: a.right.buf.ContentVersion()}
	}
	a.diffErr = nil
	if a.coordinator != nil {
		a.coordinator.AcceptCurrent(a.versions)
	}
	a.layoutVersions = a.versions
}

func (a *App) expandWithAnchor(previous anchor, snapshots ...[]string) {
	left, right := a.snapshots()
	if len(snapshots) == 2 {
		left, right = snapshots[0], snapshots[1]
	}
	a.layout = layoutFor(a.pairs, func(line int) int { return a.wrapHeight(&a.left, line) }, func(line int) int { return a.wrapHeight(&a.right, line) })
	a.layoutKey = a.currentLayoutKey()
	a.restoreAnchor(previous, left, right)
	a.ensureCursorVisible()
}

func (a *App) currentLayoutKey() appLayoutKey {
	return appLayoutKey{left: paneKey(&a.left), right: paneKey(&a.right)}
}

func paneKey(p *pane) paneLayoutKey {
	if p == nil || p.buf == nil || p.view == nil {
		return paneLayoutKey{}
	}
	tabsize, _ := p.buf.Settings["tabsize"].(float64)
	return paneLayoutKey{
		width: p.width, buffer: p.view.BufView().Width,
		softwrap: p.buf.Settings["softwrap"].(bool), wordwrap: p.buf.Settings["wordwrap"].(bool),
		tabsize: int(tabsize),
	}
}

func (a *App) refreshLayoutIfNeeded() {
	key := a.currentLayoutKey()
	if key == a.layoutKey || a.left.view == nil || a.right.view == nil {
		return
	}
	anchor := a.captureAnchor()
	a.layout = layoutFor(a.pairs, func(line int) int { return a.wrapHeight(&a.left, line) }, func(line int) int { return a.wrapHeight(&a.right, line) })
	a.layoutKey = key
	a.restoreAnchor(anchor, a.displayedLeft, a.displayedRight)
}

func (a *App) restoreAnchor(previous anchor, left, right []string) {
	if !previous.valid {
		a.top = previous.fallback
		a.clampTop()
		return
	}
	lines, side := left, viewport.Left
	if previous.side == viewport.Right {
		lines, side = right, viewport.Right
	}
	bestLine, bestScore := -1, -1
	bestOccurrenceDistance, bestDistance := int(^uint(0)>>1), int(^uint(0)>>1)
	occurrence := 0
	for line := range lines {
		candidateOccurrence := -1
		if lines[line] == previous.text {
			candidateOccurrence = occurrence
			occurrence++
		}
		score := 0
		if lines[line] == previous.text {
			score += 32
		}
		score += matchingContext(lines, line, previous.before, -1)
		score += matchingContext(lines, line, previous.after, 1)
		occurrenceDistance := int(^uint(0) >> 1)
		if candidateOccurrence >= 0 {
			occurrenceDistance = abs(candidateOccurrence - previous.occurrence)
		}
		distance := abs(line - previous.line)
		if score > bestScore || (score == bestScore && occurrenceDistance < bestOccurrenceDistance) ||
			(score == bestScore && occurrenceDistance == bestOccurrenceDistance && distance < bestDistance) {
			bestLine, bestScore = line, score
			bestOccurrenceDistance, bestDistance = occurrenceDistance, distance
		}
	}
	if bestLine >= 0 && bestScore > 0 {
		wrapRow := previous.wrapRow
		if wrapRow >= a.layout.WrapHeight(side, bestLine) {
			wrapRow = max(0, a.layout.WrapHeight(side, bestLine)-1)
		}
		row := a.layout.CursorRow(side, bestLine, wrapRow)
		if row >= 0 {
			a.top = row
			a.clampTop()
			return
		}
	}
	a.top = previous.fallback
	a.clampTop()
}

func matchingContext(lines []string, line int, context []string, direction int) int {
	matches := 0
	for distance, expected := range context {
		index := line + direction*(distance+1)
		if index < 0 || index >= len(lines) {
			break
		}
		if lines[index] != expected {
			break
		}
		matches += 1 << max(0, 2-distance)
	}
	return matches
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (a *App) focusedPane() *pane {
	if a.focus == viewport.Right {
		return &a.right
	}
	return &a.left
}

func (a *App) setFocus() {
	if a.microTab != nil {
		a.microTab.SetActive(int(a.focus))
	}
	a.left.view.SetActive(a.focus == viewport.Left)
	a.right.view.SetActive(a.focus == viewport.Right)
	a.left.statusView.SetActive(a.focus == viewport.Left)
	a.right.statusView.SetActive(a.focus == viewport.Right)
}

func (a *App) handleMouse(e *tcell.EventMouse) {
	x, y := e.Position()
	if e.Buttons()&(tcell.WheelUp|tcell.WheelDown|tcell.WheelLeft|tcell.WheelRight) != 0 {
		a.focusedPane().h.HandleEvent(e)
		a.preserveViewport = false
		return
	}
	if y >= a.rows && e.Buttons() != tcell.ButtonNone {
		return
	}
	if e.Buttons() != tcell.ButtonNone && a.mousePane == nil {
		a.mousePane = &a.left
		if x > a.left.x+a.left.width {
			a.mousePane = &a.right
		}
		a.focus = viewport.Left
		if a.mousePane == &a.right {
			a.focus = viewport.Right
		}
		a.setFocus()
	}
	p := a.mousePane
	if p == nil {
		return
	}
	p.h.HandleEvent(e)
	if e.Buttons() == tcell.ButtonNone {
		a.mousePane = nil
	}
	a.ensureCursorVisible()
}

func (a *App) mouseLocation(p *pane, e *tcell.EventMouse) buffer.Loc {
	x, y := e.Position()
	rowIndex := a.top + y
	if rowIndex < 0 || rowIndex >= len(a.layout.Rows) {
		return p.h.Cursor.Loc
	}
	row := a.layout.Rows[rowIndex]
	cell := row.Left
	side := viewport.Left
	if p == &a.right {
		cell = row.Right
		side = viewport.Right
	}
	if cell.Line < 0 {
		if !cell.Spacer {
			return p.h.Cursor.Loc
		}
		cell, _ = a.nearestCell(side, rowIndex)
		if cell.Line < 0 {
			return p.h.Cursor.Loc
		}
	}
	if cell.Continuation {
		cell.WrapRow = max(0, a.layout.WrapHeight(side, cell.Line)-1)
	}
	visualX := x - p.view.BufView().X
	visualX = max(0, visualX)
	if !p.buf.Settings["softwrap"].(bool) {
		visualX += p.view.StartCol
	}
	return p.view.LocFromVLoc(display.VLoc{
		SLoc: display.SLoc{Line: cell.Line, Row: cell.WrapRow}, VisualX: visualX,
	})
}

// nearestCell resolves a clicked virtual spacer to the closest real line in
// that pane. Ties prefer the preceding line; at the start of a pane, the next
// line is used.
func (a *App) nearestCell(side viewport.Side, row int) (viewport.Cell, int) {
	for distance := 1; distance < len(a.layout.Rows); distance++ {
		for _, candidate := range []int{row - distance, row + distance} {
			cell := a.layout.CellAt(side, candidate)
			if cell.Line >= 0 && !cell.Spacer && !cell.Continuation {
				return cell, candidate
			}
		}
	}
	return viewport.Cell{Line: -1, Spacer: true}, row
}

func (a *App) jumpDiff(direction int) {
	if len(a.layout.Rows) == 0 {
		return
	}
	type hunk struct{ start, end int }
	hunks := make([]hunk, 0)
	for row := 0; row < len(a.layout.Rows); {
		if a.layout.Rows[row].Kind == compare.Equal {
			row++
			continue
		}
		start := row
		for row < len(a.layout.Rows) && a.layout.Rows[row].Kind != compare.Equal {
			row++
		}
		hunks = append(hunks, hunk{start: start, end: row})
	}
	selected := -1
	if direction > 0 {
		for i, current := range hunks {
			if current.start > a.top {
				selected = i
				break
			}
		}
	} else {
		for i := len(hunks) - 1; i >= 0; i-- {
			if hunks[i].end <= a.top {
				selected = i
				break
			}
		}
	}
	if selected < 0 {
		return
	}
	target := hunks[selected]
	pane, side := &a.left, viewport.Left
	if a.focus == viewport.Right {
		pane, side = &a.right, viewport.Right
	}
	line := -1
	for row := target.start; row < target.end; row++ {
		cell := a.layout.CellAt(side, row)
		if cell.Line >= 0 && !cell.Continuation {
			line = cell.Line
			break
		}
	}
	if line < 0 {
		neighbor := target.end
		if direction < 0 {
			neighbor = target.start - 1
		}
		for distance := 0; line < 0 && distance < len(a.layout.Rows); distance++ {
			for _, row := range []int{neighbor + direction*distance, neighbor - direction*distance} {
				cell := a.layout.CellAt(side, row)
				if cell.Line >= 0 && !cell.Continuation {
					line = cell.Line
					break
				}
			}
		}
	}
	a.top = target.start
	if line >= 0 {
		pane.h.GotoLoc(buffer.Loc{X: 0, Y: line})
	}
	a.ensureCursorVisible()
}

// RunWithSignals is useful to tests that need to exercise interruption without
// starting a real terminal screen.
func RunWithSignals(ctx context.Context, a *App, events <-chan tcell.Event) error {
	a.draw()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-a.coordinator.TimerC():
			// Capture snapshots on the application goroutine. Workers receive only
			// detached strings and version numbers.
			left, right := a.snapshots()
			a.coordinator.DebounceElapsedOwned(diffSnapshot{left: left, right: right, versions: a.versions})
			a.draw()
		case result := <-a.coordinator.Results():
			a.installDiff(result)
			a.draw()
		case callback := <-a.pluginTimers:
			if callback != nil {
				callback()
				if a.terminalErr != nil {
					return a.terminalErr
				}
				a.syncChangedBuffers()
				a.draw()
			}
		case event, ok := <-events:
			if !ok {
				return errors.New("event stream closed")
			}
			done := a.handle(event)
			if a.terminalErr != nil {
				return a.terminalErr
			}
			if done {
				return nil
			}
			a.draw()
		}
	}
}
