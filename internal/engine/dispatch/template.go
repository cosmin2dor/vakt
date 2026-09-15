package dispatch

import (
	"strings"
	"time"

	"github.com/cosmin2dor/vakt/internal/model"
)

// CodeUnknownPayloadVariable is emitted for each unrecognized {{var}} span
// in an @payload override (SDD.md G12).
const CodeUnknownPayloadVariable = "unknown_payload_variable"

// knownVars maps a {{name}} to its substitution, per SDD.md G12.
func knownVars(task TaskContext) map[string]string {
	return map[string]string{
		"title":        task.Title,
		"id":           task.ID,
		"file_path":    task.FilePath,
		"triggered_at": task.TriggeredAt.Format(time.RFC3339),
		"state":        task.State,
	}
}

// InterpolateTemplate substitutes G12's known {{variables}} into a
// user-authored @payload override. An unrecognized variable is left
// literal in the output and produces one warning diagnostic per
// occurrence — it never fails the interpolation.
func InterpolateTemplate(override string, task TaskContext) (string, []model.Diagnostic) {
	vars := knownVars(task)
	var diags []model.Diagnostic

	var out strings.Builder
	rest := override
	for {
		start := strings.Index(rest, "{{")
		if start == -1 {
			out.WriteString(rest)
			break
		}
		out.WriteString(rest[:start])

		end := strings.Index(rest[start:], "}}")
		if end == -1 {
			// Unterminated {{ — no variable name to resolve, leave as-is.
			out.WriteString(rest[start:])
			break
		}
		end += start
		name := strings.TrimSpace(rest[start+2 : end])
		span := rest[start : end+2]

		if val, ok := vars[name]; ok {
			out.WriteString(val)
		} else {
			out.WriteString(span)
			diags = append(diags, model.Diagnostic{
				Code:     strPtr(CodeUnknownPayloadVariable),
				FilePath: task.FilePath,
				Line:     0, // dispatch-time; TaskContext carries no source line
				Message:  "unknown payload variable " + span,
				Severity: model.DiagnosticSeverityWarning,
				TaskId:   task.ID,
			})
		}

		rest = rest[end+2:]
	}

	return out.String(), diags
}

// BuildPayload picks the default payload or an @payload override,
// templating the latter. hasOverride distinguishes "no @payload" from
// an empty-but-present override, which DefaultPayload can't do alone.
func BuildPayload(task TaskContext, override string, hasOverride bool) (string, []model.Diagnostic, error) {
	if !hasOverride {
		body, err := DefaultPayload(task)
		return body, nil, err
	}
	body, diags := InterpolateTemplate(override, task)
	return body, diags, nil
}

func strPtr(s string) *string { return &s }
