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
	"github.com/cosmin2dor/vakt/internal/engine/vault"
	"github.com/cosmin2dor/vakt/internal/model"
)

// newCreateTestServer builds an engine over root and a mux serving only the
// create endpoint under test.
func newCreateTestServer(t *testing.T, root string) *httptest.Server {
	t.Helper()
	eng, err := engine.New(root, time.UTC, vault.NewHashRegistry())
	require.NoError(t, err)
	writer := vault.NewWriter(vault.NewHashRegistry())

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/tasks", CreateTaskHandler(eng, writer, root, time.UTC))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func postTask(t *testing.T, srv *httptest.Server, body model.TaskCreate) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(srv.URL+"/api/v1/tasks", "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestCreateTask_WithSchedule(t *testing.T) {
	root := seedVault(t, map[string]string{"chores.md": "# chores\n"})
	srv := newCreateTestServer(t, root)

	sched := "0 8 * * *"
	resp := postTask(t, srv, model.TaskCreate{
		Title:    "Feed the dog",
		FilePath: "chores.md",
		Schedule: &sched,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var task model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&task))
	require.Equal(t, "feed-the-dog", string(task.Id))
	require.Equal(t, "chores.md", task.FilePath)
	require.NotNil(t, task.ScheduleSummary)

	content, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.Contains(t, string(content), "@id(feed-the-dog)")
	require.Contains(t, string(content), "@schedule(0 8 * * *)")
}

func TestCreateTask_WithOnce(t *testing.T) {
	root := seedVault(t, map[string]string{"chores.md": "# chores\n"})
	srv := newCreateTestServer(t, root)

	once := time.Date(2099, 1, 1, 9, 0, 0, 0, time.UTC)
	resp := postTask(t, srv, model.TaskCreate{
		Title:    "Take out the bins",
		FilePath: "chores.md",
		Once:     &once,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var task model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&task))
	require.Equal(t, "take-out-the-bins", string(task.Id))

	content, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.Contains(t, string(content), "@once(2099-01-01T09:00:00Z)")
}

func TestCreateTask_BothScheduleAndOnce_400(t *testing.T) {
	root := seedVault(t, map[string]string{"chores.md": "# chores\n"})
	srv := newCreateTestServer(t, root)

	sched := "0 8 * * *"
	once := time.Now()
	resp := postTask(t, srv, model.TaskCreate{
		Title:    "Bad task",
		FilePath: "chores.md",
		Schedule: &sched,
		Once:     &once,
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateTask_NeitherScheduleNorOnce_400(t *testing.T) {
	root := seedVault(t, map[string]string{"chores.md": "# chores\n"})
	srv := newCreateTestServer(t, root)

	resp := postTask(t, srv, model.TaskCreate{
		Title:    "Bad task",
		FilePath: "chores.md",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateTask_TitleCollision_GetsDistinctID(t *testing.T) {
	root := seedVault(t, map[string]string{"chores.md": "# chores\n"})
	srv := newCreateTestServer(t, root)

	sched := "0 8 * * *"
	body := model.TaskCreate{Title: "Feed the dog", FilePath: "chores.md", Schedule: &sched}

	resp1 := postTask(t, srv, body)
	require.Equal(t, http.StatusCreated, resp1.StatusCode)
	var task1 model.Task
	require.NoError(t, json.NewDecoder(resp1.Body).Decode(&task1))
	require.Equal(t, "feed-the-dog", string(task1.Id))

	resp2 := postTask(t, srv, body)
	require.Equal(t, http.StatusCreated, resp2.StatusCode)
	var task2 model.Task
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&task2))
	require.Equal(t, "feed-the-dog-2", string(task2.Id))

	content, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.Contains(t, string(content), "@id(feed-the-dog)")
	require.Contains(t, string(content), "@id(feed-the-dog-2)")
}

func TestCreateTask_SlugifiesUnicodeAndPunctuation(t *testing.T) {
	root := seedVault(t, map[string]string{"chores.md": "# chores\n"})
	srv := newCreateTestServer(t, root)

	sched := "0 8 * * *"
	resp := postTask(t, srv, model.TaskCreate{
		Title:    "Nourrir le chien 🐕 café!!",
		FilePath: "chores.md",
		Schedule: &sched,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var task model.Task
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&task))
	require.Regexp(t, `^[a-z0-9_-]{1,64}$`, string(task.Id))
}

func TestCreateTask_PathTraversal_400(t *testing.T) {
	root := seedVault(t, map[string]string{"chores.md": "# chores\n"})
	srv := newCreateTestServer(t, root)

	sched := "0 8 * * *"
	resp := postTask(t, srv, model.TaskCreate{
		Title:    "Escape",
		FilePath: "../outside.md",
		Schedule: &sched,
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	_, err := os.Stat(filepath.Join(filepath.Dir(root), "outside.md"))
	require.True(t, os.IsNotExist(err))
}

func TestCreateTask_NewFile_CreatedWithOneLine(t *testing.T) {
	root := seedVault(t, map[string]string{"seed.md": "# seed\n"})
	srv := newCreateTestServer(t, root)

	sched := "0 8 * * *"
	resp := postTask(t, srv, model.TaskCreate{
		Title:    "Brand new",
		FilePath: "new/routines.md",
		Schedule: &sched,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	content, err := os.ReadFile(filepath.Join(root, "new", "routines.md"))
	require.NoError(t, err)
	require.Contains(t, string(content), "@id(brand-new)")
	require.Equal(t, 1, len(splitLines(string(content))))
}

// A file missing its trailing newline (a real, unremarkable shape any
// editor can produce) must not merge its last line into the new one.
func TestCreateTask_NoTrailingNewlineInExistingFile(t *testing.T) {
	root := seedVault(t, map[string]string{"chores.md": "# chores"})
	srv := newCreateTestServer(t, root)

	sched := "0 8 * * *"
	resp := postTask(t, srv, model.TaskCreate{
		Title:    "Feed the dog",
		FilePath: "chores.md",
		Schedule: &sched,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	content, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(content), "# chores\n"),
		"existing line must not be merged with the new one; got: %q", string(content))
}

func TestCreateTask_MalformedCron_400(t *testing.T) {
	root := seedVault(t, map[string]string{"chores.md": "# chores\n"})
	srv := newCreateTestServer(t, root)

	sched := "not a cron"
	before, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)

	resp := postTask(t, srv, model.TaskCreate{
		Title:    "Bad cron",
		FilePath: "chores.md",
		Schedule: &sched,
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	after, err := os.ReadFile(filepath.Join(root, "chores.md"))
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
}

// splitLines counts non-empty lines, tolerating a trailing newline.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			if i > start {
				lines = append(lines, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
