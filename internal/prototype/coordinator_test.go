package prototype

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nevdelap/mdiff/internal/compare"
)

type manualClock struct{ timer *manualTimer }
type manualTimer struct {
	ch      chan time.Time
	delay   time.Duration
	stopped bool
	resetN  int
}

func (c *manualClock) NewTimer(delay time.Duration) diffTimer {
	c.timer = &manualTimer{ch: make(chan time.Time, 1), delay: delay}
	return c.timer
}
func (t *manualTimer) C() <-chan time.Time { return t.ch }
func (t *manualTimer) Stop() bool          { wasActive := !t.stopped; t.stopped = true; return wasActive }
func (t *manualTimer) Reset(delay time.Duration) bool {
	t.delay, t.stopped, t.resetN = delay, false, t.resetN+1
	return true
}
func (t *manualTimer) fire() { t.stopped = true; t.ch <- time.Now() }

func TestDebounceBoundsAndLargePasteThresholds(t *testing.T) {
	tests := []struct {
		name            string
		configured      time.Duration
		bytes, newlines int
		want            time.Duration
	}{
		{"configured minimum", 40 * time.Millisecond, 0, 0, 40 * time.Millisecond},
		{"default", 80 * time.Millisecond, 0, 0, 80 * time.Millisecond},
		{"configured maximum", 180 * time.Millisecond, 0, 0, 180 * time.Millisecond},
		{"just below byte threshold", 80 * time.Millisecond, largePasteBytes - 1, 0, 80 * time.Millisecond},
		{"just below newline threshold", 80 * time.Millisecond, 0, largePasteNewlines - 1, 80 * time.Millisecond},
		{"byte threshold", 80 * time.Millisecond, largePasteBytes, 0, 100 * time.Millisecond},
		{"newline threshold", 80 * time.Millisecond, 0, largePasteNewlines, 100 * time.Millisecond},
		{"paste delay cap", 180 * time.Millisecond, largePasteBytes, 0, 200 * time.Millisecond},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := debounceDelay(test.configured, test.bytes, test.newlines); got != test.want {
				t.Fatalf("delay = %s, want %s", got, test.want)
			}
		})
	}
}

