// Package prototype contains the disposable TASK-002 Micro integration probe.
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
	"github.com/nevdelap/mdiff/internal/version"
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
	config.DisableBufferStateWrites = true
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
	// Command history belongs to Micro's command bar, which mdiff does not
	// expose, so do not persist that unrelated history file.
	config.GlobalSettings["savehistory"] = false
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
	app.pairs = compare.Align([]string{""}, []string{""})
	app.diagnostic = strings.Join(config.Diagnostics, "; ")
	app.versions = DiffVersions{Left: leftBuf.ContentVersion(), Right: rightBuf.ContentVersion()}
	app.displayedLeft, app.displayedRight = app.snapshots()
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
		return nil
	}
	if p := os.Getenv("XDG_CONFIG_HOME"); p != "" {
		config.ConfigDir = filepath.Join(p, "micro")
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}
	config.ConfigDir = filepath.Join(home, ".config", "micro")
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	events := make(chan tcell.Event, 16)
	terminal := app.terminal
	go func() {
		for {
			e := terminal.PollEvent()
			select {
			case events <- e:
			case <-ctx.Done():
				return
			}
			if e == nil {
				return
			}
		}
	}()

	return RunWithSignals(ctx, app, events)
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
	// Keep four dedicated bottom rows when the terminal can display them:
	// one Micro status line per pane, the two-row key menu, and the bottom
	// InfoBar. Compact terminals retain an editor row and hide these chrome rows
	// rather than overlapping them.
	a.statusRow, a.menuRow1, a.menuRow2 = -1, -1, -1
	if rows >= 5 {
		a.rows = rows - 4
		a.menuRow1 = a.rows + 1
		a.menuRow2 = a.rows + 2
		a.statusRow = a.rows + 3
	} else {
		a.rows = rows
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
		p.h = action.NewBufPaneFromBuf(p.buf, nil)
		p.h.PromptBufferName = func() string { return a.panePromptName(p) }
		p.view = p.h.BWindow.(*display.BufWindow)
		p.view.X, p.view.Y = p.x, 0
		if !p.buf.Settings["softwrap"].(bool) {
			p.view.StartCol = startCols[i]
		}
		p.view.Resize(p.width, a.rows)
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
		a.blockedSequence = ""
		a.handleMouse(e)
		return false
	case *tcell.EventKey:
		if e.Key() == tcell.KeyCtrlQ && action.InfoBar != nil && action.InfoBar.HasPrompt {
			a.blockedSequence = ""
			a.handlePromptEvent(e)
			return a.quitComplete
		}
		if a.rejectConflictingReservedBinding(e) {
			return false
		}
		if e.Key() == tcell.KeyCtrlQ {
			a.blockedSequence = ""
			return a.requestQuit()
		}
		if e.Key() == tcell.KeyCtrlG {
			a.blockedSequence = ""
			a.showHelp()
			return false
		}
		if e.Key() == tcell.KeyCtrlE {
			a.blockedSequence = ""
			a.openCommandBar()
			return false
		}
		if action.InfoBar != nil && action.InfoBar.HasPrompt {
			a.blockedSequence = ""
			a.handlePromptEvent(e)
			return a.quitComplete
		}
		if e.Key() == tcell.KeyCtrlW {
			a.focus = 1 - a.focus
			a.setFocus()
			a.ensureCursorVisible()
			return false
		}
		if a.blockedSequence != "" && a.handleRejectedSequence(e) {
			return false
		}
		if e.Key() == tcell.KeyCtrlO {
			a.blockedSequence = ""
			a.focusedPane().h.OpenFile()
			return false
		}
		if e.Key() == tcell.KeyCtrlS {
			a.blockedSequence = ""
			p := a.focusedPane()
			p.h.SaveWithExternalChangeCheck("Save", nil)
			return false
		}
		if e.Key() == tcell.KeyCtrlT {
			a.blockedSequence = ""
			a.newFocusedDocument()
			return false
		}
		if a.handleRejectedSequence(e) {
			return false
		}
		if e.Modifiers()&tcell.ModCtrl != 0 && (e.Key() == tcell.KeyUp || e.Key() == tcell.KeyDown) {
			delta := 1
			if e.Key() == tcell.KeyUp {
				delta = -1
			}
			a.top += delta
			a.clampTop()
			return false
		}
		if e.Modifiers()&tcell.ModAlt != 0 && (e.Key() == tcell.KeyUp || e.Key() == tcell.KeyDown) {
			direction := 1
			if e.Key() == tcell.KeyUp {
				direction = -1
			}
			a.jumpDiff(direction)
			return false
		}
		p := a.focusedPane()
		p.h.HandleEvent(e)
		a.ensureCursorVisible()
	case *tcell.EventPaste:
		a.blockedSequence = ""
		if action.InfoBar != nil && action.InfoBar.HasPrompt {
			a.handlePromptEvent(e)
			return false
		}
		p := a.focusedPane()
		a.handlingPaste = true
		p.h.HandleEvent(e)
		a.handlingPaste = false
		a.ensureCursorVisible()
	}
	return false
}

