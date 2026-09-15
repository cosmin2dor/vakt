package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/engine"
	"github.com/cosmin2dor/vakt/internal/engine/schedule"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
	"github.com/cosmin2dor/vakt/internal/model"
)

// newTaskActionsTestServer builds an engine+orchestrator sharing one
// registry (echo suppression) over a seeded vault, and a mux serving the
// four action endpoints — no config/dispatch wiring needed for writeback.
func newTaskActionsTestServer(t *testing.T, files map[string]string) (*httptest.Server, string) {
	t.Helper()
	root := seedVault(t, files)
	reg := vault.NewHashRegistry()

	eng, err := engine.New(root, time.UTC, reg)
	require.NoError(t, err)
	orch := schedule.NewOrchestrator(vault.NewWriter(reg), root, time.UTC)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/tasks/{id}/fulfill", FulfillTaskHandler(eng, orch, time.UTC))
	mux.HandleFunc("POST /api/v1/tasks/{id}/pause", PauseTaskHandler(eng, orch, time.UTC))
	mux.HandleFunc("POST /api/v1/tasks/{id}/resume", ResumeTaskHandler(eng, orch, time.UTC))
	mux.HandleFunc("POST /api/v1/tasks/{id}/skip", SkipTaskHandler(eng, orch, time.UTC))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, root
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(b)
	}
	resp, err := http.Post(url, "application/json", reader)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestFulfillTask_OnceCompletesAndChecksBox(t *testing.T) {
	srv, root := newTaskActionsTestServer(t, map[string]string{
		"chores.md": "- [ ] Take out the bins @id(once_task) @once(2099-01-01T09:00:00Z) @state(active)\n",
	})

	resp := postJSON(t, srv.URL+"/api/v1/tasks/once_task/fulfill", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tk model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tk))
	require.Equal(t, model.Completed, tk.State)

	got, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.Contains(t, string(got), "- [x]")
	require.Contains(t, string(got), "@state(completed)")
}

func TestFulfillTask_ScheduleStampsLastCompleted(t *testing.T) {
	srv, root := newTaskActionsTestServer(t, map[string]string{
		"chores.md": "- [ ] Feed the dog @id(dog_feed) @schedule(0 8 * * *) @state(active)\n",
	})

	resp := postJSON(t, srv.URL+"/api/v1/tasks/dog_feed/fulfill", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tk model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tk))
	require.NotNil(t, tk.LastCompleted)

	got, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.Contains(t, string(got), "@last_completed(")
}

func TestPauseTask_WithReason(t *testing.T) {
	srv, root := newTaskActionsTestServer(t, map[string]string{
		"chores.md": "- [ ] Feed the dog @id(dog_feed) @schedule(0 8 * * *) @state(active)\n",
	})

	resp := postJSON(t, srv.URL+"/api/v1/tasks/dog_feed/pause", map[string]any{"reason": "on vacation"})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tk model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tk))
	require.Equal(t, model.Paused, tk.State)
	require.NotNil(t, tk.Reason)
	require.Equal(t, "on vacation", *tk.Reason)

	got, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.Contains(t, string(got), "@state(paused)")
	require.Contains(t, string(got), `@reason(on vacation)`)
}

func TestResumeTask_ReturnsToActive(t *testing.T) {
	srv, _ := newTaskActionsTestServer(t, map[string]string{
		"chores.md": "- [ ] Feed the dog @id(dog_feed) @schedule(0 8 * * *) @state(paused)\n",
	})

	resp := postJSON(t, srv.URL+"/api/v1/tasks/dog_feed/resume", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tk model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tk))
	require.Equal(t, model.Active, tk.State)
}

func TestResumeTask_AlreadyActiveIsNoop(t *testing.T) {
	srv, root := newTaskActionsTestServer(t, map[string]string{
		"chores.md": "- [ ] Feed the dog @id(dog_feed) @schedule(0 8 * * *) @state(active)\n",
	})

	resp := postJSON(t, srv.URL+"/api/v1/tasks/dog_feed/resume", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tk model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tk))
	require.Equal(t, model.Active, tk.State)

	got, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.Equal(t, "- [ ] Feed the dog @id(dog_feed) @schedule(0 8 * * *) @state(active)\n", string(got), "no defined transition: file must be untouched")
}

func TestSkipTask_DefaultCountIncrementsFromAbsent(t *testing.T) {
	srv, root := newTaskActionsTestServer(t, map[string]string{
		"chores.md": "- [ ] Clean litter @id(litter) @schedule(0 6 * * *) @state(active)\n",
	})

	resp := postJSON(t, srv.URL+"/api/v1/tasks/litter/skip", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tk model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tk))
	require.True(t, tk.EffectiveSuppression.Suppressed)
	require.NotNil(t, tk.EffectiveSuppression.Reason)
	require.Equal(t, model.SkipCount, *tk.EffectiveSuppression.Reason)

	got, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.Contains(t, string(got), "@skip_count(1)")
}

func TestSkipTask_ExplicitCount(t *testing.T) {
	srv, root := newTaskActionsTestServer(t, map[string]string{
		"chores.md": "- [ ] Clean litter @id(litter) @schedule(0 6 * * *) @state(active) @skip_count(2)\n",
	})

	resp := postJSON(t, srv.URL+"/api/v1/tasks/litter/skip", map[string]any{"count": 3})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	got, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.Contains(t, string(got), "@skip_count(5)")
}

func TestFulfillTask_MissingIDReturns404(t *testing.T) {
	srv, _ := newTaskActionsTestServer(t, map[string]string{
		"chores.md": "- [ ] Feed the dog @id(dog_feed) @state(active)\n",
	})

	resp := postJSON(t, srv.URL+"/api/v1/tasks/does_not_exist/fulfill", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errBody model.Error
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	require.Equal(t, "task_not_found", errBody.Error.Code)
}

func TestSkipTask_MissingIDReturns404(t *testing.T) {
	srv, _ := newTaskActionsTestServer(t, map[string]string{
		"chores.md": "- [ ] Feed the dog @id(dog_feed) @state(active)\n",
	})

	resp := postJSON(t, srv.URL+"/api/v1/tasks/does_not_exist/skip", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestPauseTask_ByteDiffTouchesOnlyStampedSpans(t *testing.T) {
	content := "- [ ] Feed the dog @id(dog_feed) @schedule(0 8 * * *) @state(active)\n" +
		"- [ ] Water plants @id(water_plants) @state(paused)\n"
	srv, root := newTaskActionsTestServer(t, map[string]string{"chores.md": content})

	resp := postJSON(t, srv.URL+"/api/v1/tasks/dog_feed/pause", map[string]any{"reason": "busy"})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	got, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	lines := strings.SplitN(string(got), "\n", 2)
	require.Equal(t, "- [ ] Water plants @id(water_plants) @state(paused)", strings.TrimSuffix(lines[1], "\n"), "second line must be byte-identical")
	require.Contains(t, lines[0], "@state(paused)")
	require.Contains(t, lines[0], "@reason(busy)")
	require.NotContains(t, lines[0], "@schedule(0 8 * * *) @state(active)", "old @state span must be gone, not appended alongside")
}
