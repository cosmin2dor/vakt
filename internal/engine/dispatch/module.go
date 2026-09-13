package dispatch

import (
	"context"
	"time"
)

// TaskContext is the minimal, read-only view of a task a Module needs to
// dispatch it — not the API's Task DTO. State is a plain string until
// internal/model's generated type exists.
type TaskContext struct {
	ID          string
	Title       string
	FilePath    string
	State       string
	TriggeredAt time.Time
}

// Outcome is what a Module reports back after a dispatch attempt.
// Accepted means the provider accepted the message, not that it was
// delivered (SDD.md §3). Retry/pruning policy (G13/G14) consumes this
// later; this is only its shape.
type Outcome struct {
	Accepted bool
	Detail   string
}

// Module dispatches a task's payload to an external provider. It gets no
// handle onto the vault or filesystem — nothing here can write back.
type Module interface {
	Name() string
	Dispatch(ctx context.Context, task TaskContext, payload string) (Outcome, error)
}
