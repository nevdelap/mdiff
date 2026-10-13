package display

import (
	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	"github.com/nevdelap/mdiff/internal/micro/info"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"github.com/nevdelap/mdiff/internal/micro/util"
)

type InfoWindow struct {
	*info.InfoBuf
	*View

	hscroll int
}

func (i *InfoWindow) errStyle() tcell.Style {
	errStyle := config.DefStyle.
		Foreground(tcell.ColorBlack).
		Background(tcell.ColorMaroon)

	if _, ok := config.Colorscheme["error-message"]; ok {
		errStyle = config.Colorscheme["error-message"]
	}

	return errStyle
}

func (i *InfoWindow) defStyle() tcell.Style {
	defStyle := config.DefStyle

	if _, ok := config.Colorscheme["message"]; ok {
		defStyle = config.Colorscheme["message"]
	}

	return defStyle
}

func NewInfoWindow(b *info.InfoBuf) *InfoWindow {
	iw := new(InfoWindow)
	iw.InfoBuf = b
	iw.View = new(View)

	iw.Width, iw.Y = screen.Screen.Size()
	iw.Y--

	return iw
}

func (i *InfoWindow) Resize(w, h int) {
	i.Width = w
	i.Y = h
}

func (i *InfoWindow) SetBuffer(b *buffer.Buffer) {
	i.InfoBuf.Buffer = b
}

func (i *InfoWindow) Relocate() bool   { return false }
func (i *InfoWindow) GetView() *View   { return i.View }
func (i *InfoWindow) SetView(v *View)  {}
func (i *InfoWindow) SetActive(b bool) {}
func (i *InfoWindow) IsActive() bool   { return true }

func (i *InfoWindow) LocFromVisual(vloc buffer.Loc) buffer.Loc {
	c := i.Buffer.GetActiveCursor()
	l := i.Buffer.LineBytes(0)
	n := util.CharacterCountInString(i.Msg)
	return buffer.Loc{c.GetCharPosInLine(l, vloc.X-n), 0}
}

func (i *InfoWindow) BufView() View {
	return View{
		X:         0,
		Y:         i.Y,
		Width:     i.Width,
		Height:    1,
		StartLine: SLoc{0, 0},
		StartCol:  0,
	}
}

func (i *InfoWindow) Scroll(s SLoc, n int) SLoc        { return s }
func (i *InfoWindow) Diff(s1, s2 SLoc) int             { return 0 }
func (i *InfoWindow) SLocFromLoc(loc buffer.Loc) SLoc  { return SLoc{0, 0} }
func (i *InfoWindow) VLocFromLoc(loc buffer.Loc) VLoc  { return VLoc{SLoc{0, 0}, loc.X} }
func (i *InfoWindow) LocFromVLoc(vloc VLoc) buffer.Loc { return buffer.Loc{vloc.VisualX, 0} }

func (i *InfoWindow) Clear() {
	for x := 0; x < i.Width; x++ {
		screen.SetContent(x, i.Y, ' ', nil, i.defStyle())
	}
}

// DisplayPromptRow paints only the prompt row and returns its cursor column.
// Callers composing their own footer can present the cursor after composition.
func (i *InfoWindow) DisplayPromptRow() int {
	return i.promptRow(true)
}

// PromptCursorColumn returns the prompt cursor column without painting.
func (i *InfoWindow) PromptCursorColumn() int {
	return i.promptRow(false)
}

func (i *InfoWindow) promptRow(paint bool) int {
	if paint {
		i.Clear()
	}
	style := i.defStyle()
	if i.HasError {
		style = i.errStyle()
	}
	x := 0
	for msg := i.Msg; len(msg) > 0 && x < i.Width; {
		r, combc, size := util.DecodeCharacterInString(msg)
		width := runewidth.RuneWidth(r)
		if paint && width > 0 && x+width <= i.Width {
			screen.SetContent(x, i.Y, r, combc, style)
		}
		x += width
		msg = msg[size:]
	}
	if !i.HasPrompt || i.Buffer == nil || i.Width <= 0 {
		return 0
	}
	prefixWidth := min(x, i.Width)
	available := i.Width - prefixWidth
	if available <= 0 {
		return i.Width - 1
	}
	line := i.Buffer.LineBytes(0)
	cursor := i.Buffer.GetActiveCursor()
	type character struct {
		r     rune
		combc []rune
		width int
	}
	chars := make([]character, 0, util.CharacterCount(line))
	positions := []int{0}
	for len(line) > 0 {
		r, combc, size := util.DecodeCharacter(line)
		width := runewidth.RuneWidth(r)
		if r == '\t' {
			width = 4 - positions[len(positions)-1]%4
		}
		chars = append(chars, character{r, combc, width})
		positions = append(positions, positions[len(positions)-1]+width)
		line = line[size:]
	}
	index := min(max(cursor.X, 0), len(chars))
	start := 0
	for start < index && positions[index]-positions[start] >= available {
		start++
	}
	cursorX := min(i.Width-1, prefixWidth+positions[index]-positions[start])
	if !paint {
		return cursorX
	}
	selection := i.defStyle().Reverse(true)
	if s, ok := config.Colorscheme["selection"]; ok {
		selection = s
	}
	x = prefixWidth
	for n := start; n < len(chars) && x < i.Width; n++ {
		ch := chars[n]
		if x+ch.width > i.Width {
			break
		}
		charStyle := i.defStyle()
		loc := buffer.Loc{X: n, Y: 0}
		if cursor.HasSelection() &&
			(loc.GreaterEqual(cursor.CurSelection[0]) && loc.LessThan(cursor.CurSelection[1]) ||
				loc.LessThan(cursor.CurSelection[0]) && loc.GreaterEqual(cursor.CurSelection[1])) {
			charStyle = selection
		}
		if ch.r == '\t' {
			for j := 0; j < ch.width; j++ {
				screen.SetContent(x+j, i.Y, ' ', nil, charStyle)
			}
		} else {
			screen.SetContent(x, i.Y, ch.r, ch.combc, charStyle)
		}
		x += ch.width
	}
	return cursorX
}

