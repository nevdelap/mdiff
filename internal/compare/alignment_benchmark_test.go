package compare

import (
	"runtime"
	"testing"
	"time"

	"github.com/nevdelap/mdiff/internal/perfworkload"
)

func BenchmarkAlign(b *testing.B) {
	fixtures := []struct {
		name string
		make func() perfworkload.Pair
	}{
		{"10k-lines-near-equal", func() perfworkload.Pair { return perfworkload.ChangedLinePair(10_000) }},
		{"100k-lines-one-change", func() perfworkload.Pair { return perfworkload.ChangedLinePair(100_000) }},
		{"large-replacement", func() perfworkload.Pair { return perfworkload.LargeReplacement(50_000) }},
		{"repeated-insertion", perfworkload.RepeatedInsertion},
		{"unique-reversed", perfworkload.UniqueReverse},
		{"single-long-line", perfworkload.LongLine},
	}
	b.Logf("environment go=%s os=%s arch=%s cpu=%d GOMAXPROCS=%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0))
	for _, fixture := range fixtures {
		fixture := fixture
		b.Run(fixture.name, func(b *testing.B) {
			fixture := fixture.make()
			b.ReportAllocs()
			b.SetBytes(int64(fixture.Bytes))
			b.Logf("fixture=%s generator=%s lines=%d/%d input_bytes=%d", fixture.Name, fixture.Version, len(fixture.Left), len(fixture.Right), fixture.Bytes)
			coarse := false
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, coarse = AlignWithBudget(fixture.Left, fixture.Right, 1500*time.Millisecond)
			}
			b.StopTimer()
			b.ReportMetric(float64(perfworkload.PeakRSSBytes()), "peak-rss-B")
			if coarse {
				b.ReportMetric(1, "coarse-fallback/op")
			} else {
				b.ReportMetric(0, "coarse-fallback/op")
			}
		})
	}
}

func BenchmarkRSSMemoryGate(b *testing.B) {
	fixture := perfworkload.RSSBalancedNearEqual()
	b.Logf("environment go=%s os=%s arch=%s cpu=%d fixture=%s generator=%s lines=%d/%d input_bytes=%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), fixture.Name, fixture.Version, len(fixture.Left), len(fixture.Right), fixture.Bytes)
	b.ReportAllocs()
	b.SetBytes(int64(fixture.Bytes))
	for i := 0; i < b.N; i++ {
		_, _ = AlignWithBudget(fixture.Left, fixture.Right, 1500*time.Millisecond)
	}
	b.ReportMetric(float64(perfworkload.PeakRSSBytes()), "peak-rss-B")
}
