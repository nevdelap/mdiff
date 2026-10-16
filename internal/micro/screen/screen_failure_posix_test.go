//go:build linux || darwin || dragonfly || solaris || openbsd || netbsd || freebsd

package screen

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
	"github.com/nevdelap/mdiff/internal/micro/config"
	"golang.org/x/term"
)

// The child has a terminal on stdin but no controlling TTY. This makes the
// pinned tcell screen constructor succeed and its real Init fail opening
// /dev/tty, the failure path that previously panicked in Fini.
func TestInitFailureDoesNotFinalizePartialScreen(t *testing.T) {
	if os.Getenv("MDIFF_TEST_NO_CONTROLLING_TTY") == "1" {
		config.GlobalSettings = config.DefaultAllSettings()
		if err := Init(); err == nil || !strings.Contains(err.Error(), "/dev/tty") {
			t.Fatalf("screen initialization error = %v, want /dev/tty failure", err)
		}
		if Screen != nil {
			t.Fatal("failed initialization retained a screen")
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
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInitFailureDoesNotFinalizePartialScreen$")
	child.Env = append(os.Environ(), "MDIFF_TEST_NO_CONTROLLING_TTY=1", "TERM=xterm-256color")
	child.Stdin, child.Stdout, child.Stderr = slave, slave, slave
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Run(); ctx.Err() == context.DeadlineExceeded && runtime.GOOS == "darwin" {
		// The pinned tcell Darwin poller can block opening /dev/tty from a
		// detached session. Bound this failure-path probe on that platform.
		t.Skip("Darwin tcell poller blocked opening /dev/tty without a controlling terminal")
	} else if err != nil {
		t.Fatalf("screen init failure subprocess: %v", err)
	}
	after, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed screen initialization changed terminal modes")
	}
}
