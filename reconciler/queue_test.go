package reconciler

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueue_FIFOOrder(t *testing.T) {
	q := NewQueue[string](time.Millisecond, time.Second)
	q.Add("a")
	q.Add("b")
	q.Add("c")

	for _, want := range []string{"a", "b", "c"} {
		got, shutdown := q.Get()
		if shutdown {
			t.Fatalf("unexpected shutdown waiting for %q", want)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		q.Done(got)
	}
}

func TestQueue_AddCoalesces(t *testing.T) {
	q := NewQueue[string](time.Millisecond, time.Second)
	q.Add("a")
	q.Add("a")
	q.Add("a")
	if q.Len() != 1 {
		t.Fatalf("expected coalesced len=1, got %d", q.Len())
	}
}

func TestQueue_AddDuringProcessing_RequeuesOnDone(t *testing.T) {
	q := NewQueue[string](time.Millisecond, time.Second)
	q.Add("a")

	got, _ := q.Get()
	if got != "a" {
		t.Fatalf("got %q, want a", got)
	}
	// Now "a" is in-flight. Adding it again should NOT put it in the queue
	// (so a parallel Get for the same key won't fire), but it should be
	// re-queued when we Done.
	q.Add("a")
	if q.Len() != 0 {
		t.Errorf("in-flight key should not be queued during processing; len=%d", q.Len())
	}
	q.Done("a")
	if q.Len() != 1 {
		t.Errorf("after Done, dirty key should be re-queued; len=%d", q.Len())
	}
}

func TestQueue_PerKeySerialization(t *testing.T) {
	// Two workers, one key Add'd many times — only one worker should ever
	// hold "a" at a time.
	q := NewQueue[string](time.Millisecond, time.Second)

	var inFlight int32
	var maxObserved int32

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				key, shutdown := q.Get()
				if shutdown {
					return
				}
				cur := atomic.AddInt32(&inFlight, 1)
				for {
					prev := atomic.LoadInt32(&maxObserved)
					if cur <= prev || atomic.CompareAndSwapInt32(&maxObserved, prev, cur) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond)
				atomic.AddInt32(&inFlight, -1)
				q.Done(key)
			}
		}()
	}

	for i := 0; i < 50; i++ {
		q.Add("a")
	}
	// Give the workers time to process and re-add coalesce, then shut down.
	time.Sleep(50 * time.Millisecond)
	q.Shutdown()
	wg.Wait()

	if max := atomic.LoadInt32(&maxObserved); max > 1 {
		t.Errorf("expected at most 1 in-flight for same key, observed %d", max)
	}
}

func TestQueue_ShutdownUnblocksGet(t *testing.T) {
	q := NewQueue[string](time.Millisecond, time.Second)

	done := make(chan struct{})
	go func() {
		_, shutdown := q.Get()
		if !shutdown {
			t.Errorf("expected shutdown=true from Get after Shutdown")
		}
		close(done)
	}()

	time.Sleep(10 * time.Millisecond) // let the goroutine block in Get
	q.Shutdown()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Get did not unblock after Shutdown")
	}
}

func TestQueue_AddAfter(t *testing.T) {
	q := NewQueue[string](time.Millisecond, time.Second)

	start := time.Now()
	q.AddAfter("a", 20*time.Millisecond)

	key, shutdown := q.Get()
	if shutdown {
		t.Fatal("unexpected shutdown")
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Errorf("AddAfter fired too early: %v", elapsed)
	}
	if key != "a" {
		t.Errorf("got %q, want a", key)
	}
	q.Done(key)
}

func TestQueue_RateLimitedBackoff(t *testing.T) {
	q := NewQueue[string](5*time.Millisecond, 200*time.Millisecond)

	// First failure: ~5ms. Second: ~10ms. Third: ~20ms.
	q.AddRateLimited("a")
	start := time.Now()
	_, _ = q.Get()
	d1 := time.Since(start)
	q.Done("a")
	q.AddRateLimited("a")

	start = time.Now()
	_, _ = q.Get()
	d2 := time.Since(start)
	q.Done("a")

	if d2 <= d1 {
		t.Errorf("expected second backoff (%v) to exceed first (%v)", d2, d1)
	}

	// Forget should reset the counter — next AddRateLimited fires fast again.
	q.Forget("a")
	q.AddRateLimited("a")
	start = time.Now()
	_, _ = q.Get()
	d3 := time.Since(start)
	q.Done("a")

	if d3 >= d2 {
		t.Errorf("Forget didn't reset backoff: d3=%v expected < d2=%v", d3, d2)
	}
}