func (a *App) handlePromptEvent(event tcell.Event) {
	if action.InfoBar == nil || !action.InfoBar.HasPrompt {
		return
	}
	action.InfoBar.HandleEvent(event)
	a.syncFocusedBuffer()
	a.ensureCursorVisible()
	if a.quitFlow != nil && a.quitFlow.awaitingSave && (action.InfoBar == nil || !action.InfoBar.HasPrompt) {
		a.quitFlow = nil
	}
}

func (a *App) requestQuit() bool {
	if a.quitFlow != nil {
		return false
	}
	a.quitComplete = false
	flow := &quitFlow{panes: a.panes()}
	a.quitFlow = flow
	a.advanceQuitDecisions(flow)
	return a.quitComplete
}

func (a *App) panePromptName(p *pane) string {
	return fmt.Sprintf("%s (%s)", filepath.Base(p.buf.GetName()), a.paneSideName(p))
}

func (a *App) paneSideName(p *pane) string {
	side := "left pane"
	if p == &a.right {
		side = "right pane"
	}
	return side
}

func (a *App) advanceQuitDecisions(flow *quitFlow) {
	if a.quitFlow != flow {
		return
	}
	for flow.paneIndex < len(flow.panes) {
		p := flow.panes[flow.paneIndex]
		flow.paneIndex++
		if !p.buf.Modified() || p.buf.Shared() {
			continue
		}
		if autosave, _ := config.GlobalSettings["autosave"].(float64); autosave > 0 && p.buf.Path != "" {
			flow.saves = append(flow.saves, p)
			continue
		}
		action.InfoBar.YNPrompt("Save changes to "+a.panePromptName(p)+" before closing? (y,n,esc)", func(yes, canceled bool) {
			if a.quitFlow != flow {
				return
			}
			if canceled {
				a.quitFlow = nil
				return
			}
			if yes {
				flow.saves = append(flow.saves, p)
			}
			a.advanceQuitDecisions(flow)
		})
		return
	}
	a.saveForQuit(flow)
}

func (a *App) saveForQuit(flow *quitFlow) {
	if a.quitFlow != flow {
		return
	}
	if flow.saveIndex >= len(flow.saves) {
		a.quitFlow = nil
		a.quitComplete = true
		return
	}
	p := flow.saves[flow.saveIndex]
	flow.saveIndex++
	flow.awaitingSave = true
	p.h.SaveWithExternalChangeCheck("Quit", func() {
		if a.quitFlow != flow {
			return
		}
		flow.awaitingSave = false
		a.saveForQuit(flow)
	})
	if a.quitFlow == flow && flow.awaitingSave && (action.InfoBar == nil || !action.InfoBar.HasPrompt) {
		// Micro's save callback runs only after a successful write. A synchronous
		// failure leaves the buffer open and reports through InfoBar.
		a.quitFlow = nil
	}
}

func (a *App) newFocusedDocument() {
	p := a.focusedPane()
	reset := func() {
		b := buffer.NewBufferFromString("", "", buffer.BTDefault)
		anchor := a.captureAnchor()
		p.h.OpenBuffer(b)
		a.syncPaneBuffer(p)
		a.rebuildWithAnchor(anchor)
	}
	if p.buf.Modified() && !p.buf.Shared() {
		action.InfoBar.YNPrompt("Save changes to "+a.panePromptName(p)+" before closing? (y,n,esc)", func(yes, canceled bool) {
			if canceled {
				return
			}
			if yes {
				p.h.SaveWithExternalChangeCheck("Open", reset)
				return
			}
			reset()
		})
		return
	}
	reset()
}

