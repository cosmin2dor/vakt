//go:build smoketest

// Package smoketest drives the real Dockerfile/docker-compose.yml image
// over real HTTP, as opposed to internal/api's e2e_test.go which calls
// api.NewServer's handler in-process. It builds the image, runs a
// container with the same volumes/env docker-compose.yml declares, and
// asserts a real push round-trip through a host-side pushrecorder — so a
// broken volume, env var, or route fails this test instead of only
// surfacing in a real deployment.
//
// Slow (image build + container start): excluded from `go test ./...` by
// the smoketest build tag. Run via `make smoketest`.
package smoketest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/model"
	"github.com/cosmin2dor/vakt/internal/testutil/pushrecorder"
)

const image = "vakt-smoketest:test"

// TestSmoke_ContainerTriggerDispatchesPush builds the real image, runs it
// as docker-compose.yml describes, and triggers a task over real HTTP,
// asserting the resulting push is genuinely signed/encrypted per RFC
// 8291/8292 — the same assertions internal/api/e2e_test.go makes, now
// through a container instead of an in-process handler.
func TestSmoke_ContainerTriggerDispatchesPush(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	require.NoError(t, err)

	buildImage(t, repoRoot)

	vaultDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(vaultDir, "chores.md"),
		[]byte("- [ ] Take the bins out @id(bins_out) @target(ios_notifications) @schedule(0 8 * * *)\n"),
		0o644,
	))

	recorder := pushrecorder.NewServer()
	defer recorder.Close()

	configDir := t.TempDir()
	seedSubscription(t, configDir, recorder.Subscription())
	// vapid.json is deliberately left unseeded: NewVAPIDStore generates
	// one on first run, exactly as a fresh /config volume would.

	containerName := fmt.Sprintf("vakt-smoketest-%d", time.Now().UnixNano())
	runContainer(t, containerName, vaultDir, configDir, true)
	defer func() {
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	}()

	hostPort := containerPort(t, containerName)
	baseURL := "http://localhost:" + hostPort
	waitForHealthz(t, baseURL)

	resp, err := http.Post(baseURL+"/api/v1/tasks/bins_out/trigger", "application/json", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var outcome model.TriggerOutcome
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&outcome))
	require.True(t, outcome.Accepted, "outcome detail: %v", outcome.Detail)
	require.Equal(t, "bins_out", outcome.TaskId)
	require.Equal(t, "ios_notifications", outcome.Target)

	reqs := recorder.Requests()
	require.Len(t, reqs, 1)
	got := reqs[0]
	require.NoError(t, got.Err)

	// VAPID (RFC 8292): signed by the container's own generated keypair.
	require.True(t, got.VAPID.SignatureValid)
	require.Equal(t, "mailto:vakt@localhost", got.VAPID.Claims.Subject)

	// Envelope (RFC 8188 / RFC 8291): structurally well-formed.
	require.Len(t, got.Salt, 16)
	require.Equal(t, uint32(4096), got.RecordSize)
	require.Len(t, got.SenderPublicKey, 65)
}

// buildImage builds the real Dockerfile, failing the test with the
// captured build output if it doesn't succeed — this is the "freshly
// built image" the task's Done criterion names.
func buildImage(t *testing.T, repoRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "build", "-t", image, repoRoot)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "docker build failed:\n%s", out)
}

// seedSubscription writes a subscriptions.json in config.Store's on-disk
// shape (JSON object keyed by endpoint), rewriting the recorder's endpoint
// to host.docker.internal: the recorder runs on the host, and the
// container must reach back out to it the way a real deployment reaches a
// real push service, not via the container's own loopback.
func seedSubscription(t *testing.T, configDir string, sub pushrecorder.Subscription) {
	t.Helper()
	endpoint := strings.NewReplacer(
		"127.0.0.1", "host.docker.internal",
		"localhost", "host.docker.internal",
	).Replace(sub.Endpoint)

	doc := map[string]any{
		endpoint: map[string]any{
			"endpoint": endpoint,
			"keys": map[string]string{
				"p256dh": sub.Keys.P256dh,
				"auth":   sub.Keys.Auth,
			},
		},
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "subscriptions.json"), data, 0o644))
}

// runContainer starts the image with the same volumes/env
// docker-compose.yml declares, plus --add-host so the container can reach
// the host-side pushrecorder. withVault controls whether the vault bind
// mount is attached, so the misconfiguration check (see PR description)
// can omit it and observe the failure mode.
func runContainer(t *testing.T, name, vaultDir, configDir string, withVault bool) {
	t.Helper()
	args := []string{
		"run", "-d", "--name", name,
		"--add-host", "host.docker.internal:host-gateway",
		"-p", "127.0.0.1::8080",
		"-v", configDir + ":/config",
		"-e", "VAKT_VAULT_DIR=/vault",
	}
	if withVault {
		args = append(args, "-v", vaultDir+":/vault")
	}
	args = append(args, image)
	out, err := exec.Command("docker", args...).CombinedOutput()
	require.NoError(t, err, "docker run failed:\n%s", out)
}

// containerPort resolves the host port docker assigned to the container's
// published 8080/tcp, since -p 127.0.0.1::8080 leaves the choice to Docker
// to avoid clashing with anything already bound on the host.
func containerPort(t *testing.T, name string) string {
	t.Helper()
	out, err := exec.Command("docker", "port", name, "8080/tcp").CombinedOutput()
	require.NoError(t, err, "docker port failed:\n%s", out)
	// Output looks like "127.0.0.1:54321"; take the part after the colon.
	line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	idx := strings.LastIndex(line, ":")
	require.True(t, idx >= 0, "unexpected docker port output: %q", line)
	return line[idx+1:]
}

// waitForHealthz polls /healthz until it responds or the timeout elapses,
// giving the container a moment to start before the test hits it.
func waitForHealthz(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("healthz returned %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("container never became healthy: %v", lastErr)
}
