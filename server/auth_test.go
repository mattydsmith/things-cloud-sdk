package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolveAuthSecret(t *testing.T) {
	t.Run("returns empty when unset", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "")
		t.Setenv("API_KEY", "")

		if got := resolveAuthSecret(); got != "" {
			t.Fatalf("resolveAuthSecret() = %q, want empty", got)
		}
	})

	t.Run("falls back to API_KEY", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "")
		t.Setenv("API_KEY", "legacy-secret")

		if got, want := resolveAuthSecret(), "legacy-secret"; got != want {
			t.Fatalf("resolveAuthSecret() = %q, want %q", got, want)
		}
	})

	t.Run("AUTH_SECRET takes precedence", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "new-secret")
		t.Setenv("API_KEY", "legacy-secret")

		if got, want := resolveAuthSecret(), "new-secret"; got != want {
			t.Fatalf("resolveAuthSecret() = %q, want %q", got, want)
		}
	})
}

func TestSessionCookieValidation(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	value := newSessionCookieValue(secret, time.Unix(1_700_000_000, 0))

	if !isValidSessionCookieValue(secret, value, time.Unix(1_700_000_010, 0)) {
		t.Fatal("expected session cookie to validate before expiry")
	}
	if isValidSessionCookieValue(secret, value, time.Unix(1_900_000_000, 0)) {
		t.Fatal("expected expired session cookie to be rejected")
	}
	if isValidSessionCookieValue("wrong-secret", value, time.Unix(1_700_000_010, 0)) {
		t.Fatal("expected session cookie signed with another secret to be rejected")
	}
}

func TestAuthMiddleware(t *testing.T) {
	protected := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("allows through when no auth secret configured", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "")
		t.Setenv("API_KEY", "")

		req := httptest.NewRequest(http.MethodGet, "/api/tasks/today", nil)
		rec := httptest.NewRecorder()
		protected(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
		}
	})

	t.Run("accepts bearer auth", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("API_KEY", "")

		req := httptest.NewRequest(http.MethodGet, "/api/tasks/today", nil)
		req.Header.Set("Authorization", "Bearer test-secret")
		rec := httptest.NewRecorder()
		protected(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
		}
	})

	t.Run("accepts signed session cookie", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("API_KEY", "")

		req := httptest.NewRequest(http.MethodGet, "/api/tasks/today", nil)
		req.AddCookie(&http.Cookie{
			Name:  sessionCookieName,
			Value: newSessionCookieValue("test-secret", time.Now()),
		})
		rec := httptest.NewRecorder()
		protected(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
		}
	})

	t.Run("rejects request without bearer or session when secret configured", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("API_KEY", "")

		req := httptest.NewRequest(http.MethodGet, "/api/tasks/today", nil)
		rec := httptest.NewRecorder()
		protected(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
		}
	})
}

func TestHandleAuthSessionAndLogin(t *testing.T) {
	t.Run("session is open when no secret configured", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "")
		t.Setenv("API_KEY", "")

		req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
		rec := httptest.NewRecorder()
		handleAuthSession(rec, req)

		var resp authSessionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !resp.Authenticated || resp.AuthRequired {
			t.Fatalf("unexpected session response: %+v", resp)
		}
	})

	t.Run("login sets a cookie and session becomes authenticated", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("API_KEY", "")

		loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"test-secret"}`))
		loginRec := httptest.NewRecorder()
		handleAuthLogin(loginRec, loginReq)

		if loginRec.Code != http.StatusOK {
			t.Fatalf("expected login status 200, got %d", loginRec.Code)
		}
		cookies := loginRec.Result().Cookies()
		if len(cookies) == 0 {
			t.Fatal("expected login response to set a cookie")
		}

		sessionReq := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
		sessionReq.AddCookie(cookies[0])
		sessionRec := httptest.NewRecorder()
		handleAuthSession(sessionRec, sessionReq)

		var resp authSessionResponse
		if err := json.Unmarshal(sessionRec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !resp.Authenticated || !resp.AuthRequired {
			t.Fatalf("unexpected session response after login: %+v", resp)
		}
	})

	t.Run("login rejects wrong password", func(t *testing.T) {
		t.Setenv("AUTH_SECRET", "test-secret")
		t.Setenv("API_KEY", "")

		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"wrong"}`))
		rec := httptest.NewRecorder()
		handleAuthLogin(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
		}
	})
}

func TestHandleTaskByUUIDRejectsInvalidUUID(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/tasks/not-a-real-uuid", nil)
	req.SetPathValue("uuid", "not-a-real-uuid")
	rec := httptest.NewRecorder()

	handleTaskByUUID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleProjectTasksRejectsInvalidUUID(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/not-a-real-uuid/tasks", nil)
	req.SetPathValue("uuid", "not-a-real-uuid")
	rec := httptest.NewRecorder()

	handleProjectTasks(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleAreaByUUIDRejectsInvalidUUID(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/areas/not-a-real-uuid", nil)
	req.SetPathValue("uuid", "not-a-real-uuid")
	rec := httptest.NewRecorder()

	handleAreaByUUID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleAreaTasksRejectsInvalidUUID(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/api/areas/not-a-real-uuid/tasks", nil)
	req.SetPathValue("uuid", "not-a-real-uuid")
	rec := httptest.NewRecorder()

	handleAreaTasks(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestHandleCreateTaskRejectsInvalidAreaUUID(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/api/tasks/create", strings.NewReader(`{"title":"test","area":"not-a-real-uuid"}`))
	rec := httptest.NewRecorder()

	handleCreateTask(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}
