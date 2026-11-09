package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthz(t *testing.T) {
	clock := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	s := New(slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return clock })
	clock = clock.Add(90 * time.Second)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"GET is served", http.MethodGet, "/healthz", http.StatusOK},
		{"HEAD is served with GET", http.MethodHead, "/healthz", http.StatusOK},
		{"POST is refused", http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{"unknown path", http.MethodGet, "/nope", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var got health
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got != (health{Status: "ok", UptimeSeconds: 90}) {
		t.Errorf("body = %+v", got)
	}
}
