//go:build linux || darwin || dragonfly || solaris || openbsd || netbsd || freebsd

package prototype

import (
	"os"
	"syscall"
)

func suspendProcess() error {
	return syscall.Kill(os.Getpid(), syscall.SIGSTOP)
}
