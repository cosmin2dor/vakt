package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cosmin2dor/vakt/internal/api"
	"github.com/cosmin2dor/vakt/internal/config"
)

func newTestServer(t *testing.T) (*httptest.Server, *config.Store, *config.VAPIDStore) {
	t.Helper()
	dir := t.TempDir()

	store, err := config.NewStore(dir)
	if err != nil {
		t.Fatalf("config.NewStore: %v", err)
	}
	vapid, err := config.NewVAPIDStore(dir)
	if err != nil {
		t.Fatalf("config.NewVAPIDStore: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/subscriptions", api.CreateSubscriptionHandler(store))
	mux.HandleFunc("/api/v1/subscriptions/unsubscribe", api.DeleteSubscriptionHandler(store))
	mux.HandleFunc("/api/v1/vapid-public-key", api.VAPIDPublicKeyHandler(vapid))

	return httptest.NewServer(mux), store, vapid
}

func TestCreateSubscription_Valid(t *testing.T) {
	srv, store, _ := newTestServer(t)
	defer srv.Close()

	body := `{"endpoint":"https://push.example/abc","keys":{"p256dh":"pkey","auth":"akey"}}`
	resp, err := http.Post(srv.URL+"/api/v1/subscriptions", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /subscriptions: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	var got map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got["endpoint"] != "https://push.example/abc" {
		t.Errorf("response endpoint = %v, want https://push.example/abc", got["endpoint"])
	}

	subs := store.List()
	if len(subs) != 1 || subs[0].Endpoint != "https://push.example/abc" {
		t.Errorf("store.List() = %+v, want one subscription for the posted endpoint", subs)
	}
}

func TestCreateSubscription_Malformed(t *testing.T) {
	srv, store, _ := newTestServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/subscriptions", "application/json", bytes.NewBufferString("not json"))
	if err != nil {
		t.Fatalf("POST /subscriptions: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	var got map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if _, ok := got["error"]; !ok {
		t.Errorf("response body = %+v, want an \"error\" key (generated Error shape)", got)
	}

	if len(store.List()) != 0 {
		t.Errorf("store.List() = %+v, want empty after a malformed subscribe", store.List())
	}
}

func TestDeleteSubscription_Existing(t *testing.T) {
	srv, store, _ := newTestServer(t)
	defer srv.Close()

	if err := store.Add(config.PushSubscription{
		Endpoint: "https://push.example/gone",
		Keys:     config.Keys{P256dh: "p", Auth: "a"},
	}); err != nil {
		t.Fatalf("seeding store: %v", err)
	}

	body := `{"endpoint":"https://push.example/gone"}`
	resp, err := http.Post(srv.URL+"/api/v1/subscriptions/unsubscribe", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /subscriptions/unsubscribe: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if len(store.List()) != 0 {
		t.Errorf("store.List() = %+v, want empty after unsubscribe", store.List())
	}
}

func TestDeleteSubscription_Nonexistent(t *testing.T) {
	srv, _, _ := newTestServer(t)
	defer srv.Close()

	// Removing an endpoint that was never subscribed is idempotent (SDD.md
	// G14), so it is still a 204, not a 404.
	body := `{"endpoint":"https://push.example/never-existed"}`
	resp, err := http.Post(srv.URL+"/api/v1/subscriptions/unsubscribe", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /subscriptions/unsubscribe: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestDeleteSubscription_Malformed(t *testing.T) {
	srv, _, _ := newTestServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/subscriptions/unsubscribe", "application/json", bytes.NewBufferString("not json"))
	if err != nil {
		t.Fatalf("POST /subscriptions/unsubscribe: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestVAPIDPublicKey_PersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	first, err := config.NewVAPIDStore(dir)
	if err != nil {
		t.Fatalf("first config.NewVAPIDStore: %v", err)
	}
	firstKey := first.PublicKey()
	if firstKey == "" {
		t.Fatal("first store's PublicKey() is empty, want a generated key")
	}

	// A second, independent Store/loader pointed at the same directory
	// simulates a process restart: it must load the persisted keypair
	// rather than generating a new one.
	second, err := config.NewVAPIDStore(dir)
	if err != nil {
		t.Fatalf("second config.NewVAPIDStore: %v", err)
	}
	secondKey := second.PublicKey()

	if firstKey != secondKey {
		t.Errorf("public key changed across instances: first = %q, second = %q", firstKey, secondKey)
	}

	// And over HTTP, twice, against the second (restarted) instance.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vapid-public-key", api.VAPIDPublicKeyHandler(second))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for i := 0; i < 2; i++ {
		resp, err := http.Get(srv.URL + "/api/v1/vapid-public-key")
		if err != nil {
			t.Fatalf("GET /vapid-public-key (call %d): %v", i, err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status (call %d) = %d, want 200", i, resp.StatusCode)
		}
		var got struct {
			PublicKey string `json:"public_key"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decoding response (call %d): %v", i, err)
		}
		if got.PublicKey != firstKey {
			t.Errorf("call %d: public_key = %q, want %q", i, got.PublicKey, firstKey)
		}
	}
}
