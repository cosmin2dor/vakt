// Command vaktd is the Vakt daemon: it serves the API and, once built, the
// PWA bundle from a single origin, watches the vault, and drives the
// scheduler and dispatch pipeline described in SDD.md §2.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/cosmin2dor/vakt/internal/api"
	"github.com/cosmin2dor/vakt/internal/engine/dispatch"
)

func main() {
	addr := os.Getenv("VAKT_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	// VAKT_WEB_DIR points at the built PWA bundle. "web/dist" is the
	// conventional Vite build output and is not yet produced by anything
	// in this repo (create-frontend-scaffold owns that); StaticHandler
	// degrades to 404s rather than failing to start when it's empty or
	// absent.
	webDir := os.Getenv("VAKT_WEB_DIR")
	if webDir == "" {
		webDir = "web/dist"
	}

	// VAKT_VAULT_DIR points at the Markdown vault, mirroring VAKT_WEB_DIR
	// above (docker-compose.yml mounts the host vault at /vault).
	vaultDir := os.Getenv("VAKT_VAULT_DIR")
	if vaultDir == "" {
		vaultDir = "vault"
	}

	// No real modules registered yet — ios_notifications lands in a
	// later, separate task and registers itself here additively.
	registry := dispatch.NewRegistry()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", api.HealthzHandler)
	mux.HandleFunc("POST /api/v1/tasks/{id}/trigger", api.TriggerHandler(vaultDir, registry))
	mux.Handle("/", api.StaticHandler(webDir))

	log.Printf("vaktd listening on %s, serving frontend from %s", addr, webDir)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
