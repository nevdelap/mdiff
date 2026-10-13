package prototype

import (
	"strings"

	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/viewport"
)

// routeMicroAction keeps Micro's configured bindings while adapting actions
// that must operate on mdiff's two buffers or shared viewport.
func (a *App) routeMicroAction(name string, h *action.BufPane) bool {
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
	case "Quit", "QuitAll":
		a.requestQuit()
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
		a.jumpDiff(-1)
	case "DiffNext":
		a.jumpDiff(1)
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
		return false
	}
	return true
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
	if !p.buf.Modified() {
		a.saveAll(index + 1)
		return
	}
	p.h.SaveWithExternalChangeCheck("SaveAll", func() { a.saveAll(index + 1) })
}

func (a *App) runMicroCommand(h *action.BufPane, input string) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return
	}
	command := strings.ToLower(fields[0])
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
		if len(fields) > 2 {
			action.InfoBar.Error("Usage: help [topic]")
			return
		}
		topic := "help"
		if len(fields) == 2 {
			topic = fields[1]
		}
		a.openHelpTopic(topic)
		return
	}
	h.HandleCommand(input)
	a.syncChangedBuffers()
	if command == "set" || command == "toggle" || command == "reset" {
		a.resize(a.cols, a.terminalHeight())
	}
	a.ensureCursorVisible()
}
