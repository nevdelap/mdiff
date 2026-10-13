package viewport

import (
	"runtime"
	"testing"

	"github.com/nevdelap/mdiff/internal/compare"
	"github.com/nevdelap/mdiff/internal/perfworkload"
)

func BenchmarkExpandUnequalWrapWidths(b *testing.B) {
	fixture := perfworkload.RSSBalancedNearEqual()
	pairs := compare.Align(fixture.Left, fixture.Right)
	b.Logf("environment go=%s os=%s arch=%s cpu=%d fixture=%s generator=%s lines=%d/%d input_bytes=%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), fixture.Name, fixture.Version, len(fixture.Left), len(fixture.Right), fixture.Bytes)
	b.ReportAllocs()
	b.SetBytes(int64(fixture.Bytes))
	leftHeight := func(line int) int { return (len(fixture.Left[line]) + 71) / 72 }
	rightHeight := func(line int) int { return (len(fixture.Right[line]) + 39) / 40 }
	for i := 0; i < b.N; i++ {
		_ = Expand(pairs, leftHeight, rightHeight)
	}
}
