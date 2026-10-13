//go:build !linux && !darwin

package perfworkload

func PeakRSSBytes() uint64 { return 0 }
