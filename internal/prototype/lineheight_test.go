package prototype

import (
	"testing"

	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/viewport"
)

func TestLineHeightCacheInvalidatesEditedLinesAndRetainsShiftedLines(t *testing.T) {
	cache := newLineHeightCache(3, 1)
	computes := 0
	compute := func() int { computes++; return computes }
	leftLineOne := cache.height(viewport.Left, 1, 30, 4, true, false, compute)
	leftLineZero := cache.height(viewport.Left, 0, 30, 4, true, false, compute)
	rightLineZero := cache.height(viewport.Right, 0, 30, 4, true, false, compute)
	if computes != 3 {
		t.Fatalf("initial cache computations = %d, want 3", computes)
	}

	cache.change(viewport.Left, []buffer.TextChangeRange{{StartLine: 0, EndLine: 0, StartX: 0, LineDelta: 1}}, 4)
	if got := cache.height(viewport.Left, 2, 30, 4, true, false, compute); got != leftLineOne {
		t.Fatalf("shifted unchanged line height = %d, want cached %d", got, leftLineOne)
	}
	if got := cache.height(viewport.Left, 0, 30, 4, true, false, compute); got == leftLineZero {
		t.Fatalf("edited line reused stale height %d", got)
	}
	if got := cache.height(viewport.Right, 0, 30, 4, true, false, compute); got != rightLineZero {
		t.Fatalf("left edit invalidated right line cache: got %d want %d", got, rightLineZero)
	}
	if computes != 4 {
		t.Fatalf("cache computations after edit = %d, want 4", computes)
	}
}
