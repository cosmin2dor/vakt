package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/model"
)

// This file validates the RUNNING API against schema/openapi.yaml, not any
// one handler in isolation — the milestone's exit criterion is that the two
// stay honest with each other. It is orthogonal to the handler-specific
// *_test.go files alongside it: those assert behavior, this asserts shape.

// loadSpec parses and validates schema/openapi.yaml itself. Reloaded per
// caller (cheap, and each caller may mutate doc.Servers) rather than shared,
// so nothing here needs a sync.Once.
func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../schema/openapi.yaml")
	require.NoError(t, err)
	// The spec puts description/nullable alongside $ref in a few places
	// (e.g. Diagnostic.task_id) — valid, common OAS 3.0 usage that this
	// library's default strict mode otherwise rejects.
	require.NoError(t, doc.Validate(loader.Context, specValidationOpts...), "schema/openapi.yaml must itself be a valid OpenAPI 3 document")
	return doc
}

// specValidationOpts loosens the library's default strictness to accept a
// $ref with description/nullable siblings, which schema/openapi.yaml uses
// deliberately and which is valid OAS 3.0 (siblings are simply ignored by
// spec-compliant readers, not an error).
var specValidationOpts = []openapi3.ValidationOption{openapi3.AllowExtraSiblingFields("description", "nullable")}

// TestContractSpec_IsValid confirms the spec parses and validates on its
// own, independent of anything served against it below.
func TestContractSpec_IsValid(t *testing.T) {
	loadSpec(t)
}

// contractFixture is a real NewServer mux, wrapped with an openapi3filter
// validator bound to doc, over a vault seeded with tasks in varied
// states/shapes. Every request/response round-tripped through client hits
// spec conformance checking; violations are collected in failures rather
// than only surfacing as a swapped status code.
type contractFixture struct {
	client    *http.Client
	baseURL   string
	vaultRoot string
	failures  []string
	mu        sync.Mutex
}

func newContractFixture(t *testing.T) *contractFixture {
	t.Helper()
	doc := loadSpec(t)

	root := seedVault(t, map[string]string{
		"chores.md": "" +
			"- [ ] Take the bins out @id(bins_out) @target(ios_notifications) @schedule(0 8 * * *) @state(active)\n" +
			"- [ ] Water the plants @id(water_plants) @schedule(0 9 * * 1) @state(paused) @reason(on holiday)\n" +
			"- [ ] Pay rent @id(pay_rent) @once(2099-01-01T09:00:00Z) @state(active)\n" +
			"- [ ] Wash the car @id(wash_car) @schedule(0 10 * * 6) @state(active)\n",
	})

	handler, err := NewServer(context.Background(), ServerConfig{
		WebDir:       t.TempDir(),
		VaultDir:     root,
		ConfigDir:    t.TempDir(),
		VAPIDContact: "mailto:ops@example.com",
		Loc:          time.UTC,
	})
	require.NoError(t, err)

	server := httptest.NewUnstartedServer(nil)
	baseURL := "http://" + server.Listener.Addr().String() + "/api/v1"

	// The router matches requests against doc.Servers, so the spec's own
	// server entry has to be repointed at this ephemeral listener.
	doc.Servers = openapi3.Servers{{URL: baseURL}}
	require.NoError(t, doc.Validate(context.Background(), specValidationOpts...))

	router, err := gorillamux.NewRouter(doc)
	require.NoError(t, err)

	f := &contractFixture{baseURL: baseURL, vaultRoot: root}
	validator := openapi3filter.NewValidator(router, openapi3filter.Strict(true),
		openapi3filter.OnLog(func(_ context.Context, message string, err error) {
			if err == nil {
				return
			}
			f.mu.Lock()
			f.failures = append(f.failures, message+": "+err.Error())
			f.mu.Unlock()
		}))
	validated := validator.Middleware(handler)

	// /events streams indefinitely; Strict's response wrapper buffers the
	// whole body before validating, which would hang forever on an SSE
	// connection. That route bypasses the middleware and is checked by
	// hand in TestContract_Events instead.
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/events" {
			handler.ServeHTTP(w, r)
			return
		}
		validated.ServeHTTP(w, r)
	})
	server.Start()
	t.Cleanup(server.Close)
	f.client = server.Client()

	return f
}

// do performs req and returns the response plus any spec violations logged
// during it (nil means clean).
func (f *contractFixture) do(t *testing.T, method, path string, body []byte) (*http.Response, []string) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	var req *http.Request
	var err error
	if reader != nil {
		req, err = http.NewRequest(method, f.baseURL+path, reader)
	} else {
		req, err = http.NewRequest(method, f.baseURL+path, nil)
	}
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	f.mu.Lock()
	before := len(f.failures)
	f.mu.Unlock()

	resp, err := f.client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	f.mu.Lock()
	newFailures := append([]string(nil), f.failures[before:]...)
	f.mu.Unlock()
	return resp, newFailures
}

