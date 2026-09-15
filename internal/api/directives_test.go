package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/model"
)

func TestListDirectives_ServesEveryRegistryEntry(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/directives", ListDirectivesHandler())
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/directives")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var out []model.Directive
	require.NoError(t, json.Unmarshal(body, &out))
	require.Len(t, out, len(model.Directives))

	byName := make(map[string]model.Directive, len(out))
	for _, d := range out {
		byName[d.Name] = d
	}

	id, ok := byName["id"]
	require.True(t, ok)
	require.Equal(t, "string", id.ValueType)
	require.True(t, id.Required)
	require.False(t, id.SystemWritten)

	state, ok := byName["state"]
	require.True(t, ok)
	require.Equal(t, "enum", state.ValueType)
	require.True(t, state.Required)

	schedule, ok := byName["schedule"]
	require.True(t, ok)
	require.Equal(t, "cron", schedule.ValueType)
	require.False(t, schedule.Required)

	payload, ok := byName["payload"]
	require.True(t, ok)
	require.Equal(t, "string", payload.ValueType)
	require.False(t, payload.SystemWritten)

	lastTriggered, ok := byName["last_triggered"]
	require.True(t, ok)
	require.True(t, lastTriggered.SystemWritten)
}
