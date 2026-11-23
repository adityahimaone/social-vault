package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"social-vault/internal/auth"
	"social-vault/internal/db"
)

func TestAuthMiddlewareAndSessions(t *testing.T) {
	store, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	a := auth.New(store, "super-secret-pass", "api-key-123")
	protected := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Unauthenticated -> 401
	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}

	// Wrong API key -> 401
	req = httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong key, got %d", rec.Code)
	}

	// Valid Bearer API key -> 200
	req = httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	req.Header.Set("Authorization", "Bearer api-key-123")
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with bearer key, got %d", rec.Code)
	}

	// Valid X-Api-Key -> 200
	req = httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	req.Header.Set("X-Api-Key", "api-key-123")
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with X-Api-Key, got %d", rec.Code)
	}

	// Password verification
	if a.VerifyPassword("wrong") {
		t.Fatalf("expected wrong password to fail")
	}
	if !a.VerifyPassword("super-secret-pass") {
		t.Fatalf("expected valid password to pass")
	}

	// Session cookie round-trip
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	loginRec := httptest.NewRecorder()
	if err := a.CreateSession(loginReq.Context(), loginRec, loginReq); err != nil {
		t.Fatalf("create session: %v", err)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie")
	}

	authedReq := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	authedReq.AddCookie(cookies[0])
	authedRec := httptest.NewRecorder()
	protected.ServeHTTP(authedRec, authedReq)
	if authedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 with session cookie, got %d", authedRec.Code)
	}

	// Logout clears session
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(cookies[0])
	logoutRec := httptest.NewRecorder()
	a.ClearSession(logoutReq.Context(), logoutRec, logoutReq)

	afterLogoutReq := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	afterLogoutReq.AddCookie(cookies[0])
	afterLogoutRec := httptest.NewRecorder()
	protected.ServeHTTP(afterLogoutRec, afterLogoutReq)
	if afterLogoutRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", afterLogoutRec.Code)
	}
}
