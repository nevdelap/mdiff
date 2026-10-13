package prototype

import (
	"fmt"
	"path/filepath"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func (a *App) handlePromptEvent(event tcell.Event) {
	if action.InfoBar == nil || !action.InfoBar.HasPrompt {
		return
	}
	action.InfoBar.HandleEvent(event)
	a.syncChangedBuffers()
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

func (a *App) syncChangedBuffers() {
	var anchor anchor
	changed := false
	for _, p := range a.panes() {
		if p.h == nil || p.h.Buf == nil || p.buf == p.h.Buf {
			continue
		}
		if !changed {
			anchor = a.captureAnchor()
		}
		a.syncPaneBuffer(p)
		changed = true
	}
	if changed {
		a.rebuildWithAnchor(anchor)
	}
}
