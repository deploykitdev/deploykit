package reconciler

import (
	"sync"
	"time"
)

// Queue is a per-key rate-limited FIFO used to drive the reconciler.
//
// Guarantees:
//
//   - **Per-key serialization.** While a key is in-flight (Get returned, Done
//     not yet called) the queue will not hand it out again. Concurrent Adds
//     for the same key are coalesced into a single re-queue on Done. This
//     means one slow reconcile of service A never blocks workers from
//     reconciling service B, and two reconciles of service A never overlap.
//
//   - **Exponential backoff on failure.** AddRateLimited multiplies the per-key
//     failure count and requeues the key after baseDelay * 2^(attempts-1),
//     capped at maxDelay. Forget clears the counter on success.
//
//   - **Scheduled re-queue.** AddAfter schedules an Add for later; the
//     readiness gate uses this to self-requeue every fastTick for an in-flight
//     deployment without a global ticker.
//
// The type is patterned on k8s.io/client-go/util/workqueue.RateLimitingQueue
// but is small enough to own outright (no AddAfter pq, just goroutines).
// Generic over the key type so callers don't have to type-assert.
type Queue[T comparable] struct {
	mu   sync.Mutex
	cond *sync.Cond

	queue      []T              // FIFO of keys waiting for a worker
	dirty      map[T]struct{}   // keys that are queued or were re-added mid-flight
	processing map[T]struct{}   // keys a worker has Got but not yet marked Done
	failures   map[T]int        // per-key consecutive failure count

	baseDelay time.Duration
	maxDelay  time.Duration

	shuttingDown bool
	shutdownCh   chan struct{} // closed on Shutdown so AddAfter timers can bail
}

// NewQueue creates a queue with the given backoff parameters. baseDelay is
// the first retry delay; maxDelay is the cap.
func NewQueue[T comparable](baseDelay, maxDelay time.Duration) *Queue[T] {
	q := &Queue[T]{
		dirty:      map[T]struct{}{},
		processing: map[T]struct{}{},
		failures:   map[T]int{},
		baseDelay:  baseDelay,
		maxDelay:   maxDelay,
		shutdownCh: make(chan struct{}),
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Add inserts key if it isn't already queued. If key is currently being
// processed, it's marked dirty and will be re-queued when the worker calls
// Done. Adds during shutdown are silently dropped.
func (q *Queue[T]) Add(key T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.shuttingDown {
		return
	}
	if _, queued := q.dirty[key]; queued {
		return
	}
	q.dirty[key] = struct{}{}
	if _, inFlight := q.processing[key]; inFlight {
		return // re-queued on Done
	}
	q.queue = append(q.queue, key)
	q.cond.Signal()
}

// AddAfter schedules an Add after delay. If delay <= 0, behaves like Add.
// Each call spawns a short-lived goroutine that returns on Shutdown.
func (q *Queue[T]) AddAfter(key T, delay time.Duration) {
	if delay <= 0 {
		q.Add(key)
		return
	}
	go func() {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-t.C:
			q.Add(key)
		case <-q.shutdownCh:
		}
	}()
}

// AddRateLimited requeues key after baseDelay * 2^(attempts-1), capped at
// maxDelay. The per-key attempt counter is incremented; Forget resets it.
func (q *Queue[T]) AddRateLimited(key T) {
	q.mu.Lock()
	q.failures[key]++
	attempts := q.failures[key]
	q.mu.Unlock()

	delay := q.baseDelay << (attempts - 1)
	if delay <= 0 || delay > q.maxDelay {
		delay = q.maxDelay
	}
	q.AddAfter(key, delay)
}

// Get blocks until a key is available. Returns (zero, true) when the queue
// is shutting down and no keys remain. Callers MUST call Done(key) when
// finished, even on error, to allow re-queue.
func (q *Queue[T]) Get() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.queue) == 0 && !q.shuttingDown {
		q.cond.Wait()
	}
	var zero T
	if len(q.queue) == 0 {
		return zero, true
	}
	key := q.queue[0]
	q.queue = q.queue[1:]
	delete(q.dirty, key)
	q.processing[key] = struct{}{}
	return key, false
}

// Done releases the key. If Add was called while the worker was processing,
// the key is re-queued.
func (q *Queue[T]) Done(key T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.processing, key)
	if _, dirty := q.dirty[key]; dirty {
		q.queue = append(q.queue, key)
		q.cond.Signal()
	}
}

// Forget clears the failure counter for key. Call on successful reconcile.
func (q *Queue[T]) Forget(key T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.failures, key)
}

// Shutdown causes blocked Get callers to return (zero, true) and prevents
// any AddAfter timers from re-enqueueing. Idempotent.
func (q *Queue[T]) Shutdown() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.shuttingDown {
		return
	}
	q.shuttingDown = true
	close(q.shutdownCh)
	q.cond.Broadcast()
}

// Len returns the number of keys waiting for a worker (not counting
// in-flight). Useful for metrics and tests.
func (q *Queue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}
