package schedule

import (
	"testing"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/stretchr/testify/require"
)

// schedVal is a fixed daily 09:00 cron @schedule, so PrevFireValue has a
// deterministic previous fire point to test G8's window against.
func schedVal() directive.Value {
	return directive.Value{Type: directive.TypeCron, Cron: []string{"0", "9", "*", "*", "*"}}
}

func stateVal(s string) directive.Value { return directive.Value{Type: directive.TypeEnum, Str: s} }
func timeVal(t time.Time) directive.Value {
	return directive.Value{Type: directive.TypeDatetime, Time: t}
}
func intVal(n int64) directive.Value { return directive.Value{Type: directive.TypeInteger, Int: n} }

func TestEvaluate_SuppressionMatrix(t *testing.T) {
	// now is today's 09:00 fire; the previous fire (per schedVal) is yesterday 09:00.
	now := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
	prev := time.Date(2024, 6, 14, 9, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := prev.Add(time.Hour) // inside the [prev, now) window

	base := func(values map[string]directive.Value) index.Task {
		v := map[string]directive.Value{"schedule": schedVal(), "state": stateVal("active")}
		for k, val := range values {
			v[k] = val
		}
		return index.Task{ID: "t", Values: v}
	}

	tests := []struct {
		name      string
		task      index.Task
		wantRung  Rung
		wantSup   bool
		wantWBInt *int64 // expected decremented skip_count, if a writeback is expected
	}{
		{
			name:     "paused state suppresses before anything else is checked",
			task:     base(map[string]directive.Value{"state": stateVal("paused")}),
			wantRung: RungState, wantSup: true,
		},
		{
			name:     "failed state suppresses",
			task:     base(map[string]directive.Value{"state": stateVal("failed")}),
			wantRung: RungState, wantSup: true,
		},
		{
			name:     "triggered state is allowed to reach later rungs",
			task:     base(map[string]directive.Value{"state": stateVal("triggered")}),
			wantRung: RungDispatch, wantSup: false,
		},
		{
			name:     "future skip_until suppresses",
			task:     base(map[string]directive.Value{"skip_until": timeVal(future)}),
			wantRung: RungSkipUntil, wantSup: true,
		},
		{
			name:     "past skip_until does not suppress",
			task:     base(map[string]directive.Value{"skip_until": timeVal(now.Add(-time.Hour))}),
			wantRung: RungDispatch, wantSup: false,
		},
		{
			name: "future skip_until suppresses BEFORE skip_count is ever looked at (rung 2 before rung 4)",
			task: base(map[string]directive.Value{
				"skip_until": timeVal(future),
				"skip_count": intVal(3),
			}),
			wantRung: RungSkipUntil, wantSup: true, wantWBInt: nil,
		},
		{
			name:     "last_completed inside the window suppresses",
			task:     base(map[string]directive.Value{"last_completed": timeVal(past)}),
			wantRung: RungLastCompleted, wantSup: true,
		},
		{
			name:     "last_completed at the window's inclusive lower bound suppresses",
			task:     base(map[string]directive.Value{"last_completed": timeVal(prev)}),
			wantRung: RungLastCompleted, wantSup: true,
		},
		{
			name:     "last_completed equal to now (exclusive upper bound) does not suppress via rung 3",
			task:     base(map[string]directive.Value{"last_completed": timeVal(now)}),
			wantRung: RungDispatch, wantSup: false,
		},
		{
			name:     "last_completed before the window does not suppress",
			task:     base(map[string]directive.Value{"last_completed": timeVal(prev.Add(-time.Hour))}),
			wantRung: RungDispatch, wantSup: false,
		},
		{
			name: "last_completed in-window suppresses BEFORE skip_count is looked at (rung 3 before rung 4)",
			task: base(map[string]directive.Value{
				"last_completed": timeVal(past),
				"skip_count":     intVal(2),
			}),
			wantRung: RungLastCompleted, wantSup: true, wantWBInt: nil,
		},
		{
			name:     "positive skip_count suppresses and decrements",
			task:     base(map[string]directive.Value{"skip_count": intVal(1)}),
			wantRung: RungSkipCount, wantSup: true, wantWBInt: ptr(int64(0)),
		},
		{
			name:     "skip_count above one decrements by exactly one",
			task:     base(map[string]directive.Value{"skip_count": intVal(3)}),
			wantRung: RungSkipCount, wantSup: true, wantWBInt: ptr(int64(2)),
		},
		{
			name:     "zero skip_count does not suppress",
			task:     base(map[string]directive.Value{"skip_count": intVal(0)}),
			wantRung: RungDispatch, wantSup: false,
		},
		{
			name:     "no suppressing directives at all dispatches",
			task:     base(nil),
			wantRung: RungDispatch, wantSup: false,
		},
		{
			name: "all four suppressing conditions present: state wins (rung 1 before everything)",
			task: base(map[string]directive.Value{
				"state":          stateVal("completed"),
				"skip_until":     timeVal(future),
				"last_completed": timeVal(past),
				"skip_count":     intVal(5),
			}),
			wantRung: RungState, wantSup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.task, now)
			require.NoError(t, err)
			require.Equal(t, tt.wantRung, got.Rung)
			require.Equal(t, tt.wantSup, got.Suppressed)
			if tt.wantWBInt == nil {
				require.Nil(t, got.WriteBack)
				return
			}
			require.NotNil(t, got.WriteBack)
			require.Equal(t, "skip_count", got.WriteBack.Directive)
			require.Equal(t, *tt.wantWBInt, got.WriteBack.Value.Int)
		})
	}
}

func ptr[T any](v T) *T { return &v }
