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
	preparePTYConfig(t, configDir)
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
	if transcript.mouseModeRequested() {
		mouseStart := transcript.mark()
		writePTY(t, terminal, []byte{0x1b, '[', 'M', 96, 55, 34}) // X11 wheel-up over the right pane.
		transcript.waitForScreenSince(t, mouseStart, "L20")
	} else {
		t.Log("PTY terminfo did not enable mouse reporting; simulation tests cover wheel routing")
	}

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
	// R04 was already visible before the resize. Wait for the menu to move
	// to its new row and the cursor to return to the resized right pane.
	transcript.waitForResizeRedraw(t, resizeStart, 10, 1, 8, 31, 60)

	quitStart := transcript.mark()
	writePTY(t, terminal, []byte{0x11}) // Ctrl-Q prompts for each modified unnamed buffer.
	transcript.waitForScreenSince(t, quitStart, "Save changes to No name (left pane)")
	writePTY(t, terminal, []byte{'n'})
	secondQuitPrompt := transcript.mark()
	transcript.waitForScreenSince(t, secondQuitPrompt, "Save changes to No name (right pane)")
	writePTY(t, terminal, []byte{'n'}) // Discard both and exit.
	waitProcess(t, cmd)
	transcript.waitFor(t, []byte("\x1b[?1049l"))
	assertNoUnexpectedPersistedState(t, configDir)
}

func TestPTYInterruptRestoresTerminalWithoutBufferState(t *testing.T) {
	binary := buildPTYBinary(t)
	configDir := t.TempDir()
	preparePTYConfig(t, configDir)
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
	assertNoUnexpectedPersistedState(t, configDir)
}

func TestCLIRejectsMoreThanTwoPathsBeforeTerminalInitialization(t *testing.T) {
	binary := buildPTYBinary(t)
	configDir := t.TempDir()
	cmd := exec.Command(binary, "left.txt", "right.txt", "extra.txt")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "MICRO_CONFIG_HOME="+configDir)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("CLI accepted three paths: %q", output)
	}
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 2 {
		t.Fatalf("three-path exit = %v, want status 2", err)
	}
	if !bytes.Contains(output, []byte("at most two file paths")) || bytes.Contains(output, []byte("\x1b[")) {
		t.Fatalf("three-path validation did not exit before terminal startup: %q", output)
	}
	entries, err := os.ReadDir(configDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("three-path rejection initialized Micro or wrote config: entries=%v err=%v", entries, err)
	}
}

func TestPTYFileArgumentsOpenSaveAndOpenOnlyFocusedPane(t *testing.T) {
	binary := buildPTYBinary(t)
	configDir := t.TempDir()
	preparePTYConfig(t, configDir)
	filesDir := t.TempDir()
	leftPath := filepath.Join(filesDir, "left.txt")
	rightPath := filepath.Join(filesDir, "right.txt")
	openPath := filepath.Join(filesDir, "opened.txt")
	for path, contents := range map[string]string{
		leftPath:  "LEFT-ORIGINAL",
		rightPath: "RIGHT-ORIGINAL",
		openPath:  "OPENED-MARKER",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(binary, leftPath, rightPath)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"MICRO_CONFIG_HOME="+configDir,
		"MICRO_TRUECOLOR=0",
	)
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 10, Cols: 60})
	if err != nil {
		t.Fatal(err)
	}
	transcript := capturePTY(t, terminal)
	defer func() { _ = terminal.Close() }()
	transcript.waitFor(t, []byte("^Q Quit"))
	transcript.waitForScreenSince(t, 0, "LEFT-ORIGINAL")
	transcript.waitForScreenSince(t, 0, "RIGHT-ORIGINAL")

	writePTY(t, terminal, []byte{0x01}) // Ctrl+A selects the focused buffer.
	writePTY(t, terminal, []byte("\x1b[200~LEFT-SAVED\x1b[201~"))
	writePTY(t, terminal, []byte{0x13}) // Ctrl+S saves only the left pane.
	waitForFileContents(t, leftPath, "LEFT-SAVED\n")
	if got, err := os.ReadFile(rightPath); err != nil || string(got) != "RIGHT-ORIGINAL" {
		t.Fatalf("saving left pane changed right file: contents=%q err=%v", got, err)
	}

	openStart := transcript.mark()
	writePTY(t, terminal, []byte{0x0f}) // Ctrl+O opens into the focused pane.
	writePTY(t, terminal, []byte(openPath))
	writePTY(t, terminal, []byte{0x0d})
	transcript.waitForScreenSince(t, openStart, "OPENED-MARKER")
	writePTY(t, terminal, []byte{0x11}) // Ctrl+Q.
	waitProcess(t, cmd)
	transcript.waitFor(t, []byte("\x1b[?1049l"))
	assertNoUnexpectedPersistedState(t, configDir)
}

