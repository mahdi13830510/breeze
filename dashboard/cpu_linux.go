//go:build linux

package dashboard

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// getCPUTime reads /proc/self/stat to extract user and system CPU time
// (in clock ticks), then converts to time.Duration using SC_CLK_TCK (which
// is 100 Hz on essentially every Linux system).
//
// This is the same data the Linux kernel exposes via getrusage(2), but
// avoids the syscall binding so we stay within the stdlib.
func getCPUTime() (time.Duration, time.Duration) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, 0
	}
	// /proc/self/stat has 52+ whitespace-separated fields. The comm field
	// (2nd) may contain spaces inside parens, so we find the closing paren
	// and split from there.
	s := string(data)
	rparen := strings.LastIndexByte(s, ')')
	if rparen < 0 || rparen+2 >= len(s) {
		return 0, 0
	}
	rest := strings.Fields(s[rparen+2:])
	// rest[0] = state, rest[1] = ppid, ..., rest[11] = utime, rest[12] = stime
	if len(rest) < 13 {
		return 0, 0
	}
	uticks, err1 := strconv.ParseInt(rest[11], 10, 64)
	sticks, err2 := strconv.ParseInt(rest[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	// 100 Hz clock — convert ticks to nanoseconds.
	const hz = 100
	user := time.Duration(uticks) * time.Second / time.Duration(hz)
	sys := time.Duration(sticks) * time.Second / time.Duration(hz)
	return user, sys
}
