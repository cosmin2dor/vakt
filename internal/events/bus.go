package events

import (
	"sync"
	"sync/atomic"
)

// Bus is a generic pub-sub fan-out: one publisher, N subscribers, each
// getting every value. Generic rather than concrete to index.Change: M4's
// SSE hub reuses this same bus for a different payload (EventEnvelope),
// and there's no reason to bake one shape into the only type here.
//
// Slow-subscriber policy: each subscriber has a fixed-size buffered
// channel. Publish never blocks — a send that would block because a
// subscriber's buffer is full is dropped (the new value, not the oldest
// queued one) and counted on that subscription. A stalled subscriber
// therefore only loses messages, in bounded time, and never slows down
// the publisher or any other subscriber.
type Bus[T any] struct {
	mu     sync.Mutex
	subs   map[int]*subscription[T]
	nextID int
}

// New returns an empty Bus.
func New[T any]() *Bus[T] {
	return &Bus[T]{subs: make(map[int]*subscription[T])}
}

// Publish delivers v to every current subscriber. Non-blocking by
// construction: a full subscriber buffer drops v for that subscriber
// rather than waiting.
func (b *Bus[T]) Publish(v T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range b.subs {
		select {
		case sub.ch <- v:
		default:
			sub.dropped.Add(1)
		}
	}
}

// Subscribe registers a new subscriber with a channel buffered to
// bufLen (0 means unbuffered: a value only lands if this subscriber is
// already blocked on a receive at the moment of Publish) and returns its
// handle. The caller must call Unsubscribe when done, or the
// subscription — and its channel — leaks for the life of the bus.
func (b *Bus[T]) Subscribe(bufLen int) *Subscription[T] {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	sub := &subscription[T]{ch: make(chan T, bufLen)}
	b.subs[id] = sub

	return &Subscription[T]{bus: b, id: id, sub: sub}
}

// subscription is the bus-side state for one subscriber.
type subscription[T any] struct {
	ch      chan T
	dropped atomic.Uint64
}

// Subscription is the caller-side handle returned by Subscribe.
type Subscription[T any] struct {
	bus  *Bus[T]
	id   int
	sub  *subscription[T]
	once sync.Once
}

// C returns the channel to receive published values from.
func (s *Subscription[T]) C() <-chan T {
	return s.sub.ch
}

// Dropped returns how many values this subscriber has missed because its
// buffer was full when Publish tried to deliver.
func (s *Subscription[T]) Dropped() uint64 {
	return s.sub.dropped.Load()
}

// Unsubscribe stops delivery to this subscriber and releases its
// channel. Safe to call more than once; only the first call has effect.
func (s *Subscription[T]) Unsubscribe() {
	s.once.Do(func() {
		s.bus.mu.Lock()
		delete(s.bus.subs, s.id)
		s.bus.mu.Unlock()
		close(s.sub.ch)
	})
}
