package viewport

import (
	"reflect"
	"testing"

	"github.com/nevdelap/mdiff/internal/compare"
)

func TestExpandSharesWrapHeightAcrossPairedLines(t *testing.T) {
	pairs := []compare.Pair{
		{Left: 0, Right: 0, Kind: compare.Equal},
		{Left: 1, Right: -1, Kind: compare.Spacer},
	}
	layout := Expand(pairs, func(line int) int { return []int{1, 2}[line] }, func(int) int { return 3 })
	if len(layout.Rows) != 5 {
		t.Fatalf("got %d visual rows, want 5", len(layout.Rows))
	}
	if layout.Rows[0].Left.Line != 0 || layout.Rows[0].Right.Line != 0 {
		t.Fatalf("first pair diverged: %#v", layout.Rows[0])
	}
	for i := 1; i < 3; i++ {
		row := layout.Rows[i]
		if row.Left.Line != 0 || row.Left.WrapRow != i || row.Right.Line != 0 || row.Right.WrapRow != i {
			t.Fatalf("wrapped pair row %d diverged: %#v", i, row)
		}
	}
	for i := 3; i < 5; i++ {
		row := layout.Rows[i]
		if row.Left.Line != 1 || row.Left.WrapRow != i-3 || !row.Right.Spacer {
			t.Fatalf("spacer pair row %d diverged: %#v", i, row)
		}
	}
}

func TestCursorRowAndLineMappingUseSharedCoordinates(t *testing.T) {
	layout := Expand([]compare.Pair{
		{Left: 0, Right: 0, Kind: compare.Equal},
		{Left: 1, Right: -1, Kind: compare.Spacer},
		{Left: 2, Right: 1, Kind: compare.Equal},
	}, func(line int) int { return []int{2, 1, 1}[line] }, func(int) int { return 1 })
	if got := layout.RowFor(Left, 2); got != 3 {
		t.Fatalf("left line 2 starts at %d, want 3", got)
	}
	if got := layout.RowFor(Right, 1); got != 3 {
		t.Fatalf("right line 1 starts at %d, want 3", got)
	}
	if got := layout.CursorRow(Left, 0, 1); got != 1 {
		t.Fatalf("wrapped cursor maps to %d, want 1", got)
	}
}

func TestExpandProjectionPreservesLogicalSourceOrder(t *testing.T) {
	pairs := compare.Align([]string{"a", "b", "c"}, []string{"a", "x", "c"})
	layout := Expand(pairs, func(line int) int { return 1 + line%2 }, func(line int) int { return 1 })
	if got := projectedLines(layout.Rows, true); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Fatalf("left source order changed: %v", got)
	}
	if got := projectedLines(layout.Rows, false); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Fatalf("right source order changed: %v", got)
	}
}

func projectedLines(rows []Row, left bool) []int {
	var lines []int
	for _, row := range rows {
		cell := row.Right
		if left {
			cell = row.Left
		}
		if cell.Line >= 0 && !cell.Continuation && cell.WrapRow == 0 {
			lines = append(lines, cell.Line)
		}
	}
	return lines
}
