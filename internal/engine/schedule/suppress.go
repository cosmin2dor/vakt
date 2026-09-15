package schedule

import (
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
)

// Rung identifies which step of G7's suppression ladder produced a Decision.
type Rung int

const (
	RungState Rung = iota + 1
	RungSkipUntil
	RungLastCompleted
	RungSkipCount
	RungDispatch
)

// WriteBackRequest describes a directive change the ladder decided is
// needed, for the writeback-orchestration task to actually apply — this
// package never touches the vault itself.
type WriteBackRequest struct {
	Task      index.Task
	Directive string // directive name, e.g. "skip_count"
	Value     directive.Value
}

// Decision is the ladder's verdict for one task at one fire instant: which
// rung fired, whether it suppresses the fire, and any writeback it implies.
type Decision struct {
	Rung       Rung
	Suppressed bool
	WriteBack  *WriteBackRequest
}

// Evaluate runs G7's five rungs, in strict order, for t at fire instant now
// (the fire point Scheduler.Due() just handed the caller). Only rung 4
// implies a WriteBack; rungs 1-3 suppress with no side effect at all, and a
// later rung is never reached once an earlier one has decided.
func Evaluate(t index.Task, now time.Time) (Decision, error) {
	// Rung 1: state must be active or triggered.
	if state := t.Values["state"].Str; state != "active" && state != "triggered" {
		return Decision{Rung: RungState, Suppressed: true}, nil
	}

	// Rung 2: a future @skip_until suppresses outright; skip_count is untouched.
	if v, ok := t.Values["skip_until"]; ok && v.Time.After(now) {
		return Decision{Rung: RungSkipUntil, Suppressed: true}, nil
	}

	// Rung 3: @last_completed inside [previous fire, now) (G8) bypasses this trigger.
	if lc, ok := t.Values["last_completed"]; ok {
		if sv, ok := schedulableValue(t); ok {
			// PrevFireValue's "after" bound is inclusive, and now is itself
			// the fire that just arrived — step back a tick so it finds the
			// prior occurrence, not this one.
			prev, ok, err := PrevFireValue(sv, now.Add(-time.Nanosecond))
			if err != nil {
				return Decision{}, err
			}
			if ok && !lc.Time.Before(prev) && lc.Time.Before(now) {
				return Decision{Rung: RungLastCompleted, Suppressed: true}, nil
			}
		}
	}

	// Rung 4: a queued @skip_count suppresses and decrements by one.
	if v, ok := t.Values["skip_count"]; ok && v.Int > 0 {
		decremented := v
		decremented.Int = v.Int - 1
		decremented.Raw = decremented.Format()
		return Decision{
			Rung:       RungSkipCount,
			Suppressed: true,
			WriteBack: &WriteBackRequest{
				Task:      t,
				Directive: "skip_count",
				Value:     decremented,
			},
		}, nil
	}

	// Rung 5: nothing suppressed — dispatch.
	return Decision{Rung: RungDispatch, Suppressed: false}, nil
}
