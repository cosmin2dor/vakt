package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine"
	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/engine/schedule"
	"github.com/cosmin2dor/vakt/internal/model"
)

// ListTasksHandler returns every indexed task, unfiltered and unpaginated
// (SDD.md §1: "a household vault is small, the client filters").
func ListTasksHandler(eng *engine.Engine, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().In(loc) // G9: cron/datetime evaluation happens in the vault's own timezone
		tasks := eng.Index().Tasks()
		out := make([]model.Task, 0, len(tasks)) // [] not null when empty
		for _, t := range tasks {
			out = append(out, buildTaskDTO(t, now))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetTaskHandler resolves one task by id, 404 if the index doesn't know it.
func GetTaskHandler(eng *engine.Engine, loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		t, ok := eng.Index().Task(id)
		if !ok {
			writeError(w, http.StatusNotFound, "task_not_found", fmt.Sprintf("no task with id %q is known", id))
			return
		}
		writeJSON(w, http.StatusOK, buildTaskDTO(t, time.Now().In(loc))) // G9: vault-local evaluation
	}
}

// buildTaskDTO computes every derived field of the Task DTO from an index
// entry plus "now" (CLAUDE.md: the backend is the only parser — nothing
// here hands the client raw directive text to interpret).
func buildTaskDTO(t index.Task, now time.Time) model.Task {
	dto := model.Task{
		Id:       t.ID,
		Title:    taskTitle(t.Raw),
		FilePath: t.Path,
		State:    model.TaskState(t.Values["state"].Str),
	}

	if v, ok := t.Values["target"]; ok {
		s := v.Str
		dto.Target = &s
	}
	if v, ok := t.Values["reason"]; ok {
		s := v.Str
		dto.Reason = &s
	}
	if v, ok := t.Values["last_triggered"]; ok {
		tm := v.Time
		dto.LastTriggered = &tm
	}
	if v, ok := t.Values["last_completed"]; ok {
		tm := v.Time
		dto.LastCompleted = &tm
	}

	sv, hasSchedule := scheduleValue(t)
	if hasSchedule {
		if nf, ok, err := schedule.NextFireValue(sv, now); err == nil && ok {
			dto.NextFire = &nf
		}
		summary := summarizeSchedule(sv)
		dto.ScheduleSummary = &summary
	}

	dto.EffectiveSuppression = model.EffectiveSuppression{Suppressed: false}
	if d, err := schedule.Evaluate(t, now); err == nil {
		dto.EffectiveSuppression = suppressionDTO(d)
	}

	return dto
}

// scheduleValue returns whichever of @schedule/@once is present — a task
// has at most one (PRD.md §3.2).
func scheduleValue(t index.Task) (directive.Value, bool) {
	if v, ok := t.Values["schedule"]; ok {
		return v, true
	}
	if v, ok := t.Values["once"]; ok {
		return v, true
	}
	return directive.Value{}, false
}

// suppressionDTO maps a suppress.Decision to the DTO's collapsed shape.
// RungDispatch (not suppressed) carries no reason.
func suppressionDTO(d schedule.Decision) model.EffectiveSuppression {
	if !d.Suppressed {
		return model.EffectiveSuppression{Suppressed: false}
	}
	var reason model.EffectiveSuppressionReason
	switch d.Rung {
	case schedule.RungState:
		reason = model.StateNotActive
	case schedule.RungSkipUntil:
		reason = model.SkipUntil
	case schedule.RungLastCompleted:
		reason = model.EarlyCompletion
	case schedule.RungSkipCount:
		reason = model.SkipCount
	}
	return model.EffectiveSuppression{Suppressed: true, Reason: &reason}
}

// checkboxRe strips a Markdown task checkbox prefix ("- [ ] "/"- [x] ").
var checkboxRe = regexp.MustCompile(`^-\s*\[[ xX]\]\s*`)

// taskTitle is the human text of a task line: everything before its first
// @directive, with the checkbox syntax and surrounding whitespace trimmed.
func taskTitle(raw string) string {
	s := raw
	if spans := directive.Lex(raw); len(spans) > 0 {
		s = raw[:spans[0].Start]
	}
	s = checkboxRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// summarizeSchedule renders @schedule/@once as human prose — never the raw
// cron expression or ISO timestamp (schema/openapi.yaml: Task.schedule_summary).
func summarizeSchedule(v directive.Value) string {
	switch v.Type {
	case directive.TypeDatetime:
		return "Once, " + v.Time.Format("Jan 2 2006 15:04")
	case directive.TypeCron:
		return summarizeCron(v.Cron)
	default:
		return v.Raw
	}
}

var dowNames = map[string]string{
	"0": "Sunday", "1": "Monday", "2": "Tuesday", "3": "Wednesday",
	"4": "Thursday", "5": "Friday", "6": "Saturday", "7": "Sunday",
}

// summarizeCron produces readable prose for common shapes (daily/weekly at
// a fixed time) and a plain field-by-field description otherwise — not
// exhaustive cron prose, just never the raw expression verbatim.
func summarizeCron(fields []string) string {
	if len(fields) != 5 {
		return strings.Join(fields, " ")
	}
	minute, hour, dom, month, dow := fields[0], fields[1], fields[2], fields[3], fields[4]

	m, err1 := strconv.Atoi(minute)
	h, err2 := strconv.Atoi(hour)
	fixedTime := err1 == nil && err2 == nil

	if fixedTime && dom == "*" && month == "*" && dow == "*" {
		return fmt.Sprintf("Daily at %02d:%02d", h, m)
	}
	if fixedTime && dom == "*" && month == "*" {
		if name, ok := dowNames[dow]; ok {
			return fmt.Sprintf("Weekly on %s at %02d:%02d", name, h, m)
		}
	}
	return fmt.Sprintf("At minute %s, hour %s, day %s, month %s, weekday %s", minute, hour, dom, month, dow)
}
