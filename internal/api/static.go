package api

import "net/http"

// HealthzHandler reports basic liveness. It has no dependency on the
// engine — a 200 here means only that the HTTP server is accepting
// connections.
func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// StaticHandler serves the built PWA bundle from dir, so the Go binary can
// answer both the API and the frontend from a single origin (SDD.md
// §2.3). dir is conventionally "web/dist" — the frontend's own build
// tooling (create-frontend-scaffold) decides what lands there; this
// handler works whether that directory is fully built, still empty, or
// missing entirely, since http.FileServer only touches disk per-request.
func StaticHandler(dir string) http.Handler {
	return http.FileServer(http.Dir(dir))
}
