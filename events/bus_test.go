package events_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/deploykitdev/deploykit"
	"github.com/deploykitdev/deploykit/events"
)

func newTestBus() *events.Bus {
	return events.NewBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestBus_PublishFanout(t *testing.T) {
	bus := newTestBus()
	sub1 := bus.Subscribe(4)
	sub2 := bus.Subscribe(4)
	defer sub1.Close()
	defer sub2.Close()

	evt := deploykit.Event{Type: deploykit.EventServiceCreated, ProjectID: "p1"}
	bus.Publish(context.Background(), evt)

	for i, c := range []<-chan deploykit.Event{sub1.C(), sub2.C()} {
		select {
		case got := <-c:
			if got.Type != evt.Type || got.ProjectID != evt.ProjectID {
				t.Errorf("subscriber %d: got %+v, want %+v", i, got, evt)
			}
			if got.At.IsZero() {
				t.Errorf("subscriber %d: At was not populated", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestBus_Close(t *testing.T) {
	bus := newTestBus()
	sub := bus.Subscribe(4)
	sub.Close()

	// Channel is closed after Close.
	if _, ok := <-sub.C(); ok {
		t.Fatal("channel should be closed after Close()")
	}

	// Close is idempotent.
	sub.Close()

	// Publishing after close must not panic or deliver.
	bus.Publish(context.Background(), deploykit.Event{Type: deploykit.EventServiceCreated})
}

func TestBus_SlowSubscriberDrops(t *testing.T) {
	bus := newTestBus()
	slow := bus.Subscribe(1)
	fast := bus.Subscribe(16)
	defer slow.Close()
	defer fast.Close()

	for range 10 {
		bus.Publish(context.Background(), deploykit.Event{Type: deploykit.EventServiceCreated})
	}

	// Fast subscriber should have received all 10.
	received := 0
	deadline := time.After(time.Second)
	for received < 10 {
		select {
		case <-fast.C():
			received++
		case <-deadline:
			t.Fatalf("fast subscriber got %d/10 events", received)
		}
	}

	// Slow subscriber should have received exactly 1 (buffer size) — the rest
	// were dropped because Publish is non-blocking.
	slowGot := 0
drain:
	for {
		select {
		case <-slow.C():
			slowGot++
		case <-time.After(50 * time.Millisecond):
			break drain
		}
	}
	if slowGot != 1 {
		t.Errorf("slow subscriber: got %d events, want 1", slowGot)
	}
}

func TestBus_ConcurrentPublishSubscribe(t *testing.T) {
	bus := newTestBus()
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			s := bus.Subscribe(4)
			bus.Publish(context.Background(), deploykit.Event{Type: deploykit.EventServiceCreated})
			s.Close()
		}()
	}
	wg.Wait()
}
