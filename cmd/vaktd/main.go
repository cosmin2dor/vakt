// Command vaktd is the Vakt daemon: it serves the API and, once built, the
// PWA bundle from a single origin, watches the vault, and drives the
// scheduler and dispatch pipeline described in SDD.md §2.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/cosmin2dor/vakt/internal/api"
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

	// VAKT_CONFIG_DIR points at the /config volume: system state (VAPID
	// keypair, push subscriptions) that must survive restarts but never
	// belongs in the vault (SDD.md §2.3).
	configDir := os.Getenv("VAKT_CONFIG_DIR")
	if configDir == "" {
		configDir = "config"
	}

	// VAKT_VAPID_CONTACT is the mailto: contact a push service can reach
	// operators at, per RFC 8292's "sub" claim. Not a secret; a household
	// deployment gets a working default if it's never set.
	vapidContact := os.Getenv("VAKT_VAPID_CONTACT")
	if vapidContact == "" {
		vapidContact = "mailto:vakt@localhost"
	}

	handler, err := api.NewServer(api.ServerConfig{
		WebDir:       webDir,
		VaultDir:     vaultDir,
		ConfigDir:    configDir,
		VAPIDContact: vapidContact,
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("vaktd listening on %s, serving frontend from %s", addr, webDir)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