func (a *App) syncPaneBuffer(p *pane) {
	if p == nil || p.h == nil || p.h.Buf == nil || p.buf == p.h.Buf {
		return
	}
	p.buf = p.h.Buf
	p.statusView.Buf = p.buf
	p.buf.Settings["scrollbar"] = false
	p.buf.Settings["diffgutter"] = false
	side := viewport.Left
	if p == &a.right {
		side = viewport.Right
	}
	a.lineHeights.reset(side, p.buf.LinesNum())
	p.buf.SetChangeHandler(func(change buffer.TextChange) { a.textChanged(side, change) })
	p.buf.UpdateRules()
}

func (a *App) syncFocusedBuffer() {
	p := a.focusedPane()
	if p != nil && p.h != nil && p.buf != p.h.Buf {
		anchor := a.captureAnchor()
		a.syncPaneBuffer(p)
		a.rebuildWithAnchor(anchor)
	}
}

func (a *App) showHelp() {
	if action.InfoBar != nil {
		action.InfoBar.Message("mdiff: Ctrl+W switches pane; Ctrl+Up/Down scroll; Alt+Up/Down jumps differences; Ctrl+Q quits")
	}
}

// rejectConflictingReservedBinding prevents a configured Micro action from
// being silently shadowed by mdiff's lifecycle, help, command, or pane-switch
// keys. The matching built-in Micro actions remain compatible with mdiff's
// equivalent behavior.
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
	var allowed []string
	switch event.Key() {
	case tcell.KeyCtrlQ:
		allowed = []string{"Quit"}
	case tcell.KeyCtrlG:
		allowed = []string{"ToggleHelp"}
	case tcell.KeyCtrlE:
		allowed = []string{"CommandMode"}
	case tcell.KeyCtrlW:
		allowed = []string{"NextSplit", "FirstSplit"}
	default:
		return false
	}

	rejected := action.RejectedBufferBindingForKey(event)
	if rejected == "" {
		return false
	}
	allowedActions := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowedActions[name] = true
	}
	for _, name := range strings.FieldsFunc(rejected, func(r rune) bool {
		return r == '|' || r == '&' || r == ','
	}) {
		if !allowedActions[strings.TrimSpace(name)] {
			a.blockedSequence = ""
			a.reportRejectedAction(rejected)
			return true
		}
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
		command := strings.Fields(input)[0]
		if !safeCommand(command) {
			action.InfoBar.Error("Command unavailable in mdiff: " + command)
			return
		}
		p.h.HandleCommand(input)
		a.ensureCursorVisible()
	})
}

func safeCommand(command string) bool {
	switch strings.ToLower(command) {
	case "goto", "jump", "replace", "replaceall", "retab", "show", "showkey", "memusage":
		return true
	default:
		return false
	}
}

