package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzGet(t *testing.T) {
	rec := httptest.NewRecorder()
	NewMux().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestHealthzRejectsPost(t *testing.T) {
	rec := httptest.NewRecorder()
	NewMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
