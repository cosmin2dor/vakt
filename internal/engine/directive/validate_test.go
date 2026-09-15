package directive

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cosmin2dor/vakt/internal/model"
)

// TestValidateFile_EachFailureMode is the task's exit bar: one file
// exercising every rule, asserting the exact diagnostic set produced.
func TestValidateFile_EachFailureMode(t *testing.T) {
	content := "" +
		"- [ ] clean task @id(clean_task) @state(active)\n" + // line 1: valid, no diagnostics
		"- [ ] bad id @id(Has Spaces) @state(active)\n" + // line 2: G3
		"- [ ] first claim @id(dupe_id) @state(active)\n" + // line 3: first occurrence, wins
		"- [ ] second claim @id(dupe_id) @state(active)\n" + // line 4: G4, duplicate of line 3
		"- [ ] no id at all @state(active)\n" + // line 5: G5
		"- [ ] mystery @id(mystery_task) @bogus(x)\n" + // line 6: unknown directive
		"- [ ] empty value @id(empty_value_task) @schedule()\n" + // line 7: arity/empty
		"- [ ] bad enum @id(bad_enum_task) @state(sideways)\n" + // line 8: invalid enum
		"plain prose, no directives at all\n" // line 9: not a task line, no diagnostics

	diags := ValidateFile("tasks.md", content, time.UTC)

	want := []model.Diagnostic{
		{
			Code: strPtr(CodeInvalidID), FilePath: "tasks.md", Line: 2,
			Message: "@id value does not match the required pattern", Severity: model.DiagnosticSeverityError,
			TaskId: "Has Spaces",
		},
		{
			Code: strPtr(CodeDuplicateID), FilePath: "tasks.md", Line: 4,
			Message: "@id is already used earlier in this file", Severity: model.DiagnosticSeverityError,
			TaskId: "dupe_id",
		},
		{
			Code: strPtr(CodeMissingID), FilePath: "tasks.md", Line: 5,
			Message: "task line has no @id; it will not be scheduled", Severity: model.DiagnosticSeverityWarning,
			TaskId: "",
		},
		{
			Code: strPtr(CodeUnknownDirective), FilePath: "tasks.md", Line: 6,
			Message: "unrecognized directive @bogus", Severity: model.DiagnosticSeverityWarning,
			TaskId: "mystery_task",
		},
		{
			Code: strPtr(CodeEmptyValue), FilePath: "tasks.md", Line: 7,
			Message: "@schedule has no value", Severity: model.DiagnosticSeverityError,
			TaskId: "empty_value_task",
		},
		{
			Code: strPtr(CodeInvalidEnumValue), FilePath: "tasks.md", Line: 8,
			Message: "@state value is not one of the allowed values", Severity: model.DiagnosticSeverityError,
			TaskId: "bad_enum_task",
		},
	}

	assert.Equal(t, want, diags)
}

func TestValidateFile_CleanFileYieldsNoDiagnostics(t *testing.T) {
	content := "- [ ] one @id(one) @state(active)\n" +
		"- [ ] two @id(two) @state(paused) @schedule(0 8 * * *)\n"

	diags := ValidateFile("clean.md", content, time.UTC)
	assert.Empty(t, diags)
}

func TestValidateFile_DuplicateOrderingIsByLineNumber(t *testing.T) {
	// The later line, by line number, is the one flagged — never the earlier.
	content := "- [ ] a @id(x) @state(active)\n" +
		"- [ ] b @id(x) @state(active)\n" +
		"- [ ] c @id(x) @state(active)\n"

	diags := ValidateFile("order.md", content, time.UTC)

	var dupLines []int
	for _, d := range diags {
		if d.Code != nil && *d.Code == CodeDuplicateID {
			dupLines = append(dupLines, d.Line)
		}
	}
	assert.Equal(t, []int{2, 3}, dupLines)
}

func strPtr(s string) *string { return &s }
