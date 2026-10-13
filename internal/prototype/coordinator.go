package prototype

import (
	"sync"
	"time"

	"github.com/nevdelap/mdiff/internal/compare"
	"github.com/nevdelap/mdiff/internal/viewport"
)

const (
	defaultDiffDebounce = 80 * time.Millisecond
	minDiffDebounce     = 40 * time.Millisecond
	maxDiffDebounce     = 180 * time.Millisecond
	largePasteBytes     = 256 * 1024
	largePasteNewlines  = 10_000
)

// DiffVersions identifies the immutable pair of buffer states used for a diff.
type DiffVersions struct{ Left, Right uint64 }

type diffSnapshot struct {
	left, right []string
	versions    DiffVersions
}

type diffResult struct {
	snapshot diffSnapshot
	pairs    []compare.Pair
	coarse   bool
	err      error
}

type diffTimer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(time.Duration) bool
}

type diffClock interface{ NewTimer(time.Duration) diffTimer }

type realDiffClock struct{}
type realDiffTimer struct{ timer *time.Timer }

func (realDiffClock) NewTimer(d time.Duration) diffTimer { return realDiffTimer{time.NewTimer(d)} }
func (t realDiffTimer) C() <-chan time.Time              { return t.timer.C }
func (t realDiffTimer) Stop() bool                       { return t.timer.Stop() }
func (t realDiffTimer) Reset(d time.Duration) bool       { return t.timer.Reset(d) }

type diffCompare func([]string, []string) ([]compare.Pair, error)

// diffCoordinator is owned by the app goroutine. Its only goroutine reads
// immutable snapshots and publishes a result; it never touches Micro buffers.
type diffCoordinator struct {
	clock      diffClock
	compare    diffCompare
	baseDelay  time.Duration
	timer      diffTimer
	results    chan diffResult
	mu         sync.Mutex
	closed     bool
	active     bool
	activeDone chan struct{}
	pending    *diffSnapshot
	latest     DiffVersions
	dirty      bool
}

func newDiffCoordinator(delay time.Duration, clock diffClock, compareFn diffCompare) *diffCoordinator {
	if delay < minDiffDebounce || delay > maxDiffDebounce {
		delay = defaultDiffDebounce
	}
	if clock == nil {
		clock = realDiffClock{}
	}
	return &diffCoordinator{
		clock: clock, compare: compareFn, baseDelay: delay,
		timer: clock.NewTimer(time.Hour), results: make(chan diffResult, 1),
	}
}

func debounceDelay(configured time.Duration, pasteBytes, pasteNewlines int) time.Duration {
	if configured < minDiffDebounce || configured > maxDiffDebounce {
		configured = defaultDiffDebounce
	}
	if pasteBytes >= largePasteBytes || pasteNewlines >= largePasteNewlines {
		return min(configured+20*time.Millisecond, 200*time.Millisecond)
	}
	return configured
}

func (c *diffCoordinator) MarkDirty(versions DiffVersions, pasteBytes, pasteNewlines int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.latest = versions
	c.dirty = true
	// A pending snapshot was captured for an earlier debounce window. A new
	// edit makes it obsolete; if the active comparison finishes before this
	// edit's debounce expires, Complete will ask the app goroutine to capture
	// the latest buffers immediately.
	c.pending = nil
	if c.timer == nil {
		c.timer = c.clock.NewTimer(debounceDelay(c.baseDelay, pasteBytes, pasteNewlines))
		return
	}
	if !c.timer.Stop() {
		select {
		case <-c.timer.C():
		default:
		}
	}
	c.timer.Reset(debounceDelay(c.baseDelay, pasteBytes, pasteNewlines))
}

// AcceptCurrent resets debounce state after an explicit synchronous rebuild.
func (c *diffCoordinator) AcceptCurrent(versions DiffVersions) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latest = versions
	c.dirty = false
	c.pending = nil
	if c.timer != nil {
		if !c.timer.Stop() {
			select {
			case <-c.timer.C():
			default:
			}
		}
	}
}

func (c *diffCoordinator) TimerC() <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.dirty || c.timer == nil {
		return nil
	}
	return c.timer.C()
}

// DebounceElapsed captures must be made by the caller on the app goroutine.
func (c *diffCoordinator) DebounceElapsed(snapshot diffSnapshot) {
	c.DebounceElapsedOwned(cloneSnapshot(snapshot))
}

// DebounceElapsedOwned transfers the caller's detached snapshot slices to the
// coordinator. The caller must not mutate them after this call.
func (c *diffCoordinator) DebounceElapsedOwned(snapshot diffSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.dirty {
		return
	}
	c.dirty = false
	if c.active {
		c.pending = &snapshot
		return
	}
	c.startLocked(snapshot)
}

func (c *diffCoordinator) startLocked(snapshot diffSnapshot) {
	c.active = true
	done := make(chan struct{})
	c.activeDone = done
	go func() {
		var result diffResult
		result.snapshot = snapshot
		if c.compare == nil {
			result.pairs, result.coarse = compare.AlignWithBudget(snapshot.left, snapshot.right, compare.DefaultBudget)
		} else {
			result.pairs, result.err = c.compare(snapshot.left, snapshot.right)
		}
		c.results <- result
		close(done)
	}()
}

func (c *diffCoordinator) Results() <-chan diffResult { return c.results }

// Complete returns an installable result only when it still matches the newest
// known buffer versions. It starts the pending snapshot only if it still
// matches those versions; otherwise the caller can capture the latest buffers.
func (c *diffCoordinator) Complete(result diffResult) (diffResult, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = false
	c.activeDone = nil
	install := !c.closed && result.snapshot.versions == c.latest
	if c.pending != nil && !c.closed {
		next := *c.pending
		c.pending = nil
		if next.versions == c.latest {
			c.startLocked(next)
		}
	}
	restartNow := !install && c.pending == nil && c.dirty && !c.active && !c.closed
	return result, install, restartNow
}

func (c *diffCoordinator) Stale() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && (c.dirty || c.active || c.pending != nil)
}

func (c *diffCoordinator) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.dirty = false
	c.pending = nil
	if c.timer != nil {
		c.timer.Stop()
	}
	done := c.activeDone
	c.mu.Unlock()
	if done != nil {
		<-done
	}
}

func cloneSnapshot(snapshot diffSnapshot) diffSnapshot {
	return diffSnapshot{
		left: append([]string(nil), snapshot.left...), right: append([]string(nil), snapshot.right...),
		versions: snapshot.versions,
	}
}

func layoutFor(pairs []compare.Pair, leftHeight, rightHeight func(int) int) viewport.Layout {
	return viewport.Expand(pairs, leftHeight, rightHeight)
}
