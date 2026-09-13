package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/engine/dispatch"
	"github.com/cosmin2dor/vakt/internal/model"
)

// seedVault writes files (relative-path -> content) under a fresh temp
// directory and returns the directory's root. Mirrors
// internal/engine/vault/walker_test.go's helper of the same name, kept
// local since it lives in a different package.
func seedVault(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
	}
	return root
}

// fakeModule is a minimal dispatch.Module, mirroring
// internal/engine/dispatch/registry_test.go's fake of the same name.
type fakeModule struct {
	name      string
	gotTaskID string
	called    bool
	outcome   dispatch.Outcome
	err       error
}

func (m *fakeModule) Name() string { return m.name }

func (m *fakeModule) Dispatch(_ context.Context, task dispatch.TaskContext, _ string) (dispatch.Outcome, error) {
	m.called = true
	m.gotTaskID = task.ID
	return m.outcome, m.err
}

func newMux(t *testing.T, vaultRoot string, registry *dispatch.Registry) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/tasks/{id}/trigger", TriggerHandler(vaultRoot, registry))
	return mux
}

func TestTriggerHandler_DispatchesThroughRegisteredModule(t *testing.T) {
	const raw = "- [ ] Buy milk @id(buy_milk) @target(ios_notifications)\n"
	root := seedVault(t, map[string]string{"errands.md": raw})

	fake := &fakeModule{name: "ios_notifications", outcome: dispatch.Outcome{Accepted: true, Detail: "202 accepted"}}
	registry := dispatch.NewRegistry()
	require.NoError(t, registry.Register(fake))

	mux := newMux(t, root, registry)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/buy_milk/trigger", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var outcome model.TriggerOutcome
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &outcome))

	assert.True(t, outcome.Accepted)
	assert.Equal(t, "buy_milk", outcome.TaskId)
	assert.Equal(t, "ios_notifications", outcome.Target)
	require.NotNil(t, outcome.Detail)
	assert.Equal(t, "202 accepted", *outcome.Detail)

	assert.True(t, fake.called, "expected the registered fake module to be dispatched to")
	assert.Equal(t, "buy_milk", fake.gotTaskID)

	// No write-back: the seeded file must be byte-for-byte unchanged.
	after, err := os.ReadFile(filepath.Join(root, "errands.md"))
	require.NoError(t, err)
	assert.Equal(t, raw, string(after))
}

func TestTriggerHandler_UnknownTaskIDReturns404(t *testing.T) {
	root := seedVault(t, map[string]string{"errands.md": "- [ ] Buy milk @id(buy_milk)\n"})
	registry := dispatch.NewRegistry()
	mux := newMux(t, root, registry)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/does_not_exist/trigger", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)

	var errBody model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	assert.NotEmpty(t, errBody.Error.Code)
	assert.NotEmpty(t, errBody.Error.Message)
}

func TestTriggerHandler_TargetWithNoRegisteredModuleReturnsWellFormedOutcome(t *testing.T) {
	const raw = "- [ ] Buy milk @id(buy_milk) @target(ios_notifications)\n"
	root := seedVault(t, map[string]string{"errands.md": raw})

	// Empty registry: nothing registered under "ios_notifications" yet.
	registry := dispatch.NewRegistry()
	mux := newMux(t, root, registry)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/buy_milk/trigger", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var outcome model.TriggerOutcome
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &outcome))

	assert.False(t, outcome.Accepted)
	assert.Equal(t, "buy_milk", outcome.TaskId)
	assert.Equal(t, "ios_notifications", outcome.Target)
	require.NotNil(t, outcome.Detail)
	assert.NotEmpty(t, *outcome.Detail)

	// No write-back here either.
	after, err := os.ReadFile(filepath.Join(root, "errands.md"))
	require.NoError(t, err)
	assert.Equal(t, raw, string(after))
}

func TestTriggerHandler_TaskWithNoTargetDirectiveReturnsWellFormedOutcome(t *testing.T) {
	const raw = "- [ ] Water the plants @id(water_plants)\n"
	root := seedVault(t, map[string]string{"chores.md": raw})

	registry := dispatch.NewRegistry()
	mux := newMux(t, root, registry)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/water_plants/trigger", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var outcome model.TriggerOutcome
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &outcome))

	assert.False(t, outcome.Accepted)
	assert.Equal(t, "water_plants", outcome.TaskId)
	assert.Empty(t, outcome.Target)
	require.NotNil(t, outcome.Detail)
}
