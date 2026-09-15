package dispatch

import (
	"encoding/json"
	"testing"
	"time"
)

func testTask() TaskContext {
	return TaskContext{
		ID:          "water-plants",
		Title:       "Water plants",
		FilePath:    "Home/Chores.md",
		State:       "triggered",
		TriggeredAt: time.Date(2026, 9, 15, 8, 30, 0, 0, time.UTC),
	}
}

func TestInterpolateTemplate_KnownVariables(t *testing.T) {
	task := testTask()
	cases := map[string]string{
		"{{title}}":        "Water plants",
		"{{id}}":           "water-plants",
		"{{file_path}}":    "Home/Chores.md",
		"{{triggered_at}}": "2026-09-15T08:30:00Z",
		"{{state}}":        "triggered",
	}
	for tmpl, want := range cases {
		got, diags := InterpolateTemplate(tmpl, task)
		if got != want {
			t.Errorf("%s => %q, want %q", tmpl, got, want)
		}
		if len(diags) != 0 {
			t.Errorf("%s produced diagnostics: %v", tmpl, diags)
		}
	}
}

func TestInterpolateTemplate_MultipleKnownVariables(t *testing.T) {
	task := testTask()
	got, diags := InterpolateTemplate(`{"title":"{{title}}","id":"{{id}}","state":"{{state}}"}`, task)
	want := `{"title":"Water plants","id":"water-plants","state":"triggered"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics: %v", diags)
	}
}

func TestInterpolateTemplate_UnknownVariable(t *testing.T) {
	task := testTask()
	got, diags := InterpolateTemplate("Reminder: {{unknown_thing}}", task)
	if got != "Reminder: {{unknown_thing}}" {
		t.Errorf("unknown variable was not left literal: %q", got)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}
	d := diags[0]
	if d.Severity != "warning" {
		t.Errorf("Severity = %q, want warning", d.Severity)
	}
	if d.Code == nil || *d.Code != CodeUnknownPayloadVariable {
		t.Errorf("Code = %v, want %q", d.Code, CodeUnknownPayloadVariable)
	}
	if d.TaskId != task.ID {
		t.Errorf("TaskId = %q, want %q", d.TaskId, task.ID)
	}
	if d.FilePath != task.FilePath {
		t.Errorf("FilePath = %q, want %q", d.FilePath, task.FilePath)
	}
}

func TestInterpolateTemplate_MixedKnownAndUnknown(t *testing.T) {
	task := testTask()
	got, diags := InterpolateTemplate("{{title}} is due, see {{nonsense}}", task)
	want := "Water plants is due, see {{nonsense}}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}
}

func TestInterpolateTemplate_RepeatedUnknownVariable(t *testing.T) {
	// One diagnostic per occurrence, not per distinct name.
	task := testTask()
	_, diags := InterpolateTemplate("{{oops}} and {{oops}} again", task)
	if len(diags) != 2 {
		t.Fatalf("got %d diagnostics, want 2", len(diags))
	}
}

func TestInterpolateTemplate_UnterminatedBraces(t *testing.T) {
	task := testTask()
	got, diags := InterpolateTemplate("Reminder: {{title", task)
	if got != "Reminder: {{title" {
		t.Errorf("unterminated span was altered: %q", got)
	}
	if len(diags) != 0 {
		t.Errorf("unterminated span should not produce a diagnostic: %v", diags)
	}
}

func TestBuildPayload_NoOverride(t *testing.T) {
	task := testTask()
	got, diags, err := BuildPayload(task, "", false)
	if err != nil {
		t.Fatalf("BuildPayload returned error: %v", err)
	}
	if len(diags) != 0 {
		t.Errorf("unexpected diagnostics: %v", diags)
	}
	want, err := DefaultPayload(task)
	if err != nil {
		t.Fatalf("DefaultPayload returned error: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	var body payloadBody
	if err := json.Unmarshal([]byte(got), &body); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

func TestBuildPayload_WithOverride(t *testing.T) {
	task := testTask()
	got, diags, err := BuildPayload(task, `{"title":"{{title}}","x":"{{bogus}}"}`, true)
	if err != nil {
		t.Fatalf("BuildPayload returned error: %v", err)
	}
	want := `{"title":"Water plants","x":"{{bogus}}"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}
}
