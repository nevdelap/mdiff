// Package perfworkload builds deterministic inputs shared by performance
// benchmarks and reports. Fixture formats are versioned in their names.
package perfworkload

import (
	"fmt"
	"math/rand"
	"strings"
)

const RSSFixtureName = "rss-10mib-100k-balanced-near-equal-v1"

type Pair struct {
	Left, Right []string
	Bytes       int
	Name        string
	Version     string
}

func NumberedLines(count int) []string {
	lines := make([]string, count)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%08d", i)
	}
	return lines
}

func ChangedLinePair(count int) Pair {
	left := NumberedLines(count)
	right := append([]string(nil), left...)
	if count > 0 {
		right[count/2] += "-changed"
	}
	return Pair{Name: fmt.Sprintf("near-equal-%d-lines", count), Version: "numbered-lines-v1", Left: left, Right: right, Bytes: byteCount(left) + byteCount(right)}
}

func LargeReplacement(count int) Pair {
	left, right := make([]string, count), make([]string, count)
	for i := 0; i < count; i++ {
		left[i] = fmt.Sprintf("old-%08d", i)
		right[i] = fmt.Sprintf("new-%08d", i)
	}
	return Pair{Name: fmt.Sprintf("replacement-%d-lines", count), Version: "numbered-replacement-v1", Left: left, Right: right, Bytes: byteCount(left) + byteCount(right)}
}

func RepeatedInsertion() Pair {
	left := make([]string, 100_000)
	for i := range left {
		left[i] = "repeat"
	}
	right := make([]string, 100_001)
	copy(right[:50_000], left[:50_000])
	right[50_000] = "changed"
	copy(right[50_001:], left[50_000:])
	return Pair{Name: "repeated-insertion-100k-v1", Version: "repeat-fixture-v1", Left: left, Right: right, Bytes: byteCount(left) + byteCount(right)}
}

func UniqueReverse() Pair {
	left := make([]string, 50_000)
	rng := rand.New(rand.NewSource(20261011))
	for i := range left {
		left[i] = fmt.Sprintf("L%08d:%016x", i, rng.Uint64())
	}
	right := make([]string, len(left))
	for i := range left {
		right[len(left)-1-i] = left[i]
	}
	return Pair{Name: "unique-reverse-50k-seed-20261011-v1", Version: "math-rand-source-20261011-v1", Left: left, Right: right, Bytes: byteCount(left) + byteCount(right)}
}

func LongLine() Pair {
	left := strings.Repeat("x", 1<<20)
	right := []byte(left)
	right[len(right)/2] = 'y'
	leftLines, rightLines := []string{left}, []string{string(right)}
	return Pair{Name: "single-1mib-line-middle-change-v1", Version: "ascii-middle-byte-v1", Left: leftLines, Right: rightLines, Bytes: byteCount(leftLines) + byteCount(rightLines)}
}

func RSSBalancedNearEqual() Pair {
	left := make([]string, 50_000)
	right := make([]string, len(left))
	for i := range left {
		payload := 104
		if i >= 42_880 {
			payload = 103
		}
		line := make([]byte, payload)
		copy(line, fmt.Sprintf("L%05d:", i))
		for offset := 0; offset < payload-6; offset++ {
			line[6+offset] = byte('a' + (i+offset)%26)
		}
		left[i] = string(line)
		rightLine := append([]byte(nil), line...)
		if i%1_000 == 0 {
			index := 6
			rightLine[index] = byte('a' + (rightLine[index]-'a'+1)%26)
		}
		right[i] = string(rightLine)
	}
	return Pair{Name: RSSFixtureName, Version: RSSFixtureName, Left: left, Right: right, Bytes: byteCount(left) + byteCount(right)}
}

func byteCount(lines []string) int {
	total := 0
	for _, line := range lines {
		total += len(line) + 1
	}
	return total
}
