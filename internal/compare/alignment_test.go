package compare

import (
	"reflect"
	"strconv"
	"testing"
	"unicode/utf8"
)

func TestAlignAddsVirtualSpacerForInsertion(t *testing.T) {
	rows := Align([]string{"alpha", "omega"}, []string{"alpha", "middle", "omega"})
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %#v", len(rows), rows)
	}
	if rows[1].Kind != Spacer || (rows[1].Left < 0) == (rows[1].Right < 0) {
		t.Fatalf("middle row is not a one-sided virtual spacer: %#v", rows[1])
	}
	if !reflect.DeepEqual(rows[0], Pair{Left: 0, Right: 0, Kind: Equal}) || !reflect.DeepEqual(rows[2], Pair{Left: 1, Right: 2, Kind: Equal}) {
		t.Fatalf("equal lines lost their correspondence: %#v", rows)
	}
}

func TestAlignCountsRuneEncodedLineTokens(t *testing.T) {
	leftPrefix := make([]string, 140)
	for i := range leftPrefix {
		leftPrefix[i] = "line-" + strconv.Itoa(i)
	}
	left := append(append([]string(nil), leftPrefix...), "tail")
	right := append(append([]string(nil), leftPrefix...), "inserted", "tail")
	rows := Align(left, right)
	if len(rows) != len(right) {
		t.Fatalf("got %d aligned rows for %d right lines; rune token counts were likely treated as byte counts", len(rows), len(right))
	}
	if row := rows[len(rows)-2]; row.Left != -1 || row.Right != len(right)-2 || row.Kind != Spacer {
		t.Fatalf("inserted high-token line was not represented by one spacer: %#v", row)
	}
	if rows[len(rows)-1].Left != len(left)-1 || rows[len(rows)-1].Right != len(right)-1 || rows[len(rows)-1].Kind != Equal {
		t.Fatalf("tail lines lost alignment after high-token insertion: %#v", rows[len(rows)-1])
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
	if len(rows[1].Changes) == 0 || rows[1].Changes[0].Left == (RuneSpan{}) || rows[1].Changes[0].Right == (RuneSpan{}) {
		t.Fatalf("paired replacement row has no per-side character spans: %#v", rows[1])
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
		if !reflect.DeepEqual(row, Pair{Left: i, Right: i, Kind: Equal}) {
			t.Fatalf("row %d is not identity: %#v", i, row)
		}
	}
}

func TestDiffLineChangedSpans(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
		wantLeft    []RuneSpan
		wantRight   []RuneSpan
	}{
		{name: "identical", left: "same", right: "same"},
		{name: "insertion", left: "cat", right: "caXt", wantLeft: []RuneSpan{{Start: 2, End: 2}}, wantRight: []RuneSpan{{Start: 2, End: 3}}},
		{name: "deletion", left: "caXt", right: "cat", wantLeft: []RuneSpan{{Start: 2, End: 3}}, wantRight: []RuneSpan{{Start: 2, End: 2}}},
		{name: "replacement", left: "cat", right: "cut", wantLeft: []RuneSpan{{Start: 1, End: 2}}, wantRight: []RuneSpan{{Start: 1, End: 2}}},
		{name: "disjoint", left: "abcdef", right: "aXcdYf", wantLeft: []RuneSpan{{Start: 1, End: 2}, {Start: 4, End: 5}}, wantRight: []RuneSpan{{Start: 1, End: 2}, {Start: 4, End: 5}}},
		{name: "empty lines", left: "", right: ""},
		{name: "empty to text", left: "", right: "word", wantLeft: []RuneSpan{{Start: 0, End: 0}}, wantRight: []RuneSpan{{Start: 0, End: 4}}},
		{name: "unicode runes", left: "a猫z", right: "a犬z", wantLeft: []RuneSpan{{Start: 1, End: 2}}, wantRight: []RuneSpan{{Start: 1, End: 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := DiffLine(tt.left, tt.right)
			if got := leftSpans(changes); !reflect.DeepEqual(got, tt.wantLeft) {
				t.Fatalf("left spans = %#v, want %#v (changes %#v)", got, tt.wantLeft, changes)
			}
			if got := rightSpans(changes); !reflect.DeepEqual(got, tt.wantRight) {
				t.Fatalf("right spans = %#v, want %#v (changes %#v)", got, tt.wantRight, changes)
			}
			if got := reconstructRight(tt.left, tt.right, changes); got != tt.right {
				t.Fatalf("spans reconstructed %q, want %q", got, tt.right)
			}
			if got := reconstructLeft(tt.left, tt.right, changes); got != tt.left {
				t.Fatalf("spans reconstructed %q, want %q", got, tt.left)
			}
		})
	}
}

func TestAlignAttachesWholeLineSpansToSpacerRows(t *testing.T) {
	rows := Align([]string{"keep", "removed"}, []string{"keep", "added"})
	if len(rows) != 2 || rows[1].Kind != Changed {
		t.Fatalf("replacement was not paired: %#v", rows)
	}
	rows = Align([]string{"keep", "removed"}, []string{"keep"})
	if rows[1].Kind != Spacer || !reflect.DeepEqual(rows[1].Changes, []ChangeSpan{{Left: RuneSpan{Start: 0, End: 7}}}) {
		t.Fatalf("deleted line does not have a whole-line span: %#v", rows[1])
	}
	rows = Align([]string{"keep"}, []string{"keep", "added"})
	if rows[1].Kind != Spacer || !reflect.DeepEqual(rows[1].Changes, []ChangeSpan{{Right: RuneSpan{Start: 0, End: 5}}}) {
		t.Fatalf("inserted line does not have a whole-line span: %#v", rows[1])
	}
}

func FuzzDiffLineSpansReconstructRight(f *testing.F) {
	for _, pair := range [][2]string{{"", ""}, {"abc", "axc"}, {"ab", "aXYb"}, {"a🐈z", "a🐕z"}, {"e\u0301", "e\u0300"}} {
		f.Add(pair[0], pair[1])
	}
	f.Fuzz(func(t *testing.T, left, right string) {
		if !utf8.ValidString(left) || !utf8.ValidString(right) {
			t.Skip()
		}
		changes := DiffLine(left, right)
		if got := reconstructRight(left, right, changes); got != right {
			t.Fatalf("spans reconstructed %q, want %q; changes=%#v", got, right, changes)
		}
		if got := reconstructLeft(left, right, changes); got != left {
			t.Fatalf("spans reconstructed %q, want %q; changes=%#v", got, left, changes)
		}
	})
}

func leftSpans(changes []ChangeSpan) []RuneSpan {
	if len(changes) == 0 {
		return nil
	}
	spans := make([]RuneSpan, len(changes))
	for i, change := range changes {
		spans[i] = change.Left
	}
	return spans
}

func rightSpans(changes []ChangeSpan) []RuneSpan {
	if len(changes) == 0 {
		return nil
	}
	spans := make([]RuneSpan, len(changes))
	for i, change := range changes {
		spans[i] = change.Right
	}
	return spans
}

func reconstructRight(left, right string, changes []ChangeSpan) string {
	leftRunes, rightRunes := []rune(left), []rune(right)
	var out []rune
	leftAt, rightAt := 0, 0
	for _, change := range changes {
		if change.Left.Start < leftAt || change.Right.Start < rightAt ||
			change.Left.End < change.Left.Start || change.Right.End < change.Right.Start ||
			change.Left.End > len(leftRunes) || change.Right.End > len(rightRunes) {
			return "<invalid spans>"
		}
		leftEqual := leftRunes[leftAt:change.Left.Start]
		rightEqual := rightRunes[rightAt:change.Right.Start]
		if !reflect.DeepEqual(leftEqual, rightEqual) {
			return "<unequal context>"
		}
		out = append(out, rightEqual...)
		out = append(out, rightRunes[change.Right.Start:change.Right.End]...)
		leftAt, rightAt = change.Left.End, change.Right.End
	}
	leftTail, rightTail := leftRunes[leftAt:], rightRunes[rightAt:]
	if !reflect.DeepEqual(leftTail, rightTail) {
		return "<unequal suffix>"
	}
	out = append(out, rightTail...)
	return string(out)
}

func reconstructLeft(left, right string, changes []ChangeSpan) string {
	reversed := make([]ChangeSpan, len(changes))
	for i, change := range changes {
		reversed[i] = ChangeSpan{Left: change.Right, Right: change.Left}
	}
	return reconstructRight(right, left, reversed)
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
