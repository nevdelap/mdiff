package prototype

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestPTYPasteScrollResizeAndTerminalRestore(t *testing.T) {
	binary := buildPTYBinary(t)
	configDir := t.TempDir()
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"MICRO_CONFIG_HOME="+configDir,
		"MICRO_TRUECOLOR=0",
	)
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 8, Cols: 42})
	if err != nil {
		t.Fatal(err)
	}
	transcript := capturePTY(t, terminal)
	defer func() { _ = terminal.Close() }()
	transcript.waitFor(t, []byte("^Q Quit"))
	transcript.waitForCursorInColumnSince(t, 0, 1, 21)
	left := numberedLines("L", 24)
	right := numberedLines("R", 24)
	right = "INSERT\n" + right
	writePTY(t, terminal, []byte("\x1b[200~"+left+"\x1b[201~"))
	transcript.waitFor(t, []byte("L23"))
	focusRightStart := transcript.mark()
	writePTY(t, terminal, []byte{0x17}) // Ctrl+W switches panes.
	transcript.waitForCursorInColumnSince(t, focusRightStart, 22, 42)
	writePTY(t, terminal, []byte("\x1b[200~"+right+"\x1b[201~"))
	transcript.waitFor(t, []byte("R23"))

	// Scroll upward from the left pane until the first left line is visible.
	focusLeftStart := transcript.mark()
	writePTY(t, terminal, []byte{0x17}) // Ctrl-W is the reliable focus-switch fallback.
	transcript.waitForCursorInColumnSince(t, focusLeftStart, 1, 21)
	leftScrollStart := transcript.mark()
	writePTY(t, terminal, bytes.Repeat([]byte("\x1b[1;5A"), 24))
	transcript.waitForScreenSince(t, leftScrollStart, "L00")

	// Focus the right pane, move its cursor to the beginning, then scroll down
	// from that pane. Each checkpoint waits for the corresponding content to be
	// painted instead of relying on a scheduler-dependent delay.
	focusRightStart = transcript.mark()
	writePTY(t, terminal, []byte{0x17}) // Ctrl+W switches panes.
	transcript.waitForCursorInColumnSince(t, focusRightStart, 22, 42)
	rightHomeStart := transcript.mark()
	writePTY(t, terminal, []byte("\x1b[1;5H")) // Ctrl-Home.
	transcript.waitForScreenSince(t, rightHomeStart, "INSERT")
	rightScrollStart := transcript.mark()
	writePTY(t, terminal, bytes.Repeat([]byte("\x1b[1;5B"), 24))
	transcript.waitForScreenSince(t, rightScrollStart, "R23")
	rightCursorStart := transcript.mark()
	writePTY(t, terminal, bytes.Repeat([]byte("\x1b[B"), 5))
	transcript.waitForScreenSince(t, rightCursorStart, "R04")

	resizeStart := transcript.mark()
	if err := pty.Setsize(terminal, &pty.Winsize{Rows: 12, Cols: 60}); err != nil {
		t.Fatal(err)
	}
	resized := transcript.waitForScreenSince(t, resizeStart, "R04")
	if !hasVisibleCursorWithin(resized[resizeStart:], 12, 60) {
		t.Fatalf("resize redraw did not place a visible cursor inside the new terminal bounds; cursor sequences=%q output tail=%q", cursorPositionBeforeShow.FindAllSubmatch(resized[resizeStart:], -1), tail(resized[resizeStart:], 500))
	}

	writePTY(t, terminal, []byte{0x11}) // Ctrl-Q.
	waitProcess(t, cmd)
	transcript.waitFor(t, []byte("\x1b[?1049l"))
	assertConfigDirEmpty(t, configDir)
}

