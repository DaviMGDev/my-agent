package bus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// InMemoryBus - Happy path
// ---------------------------------------------------------------------------

func TestInMemoryBus_SubscribeAndPublish(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	received := make(chan Event, 1)
	_, err := b.Subscribe("test.event", HandlerFunc(func(ctx context.Context, e Event) error {
		received <- e
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	want := Event{Type: "test.event", Payload: "hello"}
	if err := b.Publish(context.Background(), want); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-received:
		if got.Type != want.Type {
			t.Errorf("got type %q, want %q", got.Type, want.Type)
		}
		if got.Payload != want.Payload {
			t.Errorf("got payload %v, want %v", got.Payload, want.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event delivery")
	}
}

func TestInMemoryBus_MultipleSubscribers(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	const n = 5
	var count atomic.Int32
	for i := 0; i < n; i++ {
		_, err := b.Subscribe("test.event", HandlerFunc(func(ctx context.Context, e Event) error {
			count.Add(1)
			return nil
		}))
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := b.Publish(context.Background(), Event{Type: "test.event"}); err != nil {
		t.Fatal(err)
	}

	// All 5 handlers should fire.
	deadline := time.After(time.Second)
	for {
		if count.Load() == int32(n) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected %d deliveries, got %d", n, count.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestInMemoryBus_DifferentEventTypes(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	var gotA, gotB atomic.Bool

	_, _ = b.Subscribe("type.a", HandlerFunc(func(ctx context.Context, e Event) error {
		gotA.Store(true)
		return nil
	}))
	_, _ = b.Subscribe("type.b", HandlerFunc(func(ctx context.Context, e Event) error {
		gotB.Store(true)
		return nil
	}))

	_ = b.Publish(context.Background(), Event{Type: "type.a"})
	time.Sleep(50 * time.Millisecond)

	if !gotA.Load() {
		t.Error("type.a handler did not fire")
	}
	if gotB.Load() {
		t.Error("type.b handler fired for type.a event")
	}
}

// ---------------------------------------------------------------------------
// Unsubscribe
// ---------------------------------------------------------------------------

func TestInMemoryBus_Unsubscribe(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	var count atomic.Int32
	unsub, err := b.Subscribe("test.event", HandlerFunc(func(ctx context.Context, e Event) error {
		count.Add(1)
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}

	// First publish — handler should fire.
	_ = b.Publish(context.Background(), Event{Type: "test.event"})
	time.Sleep(50 * time.Millisecond)
	if count.Load() != 1 {
		t.Fatalf("expected 1 delivery before unsubscribe, got %d", count.Load())
	}

	unsub()

	// Second publish — handler should NOT fire.
	_ = b.Publish(context.Background(), Event{Type: "test.event"})
	time.Sleep(50 * time.Millisecond)
	if count.Load() != 1 {
		t.Fatalf("expected 1 delivery after unsubscribe, got %d", count.Load())
	}
}

func TestInMemoryBus_UnsubscribeFromWithinHandler(t *testing.T) {
	// This test verifies there is no deadlock when a handler unsubscribes
	// itself during event processing.
	b := NewInMemoryBus()
	defer b.Close()

	var mu sync.Mutex
	unsubOnce := sync.Once{}
	var unsub func()

	var handled atomic.Bool
	unsub, _ = b.Subscribe("test.event", HandlerFunc(func(ctx context.Context, e Event) error {
		mu.Lock()
		defer mu.Unlock()
		handled.Store(true)
		unsubOnce.Do(func() {
			unsub() // self-unsubscribe while processing
		})
		return nil
	}))

	_ = b.Publish(context.Background(), Event{Type: "test.event"})
	time.Sleep(100 * time.Millisecond)

	if !handled.Load() {
		t.Error("handler did not fire")
	}

	// Publish again — should not be delivered (already unsubscribed).
	handled.Store(false)
	_ = b.Publish(context.Background(), Event{Type: "test.event"})
	time.Sleep(100 * time.Millisecond)
	if handled.Load() {
		t.Error("handler received event after self-unsubscribe")
	}
}

func TestInMemoryBus_UnsubscribeFromDifferentEventType(t *testing.T) {
	// Ensure unsubscribing one event type does not affect another.
	b := NewInMemoryBus()
	defer b.Close()

	var countA, countB atomic.Int32

	unsub, _ := b.Subscribe("type.a", HandlerFunc(func(ctx context.Context, e Event) error {
		countA.Add(1)
		return nil
	}))
	_, _ = b.Subscribe("type.b", HandlerFunc(func(ctx context.Context, e Event) error {
		countB.Add(1)
		return nil
	}))

	_ = b.Publish(context.Background(), Event{Type: "type.a"})
	_ = b.Publish(context.Background(), Event{Type: "type.b"})
	time.Sleep(50 * time.Millisecond)

	unsub() // only unsubscribes from type.a

	_ = b.Publish(context.Background(), Event{Type: "type.a"})
	_ = b.Publish(context.Background(), Event{Type: "type.b"})
	time.Sleep(50 * time.Millisecond)

	if countA.Load() != 1 {
		t.Errorf("expected 1 delivery for type.a, got %d", countA.Load())
	}
	if countB.Load() != 2 {
		t.Errorf("expected 2 deliveries for type.b, got %d", countB.Load())
	}
}

// ---------------------------------------------------------------------------
// Close behaviour
// ---------------------------------------------------------------------------

func TestInMemoryBus_PublishAfterClose(t *testing.T) {
	b := NewInMemoryBus()
	b.Close()
	if err := b.Publish(context.Background(), Event{Type: "test"}); !errors.Is(err, ErrBusClosed) {
		t.Fatalf("expected ErrBusClosed, got %v", err)
	}
}

func TestInMemoryBus_SubscribeAfterClose(t *testing.T) {
	b := NewInMemoryBus()
	b.Close()
	_, err := b.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		return nil
	}))
	if !errors.Is(err, ErrBusClosed) {
		t.Fatalf("expected ErrBusClosed, got %v", err)
	}
}

func TestInMemoryBus_CloseIsIdempotent(t *testing.T) {
	b := NewInMemoryBus()
	b.Close()
	b.Close() // second close should not panic
}

func TestInMemoryBus_CloseWaitsForHandlers(t *testing.T) {
	b := NewInMemoryBus()

	done := make(chan struct{})
	_, _ = b.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		time.Sleep(100 * time.Millisecond)
		close(done)
		return nil
	}))

	_ = b.Publish(context.Background(), Event{Type: "test"})

	start := time.Now()
	b.Close()
	elapsed := time.Since(start)

	if elapsed < 100*time.Millisecond {
		t.Errorf("Close returned too quickly; handler may not have finished")
	}

	// Verify the handler actually ran.
	select {
	case <-done:
	default:
		t.Error("handler did not run before Close returned")
	}
}

// ---------------------------------------------------------------------------
// Nil handler
// ---------------------------------------------------------------------------

func TestInMemoryBus_SubscribeNilHandler(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	_, err := b.Subscribe("test", nil)
	if err == nil {
		t.Fatal("expected error for nil handler, got nil")
	}
}

// ---------------------------------------------------------------------------
// Handler panic recovery
// ---------------------------------------------------------------------------

func TestInMemoryBus_HandlerPanicDoesNotCrashBus(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	var count atomic.Int32
	_, _ = b.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		panic("boom")
	}))
	_, _ = b.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		count.Add(1)
		return nil
	}))

	_ = b.Publish(context.Background(), Event{Type: "test"})
	time.Sleep(50 * time.Millisecond)

	// The second (non-panicking) handler should still have received the event.
	if count.Load() != 1 {
		t.Errorf("expected surviving handler to receive event, got %d", count.Load())
	}
}

