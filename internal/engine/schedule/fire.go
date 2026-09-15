package schedule

import (
	"fmt"
	"strings"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/robfig/cron/v3"
)

// lookbackWindows are tried in increasing size when hunting for a previous
// fire: cheap for frequent schedules, still correct for a once-a-year one.
var lookbackWindows = []time.Duration{
	24 * time.Hour,
	7 * 24 * time.Hour,
	31 * 24 * time.Hour,
	366 * 24 * time.Hour,
	5 * 366 * 24 * time.Hour,
}

// PrevFire returns the latest fire time at-or-before after (G8's inclusive
// lower bound). robfig/cron only computes forward, so this walks Next()
// forward from a lookback point until it would pass after, escalating the
// window until a fire is found or the largest window is exhausted.
func PrevFire(expr string, after time.Time) (time.Time, bool, error) {
	sched, err := parser.Parse(expr)
	if err != nil {
		return time.Time{}, false, err
	}
	for _, window := range lookbackWindows {
		if prev, ok := latestFireInRange(sched, after.Add(-window), after); ok {
			return prev, true, nil
		}
	}
	return time.Time{}, false, nil
}

// latestFireInRange returns the last fire in (start, end], or ok=false if
// none — including robfig/cron's own "never fires" signal, a zero time.
func latestFireInRange(sched cron.Schedule, start, end time.Time) (time.Time, bool) {
	var last time.Time
	found := false
	for t := sched.Next(start); !t.IsZero() && !t.After(end); t = sched.Next(t) {
		last, found = t, true
	}
	return last, found
}

// NextFireValue returns v's next fire strictly after after, per its Type.
// A @once (TypeDatetime) fires exactly once and only if still ahead.
func NextFireValue(v directive.Value, after time.Time) (time.Time, bool, error) {
	switch v.Type {
	case directive.TypeCron:
		t, err := NextFire(cronExpr(v), after)
		if err != nil {
			return time.Time{}, false, err
		}
		return t, true, nil
	case directive.TypeDatetime:
		if v.Time.After(after) {
			return v.Time, true, nil
		}
		return time.Time{}, false, nil
	default:
		return time.Time{}, false, fmt.Errorf("schedule: value type %q has no fire semantics", v.Type)
	}
}

// PrevFireValue returns v's previous fire at-or-before after (G8). A @once's
// previous fire is the timestamp itself once it has already happened.
func PrevFireValue(v directive.Value, after time.Time) (time.Time, bool, error) {
	switch v.Type {
	case directive.TypeCron:
		return PrevFire(cronExpr(v), after)
	case directive.TypeDatetime:
		if !v.Time.After(after) {
			return v.Time, true, nil
		}
		return time.Time{}, false, nil
	default:
		return time.Time{}, false, fmt.Errorf("schedule: value type %q has no fire semantics", v.Type)
	}
}

func cronExpr(v directive.Value) string {
	return strings.Join(v.Cron, " ")
}
