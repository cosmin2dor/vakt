package events

import (
	"testing"
	"time"
)

// TestPublishDoesNotBlockOnSlowSubscriber is the exit bar: a subscriber
// that never reads must not stall the publisher, and a fast subscriber
// must still get everything.
func TestPublishDoesNotBlockOnSlowSubscriber(t *testing.T) {
	const n = 1000

	b := New[int]()

	slow := b.Subscribe(1) // tiny buffer: fills fast since nothing reads it
	defer slow.Unsubscribe()
	fast := b.Subscribe(n) // large enough to never drop while draining concurrently
	defer fast.Unsubscribe()

	// fast drains concurrently with publishing, so its small buffer never
	// backs up; slow never reads, so its buffer fills and Publish must
	// still not block on it.
	got := make([]int, 0, n)
	fastDone := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			got = append(got, <-fast.C())
		}
		close(fastDone)
	}()

	done := make(chan struct{})
	go func() {
		for i := 0; i < n; i++ {
			b.Publish(i)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}

	select {
	case <-fastDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("fast subscriber only received %d/%d values", len(got), n)
	}

	if got := slow.Dropped(); got == 0 {
		t.Errorf("expected the never-reading subscriber to have dropped some values, got 0")
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("fast subscriber received out-of-order value at %d: got %d", i, v)
		}
	}
}

// TestUnsubscribeStopsDelivery confirms an unsubscribed subscriber gets
// nothing published afterward, and that its channel is actually closed
// (resources released, not just ignored).
func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := New[string]()
	sub := b.Subscribe(4)

	b.Publish("before")
	select {
	case v := <-sub.C():
		if v != "before" {
			t.Fatalf("got %q, want %q", v, "before")
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive value published before unsubscribe")
	}

	sub.Unsubscribe()
	b.Publish("after")

	v, ok := <-sub.C()
	if ok {
		t.Fatalf("expected closed channel after unsubscribe, got value %q", v)
	}

	sub.Unsubscribe() // must not panic on repeat
}

// TestMultipleSubscribersAllReceive is the base fan-out case: every
// subscriber gets every published value.
func TestMultipleSubscribersAllReceive(t *testing.T) {
	b := New[int]()
	a := b.Subscribe(4)
	defer a.Unsubscribe()
	c := b.Subscribe(4)
	defer c.Unsubscribe()

	b.Publish(42)

	for _, sub := range []*Subscription[int]{a, c} {
		select {
		case v := <-sub.C():
			if v != 42 {
				t.Fatalf("got %d, want 42", v)
			}
		case <-time.After(time.Second):
			t.Fatal("subscriber did not receive published value")
		}
	}
}
