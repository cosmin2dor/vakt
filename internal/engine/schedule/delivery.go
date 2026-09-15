package schedule

import (
	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/dispatch"
	"github.com/cosmin2dor/vakt/internal/engine/index"
)

// ApplyDeliveryOutcome records the result of a dispatch attempt that has
// already happened. A successful outcome needs no writeback here — the
// active->triggered transition is EventDispatched's concern, handled by
// the caller before dispatch even ran. A failed outcome (SDD G13) moves
// the task to failed and stamps why; there is no retry or backoff.
func (o *Orchestrator) ApplyDeliveryOutcome(task index.Task, outcome dispatch.Outcome) error {
	if outcome.Accepted {
		return nil
	}

	current := State(task.Values["state"].Str)
	next, ok := Advance(current, EventDeliveryFailed, false)
	if !ok {
		return nil
	}

	if err := o.StampState(task, next); err != nil {
		return err
	}
	reason := directive.Value{Type: directive.TypeString, Str: "delivery failed: " + outcome.Detail}
	return o.StampDirective(task, "reason", reason)
}

// ClearFailure is the manual-clear path (SDD G13): a failed task moves
// back to active. No UI/API calls this yet; it exists to be wired in
// later.
func (o *Orchestrator) ClearFailure(task index.Task) error {
	current := State(task.Values["state"].Str)
	next, ok := Advance(current, EventCleared, false)
	if !ok {
		return nil
	}
	return o.StampState(task, next)
}
