package prototype

import (
	"sort"
	"strings"

	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/compare"
	"github.com/nevdelap/mdiff/internal/micro/action"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/display"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/micro/util"
	"github.com/nevdelap/mdiff/internal/version"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func (a *App) draw() {
	if a.terminal == nil || a.paintScreen == nil {
		return
	}
	width, height := a.terminal.Size()
	if max(2, height) == 2 {
		prompt := action.InfoBar != nil && action.InfoBar.HasPrompt
		if prompt != (a.statusRow >= 0) {
			a.resize(width, height)
		}
	}
	a.refreshLayoutIfNeeded()
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
		marker, style := '│', config.DefStyle
		if row := a.top + y; row < len(a.layout.Rows) {
			marker, style = diffDivider(a.layout.Rows[row])
		}
		screen.SetContent(a.left.width, y, marker, nil, style)
	}
	if a.helpVisible {
		if a.helpWindow != nil {
			a.helpWindow.X, a.helpWindow.Y = 0, 0
			a.helpWindow.Resize(a.cols, a.rows)
			a.helpWindow.Display()
		}
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
		status = ""
	}
	if a.statusRow >= 0 {
		if enabled, _ := config.GetGlobalOption("statusline").(bool); enabled && a.statusRow > a.rows {
			for _, p := range a.panes() {
				display.NewStatusLine(p.statusView).Display()
			}
			screen.SetContent(a.left.width, a.rows, '│', nil, config.DefStyle)
		}
		menuStyle := microMenuStyle()
		if a.menuRow1 >= 0 {
			a.renderMenuLine(a.menuRow1, a.shortcutBar(0), a.focusNameIndicator(), menuStyle)
			a.renderMenuLine(a.menuRow2, a.shortcutBar(1), "mdiff (wrapping micro) "+version.Version, menuStyle)
		}
		if action.InfoBar == nil || !action.InfoBar.HasPrompt {
			a.renderLine(a.statusRow, status, config.DefStyle)
		} else {
			infoWindow := display.NewInfoWindow(action.InfoBar.InfoBuf)
			infoWindow.Resize(a.cols, a.statusRow)
			infoWindow.DisplayPromptRow()
		}
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
		if screen.UseFake() {
			terminal.HideCursor()
			screen.ShowFakeCursorMulti(cursorX, cursorY)
		} else {
			terminal.ShowCursor(cursorX, cursorY)
		}
	} else {
		terminal.HideCursor()
	}
	a.terminal.Show()
}

func diffDivider(row viewport.Row) (rune, tcell.Style) {
	style := config.DefStyle
	_, background, _ := style.Decompose()
	if row.Kind == compare.Equal || background.Hex() >= 0 {
		return '│', style
	}
	marker, group := '!', "diff-modified"
	if row.Kind == compare.Spacer {
		marker, group = '<', "diff-deleted"
		if row.Left.Spacer {
			marker, group = '>', "diff-added"
		}
	}
	if configured, ok := config.Colorscheme[group]; ok {
		foreground, _, attrs := configured.Decompose()
		if foreground != tcell.ColorDefault {
			style = style.Foreground(foreground)
		}
		style = applyAttributes(style, attrs)
	}
	return marker, style
}

func (a *App) renderLine(y int, text string, style tcell.Style) {
	_, height := a.surface.Size()
	if y < 0 || y >= height {
		return
	}
	for x := 0; x < a.cols; x++ {
		screen.SetContent(x, y, ' ', nil, style)
	}
	a.paintText(y, 0, a.cols, text, style)
}

func (a *App) paintText(y, start, end int, text string, style tcell.Style) {
	x := start
	for len(text) > 0 && x < end {
		r, combc, size := util.DecodeCharacterInString(text)
		width := runewidth.RuneWidth(r)
		if r == '\t' {
			width = 4 - (x-start)%4
		}
		if x+width > end {
			break
		}
		if r == '\t' {
			for j := 0; j < width; j++ {
				screen.SetContent(x+j, y, ' ', nil, style)
			}
		} else if width > 0 {
			screen.SetContent(x, y, r, combc, style)
		}
		x += width
		text = text[size:]
	}
}

func (a *App) shortcutBar(row int) string {
	items := []struct{ action, label string }{
		{"NextSplit", "Switch Panes"},
		{"Quit", "Quit"},
		{"Save", "Save"},
		{"OpenFile", "Open"},
		{"ToggleHelp", "Help"},
		{"CommandMode", "Command Bar"},
		{"CutLine", "Cut Line"},
		{"Find", "Find"},
		{"Undo", "Undo"},
		{"Redo", "Redo"},
		{"SelectAll", "Select All"},
		{"DuplicateLine", "Duplicate Line"},
		{"AddTab", "New File"},
	}
	start, end := 0, len(items)
	if row == 0 {
		end = 7
	} else {
		start = 7
	}
	labels := make([]string, 0, end-start)
	for _, item := range items[start:end] {
		if key := configuredKeyForAction(item.action); key != "" {
			labels = append(labels, key+" "+item.label)
		}
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
		if action.RejectedBufferBindings[key] != "" {
			continue
		}
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
	right, rightWidth := textPrefixForCells(right, a.cols)
	left, leftWidth := textPrefixForCells(left, max(0, a.cols-rightWidth-1))
	a.paintText(y, 0, leftWidth, left, style)
	start := a.cols - rightWidth
	a.paintText(y, start, a.cols, right, style)
}

func textPrefixForCells(text string, limit int) (string, int) {
	used, end := 0, 0
	for end < len(text) {
		r, _, size := util.DecodeCharacterInString(text[end:])
		width := runewidth.RuneWidth(r)
		if r == '\t' {
			width = 4 - used%4
		}
		if used+width > limit {
			break
		}
		used += width
		end += size
	}
	return text[:end], used
}

func (a *App) cursorPosition() (x, y int, visible bool) {
	if action.InfoBar != nil && action.InfoBar.HasPrompt {
		infoWindow := display.NewInfoWindow(action.InfoBar.InfoBuf)
		row := a.statusRow
		if row < 0 {
			row = min(1, a.terminalHeight()-1)
		}
		infoWindow.Resize(a.cols, row)
		x = infoWindow.PromptCursorColumn()
		return x, row, row >= 0
	}
	if a.helpVisible && a.helpPane != nil && a.helpWindow != nil {
		loc := a.helpPane.Cursor.Loc
		visual := a.helpWindow.VLocFromLoc(loc)
		position := a.helpWindow.SLocFromLoc(loc)
		current := a.helpWindow.StartLine
		row := 0
		for current.LessThan(position) && row <= a.rows {
			current = a.helpWindow.Scroll(current, 1)
			row++
		}
		return a.helpWindow.BufView().X + visual.VisualX - a.helpWindow.StartCol, row, row >= 0 && row < a.rows
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
