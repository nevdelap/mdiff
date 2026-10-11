// Package prototype contains the disposable TASK-002 Micro integration probe.
package prototype

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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

// App owns the two transient buffers, panes, alignment, and shared viewport.
type App struct {
	left, right     pane
	layout          viewport.Layout
	terminal        tcell.Screen
	surface         tcell.SimulationScreen
	paintScreen     tcell.SimulationScreen
	top             int
	rows            int
	cols            int
	statusRow       int
	menuRow1        int
	menuRow2        int
	focus           viewport.Side
	closed          bool
	coordinator     *diffCoordinator
	versions        DiffVersions
	layoutVersions  DiffVersions
	pairs           []compare.Pair
	displayedLeft   []string
	displayedRight  []string
	diffErr         error
	handlingPaste   bool
	blockedSequence string
}

// New initializes Micro's in-memory settings and real buffers/actions. Passing
// nil as the screen initializes the real tcell terminal; tests pass a
// SimulationScreen instead.
func New(s tcell.Screen, configDir string) (*App, error) {
	return NewWithDebounce(s, configDir, nil)
}

// NewWithDebounce constructs the app with an optional CLI debounce override.
func NewWithDebounce(s tcell.Screen, configDir string, cliDebounceMS *int) (*App, error) {
	if err := setConfigDir(configDir); err != nil {
		return nil, err
	}
	config.InitRuntimeFiles(true)
	if err := config.ReadSettings(); err != nil {
		return nil, err
	}
	if err := config.InitGlobalSettings(); err != nil {
		return nil, err
	}
	if err := config.InitColorscheme(); err != nil {
		return nil, err
	}
	debounce, err := configuredDebounce(cliDebounceMS)
	if err != nil {
		return nil, err
	}
	// Command history belongs to Micro's command bar, which mdiff does not
	// expose. Keep its shutdown path inside the prototype's no-write policy.
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
	screen.Events = make(chan tcell.Event, 16)
	action.Tabs = nil
	action.InitGlobals()
	action.InitBindings() // Patched Micro helper reads bindings without creating one.
	action.InitCommands()

	leftBuf := buffer.NewBufferFromString("", "", buffer.BTTransient)
	rightBuf := buffer.NewBufferFromString("", "", buffer.BTTransient)
	for _, b := range []*buffer.Buffer{leftBuf, rightBuf} {
		b.Settings["scrollbar"] = false
		b.Settings["diffgutter"] = false
	}

	app := &App{
		left:        pane{buf: leftBuf},
		right:       pane{buf: rightBuf},
		pairs:       compare.Align([]string{""}, []string{""}),
		focus:       viewport.Left,
		terminal:    screen.Screen,
		paintScreen: paintScreen,
		surface:     surface,
		coordinator: newDiffCoordinator(debounce, nil, nil),
	}
	app.versions = DiffVersions{Left: leftBuf.ContentVersion(), Right: rightBuf.ContentVersion()}
	app.displayedLeft, app.displayedRight = app.snapshots()
	app.coordinator.AcceptCurrent(app.versions)
	leftBuf.SetChangeHandler(func(change buffer.TextChange) { app.textChanged(viewport.Left, change) })
	rightBuf.SetChangeHandler(func(change buffer.TextChange) { app.textChanged(viewport.Right, change) })
	app.resize(s.Size())
	return app, nil
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

func (a *App) isDiffStale() bool {
	return a.layoutVersions != a.versions || a.coordinator.Stale()
}

func (a *App) installDiff(result diffResult) {
	completed, current, restartNow := a.coordinator.Complete(result)
	if !current {
		if restartNow {
			left, right := a.snapshots()
			a.coordinator.DebounceElapsed(diffSnapshot{left: left, right: right, versions: a.versions})
		}
		return
	}
	if completed.err != nil {
		a.diffErr = completed.err
		return
	}
	anchor := a.captureAnchor()
	a.pairs = completed.pairs
	a.displayedLeft = append([]string(nil), completed.snapshot.left...)
	a.displayedRight = append([]string(nil), completed.snapshot.right...)
	a.expandWithAnchor(anchor, completed.snapshot.left, completed.snapshot.right)
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
	app, err := NewWithDebounce(nil, configDir, debounceMS)
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

// Close uses the transient Micro finalizer and restores the terminal exactly
// once. Transient Fini intentionally has no serializer or backup path.
func (a *App) Close() {
	if a.closed {
		return
	}
	a.closed = true
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
	// one Micro status line per pane, one shared transient status row, and the
	// two-row Micro key menu. Compact terminals retain an editor row and hide
	// these chrome rows rather than overlapping them.
	a.statusRow, a.menuRow1, a.menuRow2 = -1, -1, -1
	if rows >= 5 {
		a.rows = rows - 4
		a.statusRow = a.rows + 1
		a.menuRow1 = a.rows + 2
		a.menuRow2 = a.rows + 3
	} else {
		a.rows = rows
	}
	leftWidth := cols / 2
	rightWidth := cols - leftWidth - 1
	if rightWidth < 1 {
		rightWidth = 1
	}
	for i, p := range a.panes() {
		p.x = 0
		p.width = leftWidth
		if i == 1 {
			p.x = leftWidth + 1
			p.width = rightWidth
		}
		p.view = display.NewBufWindow(p.x, 0, p.width, a.rows, p.buf)
		p.view.Resize(p.width, a.rows)
		p.view.SetActive((i == 0 && a.focus == viewport.Left) || (i == 1 && a.focus == viewport.Right))
		p.statusView = &display.BufWindow{
			View: &display.View{X: p.x, Y: a.rows, Width: p.width, Height: 1},
			Buf:  p.buf,
		}
		p.statusView.SetActive((i == 0 && a.focus == viewport.Left) || (i == 1 && a.focus == viewport.Right))
		p.h = action.NewBufPane(p.buf, p.view, nil)
	}
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
	end := len([]rune(p.buf.Line(line)))
	return p.view.VLocFromLoc(buffer.Loc{X: end, Y: line}).Row + 1
}

type anchor struct {
	side       viewport.Side
	text       string
	occurrence int
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
	return anchor{side: side, text: text, occurrence: occurrence, wrapRow: cell.WrapRow, fallback: rowIndex, valid: true}
}

func (a *App) ensureCursorVisible() {
	if len(a.layout.Rows) == 0 || a.rows <= 0 {
		a.top = 0
		return
	}
	p, side := &a.left, viewport.Left
	if a.focus == viewport.Right {
		p, side = &a.right, viewport.Right
	}
	loc := p.h.Cursor.Loc
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
		if a.rejectConflictingReservedBinding(e) {
			return false
		}
		if e.Key() == tcell.KeyCtrlQ {
			a.blockedSequence = ""
			return true
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
			return false
		}
		if e.Key() == tcell.KeyCtrlW {
			a.focus = 1 - a.focus
			a.setFocus()
			a.ensureCursorVisible()
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
		p.buf.UpdateRules()
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
		p.buf.UpdateRules()
		a.ensureCursorVisible()
	}
	return false
}

func (a *App) handlePromptEvent(event tcell.Event) {
	if action.InfoBar == nil || !action.InfoBar.HasPrompt {
		return
	}
	action.InfoBar.HandleEvent(event)
	a.focusedPane().buf.UpdateRules()
	a.ensureCursorVisible()
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
		a.blockedSequence = ""
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
		p.buf.UpdateRules()
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
	if strings.Contains(name, "Save") {
		action.InfoBar.Error("Save is unavailable: mdiff buffers exist only in memory")
		return
	}
	if strings.Contains(name, "OpenFile") {
		action.InfoBar.Error("Open is unavailable: mdiff edits exactly two in-memory buffers")
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
	a.pairs = compare.Align(left, right)
	a.displayedLeft = append([]string(nil), left...)
	a.displayedRight = append([]string(nil), right...)
	a.expandWithAnchor(previous, left, right)
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
	a.restoreAnchor(previous, left, right)
	a.ensureCursorVisible()
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
	occurrence := 0
	for line, text := range lines {
		if text != previous.text {
			continue
		}
		if occurrence == previous.occurrence {
			row := a.layout.CursorRow(side, line, previous.wrapRow)
			if row >= 0 {
				a.top = row
				a.clampTop()
				return
			}
		}
		occurrence++
	}
	a.top = previous.fallback
	a.clampTop()
}

func (a *App) focusedPane() *pane {
	if a.focus == viewport.Right {
		return &a.right
	}
	return &a.left
}

func (a *App) setFocus() {
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
		return
	}
	visualX := x - p.x
	if !p.buf.Settings["softwrap"].(bool) {
		visualX += p.view.StartCol
	}
	p.h.GotoLoc(p.view.LocFromVLoc(display.VLoc{
		SLoc: display.SLoc{Line: cell.Line, Row: cell.WrapRow}, VisualX: visualX,
	}))
	a.ensureCursorVisible()
}

func (a *App) jumpDiff(direction int) {
	if len(a.layout.Rows) == 0 {
		return
	}
	start := a.top
	if direction > 0 {
		start++
	} else {
		start--
	}
	for i := start; i >= 0 && i < len(a.layout.Rows); i += direction {
		if a.layout.Rows[i].Kind == compare.Equal {
			continue
		}
		hunkStart := i
		for hunkStart > 0 && a.layout.Rows[hunkStart-1].Kind != compare.Equal {
			hunkStart--
		}
		hunkEnd := i + 1
		for hunkEnd < len(a.layout.Rows) && a.layout.Rows[hunkEnd].Kind != compare.Equal {
			hunkEnd++
		}
		pane, side := &a.left, viewport.Left
		if a.focus == viewport.Right {
			pane, side = &a.right, viewport.Right
		}
		line := -1
		for rowIndex := hunkStart; rowIndex < hunkEnd; rowIndex++ {
			cell := a.layout.Rows[rowIndex].Left
			if side == viewport.Right {
				cell = a.layout.Rows[rowIndex].Right
			}
			if cell.Line >= 0 {
				line = cell.Line
				break
			}
		}
		if line < 0 {
			for distance := 1; line < 0; distance++ {
				rowIndices := []int{hunkStart - distance, hunkEnd - 1 + distance}
				if direction > 0 {
					rowIndices = []int{hunkEnd - 1 + distance, hunkStart - distance}
				}
				for _, rowIndex := range rowIndices {
					if rowIndex < 0 || rowIndex >= len(a.layout.Rows) {
						continue
					}
					cell := a.layout.Rows[rowIndex].Left
					if side == viewport.Right {
						cell = a.layout.Rows[rowIndex].Right
					}
					if cell.Line >= 0 {
						line = cell.Line
						break
					}
				}
				if hunkStart-distance < 0 && hunkEnd-1+distance >= len(a.layout.Rows) {
					break
				}
			}
		}
		if line >= 0 {
			pane.h.GotoLoc(buffer.Loc{X: 0, Y: line})
		}
		a.top = hunkStart
		a.ensureCursorVisible()
		return
	}
}

func (a *App) draw() {
	if a.terminal == nil || a.paintScreen == nil {
		return
	}
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
	status := a.focusName()
	if a.isDiffStale() {
		status = "Diff updating… | " + status
	}
	if a.diffErr != nil {
		status = "Diff error: " + a.diffErr.Error() + " | " + status
	}
	if action.InfoBar != nil && (action.InfoBar.HasMessage || action.InfoBar.HasError) {
		status = action.InfoBar.Msg + " | " + status
	}
	if action.InfoBar != nil && action.InfoBar.HasPrompt {
		status, _ = a.promptDisplay()
	}
	if a.statusRow >= 0 {
		for _, p := range a.panes() {
			display.NewStatusLine(p.statusView).Display()
		}
		a.renderLine(a.statusRow, status, config.DefStyle)
		a.renderMenuLine(a.menuRow1, a.shortcutBar(0), a.focusNameIndicator())
		a.renderMenuLine(a.menuRow2, "^F Find, ^Z Undo, ^Y Redo, ^A Select All, ^D Duplicate Line, ^T New Tab", "mdiff (wrapping micro) "+version.Version)
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

func (a *App) focusName() string {
	if a.focus == viewport.Right {
		return "right"
	}
	return "left"
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

func (a *App) shortcutBar(_ int) string {
	return "^W Switch Panes, ^Q Quit, ^S Save, ^O Open, ^G Help, ^E Command Bar, ^K Cut Line"
}

func (a *App) focusNameIndicator() string {
	if a.focus == viewport.Right {
		return "Right"
	}
	return "Left"
}

func (a *App) renderMenuLine(y int, left, right string) {
	style := config.DefStyle.Reverse(true)
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
	x = p.x + visual.VisualX - p.view.StartCol
	if p.buf.Settings["softwrap"].(bool) {
		x = p.x + visual.VisualX
	}
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
			a.coordinator.DebounceElapsed(diffSnapshot{left: left, right: right, versions: a.versions})
			a.draw()
		case result := <-a.coordinator.Results():
			a.installDiff(result)
			a.draw()
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
