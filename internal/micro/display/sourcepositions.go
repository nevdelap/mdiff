package display

import (
	runewidth "github.com/mattn/go-runewidth"
	"github.com/nevdelap/mdiff/internal/micro/util"
)

// RuneBoundaryMap describes a Go rune range in one Micro buffer line. Visual
// uses Micro's decoded-character locations; Character maps each rune boundary
// to a decoded-character offset. Runes contains the range's source runes and
// RuneOffset is its absolute starting rune index. For a base rune and its
// combining marks, the base starts at the character start and each later
// boundary through the marks is the character end.
type RuneBoundaryMap struct {
	Visual     []VLoc
	Character  []int
	Runes      []rune
	RuneOffset int
}

type sourceCharacter struct {
	index, runeIndex, runes, width int
	base                           rune
	marks                          []rune
}

// LineEndVLoc returns the position after the final decoded character without
// allocating a rune map or first counting characters.
func (w *BufWindow) LineEndVLoc(line int) VLoc {
	return w.scanSourceLine(line, -1, -1, nil)
}

// RuneBoundaries scans a line once using the same character widths and wrap
// rules as VLocFromLoc. It allocates O(runes) output and does not call the
// single-position lookup for each rune.
func (w *BufWindow) RuneBoundaries(line int) RuneBoundaryMap {
	result := RuneBoundaryMap{
		Visual:     []VLoc{{SLoc: SLoc{Line: line}}},
		Character:  []int{0},
		RuneOffset: 0,
	}
	previousStart := 0
	hasPrevious := false
	end := w.scanSourceLine(line, -1, -1, func(index, runeIndex, runes int, start, end VLoc, base rune, marks []rune) {
		if hasPrevious {
			for i := previousStart + 1; i < len(result.Visual); i++ {
				result.Visual[i] = start
			}
		}
		previousStart = len(result.Visual) - 1
		result.Visual[previousStart] = start
		result.Character[previousStart] = index
		for range runes {
			result.Visual = append(result.Visual, VLoc{})
			result.Character = append(result.Character, index+1)
		}
		result.Runes = append(result.Runes, base)
		result.Runes = append(result.Runes, marks...)
		hasPrevious = true
	})
	if hasPrevious {
		for i := previousStart + 1; i < len(result.Visual); i++ {
			result.Visual[i] = end
		}
	} else {
		result.Visual[0] = end
	}
	return result
}

// VisibleRuneBoundaries returns source mappings only for characters whose
// cells intersect the unwrapped visual-column range. It scans the prefix to
// preserve tab widths and absolute offsets, but retains no prefix mapping and
// stops at the range's right edge.
func (w *BufWindow) VisibleRuneBoundaries(line, startColumn, width int) RuneBoundaryMap {
	endColumn := startColumn + max(0, width)
	result := RuneBoundaryMap{RuneOffset: -1}
	previous := false
	w.scanSourceLine(line, -1, endColumn, func(index, runeIndex, runes int, start, end VLoc, base rune, marks []rune) {
		if end.VisualX <= startColumn || start.VisualX >= endColumn {
			return
		}
		if !previous {
			result.RuneOffset = runeIndex
			result.Visual = append(result.Visual, start)
			result.Character = append(result.Character, index)
		} else {
			result.Visual[len(result.Visual)-1] = start
			result.Character[len(result.Character)-1] = index
		}
		result.Runes = append(result.Runes, base)
		result.Runes = append(result.Runes, marks...)
		for range runes {
			result.Visual = append(result.Visual, end)
			result.Character = append(result.Character, index+1)
		}
		previous = true
	})
	return result
}

// scanSourceLine emits decoded characters in visual order. A position query
// stops as soon as the requested character's start is known; word wrap can
// require looking ahead to the end of that word to decide its starting row.
func (w *BufWindow) scanSourceLine(line, target, stopColumn int, emit func(index, runeIndex, runes int, start, end VLoc, base rune, marks []rune)) VLoc {
	vloc := VLoc{SLoc: SLoc{Line: line}}
	if target == 0 {
		return vloc
	}
	softwrap := w.Buf.Settings["softwrap"].(bool)
	bufWidth := w.bufWidth
	wordwrap := softwrap && w.Buf.Settings["wordwrap"].(bool)
	tabSize := util.IntOpt(w.Buf.Settings["tabsize"])
	data := w.Buf.LineBytes(line)
	if softwrap && bufWidth <= 0 {
		for index := 0; len(data) > 0; index++ {
			base, marks, size := util.DecodeCharacter(data)
			data = data[size:]
			if target == index {
				return vloc
			}
			if emit != nil {
				emit(index, index, 1+len(marks), vloc, vloc, base, marks)
			}
		}
		return vloc
	}

	var pending []sourceCharacter
	wordWidth, totalWidth, index, runeIndex := 0, 0, 0, 0
	for len(data) > 0 {
		r, marks, size := util.DecodeCharacter(data)
		data = data[size:]
		charWidth := runewidth.RuneWidth(r)
		if r == '\t' {
			tabWidth := tabSize - totalWidth%tabSize
			charWidth = tabWidth
			if softwrap {
				charWidth = min(tabWidth, bufWidth-vloc.VisualX)
			}
			totalWidth += tabWidth
		} else {
			totalWidth += charWidth
		}
		pending = append(pending, sourceCharacter{index: index, runeIndex: runeIndex, runes: 1 + len(marks), width: charWidth, base: r, marks: marks})
		index++
		runeIndex += 1 + len(marks)
		wordWidth += charWidth
		if wordwrap && !util.IsWhitespace(r) && len(data) > 0 && wordWidth < bufWidth {
			continue
		}

		if softwrap && vloc.VisualX+wordWidth > bufWidth && vloc.VisualX > 0 {
			vloc.Row++
			vloc.VisualX = 0
		}
		for _, char := range pending {
			if target == char.index {
				return vloc
			}
			if stopColumn >= 0 && vloc.Row == 0 && vloc.VisualX >= stopColumn {
				return vloc
			}
			start := vloc
			end := vloc
			end.VisualX += char.width
			if emit != nil {
				emit(char.index, char.runeIndex, char.runes, start, end, char.base, char.marks)
			}
			vloc.VisualX += char.width
		}
		pending = pending[:0]
		wordWidth = 0
		if softwrap && vloc.VisualX >= bufWidth {
			vloc.Row++
			vloc.VisualX = 0
		}
	}
	return vloc
}
