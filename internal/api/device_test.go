//go:build device

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/config"
	"github.com/cosmin2dor/vakt/internal/model"
)

// deviceSubscriptionPath is the gitignored fixture a human copies a real,
// enrolled device's subscription entry into (see deploy/RUNBOOK.md). It is
// never fabricated and never committed: endpoint + p256dh + auth is a
// secret, and it's device-bound and expiring besides.
const deviceSubscriptionPath = "testdata/device-subscription.json"

// deviceConfigDirEnv names the /config directory of a real, running vaktd
// instance — the one that generated the VAPID keypair the fixture
// subscription above was created against. A push service rejects a
// message signed with any other keypair, so this must match exactly
// (deploy/RUNBOOK.md documents how to point it at the right directory).
const deviceConfigDirEnv = "VAKT_DEVICE_CONFIG_DIR"

// TestDevice_TriggerDispatchesARealPushToARealSubscription is the milestone's
// device-verification check (SDD.md §4 M1 "On device verification"): the
// same end-to-end path as TestE2E_TriggerDispatchesAReallySignedAndEncryptedPush,
// but against a real subscription and the real VAPID keypair it was created
// with, so it actually reaches a real push service (Apple/Google/Mozilla).
// It never runs in CI (build-tagged "device", excluded from `go test ./...`
// and `make test`) and is opt-in: it skips cleanly when the fixture or the
// config dir env var is absent, rather than failing.
func TestDevice_TriggerDispatchesARealPushToARealSubscription(t *testing.T) {
	data, err := os.ReadFile(deviceSubscriptionPath)
	if os.IsNotExist(err) {
		t.Skipf("skipping: no real device subscription at %s — see deploy/RUNBOOK.md to enrol one", deviceSubscriptionPath)
	}
	require.NoError(t, err)

	var sub config.PushSubscription
	require.NoError(t, json.Unmarshal(data, &sub))
	require.NotEmpty(t, sub.Endpoint, "%s: endpoint must not be empty", deviceSubscriptionPath)

	configDir := os.Getenv(deviceConfigDirEnv)
	if configDir == "" {
		t.Skipf("skipping: %s is unset — point it at the /config directory of the vaktd instance that issued the VAPID keypair this subscription was created with (deploy/RUNBOOK.md)", deviceConfigDirEnv)
	}

	vaultDir := seedVault(t, map[string]string{
		"chores.md": "- [ ] Take the bins out @id(bins_out) @target(ios_notifications) @schedule(0 8 * * *)\n",
	})

	// A dedicated config dir seeded with only the one subscription under
	// test, so this never spams every device an operator has enrolled.
	// The VAPID keypair still comes from the real, shared configDir below.
	subDir := t.TempDir()
	subscriptions, err := config.NewStore(subDir)
	require.NoError(t, err)
	require.NoError(t, subscriptions.Add(sub))

	// Real VAPID keypair, read from the real /config directory — never
	// generated here. api.NewServer opens its own subscription store from
	// ConfigDir, so we point ConfigDir at subDir (the one-subscription
	// store) and instead borrow the real keypair by copying it in: the
	// simplest way to keep NewServer's single ConfigDir contract while
	// using the caller's real key material.
	vapidPath := configDir + "/vapid.json"
	vapidBytes, err := os.ReadFile(vapidPath)
	require.NoError(t, err, "reading vapid.json from %s (%s)", configDir, deviceConfigDirEnv)
	require.NoError(t, os.WriteFile(subDir+"/vapid.json", vapidBytes, 0o600))

	handler, err := NewServer(ServerConfig{
		WebDir:       t.TempDir(),
		VaultDir:     vaultDir,
		ConfigDir:    subDir,
		VAPIDContact: "mailto:ops@example.com",
	})
	require.NoError(t, err)

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/v1/tasks/bins_out/trigger", "application/json", nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var outcome model.TriggerOutcome
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&outcome))
	// Web Push gives no delivery confirmation (SDD.md §3) — "accepted" by
	// the push service is the strongest claim any test can honestly make.
	require.True(t, outcome.Accepted, "outcome detail: %v", outcome.Detail)
	require.Equal(t, "bins_out", outcome.TaskId)
	require.Equal(t, "ios_notifications", outcome.Target)
}
