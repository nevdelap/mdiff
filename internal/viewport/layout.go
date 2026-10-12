// Package viewport expands logical diff rows into one shared visual coordinate.
package viewport

import "github.com/nevdelap/mdiff/internal/compare"

type Side uint8

const (
	Left Side = iota
	Right
)

// Cell identifies the source line and wrapped subrow occupying one pane at a
// shared visual coordinate. Spacer and Continuation cells render as blanks.
type Cell struct {
	Line         int
	WrapRow      int
	Spacer       bool
	Continuation bool
}

type Row struct {
	Left, Right Cell
	Kind        compare.Kind
	PairIndex   int
	Changes     []compare.ChangeSpan
}

// Layout contains expanded rows and logical-line indexes for the current
// widths. The line indexes map each source line to the first shared visual row
// occupied by that line; continuation rows are valid subrows only up to the
// side's own wrap height.
type Layout struct {
	Rows         []Row
	LeftLine     []int
	RightLine    []int
	LeftHeights  []int
	RightHeights []int
}

// Expand uses each pane's real Micro BufWindow wrap calculation. A paired row
// has the maximum wrap height on either side, so the next logical pair begins
// at the same vertical coordinate in both panels.
func Expand(pairs []compare.Pair, leftHeight, rightHeight func(int) int) Layout {
	layout := Layout{
		LeftLine:     make([]int, lineCount(pairs, true)),
		RightLine:    make([]int, lineCount(pairs, false)),
		LeftHeights:  make([]int, lineCount(pairs, true)),
		RightHeights: make([]int, lineCount(pairs, false)),
	}
	for i := range layout.LeftLine {
		layout.LeftLine[i] = -1
	}
	for i := range layout.RightLine {
		layout.RightLine[i] = -1
	}
	for pairIndex, pair := range pairs {
		lh, rh := 1, 1
		if pair.Left >= 0 {
			lh = max(1, leftHeight(pair.Left))
		}
		if pair.Right >= 0 {
			rh = max(1, rightHeight(pair.Right))
		}
		height := max(lh, rh)
		start := len(layout.Rows)
		if pair.Left >= 0 {
			layout.LeftLine[pair.Left] = start
			layout.LeftHeights[pair.Left] = lh
		}
		if pair.Right >= 0 {
			layout.RightLine[pair.Right] = start
			layout.RightHeights[pair.Right] = rh
		}
		for subrow := range height {
			row := Row{Kind: pair.Kind, PairIndex: pairIndex, Changes: pair.Changes}
			row.Left = alignedCell(pair.Left, subrow, lh)
			row.Right = alignedCell(pair.Right, subrow, rh)
			layout.Rows = append(layout.Rows, row)
		}
	}
	return layout
}

func lineCount(pairs []compare.Pair, left bool) int {
	count := 0
	for _, pair := range pairs {
		line := pair.Right
		if left {
			line = pair.Left
		}
		if line >= count {
			count = line + 1
		}
	}
	return count
}

func alignedCell(line, subrow, height int) Cell {
	if line < 0 {
		return Cell{Line: -1, Spacer: true}
	}
	return Cell{Line: line, WrapRow: subrow, Continuation: subrow >= height}
}

// RowFor returns the first expanded visual row for a logical line.
func (l Layout) RowFor(side Side, line int) int {
	starts := l.LeftLine
	if side == Right {
		starts = l.RightLine
	}
	if line < 0 || line >= len(starts) {
		return -1
	}
	return starts[line]
}

// CursorRow returns the expanded visual row for a line and its Micro wrap row.
func (l Layout) CursorRow(side Side, line, wrapRow int) int {
	row := l.RowFor(side, line)
	heights := l.LeftHeights
	if side == Right {
		heights = l.RightHeights
	}
	if row < 0 || line < 0 || line >= len(heights) || wrapRow < 0 || wrapRow >= heights[line] {
		return -1
	}
	return row + wrapRow
}

// WrapHeight reports the number of real wrapped subrows for one source line.
func (l Layout) WrapHeight(side Side, line int) int {
	heights := l.LeftHeights
	if side == Right {
		heights = l.RightHeights
	}
	if line < 0 || line >= len(heights) {
		return 0
	}
	return heights[line]
}

// CellAt returns the source cell at an aligned visual row. Out-of-range rows
// return a spacer cell so callers can handle clipped positions deterministically.
func (l Layout) CellAt(side Side, row int) Cell {
	if row < 0 || row >= len(l.Rows) {
		return Cell{Line: -1, Spacer: true}
	}
	if side == Right {
		return l.Rows[row].Right
	}
	return l.Rows[row].Left
}
