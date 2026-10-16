package prototype

import (
	"fmt"

	"github.com/kballard/go-shellquote"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
	"github.com/nevdelap/mdiff/internal/micro/shell"
	"github.com/nevdelap/mdiff/internal/viewport"
)

// routeMicroAction keeps Micro's configured bindings while adapting actions
// that must operate on mdiff's two buffers or shared viewport.
func (a *App) routeMicroAction(name string, h *action.BufPane) (bool, bool) {
	switch name {
	case "DiffPrevious", "DiffNext", "ScrollUp", "ScrollDown", "PageUp", "PageDown", "HalfPageUp", "HalfPageDown", "Start", "End", "Center":
	default:
		a.diffNavValid = false
	}
	if a.helpVisible && h == a.helpPane {
		return a.routeHelpAction(name, h)
	}
	if h != a.left.h && h != a.right.h {
		return false, false
	}
	switch name {
	case "Save":
		h.SaveWithExternalChangeCheck("Save", nil)
	case "SaveAs":
		h.SaveAs()
	case "SaveAll":
		a.saveAll(0)
	case "OpenFile":
		h.OpenFile()
	case "AddTab":
		a.newFocusedDocument()
	case "Suspend":
		if a.handoff == nil {
			action.InfoBar.Error("Suspend requires a terminal session")
			break
		}
		if err := a.handoff(suspendProcess); err != nil {
			a.terminalErr = err
		}
	case "NextSplit", "PreviousSplit":
		a.focus = 1 - a.focus
		a.setFocus()
		a.ensureCursorVisible()
	case "FirstSplit":
		a.focus = 0
		a.setFocus()
		a.ensureCursorVisible()
	case "LastSplit":
		a.focus = 1
		a.setFocus()
		a.ensureCursorVisible()
	case "Quit":
		a.requestQuit()
	case "QuitAll":
		a.requestQuitAll()
	case "ForceQuit":
		a.quitComplete = true
	case "CommandMode":
		a.openCommandBar()
	case "ToggleHelp":
		a.showHelp()
	case "ToggleKeyMenu":
		config.GlobalSettings["keymenu"] = !config.GetGlobalOption("keymenu").(bool)
		a.resize(a.cols, a.terminalHeight())
	case "DiffPrevious":
		return true, a.jumpDiff(-1)
	case "DiffNext":
		return true, a.jumpDiff(1)
	case "CursorPageUp", "CursorPageDown", "SelectPageUp", "SelectPageDown":
		p := &a.left
		if h == a.right.h {
			p = &a.right
		}
		side := viewport.Left
		if p == &a.right {
			side = viewport.Right
		}
		a.syncPaneStartToSharedTop(p, side)
		switch name {
		case "CursorPageUp":
			h.CursorPageUp()
		case "CursorPageDown":
			h.CursorPageDown()
		case "SelectPageUp":
			h.SelectPageUp()
		case "SelectPageDown":
			h.SelectPageDown()
		}
		start := p.view.StartLine
		if row := a.layout.CursorRow(side, start.Line, start.Row); row >= 0 {
			a.top = row
			a.clampTop()
		}
		a.preserveViewport = true
		a.ensureCursorVisible()
	case "ScrollUp":
		a.top -= microIntSetting(h, "scrollspeed")
		a.clampTop()
		a.preserveViewport = true
	case "ScrollDown":
		a.top += microIntSetting(h, "scrollspeed")
		a.clampTop()
		a.preserveViewport = true
	case "PageUp":
		a.top -= max(1, a.rows-microIntSetting(h, "pageoverlap"))
		a.clampTop()
		a.preserveViewport = true
	case "HalfPageUp":
		a.top -= max(1, a.rows/2)
		a.clampTop()
		a.preserveViewport = true
	case "PageDown":
		a.top += max(1, a.rows-microIntSetting(h, "pageoverlap"))
		a.clampTop()
		a.preserveViewport = true
	case "HalfPageDown":
		a.top += max(1, a.rows/2)
		a.clampTop()
		a.preserveViewport = true
	case "Start":
		a.top = 0
		a.preserveViewport = true
	case "End":
		a.top = max(0, len(a.layout.Rows)-a.rows)
		a.preserveViewport = true
	case "Center":
		p, side := &a.left, viewport.Left
		if a.focus == viewport.Right {
			p, side = &a.right, viewport.Right
		}
		loc := p.h.Cursor.Loc
		row := a.layout.CursorRow(side, loc.Y, p.view.VLocFromLoc(loc).Row)
		if row >= 0 {
			a.top = row - a.rows/2
			a.clampTop()
		}
		a.preserveViewport = true
	default:
		return false, false
	}
	return true, true
}

