//go:build linux

package perfworkload

import "syscall"

func PeakRSSBytes() uint64 {
	var usage syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) != nil || usage.Maxrss < 0 {
		return 0
	}
	return uint64(usage.Maxrss) * 1024
}
