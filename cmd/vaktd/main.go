// Command vaktd is the Vakt daemon: it serves the API and, once built, the
// PWA bundle from a single origin, watches the vault, and drives the
// scheduler and dispatch pipeline described in SDD.md §2.
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("VAKT_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	// no op for now

	log.Printf("vaktd listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