func TestDebounceConfigurationAndCLIOverride(t *testing.T) {
	cli := 40
	tests := []struct {
		name     string
		settings map[string]any
		cli      *int
		want     time.Duration
		wantErr  bool
	}{
		{"default", nil, nil, 80 * time.Millisecond, false},
		{"namespaced setting", map[string]any{"mdiff": map[string]any{"diff-debounce-ms": float64(120)}}, nil, 120 * time.Millisecond, false},
		{"CLI takes precedence", map[string]any{"mdiff": map[string]any{"diff-debounce-ms": float64(39)}}, &cli, 40 * time.Millisecond, false},
		{"invalid setting rejected", map[string]any{"mdiff": map[string]any{"diff-debounce-ms": float64(39)}}, nil, 0, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveDebounce(test.settings, test.cli)
			if (err != nil) != test.wantErr {
				t.Fatalf("resolveDebounce() error = %v, wantErr=%t", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("resolveDebounce() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestCoordinatorResetsTimerAndCoalescesToNewestSnapshot(t *testing.T) {
	clock := &manualClock{}
	started := make(chan diffSnapshot, 2)
	release := make(chan struct{})
	var active, maxActive atomic.Int32
	c := newDiffCoordinator(80*time.Millisecond, clock, func(left, right []string) ([]compare.Pair, error) {
		current := active.Add(1)
		for old := maxActive.Load(); current > old && !maxActive.CompareAndSwap(old, current); old = maxActive.Load() {
		}
		started <- diffSnapshot{left: left, right: right}
		<-release
		active.Add(-1)
		return compare.Align(left, right), nil
	})
	defer c.Close()

	c.MarkDirty(DiffVersions{Left: 1}, 0, 0)
	if clock.timer.delay != 80*time.Millisecond {
		t.Fatalf("initial delay = %s", clock.timer.delay)
	}
	c.MarkDirty(DiffVersions{Left: 2}, 0, 0)
	if clock.timer.delay != 80*time.Millisecond || clock.timer.resetN != 2 {
		t.Fatalf("timer did not reset after edit: delay=%s reset=%d", clock.timer.delay, clock.timer.resetN)
	}
	clock.timer.fire()
	c.DebounceElapsed(diffSnapshot{left: []string{"v2"}, versions: DiffVersions{Left: 2}})
	if first := <-started; first.left[0] != "v2" {
		t.Fatalf("first job snapshot = %#v", first)
	}

	c.MarkDirty(DiffVersions{Left: 3}, 0, 0)
	clock.timer.fire()
	c.DebounceElapsed(diffSnapshot{left: []string{"v3"}, versions: DiffVersions{Left: 3}})
	c.MarkDirty(DiffVersions{Left: 4}, 0, 0)
	clock.timer.fire()
	c.DebounceElapsed(diffSnapshot{left: []string{"v4"}, versions: DiffVersions{Left: 4}})
	if !c.Stale() {
		t.Fatal("coordinator did not report pending/stale work")
	}
	release <- struct{}{}
	result := <-c.Results()
	if _, install, restart := c.Complete(result); install || restart {
		t.Fatal("obsolete result was installable")
	}
	if got := <-started; got.left[0] != "v4" {
		t.Fatalf("pending work was not coalesced to newest snapshot: %#v", got)
	}
	if maxActive.Load() != 1 {
		t.Fatalf("concurrent comparisons = %d, want 1", maxActive.Load())
	}
	release <- struct{}{}
	latest := <-c.Results()
	if _, install, restart := c.Complete(latest); !install || restart {
		t.Fatal("latest matching result was rejected")
	}
}

func TestLargePasteDelayAppliesOnlyToPasteMutation(t *testing.T) {
	clock := &manualClock{}
	c := newDiffCoordinator(80*time.Millisecond, clock, nil)
	defer c.Close()
	c.MarkDirty(DiffVersions{Left: 1}, largePasteBytes, 0)
	if clock.timer.delay != 100*time.Millisecond {
		t.Fatalf("large paste delay = %s", clock.timer.delay)
	}
	c.MarkDirty(DiffVersions{Left: 2}, 0, 0)
	if clock.timer.delay != 80*time.Millisecond {
		t.Fatalf("ordinary edit inherited paste delay: %s", clock.timer.delay)
	}
}

func TestCoordinatorReportsErrorAndClonesSnapshot(t *testing.T) {
	clock := &manualClock{}
	wantErr := errors.New("compare failed")
	c := newDiffCoordinator(40*time.Millisecond, clock, func(left, right []string) ([]compare.Pair, error) {
		return nil, wantErr
	})
	defer c.Close()
	left := []string{"before"}
	c.MarkDirty(DiffVersions{Left: 1}, 0, 0)
	clock.timer.fire()
	c.DebounceElapsed(diffSnapshot{left: left, versions: DiffVersions{Left: 1}})
	left[0] = "mutated after submission"
	result := <-c.Results()
	if result.snapshot.left[0] != "before" {
		t.Fatalf("worker snapshot aliases caller: %#v", result.snapshot.left)
	}
	completed, install, restart := c.Complete(result)
	if !install || restart || !errors.Is(completed.err, wantErr) {
		t.Fatalf("error result = %#v, install=%v", completed, install)
	}
}

func TestObsoleteCompletionImmediatelyStartsLatestDirtySnapshot(t *testing.T) {
	clock := &manualClock{}
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	started := make(chan string, 2)
	var calls atomic.Int32
	c := newDiffCoordinator(80*time.Millisecond, clock, func(left, right []string) ([]compare.Pair, error) {
		call := calls.Add(1)
		started <- left[0]
		if call == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		return compare.Align(left, right), nil
	})
	defer c.Close()

	c.MarkDirty(DiffVersions{Left: 1}, 0, 0)
	clock.timer.fire()
	c.DebounceElapsed(diffSnapshot{left: []string{"old"}, versions: DiffVersions{Left: 1}})
	<-firstStarted
	<-started
	c.MarkDirty(DiffVersions{Left: 2}, 0, 0) // Leave its debounce timer pending.
	close(releaseFirst)
	old := <-c.Results()
	if _, install, restart := c.Complete(old); install || !restart {
		t.Fatalf("obsolete completion = install %t, restart %t", install, restart)
	}
	c.DebounceElapsed(diffSnapshot{left: []string{"latest"}, versions: DiffVersions{Left: 2}})
	if got := <-started; got != "latest" {
		t.Fatalf("immediate replacement snapshot = %q", got)
	}
	latest := <-c.Results()
	if _, install, restart := c.Complete(latest); !install || restart {
		t.Fatal("latest immediate comparison was not installable")
	}
}

func TestNewEditInvalidatesCapturedPendingSnapshot(t *testing.T) {
	clock := &manualClock{}
	started := make(chan string, 3)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int32
	c := newDiffCoordinator(80*time.Millisecond, clock, func(left, right []string) ([]compare.Pair, error) {
		call := calls.Add(1)
		started <- left[0]
		if call == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		return compare.Align(left, right), nil
	})
	defer c.Close()

	c.MarkDirty(DiffVersions{Left: 1}, 0, 0)
	clock.timer.fire()
	c.DebounceElapsed(diffSnapshot{left: []string{"v1"}, versions: DiffVersions{Left: 1}})
	<-firstStarted
	if got := <-started; got != "v1" {
		t.Fatalf("first comparison = %q, want v1", got)
	}

	c.MarkDirty(DiffVersions{Left: 2}, 0, 0)
	clock.timer.fire()
	c.DebounceElapsed(diffSnapshot{left: []string{"v2"}, versions: DiffVersions{Left: 2}})
	c.MarkDirty(DiffVersions{Left: 3}, 0, 0) // Keep v3 inside its debounce window.

	close(releaseFirst)
	old := <-c.Results()
	if _, install, restart := c.Complete(old); install || !restart {
		t.Fatalf("obsolete completion = install %t, restart %t; want latest capture", install, restart)
	}

	// This is the app goroutine's immediate capture requested by Complete.
	c.DebounceElapsed(diffSnapshot{left: []string{"v3"}, versions: DiffVersions{Left: 3}})
	if got := <-started; got != "v3" {
		t.Fatalf("next comparison = %q, want v3 (v2 must never start)", got)
	}
	latest := <-c.Results()
	if _, install, restart := c.Complete(latest); !install || restart {
		t.Fatal("latest comparison was not installable")
	}
	select {
	case got := <-started:
		t.Fatalf("unexpected comparison started: %q", got)
	default:
	}
}

func TestCoordinatorCloseWaitsForActiveWorkerAndRejectsNewWork(t *testing.T) {
	clock := &manualClock{}
	started := make(chan struct{})
	release := make(chan struct{})
	c := newDiffCoordinator(80*time.Millisecond, clock, func(left, right []string) ([]compare.Pair, error) {
		close(started)
		<-release
		return compare.Align(left, right), nil
	})
	c.MarkDirty(DiffVersions{Left: 1}, 0, 0)
	clock.timer.fire()
	c.DebounceElapsed(diffSnapshot{left: []string{"a"}, versions: DiffVersions{Left: 1}})
	<-started
	closed := make(chan struct{})
	go func() { c.Close(); close(closed) }()
	close(release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not join the comparison worker")
	}
	c.MarkDirty(DiffVersions{Left: 2}, 0, 0)
	if c.Stale() {
		t.Fatal("closed coordinator accepted new work")
	}
}
