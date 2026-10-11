// Package compare turns two line snapshots into a side-by-side logical row map.
package compare

import (
	"strings"
	"unicode/utf8"

	dmp "github.com/sergi/go-diff/diffmatchpatch"
)

type Kind uint8

const (
	Equal Kind = iota
	Changed
	Spacer
)

// Pair maps one logical alignment row to a line in each buffer. A negative line
// is a virtual spacer and never represents text in the source buffer.
type Pair struct {
	Left    int
	Right   int
	Kind    Kind
	Changes []ChangeSpan
}

// RuneSpan is a half-open range of Unicode rune offsets in one source line.
// It is independent of UTF-8 byte offsets and terminal display columns.
type RuneSpan struct {
	Start int
	End   int
}

// ChangeSpan pairs changed ranges in the left and right source lines. An
// insertion has an empty Left range; a deletion has an empty Right range.
type ChangeSpan struct {
	Left  RuneSpan
	Right RuneSpan
}

// Align computes a line diff with Micro's pinned go-diff dependency. Lines are
// represented by newline-terminated tokens so a final empty Micro buffer line
// remains distinct from the preceding line.
func Align(left, right []string) []Pair {
	leftTokens := lineTokens(left)
	rightTokens := lineTokens(right)
	differ := dmp.New()
	l1, l2, _ := differ.DiffLinesToRunes(leftTokens, rightTokens)
	diffs := differ.DiffMainRunes(l1, l2, false)

	rows := make([]Pair, 0, max(len(left), len(right)))
	li, ri := 0, 0
	for i := 0; i < len(diffs); {
		if diffs[i].Type == dmp.DiffEqual {
			for range diffs[i].Text {
				rows = append(rows, Pair{Left: li, Right: ri, Kind: Equal})
				li++
				ri++
			}
			i++
			continue
		}

		deleted := 0
		inserted := 0
		for i < len(diffs) && diffs[i].Type != dmp.DiffEqual {
			switch diffs[i].Type {
			case dmp.DiffDelete:
				deleted += utf8.RuneCountInString(diffs[i].Text)
			case dmp.DiffInsert:
				inserted += utf8.RuneCountInString(diffs[i].Text)
			}
			i++
		}

		paired := min(deleted, inserted)
		for range paired {
			rows = append(rows, Pair{
				Left: li, Right: ri, Kind: Changed,
				Changes: DiffLine(left[li], right[ri]),
			})
			li++
			ri++
		}
		for range deleted - paired {
			rows = append(rows, Pair{
				Left: li, Right: -1, Kind: Spacer,
				Changes: []ChangeSpan{{Left: RuneSpan{End: len([]rune(left[li]))}}},
			})
			li++
		}
		for range inserted - paired {
			rows = append(rows, Pair{
				Left: -1, Right: ri, Kind: Spacer,
				Changes: []ChangeSpan{{Right: RuneSpan{End: len([]rune(right[ri]))}}},
			})
			ri++
		}
	}
	return rows
}

// DiffLine returns ordered, non-overlapping rune ranges that can replace the
// left changed regions with the right regions while preserving equal context.
// The returned slices belong to the result and are not shared with either input.
func DiffLine(left, right string) []ChangeSpan {
	differ := dmp.New()
	diffs := differ.DiffMainRunes([]rune(left), []rune(right), false)
	changes := make([]ChangeSpan, 0, len(diffs)/2)
	leftPos, rightPos := 0, 0
	var current ChangeSpan
	inChange := false
	for _, diff := range diffs {
		switch diff.Type {
		case dmp.DiffEqual:
			if inChange {
				current.Left.End = leftPos
				current.Right.End = rightPos
				changes = append(changes, current)
				inChange = false
			}
			runeCount := utf8.RuneCountInString(diff.Text)
			leftPos += runeCount
			rightPos += runeCount
		case dmp.DiffDelete:
			if !inChange {
				current = ChangeSpan{
					Left:  RuneSpan{Start: leftPos},
					Right: RuneSpan{Start: rightPos},
				}
				inChange = true
			}
			leftPos += utf8.RuneCountInString(diff.Text)
		case dmp.DiffInsert:
			if !inChange {
				current = ChangeSpan{
					Left:  RuneSpan{Start: leftPos},
					Right: RuneSpan{Start: rightPos},
				}
				inChange = true
			}
			rightPos += utf8.RuneCountInString(diff.Text)
		}
	}
	if inChange {
		current.Left.End = leftPos
		current.Right.End = rightPos
		changes = append(changes, current)
	}
	if len(changes) == 0 {
		return nil
	}
	return changes
}

func lineTokens(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var out strings.Builder
	for _, line := range lines {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}
