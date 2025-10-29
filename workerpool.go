package breeze

import (
	"context"
	"sync"
)

type WorkerPool struct {
	tasks chan func()
	wg    sync.WaitGroup
	count int
}

func NewWorkerPool(concurrency int) *WorkerPool {
	if concurrency <= 0 {
		concurrency = 4
	}
	p := &WorkerPool{tasks: make(chan func(), concurrency), count: concurrency}
	for i := 0; i < concurrency; i++ {
		go func() {
			for task := range p.tasks {
				task()
			}
		}()
	}
	return p
}

func (p *WorkerPool) Submit(f func()) {
	p.wg.Add(1)
	p.tasks <- func() {
		defer p.wg.Done()
		f()
	}
}

func (p *WorkerPool) Shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	close(p.tasks)
}
