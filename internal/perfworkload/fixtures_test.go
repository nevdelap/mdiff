package perfworkload

import "testing"

func TestRSSFixtureHasPinnedSizeAndMutations(t *testing.T) {
	fixture := RSSBalancedNearEqual()
	if fixture.Name != RSSFixtureName || len(fixture.Left) != 50_000 || len(fixture.Right) != 50_000 || fixture.Bytes != 10_485_760 {
		t.Fatalf("RSS fixture metadata = %q %d/%d lines %d bytes", fixture.Name, len(fixture.Left), len(fixture.Right), fixture.Bytes)
	}
	changes := 0
	for line := range fixture.Left {
		wantLen := 104
		if line >= 42_880 {
			wantLen = 103
		}
		if len(fixture.Left[line]) != wantLen || len(fixture.Right[line]) != wantLen {
			t.Fatalf("fixture line %d lengths = %d/%d, want %d", line, len(fixture.Left[line]), len(fixture.Right[line]), wantLen)
		}
		if fixture.Left[line] != fixture.Right[line] {
			changes++
			if line%1_000 != 0 || fixture.Right[line][6] != nextLetter(fixture.Left[line][6]) {
				t.Fatalf("unexpected right-side mutation at line %d", line)
			}
		}
	}
	if changes != 50 {
		t.Fatalf("changed lines = %d, want 50", changes)
	}
}

func nextLetter(current byte) byte { return byte('a' + (current-'a'+1)%26) }
