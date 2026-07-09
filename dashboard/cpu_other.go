//go:build !linux

package dashboard

import "time"

// getCPUTime is a no-op on non-Linux platforms. The dashboard's CPU% will
// show as 0; all other metrics are unaffected.
//
// Porting note: on macOS use getrusage via syscall; on Windows use
// GetProcessTimes. Both are out of scope for the dashboard's stdlib-only
// stance.
func getCPUTime() (time.Duration, time.Duration) {
	return 0, 0
}