// ---------------------------------------------------------------------------
// Non-blocking publish on full buffer
// ---------------------------------------------------------------------------

func TestInMemoryBus_PublishNonBlockingOnFullBuffer(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	// Block the handler so it never consumes from the channel.
	blocked := make(chan struct{})
	_, _ = b.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		<-blocked
		return nil
	}))

	// Publish more events than the buffer capacity (64) — must not block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 128; i++ {
			_ = b.Publish(context.Background(), Event{Type: "test", Payload: i})
		}
		close(done)
	}()

	select {
	case <-done:
		// all publishes completed without blocking
	case <-time.After(2 * time.Second):
		t.Fatal("publish blocked despite full subscriber buffer")
	}

	close(blocked)
}

// ---------------------------------------------------------------------------
// Multiple subs, only one unsubscribes
// ---------------------------------------------------------------------------

func TestInMemoryBus_SubsetUnsubscribe(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	var count1, count2 atomic.Int32

	unsub1, _ := b.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		count1.Add(1)
		return nil
	}))
	_, _ = b.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		count2.Add(1)
		return nil
	}))

	_ = b.Publish(context.Background(), Event{Type: "test"})
	time.Sleep(50 * time.Millisecond)

	unsub1() // only first subscriber leaves

	_ = b.Publish(context.Background(), Event{Type: "test"})
	time.Sleep(50 * time.Millisecond)

	if count1.Load() != 1 {
		t.Errorf("unsubscribed handler received %d events, want 1", count1.Load())
	}
	if count2.Load() != 2 {
		t.Errorf("remaining handler received %d events, want 2", count2.Load())
	}
}

// ---------------------------------------------------------------------------
// HandlerFunc adapter
// ---------------------------------------------------------------------------