func (a *App) routeHelpAction(name string, h *action.BufPane) (bool, bool) {
	closeHelp := a.closeHelpOverlay
	switch name {
	case "Quit", "ForceQuit", "ToggleHelp":
		closeHelp()
	case "QuitAll":
		a.requestQuitAll()
	case "Save", "SaveAs":
		action.InfoBar.Error("Cannot save readonly help buffer")
	case "SaveAll":
		a.saveAll(0)
	case "OpenFile":
		closeHelp()
		a.focusedPane().h.OpenFile()
	case "AddTab":
		closeHelp()
		a.newFocusedDocument()
	case "PreviousTab", "NextTab", "FirstTab", "LastTab", "VSplit", "HSplit":
		action.InfoBar.Error("Command unavailable in mdiff: " + name)
	case "PreviousSplit", "NextSplit":
		closeHelp()
		a.focus = 1 - a.focus
		a.setFocus()
	case "FirstSplit":
		closeHelp()
		a.focus = viewport.Left
		a.setFocus()
	case "LastSplit":
		closeHelp()
		a.focus = viewport.Right
		a.setFocus()
	case "ToggleKeyMenu":
		config.GlobalSettings["keymenu"] = !config.GetGlobalOption("keymenu").(bool)
		a.resize(a.cols, a.terminalHeight())
	case "CommandMode":
		a.openCommandBarFor(h)
	default:
		return false, false
	}
	return true, true
}

func (a *App) syncPaneStartToSharedTop(p *pane, side viewport.Side) {
	if len(a.layout.Rows) == 0 {
		return
	}
	row := min(max(a.top, 0), len(a.layout.Rows)-1)
	cell := a.layout.CellAt(side, row)
	if cell.Line < 0 || cell.Spacer {
		cell, _ = a.nearestCell(side, row)
	}
	if cell.Line >= 0 {
		p.view.StartLine = display.SLoc{Line: cell.Line, Row: cell.WrapRow}
	}
}

func (a *App) runInteractiveShell(input string, wait, getOutput bool) (string, error) {
	command, err := shell.PrepareInteractiveCommand(input, wait, getOutput)
	if err != nil {
		action.InfoBar.Error(err.Error())
		return "", err
	}
	if a.handoff == nil {
		err := fmt.Errorf("ShellMode requires a terminal session")
		action.InfoBar.Error(err.Error())
		return "", err
	}
	var output string
	var commandErr error
	if err := a.handoff(func() error {
		output, commandErr = command.RunWithReleasedTerminal()
		return nil
	}); err != nil {
		a.terminalErr = err
		return output, err
	}
	if commandErr != nil {
		action.InfoBar.Error(commandErr.Error())
	}
	return output, commandErr
}

func microIntSetting(h *action.BufPane, name string) int {
	if value, ok := h.Buf.Settings[name].(float64); ok {
		return int(value)
	}
	return 0
}

func (a *App) terminalHeight() int {
	_, height := a.terminal.Size()
	return height
}

func (a *App) saveAll(index int) {
	panes := a.panes()
	if index >= len(panes) {
		return
	}
	p := panes[index]
	_ = p.buf.Save()
	a.saveAll(index + 1)
}

func (a *App) runMicroCommand(h *action.BufPane, input string) {
	fields, err := shellquote.Split(input)
	if err != nil {
		action.InfoBar.Error("Error parsing args ", err)
		return
	}
	if len(fields) == 0 {
		return
	}
	command := fields[0]
	if a.helpVisible && h == a.helpPane {
		switch command {
		case "quit", "q":
			a.closeHelpOverlay()
			return
		case "quitall":
			a.requestQuitAll()
			return
		case "save", "saveas":
			action.InfoBar.Error("Cannot save readonly help buffer")
			return
		case "open":
			a.closeHelpOverlay()
			a.focusedPane().h.HandleParsedCommand(input, fields)
			a.syncChangedBuffers()
			return
		}
	}
	if !action.IsRegisteredCommand(command) {
		h.HandleParsedCommand(input, fields)
		return
	}
	if !safeCommand(command) {
		action.InfoBar.Error("Command unavailable in mdiff: " + command)
		return
	}
	switch command {
	case "quit":
		a.requestQuit()
		return
	case "save":
		if len(fields) == 1 {
			h.SaveWithExternalChangeCheck("Save", nil)
			return
		}
	case "help":
		a.openHelpTopics(fields[1:])
		return
	}
	h.HandleParsedCommand(input, fields)
	a.syncChangedBuffers()
	if command == "set" || command == "toggle" || command == "reset" {
		a.resize(a.cols, a.terminalHeight())
	}
	a.ensureCursorVisible()
}
