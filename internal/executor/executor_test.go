package executor

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recorder struct {
	mu    sync.Mutex
	calls map[string]int
	order []string
	block chan struct{}
	wait  chan struct{}
}

func newRecorder() *recorder {
	return &recorder{calls: map[string]int{}, wait: make(chan struct{}, 16)}
}

func (r *recorder) ExecuteRun(ctx context.Context, id string) error {
	r.mu.Lock()
	r.calls[id]++
	r.order = append(r.order, id)
	r.mu.Unlock()
	r.wait <- struct{}{}
	if r.block != nil {
		select {
		case <-r.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (r *recorder) count(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[id]
}

func (r *recorder) waitFor(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-r.wait:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for run %d/%d", i+1, n)
		}
	}
}

func TestQueueExecutesEverySubmittedRunExactlyOnce(t *testing.T) {
	rec := newRecorder()
	q := New(rec, 2, 8, time.Second)
	defer func() { _ = q.Shutdown(context.Background()) }()

	ids := []string{"run_a", "run_b", "run_c", "run_d"}
	for _, id := range ids {
		if err := q.Submit(id); err != nil {
			t.Fatalf("Submit(%s): %v", id, err)
		}
	}
	rec.waitFor(t, len(ids))

	deadline := time.Now().Add(time.Second)
	for {
		waiting, running := q.Pending()
		if waiting == 0 && running == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("queue did not drain: waiting=%d running=%d", waiting, running)
		}
		time.Sleep(5 * time.Millisecond)
	}

	for _, id := range ids {
		if got := rec.count(id); got != 1 {
			t.Fatalf("%s executed %d times, want 1", id, got)
		}
	}
}

func TestQueueIgnoresDuplicateSubmission(t *testing.T) {
	rec := newRecorder()
	rec.block = make(chan struct{})
	q := New(rec, 1, 4, time.Second)
	defer func() { _ = q.Shutdown(context.Background()) }()

	if err := q.Submit("run_dup"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := q.Submit("run_dup"); err != nil {
		t.Fatalf("duplicate Submit must be a no-op, got %v", err)
	}
	rec.waitFor(t, 1)
	close(rec.block)

	time.Sleep(50 * time.Millisecond)
	if got := rec.count("run_dup"); got != 1 {
		t.Fatalf("duplicate id executed %d times", got)
	}
}

func TestQueueRejectsWhenBufferIsFull(t *testing.T) {
	rec := newRecorder()
	rec.block = make(chan struct{})
	q := New(rec, 1, 1, time.Second)
	defer func() { _ = q.Shutdown(context.Background()) }()

	if err := q.Submit("run_busy"); err != nil {
		t.Fatalf("busy Submit: %v", err)
	}
	rec.waitFor(t, 1) // the single worker is now blocked
	if err := q.Submit("run_buffered"); err != nil {
		t.Fatalf("buffered Submit: %v", err)
	}
	if err := q.Submit("run_overflow"); err == nil {
		t.Fatal("expected queue-full error once buffer and workers are saturated")
	}
	close(rec.block)
}

func TestShutdownStopsIntakeButLetsInflightFinish(t *testing.T) {
	rec := newRecorder()
	rec.block = make(chan struct{})
	q := New(rec, 1, 2, 2*time.Second)

	if err := q.Submit("run_inflight"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	rec.waitFor(t, 1)

	done := make(chan error, 1)
	go func() { done <- q.Shutdown(context.Background()) }()

	time.Sleep(30 * time.Millisecond)
	close(rec.block)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not wait for the in-flight run")
	}

	if err := q.Submit("run_after"); err != ErrClosed {
		t.Fatalf("Submit after Shutdown = %v, want ErrClosed", err)
	}
}

func TestPerRunTimeoutIsApplied(t *testing.T) {
	rec := newRecorder()
	rec.block = make(chan struct{})
	defer close(rec.block)

	q := New(rec, 1, 1, 60*time.Millisecond)
	defer func() { _ = q.Shutdown(context.Background()) }()

	if err := q.Submit("run_slow"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	rec.waitFor(t, 1)

	deadline := time.Now().Add(time.Second)
	for {
		waiting, running := q.Pending()
		if waiting == 0 && running == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("run did not stop at the per-run deadline")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type panicky struct {
	mu    sync.Mutex
	calls []string
	done  atomic.Int32
}

func (p *panicky) ExecuteRun(_ context.Context, id string) error {
	p.mu.Lock()
	p.calls = append(p.calls, id)
	p.mu.Unlock()
	if id == "run_boom" {
		panic("boom")
	}
	p.done.Add(1)
	return nil
}

func TestWorkerSurvivesAPanickingRun(t *testing.T) {
	p := &panicky{}
	q := New(p, 1, 4, time.Second)
	defer func() { _ = q.Shutdown(context.Background()) }()

	if err := q.Submit("run_boom"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := q.Submit("run_ok"); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for p.done.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("worker died with the panicking run")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
