package api

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"social-vault/internal/auth"
	"social-vault/internal/crawler"
	"social-vault/internal/db"
	"social-vault/internal/tagging"
)

const maxJSONBodyBytes = 1 << 20 // 1 MiB

type Server struct {
	store   *db.Store
	auth    *auth.Authenticator
	fetcher *crawler.Fetcher
	webFS   fs.FS
	tagger  *tagging.Service
	mux     *http.ServeMux
}

func NewServer(store *db.Store, authenticator *auth.Authenticator, fetcher *crawler.Fetcher, webFS fs.FS, tagger *tagging.Service) *Server {
	if tagger == nil {
		tagger = tagging.NewService(store, nil)
	}
	s := &Server{
		store:   store,
		auth:    authenticator,
		fetcher: fetcher,
		webFS:   webFS,
		tagger:  tagger,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	// Public routes
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/auth/me", s.handleAuthMe)
	s.mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/auth/logout", s.handleLogout)

	// Protected API routes
	protect := s.auth.Middleware
	s.mux.Handle("GET /api/stats", protect(http.HandlerFunc(s.handleStats)))
	s.mux.Handle("GET /api/bookmarks", protect(http.HandlerFunc(s.handleListBookmarks)))
	s.mux.Handle("POST /api/bookmarks", protect(http.HandlerFunc(s.handleCreateBookmark)))
	s.mux.Handle("GET /api/bookmarks/{id}", protect(http.HandlerFunc(s.handleGetBookmark)))
	s.mux.Handle("PATCH /api/bookmarks/{id}", protect(http.HandlerFunc(s.handleUpdateBookmark)))
	s.mux.Handle("DELETE /api/bookmarks/{id}", protect(http.HandlerFunc(s.handleDeleteBookmark)))

	s.mux.Handle("GET /api/tags", protect(http.HandlerFunc(s.handleListTags)))
	s.mux.Handle("POST /api/tags", protect(http.HandlerFunc(s.handleCreateTag)))
	s.mux.Handle("DELETE /api/tags/{id}", protect(http.HandlerFunc(s.handleDeleteTag)))

	s.mux.Handle("GET /api/lists", protect(http.HandlerFunc(s.handleListLists)))
	s.mux.Handle("POST /api/lists", protect(http.HandlerFunc(s.handleCreateList)))
	s.mux.Handle("PATCH /api/lists/{id}", protect(http.HandlerFunc(s.handleUpdateList)))
	s.mux.Handle("DELETE /api/lists/{id}", protect(http.HandlerFunc(s.handleDeleteList)))

	s.mux.Handle("POST /api/import", protect(http.HandlerFunc(s.handleImport)))

	s.mux.Handle("GET /api/tag-review", protect(http.HandlerFunc(s.handleListTagReview)))
	s.mux.Handle("POST /api/tag-review/{id}/apply", protect(http.HandlerFunc(s.handleApplyTagReview)))
	s.mux.Handle("POST /api/tag-review/{id}/dismiss", protect(http.HandlerFunc(s.handleDismissTagReview)))
	s.mux.Handle("POST /api/tagging/backfill", protect(http.HandlerFunc(s.handleBackfillTagging)))
	s.mux.Handle("GET /api/tagging/status", protect(http.HandlerFunc(s.handleTaggingStatus)))

	s.mux.Handle("POST /api/nsfw/unlock", protect(http.HandlerFunc(s.handleNsfwUnlock)))
	s.mux.Handle("POST /api/nsfw/lock", protect(http.HandlerFunc(s.handleNsfwLock)))
	s.mux.Handle("GET /api/nsfw/status", protect(http.HandlerFunc(s.handleNsfwStatus)))

	// Return 404 JSON for unmatched /api/* routes, SPA static assets otherwise
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "endpoint not found")
	})
	s.mux.HandleFunc("/", s.handleSPA)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":       s.auth.IsAuthenticated(r),
		"password_configured": s.auth.PasswordConfigured(),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.auth.PasswordConfigured() {
		writeError(w, http.StatusServiceUnavailable, "VAULT_PASSWORD is not configured on the server")
		return
	}
	if !s.auth.VerifyPassword(body.Password) {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}
	if err := s.auth.CreateSession(r.Context(), w, r); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.ClearSession(r.Context(), w, r)
	s.auth.ClearNsfwUnlocked(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if s.webFS == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cleaned := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if cleaned == "" {
		cleaned = "index.html"
	}

	if info, err := fs.Stat(s.webFS, cleaned); err == nil && !info.IsDir() {
		http.FileServer(http.FS(s.webFS)).ServeHTTP(w, r)
		return
	}

	indexBytes, err := fs.ReadFile(s.webFS, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(indexBytes)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is empty")
		}
		return errors.New("invalid JSON payload")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
