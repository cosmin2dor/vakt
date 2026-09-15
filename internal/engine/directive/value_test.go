package directive

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/model"
)

// sampleFor returns a valid raw value for each value type in the registry.
func sampleFor(valueType string) string {
	switch valueType {
	case TypeString:
		return "dog_feed"
	case TypeEnum:
		return "active"
	case TypeCron:
		return "0 8 * * *"
	case TypeDatetime:
		return "2026-01-02T08:30:00Z"
	case TypeInteger:
		return "3"
	default:
		return ""
	}
}

// Every registry entry must parse and round-trip — the task's exit bar.
func TestParseSpan_EveryRegistryDirectiveRoundTrips(t *testing.T) {
	loc := time.UTC
	require.NotEmpty(t, model.Directives)

	for _, desc := range model.Directives {
		t.Run(desc.Name, func(t *testing.T) {
			raw := sampleFor(desc.ValueType)
			require.NotEmpty(t, raw, "no sample for value type %q", desc.ValueType)

			span := Span{Name: desc.Name, Value: raw}
			v, err := ParseSpan(span, loc)
			require.NoError(t, err)
			assert.Equal(t, desc.ValueType, v.Type)
			assert.Equal(t, raw, v.Raw)

			reparsed, err := ParseValue(v.Type, v.Format(), loc)
			require.NoError(t, err)
			assert.Equal(t, v.Format(), reparsed.Format(), "text -> value -> text is not stable")
		})
	}
}

func TestParseValue_Datetime_OffsetIsAbsoluteAndBareIsLocal(t *testing.T) {
	// A vault timezone that is deliberately not UTC, so "local" is visible.
	loc, err := time.LoadLocation("Europe/Bucharest")
	require.NoError(t, err)

	tests := []struct {
		name string
		raw  string
		want time.Time
	}{
		{"explicit Z is absolute", "2026-01-02T08:30:00Z", time.Date(2026, 1, 2, 8, 30, 0, 0, time.UTC)},
		{"explicit offset is absolute", "2026-01-02T08:30:00+03:00", time.Date(2026, 1, 2, 5, 30, 0, 0, time.UTC)},
		{"bare timestamp is vault-local", "2026-01-02T08:30:00", time.Date(2026, 1, 2, 8, 30, 0, 0, loc)},
		{"minute precision is vault-local", "2026-01-02T08:30", time.Date(2026, 1, 2, 8, 30, 0, 0, loc)},
		{"date only is vault-local midnight", "2026-01-02", time.Date(2026, 1, 2, 0, 0, 0, 0, loc)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := ParseValue(TypeDatetime, tt.raw, loc)
			require.NoError(t, err)
			assert.True(t, tt.want.Equal(v.Time), "got %s, want %s", v.Time, tt.want)
		})
	}
}

func TestParseValue_ParseFailures(t *testing.T) {
	tests := []struct {
		name      string
		valueType string
		raw       string
		wantErr   error
	}{
		{"integer with letters", TypeInteger, "3x", ErrNotInteger},
		{"empty integer", TypeInteger, "", ErrNotInteger},
		{"unparseable timestamp", TypeDatetime, "next tuesday", ErrNotTimestamp},
		{"cron with too few fields", TypeCron, "0 8 * *", ErrNotCron},
		{"cron with too many fields", TypeCron, "0 8 * * * *", ErrNotCron},
		{"empty cron", TypeCron, "", ErrNotCron},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseValue(tt.valueType, tt.raw, time.UTC)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestParseSpan_UnknownDirective(t *testing.T) {
	_, err := ParseSpan(Span{Name: "not_a_directive", Value: "x"}, time.UTC)
	assert.ErrorIs(t, err, ErrUnknownDirective)
}

// Rule enforcement belongs to the validation layer, not here — these parse
// cleanly on purpose.
func TestParseSpan_DoesNotEnforceRules(t *testing.T) {
	v, err := ParseSpan(Span{Name: "state", Value: "not_a_real_state"}, time.UTC)
	require.NoError(t, err, "enum membership is the validator's job")
	assert.Equal(t, "not_a_real_state", v.Str)

	v, err = ParseSpan(Span{Name: "id", Value: "Has Spaces And Caps"}, time.UTC)
	require.NoError(t, err, "@id's pattern (G3) is the validator's job")
	assert.Equal(t, "Has Spaces And Caps", v.Str)
}

func TestParseValue_NilLocationDefaultsToLocal(t *testing.T) {
	v, err := ParseValue(TypeDatetime, "2026-01-02T08:30:00", nil)
	require.NoError(t, err)
	_, offset := v.Time.Zone()
	_, wantOffset := time.Date(2026, 1, 2, 8, 30, 0, 0, time.Local).Zone()
	assert.Equal(t, wantOffset, offset)
}

func TestDescriptor(t *testing.T) {
	d, ok := Descriptor("skip_count")
	require.True(t, ok)
	assert.Equal(t, TypeInteger, d.ValueType)

	_, ok = Descriptor("nope")
	assert.False(t, ok)
}
