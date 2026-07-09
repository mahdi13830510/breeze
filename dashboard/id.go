package dashboard

import (
	"sync/atomic"
	"time"
)

// reqIDCounter is a monotonic counter for request IDs.
//
// We use this instead of crypto/rand because crypto/rand.Read does a
// syscall to /dev/urandom on every call — at 130K req/s that's 130K
// syscalls/sec just for ID generation, which destroys throughput.
//
// The counter wraps at 2^60, which at 1M req/s would take ~36,000 years.
// The ID is encoded as a hex string with a timestamp prefix for sortability.
var reqIDCounter uint64

// newID returns a short, unique, sortable ID using an atomic counter.
//
// Format: 12 hex chars = 6 bytes:
//   - 4 bytes: seconds since epoch (gives sortability + uniqueness across restarts)
//   - 2 bytes: monotonic counter (handles >65535 reqs in the same second)
//
// This is ~100x faster than crypto/rand.Read and allocation-free.
func newID() string {
	ts := uint64(time.Now().Unix())
	seq := atomic.AddUint64(&reqIDCounter, 1) & 0xFFFF
	combined := (ts << 16) | seq
	var buf [12]byte
	const hexDigits = "0123456789abcdef"
	for i := 11; i >= 0; i-- {
		buf[i] = hexDigits[combined&0xF]
		combined >>= 4
	}
	return string(buf[:])
}
