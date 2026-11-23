package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"social-vault/internal/db"
)

const (
	sessionCookieName = "sv_session"
	sessionTTL        = 30 * 24 * time.Hour
)

type Authenticator struct {
	store              *db.Store
	passwordHash       [32]byte
	passwordConfigured bool
	apiKeyHash         [32]byte
	apiKeyConfigured   bool
}

func New(store *db.Store, password, apiKey string) *Authenticator {
	a := &Authenticator{store: store}
	if password != "" {
		a.passwordHash = sha256.Sum256([]byte(password))
		a.passwordConfigured = true
	}
	if apiKey != "" {
		a.apiKeyHash = sha256.Sum256([]byte(apiKey))
		a.apiKeyConfigured = true
	}
	return a
}

func (a *Authenticator) PasswordConfigured() bool {
	return a.passwordConfigured
}

func (a *Authenticator) VerifyPassword(candidate string) bool {
	if !a.passwordConfigured || candidate == "" {
		return false
	}
	sum := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(a.passwordHash[:], sum[:]) == 1
}

func (a *Authenticator) VerifyAPIKey(r *http.Request) bool {
	if !a.apiKeyConfigured {
		return false
	}
	candidate := extractAPIKey(r)
	if candidate == "" {
		return false
	}
	sum := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(a.apiKeyHash[:], sum[:]) == 1
}

func extractAPIKey(r *http.Request) string {
	if authz := strings.TrimSpace(r.Header.Get("Authorization")); authz != "" {
		if len(authz) > 7 && strings.EqualFold(authz[:7], "Bearer ") {
			return strings.TrimSpace(authz[7:])
		}
	}
	if key := strings.TrimSpace(r.Header.Get("X-Api-Key")); key != "" {
		return key
	}
	return ""
}

func (a *Authenticator) CreateSession(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := hex.EncodeToString(raw)
	hash := hashToken(token)

	if err := a.store.CreateSession(ctx, hash, sessionTTL); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
		MaxAge:   int(sessionTTL.Seconds()),
	})
	return nil
}

func (a *Authenticator) ClearSession(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		_ = a.store.DeleteSession(ctx, hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
		MaxAge:   -1,
	})
}

func (a *Authenticator) IsAuthenticated(r *http.Request) bool {
	if a.VerifyAPIKey(r) {
		return true
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(c.Value) == "" {
		return false
	}
	ok, err := a.store.ValidateSession(r.Context(), hashToken(c.Value))
	return err == nil && ok
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.IsAuthenticated(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
