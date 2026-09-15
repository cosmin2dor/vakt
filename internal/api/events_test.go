package api

import (
	"bufio"
	"context"
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

// newEventsTestServer builds a running engine (watcher started, unlike
// newTasksTestServer's static one — SSE needs live reconciliation) over a
// seeded vault, and a real listening server for the events route.
func newEventsTestServer(t *testing.T, files map[string]string, heartbeat time.Duration) (*httptest.Server, string) {
	t.Helper()
	root := seedVault(t, files)

	eng, err := engine.New(root, time.UTC, vault.NewHashRegistry())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = eng.Run(ctx) }()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/events", StreamEventsHandler(eng, time.UTC, heartbeat))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, root
}

// sseClient opens a streaming GET to url and hands back a line reader over
// the response body plus a cancel that tears the connection down.
func sseClient(t *testing.T, url string) (*bufio.Reader, context.CancelFunc, *http.Response) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	return bufio.NewReader(resp.Body), cancel, resp
}

// readEnvelope reads lines off r until it finds a non-empty "data: " line,
// decodes it, and fails the test if none arrives before deadline.
func readEnvelope(t *testing.T, r *bufio.Reader, deadline time.Duration) model.EventEnvelope {
	t.Helper()
	type result struct {
		env model.EventEnvelope
		err error
	}
	out := make(chan result, 1)
	go func() {
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				out <- result{err: err}
				return
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var env model.EventEnvelope
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &env); err != nil {
				out <- result{err: err}
				return
			}
			out <- result{env: env}
			return
		}
	}()

	select {
	case r := <-out:
		require.NoError(t, r.err)
		return r.env
	case <-time.After(deadline):
		t.Fatal("timed out waiting for an SSE event")
		return model.EventEnvelope{}
	}
}

// readEnvelopeUntil reads envelopes, skipping heartbeats, until it finds
// one of the wanted type or the deadline elapses.
func readEnvelopeUntil(t *testing.T, r *bufio.Reader, want model.EventEnvelopeType, deadline time.Duration) model.EventEnvelope {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		env := readEnvelope(t, r, deadline)
		if env.Type == want {
			return env
		}
	}
	t.Fatalf("no %s event within %s", want, deadline)
	return model.EventEnvelope{}
}

func TestStreamEvents_ExternalEditProducesTaskUpserted(t *testing.T) {
	srv, root := newEventsTestServer(t, map[string]string{"chores.md": "# empty\n"}, time.Hour)
	r, cancel, resp := sseClient(t, srv.URL+"/api/v1/events")
	defer cancel()
	defer func() { _ = resp.Body.Close() }()

	require.NoError(t, os.WriteFile(filepath.Join(root, "chores.md"),
		[]byte("- [ ] wash dishes @id(wash) @state(active)\n"), 0o644))

	env := readEnvelopeUntil(t, r, model.TaskUpserted, 3*time.Second)
	require.NotNil(t, env.Task)
	require.Equal(t, model.TaskId("wash"), env.Task.Id)
}

func TestStreamEvents_RemovedTaskProducesTaskRemoved(t *testing.T) {
	// The watcher does not report whole-file deletion (documented, out
	// of scope in internal/engine/watcher), so removal here is exercised
	// via clearing the @id from the file — a Write the watcher does see.
	rootFiles := map[string]string{"chores.md": "- [ ] wash dishes @id(wash) @state(active)\n"}
	srv, root := newEventsTestServer(t, rootFiles, time.Hour)
	r, cancel, resp := sseClient(t, srv.URL+"/api/v1/events")
	defer cancel()
	defer func() { _ = resp.Body.Close() }()

	require.NoError(t, os.WriteFile(filepath.Join(root, "chores.md"), []byte("wash dishes, no id anymore\n"), 0o644))

	env := readEnvelopeUntil(t, r, model.TaskRemoved, 3*time.Second)
	require.NotNil(t, env.TaskId)
	require.Equal(t, model.TaskId("wash"), *env.TaskId)
}

