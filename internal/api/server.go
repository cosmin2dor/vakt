package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/cosmin2dor/vakt/internal/config"
	"github.com/cosmin2dor/vakt/internal/engine"
	"github.com/cosmin2dor/vakt/internal/engine/dispatch"
	"github.com/cosmin2dor/vakt/internal/engine/schedule"
	"github.com/cosmin2dor/vakt/internal/engine/vault"
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

	// Loc is the timezone every datetime/cron evaluation is done in
	// (SDD.md G9). Nil defaults to time.Local, mirroring cmd/vaultdebug.
	Loc *time.Location
}

// NewServer builds the production mux: the config stores, the dispatch
// registry with the real ios_notifications module registered, the live
// vault index driving the read endpoints, and every route main.go serves
// today. ctx governs the index's background reconciliation loop (Engine.Run)
// — it is not tied to any one request's lifetime.
func NewServer(ctx context.Context, cfg ServerConfig) (http.Handler, error) {
	loc := cfg.Loc
	if loc == nil {
		loc = time.Local
	}

	// A fresh registry: nothing in the API layer writes to the vault yet,
	// so nothing else needs to share it. A future writeback endpoint must
	// reuse this same registry (echo suppression, SDD.md §2.2) — keep it
	// a local var here rather than burying it, so lifting it into
	// ServerConfig later is a small change.
	reg := vault.NewHashRegistry()
	eng, err := engine.New(cfg.VaultDir, loc, reg)
	if err != nil {
		return nil, fmt.Errorf("api: constructing engine: %w", err)
	}
	go func() {
		if err := eng.Run(ctx); err != nil {
			log.Printf("api: engine run loop exited: %v", err)
		}
	}()

	// Shares reg with eng so writeback's own writes are echo-suppressed in
	// the index's reconciliation, same as any other vault mutation.
	writer := vault.NewWriter(reg)
	orch := schedule.NewOrchestrator(writer, cfg.VaultDir, loc)

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
	pushModule, err := webpush.New(privateKey, cfg.VAPIDContact, subscriptions, subscriptions, nil)
	if err != nil {
		return nil, fmt.Errorf("api: constructing ios_notifications module: %w", err)
	}
	if err := registry.Register(pushModule); err != nil {
		return nil, fmt.Errorf("api: registering ios_notifications module: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", HealthzHandler)
	mux.HandleFunc("GET /api/v1/tasks", ListTasksHandler(eng, loc))
	mux.HandleFunc("POST /api/v1/tasks", CreateTaskHandler(eng, writer, cfg.VaultDir, loc))
	mux.HandleFunc("GET /api/v1/tasks/{id}", GetTaskHandler(eng, loc))
	mux.HandleFunc("POST /api/v1/tasks/{id}/trigger", TriggerHandler(cfg.VaultDir, registry))
	mux.HandleFunc("POST /api/v1/tasks/{id}/fulfill", FulfillTaskHandler(eng, orch, loc))
	mux.HandleFunc("POST /api/v1/tasks/{id}/pause", PauseTaskHandler(eng, orch, loc))
	mux.HandleFunc("POST /api/v1/tasks/{id}/resume", ResumeTaskHandler(eng, orch, loc))
	mux.HandleFunc("POST /api/v1/tasks/{id}/skip", SkipTaskHandler(eng, orch, loc))
	mux.HandleFunc("/api/v1/subscriptions", CreateSubscriptionHandler(subscriptions))
	mux.HandleFunc("/api/v1/subscriptions/unsubscribe", DeleteSubscriptionHandler(subscriptions))
	mux.HandleFunc("/api/v1/vapid-public-key", VAPIDPublicKeyHandler(vapid))
	mux.Handle("/", StaticHandler(cfg.WebDir))

	return mux, nil
}
