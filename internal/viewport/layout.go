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

// Layout contains expanded rows and source line maps for the current widths.
type Layout struct {
	Rows      []Row
	LeftLine  []int
	RightLine []int
}

// Expand uses each pane's real Micro BufWindow wrap calculation. A paired row
// has the maximum wrap height on either side, so the next logical pair begins
// at the same vertical coordinate in both panels.
func Expand(pairs []compare.Pair, leftHeight, rightHeight func(int) int) Layout {
	layout := Layout{}
	for pairIndex, pair := range pairs {
		lh, rh := 1, 1
		if pair.Left >= 0 {
			lh = max(1, leftHeight(pair.Left))
		}
		if pair.Right >= 0 {
			rh = max(1, rightHeight(pair.Right))
		}
		height := max(lh, rh)
		for subrow := range height {
			row := Row{Kind: pair.Kind, PairIndex: pairIndex, Changes: pair.Changes}
			row.Left = alignedCell(pair.Left, subrow, lh)
			row.Right = alignedCell(pair.Right, subrow, rh)
			layout.Rows = append(layout.Rows, row)
		}
		if pair.Left >= 0 {
			for range lh {
				layout.LeftLine = append(layout.LeftLine, pair.Left)
			}
		}
		if pair.Right >= 0 {
			for range rh {
				layout.RightLine = append(layout.RightLine, pair.Right)
			}
		}
	}
	return layout
}

func alignedCell(line, subrow, height int) Cell {
	if line < 0 {
		return Cell{Line: -1, Spacer: true}
	}
	return Cell{Line: line, WrapRow: subrow, Continuation: subrow >= height}
}

// RowFor returns the first expanded visual row for a logical line.
func (l Layout) RowFor(side Side, line int) int {
	for i, row := range l.Rows {
		cell := row.Left
		if side == Right {
			cell = row.Right
		}
		if cell.Line == line && cell.WrapRow == 0 {
			return i
		}
	}
	return -1
}

// CursorRow returns the expanded visual row for a line and its Micro wrap row.
func (l Layout) CursorRow(side Side, line, wrapRow int) int {
	for i, row := range l.Rows {
		cell := row.Left
		if side == Right {
			cell = row.Right
		}
		if cell.Line == line && cell.WrapRow == wrapRow {
			return i
		}
	}
	return l.RowFor(side, line)
}
