package prototype

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/util"
)

type helpMatchPosition struct {
	first, last int
}

// Help uses the bundled Micro pages without opening another editor buffer.
func (a *App) openHelpTopic(topic string) {
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
	a.helpTopic = topic
	a.helpText = string(data)
	a.helpTop = 0
	a.helpQuery = ""
	a.helpMatch = -1
	a.helpVisible = true
}

func (a *App) helpMatches() []helpMatchPosition {
	if a.helpQuery == "" {
		return nil
	}
	search, err := helpSearchRegex(a.helpQuery)
	if err != nil {
		return nil
	}
	width := max(1, a.cols)
	var matches []helpMatchPosition
	row := 0
	for _, line := range strings.Split(a.helpText, "\n") {
		wrapped, runeRows := wrapHelpLine(line, width)
		for _, loc := range search.FindAllStringIndex(line, -1) {
			start := utf8.RuneCountInString(line[:loc[0]])
			end := utf8.RuneCountInString(line[:loc[1]])
			first := len(wrapped) - 1
			if start < len(runeRows) {
				first = runeRows[start].first
			}
			last := first
			if end > start && end-1 < len(runeRows) {
				last = runeRows[end-1].last
			}
			matches = append(matches, helpMatchPosition{first: row + first, last: row + last})
		}
		row += len(wrapped)
	}
	return matches
}

func helpSearchRegex(query string) (*regexp.Regexp, error) {
	ignoreCase, _ := config.GetGlobalOption("ignorecase").(bool)
	return buffer.CompileSearchPattern(query, ignoreCase, true)
}

func (a *App) findHelp(query string) {
	query = strings.TrimSpace(query)
	if query != "" {
		if _, err := helpSearchRegex(query); err != nil {
			action.InfoBar.Error(err)
			return
		}
	}
	a.helpQuery = query
	matches := a.helpMatches()
	a.helpMatch = -1
	if len(matches) == 0 {
		if a.helpQuery != "" {
			action.InfoBar.Error("No help matches: " + a.helpQuery)
		}
		return
	}
	for index, match := range matches {
		if match.first >= a.helpTop {
			a.helpMatch = index
			break
		}
	}
	if a.helpMatch < 0 {
		a.helpMatch = 0
	}
	a.helpTop = matches[a.helpMatch].first
}

func (a *App) nextHelpMatch(delta int) {
	matches := a.helpMatches()
	if len(matches) == 0 {
		return
	}
	a.helpMatch = (a.helpMatch + delta + len(matches)) % len(matches)
	a.helpTop = matches[a.helpMatch].first
	a.scrollHelp(0)
}

func (a *App) openHelpSearch() {
	action.InfoBar.Prompt("Find help (regex): ", a.helpQuery, "HelpSearch", nil, func(query string, canceled bool) {
		if !canceled {
			a.findHelp(query)
		}
	})
}

func (a *App) helpLines() []string {
	width := max(1, a.cols)
	var lines []string
	for _, line := range strings.Split(a.helpText, "\n") {
		wrapped, _ := wrapHelpLine(line, width)
		lines = append(lines, wrapped...)
	}
	return lines
}

// wrapHelpLine returns painted rows and the rows occupied by each source rune.
// Both help navigation and display use this visual-cell mapping.
func wrapHelpLine(line string, width int) ([]string, []helpMatchPosition) {
	width = max(1, width)
	rows := make([]string, 0, 1)
	runeRows := make([]helpMatchPosition, 0, utf8.RuneCountInString(line))
	var current strings.Builder
	x := 0
	flush := func() {
		rows = append(rows, current.String())
		current.Reset()
		x = 0
	}
	for len(line) > 0 {
		r, combc, size := util.DecodeCharacterInString(line)
		if r == '\t' {
			// Expand tabs before wrapping so the painter sees the same cells.
			spaces := 4 - x%4
			occupied := helpMatchPosition{first: len(rows)}
			for j := 0; j < spaces; j++ {
				if x == width {
					flush()
				}
				current.WriteByte(' ')
				x++
				occupied.last = len(rows)
			}
			runeRows = append(runeRows, occupied)
		} else {
			cellWidth := runewidth.RuneWidth(r)
			if x > 0 && x+cellWidth > width {
				flush()
			}
			for range 1 + len(combc) {
				runeRows = append(runeRows, helpMatchPosition{first: len(rows), last: len(rows)})
			}
			current.WriteRune(r)
			for _, mark := range combc {
				current.WriteRune(mark)
			}
			x += cellWidth
		}
		line = line[size:]
	}
	flush()
	return rows, runeRows
}

func (a *App) scrollHelp(delta int) {
	limit := max(0, len(a.helpLines())-max(1, a.rows-1))
	a.helpTop = min(max(0, a.helpTop+delta), limit)
}

func (a *App) nextHelpTopic(delta int) {
	names := make([]string, 0, len(config.ListRuntimeFiles(config.RTHelp)))
	seen := make(map[string]bool)
	for _, file := range config.ListRuntimeFiles(config.RTHelp) {
		if !seen[file.Name()] {
			names = append(names, file.Name())
			seen[file.Name()] = true
		}
	}
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	current := 0
	for i, name := range names {
		if name == a.helpTopic {
			current = i
			break
		}
	}
	a.openHelpTopic(names[(current+delta+len(names))%len(names)])
}

func (a *App) handleHelpKey(e *tcell.EventKey) bool {
	switch e.Key() {
	case tcell.KeyCtrlF:
		a.openHelpSearch()
	case tcell.KeyUp:
		a.scrollHelp(-1)
	case tcell.KeyDown:
		a.scrollHelp(1)
	case tcell.KeyPgUp:
		a.scrollHelp(-max(1, a.rows-2))
	case tcell.KeyPgDn:
		a.scrollHelp(max(1, a.rows-2))
	case tcell.KeyHome:
		a.helpTop = 0
	case tcell.KeyEnd:
		a.scrollHelp(len(a.helpLines()))
	case tcell.KeyLeft:
		a.nextHelpTopic(-1)
	case tcell.KeyRight:
		a.nextHelpTopic(1)
	case tcell.KeyRune:
		switch e.Rune() {
		case 'n':
			a.nextHelpMatch(1)
		case 'N':
			a.nextHelpMatch(-1)
		default:
			a.helpVisible = false
		}
	default:
		a.helpVisible = false
	}
	return false
}

func (a *App) handleHelpMouse(e *tcell.EventMouse) bool {
	switch {
	case e.Buttons()&tcell.WheelUp != 0:
		a.scrollHelp(-3)
	case e.Buttons()&tcell.WheelDown != 0:
		a.scrollHelp(3)
	default:
		a.helpVisible = false
	}
	return false
}
