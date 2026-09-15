package schedule

// State is a task's @state value (PRD §5.3). Pure data — no I/O.
type State string

const (
	StateActive    State = "active"
	StateTriggered State = "triggered"
	StatePaused    State = "paused"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
)

// Event is something that happened to a task that may move its state.
type Event int

const (
	EventDispatched           Event = iota // dispatch module was just handed the task
	EventFulfilled                         // user marked the task done
	EventScheduledSlotArrived              // G6 timeout: next fire point reached while still triggered
	EventPaused
	EventResumed
	EventDeliveryFailed
	EventCleared // manual clear of a failed state (G13)
)

// Advance computes the resulting state for current, given ev. once
// distinguishes the @once vs @schedule branch of EventFulfilled. ok is false
// when ev has no defined transition from current — callers treat that as a
// no-op, not an error.
func Advance(current State, ev Event, once bool) (next State, ok bool) {
	switch ev {
	case EventDispatched:
		if current == StateActive {
			return StateTriggered, true
		}

	case EventFulfilled:
		// @once tasks complete outright; @schedule tasks return to active,
		// which is a no-op if already active and the G6 fulfillment path
		// if triggered. Paused/completed/failed do not accept fulfillment.
		switch current {
		case StateActive, StateTriggered:
			if once {
				return StateCompleted, true
			}
			return StateActive, true
		}

	case EventScheduledSlotArrived:
		// G6's other path out of triggered: a new cycle began with nobody
		// confirming the previous one.
		if current == StateTriggered {
			return StateActive, true
		}

	case EventPaused:
		if current == StateActive || current == StateTriggered {
			return StatePaused, true
		}

	case EventResumed:
		if current == StatePaused {
			return StateActive, true
		}

	case EventDeliveryFailed:
		// Any dispatch-eligible state can fail delivery (SDD G13).
		if current == StateActive || current == StateTriggered {
			return StateFailed, true
		}

	case EventCleared:
		if current == StateFailed {
			return StateActive, true
		}
	}

	// completed is terminal; every other unmatched (state, event) pair is
	// simply not a defined transition.
	return current, false
}
