package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/config"
	"github.com/cosmin2dor/vakt/internal/model"
	"github.com/cosmin2dor/vakt/internal/testutil/pushrecorder"
)

// TestE2E_TriggerDispatchesAReallySignedAndEncryptedPush is the milestone's
// CI gate (SDD.md §4 M1): seed a vault and a fabricated subscription
// pointing at a recording server, hit the real trigger endpoint through
// api.NewServer — the exact construction path cmd/vaktd/main.go uses in
// production — and assert on the actual bytes the recorder decrypted. The
// only thing that differs from a real deployment is the subscription's
// endpoint URL, because a subscription carries its own endpoint (no
// test-only branch anywhere in the code under test).
func TestE2E_TriggerDispatchesAReallySignedAndEncryptedPush(t *testing.T) {
	vaultDir := seedVault(t, map[string]string{
		"chores.md": "- [ ] Take the bins out @id(bins_out) @target(ios_notifications) @schedule(0 8 * * *)\n",
	})

	recorder := pushrecorder.NewServer()
	defer recorder.Close()

	configDir := t.TempDir()
	subscriptions, err := config.NewStore(configDir)
	require.NoError(t, err)
	sub := recorder.Subscription()
	require.NoError(t, subscriptions.Add(config.PushSubscription{
		Endpoint: sub.Endpoint,
		Keys:     config.Keys{P256dh: sub.Keys.P256dh, Auth: sub.Keys.Auth},
	}))
	// Seeding a real VAPID keypair via the same store api.NewServer opens
	// itself (generate-if-absent), so the test exercises the identical
	// key-sourcing path production uses.
	_, err = config.NewVAPIDStore(configDir)
	require.NoError(t, err)

	handler, err := NewServer(context.Background(), ServerConfig{
		WebDir:       t.TempDir(),
		VaultDir:     vaultDir,
		ConfigDir:    configDir,
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
	require.True(t, outcome.Accepted, "outcome detail: %v", outcome.Detail)
	require.Equal(t, "bins_out", outcome.TaskId)
	require.Equal(t, "ios_notifications", outcome.Target)

	reqs := recorder.Requests()
	require.Len(t, reqs, 1)
	got := reqs[0]
	require.NoError(t, got.Err)

	// VAPID (RFC 8292): signed with the module's own private key, over
	// the recorder's own origin, carrying the contact configured above.
	require.True(t, got.VAPID.SignatureValid)
	require.Equal(t, "mailto:ops@example.com", got.VAPID.Claims.Subject)

	// Envelope (RFC 8188 / RFC 8291): structurally well-formed.
	require.Len(t, got.Salt, 16)
	require.Equal(t, uint32(4096), got.RecordSize)
	require.Len(t, got.SenderPublicKey, 65)

	// Payload templating is M3 (trigger.go); the production code path
	// sends "" today, so that's what a correct dispatch decrypts to.
	require.Equal(t, "", string(got.Plaintext))
}
