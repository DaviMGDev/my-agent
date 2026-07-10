package bus

import (
	"context"
	"errors"
	"sync"
)

const defaultBufferSize = 64

// subscription holds the state for a single subscriber.
type subscription struct {
	handler Handler
	ch      chan Event
	closed  bool
}

// InMemoryBus is a concrete, goroutine-safe, in-process event bus.
//
// Each subscriber runs in its own goroutine that reads from a buffered
// channel. Publish sends the event to all matching subscriber channels
// with a non-blocking send: if a subscriber's buffer is full, the event
// is dropped for that subscriber (at-most-once delivery).
//
// The zero value is NOT usable — use [NewInMemoryBus] to create an instance.
type InMemoryBus struct {
	mu          sync.RWMutex
	subscribers map[string][]*subscription
	wg          sync.WaitGroup
	closed      bool
}

// NewInMemoryBus returns a ready-to-use InMemoryBus.
func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		subscribers: make(map[string][]*subscription),
	}
}

// Subscribe registers a handler for the given event type.
//
// The provided handler must not be nil. If the bus has been closed,
// Subscribe returns [ErrBusClosed].
func (b *InMemoryBus) Subscribe(eventType string, handler Handler) (func(), error) {
	if handler == nil {
		return nil, errHandlerNil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, ErrBusClosed
	}

	sub := &subscription{
		handler: handler,
		ch:      make(chan Event, defaultBufferSize),
	}

	b.subscribers[eventType] = append(b.subscribers[eventType], sub)

	b.wg.Add(1)
	go sub.run(&b.wg)

	unsubscribe := func() {
		b.remove(eventType, sub)
	}

	return unsubscribe, nil
}

// Publish delivers an event to all handlers subscribed to its Type.
//
// Delivery is asynchronous and non-blocking. If a subscriber's buffer
// is full, the event is dropped for that subscriber. If the bus has been
// closed, Publish returns [ErrBusClosed].
func (b *InMemoryBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return ErrBusClosed
	}

	subs := b.subscribers[event.Type]
	for _, sub := range subs {
		select {
		case sub.ch <- event:
		default:
			// subscriber buffer full — drop for this subscriber
		}
	}

	return nil
}

// Close shuts down the bus gracefully. All subscriber channels are closed
// and the call blocks until every in-flight handler has finished. After
// Close returns, all Subscribe and Publish calls return [ErrBusClosed].
//
// Close is idempotent — calling it more than once is safe.
func (b *InMemoryBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}
	b.closed = true

	// Close all subscriber channels and clear the map so that any delayed
	// unsubscribe calls from before Close are no-ops.
	for _, subs := range b.subscribers {
		for _, sub := range subs {
			if !sub.closed {
				sub.closed = true
				close(sub.ch)
			}
		}
	}
	b.subscribers = make(map[string][]*subscription)

	// Wait for all handler goroutines to finish.
	b.wg.Wait()
	return nil
}

// remove unsubscribes a specific subscription. It is safe to call
// multiple times and safe to call after Close.
func (b *InMemoryBus) remove(eventType string, sub *subscription) {
	b.mu.Lock()

	// Prevent double-close of the channel (e.g. if Close ran first).
	if sub.closed {
		b.mu.Unlock()
		return
	}
	sub.closed = true

	subs := b.subscribers[eventType]
	for i, s := range subs {
		if s == sub {
			b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	b.mu.Unlock()

	close(sub.ch)
}

// run is the event-processing loop for a subscriber goroutine.
func (s *subscription) run(wg *sync.WaitGroup) {
	defer wg.Done()
	for event := range s.ch {
		safeHandle(s.handler, event)
	}
}

// safeHandle calls handler.HandleEvent and recovers from panics so that
// a misbehaving handler never crashes the entire bus.
func safeHandle(h Handler, event Event) {
	defer func() { recover() }()
	_ = h.HandleEvent(context.Background(), event)
}

// errHandlerNil is returned by Subscribe when the handler argument is nil.
var errHandlerNil = errors.New("bus: handler cannot be nil")
