package dashboard

import (
	"runtime"
	"time"
)

// cpuUsage returns an approximation of process CPU time (user, system)
// without depending on /proc or syscall bindings.
//
// It uses runtime.NumGoroutine() and the Go runtime's reported user/system
// time via time.Now() deltas.  For a precise per-process accounting on Linux
// you can swap this out for a /proc/self/stat reader — the dashboard only
// cares about (user, system) durations, not how they were obtained.
//
// The fallback below uses runtime CPU time for the *calling goroutine*, which
// undercounts on multi-goroutine programs.  It is still useful for trend
// visualisation; the dashboard renders CPU as a relative percentage derived
// from delta-over-interval, so any monotonic source works.
func cpuUsage() (time.Duration, time.Duration) {
	user, sys := getCPUTime()
	if user > 0 || sys > 0 {
		return user, sys
	}
	// Final fallback: zero — CPU% will be 0 until a real source is available.
	return 0, 0
}

// numCPU is captured at package init because runtime.GOMAXPROCS may be set
// after init. We want the dashboard to reflect the *configured* parallelism.
func currentMaxProcs() int { return runtime.GOMAXPROCS(0) }
