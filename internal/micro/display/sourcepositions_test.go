package display

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/nevdelap/mdiff/internal/micro/buffer"
	"github.com/nevdelap/mdiff/internal/micro/config"
	ulua "github.com/nevdelap/mdiff/internal/micro/lua"
	lua "github.com/yuin/gopher-lua"
)

func init() {
	ulua.L = lua.NewState()
	config.InitRuntimeFiles(false)
	config.InitGlobalSettings()
}

func sourcePositionWindow(text string, width int, softwrap, wordwrap bool) *BufWindow {
	b := buffer.NewBufferFromString(text, "", buffer.BTDefault)
	b.Settings["softwrap"] = softwrap
	b.Settings["wordwrap"] = wordwrap
	b.Settings["tabsize"] = float64(4)
	return &BufWindow{Buf: b, bufWidth: width}
}

func TestRuneBoundariesVisualPositions(t *testing.T) {
	for _, tc := range []struct {
		name, line         string
		width              int
		softwrap, wordwrap bool
		want               []VLoc
		characters         []int
	}{
		{"empty", "", 4, true, true, []VLoc{{}}, []int{0}},
		{"unwrapped tab and wide", "a\t猫", 4, false, false, []VLoc{{}, {VisualX: 1}, {VisualX: 4}, {VisualX: 6}}, []int{0, 1, 2, 3}},
		{"wrapped tab and wide", "a\t猫", 4, true, false, []VLoc{{}, {VisualX: 1}, {SLoc: SLoc{Row: 1}}, {SLoc: SLoc{Row: 1}, VisualX: 2}}, []int{0, 1, 2, 3}},
		{"word moves boundary", "ab cd", 4, true, true, []VLoc{{}, {VisualX: 1}, {VisualX: 2}, {SLoc: SLoc{Row: 1}}, {SLoc: SLoc{Row: 1}, VisualX: 1}, {SLoc: SLoc{Row: 1}, VisualX: 2}}, []int{0, 1, 2, 3, 4, 5}},
		{"marks at wrap", "ab e\u0301\u0327", 4, true, false, []VLoc{{}, {VisualX: 1}, {VisualX: 2}, {VisualX: 3}, {SLoc: SLoc{Row: 1}}, {SLoc: SLoc{Row: 1}}, {SLoc: SLoc{Row: 1}}}, []int{0, 1, 2, 3, 4, 4, 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := sourcePositionWindow(tc.line, tc.width, tc.softwrap, tc.wordwrap)
			defer w.Buf.Close()
			got := w.RuneBoundaries(0)
			if !reflect.DeepEqual(got.Visual, tc.want) || !reflect.DeepEqual(got.Character, tc.characters) {
				t.Fatalf("boundaries = %#v, characters = %v; want %#v, %v", got.Visual, got.Character, tc.want, tc.characters)
			}
			for i, character := range got.Character {
				if loc := w.VLocFromLoc(buffer.Loc{X: character}); loc != got.Visual[i] {
					t.Fatalf("rune %d, character %d: lookup = %#v, batch = %#v", i, character, loc, got.Visual[i])
				}
			}
		})
	}
}

func TestNegativeMicroLocationsRemainAtLineStart(t *testing.T) {
	for _, softwrap := range []bool{false, true} {
		w := sourcePositionWindow("abcdefgh", 3, softwrap, false)
		got := w.VLocFromLoc(buffer.Loc{X: -1})
		if want := (VLoc{}); got != want {
			t.Errorf("softwrap=%t: negative location = %#v, want line start %#v", softwrap, got, want)
		}
		if got := w.SLocFromLoc(buffer.Loc{X: -1}); got != (SLoc{}) {
			t.Errorf("softwrap=%t: negative scroll location = %#v, want line start", softwrap, got)
		}
		w.Buf.Close()
	}
}

func TestVisibleRuneBoundariesLimitUnwrappedMap(t *testing.T) {
	w := sourcePositionWindow("abc\t猫e\u0301xyz", 3, false, false)
	defer w.Buf.Close()
	got := w.VisibleRuneBoundaries(0, 4, 3)
	if got.RuneOffset != 4 || !reflect.DeepEqual(got.Runes, []rune("猫e\u0301")) ||
		!reflect.DeepEqual(got.Character, []int{4, 5, 6, 6}) ||
		!reflect.DeepEqual(got.Visual, []VLoc{{VisualX: 4}, {VisualX: 6}, {VisualX: 7}, {VisualX: 7}}) {
		t.Fatalf("visible map = %#v, want rune offset 4 and only the visible wide/combining characters", got)
	}
}

var sourcePositionSink VLoc
var sourceBoundarySink RuneBoundaryMap

func benchmarkSourceWindow(repetitions int) (*BufWindow, int) {
	line := strings.Repeat("word\t界e\u0301 ", repetitions)
	return sourcePositionWindow(line, 40, true, true), len(line)
}

func BenchmarkRuneBoundariesLongLine(b *testing.B) {
	for _, repetitions := range []int{512, 1024, 2048} {
		b.Run(fmt.Sprintf("repetitions=%d", repetitions), func(b *testing.B) {
			w, bytes := benchmarkSourceWindow(repetitions)
			defer w.Buf.Close()
			b.SetBytes(int64(bytes))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				sourceBoundarySink = w.RuneBoundaries(0)
			}
		})
	}
}

func BenchmarkCursorPositionLongLine(b *testing.B) {
	for _, repetitions := range []int{512, 1024, 2048} {
		b.Run(fmt.Sprintf("repetitions=%d", repetitions), func(b *testing.B) {
			w, bytes := benchmarkSourceWindow(repetitions)
			defer w.Buf.Close()
			b.SetBytes(int64(bytes))
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				// Clear bounded cache to measure a real scan at each offset.
				clear(w.vlocCache)
				x := 1 + (i%4)*(repetitions*8/4)
				sourcePositionSink = w.VLocFromLoc(buffer.Loc{X: x})
			}
		})
	}
}
