package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	mcpInitializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	mcpToolsListBody  = `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
)

func performMCPRequest(t *testing.T, handler http.Handler, body, authHeader string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestMCPHandlerAllowsUnauthenticatedAccessWhenNoTokenConfigured(t *testing.T) {
	t.Setenv("AUTH_SECRET", "")
	t.Setenv("API_KEY", "")

	rec := performMCPRequest(t, authHandlerMiddleware(newMCPHandler()), mcpInitializeBody, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Result struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Result.ServerInfo.Name != "Things Cloud" {
		t.Fatalf("server name = %q, want %q", resp.Result.ServerInfo.Name, "Things Cloud")
	}
}

func TestMCPHandlerRequiresAuthorizationWhenTokenConfigured(t *testing.T) {
	t.Setenv("AUTH_SECRET", "auth-secret")
	t.Setenv("API_KEY", "")

	rec := performMCPRequest(t, authHandlerMiddleware(newMCPHandler()), mcpInitializeBody, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMCPHandlerAcceptsAuthorizedRequests(t *testing.T) {
	t.Setenv("AUTH_SECRET", "auth-secret")
	t.Setenv("API_KEY", "")

	handler := authHandlerMiddleware(newMCPHandler())

	initRec := performMCPRequest(t, handler, mcpInitializeBody, "Bearer auth-secret")
	if initRec.Code != http.StatusOK {
		t.Fatalf("initialize status = %d, want %d", initRec.Code, http.StatusOK)
	}

	listRec := performMCPRequest(t, handler, mcpToolsListBody, "Bearer auth-secret")
	if listRec.Code != http.StatusOK {
		t.Fatalf("tools/list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Result.Tools) == 0 {
		t.Fatal("expected tools/list to return at least one tool")
	}
}

func TestMCPHandlerPrefersAuthSecretOverAPIKey(t *testing.T) {
	t.Setenv("AUTH_SECRET", "auth-secret")
	t.Setenv("API_KEY", "api-key")

	handler := authHandlerMiddleware(newMCPHandler())

	rec := performMCPRequest(t, handler, mcpInitializeBody, "Bearer api-key")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("api key status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	rec = performMCPRequest(t, handler, mcpInitializeBody, "Bearer auth-secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("auth secret status = %d, want %d", rec.Code, http.StatusOK)
	}
}
