package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/engine"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
	"github.com/cosmin2dor/vakt/internal/model"
)

// newTasksTestServer builds an engine over a seeded vault and a mux serving
// only the two read endpoints under test — no config stores or dispatch
// registry needed since fixture directives never carry @target.
func newTasksTestServer(t *testing.T, files map[string]string) *httptest.Server {
	t.Helper()
	root := seedVault(t, files)

	eng, err := engine.New(root, time.UTC, vault.NewHashRegistry())
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tasks", ListTasksHandler(eng, time.UTC))
	mux.HandleFunc("GET /api/v1/tasks/{id}", GetTaskHandler(eng, time.UTC))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestListTasks_EmptyVaultReturnsEmptyArrayNotNull(t *testing.T) {
	srv := newTasksTestServer(t, map[string]string{"empty.md": "# nothing here\n"})

	resp, err := http.Get(srv.URL + "/api/v1/tasks")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.JSONEq(t, "[]", string(body))
}

func TestListTasks_PopulatesDerivedFieldsPerTaskShape(t *testing.T) {
	srv := newTasksTestServer(t, map[string]string{
		"chores.md": "" +
			"- [ ] Feed the dog @id(dog_feed) @target(ios_notifications) @schedule(0 8 * * *) @state(active)\n" +
			"- [ ] Take out the bins @id(once_task) @once(2099-01-01T09:00:00Z) @state(active)\n" +
			"- [ ] Water plants @id(paused_task) @schedule(0 9 * * *) @state(paused)\n" +
			"- [ ] Walk the dog @id(skip_until_task) @schedule(0 7 * * *) @state(active) @skip_until(2099-01-01)\n" +
			"- [ ] Clean litter @id(skip_count_task) @schedule(0 6 * * *) @state(active) @skip_count(2)\n" +
			"- [ ] Expired reminder @id(elapsed_once) @once(2000-01-01T09:00:00Z) @state(active)\n",
	})

	resp, err := http.Get(srv.URL + "/api/v1/tasks")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tasks []model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tasks))
	require.Len(t, tasks, 6)

	byID := make(map[string]model.Task, len(tasks))
	for _, tk := range tasks {
		byID[tk.Id] = tk
	}

	dog := byID["dog_feed"]
	require.Equal(t, "Feed the dog", dog.Title)
	require.Equal(t, "chores.md", dog.FilePath)
	require.Equal(t, model.Active, dog.State)
	require.NotNil(t, dog.Target)
	require.Equal(t, "ios_notifications", *dog.Target)
	require.NotNil(t, dog.NextFire)
	require.NotNil(t, dog.ScheduleSummary)
	require.NotEqual(t, "0 8 * * *", *dog.ScheduleSummary, "must not be the raw cron string")
	require.Contains(t, *dog.ScheduleSummary, "08:00")
	require.False(t, dog.EffectiveSuppression.Suppressed)
	require.Nil(t, dog.EffectiveSuppression.Reason)

	once := byID["once_task"]
	require.Equal(t, "Take out the bins", once.Title)
	require.NotNil(t, once.NextFire)
	require.NotNil(t, once.ScheduleSummary)
	require.Contains(t, *once.ScheduleSummary, "Once,")
	require.NotContains(t, *once.ScheduleSummary, "2099-01-01T09:00:00Z", "must not be the raw ISO timestamp")

	paused := byID["paused_task"]
	require.Equal(t, model.Paused, paused.State)
	require.True(t, paused.EffectiveSuppression.Suppressed)
	require.NotNil(t, paused.EffectiveSuppression.Reason)
	require.Equal(t, model.StateNotActive, *paused.EffectiveSuppression.Reason)

	skipUntil := byID["skip_until_task"]
	require.True(t, skipUntil.EffectiveSuppression.Suppressed)
	require.NotNil(t, skipUntil.EffectiveSuppression.Reason)
	require.Equal(t, model.SkipUntil, *skipUntil.EffectiveSuppression.Reason)

	skipCount := byID["skip_count_task"]
	require.True(t, skipCount.EffectiveSuppression.Suppressed)
	require.NotNil(t, skipCount.EffectiveSuppression.Reason)
	require.Equal(t, model.SkipCount, *skipCount.EffectiveSuppression.Reason)

	// An elapsed @once (G11) is unschedulable but still indexed — it must
	// still appear here, just with a null next_fire.
	elapsed := byID["elapsed_once"]
	require.Equal(t, "Expired reminder", elapsed.Title)
	require.Nil(t, elapsed.NextFire)
	require.NotNil(t, elapsed.ScheduleSummary)
}

func TestGetTask_ExistingID(t *testing.T) {
	srv := newTasksTestServer(t, map[string]string{
		"chores.md": "- [ ] Feed the dog @id(dog_feed) @target(ios_notifications) @schedule(0 8 * * *) @state(active)\n",
	})

	resp, err := http.Get(srv.URL + "/api/v1/tasks/dog_feed")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tk model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tk))
	require.Equal(t, "dog_feed", tk.Id)
	require.Equal(t, "Feed the dog", tk.Title)
	require.Equal(t, "chores.md", tk.FilePath)
	require.Equal(t, model.Active, tk.State)
	require.NotNil(t, tk.Target)
	require.Equal(t, "ios_notifications", *tk.Target)
	require.NotNil(t, tk.NextFire)
	require.NotNil(t, tk.ScheduleSummary)
	require.False(t, tk.EffectiveSuppression.Suppressed)
	require.Nil(t, tk.LastTriggered)
	require.Nil(t, tk.LastCompleted)
	require.Nil(t, tk.Reason)
}

// TestGetTask_NextFireEvaluatedInConfiguredLocation guards against
// buildTaskDTO silently using the server process's own local clock instead
// of loc (SDD.md G9) — the two only diverge in a real deployment, so this
// pins a non-UTC loc explicitly rather than relying on the default fixture.
func TestGetTask_NextFireEvaluatedInConfiguredLocation(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	root := seedVault(t, map[string]string{
		"chores.md": "- [ ] Feed the dog @id(dog_feed) @schedule(30 23 * * *) @state(active)\n",
	})
	eng, err := engine.New(root, ny, vault.NewHashRegistry())
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tasks/{id}", GetTaskHandler(eng, ny))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/tasks/dog_feed")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	var tk model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tk))
	require.NotNil(t, tk.NextFire)

	// The task fires at 23:30 wall-clock in America/New_York. If the
	// handler used the server's raw time.Now() (its own Location, not
	// ny) instead of time.Now().In(ny), robfig/cron would match the
	// 23:30 field against the WRONG location's wall clock (per its own
	// "schedules... are treated as local to the time provided" docs),
	// silently shifting next_fire by whatever offset separates the two
	// zones. Asserting the fire time's wall-clock hour/minute in ny
	// pins the correct location was actually used.
	inNY := tk.NextFire.In(ny)
	require.Equal(t, 23, inNY.Hour())
	require.Equal(t, 30, inNY.Minute())
}

func TestGetTask_MissingIDReturns404(t *testing.T) {
	srv := newTasksTestServer(t, map[string]string{
		"chores.md": "- [ ] Feed the dog @id(dog_feed) @state(active)\n",
	})

	resp, err := http.Get(srv.URL + "/api/v1/tasks/does_not_exist")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errBody model.Error
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	require.Equal(t, "task_not_found", errBody.Error.Code)
}
