package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine"
	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/engine/schedule"
	"github.com/cosmin2dor/vakt/internal/model"
)

// lookupTask resolves id or writes the 404 shape GetTaskHandler uses. ok is
// false when the response has already been written.
func lookupTask(w http.ResponseWriter, eng *engine.Engine, id string) (index.Task, bool) {
	t, found := eng.Index().Task(id)
	if !found {
		writeError(w, http.StatusNotFound, "task_not_found", fmt.Sprintf("no task with id %q is known", id))
		return index.Task{}, false
	}
	return t, true
}

// withUpdatedValue returns a copy of task with name set to value in Values,
// so the response DTO reflects what was just written rather than the
// possibly-stale in-memory task or a re-read racing the watcher's own
// reconciliation.
func withUpdatedValue(task index.Task, name string, value directive.Value) index.Task {
	values := make(map[string]directive.Value, len(task.Values)+1)
	for k, v := range task.Values {
		values[k] = v
	}
	values[name] = value
	task.Values = values
	return task
}

// decodeOptionalJSON decodes r.Body into dst; an empty body is valid (every
// action body here is optional) and is silently treated as "no fields set".
func decodeOptionalJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// FulfillTaskHandler runs M3's Fulfill against task, then responds with the
// updated DTO built from the just-written values (@state, and @last_completed
// for @schedule tasks).
func FulfillTaskHandler(eng *engine.Engine, orch *schedule.Orchestrator, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, ok := lookupTask(w, eng, r.PathValue("id"))
		if !ok {
			return
		}

		now := time.Now().In(loc)
		if err := orch.Fulfill(task, now); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}

		if _, hasOnce := task.Values["once"]; hasOnce {
			task = withUpdatedValue(task, "state", directive.Value{Type: directive.TypeEnum, Str: string(schedule.StateCompleted)})
		} else {
			task = withUpdatedValue(task, "last_completed", directive.Value{Type: directive.TypeDatetime, Time: now})
			if current := schedule.State(task.Values["state"].Str); current == schedule.StateTriggered {
				task = withUpdatedValue(task, "state", directive.Value{Type: directive.TypeEnum, Str: string(schedule.StateActive)})
			}
		}
		writeJSON(w, http.StatusOK, buildTaskDTO(task, now))
	}
}

// PauseTaskHandler transitions an active/triggered task to paused and,
// optionally, records @reason.
func PauseTaskHandler(eng *engine.Engine, orch *schedule.Orchestrator, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, ok := lookupTask(w, eng, r.PathValue("id"))
		if !ok {
			return
		}

		var body model.PauseTaskJSONRequestBody
		if err := decodeOptionalJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		current := schedule.State(task.Values["state"].Str)
		next, transitioned := schedule.Advance(current, schedule.EventPaused, false)
		// Not a defined transition (e.g. already paused/completed): no-op
		// success, since the contract has no distinct error for this.
		if transitioned {
			if err := orch.StampState(task, next); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			task = withUpdatedValue(task, "state", directive.Value{Type: directive.TypeEnum, Str: string(next)})
		}

		if body.Reason != nil && *body.Reason != "" {
			if err := orch.StampDirective(task, "reason", directive.Value{Type: directive.TypeString, Str: *body.Reason}); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			task = withUpdatedValue(task, "reason", directive.Value{Type: directive.TypeString, Str: *body.Reason})
		}

		writeJSON(w, http.StatusOK, buildTaskDTO(task, time.Now().In(loc)))
	}
}

// ResumeTaskHandler transitions a paused task back to active.
func ResumeTaskHandler(eng *engine.Engine, orch *schedule.Orchestrator, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, ok := lookupTask(w, eng, r.PathValue("id"))
		if !ok {
			return
		}

		current := schedule.State(task.Values["state"].Str)
		next, transitioned := schedule.Advance(current, schedule.EventResumed, false)
		if transitioned {
			if err := orch.StampState(task, next); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			task = withUpdatedValue(task, "state", directive.Value{Type: directive.TypeEnum, Str: string(next)})
		}

		writeJSON(w, http.StatusOK, buildTaskDTO(task, time.Now().In(loc)))
	}
}

// SkipTaskHandler increments @skip_count by the request's count (default 1).
func SkipTaskHandler(eng *engine.Engine, orch *schedule.Orchestrator, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, ok := lookupTask(w, eng, r.PathValue("id"))
		if !ok {
			return
		}

		var body model.SkipTaskJSONRequestBody
		if err := decodeOptionalJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		count := int64(1)
		if body.Count != nil {
			count = int64(*body.Count)
		}

		newCount := task.Values["skip_count"].Int + count
		if err := orch.StampDirective(task, "skip_count", directive.Value{Type: directive.TypeInteger, Int: newCount}); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		task = withUpdatedValue(task, "skip_count", directive.Value{Type: directive.TypeInteger, Int: newCount})

		writeJSON(w, http.StatusOK, buildTaskDTO(task, time.Now().In(loc)))
	}
}
