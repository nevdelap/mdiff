package compare

import (
	"reflect"
	"testing"
)

func TestAlignAddsVirtualSpacerForInsertion(t *testing.T) {
	rows := Align([]string{"alpha", "omega"}, []string{"alpha", "middle", "omega"})
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %#v", len(rows), rows)
	}
	if rows[1].Kind != Spacer || (rows[1].Left < 0) == (rows[1].Right < 0) {
		t.Fatalf("middle row is not a one-sided virtual spacer: %#v", rows[1])
	}
	if rows[0] != (Pair{Left: 0, Right: 0, Kind: Equal}) || rows[2] != (Pair{Left: 1, Right: 2, Kind: Equal}) {
		t.Fatalf("equal lines lost their correspondence: %#v", rows)
	}
}

func TestAlignPairsReplacementsAndPreservesSourceOrder(t *testing.T) {
	left := []string{"before", "old one", "old two", "after"}
	right := []string{"before", "new one", "after"}
	rows := Align(left, right)
	if rows[0].Kind != Equal || rows[len(rows)-1].Kind != Equal {
		t.Fatalf("equal context missing: %#v", rows)
	}
	if rows[1].Kind != Changed || rows[1].Left != 1 || rows[1].Right != 1 {
		t.Fatalf("replacement was not paired: %#v", rows[1])
	}
	if rows[2].Kind != Spacer || rows[2].Left != 2 || rows[2].Right != -1 {
		t.Fatalf("extra deletion was not represented as a spacer: %#v", rows[2])
	}
	if got := project(rows, true); !reflect.DeepEqual(got, []int{0, 1, 2, 3}) {
		t.Fatalf("left source order changed: %v", got)
	}
	if got := project(rows, false); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Fatalf("right source order changed: %v", got)
	}
}

func TestAlignIdentityAndEmptyInput(t *testing.T) {
	if got := Align(nil, nil); len(got) != 0 {
		t.Fatalf("empty inputs produced rows: %#v", got)
	}
	input := []string{"", "same", "same", "last"}
	rows := Align(input, input)
	if len(rows) != len(input) {
		t.Fatalf("identity alignment has %d rows, want %d", len(rows), len(input))
	}
	for i, row := range rows {
		if row != (Pair{Left: i, Right: i, Kind: Equal}) {
			t.Fatalf("row %d is not identity: %#v", i, row)
		}
	}
}

func project(rows []Pair, left bool) []int {
	var result []int
	for _, row := range rows {
		line := row.Right
		if left {
			line = row.Left
		}
		if line >= 0 {
			result = append(result, line)
		}
	}
	return result
}