func TestContract_Tasks(t *testing.T) {
	f := newContractFixture(t)

	t.Run("list", func(t *testing.T) {
		resp, failures := f.do(t, http.MethodGet, "/tasks", nil)
		require.Empty(t, failures)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var tasks []model.Task
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&tasks))
		require.Len(t, tasks, 4)
	})

	t.Run("get existing", func(t *testing.T) {
		resp, failures := f.do(t, http.MethodGet, "/tasks/bins_out", nil)
		require.Empty(t, failures)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("get missing", func(t *testing.T) {
		resp, failures := f.do(t, http.MethodGet, "/tasks/does-not-exist", nil)
		require.Empty(t, failures)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("create valid", func(t *testing.T) {
		sched := "0 8 * * *"
		body, err := json.Marshal(model.TaskCreate{Title: "Feed the dog", FilePath: "chores.md", Schedule: &sched})
		require.NoError(t, err)
		resp, failures := f.do(t, http.MethodPost, "/tasks", body)
		require.Empty(t, failures)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("create invalid missing required field", func(t *testing.T) {
		// Deliberately sends a request the spec itself declares invalid
		// (TaskCreate.title is required), so the validator's own
		// request-side rejection is expected here, not a failure to assert
		// against — the interesting assertion is the 400 status.
		resp, _ := f.do(t, http.MethodPost, "/tasks", []byte(`{"file_path":"chores.md"}`))
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("fulfill", func(t *testing.T) {
		resp, failures := f.do(t, http.MethodPost, "/tasks/pay_rent/fulfill", nil)
		require.Empty(t, failures)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("pause", func(t *testing.T) {
		resp, failures := f.do(t, http.MethodPost, "/tasks/bins_out/pause", nil)
		require.Empty(t, failures)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("resume", func(t *testing.T) {
		resp, failures := f.do(t, http.MethodPost, "/tasks/water_plants/resume", nil)
		require.Empty(t, failures)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("skip", func(t *testing.T) {
		resp, failures := f.do(t, http.MethodPost, "/tasks/wash_car/skip", nil)
		require.Empty(t, failures)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestContract_VaultBrowsing(t *testing.T) {
	f := newContractFixture(t)

	t.Run("directories", func(t *testing.T) {
		resp, failures := f.do(t, http.MethodGet, "/directories", nil)
		require.Empty(t, failures)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("existing file", func(t *testing.T) {
		resp, failures := f.do(t, http.MethodGet, "/files?path=chores.md", nil)
		require.Empty(t, failures)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("missing file", func(t *testing.T) {
		resp, failures := f.do(t, http.MethodGet, "/files?path=nope.md", nil)
		require.Empty(t, failures)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestContract_Directives(t *testing.T) {
	f := newContractFixture(t)
	resp, failures := f.do(t, http.MethodGet, "/directives", nil)
	require.Empty(t, failures)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestContract_Events checks the one route the validator middleware can't
// (a stream, not a single JSON body): the declared content type, and one
// decoded data: line validated by hand against the EventEnvelope schema
// component, since openapi3filter's response validation expects a complete
// body up front.
func TestContract_Events(t *testing.T) {
	doc := loadSpec(t)
	f := newContractFixture(t)

	req, err := http.NewRequest(http.MethodGet, f.baseURL+"/events", nil)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := f.client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	op := doc.Paths.Find("/events").Get
	declared := op.Responses.Value("200").Value.Content
	require.Contains(t, declared, "text/event-stream", "spec's own declared content type for GET /events")
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	envelopeSchema := doc.Components.Schemas["EventEnvelope"].Value

	// The real server's heartbeat is 20s (server.go); rather than wait that
	// long, force a task_upserted quickly by editing the seeded vault file
	// the watcher is already watching.
	require.NoError(t, os.WriteFile(f.vaultRoot+"/chores.md",
		[]byte("- [ ] wash dishes @id(wash_dishes) @state(active)\n"), 0o644))

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		require.NoError(t, err, "expected at least one data: line before the context deadline")
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")

		var decoded map[string]any
		require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
		require.NoError(t, envelopeSchema.VisitJSON(decoded), "SSE data: payload must validate as EventEnvelope")
		return
	}
}

// TestContract_CatchesDriftInResponseShape is the load-bearing regression
// proof (milestone exit criterion: "a deliberate handler/spec divergence
// fails the suite"). It never touches a real handler — it feeds
// openapi3filter.ValidateResponse a hand-built Task response, wired to the
// real GET /tasks/{id} operation, that omits the required "state" field, and
// asserts the validator rejects it. This is the same code path
// newContractFixture's middleware runs on every request above; a permanent,
// self-contained version of the manual handler-edit-and-revert check.
func TestContract_CatchesDriftInResponseShape(t *testing.T) {
	doc := loadSpec(t)
	doc.Servers = openapi3.Servers{{URL: "http://example.invalid/api/v1"}}
	require.NoError(t, doc.Validate(context.Background(), specValidationOpts...))

	router, err := gorillamux.NewRouter(doc)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/api/v1/tasks/bins_out", nil)
	require.NoError(t, err)
	route, pathParams, err := router.FindRoute(req)
	require.NoError(t, err)

	reqInput := &openapi3filter.RequestValidationInput{Request: req, PathParams: pathParams, Route: route}

	// A structurally complete Task, per schema/openapi.yaml's Task schema,
	// except "state" is missing — the shape a handler regression (e.g.
	// dropping a struct field, or a JSON tag rename) would actually produce.
	malformed := []byte(`{
		"id": "bins_out",
		"title": "Take the bins out",
		"file_path": "chores.md",
		"next_fire": null,
		"schedule_summary": "Daily at 08:00",
		"effective_suppression": {"suppressed": false, "reason": null},
		"last_triggered": null,
		"last_completed": null,
		"reason": null,
		"target": "ios_notifications"
	}`)

	err = openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: reqInput,
		Status:                 http.StatusOK,
		Header:                 http.Header{"Content-Type": []string{"application/json"}},
		Body:                   newBody(malformed),
	})
	require.Error(t, err, "a Task response missing the required state field must fail contract validation")
	require.Contains(t, err.Error(), "state")
}

// newBody wraps b as the io.ReadCloser ResponseValidationInput.Body expects.
func newBody(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}
