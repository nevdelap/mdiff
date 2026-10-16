//go:build linux || darwin || dragonfly || solaris || openbsd || netbsd || freebsd

package prototype

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/micro-editor/tcell/v2"
	"github.com/nevdelap/mdiff/internal/micro/screen"
	"golang.org/x/term"
)

func TestTerminalHandoffRealRestartFailureRestoresModes(t *testing.T) {
	if os.Getenv("MDIFF_TEST_REAL_RESTART_FAILURE") == "1" {
		sim := tcell.NewSimulationScreen("")
		if err := sim.Init(); err != nil {
			t.Fatal(err)
		}
		sim.SetSize(60, 10)
		app, err := New(sim, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer app.Close()
		session := newTerminalSession(app, make(chan tcell.Event, 16))
		// This calls the real screen.TempStart -> screen.Init path after the
		// simulation screen is released. The child has no controlling TTY.
		err = session.handoff(func() error { return nil })
		if err == nil || !strings.Contains(err.Error(), "restart terminal after handoff") ||
			!strings.Contains(err.Error(), "/dev/tty") {
			t.Fatalf("missing restart error: %v", err)
		}
		if session.reader != nil || app.terminal != nil || screen.Screen != nil {
			t.Fatal("failed restart reused a released terminal or reader")
		}
		return
	}
	master, slave, err := pty.Open()
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()
	before, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTerminalHandoffRealRestartFailureRestoresModes$")
	child.Env = append(os.Environ(), "MDIFF_TEST_REAL_RESTART_FAILURE=1", "TERM=xterm-256color")
	child.Stdin, child.Stdout, child.Stderr = slave, slave, slave
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Run(); ctx.Err() == context.DeadlineExceeded && runtime.GOOS == "darwin" {
		// The pinned tcell Darwin poller can block opening /dev/tty from a
		// detached session. Bound this failure-path probe on that platform.
		t.Skip("Darwin tcell poller blocked opening /dev/tty without a controlling terminal")
	} else if err != nil {
		t.Fatalf("real restart failure subprocess: %v", err)
	}
	after, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("restart failure changed terminal modes")
	}
}