var keydisplay = []string{"^Q Quit, ^S Save, ^O Open, ^G Help, ^E Command Bar, ^K Cut Line", "^F Find, ^Z Undo, ^Y Redo, ^A Select All, ^D Duplicate Line, ^T New Tab"}

func (i *InfoWindow) displayKeyMenu() {
	// TODO: maybe make this based on the actual keybindings

	for y := 0; y < len(keydisplay); y++ {
		for x := 0; x < i.Width; x++ {
			if x < len(keydisplay[y]) {
				screen.SetContent(x, i.Y-len(keydisplay)+y, rune(keydisplay[y][x]), nil, i.defStyle())
			} else {
				screen.SetContent(x, i.Y-len(keydisplay)+y, ' ', nil, i.defStyle())
			}
		}
	}
}

func (i *InfoWindow) totalSize() int {
	sum := 0
	for _, n := range i.Suggestions {
		sum += runewidth.StringWidth(n) + 1
	}
	return sum
}

func (i *InfoWindow) scrollToSuggestion() {
	x := 0
	s := i.totalSize()

	for j, n := range i.Suggestions {
		c := util.CharacterCountInString(n)
		if j == i.CurSuggestion {
			if x+c >= i.hscroll+i.Width {
				i.hscroll = util.Clamp(x+c+1-i.Width, 0, s-i.Width)
			} else if x < i.hscroll {
				i.hscroll = util.Clamp(x-1, 0, s-i.Width)
			}
			break
		}
		x += c + 1
	}

	if s-i.Width <= 0 {
		i.hscroll = 0
	}
}

func (i *InfoWindow) Display() {
	if i.HasPrompt || config.GlobalSettings["infobar"].(bool) {
		i.Clear()
		if config.GetGlobalOption("keymenu").(bool) {
			i.displayKeyMenu()
		}

		if !i.HasPrompt && !i.HasMessage && !i.HasError {
			return
		}
		if i.HasPrompt {
			screen.ShowCursor(i.DisplayPromptRow(), i.Y)
		} else {
			style := i.defStyle()
			if i.HasError {
				style = i.errStyle()
			}
			x := 0
			for _, c := range i.Msg {
				screen.SetContent(x, i.Y, c, nil, style)
				x += runewidth.RuneWidth(c)
			}
		}
	}

	if i.HasSuggestions && len(i.Suggestions) > 1 {
		i.scrollToSuggestion()

		x := -i.hscroll
		done := false

		statusLineStyle := config.DefStyle.Reverse(true)
		if style, ok := config.Colorscheme["statusline.suggestions"]; ok {
			statusLineStyle = style
		} else if style, ok := config.Colorscheme["statusline"]; ok {
			statusLineStyle = style
		}
		keymenuOffset := 0
		if config.GetGlobalOption("keymenu").(bool) {
			keymenuOffset = len(keydisplay)
		}

		draw := func(r rune, s tcell.Style) {
			y := i.Y - keymenuOffset - 1
			rw := runewidth.RuneWidth(r)
			for j := 0; j < rw; j++ {
				c := r
				if j > 0 {
					c = ' '
				}

				if x == i.Width-1 && !done {
					screen.SetContent(i.Width-1, y, '>', nil, s)
					x++
					break
				} else if x == 0 && i.hscroll > 0 {
					screen.SetContent(0, y, '<', nil, s)
				} else if x >= 0 && x < i.Width {
					screen.SetContent(x, y, c, nil, s)
				}
				x++
			}
		}

		for j, s := range i.Suggestions {
			style := statusLineStyle
			if i.CurSuggestion == j {
				style = style.Reverse(true)
			}
			for _, r := range s {
				draw(r, style)
				// screen.SetContent(x, i.Y-keymenuOffset-1, r, nil, style)
			}
			draw(' ', statusLineStyle)
		}

		for x < i.Width {
			draw(' ', statusLineStyle)
		}
	}
}
