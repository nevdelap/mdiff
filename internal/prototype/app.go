// Package prototype contains the disposable TASK-002 Micro integration probe.
package prototype

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/compare"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
	ulua "github.com/nevdelap/mdiff/internal/micro/lua"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/viewport"
	lua "github.com/yuin/gopher-lua"
)

type pane struct {
	buf   *buffer.Buffer
	view  *display.BufWindow
	h     *action.BufPane
	x     int
	width int
}

// App owns the two transient buffers, panes, alignment, and shared viewport.
type App struct {
	left, right pane
	layout      viewport.Layout
	terminal    tcell.Screen
	surface     tcell.SimulationScreen
	paintScreen tcell.SimulationScreen
	top         int
	rows        int
	cols        int
	focus       viewport.Side
	closed      bool
}

// New initializes Micro's in-memory settings and real buffers/actions. Passing
// nil as the screen initializes the real tcell terminal; tests pass a
// SimulationScreen instead.
func New(s tcell.Screen, configDir string) (*App, error) {
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
		b.SetName("Untitled")
		b.Settings["statusline"] = false
		b.Settings["ruler"] = false
		b.Settings["scrollbar"] = false
		b.Settings["diffgutter"] = false
	}

	app := &App{
		left:        pane{buf: leftBuf},
		right:       pane{buf: rightBuf},
		focus:       viewport.Left,
		terminal:    screen.Screen,
		paintScreen: paintScreen,
		surface:     surface,
	}
	app.resize(s.Size())
	return app, nil
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
	app, err := New(nil, configDir)
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

	app.draw()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-events:
			if app.handle(event) {
				return nil
			}
			app.draw()
		}
	}
}

// Close uses the transient Micro finalizer and restores the terminal exactly
// once. Transient Fini intentionally has no serializer or backup path.
func (a *App) Close() {
	if a.closed {
		return
	}
	a.closed = true
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
	a.cols, a.rows = cols, rows-1 // reserve a footer row
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
		p.h = action.NewBufPane(p.buf, p.view, nil)
	}
	a.rebuildWithAnchor(anchor)
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
	side, cell, p := viewport.Left, row.Left, &a.left
	if cell.Spacer {
		side, cell, p = viewport.Right, row.Right, &a.right
	}
	if cell.Spacer || cell.Line < 0 {
		return anchor{fallback: rowIndex}
	}
	occurrence := 0
	for i := 0; i < cell.Line; i++ {
		if p.buf.Line(i) == p.buf.Line(cell.Line) {
			occurrence++
		}
	}
	return anchor{side: side, text: p.buf.Line(cell.Line), occurrence: occurrence, wrapRow: cell.WrapRow, fallback: rowIndex, valid: true}
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
		a.handleMouse(e)
		return false
	case *tcell.EventKey:
		if e.Key() == tcell.KeyCtrlQ {
			return true
		}
		if e.Key() == tcell.KeyF6 {
			a.focus = 1 - a.focus
			a.setFocus()
			a.ensureCursorVisible()
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
		before := a.captureAnchor()
		p.h.HandleEvent(e)
		p.buf.UpdateRules()
		a.rebuildWithAnchor(before)
	case *tcell.EventPaste:
		before := a.captureAnchor()
		p := a.focusedPane()
		p.h.HandleEvent(e)
		p.buf.UpdateRules()
		a.rebuildWithAnchor(before)
	}
	return false
}

func (a *App) rebuildWithAnchor(previous anchor) {
	left, right := a.snapshots()
	a.layout = viewport.Expand(
		compare.Align(left, right),
		func(line int) int { return a.wrapHeight(&a.left, line) },
		func(line int) int { return a.wrapHeight(&a.right, line) },
	)
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
	footer := " F6: switch   Ctrl-Up/Down: scroll   Alt-Up/Down: diff   Ctrl-Q: quit "
	if action.InfoBar != nil && action.InfoBar.HasMessage {
		footer = " " + action.InfoBar.Msg + " |" + footer
	}
	if a.focus == viewport.Right {
		footer += " [right]"
	} else {
		footer += " [left]"
	}
	for x, r := range []rune(footer) {
		if x >= a.cols {
			break
		}
		screen.SetContent(x, a.rows, r, nil, config.DefStyle.Reverse(true))
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

func (a *App) cursorPosition() (x, y int, visible bool) {
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
	for {
		select {
		case <-ctx.Done():
			return nil
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
