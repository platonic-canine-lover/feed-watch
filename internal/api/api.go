// Package api is feedwatch's HTTP surface. It only reads: nothing here
// changes what the service checks or how.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Server holds what the handlers need. More fields arrive with /sources.
type Server struct {
	log     *slog.Logger
	started time.Time
	now     func() time.Time // a clock tests can control
}

// New returns a Server whose uptime counts from now.
func New(log *slog.Logger, now func() time.Time) *Server {
	if now == nil {
		now = time.Now
	}
	return &Server{log: log, started: now(), now: now}
}

// Handler routes requests. The method is part of each pattern (Go 1.22
// routing), so a POST to /healthz gets 405 without any code of ours.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	return mux
}

type health struct {
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptimeSeconds"`
}

// healthz says the process is up and serving. It does not say the feeds are
// healthy: that is /sources, and mixing the two would make a stalled feed
// look like a dead service to whatever restarts it.
func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.log, http.StatusOK, health{
		Status:        "ok",
		UptimeSeconds: int64(s.now().Sub(s.started).Seconds()),
	})
}

func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already sent; all that is left is to say so.
		log.Warn("writing response", "err", err)
	}
}
