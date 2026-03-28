package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveAuthToken(t *testing.T) {
	tests := []struct {
		name       string
		authSecret string
		apiKey     string
		want       string
	}{
		{name: "none configured", want: ""},
		{name: "api key only", apiKey: "api-key", want: "api-key"},
		{name: "auth secret only", authSecret: "auth-secret", want: "auth-secret"},
		{name: "auth secret wins over api key", authSecret: "auth-secret", apiKey: "api-key", want: "auth-secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AUTH_SECRET", tt.authSecret)
			t.Setenv("API_KEY", tt.apiKey)

			if got := resolveAuthToken(); got != tt.want {
				t.Fatalf("resolveAuthToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasConfiguredAuthToken(t *testing.T) {
	tests := []struct {
		name       string
		authSecret string
		apiKey     string
		want       bool
	}{
		{name: "none configured", want: false},
		{name: "api key only", apiKey: "api-key", want: true},
		{name: "auth secret only", authSecret: "auth-secret", want: true},
		{name: "both configured", authSecret: "auth-secret", apiKey: "api-key", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AUTH_SECRET", tt.authSecret)
			t.Setenv("API_KEY", tt.apiKey)

			if got := hasConfiguredAuthToken(); got != tt.want {
				t.Fatalf("hasConfiguredAuthToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		authSecret  string
		apiKey      string
		authHeader  string
		wantStatus  int
		wantInvoked bool
	}{
		{name: "open when no token configured", wantStatus: http.StatusNoContent, wantInvoked: true},
		{name: "api key only requires auth", apiKey: "api-key", wantStatus: http.StatusUnauthorized},
		{name: "api key only accepts matching bearer", apiKey: "api-key", authHeader: "Bearer api-key", wantStatus: http.StatusNoContent, wantInvoked: true},
		{name: "auth secret only accepts matching bearer", authSecret: "auth-secret", authHeader: "Bearer auth-secret", wantStatus: http.StatusNoContent, wantInvoked: true},
		{name: "auth secret wins when both configured", authSecret: "auth-secret", apiKey: "api-key", authHeader: "Bearer api-key", wantStatus: http.StatusUnauthorized},
		{name: "auth secret accepted when both configured", authSecret: "auth-secret", apiKey: "api-key", authHeader: "Bearer auth-secret", wantStatus: http.StatusNoContent, wantInvoked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AUTH_SECRET", tt.authSecret)
			t.Setenv("API_KEY", tt.apiKey)

			invoked := false
			handler := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
				invoked = true
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if invoked != tt.wantInvoked {
				t.Fatalf("next invoked = %v, want %v", invoked, tt.wantInvoked)
			}
		})
	}
}

func TestAuthHandlerMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		authSecret  string
		apiKey      string
		authHeader  string
		wantStatus  int
		wantInvoked bool
	}{
		{name: "open when no token configured", wantStatus: http.StatusNoContent, wantInvoked: true},
		{name: "missing auth rejected when token configured", authSecret: "auth-secret", wantStatus: http.StatusUnauthorized},
		{name: "matching auth accepted", authSecret: "auth-secret", authHeader: "Bearer auth-secret", wantStatus: http.StatusNoContent, wantInvoked: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AUTH_SECRET", tt.authSecret)
			t.Setenv("API_KEY", tt.apiKey)

			invoked := false
			handler := authHandlerMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				invoked = true
				w.WriteHeader(http.StatusNoContent)
			}))

			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if invoked != tt.wantInvoked {
				t.Fatalf("next invoked = %v, want %v", invoked, tt.wantInvoked)
			}
		})
	}
}

func TestDebugAuthMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		debug       string
		authSecret  string
		apiKey      string
		authHeader  string
		wantStatus  int
		wantInvoked bool
	}{
		{name: "debug disabled returns not found", wantStatus: http.StatusNotFound},
		{name: "debug enabled requires configured token", debug: "true", wantStatus: http.StatusServiceUnavailable},
		{name: "debug enabled rejects missing auth", debug: "true", authSecret: "auth-secret", wantStatus: http.StatusUnauthorized},
		{name: "debug enabled accepts auth secret", debug: "true", authSecret: "auth-secret", authHeader: "Bearer auth-secret", wantStatus: http.StatusNoContent, wantInvoked: true},
		{name: "debug enabled prefers auth secret over api key", debug: "true", authSecret: "auth-secret", apiKey: "api-key", authHeader: "Bearer api-key", wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DEBUG", tt.debug)
			t.Setenv("AUTH_SECRET", tt.authSecret)
			t.Setenv("API_KEY", tt.apiKey)

			invoked := false
			handler := debugAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
				invoked = true
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/debug/history", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if invoked != tt.wantInvoked {
				t.Fatalf("next invoked = %v, want %v", invoked, tt.wantInvoked)
			}
		})
	}
}
