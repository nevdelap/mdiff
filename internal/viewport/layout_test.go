package viewport

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"

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

func TestExpandCarriesCharacterChangesIntoVisualRows(t *testing.T) {
	changes := []compare.ChangeSpan{{
		Left:  compare.RuneSpan{Start: 1, End: 2},
		Right: compare.RuneSpan{Start: 1, End: 3},
	}}
	pairs := []compare.Pair{{Left: 0, Right: 0, Kind: compare.Changed, Changes: changes}}
	layout := Expand(pairs, func(int) int { return 2 }, func(int) int { return 1 })
	if len(layout.Rows) != 2 {
		t.Fatalf("got %d expanded rows, want 2", len(layout.Rows))
	}
	for i, row := range layout.Rows {
		if len(row.Changes) != 1 || row.Changes[0] != changes[0] {
			t.Fatalf("visual row %d lost character spans: %#v", i, row)
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
	if got := layout.WrapHeight(Left, 0); got != 2 {
		t.Fatalf("left wrap height = %d, want 2", got)
	}
	if got := layout.CursorRow(Left, 0, 2); got != -1 {
		t.Fatalf("invalid wrap subrow maps to %d, want -1", got)
	}
}

func TestLogicalAndVisualCoordinatesAreInverseForGeneratedLayouts(t *testing.T) {
	property := func(leftSeed, rightSeed int64) bool {
		left := generatedLines(leftSeed)
		right := generatedLines(rightSeed)
		pairs := compare.Align(left, right)
		layout := Expand(pairs, func(line int) int { return generatedHeight(leftSeed, line) }, func(line int) int { return generatedHeight(rightSeed, line) })
		for _, side := range []Side{Left, Right} {
			lines := left
			if side == Right {
				lines = right
			}
			lastStart := -1
			for line := range lines {
				start := layout.RowFor(side, line)
				if start < 0 || start <= lastStart || layout.WrapHeight(side, line) < 1 {
					return false
				}
				lastStart = start
				for subrow := 0; subrow < layout.WrapHeight(side, line); subrow++ {
					visual := layout.CursorRow(side, line, subrow)
					cell := layout.CellAt(side, visual)
					if visual != start+subrow || cell.Line != line || cell.WrapRow != subrow || cell.Spacer || cell.Continuation {
						return false
					}
				}
				if layout.CursorRow(side, line, layout.WrapHeight(side, line)) != -1 {
					return false
				}
			}
		}
		for visual := range layout.Rows {
			for _, side := range []Side{Left, Right} {
				cell := layout.CellAt(side, visual)
				if cell.Line < 0 || cell.Spacer || cell.Continuation {
					continue
				}
				if layout.CursorRow(side, cell.Line, cell.WrapRow) != visual {
					return false
				}
			}
		}
		return true
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 500, Rand: rand.New(rand.NewSource(20261013))}); err != nil {
		t.Fatal(err)
	}
}

func generatedLines(seed int64) []string {
	count := 1 + int(uint64(seed)>>60)%12
	lines := make([]string, count)
	state := uint64(seed)
	for i := range lines {
		state = state*6364136223846793005 + 1442695040888963407
		lines[i] = fmt.Sprintf("line-%d", (state>>32)%7)
	}
	return lines
}

func generatedHeight(seed int64, line int) int {
	return 1 + int((uint64(seed)+uint64(line)*17)%4)
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
