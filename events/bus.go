// Package events provides an in-process implementation of deploykit.EventBus.
//
// Publish is non-blocking: each subscriber has its own buffered channel and a
// full buffer causes the message to be dropped for that subscriber and logged.
// This keeps the publisher (typically the reconciler or an HTTP handler) from
// stalling on slow consumers.
package events

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/heyjorgedev/deploykit"
)

const defaultBuffer = 64

// Bus is an in-process EventBus.
type Bus struct {
	logger *slog.Logger

	mu   sync.RWMutex
	subs map[*subscription]struct{}
}

// NewBus constructs a Bus. logger may be nil, in which case slog.Default is used.
func NewBus(logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bus{
		logger: logger,
		subs:   make(map[*subscription]struct{}),
	}
}

// Publish sends evt to every subscriber. If evt.At is zero, it is set to now.
// Slow subscribers (full buffer) drop the message.
func (b *Bus) Publish(_ context.Context, evt deploykit.Event) {
	if evt.At.IsZero() {
		evt.At = time.Now()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs {
		select {
		case s.ch <- evt:
		default:
			b.logger.Warn("event bus: dropping event, subscriber buffer full",
				"type", evt.Type,
				"project_id", evt.ProjectID,
			)
		}
	}
}

// Subscribe returns a new Subscription. If buffer <= 0, a default is used.
func (b *Bus) Subscribe(buffer int) deploykit.Subscription {
	if buffer <= 0 {
		buffer = defaultBuffer
	}
	s := &subscription{
		bus: b,
		ch:  make(chan deploykit.Event, buffer),
	}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s
}

type subscription struct {
	bus *Bus

	closeOnce sync.Once
	ch        chan deploykit.Event
}

func (s *subscription) C() <-chan deploykit.Event {
	return s.ch
}

func (s *subscription) Close() {
	s.closeOnce.Do(func() {
		s.bus.mu.Lock()
		delete(s.bus.subs, s)
		s.bus.mu.Unlock()
		close(s.ch)
	})
}
