package schedule

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/events"
)

// fakeClock is a deterministic Clock: time only moves when Advance is called.
type fakeClock struct {
	mu   sync.Mutex
	now  time.Time
	wait []*fakeTimer
}

type fakeTimer struct {
	at time.Time
	c  chan time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	at := c.now.Add(d)
	if !at.After(c.now) {
		ch <- at
		return ch
	}
	c.wait = append(c.wait, &fakeTimer{at: at, c: ch})
	return ch
}

// Advance moves the clock forward by d, firing any timers now due.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	var remaining, due []*fakeTimer
	for _, tm := range c.wait {
		if !tm.at.After(now) {
			due = append(due, tm)
		} else {
			remaining = append(remaining, tm)
		}
	}
	c.wait = remaining
	c.mu.Unlock()
	for _, tm := range due {
		tm.c <- tm.at
	}
}

func cronTask(id, expr string) index.Task {
	return index.Task{
		ID: id,
		Values: map[string]directive.Value{
			"schedule": {Type: directive.TypeCron, Cron: []string{expr}},
		},
	}
}

func onceTask(id string, at time.Time) index.Task {
	return index.Task{
		ID: id,
		Values: map[string]directive.Value{
			"once": {Type: directive.TypeDatetime, Time: at},
		},
	}
}

func unschedulableTask(id string) index.Task {
	return index.Task{ID: id, Values: map[string]directive.Value{}}
}

// TestNew_SeedsFromInitialTasks: the scheduler starts populated from the
// tasks it's constructed with, not just from later changes.
func TestNew_SeedsFromInitialTasks(t *testing.T) {
	clock := newFakeClock(utc(2024, 1, 1, 0, 0))
	tasks := map[string]index.Task{
		"a": cronTask("a", "0 9 * * *"),
	}
	s := New(tasks, nil, clock)

	fireAt, ok := s.NextFireAt()
	require.True(t, ok)
	require.Equal(t, utc(2024, 1, 1, 9, 0), fireAt)
}

// TestApply_AddRearmsToEarliest: adding a task inserts its fire point and
// the heap's earliest reflects it.
func TestApply_AddRearmsToEarliest(t *testing.T) {
	clock := newFakeClock(utc(2024, 1, 1, 0, 0))
	s := New(nil, nil, clock)

	_, ok := s.NextFireAt()
	require.False(t, ok, "empty scheduler has nothing scheduled")

	s.apply(index.Change{Added: []index.Task{cronTask("a", "0 9 * * *")}})

	fireAt, ok := s.NextFireAt()
	require.True(t, ok)
	require.Equal(t, utc(2024, 1, 1, 9, 0), fireAt)
}

// TestApply_UpdateReplacesFirePoint: editing a task's schedule replaces its
// heap entry rather than leaving a stale one behind.
func TestApply_UpdateReplacesFirePoint(t *testing.T) {
	clock := newFakeClock(utc(2024, 1, 1, 0, 0))
	s := New(map[string]index.Task{"a": cronTask("a", "0 9 * * *")}, nil, clock)

	s.apply(index.Change{Updated: []index.Task{cronTask("a", "0 20 * * *")}})

	fireAt, ok := s.NextFireAt()
	require.True(t, ok)
	require.Equal(t, utc(2024, 1, 1, 20, 0), fireAt)
	require.Len(t, s.heap, 1, "the old entry must not linger alongside the new one")
}

// TestApply_RemoveTheOnlyTaskDisarms: removing the last scheduled task
// leaves nothing armed rather than firing spuriously.
func TestApply_RemoveTheOnlyTaskDisarms(t *testing.T) {
	clock := newFakeClock(utc(2024, 1, 1, 0, 0))
	s := New(map[string]index.Task{"a": cronTask("a", "0 9 * * *")}, nil, clock)

	s.apply(index.Change{Removed: []string{"a"}})

	_, ok := s.NextFireAt()
	require.False(t, ok)
	require.Nil(t, s.rearm(), "no timer should be armed once the heap is empty")
}

// TestApply_UnschedulableTaskIsSkipped: a task with neither @schedule nor
// @once is simply not scheduled, not an error.
func TestApply_UnschedulableTaskIsSkipped(t *testing.T) {
	clock := newFakeClock(utc(2024, 1, 1, 0, 0))
	s := New(nil, nil, clock)

	s.apply(index.Change{Added: []index.Task{unschedulableTask("a")}})

	_, ok := s.NextFireAt()
	require.False(t, ok)
}

