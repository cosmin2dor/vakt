package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/model"
)

// doParse posts line through ParseHandler and decodes the result. No vault,
// index, or writer is ever constructed — the handler needs none.
func doParse(t *testing.T, loc *time.Location, line string) model.ParseResult {
	t.Helper()
	body, err := json.Marshal(model.ParseRequest{Line: line})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/parse", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	ParseHandler(loc)(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var result model.ParseResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	return result
}

func spanByName(t *testing.T, result model.ParseResult, name string) model.ParsedSpan {
	t.Helper()
	for _, s := range result.Spans {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no span named %q in %+v", name, result.Spans)
	return model.ParsedSpan{}
}

func TestParseHandler_WellFormedLine(t *testing.T) {
	loc := time.UTC
	line := "- [ ] Feed the dog @id(dog_feed) @state(active) @schedule(0 8 * * *)"
	result := doParse(t, loc, line)

	assert.Empty(t, result.Diagnostics)
	require.Len(t, result.Spans, 3)

	id := spanByName(t, result, "id")
	assert.Equal(t, "dog_feed", id.RawValue)
	require.NotNil(t, id.ValueType)
	assert.Equal(t, "string", *id.ValueType)
	require.NotNil(t, id.Str)
	assert.Equal(t, "dog_feed", *id.Str)

	sched := spanByName(t, result, "schedule")
	require.NotNil(t, sched.Cron)
	assert.Equal(t, []string{"0", "8", "*", "*", "*"}, *sched.Cron)

	require.NotNil(t, result.UpcomingFires)
	fires := *result.UpcomingFires
	require.Len(t, fires, 5)
	for i := 1; i < len(fires); i++ {
		assert.True(t, fires[i].After(fires[i-1]))
		assert.Equal(t, 24*time.Hour, fires[i].Sub(fires[i-1]))
	}
}

func TestParseHandler_UnknownDirective(t *testing.T) {
	result := doParse(t, time.UTC, "Water plants @bogus(x) @id(water_plants)")

	bogus := spanByName(t, result, "bogus")
	assert.Nil(t, bogus.ValueType)
	assert.Nil(t, bogus.Str)

	found := false
	for _, d := range result.Diagnostics {
		if d.Code != nil && *d.Code == "unknown_directive" {
			found = true
			assert.Equal(t, bogus.Start, d.Start)
			assert.Equal(t, bogus.End, d.End)
		}
	}
	assert.True(t, found, "expected an unknown_directive diagnostic")
}

func TestParseHandler_MalformedValue(t *testing.T) {
	line := "Take out trash @id(take_out_trash) @skip_count(not-a-number)"
	result := doParse(t, time.UTC, line)

	sc := spanByName(t, result, "skip_count")
	assert.Nil(t, sc.ValueType)
	assert.Nil(t, sc.Int)

	found := false
	for _, d := range result.Diagnostics {
		if d.Severity == model.ParseDiagnosticSeverityError && d.Start == sc.ValueStart && d.End == sc.ValueEnd {
			found = true
		}
	}
	assert.True(t, found, "expected an error diagnostic over skip_count's value range")
}

func TestParseHandler_InvalidID(t *testing.T) {
	result := doParse(t, time.UTC, "Do a thing @id(Not Valid!) @state(active)")

	found := false
	for _, d := range result.Diagnostics {
		if d.Code != nil && *d.Code == "invalid_id" {
			found = true
			assert.Equal(t, model.ParseDiagnosticSeverityError, d.Severity)
		}
	}
	assert.True(t, found, "expected an invalid_id diagnostic")
}

func TestParseHandler_MissingID(t *testing.T) {
	line := "Do a thing @state(active)"
	result := doParse(t, time.UTC, line)

	require.Len(t, result.Spans, 1) // @state still parses fine

	found := false
	for _, d := range result.Diagnostics {
		if d.Code != nil && *d.Code == "missing_id" {
			found = true
			assert.Equal(t, model.ParseDiagnosticSeverityWarning, d.Severity)
			assert.Equal(t, 0, d.Start)
			assert.Equal(t, len(line), d.End)
		}
	}
	assert.True(t, found, "expected a missing_id warning")
}

func TestParseHandler_OnceInFuture(t *testing.T) {
	loc := time.UTC
	future := time.Now().In(loc).Add(48 * time.Hour).Format("2006-01-02T15:04:05")
	line := "Pay rent @id(pay_rent) @once(" + future + ")"
	result := doParse(t, loc, line)

	require.NotNil(t, result.UpcomingFires)
	assert.Len(t, *result.UpcomingFires, 1)
}

func TestParseHandler_OnceInPast(t *testing.T) {
	line := "Pay rent @id(pay_rent) @once(2000-01-01T00:00:00)"
	result := doParse(t, time.UTC, line)

	assert.Nil(t, result.UpcomingFires)
}

func TestParseHandler_NoSchedule(t *testing.T) {
	result := doParse(t, time.UTC, "Do a thing @id(do_a_thing) @state(active)")
	assert.Nil(t, result.UpcomingFires)
}