func (a *App) reportRejectedAction(binding string) {
	if action.InfoBar == nil {
		return
	}
	name := strings.TrimSpace(strings.NewReplacer("|", ", ", "&", ", ", ",", ", ").Replace(binding))
	if strings.Contains(name, "SaveAll") {
		action.InfoBar.Error("Save All is unavailable: mdiff has exactly two independently focused panes")
		return
	}
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
	if e.Buttons()&tcell.WheelUp != 0 {
		a.top -= 3
		a.clampTop()
		return
	}
	if e.Buttons()&tcell.WheelDown != 0 {
		a.top += 3
		a.clampTop()
		return
	}
	if y >= a.rows || (e.Buttons()&tcell.Button1) == 0 {
		return
	}
	side := viewport.Left
	if x > a.left.x+a.left.width {
		side = viewport.Right
	}
	a.focus = side
	a.setFocus()
	rowIndex := a.top + y
	if rowIndex >= len(a.layout.Rows) {
		return
	}
	row := a.layout.Rows[rowIndex]
	cell, p := row.Left, &a.left
	if side == viewport.Right {
		cell, p = row.Right, &a.right
	}
	if cell.Line < 0 {
		if !cell.Spacer {
			return
		}
		cell, _ = a.nearestCell(side, rowIndex)
		if cell.Line < 0 {
			return
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
	p.h.GotoLoc(p.view.LocFromVLoc(display.VLoc{
		SLoc: display.SLoc{Line: cell.Line, Row: cell.WrapRow}, VisualX: visualX,
	}))
	a.ensureCursorVisible()
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

func (a *App) draw() {
	if a.terminal == nil || a.paintScreen == nil {
		return
	}
	a.refreshLayoutIfNeeded()
	width, height := a.terminal.Size()
	a.surface.SetSize(width, height)
	a.paintScreen.SetSize(width, height)
	a.surface.Clear()
	terminal := screen.Screen
	defer func() { screen.Screen = terminal }()
	screen.Screen = a.surface
	for _, p := range a.panes() {
		a.renderPane(p, a.layout, a.top, a.rows)
	}
	for y := 0; y < a.rows; y++ {
		screen.SetContent(a.left.width, y, '│', nil, config.DefStyle)
	}
	status := ""
	if a.diffErr != nil {
		status = "Diff error: " + a.diffErr.Error() + " | " + status
	}
	if a.diffWarning != "" {
		status = a.diffWarning + " | " + status
	}
	if action.InfoBar != nil && (action.InfoBar.HasMessage || action.InfoBar.HasError) {
		status = action.InfoBar.Msg + " | " + status
	}
	if a.diagnostic != "" && (action.InfoBar == nil || (!action.InfoBar.HasMessage && !action.InfoBar.HasError)) {
		status = a.diagnostic + " | " + status
	}
	if action.InfoBar != nil && action.InfoBar.HasPrompt {
		status, _ = a.promptDisplay()
	}
	if a.statusRow >= 0 {
		for _, p := range a.panes() {
			display.NewStatusLine(p.statusView).Display()
		}
		screen.SetContent(a.left.width, a.rows, '│', nil, config.DefStyle)
		menuStyle := microMenuStyle()
		a.renderMenuLine(a.menuRow1, a.shortcutBar(0), a.focusNameIndicator(), menuStyle)
		a.renderMenuLine(a.menuRow2, a.shortcutBar(1), "mdiff (wrapping micro) "+version.Version, menuStyle)
		a.renderLine(a.statusRow, status, config.DefStyle)
	}
	cursorX, cursorY, cursorVisible := a.cursorPosition()
	screen.Screen = terminal
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, combc, style, cellWidth := a.surface.GetContent(x, y)
			if cellWidth == 0 {
				continue
			}
			terminal.SetContent(x, y, r, combc, style)
		}
	}
	if cursorVisible {
		terminal.ShowCursor(cursorX, cursorY)
	} else {
		terminal.HideCursor()
	}
	a.terminal.Show()
}

func (a *App) renderLine(y int, text string, style tcell.Style) {
	_, height := a.surface.Size()
	if y < 0 || y >= height {
		return
	}
	for x := 0; x < a.cols; x++ {
		screen.SetContent(x, y, ' ', nil, style)
	}
	for x, r := range []rune(text) {
		if x >= a.cols {
			break
		}
		screen.SetContent(x, y, r, nil, style)
	}
}

func (a *App) shortcutBar(row int) string {
	items := []struct{ label, action, fallback string }{
		{"^W Switch Panes", "", ""},
		{"^Q Quit", "Quit", "^Q"},
		{"^S Save", "Save", "^S"},
		{"^O Open", "OpenFile", "^O"},
		{"^G Help", "ToggleHelp", "^G"},
		{"^E Command Bar", "CommandMode", "^E"},
		{"^K Cut Line", "CutLine", "^K"},
		{"^F Find", "Find", "^F"},
		{"^Z Undo", "Undo", "^Z"},
		{"^Y Redo", "Redo", "^Y"},
		{"^A Select All", "SelectAll", "^A"},
		{"^D Duplicate Line", "DuplicateLine", "^D"},
		{"^T New Tab", "AddTab", "^T"},
	}
	start, end := 1, len(items)
	if row == 0 {
		end = 7
	} else {
		start = 7
	}
	labels := make([]string, 0, end-start)
	for _, item := range items[start:end] {
		key := item.fallback
		if item.action != "" {
			if configured := configuredKeyForAction(item.action); configured != "" {
				key = configured
			} else if item.action != "Quit" && item.action != "CommandMode" && item.action != "ToggleHelp" {
				continue
			}
		}
		labels = append(labels, key+item.label[len(item.fallback):])
	}
	if row == 0 {
		labels = append([]string{"^W Switch Panes"}, labels...)
	}
	return strings.Join(labels, ", ")
}

func configuredKeyForAction(want string) string {
	keys := make([]string, 0, len(config.Bindings["buffer"]))
	for key := range config.Bindings["buffer"] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		actionList := config.Bindings["buffer"][key]
		for _, actionName := range strings.FieldsFunc(actionList, func(r rune) bool { return r == '|' || r == '&' || r == ',' }) {
			if actionName == want {
				return displayBindingKey(key)
			}
		}
	}
	return ""
}

