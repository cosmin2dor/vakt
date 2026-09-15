package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/engine"
	"github.com/cosmin2dor/vakt/internal/engine/index"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
)

// TestPerfCheck_ListTasks is milestone4's run-performance-sanity-pass: the
// number that answers "does SDD.md §1's client-filters-everything decision
// still hold at household-plus scale" is how long GET /tasks itself takes
// to serve — see deploy/RUNBOOK.md for the recorded number, and
// internal/engine/index and internal/engine/schedule for the index and
// scheduler legs of the same check.
func TestPerfCheck_ListTasks(t *testing.T) {
	shape := index.HouseholdPlusShape()
	files := index.GenerateSyntheticVault(shape)
	root := seedVault(t, files)

	eng, err := engine.New(root, time.UTC, vault.NewHashRegistry())
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tasks", ListTasksHandler(eng, time.UTC))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	listStart := time.Now()
	resp, err := http.Get(srv.URL + "/api/v1/tasks")
	listElapsed := time.Since(listStart)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	taskCount := len(eng.Index().Tasks())
	t.Logf("perfcheck: GET /api/v1/tasks over %d tasks: %s", taskCount, listElapsed)

	// Sanity bar, not a strict benchmark: flag an order-of-magnitude
	// regression, not micro-fluctuation.
	require.Less(t, listElapsed, 500*time.Millisecond, "GET /tasks got surprisingly slow at household-plus scale")
}
