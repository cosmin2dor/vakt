package schedule

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
)

// Fulfill records that task's real-world work was done at now (PRD §5.1),
// independent of any dispatch. @once completes outright; @schedule stays
// scheduled but stamps @last_completed, which is what G7 rung 3 later reads
// to bypass one trigger inside G8's [previous fire, upcoming fire) window.
func (o *Orchestrator) Fulfill(task index.Task, now time.Time) error {
	_, hasSchedule := task.Values["schedule"]
	_, hasOnce := task.Values["once"]
	if !hasSchedule && !hasOnce {
		return fmt.Errorf("writeback: task %q has neither @once nor @schedule", task.ID)
	}

	if hasOnce && !hasSchedule {
		return o.fulfillOnce(task)
	}
	return o.fulfillSchedule(task, now)
}

// fulfillOnce checks the checkbox and completes the task. Already-checked
// (a second fulfillment, or a hand-edited file) is a no-op, not an error.
func (o *Orchestrator) fulfillOnce(task index.Task) error {
	if err := o.StampCheckbox(task); err != nil {
		if errors.Is(err, directive.ErrNoUncheckedCheckbox) {
			return nil
		}
		return err
	}
	return o.StampState(task, StateCompleted)
}

// fulfillSchedule stamps @last_completed(now) and, if the task was
// triggered, runs the G6 fulfillment transition back to active. Already-
// active is a no-op transition, so no redundant @state write happens.
func (o *Orchestrator) fulfillSchedule(task index.Task, now time.Time) error {
	if err := o.StampDirective(task, "last_completed", directive.Value{Type: directive.TypeDatetime, Time: now}); err != nil {
		return err
	}

	current := State(task.Values["state"].Str)
	next, ok := Advance(current, EventFulfilled, false)
	if !ok || next == current {
		return nil
	}
	return o.StampState(task, next)
}

// StampCheckbox flips task's line's first unchecked checkbox to "[x]",
// mirroring StampDirective's read-validate-patch-write cycle but for the
// non-directive checkbox span.
func (o *Orchestrator) StampCheckbox(task index.Task) error {
	absPath := filepath.Join(o.root, filepath.FromSlash(task.Path))

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("writeback: reading %q: %w", absPath, err)
	}

	if hasErrorDiagnostic(directive.ValidateFile(task.Path, string(data), o.loc)) {
		return fmt.Errorf("%w: %s", ErrFileInvalid, task.Path)
	}

	start, end, ok := lineByteRange(data, task.Line)
	if !ok {
		return fmt.Errorf("writeback: line %d not found in %q", task.Line, absPath)
	}

	patch, err := directive.PatchCheckbox(string(data[start:end]))
	if err != nil {
		return err
	}

	newData := directive.SplicePatch(data, start, patch)
	if _, err := o.writer.Write(absPath, newData); err != nil {
		return fmt.Errorf("writeback: writing %q: %w", absPath, err)
	}
	return nil
}
