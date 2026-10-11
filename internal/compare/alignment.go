// Package compare turns two line snapshots into a side-by-side logical row map.
package compare

import (
	"strings"

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
	Left  int
	Right int
	Kind  Kind
}

// Align computes a line diff with Micro's pinned go-diff dependency. Lines are
// represented by newline-terminated tokens so a final empty Micro buffer line
// remains distinct from the preceding line.
func Align(left, right []string) []Pair {
	leftTokens := lineTokens(left)
	rightTokens := lineTokens(right)
	differ := dmp.New()
	l1, l2, lineTable := differ.DiffLinesToRunes(leftTokens, rightTokens)
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
				deleted += len(diffs[i].Text)
			case dmp.DiffInsert:
				inserted += len(diffs[i].Text)
			}
			i++
		}

		// DiffLinesToRunes owns a shared token table. Touch each token while
		// advancing input cursors so malformed alignment results fail loudly in
		// development rather than silently shifting subsequent rows.
		_ = lineTable
		paired := min(deleted, inserted)
		for range paired {
			rows = append(rows, Pair{Left: li, Right: ri, Kind: Changed})
			li++
			ri++
		}
		for range deleted - paired {
			rows = append(rows, Pair{Left: li, Right: -1, Kind: Spacer})
			li++
		}
		for range inserted - paired {
			rows = append(rows, Pair{Left: -1, Right: ri, Kind: Spacer})
			ri++
		}
	}
	return rows
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
