package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

const (
	nsfwCookieName = "sv_nsfw"
	nsfwHMACInput  = "nsfw-unlock"
)

// WithNsfwPassword configures the password lock for the nsfw tag without storing the plaintext password.
func (a *Authenticator) WithNsfwPassword(password string) *Authenticator {
	if password == "" {
		a.nsfwHash = [32]byte{}
		a.nsfwToken = ""
		a.nsfwConfigured = false
		return a
	}

	a.nsfwHash = sha256.Sum256([]byte(password))
	mac := hmac.New(sha256.New, []byte(password))
	_, _ = mac.Write([]byte(nsfwHMACInput))
	a.nsfwToken = hex.EncodeToString(mac.Sum(nil))
	a.nsfwConfigured = true
	return a
}

// NsfwConfigured reports whether NSFW_TAG_PASSWORD is set.
func (a *Authenticator) NsfwConfigured() bool {
	return a.nsfwConfigured
}

// VerifyNsfwPassword checks candidate against the stored SHA-256 password hash in constant time.
func (a *Authenticator) VerifyNsfwPassword(candidate string) bool {
	if !a.nsfwConfigured || candidate == "" {
		return false
	}
	sum := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(a.nsfwHash[:], sum[:]) == 1
}

// IsNsfwUnlocked reports whether the request may view and manage nsfw-tagged content.
func (a *Authenticator) IsNsfwUnlocked(r *http.Request) bool {
	if a.VerifyAPIKey(r) {
		return true
	}
	if !a.nsfwConfigured || a.nsfwToken == "" {
		return false
	}
	c, err := r.Cookie(nsfwCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(a.nsfwToken)) == 1
}

// SetNsfwUnlocked sets a session-scoped HttpOnly cookie carrying the HMAC unlock token.
func (a *Authenticator) SetNsfwUnlocked(w http.ResponseWriter, r *http.Request) {
	if !a.nsfwConfigured || a.nsfwToken == "" {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     nsfwCookieName,
		Value:    a.nsfwToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
	})
}

// ClearNsfwUnlocked clears the nsfw unlock cookie.
func (a *Authenticator) ClearNsfwUnlocked(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     nsfwCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS(r),
		MaxAge:   -1,
	})
}