func TestPTYQuitPromptsForBothNamedPanesAndHonorsChoices(t *testing.T) {
	binary := buildPTYBinary(t)
	configDir := t.TempDir()
	preparePTYConfig(t, configDir)
	leftPath, rightPath := writeWorkflowFiles(t, "left.txt", "LEFT-OLD\n", "right.txt", "RIGHT-OLD\n")
	cmd := exec.Command(binary, leftPath, rightPath)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "MICRO_CONFIG_HOME="+configDir, "MICRO_TRUECOLOR=0")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 12, Cols: 120})
	if err != nil {
		t.Fatal(err)
	}
	transcript := capturePTY(t, terminal)
	defer func() { _ = terminal.Close() }()
	transcript.waitFor(t, []byte("^Q Quit"))

	writePTY(t, terminal, []byte{0x01}) // Ctrl+A selects all in the left pane.
	writePTY(t, terminal, []byte("\x1b[200~LEFT-NEW\n\x1b[201~"))
	writePTY(t, terminal, []byte{0x17}) // Ctrl+W switches to the right pane.
	writePTY(t, terminal, []byte{0x01})
	writePTY(t, terminal, []byte("\x1b[200~RIGHT-NEW\n\x1b[201~"))
	writePTY(t, terminal, []byte{0x17}) // Make left the first quit decision.
	firstPrompt := transcript.mark()
	writePTY(t, terminal, []byte{0x11}) // Ctrl+Q.
	transcript.waitForScreenSince(t, firstPrompt, "before")
	writePTY(t, terminal, []byte{'y'}) // Save left.
	secondPrompt := transcript.mark()
	transcript.waitForScreenSince(t, secondPrompt, "before")
	writePTY(t, terminal, []byte{'n'}) // Discard right.
	waitProcess(t, cmd)
	transcript.waitFor(t, []byte("\x1b[?1049l"))
	if got := readWorkflowFile(t, leftPath); got != "LEFT-NEW\n" {
		t.Fatalf("left file after prompted quit=%q", got)
	}
	if got := readWorkflowFile(t, rightPath); got != "RIGHT-OLD\n" {
		t.Fatalf("discarded right file changed: %q", got)
	}
	assertNoUnexpectedPersistedState(t, configDir)
}

func TestPTYQuitCancelPreservesDirtyFile(t *testing.T) {
	binary := buildPTYBinary(t)
	configDir := t.TempDir()
	preparePTYConfig(t, configDir)
	leftPath := filepath.Join(t.TempDir(), "left.txt")
	if err := os.WriteFile(leftPath, []byte("LEFT-OLD\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, leftPath)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "MICRO_CONFIG_HOME="+configDir, "MICRO_TRUECOLOR=0")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 12, Cols: 120})
	if err != nil {
		t.Fatal(err)
	}
	transcript := capturePTY(t, terminal)
	defer func() { _ = terminal.Close() }()
	transcript.waitFor(t, []byte("^Q Quit"))
	writePTY(t, terminal, []byte{0x01})
	writePTY(t, terminal, []byte("\x1b[200~LEFT-NEW\n\x1b[201~"))

	firstPrompt := transcript.mark()
	writePTY(t, terminal, []byte{0x11})
	transcript.waitForScreenSince(t, firstPrompt, "before")
	writePTY(t, terminal, []byte{0x1b}) // Cancel the dirty-buffer quit prompt.

	firstPrompt = transcript.mark()
	writePTY(t, terminal, []byte{0x11})
	transcript.waitForScreenSince(t, firstPrompt, "before")
	writePTY(t, terminal, []byte{'n'})
	waitProcess(t, cmd)
	transcript.waitFor(t, []byte("\x1b[?1049l"))
	if got := readWorkflowFile(t, leftPath); got != "LEFT-OLD\n" {
		t.Fatalf("cancelled quit changed left file: %q", got)
	}
	assertNoUnexpectedPersistedState(t, configDir)
}

func TestPTYCtrlQCancelsPromptAndUnnamedQuitCanSave(t *testing.T) {
	binary := buildPTYBinary(t)
	configDir := t.TempDir()
	preparePTYConfig(t, configDir)
	savePath := filepath.Join(t.TempDir(), "unnamed-save.txt")
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "MICRO_CONFIG_HOME="+configDir, "MICRO_TRUECOLOR=0")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 12, Cols: 64})
	if err != nil {
		t.Fatal(err)
	}
	transcript := capturePTY(t, terminal)
	defer func() { _ = terminal.Close() }()
	transcript.waitFor(t, []byte("^Q Quit"))
	writePTY(t, terminal, []byte("\x1b[200~UNNAMED-EDIT\n\x1b[201~"))
	writePTY(t, terminal, []byte{0x11}) // Ctrl+Q asks about the unnamed buffer.
	transcript.waitForScreenSince(t, 0, "Save changes to No name (left pane)")
	writePTY(t, terminal, []byte{0x11}) // Micro's command prompt binding cancels.
	secondPrompt := transcript.mark()
	writePTY(t, terminal, []byte{0x11})
	transcript.waitForScreenSince(t, secondPrompt, "Save changes to No name (left pane)")
	writePTY(t, terminal, []byte{'y'})
	transcript.waitForScreenSince(t, 0, "Filename:")
	writePTY(t, terminal, []byte(savePath))
	writePTY(t, terminal, []byte{0x0d})
	waitProcess(t, cmd)
	transcript.waitFor(t, []byte("\x1b[?1049l"))
	if got := readWorkflowFile(t, savePath); got != "UNNAMED-EDIT\n" {
		t.Fatalf("saved unnamed buffer=%q", got)
	}
	assertNoUnexpectedPersistedState(t, configDir)
}

