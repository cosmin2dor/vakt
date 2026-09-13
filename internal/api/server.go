package api

import (
	"fmt"
	"net/http"

	"github.com/cosmin2dor/vakt/internal/config"
	"github.com/cosmin2dor/vakt/internal/engine/dispatch"
	"github.com/cosmin2dor/vakt/internal/integration/webpush"
)

// ServerConfig is every dependency needed to build vaktd's HTTP surface.
// cmd/vaktd populates it from env vars in production; a test populates it
// with temp directories, so both go through the exact same construction
// path (SDD.md §4 M1 — no test-only branch).
type ServerConfig struct {
	WebDir    string // built PWA bundle, served at "/"
	VaultDir  string // vault root the trigger endpoint walks
	ConfigDir string // /config volume: subscriptions + VAPID keypair

	// VAPIDContact is the "sub" claim (a mailto: URI) every signed VAPID
	// JWT carries, per RFC 8292.
	VAPIDContact string
}

// NewServer builds the production mux: the config stores, the dispatch
// registry with the real ios_notifications module registered, and every
// route main.go serves today.
func NewServer(cfg ServerConfig) (http.Handler, error) {
	subscriptions, err := config.NewStore(cfg.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("api: opening subscription store: %w", err)
	}
	vapid, err := config.NewVAPIDStore(cfg.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("api: opening vapid store: %w", err)
	}

	privateKey, err := vapid.PrivateKey()
	if err != nil {
		return nil, fmt.Errorf("api: loading vapid private key: %w", err)
	}

	registry := dispatch.NewRegistry()
	pushModule, err := webpush.New(privateKey, cfg.VAPIDContact, subscriptions, nil)
	if err != nil {
		return nil, fmt.Errorf("api: constructing ios_notifications module: %w", err)
	}
	if err := registry.Register(pushModule); err != nil {
		return nil, fmt.Errorf("api: registering ios_notifications module: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", HealthzHandler)
	mux.HandleFunc("POST /api/v1/tasks/{id}/trigger", TriggerHandler(cfg.VaultDir, registry))
	mux.HandleFunc("/api/v1/subscriptions", CreateSubscriptionHandler(subscriptions))
	mux.HandleFunc("/api/v1/subscriptions/unsubscribe", DeleteSubscriptionHandler(subscriptions))
	mux.HandleFunc("/api/v1/vapid-public-key", VAPIDPublicKeyHandler(vapid))
	mux.Handle("/", StaticHandler(cfg.WebDir))

	return mux, nil
}
