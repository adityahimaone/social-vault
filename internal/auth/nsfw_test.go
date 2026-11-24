package auth_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"social-vault/internal/auth"
	"social-vault/internal/db"
)

func expectedNsfwHMAC(password string) string {
	mac := hmac.New(sha256.New, []byte(password))
	_, _ = mac.Write([]byte("nsfw-unlock"))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestNsfwPasswordAndCookieLock(t *testing.T) {
	store, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	const dummyPass = "test-nsfw-pass"
	a := auth.New(store, "vault-pass", "api-key-123").WithNsfwPassword(dummyPass)

	if !a.NsfwConfigured() {
		t.Fatalf("expected NsfwConfigured() to be true")
	}
	if !a.VerifyNsfwPassword(dummyPass) {
		t.Fatalf("expected valid nsfw password to verify")
	}
	if a.VerifyNsfwPassword("wrong-pass") {
		t.Fatalf("expected wrong nsfw password to fail")
	}
	if a.VerifyNsfwPassword("") {
		t.Fatalf("expected empty nsfw password to fail")
	}

	// SetNsfwUnlocked over HTTP sets session cookie with HMAC token (never bare "1").
	reqHTTP := httptest.NewRequest(http.MethodPost, "/api/nsfw/unlock", nil)
	recHTTP := httptest.NewRecorder()
	a.SetNsfwUnlocked(recHTTP, reqHTTP)
	cookies := recHTTP.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	wantToken := expectedNsfwHMAC(dummyPass)
	if c.Name != "sv_nsfw" || c.Path != "/" || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 0 || c.Secure {
		t.Fatalf("unexpected HTTP cookie attributes: %+v", c)
	}
	if c.Value != wantToken || c.Value == "1" {
		t.Fatalf("unexpected cookie token %q, want %q", c.Value, wantToken)
	}

	// X-Forwarded-Proto: https sets Secure: true.
	reqHTTPS := httptest.NewRequest(http.MethodPost, "/api/nsfw/unlock", nil)
	reqHTTPS.Header.Set("X-Forwarded-Proto", "https")
	recHTTPS := httptest.NewRecorder()
	a.SetNsfwUnlocked(recHTTPS, reqHTTPS)
	if httpsCookies := recHTTPS.Result().Cookies(); len(httpsCookies) != 1 || !httpsCookies[0].Secure {
		t.Fatalf("expected Secure=true cookie on HTTPS, got %+v", httpsCookies)
	}

	// Valid cookie -> unlocked.
	checkReq := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	checkReq.AddCookie(c)
	if !a.IsNsfwUnlocked(checkReq) {
		t.Fatalf("expected valid sv_nsfw cookie to unlock")
	}

	// Missing cookie -> locked.
	if a.IsNsfwUnlocked(httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)) {
		t.Fatalf("expected request without cookie to stay locked")
	}

	// Bare "1" cookie -> rejected.
	bareReq := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	bareReq.AddCookie(&http.Cookie{Name: "sv_nsfw", Value: "1"})
	if a.IsNsfwUnlocked(bareReq) {
		t.Fatalf("expected bare '1' cookie to be rejected")
	}

	// Tampered HMAC cookie -> rejected.
	lastChar := wantToken[len(wantToken)-1]
	replacement := "0"
	if lastChar == '0' {
		replacement = "1"
	}
	tampered := wantToken[:len(wantToken)-1] + replacement
	tamperedReq := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	tamperedReq.AddCookie(&http.Cookie{Name: "sv_nsfw", Value: tampered})
	if a.IsNsfwUnlocked(tamperedReq) {
		t.Fatalf("expected tampered sv_nsfw cookie to be rejected")
	}

	// API key always bypasses lock even without sv_nsfw cookie.
	bearerReq := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	bearerReq.Header.Set("Authorization", "Bearer api-key-123")
	if !a.IsNsfwUnlocked(bearerReq) {
		t.Fatalf("expected Bearer API key to bypass nsfw lock")
	}

	xKeyReq := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	xKeyReq.Header.Set("X-Api-Key", "api-key-123")
	if !a.IsNsfwUnlocked(xKeyReq) {
		t.Fatalf("expected X-Api-Key to bypass nsfw lock")
	}

	// ClearNsfwUnlocked sets MaxAge: -1 and empty value.
	clearRec := httptest.NewRecorder()
	a.ClearNsfwUnlocked(clearRec, reqHTTP)
	cleared := clearRec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != "sv_nsfw" || cleared[0].Value != "" || cleared[0].MaxAge != -1 {
		t.Fatalf("expected cleared sv_nsfw cookie with MaxAge=-1, got %+v", cleared)
	}
}

func TestNsfwUnconfiguredFailClosed(t *testing.T) {
	store, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	a := auth.New(store, "vault-pass", "api-key-123").WithNsfwPassword("")
	if a.NsfwConfigured() {
		t.Fatalf("expected NsfwConfigured() to be false when empty")
	}
	if a.VerifyNsfwPassword("") || a.VerifyNsfwPassword("test-nsfw-pass") {
		t.Fatalf("expected VerifyNsfwPassword to return false when unconfigured")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	req.AddCookie(&http.Cookie{Name: "sv_nsfw", Value: expectedNsfwHMAC("test-nsfw-pass")})
	if a.IsNsfwUnlocked(req) {
		t.Fatalf("expected cookie request to fail closed when unconfigured")
	}

	// API key still bypasses even when NSFW password is not configured.
	apiReq := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	apiReq.Header.Set("Authorization", "Bearer api-key-123")
	if !a.IsNsfwUnlocked(apiReq) {
		t.Fatalf("expected API key to bypass even when unconfigured")
	}
}