func TestPTYInterruptRestoresTerminalWithoutBufferState(t *testing.T) {
	binary := buildPTYBinary(t)
	configDir := t.TempDir()
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "MICRO_CONFIG_HOME="+configDir)
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 10, Cols: 40})
	if err != nil {
		t.Fatal(err)
	}
	transcript := capturePTY(t, terminal)
	transcript.waitFor(t, []byte("^Q Quit"))
	writePTY(t, terminal, []byte("\x1b[200~ABORT-MARKER\x1b[201~"))
	transcript.waitFor(t, []byte("ABORT-MARKER"))
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	waitProcess(t, cmd)
	if err := terminal.Close(); err != nil {
		t.Fatal(err)
	}
	transcript.waitFor(t, []byte("\x1b[?1049l"))
	assertConfigDirEmpty(t, configDir)
}

type ptyTranscript struct {
	mu      sync.Mutex
	data    []byte
	changed chan struct{}
	done    chan struct{}
}

func capturePTY(t *testing.T, terminal *os.File) *ptyTranscript {
	t.Helper()
	capture := &ptyTranscript{changed: make(chan struct{}, 1), done: make(chan struct{})}
	go func() {
		defer close(capture.done)
		chunk := make([]byte, 4096)
		for {
			n, err := terminal.Read(chunk)
			if n > 0 {
				capture.mu.Lock()
				capture.data = append(capture.data, chunk[:n]...)
				capture.mu.Unlock()
				select {
				case capture.changed <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return capture
}

func (p *ptyTranscript) mark() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.data)
}

func (p *ptyTranscript) waitFor(t *testing.T, marker []byte) []byte {
	t.Helper()
	return p.waitForSince(t, 0, marker)
}

func (p *ptyTranscript) waitForSince(t *testing.T, start int, marker []byte) []byte {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		p.mu.Lock()
		data := append([]byte(nil), p.data...)
		p.mu.Unlock()
		if start <= len(data) && bytes.Contains(data[start:], marker) {
			return data
		}
		select {
		case <-p.changed:
		case <-p.done:
			t.Fatalf("PTY closed before output contained %q", marker)
		case <-timer.C:
			t.Fatalf("timed out waiting for PTY output %q; terminal output tail: %q", marker, tail(data, 500))
		}
	}
}

func (p *ptyTranscript) waitForCursorInColumnSince(t *testing.T, start, minCol, maxCol int) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		p.mu.Lock()
		data := append([]byte(nil), p.data...)
		p.mu.Unlock()
		if start <= len(data) && hasVisibleCursorInColumnRange(data[start:], minCol, maxCol) {
			return
		}
		select {
		case <-p.changed:
		case <-p.done:
			t.Fatalf("PTY closed before cursor appeared in columns %d..%d", minCol, maxCol)
		case <-timer.C:
			t.Fatalf("timed out waiting for cursor in columns %d..%d; terminal output tail: %q", minCol, maxCol, tail(data, 500))
		}
	}
}

func (p *ptyTranscript) waitForScreenSince(t *testing.T, start int, marker string) []byte {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		p.mu.Lock()
		data := append([]byte(nil), p.data...)
		p.mu.Unlock()
		if start < len(data) && screenContainsSince(data, start, marker) {
			return data
		}
		select {
		case <-p.changed:
		case <-p.done:
			t.Fatalf("PTY closed before screen displayed %q", marker)
		case <-timer.C:
			t.Fatalf("timed out waiting for terminal screen to display %q; output tail: %q", marker, tail(data, 500))
		}
	}
}

func tail(data []byte, length int) []byte {
	if len(data) > length {
		data = data[len(data)-length:]
	}
	return data
}

func screenContainsSince(data []byte, start int, marker string) bool {
	screen := newTestTerminal()
	screen.apply(data[:start])
	screen.apply(data[start:])
	return strings.Contains(screen.text(), marker)
}

type testTerminal struct {
	cells [][]byte
	x, y  int
}

func newTestTerminal() *testTerminal {
	terminal := &testTerminal{cells: make([][]byte, 100)}
	for y := range terminal.cells {
		terminal.cells[y] = bytes.Repeat([]byte{' '}, 200)
	}
	return terminal
}

