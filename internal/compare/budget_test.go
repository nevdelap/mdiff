package compare

import (
	"testing"
	"time"

	"github.com/nevdelap/mdiff/internal/perfworkload"
)

func TestAlignWithExpiredBudgetReturnsCoarseChangedRegion(t *testing.T) {
	left := []string{"before", "old one", "old two", "after"}
	right := []string{"before", "new one", "new two", "after"}
	pairs, coarse := AlignWithBudget(left, right, time.Nanosecond)
	if !coarse {
		t.Fatal("expired comparison did not report coarse fallback")
	}
	if len(pairs) != 4 || pairs[0].Kind != Equal || pairs[1].Kind != Changed || pairs[2].Kind != Changed || pairs[3].Kind != Equal {
		t.Fatalf("coarse alignment = %#v", pairs)
	}
}

func TestAlignBudgetBoundsAdversarialWork(t *testing.T) {
	fixtures := []perfworkload.Pair{
		perfworkload.RepeatedInsertion(),
		perfworkload.UniqueReverse(),
		perfworkload.LongLine(),
	}
	for _, fixture := range fixtures {
		start := time.Now()
		_, _ = AlignWithBudget(fixture.Left, fixture.Right, 50*time.Millisecond)
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Errorf("fixture %s exceeded bounded-work test allowance: %v", fixture.Name, elapsed)
		}
	}
}

func TestAlignWithBudgetPreservesExactSmallDiff(t *testing.T) {
	pairs, coarse := AlignWithBudget([]string{"a", "old", "z"}, []string{"a", "new", "z"}, time.Second)
	if coarse || len(pairs) != 3 || pairs[1].Kind != Changed || len(pairs[1].Changes) == 0 {
		t.Fatalf("small alignment = %#v, coarse=%v", pairs, coarse)
	}
}
