package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookieName = "things_session"
	sessionTTL        = 30 * 24 * time.Hour
)

type loginRequest struct {
	Password string `json:"password"`
}

type authSessionResponse struct {
	Authenticated bool `json:"authenticated"`
	AuthRequired  bool `json:"auth_required"`
}

func resolveAuthSecret() string {
	if secret := os.Getenv("AUTH_SECRET"); secret != "" {
		return secret
	}
	return os.Getenv("API_KEY")
}

func hasConfiguredAuthSecret() bool {
	return resolveAuthSecret() != ""
}

func requestHasBearerAuth(r *http.Request, secret string) bool {
	return secret != "" && r.Header.Get("Authorization") == "Bearer "+secret
}

func signSession(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func newSessionCookieValue(secret string, now time.Time) string {
	expiresAt := strconv.FormatInt(now.Add(sessionTTL).Unix(), 10)
	return expiresAt + "." + signSession(secret, expiresAt)
}

func isValidSessionCookieValue(secret, value string, now time.Time) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return false
	}

	expiresAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || now.Unix() > expiresAt {
		return false
	}

	expectedSig := signSession(secret, parts[0])
	return hmac.Equal([]byte(parts[1]), []byte(expectedSig))
}

func requestHasValidSession(r *http.Request, secret string) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return isValidSessionCookieValue(secret, cookie.Value, time.Now())
}

func requestIsAuthenticated(r *http.Request) bool {
	secret := resolveAuthSecret()
	if secret == "" {
		return true
	}
	return requestHasBearerAuth(r, secret) || requestHasValidSession(r, secret)
}

func authSessionState(r *http.Request) authSessionResponse {
	secret := resolveAuthSecret()
	if secret == "" {
		return authSessionResponse{Authenticated: true, AuthRequired: false}
	}
	return authSessionResponse{
		Authenticated: requestHasBearerAuth(r, secret) || requestHasValidSession(r, secret),
		AuthRequired:  true,
	}
}

func isHTTPSRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, secret string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    newSessionCookieValue(secret, time.Now()),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPSRequest(r),
		MaxAge:   int(sessionTTL / time.Second),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPSRequest(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requestIsAuthenticated(r) {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func authHandlerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requestIsAuthenticated(r) {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerOnlyAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret := resolveAuthSecret()
		if secret == "" {
			next(w, r)
			return
		}
		if !requestHasBearerAuth(r, secret) {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func debugAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("DEBUG") != "true" {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}

		secret := resolveAuthSecret()
		if secret == "" {
			jsonError(w, "debug endpoints require AUTH_SECRET or API_KEY", http.StatusServiceUnavailable)
			return
		}

		if !requestHasBearerAuth(r, secret) {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	secret := resolveAuthSecret()
	if secret == "" {
		jsonResponse(w, authSessionResponse{Authenticated: true, AuthRequired: false})
		return
	}

	var req loginRequest
	if err := decodeJSONBody(w, r, &req); err != nil {
		writeJSONDecodeError(w, "invalid JSON: ", err)
		return
	}
	if req.Password == "" {
		jsonError(w, "password is required", http.StatusBadRequest)
		return
	}
	if req.Password != secret {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	setSessionCookie(w, r, secret)
	jsonResponse(w, authSessionResponse{Authenticated: true, AuthRequired: true})
}

func handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clearSessionCookie(w, r)

	secret := resolveAuthSecret()
	if secret == "" {
		jsonResponse(w, authSessionResponse{Authenticated: true, AuthRequired: false})
		return
	}
	jsonResponse(w, authSessionResponse{Authenticated: false, AuthRequired: true})
}

func handleAuthSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jsonResponse(w, authSessionState(r))
}