func (s *testTerminal) apply(data []byte) {
	for i := 0; i < len(data); {
		b := data[i]
		switch {
		case b == 0x1b && i+1 < len(data) && data[i+1] == '[':
			j := i + 2
			for j < len(data) && (data[j] < 0x40 || data[j] > 0x7e) {
				j++
			}
			if j >= len(data) {
				return
			}
			s.applyCSI(string(data[i+2:j]), data[j])
			i = j + 1
		case b == 0x1b && i+2 < len(data) && data[i+1] == '(':
			i += 3 // Character-set selection.
		case b == '\r':
			s.x = 0
			i++
		case b == '\n':
			s.y = min(s.y+1, len(s.cells)-1)
			i++
		case b >= 0x20 && b < 0x7f:
			if s.y >= 0 && s.y < len(s.cells) && s.x >= 0 && s.x < len(s.cells[s.y]) {
				s.cells[s.y][s.x] = b
			}
			s.x = min(s.x+1, len(s.cells[0])-1)
			i++
		default:
			i++
		}
	}
}

func (s *testTerminal) applyCSI(params string, final byte) {
	values := strings.TrimLeft(params, "? ")
	parts := strings.Split(values, ";")
	param := func(index, fallback int) int {
		if index >= len(parts) || parts[index] == "" {
			return fallback
		}
		value, err := strconv.Atoi(parts[index])
		if err != nil {
			return fallback
		}
		return value
	}
	switch final {
	case 'H', 'f':
		s.y = min(max(0, param(0, 1)-1), len(s.cells)-1)
		s.x = min(max(0, param(1, 1)-1), len(s.cells[0])-1)
	case 'J':
		if param(0, 0) == 2 {
			for y := range s.cells {
				clear(s.cells[y])
			}
		}
	case 'K':
		if s.y >= 0 && s.y < len(s.cells) {
			clear(s.cells[s.y][s.x:])
		}
	}
}

func (s *testTerminal) text() string {
	rows := make([]string, len(s.cells))
	for i := range s.cells {
		rows[i] = string(s.cells[i])
	}
	return strings.Join(rows, "\n")
}

func writePTY(t *testing.T, terminal *os.File, input []byte) {
	t.Helper()
	if _, err := terminal.Write(input); err != nil {
		t.Fatalf("write PTY input: %v", err)
	}
}

func numberedLines(prefix string, count int) string {
	var lines string
	for i := 0; i < count; i++ {
		lines += fmt.Sprintf("%s%02d\n", prefix, i)
	}
	return lines
}

var cursorPositionBeforeShow = regexp.MustCompile(`\x1b\[([0-9]+);([0-9]+)H(\x1b\[[0-9;?]*[A-Za-z])*\x1b\[\?25h`)

func hasVisibleCursorWithin(transcript []byte, rows, cols int) bool {
	for _, match := range cursorPositionBeforeShow.FindAllSubmatch(transcript, -1) {
		row, _ := strconv.Atoi(string(match[1]))
		col, _ := strconv.Atoi(string(match[2]))
		if row >= 1 && row <= rows && col >= 1 && col <= cols {
			return true
		}
	}
	return false
}

func hasVisibleCursorInColumnRange(transcript []byte, minCol, maxCol int) bool {
	for _, match := range cursorPositionBeforeShow.FindAllSubmatch(transcript, -1) {
		col, _ := strconv.Atoi(string(match[2]))
		if col >= minCol && col <= maxCol {
			return true
		}
	}
	return false
}

func assertConfigDirEmpty(t *testing.T, configDir string) {
	t.Helper()
	entries, err := os.ReadDir(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("editor wrote config or buffer state: %v", entries)
	}
}

func buildPTYBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTY prototype coverage currently targets Unix terminals")
	}
	path := filepath.Join(t.TempDir(), "mdiff")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", path, "../../cmd/mdiff")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build PTY command: %v\n%s", err, out)
	}
	return path
}

func waitProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("mdiff exited with error: %v", err)
		}
	case <-time.After(8 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for mdiff")
	}
}
