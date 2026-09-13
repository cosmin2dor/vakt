package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmin2dor/vakt/internal/config"
	"github.com/cosmin2dor/vakt/internal/engine/dispatch"
	"github.com/cosmin2dor/vakt/internal/testutil/pushrecorder"
)

// fakeSubscriptionSource is a SubscriptionSource that returns a fixed list,
// standing in for *config.Store so tests don't need a real store on disk.
type fakeSubscriptionSource struct {
	subs []config.PushSubscription
}

func (f fakeSubscriptionSource) List() []config.PushSubscription {
	return f.subs
}

// roundTripFunc adapts a function to http.RoundTripper, for asserting no
// HTTP call was made without standing up a real listener.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func generateVAPIDKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return priv
}

// arbitrarySubscriptionKeys builds a structurally valid but otherwise
// unrelated p256dh/auth pair, for a fake subscriber whose push service
// never actually decrypts (it just returns a fixed status).
func arbitrarySubscriptionKeys(t *testing.T) config.Keys {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	require.NoError(t, err)
	auth := make([]byte, 16)
	_, err = rand.Read(auth)
	require.NoError(t, err)
	return config.Keys{
		P256dh: base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
		Auth:   base64.RawURLEncoding.EncodeToString(auth),
	}
}

// TestDispatchVerifiesAgainstRecordingServer is the primary correctness
// gate: it sends a real Dispatch to a pushrecorder.Server and checks all
// three RFC layers the recorder captured — VAPID claims/signature, the
// aes128gcm envelope's structural fields, and the decrypted plaintext.
func TestDispatchVerifiesAgainstRecordingServer(t *testing.T) {
	recorder := pushrecorder.NewServer()
	defer recorder.Close()

	sub := recorder.Subscription()
	subs := fakeSubscriptionSource{subs: []config.PushSubscription{{
		Endpoint: sub.Endpoint,
		Keys:     config.Keys{P256dh: sub.Keys.P256dh, Auth: sub.Keys.Auth},
	}}}

	priv := generateVAPIDKey(t)
	mod, err := New(priv, "mailto:ops@example.com", subs, nil)
	require.NoError(t, err)
	require.Equal(t, "ios_notifications", mod.Name())

	const payload = `{"title":"Take the bins out"}`
	outcome, err := mod.Dispatch(context.Background(), dispatch.TaskContext{}, payload)
	require.NoError(t, err)
	require.True(t, outcome.Accepted)
	require.Contains(t, outcome.Detail, "1/1 accepted")

	reqs := recorder.Requests()
	require.Len(t, reqs, 1)
	got := reqs[0]
	require.NoError(t, got.Err)

	// VAPID (RFC 8292).
	require.True(t, got.VAPID.SignatureValid)
	require.Equal(t, "mailto:ops@example.com", got.VAPID.Claims.Subject)
	wantAud, err := url.Parse(recorder.URL())
	require.NoError(t, err)
	require.Equal(t, wantAud.Scheme+"://"+wantAud.Host, got.VAPID.Claims.Audience)

	// Envelope (RFC 8188 / RFC 8291).
	require.Len(t, got.Salt, 16)
	require.Equal(t, uint32(4096), got.RecordSize)
	require.Len(t, got.SenderPublicKey, 65)

	// Plaintext, verbatim.
	require.Equal(t, payload, string(got.Plaintext))
}

// TestDispatchNoSubscriptionsFails covers G14's edge: zero enrolled
// subscribers means the task fails (nothing was accepted), and no HTTP
// call should ever be attempted.
func TestDispatchNoSubscriptionsFails(t *testing.T) {
	priv := generateVAPIDKey(t)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatal("no HTTP request should be made with zero enrolled subscriptions")
		return nil, nil
	})}

	mod, err := New(priv, "mailto:ops@example.com", fakeSubscriptionSource{}, client)
	require.NoError(t, err)

	outcome, err := mod.Dispatch(context.Background(), dispatch.TaskContext{}, "payload")
	require.NoError(t, err)
	require.False(t, outcome.Accepted)
	require.NotEmpty(t, outcome.Detail)
}

// TestDispatchPartialFailureStillAccepted covers G14's core ruling: one
// dead subscriber (simulated with a server returning 410 Gone) must not
// mark the dispatch failed while another subscriber still accepts.
func TestDispatchPartialFailureStillAccepted(t *testing.T) {
	recorder := pushrecorder.NewServer()
	defer recorder.Close()
	goodSub := recorder.Subscription()

	gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	defer gone.Close()

	subs := fakeSubscriptionSource{subs: []config.PushSubscription{
		{Endpoint: goodSub.Endpoint, Keys: config.Keys{P256dh: goodSub.Keys.P256dh, Auth: goodSub.Keys.Auth}},
		{Endpoint: gone.URL, Keys: arbitrarySubscriptionKeys(t)},
	}}

	priv := generateVAPIDKey(t)
	mod, err := New(priv, "mailto:ops@example.com", subs, nil)
	require.NoError(t, err)

	outcome, err := mod.Dispatch(context.Background(), dispatch.TaskContext{}, "hello")
	require.NoError(t, err)
	require.True(t, outcome.Accepted)
	require.Contains(t, outcome.Detail, "1/2 accepted")
	require.Contains(t, outcome.Detail, "410")

	reqs := recorder.Requests()
	require.Len(t, reqs, 1)
	require.NoError(t, reqs[0].Err)
	require.Equal(t, "hello", string(reqs[0].Plaintext))
}
