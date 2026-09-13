package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/dispatch"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
	"github.com/cosmin2dor/vakt/internal/model"
)

// TriggerHandler resolves a task by id and dispatches it through registry.
// It never writes to the vault — no @last_triggered stamp, no touching the
// file beyond the read Walk already does (M2's surgical patcher owns
// writeback).
func TriggerHandler(vaultRoot string, registry *dispatch.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		tasks, _, err := vault.Walk(vaultRoot)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to walk vault: "+err.Error())
			return
		}

		task, ok := tasks[id]
		if !ok {
			writeError(w, http.StatusNotFound, "task_not_found", fmt.Sprintf("no task with id %q is known", id))
			return
		}

		// The walker only extracts @id; find @target directly from the
		// task's raw line since vault.Task doesn't carry it (yet).
		target, hasTarget := firstTarget(task.Raw)

		now := time.Now().UTC()
		outcome := model.TriggerOutcome{
			TaskId:       task.ID,
			Target:       target,
			DispatchedAt: now,
		}

		if !hasTarget {
			detail := "task has no @target directive; nothing to dispatch to"
			outcome.Accepted = false
			outcome.Detail = &detail
			writeJSON(w, http.StatusOK, outcome)
			return
		}

		// TaskContext fields beyond ID/FilePath have no source in the
		// walker's minimal vault.Task yet — Title mirrors ID and State
		// is a placeholder "active" until M2's typed directive parsing
		// exists. Payload templating is M3; an empty payload stands in
		// for it here.
		taskCtx := dispatch.TaskContext{
			ID:          task.ID,
			Title:       task.ID,
			FilePath:    task.Path,
			State:       "active",
			TriggeredAt: now,
		}

		dispatchOutcome, err := registry.Dispatch(r.Context(), target, taskCtx, "")
		if err != nil {
			detail := err.Error()
			outcome.Accepted = false
			outcome.Detail = &detail
			writeJSON(w, http.StatusOK, outcome)
			return
		}

		outcome.Accepted = dispatchOutcome.Accepted
		if dispatchOutcome.Detail != "" {
			detail := dispatchOutcome.Detail
			outcome.Detail = &detail
		}
		writeJSON(w, http.StatusOK, outcome)
	}
}

// firstTarget returns the value of the first @target(...) span on line, if
// any.
func firstTarget(line string) (string, bool) {
	for _, span := range directive.Lex(line) {
		if span.Name == "target" {
			return span.Value, true
		}
	}
	return "", false
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError writes the generated Error shape (schema/openapi.yaml
// components.schemas.Error).
func writeError(w http.ResponseWriter, status int, code, message string) {
	errBody := model.Error{}
	errBody.Error.Code = code
	errBody.Error.Message = message
	writeJSON(w, status, errBody)
}