func displayBindingKey(key string) string {
	key = strings.Trim(key, "<>")
	key = strings.ReplaceAll(key, "Ctrl-", "^")
	key = strings.ReplaceAll(key, "Ctrl", "^")
	if len(key) == 2 && key[0] == '^' {
		key = strings.ToUpper(key)
	}
	return key
}

func microMenuStyle() tcell.Style {
	if style, ok := config.Colorscheme["message"]; ok {
		return style
	}
	return config.DefStyle
}

func (a *App) focusNameIndicator() string {
	if a.focus == viewport.Right {
		return "Right"
	}
	return "Left"
}

func (a *App) renderMenuLine(y int, left, right string, style tcell.Style) {
	_, height := a.surface.Size()
	if y < 0 || y >= height {
		return
	}
	for x := 0; x < a.cols; x++ {
		screen.SetContent(x, y, ' ', nil, style)
	}
	leftRunes, rightRunes := []rune(left), []rune(right)
	rightRunes = truncateRunes(rightRunes, a.cols)
	leftRunes = truncateRunes(leftRunes, max(0, a.cols-len(rightRunes)-1))
	for x, r := range leftRunes {
		screen.SetContent(x, y, r, nil, style)
	}
	start := a.cols - len(rightRunes)
	for i, r := range rightRunes {
		screen.SetContent(start+i, y, r, nil, style)
	}
}

func truncateRunes(text []rune, width int) []rune {
	if width < 0 {
		width = 0
	}
	if len(text) > width {
		return text[:width]
	}
	return text
}

func (a *App) promptDisplay() (string, int) {
	if action.InfoBar == nil {
		return "", 0
	}
	prefix := []rune(action.InfoBar.Msg)
	input := []rune(string(action.InfoBar.LineBytes(0)))
	if len(prefix) > a.cols {
		prefix = prefix[:a.cols]
	}
	available := max(0, a.cols-len(prefix))
	cursor := min(max(action.InfoBar.Cursor.X, 0), len(input))
	start := 0
	if available > 0 {
		start = max(0, cursor-available+1)
	}
	start = min(start, len(input))
	end := min(len(input), start+available)
	line := append([]rune(nil), prefix...)
	line = append(line, input[start:end]...)
	cursorX := len(prefix) + cursor - start
	if cursorX >= a.cols {
		cursorX = a.cols - 1
	}
	return string(line), cursorX
}

func (a *App) cursorPosition() (x, y int, visible bool) {
	if action.InfoBar != nil && action.InfoBar.HasPrompt {
		_, x = a.promptDisplay()
		return x, a.statusRow, true
	}
	p := a.focusedPane()
	loc := p.h.Cursor.Loc
	visual := p.view.VLocFromLoc(loc)
	row := a.layout.CursorRow(a.focus, loc.Y, visual.Row)
	if row < a.top || row >= a.top+a.rows || row < 0 {
		return 0, 0, false
	}
	// VisualX is relative to Micro's text area. Include its line-number,
	// diff, and message gutters so the terminal cursor lands on the glyph.
	x = p.view.BufView().X + visual.VisualX - p.view.StartCol
	if x < p.x || x >= p.x+p.width {
		return 0, 0, false
	}
	return x, row - a.top, true
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
				a.syncFocusedBuffer()
				a.draw()
			}
		case event, ok := <-events:
			if !ok {
				return errors.New("event stream closed")
			}
			if a.handle(event) {
				return nil
			}
			a.draw()
		}
	}
}
