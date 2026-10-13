// Package compare turns two line snapshots into a side-by-side logical row map.
package compare

import (
	"strings"
	"time"
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

// DefaultBudget bounds line and character comparison work for one snapshot.
// A coarse changed-region alignment is returned when the budget expires.
const DefaultBudget = 1500 * time.Millisecond

// Align computes a line diff with Micro's pinned go-diff dependency and the
// default compute budget. Lines are represented by newline-terminated tokens
// so a final empty Micro buffer line remains distinct from the preceding line.
func Align(left, right []string) []Pair {
	pairs, _ := AlignWithBudget(left, right, DefaultBudget)
	return pairs
}

// AlignWithBudget aligns both snapshots within one shared compute budget. When
// the budget expires, it preserves common line prefixes/suffixes and marks the
// unresolved middle as a coarse changed region. The boolean reports fallback.
func AlignWithBudget(left, right []string, budget time.Duration) ([]Pair, bool) {
	var deadline time.Time
	if budget > 0 {
		deadline = time.Now().Add(budget)
	}
	leftTokens := lineTokens(left)
	rightTokens := lineTokens(right)
	differ := dmp.New()
	if !deadline.IsZero() {
		differ.DiffTimeout = time.Until(deadline)
		if differ.DiffTimeout <= 0 {
			differ.DiffTimeout = time.Nanosecond
		}
	}
	l1, l2, _ := differ.DiffLinesToRunes(leftTokens, rightTokens)
	diffs := differ.DiffMainRunes(l1, l2, false)
	if expired(deadline) {
		return coarseAlignment(left, right), true
	}

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
			if expired(deadline) {
				return coarseAlignment(left, right), true
			}
			changes := diffLine(left[li], right[ri], deadline)
			if expired(deadline) {
				return coarseAlignment(left, right), true
			}
			rows = append(rows, Pair{
				Left: li, Right: ri, Kind: Changed,
				Changes: changes,
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
	return rows, false
}

func expired(deadline time.Time) bool { return !deadline.IsZero() && !time.Now().Before(deadline) }

func coarseAlignment(left, right []string) []Pair {
	prefix := 0
	for prefix < len(left) && prefix < len(right) && left[prefix] == right[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(left)-prefix && suffix < len(right)-prefix &&
		left[len(left)-1-suffix] == right[len(right)-1-suffix] {
		suffix++
	}
	rows := make([]Pair, 0, max(len(left), len(right)))
	for line := 0; line < prefix; line++ {
		rows = append(rows, Pair{Left: line, Right: line, Kind: Equal})
	}
	leftMiddle, rightMiddle := len(left)-prefix-suffix, len(right)-prefix-suffix
	paired := min(leftMiddle, rightMiddle)
	for i := 0; i < paired; i++ {
		rows = append(rows, Pair{Left: prefix + i, Right: prefix + i, Kind: Changed})
	}
	for i := paired; i < leftMiddle; i++ {
		rows = append(rows, Pair{Left: prefix + i, Right: -1, Kind: Spacer})
	}
	for i := paired; i < rightMiddle; i++ {
		rows = append(rows, Pair{Left: -1, Right: prefix + i, Kind: Spacer})
	}
	for i := suffix; i > 0; i-- {
		li, ri := len(left)-i, len(right)-i
		rows = append(rows, Pair{Left: li, Right: ri, Kind: Equal})
	}
	return rows
}

// DiffLine returns ordered, non-overlapping rune ranges that can replace the
// left changed regions with the right regions while preserving equal context.
// The returned slices belong to the result and are not shared with either input.
func DiffLine(left, right string) []ChangeSpan {
	return diffLine(left, right, time.Time{})
}

func diffLine(left, right string, deadline time.Time) []ChangeSpan {
	differ := dmp.New()
	if !deadline.IsZero() {
		differ.DiffTimeout = time.Until(deadline)
		if differ.DiffTimeout <= 0 {
			differ.DiffTimeout = time.Nanosecond
		}
	}
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
	total := len(lines)
	for _, line := range lines {
		total += len(line)
	}
	out.Grow(total)
	for _, line := range lines {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}
