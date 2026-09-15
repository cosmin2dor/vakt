package directive

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/cosmin2dor/vakt/internal/model"
)

// Diagnostic codes, surfaced in model.Diagnostic.Code for the UI/API to
// key off (SDD.md G3, G4, G5).
const (
	CodeInvalidID        = "invalid_id"
	CodeDuplicateID      = "duplicate_id"
	CodeMissingID        = "missing_id"
	CodeUnknownDirective = "unknown_directive"
	CodeEmptyValue       = "empty_value"
	CodeInvalidEnumValue = "invalid_enum_value"
)

// idPatternRe compiles @id's registry pattern (SDD.md G3) once, from the
// generated descriptor rather than a second hardcoded copy.
var idPatternRe = func() *regexp.Regexp {
	desc, ok := Descriptor("id")
	if !ok || desc.Pattern == nil {
		panic("directive registry has no @id descriptor/pattern")
	}
	return regexp.MustCompile(*desc.Pattern)
}()

// ValidateFile runs SDD.md G3/G4/G5 and directive-shape checks over one
// file's content, returning diagnostics in line order. Duplicate-@id
// detection here is file-local only (first-by-line wins); G4's full
// first-by-path-then-line ordering needs a vault-wide view and belongs to
// the walker/index (implement-vault-index), not this validator.
func ValidateFile(path, content string, loc *time.Location) []model.Diagnostic {
	var diags []model.Diagnostic
	seenIDs := make(map[string]int) // valid @id value -> first line number, this file only

	for i, line := range strings.Split(content, "\n") {
		lineNum := i + 1
		spans := Lex(line)
		if len(spans) == 0 {
			continue // no @directive syntax at all: prose, not a task line
		}
		diags = append(diags, validateLine(path, lineNum, spans, seenIDs, loc)...)
	}

	return diags
}

// validateLine checks one line's spans. A line reaching here already has at
// least one @directive span, recognized or not — that is this validator's
// bar for "task line" (broader than the walker's "has @id", since the point
// here is to catch the lines the walker silently skips for lacking one).
func validateLine(path string, lineNum int, spans []Span, seenIDs map[string]int, loc *time.Location) []model.Diagnostic {
	var diags []model.Diagnostic

	idSpan, hasID := firstIDSpan(spans)

	// taskID attributes non-id diagnostics on this line to the task they
	// belong to. A line with an invalid @id still uses its (invalid) raw
	// text — it identifies which line the problem is on even though the
	// value itself failed the pattern check.
	taskID := ""
	if hasID {
		taskID = idSpan.Value
	}

	switch {
	case !hasID:
		// G5: no @id at all. There is no task to attach this diagnostic to
		// by definition — model.Diagnostic.TaskId is a required plain
		// string, so we use "" here deliberately. A consumer must not read
		// an empty TaskId as a real id; this is the one diagnostic kind
		// that can legitimately carry it. Worth revisiting once the API
		// layer actually serves diagnostics (see PR description).
		diags = append(diags, diag(path, lineNum, model.DiagnosticSeverityWarning,
			CodeMissingID, "task line has no @id; it will not be scheduled", ""))

	case !idPatternRe.MatchString(idSpan.Value):
		// G3.
		diags = append(diags, diag(path, lineNum, model.DiagnosticSeverityError,
			CodeInvalidID, "@id value does not match the required pattern", taskID))

	default:
		// G4: first occurrence in this file wins, by line number.
		if _, dup := seenIDs[idSpan.Value]; dup {
			diags = append(diags, diag(path, lineNum, model.DiagnosticSeverityError,
				CodeDuplicateID, "@id is already used earlier in this file", taskID))
		} else {
			seenIDs[idSpan.Value] = lineNum
		}
	}

	for _, s := range spans {
		if s.Name == "id" {
			continue // id's own checks (pattern/duplicate/missing) handled above
		}
		diags = append(diags, validateSpan(path, lineNum, s, taskID, loc)...)
	}

	return diags
}

// validateSpan checks one non-@id span: is the name recognized, did it
// capture a value at all (arity, M2 scope: every directive is arity 1, so
// "captured nothing" is the only arity failure worth checking), and, for a
// closed enum, is the value a member.
func validateSpan(path string, lineNum int, s Span, taskID string, loc *time.Location) []model.Diagnostic {
	if _, err := ParseSpan(s, loc); errors.Is(err, ErrUnknownDirective) {
		// Severity call: an unrecognized @name(...) on a task line is odd
		// but not fatal on its own — the line's other, recognized
		// directives (including @id) still parse and schedule normally.
		// Warning, not error.
		return []model.Diagnostic{diag(path, lineNum, model.DiagnosticSeverityWarning,
			CodeUnknownDirective, "unrecognized directive @"+s.Name, taskID)}
	}

	desc, ok := Descriptor(s.Name)
	if !ok {
		return nil // unreachable: ParseSpan already reported unknown above
	}

	if strings.TrimSpace(s.Value) == "" {
		return []model.Diagnostic{diag(path, lineNum, model.DiagnosticSeverityError,
			CodeEmptyValue, "@"+s.Name+" has no value", taskID)}
	}

	if desc.ValueType == TypeEnum && len(desc.Values) > 0 && !contains(desc.Values, s.Value) {
		return []model.Diagnostic{diag(path, lineNum, model.DiagnosticSeverityError,
			CodeInvalidEnumValue, "@"+s.Name+" value is not one of the allowed values", taskID)}
	}

	return nil
}

func firstIDSpan(spans []Span) (Span, bool) {
	for _, s := range spans {
		if s.Name == "id" {
			return s, true
		}
	}
	return Span{}, false
}

func contains(values []string, v string) bool {
	for _, c := range values {
		if c == v {
			return true
		}
	}
	return false
}

func diag(path string, line int, severity model.DiagnosticSeverity, code, message, taskID string) model.Diagnostic {
	c := code
	return model.Diagnostic{
		Code:     &c,
		FilePath: path,
		Line:     line,
		Message:  message,
		Severity: severity,
		TaskId:   taskID,
	}
}
