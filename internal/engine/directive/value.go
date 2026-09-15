package directive

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cosmin2dor/vakt/internal/model"
)

// Value types, matching schema/directives.yaml's value_type field.
const (
	TypeString   = "string"
	TypeEnum     = "enum"
	TypeCron     = "cron"
	TypeDatetime = "datetime"
	TypeInteger  = "integer"
)

// Parse failures. The validation layer turns these into diagnostics; parsing
// itself only reports what it could and couldn't convert.
var (
	ErrUnknownDirective = errors.New("unknown directive")
	ErrNotInteger       = errors.New("not a whole number")
	ErrNotTimestamp     = errors.New("not a date or timestamp")
	ErrNotCron          = errors.New("not a 5-field cron expression")
)

// Value is one directive's parsed value. Which field carries it depends on
// Type; Raw is always the original text, since writeback patches bytes
// rather than re-serializing (SDD.md §4 M2).
type Value struct {
	Type string
	Raw  string

	Str  string    // string, enum
	Int  int64     // integer
	Time time.Time // datetime
	Cron []string  // cron fields; semantics come from M3's pinned library
}

// datetimeLayouts are tried in order. A layout carrying an offset (RFC3339,
// "...Z" or "+03:00") is an absolute instant and is honoured as written; the
// rest are local wall-clock in the vault's timezone.
//
// This resolves a gap the docs left open: PRD.md §3.2 writes @once as
// YYYY-MM-DDTHH:mm:ssZ (UTC), SDD.md G9 fixes one vault-wide timezone with
// local semantics, and UX.md's native date picker produces local wall-clock.
// ISO 8601's own rule settles it — offset present means absolute, absent
// means local — so both spellings work and neither is silently reinterpreted.
var datetimeLayouts = []struct {
	layout string
	local  bool
}{
	{time.RFC3339, false},
	{"2006-01-02T15:04:05", true},
	{"2006-01-02T15:04", true},
	{"2006-01-02", true}, // @skip_until is "a date or timestamp" (PRD.md §3.2)
}

// Descriptor looks up a directive's generated registry entry by name.
func Descriptor(name string) (model.DirectiveDescriptor, bool) {
	d, ok := descriptorsByName[name]
	return d, ok
}

var descriptorsByName = func() map[string]model.DirectiveDescriptor {
	m := make(map[string]model.DirectiveDescriptor, len(model.Directives))
	for _, d := range model.Directives {
		m[d.Name] = d
	}
	return m
}()

// ParseSpan converts a lexed span's raw value into a typed Value, using the
// directive's registry descriptor to decide how. loc is the vault timezone
// (SDD.md G9) that offset-less timestamps are read in.
//
// Enum membership and @id's pattern are deliberately not checked here —
// those are rule enforcement, which the validation layer owns.
func ParseSpan(s Span, loc *time.Location) (Value, error) {
	desc, ok := Descriptor(s.Name)
	if !ok {
		return Value{Raw: s.Value}, fmt.Errorf("%q: %w", s.Name, ErrUnknownDirective)
	}
	return ParseValue(desc.ValueType, s.Value, loc)
}

// ParseValue converts raw text of the given value type.
func ParseValue(valueType, raw string, loc *time.Location) (Value, error) {
	if loc == nil {
		loc = time.Local
	}
	v := Value{Type: valueType, Raw: raw}

	switch valueType {
	case TypeString, TypeEnum:
		v.Str = raw

	case TypeInteger:
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return v, fmt.Errorf("%q: %w", raw, ErrNotInteger)
		}
		v.Int = n

	case TypeDatetime:
		t, err := parseDatetime(strings.TrimSpace(raw), loc)
		if err != nil {
			return v, err
		}
		v.Time = t

	case TypeCron:
		fields := strings.Fields(raw)
		if len(fields) != 5 {
			return v, fmt.Errorf("%q has %d fields: %w", raw, len(fields), ErrNotCron)
		}
		v.Cron = fields

	default:
		return v, fmt.Errorf("value type %q: %w", valueType, ErrUnknownDirective)
	}

	return v, nil
}

func parseDatetime(raw string, loc *time.Location) (time.Time, error) {
	for _, l := range datetimeLayouts {
		if l.local {
			if t, err := time.ParseInLocation(l.layout, raw, loc); err == nil {
				return t, nil
			}
			continue
		}
		if t, err := time.Parse(l.layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%q: %w", raw, ErrNotTimestamp)
}

// Format renders a typed value back to the text a patch would write. Round
// trips with ParseValue so writeback never has to re-serialize a whole line.
func (v Value) Format() string {
	switch v.Type {
	case TypeInteger:
		return strconv.FormatInt(v.Int, 10)
	case TypeDatetime:
		return v.Time.Format(time.RFC3339)
	case TypeCron:
		return strings.Join(v.Cron, " ")
	default:
		return v.Str
	}
}
