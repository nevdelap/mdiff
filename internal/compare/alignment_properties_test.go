package compare

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

func TestAlignCorpus(t *testing.T) {
	type row struct {
		left, right int
		kind        Kind
	}
	cases := []struct {
		name        string
		left, right []string
		want        []row
	}{
		{"separated insertions", []string{"A", "B", "C", "D", "E"}, []string{"A", "X", "B", "C", "Y", "D", "E"}, []row{{0, 0, Equal}, {-1, 1, Spacer}, {1, 2, Equal}, {2, 3, Equal}, {-1, 4, Spacer}, {3, 5, Equal}, {4, 6, Equal}}},
		{"separated deletions", []string{"A", "X", "B", "C", "Y", "D"}, []string{"A", "B", "C", "D"}, []row{{0, 0, Equal}, {1, -1, Spacer}, {2, 1, Equal}, {3, 2, Equal}, {4, -1, Spacer}, {5, 3, Equal}}},
		{"separated replacements", []string{"A", "old1", "B", "old2", "C"}, []string{"A", "new1", "B", "new2", "C"}, []row{{0, 0, Equal}, {1, 1, Changed}, {2, 2, Equal}, {3, 3, Changed}, {4, 4, Equal}}},
		{"repeated context", []string{"A", "repeat", "B", "repeat", "C"}, []string{"A", "repeat", "B", "X", "repeat", "C"}, []row{{0, 0, Equal}, {1, 1, Equal}, {2, 2, Equal}, {-1, 3, Spacer}, {3, 4, Equal}, {4, 5, Equal}}},
		{"empty final line", []string{"", "tail", ""}, []string{"", "tail", "extra", ""}, []row{{0, 0, Equal}, {1, 1, Equal}, {-1, 2, Spacer}, {2, 3, Equal}}},
		{"unicode replacement", []string{"始", "猫", "終"}, []string{"始", "犬", "終"}, []row{{0, 0, Equal}, {1, 1, Changed}, {2, 2, Equal}}},
		{"asymmetric counts", []string{"A"}, []string{"A", "X", "Y", "Z"}, []row{{0, 0, Equal}, {-1, 1, Spacer}, {-1, 2, Spacer}, {-1, 3, Spacer}}},
		{"reordered block", []string{"A", "B", "C", "D"}, []string{"A", "C", "B", "D"}, []row{{0, 0, Equal}, {1, -1, Spacer}, {2, 1, Equal}, {-1, 2, Spacer}, {3, 3, Equal}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pairs := Align(tc.left, tc.right)
			got := make([]row, len(pairs))
			for i, pair := range pairs {
				got[i] = row{pair.Left, pair.Right, pair.Kind}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("alignment = %+v, want %+v", got, tc.want)
			}
			checkPairProperties(t, tc.left, tc.right, pairs)
		})
	}
}

func TestAlignGeneratedProperties(t *testing.T) {
	rng := rand.New(rand.NewSource(20261013))
	alphabet := []string{"", "A", "B", "猫", "e\u0301"}
	for sample := range 500 {
		left := make([]string, rng.Intn(7))
		right := make([]string, rng.Intn(7))
		for i := range left {
			left[i] = alphabet[rng.Intn(len(alphabet))]
		}
		for i := range right {
			right[i] = alphabet[rng.Intn(len(alphabet))]
		}
		pairs, coarse := AlignWithBudget(left, right, DefaultBudget)
		if coarse {
			t.Fatalf("sample %d unexpectedly hit diff budget: left=%q right=%q", sample, left, right)
		}
		checkPairProperties(t, left, right, pairs)
	}
}

func TestAlignUniqueSequenceMatchesIndependentLCS(t *testing.T) {
	rng := rand.New(rand.NewSource(20261014))
	for sample := range 100 {
		var left, right []string
		for line := range 6 {
			text := fmt.Sprintf("unique-%d", line)
			if rng.Intn(3) != 0 {
				left = append(left, text)
			}
			if rng.Intn(3) != 0 {
				right = append(right, text)
			}
		}
		pairs := Align(left, right)
		checkPairProperties(t, left, right, pairs)
		equal := 0
		for _, pair := range pairs {
			if pair.Kind == Equal {
				equal++
			}
		}
		if want := referenceLCS(left, right); equal != want {
			t.Fatalf("sample %d: equal pairs=%d, independent LCS=%d; left=%q right=%q pairs=%+v", sample, equal, want, left, right, pairs)
		}
	}
}

func referenceLCS(left, right []string) int {
	dp := make([][]int, len(left)+1)
	for i := range dp {
		dp[i] = make([]int, len(right)+1)
	}
	for i := len(left) - 1; i >= 0; i-- {
		for j := len(right) - 1; j >= 0; j-- {
			if left[i] == right[j] {
				dp[i][j] = 1 + dp[i+1][j+1]
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	return dp[0][0]
}

func checkPairProperties(t *testing.T, left, right []string, pairs []Pair) {
	t.Helper()
	leftAt, rightAt := 0, 0
	for index, pair := range pairs {
		if pair.Left >= 0 {
			if pair.Left != leftAt || pair.Left >= len(left) {
				t.Fatalf("row %d left projection out of order: %+v", index, pair)
			}
			leftAt++
		}
		if pair.Right >= 0 {
			if pair.Right != rightAt || pair.Right >= len(right) {
				t.Fatalf("row %d right projection out of order: %+v", index, pair)
			}
			rightAt++
		}
		switch pair.Kind {
		case Equal:
			if pair.Left < 0 || pair.Right < 0 || left[pair.Left] != right[pair.Right] || len(pair.Changes) != 0 {
				t.Fatalf("row %d invalid equal pair: %+v", index, pair)
			}
		case Changed:
			if pair.Left < 0 || pair.Right < 0 || left[pair.Left] == right[pair.Right] ||
				reconstructLeft(left[pair.Left], right[pair.Right], pair.Changes) != left[pair.Left] ||
				reconstructRight(left[pair.Left], right[pair.Right], pair.Changes) != right[pair.Right] {
				t.Fatalf("row %d invalid changed pair: %+v", index, pair)
			}
		case Spacer:
			if (pair.Left < 0) == (pair.Right < 0) {
				t.Fatalf("row %d spacer has invalid sides: %+v", index, pair)
			}
		default:
			t.Fatalf("row %d has unknown kind %d", index, pair.Kind)
		}
	}
	if leftAt != len(left) || rightAt != len(right) {
		t.Fatalf("incomplete projections: left=%d/%d right=%d/%d pairs=%+v", leftAt, len(left), rightAt, len(right), pairs)
	}
}