// TestApply_PastOnceIsNeverScheduled: a @once already behind "now" doesn't
// arm an instant fire — NextFireValue's own past-@once refusal is relied on.
func TestApply_PastOnceIsNeverScheduled(t *testing.T) {
	clock := newFakeClock(utc(2024, 1, 2, 0, 0))
	s := New(nil, nil, clock)

	s.apply(index.Change{Added: []index.Task{onceTask("a", utc(2024, 1, 1, 0, 0))}})

	_, ok := s.NextFireAt()
	require.False(t, ok)
}

// TestFire_MultipleTasks_EarliestWinsThenNext: with two tasks scheduled,
// firing the earliest pops just that one and rearms to the other.
func TestFire_MultipleTasks_EarliestWinsThenNext(t *testing.T) {
	clock := newFakeClock(utc(2024, 1, 1, 0, 0))
	s := New(map[string]index.Task{
		"early": cronTask("early", "0 9 * * *"),
		"late":  cronTask("late", "0 20 * * *"),
	}, nil, clock)

	fireAt, ok := s.NextFireAt()
	require.True(t, ok)
	require.Equal(t, utc(2024, 1, 1, 9, 0), fireAt)

	clock.Advance(9 * time.Hour)
	s.fire()

	select {
	case task := <-s.Due():
		require.Equal(t, "early", task.ID)
	default:
		t.Fatal("expected the early task to be due")
	}

	// "late" (same-day 20:00) is still earliest; "early" rescheduled itself
	// to next day and is now behind it.
	fireAt, ok = s.NextFireAt()
	require.True(t, ok)
	require.Equal(t, utc(2024, 1, 1, 20, 0), fireAt)
	require.Len(t, s.heap, 2)
	require.Equal(t, utc(2024, 1, 2, 9, 0), s.byID["early"].fireAt, "recurring cron task reschedules itself")
}

// TestFire_OnceDoesNotReschedule: a @once task fires exactly once.
func TestFire_OnceDoesNotReschedule(t *testing.T) {
	clock := newFakeClock(utc(2024, 1, 1, 0, 0))
	s := New(map[string]index.Task{
		"a": onceTask("a", utc(2024, 1, 1, 9, 0)),
	}, nil, clock)

	clock.Advance(9 * time.Hour)
	s.fire()

	select {
	case task := <-s.Due():
		require.Equal(t, "a", task.ID)
	default:
		t.Fatal("expected the once task to be due")
	}

	_, ok := s.NextFireAt()
	require.False(t, ok, "a @once must not reappear in the heap after firing")
}

// TestFire_PastDueEntryStillFires: a fire point left in the past by clock
// granularity is still due, not skipped.
func TestFire_PastDueEntryStillFires(t *testing.T) {
	clock := newFakeClock(utc(2024, 1, 1, 9, 0).Add(time.Second)) // one second past the fire point
	s := New(map[string]index.Task{
		"a": cronTask("a", "0 9 * * *"),
	}, nil, clock)

	// seeding computed the *next* day's fire (strictly after now), so drive
	// the point home directly: an entry whose fireAt is already <= now still
	// fires on the next tick rather than being silently dropped.
	s.mu.Lock()
	s.heap[0].fireAt = utc(2024, 1, 1, 9, 0)
	s.mu.Unlock()

	s.fire()

	select {
	case task := <-s.Due():
		require.Equal(t, "a", task.ID)
	default:
		t.Fatal("expected the past-due task to be due")
	}
}

// TestRun_EndToEnd drives the scheduler through its real Run loop: an index
// change arrives on the bus, the timer rearms, and advancing the fake clock
// delivers the due task — all without any real-time sleeping.
func TestRun_EndToEnd(t *testing.T) {
	bus := events.New[index.Change]()
	sub := bus.Subscribe(4)
	clock := newFakeClock(utc(2024, 1, 1, 0, 0))
	s := New(nil, sub, clock)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	bus.Publish(index.Change{Added: []index.Task{cronTask("a", "0 9 * * *")}})

	// Wait for Run's goroutine to have consumed the change and armed the
	// timer before advancing the clock — this only synchronizes goroutine
	// scheduling, the fire time itself is still deterministically fake-clock
	// driven.
	require.Eventually(t, func() bool {
		fireAt, ok := s.NextFireAt()
		return ok && fireAt.Equal(utc(2024, 1, 1, 9, 0))
	}, 2*time.Second, time.Millisecond)

	clock.Advance(9 * time.Hour)

	select {
	case task := <-s.Due():
		require.Equal(t, "a", task.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the scheduled task to fire")
	}
}
