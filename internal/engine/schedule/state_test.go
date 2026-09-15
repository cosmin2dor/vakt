package schedule

import "testing"

func TestAdvance(t *testing.T) {
	cases := []struct {
		name    string
		current State
		ev      Event
		once    bool
		want    State
		wantOK  bool
	}{
		// active -> triggered on dispatch.
		{"dispatch from active", StateActive, EventDispatched, false, StateTriggered, true},

		// G6: both paths out of triggered land on active.
		{"G6 timeout path", StateTriggered, EventScheduledSlotArrived, false, StateActive, true},
		{"G6 fulfillment path (@schedule)", StateTriggered, EventFulfilled, false, StateActive, true},

		// @once fulfillment completes, from either active or triggered.
		{"@once fulfilled from active", StateActive, EventFulfilled, true, StateCompleted, true},
		{"@once fulfilled from triggered", StateTriggered, EventFulfilled, true, StateCompleted, true},

		// @schedule fulfillment from active stays active (no-op).
		{"@schedule fulfilled from active", StateActive, EventFulfilled, false, StateActive, true},

		// Pause from active and triggered; resume from paused.
		{"pause from active", StateActive, EventPaused, false, StatePaused, true},
		{"pause from triggered", StateTriggered, EventPaused, false, StatePaused, true},
		{"resume from paused", StatePaused, EventResumed, false, StateActive, true},

		// Delivery failure and manual clear.
		{"delivery failed from active", StateActive, EventDeliveryFailed, false, StateFailed, true},
		{"delivery failed from triggered", StateTriggered, EventDeliveryFailed, false, StateFailed, true},
		{"clear from failed", StateFailed, EventCleared, false, StateActive, true},

		// completed is terminal: no event moves it.
		{"completed accepts nothing", StateCompleted, EventFulfilled, false, StateCompleted, false},

		// Nonsensical (state, event) pairs are no-ops signaled via ok=false.
		{"resume on already-active", StateActive, EventResumed, false, StateActive, false},
		{"fulfilled on paused", StatePaused, EventFulfilled, false, StatePaused, false},
		{"scheduled slot arrived on active", StateActive, EventScheduledSlotArrived, false, StateActive, false},
		{"cleared on active", StateActive, EventCleared, false, StateActive, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Advance(c.current, c.ev, c.once)
			if got != c.want || ok != c.wantOK {
				t.Errorf("Advance(%s, %v, once=%v) = (%s, %v), want (%s, %v)",
					c.current, c.ev, c.once, got, ok, c.want, c.wantOK)
			}
		})
	}
}