func TestStreamEvents_HeartbeatArrivesOnShortInterval(t *testing.T) {
	srv, _ := newEventsTestServer(t, map[string]string{"empty.md": "# nothing\n"}, 50*time.Millisecond)
	r, cancel, resp := sseClient(t, srv.URL+"/api/v1/events")
	defer cancel()
	defer func() { _ = resp.Body.Close() }()

	env := readEnvelopeUntil(t, r, model.Heartbeat, 2*time.Second)
	require.Equal(t, model.Heartbeat, env.Type)
	require.False(t, env.Timestamp.IsZero())
}

func TestStreamEvents_TwoSubscribersBothReceiveTheSameEvent(t *testing.T) {
	srv, root := newEventsTestServer(t, map[string]string{"chores.md": "# empty\n"}, time.Hour)

	r1, cancel1, resp1 := sseClient(t, srv.URL+"/api/v1/events")
	defer cancel1()
	defer func() { _ = resp1.Body.Close() }()
	r2, cancel2, resp2 := sseClient(t, srv.URL+"/api/v1/events")
	defer cancel2()
	defer func() { _ = resp2.Body.Close() }()

	require.NoError(t, os.WriteFile(filepath.Join(root, "chores.md"),
		[]byte("- [ ] wash dishes @id(wash) @state(active)\n"), 0o644))

	env1 := readEnvelopeUntil(t, r1, model.TaskUpserted, 3*time.Second)
	env2 := readEnvelopeUntil(t, r2, model.TaskUpserted, 3*time.Second)
	require.Equal(t, env1.Task.Id, env2.Task.Id)
}

func TestStreamEvents_ClientDisconnectDoesNotHangServer(t *testing.T) {
	srv, root := newEventsTestServer(t, map[string]string{"chores.md": "# empty\n"}, time.Hour)
	r, cancel, resp := sseClient(t, srv.URL+"/api/v1/events")
	_ = r

	cancel() // client disconnects
	_ = resp.Body.Close()

	// Server must keep serving other clients/requests without hanging —
	// connect a fresh client first, then produce the change it should see.
	r2, cancel2, resp2 := sseClient(t, srv.URL+"/api/v1/events")
	defer cancel2()
	defer func() { _ = resp2.Body.Close() }()

	require.NoError(t, os.WriteFile(filepath.Join(root, "chores.md"),
		[]byte("- [ ] wash dishes @id(wash) @state(active)\n"), 0o644))

	env := readEnvelopeUntil(t, r2, model.TaskUpserted, 3*time.Second)
	require.Equal(t, model.TaskId("wash"), env.Task.Id)
}

func TestStreamEvents_DiagnosticRaisedAndCleared(t *testing.T) {
	srv, root := newEventsTestServer(t, map[string]string{
		"chores.md": "- [ ] wash dishes @id(wash) @state(active)\n",
	}, time.Hour)
	r, cancel, resp := sseClient(t, srv.URL+"/api/v1/events")
	defer cancel()
	defer func() { _ = resp.Body.Close() }()

	// Introduce a duplicate @id (G4, error diagnostic).
	require.NoError(t, os.WriteFile(filepath.Join(root, "chores.md"),
		[]byte("- [ ] wash dishes @id(wash) @state(active)\n- [ ] feed cat @id(wash) @state(active)\n"), 0o644))

	raised := readEnvelopeUntil(t, r, model.DiagnosticRaised, 3*time.Second)
	require.NotNil(t, raised.Diagnostic)
	require.Equal(t, model.DiagnosticSeverityError, raised.Diagnostic.Severity)

	// Fix it: remove the duplicate.
	require.NoError(t, os.WriteFile(filepath.Join(root, "chores.md"),
		[]byte("- [ ] wash dishes @id(wash) @state(active)\n"), 0o644))

	cleared := readEnvelopeUntil(t, r, model.DiagnosticCleared, 3*time.Second)
	require.NotNil(t, cleared.Diagnostic)
}
