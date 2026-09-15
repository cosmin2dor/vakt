package schedule

import "time"

// Clock abstracts wall-clock reads and timers so the scheduler can be driven
// deterministically in tests instead of sleeping in real time.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// realClock is the production Clock, backed by the standard library.
type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
