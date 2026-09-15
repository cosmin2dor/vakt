package schedule

import (
	"container/heap"
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/events"
)

// entry is one schedulable task's next upcoming fire point.
type entry struct {
	task   index.Task
	value  directive.Value // the @schedule/@once value fireAt was computed from
	fireAt time.Time
	pos    int // slice index, maintained by heap.Interface
}

// entryHeap orders entries earliest fireAt first.
type entryHeap []*entry

func (h entryHeap) Len() int           { return len(h) }
func (h entryHeap) Less(i, j int) bool { return h[i].fireAt.Before(h[j].fireAt) }
func (h entryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].pos, h[j].pos = i, j
}
func (h *entryHeap) Push(x any) {
	e := x.(*entry)
	e.pos = len(*h)
	*h = append(*h, e)
}
func (h *entryHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return e
}

// Scheduler keeps one min-heap of next fire points behind a single armed
// timer (SDD §2.7), rebuilt as the index changes, rather than one timer per
// task. It only decides *when* a task's fire point has arrived — what to do
// about it (the suppression ladder) is a later consumer's job.
type Scheduler struct {
	clock Clock
	sub   *events.Subscription[index.Change]

	mu   sync.Mutex
	heap entryHeap
	byID map[string]*entry

	due     chan index.Task
	dropped atomic.Uint64
}

// New seeds the scheduler from tasks (typically idx.Tasks(), so it starts
// populated rather than empty until the first edit) and wires it to sub for
// subsequent Added/Updated/Removed events. clock is injectable for
// deterministic tests; a nil clock uses the real wall clock.
func New(tasks map[string]index.Task, sub *events.Subscription[index.Change], clock Clock) *Scheduler {
	if clock == nil {
		clock = realClock{}
	}
	s := &Scheduler{
		clock: clock,
		sub:   sub,
		byID:  make(map[string]*entry),
		due:   make(chan index.Task, 64),
	}
	s.mu.Lock()
	for _, t := range tasks {
		s.scheduleLocked(t, s.clock.Now())
	}
	s.mu.Unlock()
	return s
}

// Due delivers tasks as their fire point arrives. Non-blocking: a full
// buffer drops the task and counts it (Dropped) rather than stalling the
// scheduler's own timer loop.
func (s *Scheduler) Due() <-chan index.Task {
	return s.due
}

// Dropped returns how many due tasks were dropped because Due's buffer was full.
func (s *Scheduler) Dropped() uint64 {
	return s.dropped.Load()
}

// NextFireAt returns the heap's earliest fire point, if any task is scheduled.
func (s *Scheduler) NextFireAt() (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.heap) == 0 {
		return time.Time{}, false
	}
	return s.heap[0].fireAt, true
}

// Run drives the single timer: reacting to index changes on sub and firing
// due tasks, until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	defer s.sub.Unsubscribe()

	timerC := s.rearm()
	for {
		select {
		case <-ctx.Done():
			return nil
		case change, ok := <-s.sub.C():
			if !ok {
				return nil
			}
			s.apply(change)
			timerC = s.rearm()
		case <-timerC:
			s.fire()
			timerC = s.rearm()
		}
	}
}

// apply folds one index.Change into the heap: added/updated tasks get their
// fire point (re)computed, removed tasks drop out entirely.
func (s *Scheduler) apply(change index.Change) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now()
	for _, t := range change.Added {
		s.scheduleLocked(t, now)
	}
	for _, t := range change.Updated {
		s.scheduleLocked(t, now)
	}
	for _, id := range change.Removed {
		s.removeLocked(id)
	}
}

// scheduleLocked recomputes t's next fire point after "after" and replaces
// its heap entry, first removing any existing one for the same ID (a task
// can be updated in place). A task with no @schedule/@once, or a @once
// already in the past, is simply left unscheduled — not an error, and not
// the missed-trigger policy's bookkeeping (that's a later task; NextFireValue
// already refuses to hand back a past @once, so nothing here ever arms an
// instant fire storm for one). Caller holds s.mu.
func (s *Scheduler) scheduleLocked(t index.Task, after time.Time) {
	s.removeLocked(t.ID)
	v, ok := schedulableValue(t)
	if !ok {
		return
	}
	fireAt, ok, err := NextFireValue(v, after)
	if err != nil || !ok {
		return
	}
	e := &entry{task: t, value: v, fireAt: fireAt}
	heap.Push(&s.heap, e)
	s.byID[t.ID] = e
}

// removeLocked drops id's heap entry, if any. Caller holds s.mu.
func (s *Scheduler) removeLocked(id string) {
	e, ok := s.byID[id]
	if !ok {
		return
	}
	heap.Remove(&s.heap, e.pos)
	delete(s.byID, id)
}

// schedulableValue returns t's @schedule or @once value, whichever is
// present — a task may have neither, in which case it isn't schedulable.
func schedulableValue(t index.Task) (directive.Value, bool) {
	if v, ok := t.Values["schedule"]; ok {
		return v, true
	}
	if v, ok := t.Values["once"]; ok {
		return v, true
	}
	return directive.Value{}, false
}

// fire pops every entry due at-or-before now (a fire point left in the past
// by clock granularity is still due), emits each on Due, and reschedules
// recurring (@schedule/cron) tasks to their own next fire point — a @once
// fires exactly once and is not reinserted.
func (s *Scheduler) fire() {
	now := s.clock.Now()

	s.mu.Lock()
	var due []*entry
	for len(s.heap) > 0 && !s.heap[0].fireAt.After(now) {
		due = append(due, heap.Pop(&s.heap).(*entry))
	}
	for _, e := range due {
		delete(s.byID, e.task.ID)
	}
	s.mu.Unlock()

	for _, e := range due {
		s.sendDue(e.task)
		if e.value.Type == directive.TypeCron {
			s.mu.Lock()
			s.scheduleLocked(e.task, e.fireAt)
			s.mu.Unlock()
		}
	}
}

// sendDue delivers t on due without blocking the timer loop.
func (s *Scheduler) sendDue(t index.Task) {
	select {
	case s.due <- t:
	default:
		s.dropped.Add(1)
	}
}

// rearm returns a channel that fires at the heap's earliest fire point, or
// nil if nothing is scheduled — a nil channel blocks forever in select,
// which is exactly "no timer armed" rather than a spurious wakeup.
func (s *Scheduler) rearm() <-chan time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.heap) == 0 {
		return nil
	}
	d := s.heap[0].fireAt.Sub(s.clock.Now())
	if d < 0 {
		d = 0 // already due; fire on the next tick instead of arming a negative timer
	}
	return s.clock.After(d)
}
