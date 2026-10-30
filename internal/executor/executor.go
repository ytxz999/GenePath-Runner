// Package executor runs submitted analyses in the background.
//
// It is deliberately tiny: one buffered channel, a fixed number of workers and a
// per-run timeout. A message queue would be the right answer at a larger scale,
// but for a single-binary demo an in-process queue keeps the deployment story
// honest ("one container, one SQLite file") while still giving the HTTP layer an
// asynchronous contract: submit returns immediately, the client polls status.
package executor

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Runner executes one run by id. The service implements it.
type Runner interface {
	ExecuteRun(ctx context.Context, id string) error
}

// ErrClosed is returned when Submit is called after Shutdown.
var ErrClosed = errors.New("executor: closed")

// Queue is a fixed-size worker pool.
type Queue struct {
	runner  Runner
	ch      chan string
	timeout time.Duration
	wg      sync.WaitGroup
	started atomic.Bool
	closed  atomic.Bool
	active  atomic.Int64

	mu       sync.Mutex
	inflight map[string]struct{}
}

// New builds a queue. buffer bounds how many runs may wait; workers bounds how
// many call the provider at once.
func New(runner Runner, workers, buffer int, timeout time.Duration) *Queue {
	if workers < 1 {
		workers = 1
	}
	if buffer < 0 {
		buffer = 0
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	q := &Queue{
		runner:   runner,
		ch:       make(chan string, buffer),
		timeout:  timeout,
		inflight: make(map[string]struct{}),
	}
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.work(i)
	}
	q.started.Store(true)
	return q
}

// Submit hands a run id to the queue. It never blocks: if the buffer is full the
// caller is told to retry later (the HTTP handler answers 503).
func (q *Queue) Submit(id string) error {
	if q.closed.Load() {
		return ErrClosed
	}
	q.mu.Lock()
	if _, dup := q.inflight[id]; dup {
		q.mu.Unlock()
		return nil
	}
	q.inflight[id] = struct{}{}
	q.mu.Unlock()

	select {
	case q.ch <- id:
		return nil
	default:
		q.mu.Lock()
		delete(q.inflight, id)
		q.mu.Unlock()
		return errors.New("executor: queue full")
	}
}

// Pending reports how many runs are waiting plus how many are executing.
func (q *Queue) Pending() (waiting int, running int64) {
	return len(q.ch), q.active.Load()
}

func (q *Queue) work(n int) {
	defer q.wg.Done()
	for {
		select {
		case id, ok := <-q.ch:
			if !ok {
				return
			}
			q.active.Add(1)
			q.run(n, id)
			q.active.Add(-1)
			q.mu.Lock()
			delete(q.inflight, id)
			q.mu.Unlock()
		}
	}
}

func (q *Queue) run(worker int, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), q.timeout)
	defer cancel()

	started := time.Now()
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("executor: worker %d panicked on %s: %v", worker, id, rec)
		}
	}()

	if err := q.runner.ExecuteRun(ctx, id); err != nil {
		log.Printf("executor: run %s finished with error after %s: %v", id, time.Since(started).Round(time.Millisecond), err)
		return
	}
	log.Printf("executor: run %s finished in %s", id, time.Since(started).Round(time.Millisecond))
}

// Shutdown stops accepting work and waits for in-flight runs to finish. Runs
// still waiting in the buffer are left queued in the database so the next start
// can pick them up (main marks them interrupted today).
func (q *Queue) Shutdown(ctx context.Context) error {
	if q.closed.Swap(true) {
		return nil
	}
	close(q.ch)

	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
