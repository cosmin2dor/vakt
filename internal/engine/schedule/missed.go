package schedule

import (
	"errors"
	"fmt"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
)

// MissedTrigger is one task flagged by G10/G11, with the @reason to stamp.
type MissedTrigger struct {
	Task   index.Task
	Reason string
}

// DetectMissed scans tasks for G10 (a @schedule fire that elapsed,
// unrecorded, while the daemon wasn't running) and G11 (a @once already in
// the past on first parse, never triggered). Pure and side-effect free —
// callable once at startup without touching the vault or the live
// scheduler; no catch-up is computed, only that at least one fire was
// missed.
func DetectMissed(tasks map[string]index.Task, now time.Time) ([]MissedTrigger, error) {
	var missed []MissedTrigger
	for _, t := range tasks {
		m, ok, err := detectOne(t, now)
		if err != nil {
			return nil, fmt.Errorf("missed-trigger: task %q: %w", t.ID, err)
		}
		if ok {
			missed = append(missed, m)
		}
	}
	return missed, nil
}

func detectOne(t index.Task, now time.Time) (MissedTrigger, bool, error) {
	if once, ok := t.Values["once"]; ok {
		m, ok := detectOnce(t, once, now)
		return m, ok, nil
	}
	if sv, ok := t.Values["schedule"]; ok {
		return detectSchedule(t, sv, now)
	}
	return MissedTrigger{}, false, nil
}

// detectOnce implements G11. scheduleLocked already refuses to arm a past
// @once, so this only detects and records it; it never fires.
func detectOnce(t index.Task, once directive.Value, now time.Time) (MissedTrigger, bool) {
	if once.Time.After(now) {
		return MissedTrigger{}, false
	}
	if _, ok := t.Values["last_triggered"]; ok {
		return MissedTrigger{}, false // already fired successfully before
	}
	return MissedTrigger{
		Task:   t,
		Reason: fmt.Sprintf("missed trigger: @once due %s before first parse, not fired", once.Time.Format(time.RFC3339)),
	}, true
}

// detectSchedule implements G10. PrevFireValue(sv, now) is inclusive of now
// (G8), so a schedule due at this exact instant returns prev == now — the
// ordinary "about to fire, nothing missed yet" case, left for the live
// scheduler. Only prev strictly before now means an occurrence has already
// elapsed unseen. suppress.go's rung-3 nanosecond-stepback idiom doesn't
// transfer directly here: that rung runs exactly at a live dispatch instant
// and wants the fire *before* it, whereas here now is an arbitrary moment
// (startup) that may or may not itself be a fire point — testing
// prev.Before(now) on the plain, inclusive PrevFireValue already answers
// "was this instant already consumed" without needing to step back.
func detectSchedule(t index.Task, sv directive.Value, now time.Time) (MissedTrigger, bool, error) {
	prev, ok, err := PrevFireValue(sv, now)
	if err != nil {
		return MissedTrigger{}, false, err
	}
	if !ok || !prev.Before(now) {
		return MissedTrigger{}, false, nil // never due yet, or due right now
	}
	if lt, ok := t.Values["last_triggered"]; ok && !prev.After(lt.Time) {
		return MissedTrigger{}, false, nil // that fire is already recorded
	}
	return MissedTrigger{
		Task:   t,
		Reason: fmt.Sprintf("missed trigger: schedule due %s, daemon was not running", prev.Format(time.RFC3339)),
	}, true, nil
}

// RecordMissedTriggers detects, then stamps @reason for, every missed
// trigger in tasks. It only records — it never dispatches or fires
// anything; G10/G11 are entirely about not firing and saying why.
//
// @reason is user-authored (schema/directives.yaml: system_written: false),
// so stamping it risks overwriting a human's own note — the exact tension
// this task calls out. This overwrites rather than appends or storing the
// reason elsewhere: schema/directives.yaml already treats @state and
// @skip_count the same way, as fields a human may annotate but the engine
// owns at runtime, and there is no delimiter convention anywhere in the
// schema for safely appending to a single-line value. An accurate record of
// what the engine just did is worth more than a stale note about something
// else.
func (o *Orchestrator) RecordMissedTriggers(tasks map[string]index.Task, now time.Time) ([]MissedTrigger, error) {
	missed, err := DetectMissed(tasks, now)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, m := range missed {
		v := directive.Value{Type: directive.TypeString, Str: m.Reason}
		if err := o.StampDirective(m.Task, "reason", v); err != nil {
			errs = append(errs, fmt.Errorf("task %q: %w", m.Task.ID, err))
		}
	}
	return missed, errors.Join(errs...)
}
