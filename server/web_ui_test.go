package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleRootWithoutWebUI(t *testing.T) {
	t.Setenv("WEB_UI", "")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handleRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected health response, got %q", rec.Body.String())
	}
}

func TestHandleRootWithoutWebUIReturns404ForOtherPaths(t *testing.T) {
	t.Setenv("WEB_UI", "")

	req := httptest.NewRequest(http.MethodGet, "/today", nil)
	rec := httptest.NewRecorder()

	handleRoot(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestHandleRootWithWebUI(t *testing.T) {
	t.Setenv("WEB_UI", "true")

	t.Run("serves index at root", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		handleRoot(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Ting") {
			t.Fatalf("expected HTML shell, got %q", rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store, max-age=0" {
			t.Fatalf("expected Cache-Control no-store, got %q", got)
		}
	})

	t.Run("serves client routes via index fallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/today", nil)
		rec := httptest.NewRecorder()

		handleRoot(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Ting") {
			t.Fatalf("expected HTML shell, got %q", rec.Body.String())
		}
	})

	t.Run("serves static assets", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		rec := httptest.NewRecorder()

		handleRoot(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "loadView") {
			t.Fatalf("expected app script, got %q", rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store, max-age=0" {
			t.Fatalf("expected Cache-Control no-store, got %q", got)
		}
	})
}