func TestHandlerFunc(t *testing.T) {
	ctx := context.Background()
	event := Event{Type: "test", Payload: "v"}
	var got Event
	fn := HandlerFunc(func(ctx context.Context, e Event) error {
		got = e
		return nil
	})
	if err := fn.HandleEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	if got.Type != event.Type || got.Payload != event.Payload {
		t.Errorf("HandlerFunc did not pass through the event")
	}
}

// ---------------------------------------------------------------------------
// MockBus
// ---------------------------------------------------------------------------

func TestMockBus_RecordsPublishedEvents(t *testing.T) {
	m := &MockBus{}

	events := []Event{
		{Type: "a", Payload: 1},
		{Type: "b", Payload: "two"},
	}

	for _, e := range events {
		if err := m.Publish(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}

	if len(m.Published) != len(events) {
		t.Fatalf("expected %d published events, got %d", len(events), len(m.Published))
	}
	for i, e := range events {
		if m.Published[i].Type != e.Type || m.Published[i].Payload != e.Payload {
			t.Errorf("event %d mismatch", i)
		}
	}
}

func TestMockBus_SubscribeReturnsNoopUnsub(t *testing.T) {
	m := &MockBus{}
	unsub, err := m.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if unsub == nil {
		t.Fatal("expected non-nil unsubscribe func")
	}
	// Should not panic.
	unsub()
}

func TestMockBus_InjectablePublishError(t *testing.T) {
	wantErr := errors.New("injected error")
	m := &MockBus{
		PublishFunc: func(ctx context.Context, e Event) error {
			return wantErr
		},
	}

	err := m.Publish(context.Background(), Event{Type: "test"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestMockBus_InjectableSubscribeError(t *testing.T) {
	wantErr := errors.New("subscribe error")
	m := &MockBus{
		SubscribeFunc: func(eventType string, handler Handler) (func(), error) {
			return nil, wantErr
		},
	}

	_, err := m.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		return nil
	}))
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestMockBus_Close(t *testing.T) {
	m := &MockBus{}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMockBus_CloseWithInjectableError(t *testing.T) {
	wantErr := errors.New("close error")
	m := &MockBus{
		CloseFunc: func() error {
			return wantErr
		},
	}
	if err := m.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

// ---------------------------------------------------------------------------
// No subscriber for event type
// ---------------------------------------------------------------------------

func TestInMemoryBus_PublishWithoutSubscribers(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	// Publishing to a type with no subscribers should be a no-op (not an error).
	err := b.Publish(context.Background(), Event{Type: "nonexistent"})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Concurrent publish
// ---------------------------------------------------------------------------

func TestInMemoryBus_ConcurrentPublish(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	var count atomic.Int32
	_, _ = b.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		count.Add(1)
		return nil
	}))

	// Use a total number of events that fits within the default buffer (64)
	// so no events are dropped and we can verify delivery.
	const workers = 8
	const eventsPerWorker = 6
	const expected = workers * eventsPerWorker // 48, fits in buffer

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerWorker; j++ {
				_ = b.Publish(context.Background(), Event{Type: "test", Payload: id*1000 + j})
			}
		}(i)
	}
	wg.Wait()

	// Give handler goroutine time to consume all events.
	deadline := time.After(2 * time.Second)
	for {
		n := count.Load()
		if n == int32(expected) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("expected %d events, got %d", expected, n)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// ---------------------------------------------------------------------------
// Unsubscribe after Close
// ---------------------------------------------------------------------------

func TestInMemoryBus_UnsubscribeAfterClose(t *testing.T) {
	b := NewInMemoryBus()
	unsub, _ := b.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		return nil
	}))

	b.Close()

	// Must not panic.
	unsub()
}

// ---------------------------------------------------------------------------
// Empty event type
// ---------------------------------------------------------------------------

func TestInMemoryBus_EmptyEventType(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	var count atomic.Int32
	_, _ = b.Subscribe("", HandlerFunc(func(ctx context.Context, e Event) error {
		count.Add(1)
		return nil
	}))

	_ = b.Publish(context.Background(), Event{Type: ""})
	time.Sleep(50 * time.Millisecond)

	if count.Load() != 1 {
		t.Errorf("expected 1 delivery for empty event type, got %d", count.Load())
	}
}

// ---------------------------------------------------------------------------
// Nil payload
// ---------------------------------------------------------------------------

func TestInMemoryBus_NilPayload(t *testing.T) {
	b := NewInMemoryBus()
	defer b.Close()

	received := make(chan Event, 1)
	_, _ = b.Subscribe("test", HandlerFunc(func(ctx context.Context, e Event) error {
		received <- e
		return nil
	}))

	event := Event{Type: "test"} // Payload is nil
	if err := b.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-received:
		if got.Payload != nil {
			t.Error("expected nil payload")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
