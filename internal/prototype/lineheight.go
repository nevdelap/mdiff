package prototype

import (
	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/viewport"
)

const maxLineHeightEntries = 200_000

type lineHeightKey struct {
	side               viewport.Side
	lineVersion        uint64
	width, tabSize     int
	softwrap, wordwrap bool
}

// lineHeightCache assigns stable per-line content versions. Unchanged line
// identities keep their cached widths when edits insert or remove lines above.
type lineHeightCache struct {
	lines  [2][]uint64
	values map[lineHeightKey]int
	nextID uint64
}

func newLineHeightCache(leftLines, rightLines int) *lineHeightCache {
	c := &lineHeightCache{values: make(map[lineHeightKey]int)}
	c.lines[viewport.Left] = c.newIDs(leftLines)
	c.lines[viewport.Right] = c.newIDs(rightLines)
	return c
}

func (c *lineHeightCache) newIDs(count int) []uint64 {
	ids := make([]uint64, count)
	for i := range ids {
		c.nextID++
		ids[i] = c.nextID
	}
	return ids
}

func (c *lineHeightCache) height(side viewport.Side, line, width, tabSize int, softwrap, wordwrap bool, compute func() int) int {
	if c == nil || line < 0 || line >= len(c.lines[side]) {
		return compute()
	}
	key := lineHeightKey{side: side, lineVersion: c.lines[side][line], width: width, tabSize: tabSize, softwrap: softwrap, wordwrap: wordwrap}
	if height, ok := c.values[key]; ok {
		return height
	}
	height := compute()
	if len(c.values) >= maxLineHeightEntries {
		clear(c.values)
	}
	c.values[key] = height
	return height
}

func (c *lineHeightCache) reset(side viewport.Side, lineCount int) {
	if c == nil {
		return
	}
	for key := range c.values {
		if key.side == side {
			delete(c.values, key)
		}
	}
	c.lines[side] = c.newIDs(lineCount)
}

func (c *lineHeightCache) change(side viewport.Side, ranges []buffer.TextChangeRange, lineCount int) {
	if c == nil {
		return
	}
	old := c.lines[side]
	if len(ranges) != 1 || len(old) == 0 {
		c.lines[side] = c.newIDs(lineCount)
		return
	}
	r := ranges[0]
	start := max(0, min(r.StartLine, len(old)-1))
	end := max(start, min(r.EndLine, len(old)-1))
	oldAffectedEnd := end + 1
	newAffectedCount := max(1, oldAffectedEnd-start+r.LineDelta)
	newIDs := make([]uint64, lineCount)
	prefixEnd := min(start, lineCount)
	copy(newIDs[:prefixEnd], old[:prefixEnd])
	changedEnd := min(lineCount, start+newAffectedCount)
	for i := start; i < changedEnd; i++ {
		c.nextID++
		newIDs[i] = c.nextID
	}
	oldSuffix := oldAffectedEnd
	newSuffix := changedEnd
	if oldSuffix < len(old) && newSuffix < len(newIDs) {
		copy(newIDs[newSuffix:], old[oldSuffix:])
	}
	for i := range newIDs {
		if newIDs[i] == 0 {
			c.nextID++
			newIDs[i] = c.nextID
		}
	}
	changedIDs := make(map[uint64]struct{}, oldAffectedEnd-start)
	for _, id := range old[start:oldAffectedEnd] {
		changedIDs[id] = struct{}{}
	}
	for key := range c.values {
		if key.side == side {
			if _, affected := changedIDs[key.lineVersion]; affected {
				delete(c.values, key)
			}
		}
	}
	c.lines[side] = newIDs
}
