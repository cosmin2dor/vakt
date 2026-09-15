package schedule

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
	"github.com/cosmin2dor/vakt/internal/model"
)

// ErrFileInvalid is returned by StampDirective when the target file already
// fails to parse (SDD.md G15): the write is skipped entirely, and the
// index's last known-good view of the task stays live instead.
var ErrFileInvalid = errors.New("writeback: file fails validation; write skipped (G15)")

// Orchestrator routes every task state change through the surgical patcher
// (directive.PatchDirective/SplicePatch) and the serialized vault.Writer.
type Orchestrator struct {
	writer *vault.Writer
	root   string
	loc    *time.Location
}

// NewOrchestrator returns an Orchestrator writing under root (the same
// vault root a task's index was built with) via w, validating files in loc.
func NewOrchestrator(w *vault.Writer, root string, loc *time.Location) *Orchestrator {
	if loc == nil {
		loc = time.Local
	}
	return &Orchestrator{writer: w, root: root, loc: loc}
}

// StampDirective patches task's line to carry value under name, and
// publishes the whole updated file through the writer. Per G15, if the
// file's current on-disk content already fails validation, nothing is
// written and ErrFileInvalid is returned.
func (o *Orchestrator) StampDirective(task index.Task, name string, value directive.Value) error {
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

	patch, err := directive.PatchDirective(string(data[start:end]), name, value)
	if err != nil {
		return fmt.Errorf("writeback: computing patch for @%s: %w", name, err)
	}

	newData := directive.SplicePatch(data, start, patch)
	if _, err := o.writer.Write(absPath, newData); err != nil {
		return fmt.Errorf("writeback: writing %q: %w", absPath, err)
	}
	return nil
}

// StampLastTriggered records a dispatch by stamping @last_triggered with now.
func (o *Orchestrator) StampLastTriggered(task index.Task, now time.Time) error {
	return o.StampDirective(task, "last_triggered", directive.Value{Type: directive.TypeDatetime, Time: now})
}

// ApplyWriteBack executes a suppress-ladder WriteBackRequest (e.g. rung 4's
// @skip_count decrement) against the vault.
func (o *Orchestrator) ApplyWriteBack(req WriteBackRequest) error {
	return o.StampDirective(req.Task, req.Directive, req.Value)
}

// StampState records a state-machine transition by stamping @state.
func (o *Orchestrator) StampState(task index.Task, newState State) error {
	return o.StampDirective(task, "state", directive.Value{Type: directive.TypeEnum, Str: string(newState)})
}

// hasErrorDiagnostic reports whether any diagnostic is error-severity;
// warnings (e.g. G5's missing-id) don't block a write.
func hasErrorDiagnostic(diags []model.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == model.DiagnosticSeverityError {
			return true
		}
	}
	return false
}

// lineByteRange finds lineNum's (1-based) absolute [start,end) byte range in
// data by scanning for '\n' directly, never split-and-rejoin, so a CRLF
// file's line endings are never renormalized.
func lineByteRange(data []byte, lineNum int) (start, end int, ok bool) {
	line := 1
	lineStart := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if line == lineNum {
				return lineStart, i + 1, true
			}
			lineStart = i + 1
			line++
		}
	}
	if line == lineNum {
		return lineStart, len(data), true
	}
	return 0, 0, false
}
