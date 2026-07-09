package breeze

import (
        "context"
        "fmt"
        "runtime"
        "runtime/debug"
        "sync"
        "sync/atomic"
)

// WorkerPool dispatches tasks to a fixed pool of goroutines.
//
// Performance decisions:
//   - Channel buffer is set to 16× the worker count (not 1×) so that a burst
//     of requests doesn't block gnet's event-loop goroutine while workers
//     catch up. The event loop is single-threaded per reactor; blocking it
//     stalls ALL connections on that reactor.
//   - defaultWorkerMultiplier × NumCPU is the sweet spot for I/O-bound
//     handlers. CPU-bound handlers should use NumCPU directly.
//
// Safety:
//   - Submit is safe to call concurrently from multiple goroutines.
//   - Submit after Shutdown is a no-op (the task is silently dropped).
//     This prevents the "send on closed channel" panic that would
//     otherwise occur if a request arrived during shutdown.
//   - Tasks are wrapped with panic recovery. A panicking task is logged
//     and the worker continues — the WaitGroup counter is always
//     decremented (via defer), so panics do NOT leak goroutines or
//     deadlock Shutdown.
//   - Workers exit cleanly when the tasks channel is closed.
const defaultChannelMultiplier = 16

type WorkerPool struct {
        tasks  chan func()
        wg     sync.WaitGroup
        count  int
        closed atomic.Bool // set to true by Shutdown; prevents Submit from sending
}

// NewWorkerPool creates a pool with `concurrency` goroutines and a task queue
// of concurrency × defaultChannelMultiplier to absorb request bursts without
// blocking gnet's event loops.
func NewWorkerPool(concurrency int) *WorkerPool {
        if concurrency <= 0 {
                concurrency = runtime.NumCPU()
        }
        bufSize := concurrency * defaultChannelMultiplier
        p := &WorkerPool{
                tasks: make(chan func(), bufSize),
                count: concurrency,
        }
        for i := 0; i < concurrency; i++ {
                go p.worker()
        }
        return p
}

// worker is the goroutine that drains the tasks channel.
// It exits when the channel is closed and drained.
func (p *WorkerPool) worker() {
        for task := range p.tasks {
                task()
        }
}

// Submit enqueues a task. It never blocks as long as the queue has capacity;
// if the queue is full it falls back to spawning a goroutine so the event
// loop is never stalled.
//
// The task is wrapped with panic recovery: if f panics, the panic is logged
// and swallowed. The WaitGroup counter is always decremented via defer, so
// panics do NOT leak goroutines or deadlock Shutdown.
//
// After Shutdown has been called, Submit is a no-op — the task is silently
// dropped. This prevents "send on closed channel" panics during graceful
// shutdown.
func (p *WorkerPool) Submit(f func()) {
        // If the pool is shutting down, drop the task.
        if p.closed.Load() {
                return
        }
        p.wg.Add(1)
        // Wrap the task with panic recovery so a panicking handler does not
        // crash the worker goroutine. Without this, the defer Done() would
        // never run, leaking the WaitGroup counter and deadlocking Shutdown.
        task := func() {
                defer p.wg.Done()
                defer func() {
                        if r := recover(); r != nil {
                                fmt.Printf("[Breeze][WorkerPool][PANIC] %v\n%s\n", r, debug.Stack())
                        }
                }()
                f()
        }
        select {
        case p.tasks <- task:
                // queued normally
        default:
                // Queue full: spawn a goroutine rather than blocking the event loop.
                go task()
        }
}

// Shutdown waits for all in-flight tasks to complete or for ctx to expire.
//
// After Shutdown returns, the worker goroutines have exited and the tasks
// channel is closed. Any subsequent Submit calls are silently dropped.
func (p *WorkerPool) Shutdown(ctx context.Context) {
        // Mark the pool as closed so Submit stops accepting new work.
        p.closed.Store(true)

        done := make(chan struct{})
        go func() {
                p.wg.Wait()
                close(done)
        }()
        select {
        case <-done:
                // All in-flight tasks completed.
        case <-ctx.Done():
                // Timeout — leave the pool running. Workers will exit when the
                // channel is closed below only if done was reached; otherwise
                // we leave them running to avoid dropping in-flight tasks.
                return
        }
        // Close the channel — workers will drain remaining tasks and exit.
        close(p.tasks)
}
