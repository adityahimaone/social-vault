package api

import (
	"net/http"
	"strings"

	"social-vault/internal/db"
)

func isNsfwTagName(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "nsfw")
}

func hasNsfwTagName(tags []string) bool {
	for _, t := range tags {
		if isNsfwTagName(t) {
			return true
		}
	}
	return false
}

func bookmarkHasNsfwTag(b *db.Bookmark) bool {
	if b == nil {
		return false
	}
	for _, t := range b.Tags {
		if isNsfwTagName(t.Name) {
			return true
		}
	}
	return false
}

func (s *Server) handleNsfwUnlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.auth.NsfwConfigured() {
		writeError(w, http.StatusServiceUnavailable, "NSFW lock tidak dikonfigurasi")
		return
	}
	if !s.auth.VerifyNsfwPassword(body.Password) {
		writeError(w, http.StatusUnauthorized, "Password salah")
		return
	}
	s.auth.SetNsfwUnlocked(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"unlocked": true})
}

func (s *Server) handleNsfwLock(w http.ResponseWriter, r *http.Request) {
	s.auth.ClearNsfwUnlocked(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"locked": true})
}

func (s *Server) handleNsfwStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"configured": s.auth.NsfwConfigured(),
		"locked":     !s.auth.IsNsfwUnlocked(r),
	})
}
