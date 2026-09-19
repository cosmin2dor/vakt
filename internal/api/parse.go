package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cosmin2dor/vakt/internal/engine/directive"
	"github.com/cosmin2dor/vakt/internal/engine/schedule"
	"github.com/cosmin2dor/vakt/internal/model"
)

// maxUpcomingFires bounds the forward chain: enough for the cron popover's
// preview (issues/milestone5.md), not an open-ended search.
const maxUpcomingFires = 5

// idPatternRe is the registry's @id pattern (SDD.md G3), compiled once.
var idPatternRe = func() *regexp.Regexp {
	desc, ok := directive.Descriptor("id")
	if !ok || desc.Pattern == nil {
		panic("directive registry has no @id descriptor/pattern")
	}
	return regexp.MustCompile(*desc.Pattern)
}()

// ParseHandler is the sole parsing authority for one client-submitted line
// (CLAUDE.md). Pure evaluation: no vault, index, or writer access at all.
func ParseHandler(loc *time.Location) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req model.ParseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}

		spans := directive.Lex(req.Line)
		result := model.ParseResult{
			Spans:       make([]model.ParsedSpan, 0, len(spans)),
			Diagnostics: lineScopedDiagnostics(req.Line, spans),
		}

		var scheduleValue directive.Value
		hasSchedule := false
		for _, s := range spans {
			ps, v, ok, diag := parsedSpan(s, loc)
			result.Spans = append(result.Spans, ps)
			if diag != nil {
				result.Diagnostics = append(result.Diagnostics, *diag)
			}
			if ok && (s.Name == "schedule" || s.Name == "once") {
				scheduleValue, hasSchedule = v, true
			}
		}

		if hasSchedule {
			if fires := upcomingFires(scheduleValue, time.Now().In(loc)); len(fires) > 0 {
				result.UpcomingFires = &fires
			}
		}

		writeJSON(w, http.StatusOK, result)
	}
}

// parsedSpan builds one span's response shape and, when it parses, the typed
// Value other logic (schedule lookup) needs. An unregistered directive name
// leaves value_type/typed fields null (PR-79's schema note) and reports no
// diagnostic here — lineScopedDiagnostics already covers unknown directives.
// A registered directive whose value fails to parse gets an error
// diagnostic over its value range, with typed fields left null.
func parsedSpan(s directive.Span, loc *time.Location) (model.ParsedSpan, directive.Value, bool, *model.ParseDiagnostic) {
	ps := model.ParsedSpan{
		Name:       s.Name,
		Start:      s.Start,
		End:        s.End,
		NameStart:  s.NameStart,
		NameEnd:    s.NameEnd,
		ValueStart: s.ValueStart,
		ValueEnd:   s.ValueEnd,
		RawValue:   s.Value,
	}

	desc, ok := directive.Descriptor(s.Name)
	if !ok {
		return ps, directive.Value{}, false, nil
	}

	v, err := directive.ParseSpan(s, loc)
	if err != nil {
		diag := &model.ParseDiagnostic{
			Severity: model.ParseDiagnosticSeverityError,
			Message:  "@" + s.Name + " value does not parse: " + err.Error(),
			Start:    s.ValueStart,
			End:      s.ValueEnd,
		}
		return ps, directive.Value{}, false, diag
	}

	vt := desc.ValueType
	ps.ValueType = &vt
	switch v.Type {
	case directive.TypeString, directive.TypeEnum:
		ps.Str = &v.Str
	case directive.TypeInteger:
		ps.Int = &v.Int
	case directive.TypeDatetime:
		ps.Time = &v.Time
	case directive.TypeCron:
		ps.Cron = &v.Cron
	}

	return ps, v, true, nil
}

// lineScopedDiagnostics runs the subset of directive.ValidateFile's rules
// that make sense with no vault context: G3 (@id pattern), G5 (missing
// @id), unknown directives, empty values, and enum membership. G4
// (cross-file duplicate @id) is deliberately excluded — there is no other
// file or vault to check this line against.
func lineScopedDiagnostics(line string, spans []directive.Span) []model.ParseDiagnostic {
	if len(spans) == 0 {
		return nil // no @directive syntax at all: prose, not a task line
	}

	var diags []model.ParseDiagnostic

	idSpan, hasID := firstIDSpan(spans)
	switch {
	case !hasID:
		// No single span to blame; cover the whole line.
		diags = append(diags, parseDiag(model.ParseDiagnosticSeverityWarning,
			"missing_id", "task line has no @id; it will not be scheduled", 0, len(line)))
	case !idPatternRe.MatchString(idSpan.Value):
		diags = append(diags, parseDiag(model.ParseDiagnosticSeverityError,
			"invalid_id", "@id value does not match the required pattern", idSpan.Start, idSpan.End))
	}

	for _, s := range spans {
		if s.Name == "id" {
			continue // id's own checks handled above
		}
		diags = append(diags, spanDiagnostics(s)...)
	}

	return diags
}

// spanDiagnostics checks one non-@id span: is the name recognized, does it
// carry a value, and, for a closed enum, is the value a member.
func spanDiagnostics(s directive.Span) []model.ParseDiagnostic {
	desc, ok := directive.Descriptor(s.Name)
	if !ok {
		return []model.ParseDiagnostic{parseDiag(model.ParseDiagnosticSeverityWarning,
			"unknown_directive", "unrecognized directive @"+s.Name, s.Start, s.End)}
	}

	if strings.TrimSpace(s.Value) == "" {
		return []model.ParseDiagnostic{parseDiag(model.ParseDiagnosticSeverityError,
			"empty_value", "@"+s.Name+" has no value", s.Start, s.End)}
	}

	if desc.ValueType == directive.TypeEnum && len(desc.Values) > 0 && !containsStr(desc.Values, s.Value) {
		return []model.ParseDiagnostic{parseDiag(model.ParseDiagnosticSeverityError,
			"invalid_enum_value", "@"+s.Name+" value is not one of the allowed values", s.ValueStart, s.ValueEnd)}
	}

	return nil
}

func firstIDSpan(spans []directive.Span) (directive.Span, bool) {
	for _, s := range spans {
		if s.Name == "id" {
			return s, true
		}
	}
	return directive.Span{}, false
}

func containsStr(values []string, v string) bool {
	for _, c := range values {
		if c == v {
			return true
		}
	}
	return false
}

func parseDiag(severity model.ParseDiagnosticSeverity, code, message string, start, end int) model.ParseDiagnostic {
	c := code
	return model.ParseDiagnostic{
		Code:     &c,
		Severity: severity,
		Message:  message,
		Start:    start,
		End:      end,
	}
}

// upcomingFires chains NextFireValue forward, capped at maxUpcomingFires. A
// @once value fires at most once, so the chain naturally stops there.
func upcomingFires(v directive.Value, now time.Time) []time.Time {
	fires := make([]time.Time, 0, maxUpcomingFires)
	after := now
	for len(fires) < maxUpcomingFires {
		t, ok, err := schedule.NextFireValue(v, after)
		if err != nil || !ok {
			break
		}
		fires = append(fires, t)
		after = t
	}
	return fires
}
