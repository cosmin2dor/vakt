package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/cosmin2dor/vakt/internal/config"
	"github.com/cosmin2dor/vakt/internal/model"
)

// toStoreSubscription converts the generated model type into the config
// package's PushSubscription, the shape Store persists. ExpirationTime is
// carried as Unix millis on disk (matching PushSubscription.toJSON()'s
// wire shape) but decodes through the generated type as *time.Time.
func toStoreSubscription(m model.PushSubscription) config.PushSubscription {
	sub := config.PushSubscription{
		Endpoint: m.Endpoint,
		Keys: config.Keys{
			P256dh: m.Keys.P256dh,
			Auth:   m.Keys.Auth,
		},
	}
	if m.ExpirationTime != nil {
		ms := m.ExpirationTime.UnixMilli()
		sub.ExpirationTime = &ms
	}
	return sub
}

// toModelSubscription is the inverse of toStoreSubscription, for
// responding with what was actually stored.
func toModelSubscription(s config.PushSubscription) model.PushSubscription {
	m := model.PushSubscription{
		Endpoint: s.Endpoint,
	}
	m.Keys.P256dh = s.Keys.P256dh
	m.Keys.Auth = s.Keys.Auth
	if s.ExpirationTime != nil {
		t := time.UnixMilli(*s.ExpirationTime).UTC()
		m.ExpirationTime = &t
	}
	return m
}

// CreateSubscriptionHandler handles POST /api/v1/subscriptions: decode a
// PushSubscription, store it, and echo back what was stored (SDD.md §2.3 —
// persisted to /config, never the vault).
func CreateSubscriptionHandler(store *config.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body model.PushSubscription
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "malformed_subscription", "request body is not a valid PushSubscription")
			return
		}
		if body.Endpoint == "" {
			writeError(w, http.StatusBadRequest, "malformed_subscription", "endpoint is required")
			return
		}

		sub := toStoreSubscription(body)
		if err := store.Add(sub); err != nil {
			writeError(w, http.StatusBadRequest, "malformed_subscription", err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, toModelSubscription(sub))
	}
}

// unsubscribeBody mirrors DeleteSubscriptionJSONBody's shape; the
// generated type is an unexported detail of the request body, not
// something a handler decodes into directly, so this stays a plain local
// struct.
type unsubscribeBody struct {
	Endpoint string `json:"endpoint"`
}

// DeleteSubscriptionHandler handles POST /api/v1/subscriptions/unsubscribe:
// remove a subscription by endpoint. Per SDD.md G14 and the store's own
// idempotent Remove, an absent endpoint is still a 204, not a 404.
func DeleteSubscriptionHandler(store *config.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body unsubscribeBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "malformed_request", "request body must be {\"endpoint\": string}")
			return
		}
		if body.Endpoint == "" {
			writeError(w, http.StatusBadRequest, "malformed_request", "endpoint is required")
			return
		}

		if err := store.Remove(body.Endpoint); err != nil {
			writeError(w, http.StatusBadRequest, "malformed_request", err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// vapidPublicKeyResponse is the {"public_key": string} shape the schema
// defines inline for GET /vapid-public-key (no named component to reuse).
type vapidPublicKeyResponse struct {
	PublicKey string `json:"public_key"`
}

// VAPIDPublicKeyHandler handles GET /api/v1/vapid-public-key. The keypair
// is generate-if-absent (VAPIDStore), so this never has a 404 path: by the
// time the server is serving requests, a keypair already exists.
func VAPIDPublicKeyHandler(store *config.VAPIDStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, vapidPublicKeyResponse{PublicKey: store.PublicKey()})
	}
}
