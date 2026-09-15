package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine"
	"github.com/cosmin2dor/vakt/internal/model"
)

// diagKey identifies one diagnostic across snapshots for raised/cleared
// diffing — file, line, task, and code together are stable enough to spot
// "the same problem" reappearing without a persistent diagnostic id.
type diagKey struct {
	FilePath string
	Line     int
	TaskId   string
	Code     string
}

func keyOf(d model.Diagnostic) diagKey {
	code := ""
	if d.Code != nil {
		code = *d.Code
	}
	return diagKey{FilePath: d.FilePath, Line: d.Line, TaskId: string(d.TaskId), Code: code}
}

func diagSnapshot(diags []model.Diagnostic) map[diagKey]model.Diagnostic {
	m := make(map[diagKey]model.Diagnostic, len(diags))
	for _, d := range diags {
		m[keyOf(d)] = d
	}
	return m
}

// StreamEventsHandler serves GET /api/v1/events (FR-2.2): an SSE stream
// translating eng's index.Change bus into EventEnvelopes for every
// connected client. heartbeatInterval is a constructor param so tests can
// use a short one; server.go wires a production value.
//
// index.Change carries no diagnostics, so diagnostic_raised/cleared are
// derived by diffing the index's whole Diagnostics() snapshot before and
// after each change — vault-wide rather than scoped to the changed file,
// but a household vault's diagnostic list is small and this stays honest
// about raised-vs-cleared rather than re-announcing everything each time.
func StreamEventsHandler(eng *engine.Engine, loc *time.Location, heartbeatInterval time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming_unsupported", "server does not support streaming")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		sub := eng.Subscribe(32)
		defer sub.Unsubscribe()

		// Seed from the current snapshot so pre-existing diagnostics aren't
		// announced as newly "raised" the moment a client connects.
		lastDiags := diagSnapshot(eng.Index().Diagnostics())

		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case change, ok := <-sub.C():
				if !ok {
					return
				}
				now := time.Now().In(loc)
				for _, t := range change.Added {
					dto := buildTaskDTO(t, now)
					if err := writeEnvelope(w, flusher, model.EventEnvelope{Type: model.TaskUpserted, Task: &dto, Timestamp: now}); err != nil {
						return
					}
				}
				for _, t := range change.Updated {
					dto := buildTaskDTO(t, now)
					if err := writeEnvelope(w, flusher, model.EventEnvelope{Type: model.TaskUpserted, Task: &dto, Timestamp: now}); err != nil {
						return
					}
				}
				for _, id := range change.Removed {
					tid := model.TaskId(id)
					if err := writeEnvelope(w, flusher, model.EventEnvelope{Type: model.TaskRemoved, TaskId: &tid, Timestamp: now}); err != nil {
						return
					}
				}

				newDiags := diagSnapshot(eng.Index().Diagnostics())
				for k, d := range newDiags {
					if _, existed := lastDiags[k]; !existed {
						d := d
						if err := writeEnvelope(w, flusher, model.EventEnvelope{Type: model.DiagnosticRaised, Diagnostic: &d, Timestamp: now}); err != nil {
							return
						}
					}
				}
				for k, d := range lastDiags {
					if _, still := newDiags[k]; !still {
						d := d
						if err := writeEnvelope(w, flusher, model.EventEnvelope{Type: model.DiagnosticCleared, Diagnostic: &d, Timestamp: now}); err != nil {
							return
						}
					}
				}
				lastDiags = newDiags
			case <-ticker.C:
				if err := writeEnvelope(w, flusher, model.EventEnvelope{Type: model.Heartbeat, Timestamp: time.Now().In(loc)}); err != nil {
					return
				}
			}
		}
	}
}

// writeEnvelope marshals one EventEnvelope as an SSE `data:` line and
// flushes immediately. A marshal failure is skipped rather than killing
// the whole stream over one bad event; a write failure (client gone) is
// reported so the caller stops trying to use this connection.
func writeEnvelope(w http.ResponseWriter, flusher http.Flusher, env model.EventEnvelope) error {
	b, err := json.Marshal(env)
	if err != nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
