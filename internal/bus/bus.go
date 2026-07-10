// Package bus provides an in-process, pub/sub event bus for decoupling
// event producers from event consumers within the same process.
//
// The core interface is [Bus], which supports Subscribe, Publish, and Close.
// [InMemoryBus] is the concrete implementation backed by buffered channels
// and goroutines. It delivers events asynchronously with at-most-once
// delivery guarantees per subscriber. [MockBus] provides a deterministic
// test double.
//
// # Event Type Convention
//
// Event types are plain strings. By convention, use a dotted hierarchical
// naming scheme (e.g., "agent.tool_call", "llm.chunk") to enable future
// pattern-matching on subscriptions without changing the interface.
//
// # Architectural Decision
//
// The decision to add this package is recorded in docs/adr/002-event-bus-package.md.
package bus

import (
	"context"
	"errors"
	"sync"
)

// ErrBusClosed is returned by Publish or Subscribe when the bus has been
// shut down via Close.
var ErrBusClosed = errors.New("bus: closed")

// Event is a typed payload published through the bus.
type Event struct {
	Type    string
	Payload any
}

// Handler receives events published to the bus.
type Handler interface {
	// HandleEvent processes a single published event. Returning an error
	// does not prevent delivery to other handlers; the error is dropped
	// in asynchronous delivery mode.
	HandleEvent(ctx context.Context, event Event) error
}

// HandlerFunc adapts a function to the [Handler] interface.
type HandlerFunc func(ctx context.Context, event Event) error

// HandleEvent implements [Handler] by calling the underlying function.
func (f HandlerFunc) HandleEvent(ctx context.Context, event Event) error {
	return f(ctx, event)
}

// Bus is an in-process pub/sub event bus.
//
// Typical usage:
//
//	b := bus.NewInMemoryBus()
//	defer b.Close()
//
//	unsub, _ := b.Subscribe("my.event", bus.HandlerFunc(func(ctx context.Context, e bus.Event) error {
//	    fmt.Println("got:", e.Payload)
//	    return nil
//	}))
//	defer unsub()
//
//	_ = b.Publish(ctx, bus.Event{Type: "my.event", Payload: "hello"})
type Bus interface {
	// Subscribe registers a handler for the given event type. The returned
	// unsubscribe function, when called, removes the handler so it will no
	// longer receive events for this type.
	//
	// Subscribe returns ErrBusClosed if the bus has been shut down.
	Subscribe(eventType string, handler Handler) (unsubscribe func(), err error)

	// Publish delivers an event to every handler subscribed to its type.
	// Delivery is asynchronous and non-blocking: if a subscriber's buffer
	// is full, the event is dropped for that subscriber.
	//
	// Publish returns ErrBusClosed if the bus has been shut down.
	Publish(ctx context.Context, event Event) error

	// Close shuts down the bus. All subscriber channels are closed, and the
	// call blocks until every in-flight handler has finished. After Close
	// returns, all subsequent Subscribe and Publish calls return ErrBusClosed.
	Close() error
}

// compile-time interface checks.
var (
	_ Bus = (*InMemoryBus)(nil)
	_ Bus = (*MockBus)(nil)
)

// --- MockBus ---------------------------------------------------------------

// MockBus is a deterministic [Bus] implementation for testing.
// By default it records all published events and allows all subscriptions.
// Each method's behavior can be overridden via the corresponding Func field.
type MockBus struct {
	mu        sync.Mutex
	Published []Event

	// SubscribeFunc overrides Subscribe behaviour when non-nil.
	SubscribeFunc func(eventType string, handler Handler) (func(), error)
	// PublishFunc overrides Publish behaviour when non-nil.
	PublishFunc func(ctx context.Context, event Event) error
	// CloseFunc overrides Close behaviour when non-nil.
	CloseFunc func() error
}

func (m *MockBus) Subscribe(eventType string, handler Handler) (func(), error) {
	if m.SubscribeFunc != nil {
		return m.SubscribeFunc(eventType, handler)
	}
	return func() {}, nil
}

func (m *MockBus) Publish(ctx context.Context, event Event) error {
	m.mu.Lock()
	m.Published = append(m.Published, event)
	m.mu.Unlock()

	if m.PublishFunc != nil {
		return m.PublishFunc(ctx, event)
	}
	return nil
}

func (m *MockBus) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}
