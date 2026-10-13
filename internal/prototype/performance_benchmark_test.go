package prototype

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/buffer"
)

func BenchmarkPasteOneMiBVisible(b *testing.B) {
	text := strings.Repeat("x", 1<<20)
	b.Logf("environment go=%s os=%s arch=%s cpu=%d fixture=one-1mib-ascii-line-v1 bytes=%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), len(text))
	b.ReportAllocs()
	b.SetBytes(int64(len(text)))
	configDir := b.TempDir()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		sim := tcell.NewSimulationScreen("")
		if err := sim.Init(); err != nil {
			b.Fatal(err)
		}
		sim.SetSize(100, 30)
		app, err := New(sim, configDir)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		app.handle(tcell.NewEventPaste(text, ""))
		app.draw()
		b.StopTimer()
		app.Close()
	}
}

func BenchmarkCachedWrapHeights10kLines(b *testing.B) {
	configDir := b.TempDir()
	sim := tcell.NewSimulationScreen("")
	if err := sim.Init(); err != nil {
		b.Fatal(err)
	}
	sim.SetSize(100, 30)
	app, err := New(sim, configDir)
	if err != nil {
		b.Fatal(err)
	}
	defer app.Close()
	lines := make([]string, 10_000)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%05d payload", i)
	}
	content := strings.Join(lines, "\n")
	app.left.buf.Insert(buffer.Loc{X: 0, Y: 0}, content)
	if err := app.left.buf.SetOptionNative("softwrap", true); err != nil {
		b.Fatal(err)
	}
	for line := range lines {
		_ = app.wrapHeight(&app.left, line)
	}
	b.Logf("environment go=%s os=%s arch=%s cpu=%d fixture=10k-lines-wrap-v1 lines=%d bytes=%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), len(lines), len(content))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for line := range lines {
			_ = app.wrapHeight(&app.left, line)
		}
	}
}
