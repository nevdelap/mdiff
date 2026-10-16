package prototype

import (
	"fmt"
	"strings"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
)

// Help pages use Micro's help buffer type and renderer in a temporary pane.
// Requested split flags are parsed but represented in one fixed-layout overlay.
func (a *App) openHelpTopic(topic string) {
	a.openHelpTopics([]string{topic})
}

func (a *App) openHelpTopics(topics []string) {
	filtered := make([]string, 0, len(topics))
	forceSplit := false
	for _, topic := range topics {
		if topic == "-vsplit" || topic == "-hsplit" {
			if forceSplit {
				action.InfoBar.Error("hsplit and vsplit are not allowed at the same time")
				return
			}
			forceSplit = true
			continue
		}
		filtered = append(filtered, topic)
	}
	if len(filtered) == 0 {
		filtered = []string{"help"}
	}
	var pages []string
	for _, topic := range filtered {
		file := config.FindRuntimeFile(config.RTHelp, topic)
		if file == nil {
			action.InfoBar.Error("Help topic not found: " + topic)
			return
		}
		data, err := file.Data()
		if err != nil {
			action.InfoBar.Error(fmt.Sprintf("Unable to load help topic %s: %v", topic, err))
			return
		}
		pages = append(pages, string(data))
	}
	a.helpTopic = strings.Join(filtered, ", ")
	if len(pages) == 1 {
		a.helpText = pages[0]
	} else {
		for i, page := range pages {
			pages[i] = "# " + filtered[i] + "\n\n" + page
		}
		a.helpText = strings.Join(pages, "\n\n")
	}
	if a.helpBuf != nil {
		a.helpBuf.Close()
	}
	a.helpBuf = buffer.NewBufferFromString(a.helpText, filtered[0]+".md", buffer.BTHelp)
	a.helpBuf.SetName("Help " + a.helpTopic)
	if err := a.helpBuf.SetOptionNative("hltaberrors", false); err != nil {
		action.InfoBar.Error(err)
	}
	if err := a.helpBuf.SetOptionNative("hltrailingws", false); err != nil {
		action.InfoBar.Error(err)
	}
	a.helpWindow = display.NewBufWindow(0, 0, a.cols, a.rows, a.helpBuf)
	a.helpPane = action.NewBufPane(a.helpBuf, a.helpWindow, a.microTab)
	a.helpPane.PromptBufferName = func() string { return "Help " + a.helpTopic }
	a.helpPane.MouseLoc = func(event *tcell.EventMouse) buffer.Loc {
		x, y := event.Position()
		return a.helpWindow.LocFromVisual(buffer.Loc{X: x, Y: y})
	}
	a.helpVisible = true
	a.installHelpAsCurrentPane()
}

func (a *App) installHelpAsCurrentPane() {
	if a.microTab == nil || a.helpPane == nil {
		return
	}
	if a.helpTabOriginal == nil {
		a.helpTabSlot = int(a.focus)
		a.helpTabOriginal = a.microTab.Panes[a.helpTabSlot]
	}
	a.helpPane.SetID(a.helpTabOriginal.ID())
	a.helpPane.SetTab(a.microTab)
	a.microTab.Panes[a.helpTabSlot] = a.helpPane
	a.microTab.SetActive(a.helpTabSlot)
}

func (a *App) closeHelpOverlay() {
	a.helpVisible = false
	if a.microTab != nil && a.helpTabOriginal != nil {
		a.microTab.Panes[a.helpTabSlot] = a.helpTabOriginal
		a.helpTabOriginal.SetTab(a.microTab)
		a.helpTabOriginal.SetActive(true)
		a.microTab.SetActive(int(a.focus))
	}
	a.helpTabOriginal = nil
}

func (a *App) handleHelpKey(e *tcell.EventKey) bool {
	if a.helpPane != nil {
		a.helpPane.HandleEvent(e)
	}
	return a.quitComplete
}

func (a *App) handleHelpMouse(e *tcell.EventMouse) bool {
	if a.helpPane != nil {
		a.helpPane.HandleEvent(e)
	}
	return a.quitComplete
}