func waitForFileContents(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(path)
		if err == nil && string(contents) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	contents, err := os.ReadFile(path)
	t.Fatalf("file %q contents=%q err=%v, want %q", path, contents, err, want)
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

func (p *ptyTranscript) mouseModeRequested() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return bytes.Contains(p.data, []byte("?1000h")) || bytes.Contains(p.data, []byte("?1002h")) || bytes.Contains(p.data, []byte("?1006h"))
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

func (p *ptyTranscript) waitForResizeRedraw(t *testing.T, start, menuRow, minRow, maxRow, minCol, maxCol int) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		p.mu.Lock()
		data := append([]byte(nil), p.data...)
		p.mu.Unlock()
		if start <= len(data) && screenContainsAtRow(data, menuRow, "^Q Quit") && lastCursorPositionWithin(data[start:], minRow, maxRow, minCol, maxCol) && cursorRemainsVisible(data) {
			return
		}
		select {
		case <-p.changed:
		case <-p.done:
			t.Fatalf("PTY closed before resize redrew the menu and placed a visible cursor in rows %d..%d, columns %d..%d", minRow, maxRow, minCol, maxCol)
		case <-timer.C:
			t.Fatalf("timed out waiting for resize redraw and cursor in rows %d..%d, columns %d..%d; cursor positions=%q output tail=%q", minRow, maxRow, minCol, maxCol, cursorPosition.FindAllSubmatch(data[start:], -1), tail(data[start:], 500))
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
		if start < len(data) && screenContains(data, marker) {
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

func screenContains(data []byte, marker string) bool {
	screen := newTestTerminal()
	// A checkpoint may fall inside an escape sequence. Decode the transcript
	// as one stream so the screen state is independent of that byte offset.
	screen.apply(data)
	return strings.Contains(screen.text(), marker)
}

func screenContainsAtRow(data []byte, row int, marker string) bool {
	screen := newTestTerminal()
	screen.apply(data)
	return row >= 1 && row <= len(screen.cells) && strings.Contains(string(screen.cells[row-1]), marker)
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

var cursorPosition = regexp.MustCompile(`\x1b\[([0-9]+);([0-9]+)H`)
var cursorPositionBeforeShow = regexp.MustCompile(`\x1b\[([0-9]+);([0-9]+)H(\x1b\[[0-9;?]*[A-Za-z])*\x1b\[\?25h`)

func lastCursorPositionWithin(transcript []byte, minRow, maxRow, minCol, maxCol int) bool {
	matches := cursorPosition.FindAllSubmatch(transcript, -1)
	if len(matches) == 0 {
		return false
	}
	last := matches[len(matches)-1]
	row, _ := strconv.Atoi(string(last[1]))
	col, _ := strconv.Atoi(string(last[2]))
	return row >= minRow && row <= maxRow && col >= minCol && col <= maxCol
}

func cursorRemainsVisible(transcript []byte) bool {
	show, hide := bytes.LastIndex(transcript, []byte("\x1b[?25h")), bytes.LastIndex(transcript, []byte("\x1b[?25l"))
	return show >= 0 && show > hide
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

func assertNoUnexpectedPersistedState(t *testing.T, configDir string) {
	t.Helper()
	entries, err := os.ReadDir(configDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "bindings.json" && !entry.IsDir() {
			contents, err := os.ReadFile(filepath.Join(configDir, entry.Name()))
			if err != nil || !bytes.Contains(contents, []byte(`"Alt-/": "lua:comment.comment"`)) || !bytes.Contains(contents, []byte(`"CtrlUnderscore": "lua:comment.comment"`)) {
				t.Fatalf("bundled comment plugin bindings were not persisted: contents=%s err=%v", contents, err)
			}
			continue
		}
		if entry.Name() == "settings.json" && !entry.IsDir() {
			continue
		}
		if (entry.Name() != "buffers" && entry.Name() != "backups") || !entry.IsDir() {
			t.Fatalf("editor wrote unexpected configuration or buffer state: %v", entries)
		}
		children, err := os.ReadDir(filepath.Join(configDir, entry.Name()))
		if err != nil || (len(children) > 1 || (len(children) == 1 && children[0].Name() != "history")) {
			t.Fatalf("editor wrote unexpected files under %s: entries=%v err=%v", entry.Name(), children, err)
		}
	}
}

func preparePTYConfig(t *testing.T, configDir string) {
	t.Helper()
	for name, contents := range map[string]string{
		"settings.json": `{"keymenu":true}`,
		"bindings.json": `{"buffer":{"CtrlUp":"ScrollUp","CtrlDown":"ScrollDown"}}`,
	} {
		if err := os.WriteFile(filepath.Join(configDir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
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
