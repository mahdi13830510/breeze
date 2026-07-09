package dashboard

import "sync"

// ringBuffer is a fixed-capacity, thread-safe ring buffer for ordered events.
//
// All collectors (requests, queries, logs, timelines) share the same shape:
// append-only, with old entries evicted once capacity is reached, and a
// snapshot operation that returns a copy of the contents in insertion order.
//
// The implementation uses a mutex rather than a lock-free scheme because
// collectors are typically read on the WebSocket broadcast tick (a few times
// per second) and written on the request hot path — the contention is low
// and a mutex keeps the code trivially correct.
type ringBuffer[T any] struct {
	mu       sync.Mutex
	entries  []T
	head     int // index of the oldest entry
	count    int // number of valid entries
	capacity int
}

func newRingBuffer[T any](capacity int) *ringBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &ringBuffer[T]{
		entries:  make([]T, capacity),
		capacity: capacity,
	}
}

// Push appends v, evicting the oldest entry if the buffer is full.
func (r *ringBuffer[T]) Push(v T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count < r.capacity {
		idx := (r.head + r.count) % r.capacity
		r.entries[idx] = v
		r.count++
	} else {
		r.entries[r.head] = v
		r.head = (r.head + 1) % r.capacity
	}
}

// Snapshot returns a copy of all entries in insertion order (oldest first).
func (r *ringBuffer[T]) Snapshot() []T {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]T, r.count)
	for i := 0; i < r.count; i++ {
		out[i] = r.entries[(r.head+i)%r.capacity]
	}
	return out
}

// Len returns the current number of entries.
func (r *ringBuffer[T]) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// Clear removes all entries.
func (r *ringBuffer[T]) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.head = 0
	r.count = 0
}
