package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"social-vault/internal/db"
	"social-vault/internal/tagging"
)

func (s *Server) handleListTagReview(w http.ResponseWriter, r *http.Request) {
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	if limit <= 0 {
		writeError(w, http.StatusBadRequest, "limit must be a positive integer")
		return
	}

	items, err := s.store.ListPendingTagReviews(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tag review queue")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

func (s *Server) handleApplyTagReview(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tag review id")
		return
	}

	item, bookmark, err := s.store.ApplyTagReview(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "tag review item not found")
		case errors.Is(err, db.ErrAlreadyResolved):
			writeError(w, http.StatusConflict, "tag review item is already resolved")
		default:
			writeError(w, http.StatusInternalServerError, "failed to apply tag review item")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"item":     item,
		"bookmark": bookmark,
	})
}

func (s *Server) handleDismissTagReview(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tag review id")
		return
	}

	item, err := s.store.DismissTagReview(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "tag review item not found")
		case errors.Is(err, db.ErrAlreadyResolved):
			writeError(w, http.StatusConflict, "tag review item is already resolved")
		default:
			writeError(w, http.StatusInternalServerError, "failed to dismiss tag review item")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"item": item,
	})
}

func (s *Server) handleBackfillTagging(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Limit int `json:"limit"`
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON payload")
			return
		}
	}

	if body.Limit < 0 {
		writeError(w, http.StatusBadRequest, "limit must be greater than or equal to 0")
		return
	}

	limit := body.Limit
	if limit == 0 {
		limit = tagging.DefaultBackfillLimit
	}

	result, err := s.tagger.Backfill(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to backfill bookmark tags")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleTaggingStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.tagger.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get tagging status")
		return
	}
	writeJSON(w, http.StatusOK, status)
}
