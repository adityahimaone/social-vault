package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"social-vault/internal/db"
	"social-vault/internal/tagging"
)

func (s *Server) handleListTagReview(w http.ResponseWriter, r *http.Request) {
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	if limit <= 0 {
		writeError(w, http.StatusBadRequest, "limit must be a positive integer")
		return
	}

	excludeNsfw := !s.auth.IsNsfwUnlocked(r)
	items, err := s.store.ListPendingTagReviewsFiltered(r.Context(), limit, excludeNsfw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tag review queue")
		return
	}
	total, err := s.store.CountPendingTagReviewsFiltered(r.Context(), excludeNsfw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count tag review queue")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": total,
	})
}

func (s *Server) handleApplyTagReview(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tag review id")
		return
	}

	var body struct {
		Tags []string `json:"tags"`
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

	item, bookmark, err := s.store.ApplyTagReviewWithTags(r.Context(), id, body.Tags, !s.auth.IsNsfwUnlocked(r))
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "tag review item not found")
		case errors.Is(err, db.ErrAlreadyResolved):
			writeError(w, http.StatusConflict, "tag review item is already resolved")
		case errors.Is(err, db.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
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

	item, err := s.store.DismissTagReviewFiltered(r.Context(), id, !s.auth.IsNsfwUnlocked(r))
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

func (s *Server) handleBatchTagReview(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string  `json:"action"`
		IDs    []int64 `json:"ids"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	action := strings.ToLower(strings.TrimSpace(body.Action))
	if action != "apply" && action != "dismiss" {
		writeError(w, http.StatusBadRequest, "action must be 'apply' or 'dismiss'")
		return
	}
	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids cannot be empty")
		return
	}
	if len(body.IDs) > 200 {
		writeError(w, http.StatusBadRequest, "maximum 200 ids per batch request")
		return
	}

	excludeNsfw := !s.auth.IsNsfwUnlocked(r)
	seen := make(map[int64]struct{}, len(body.IDs))
	processedIDs := make([]int64, 0, len(body.IDs))
	var succeeded, skipped, failed int

	for _, id := range body.IDs {
		if id <= 0 {
			skipped++
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		var err error
		if action == "apply" {
			_, _, err = s.store.ApplyTagReviewWithTags(r.Context(), id, nil, excludeNsfw)
		} else {
			_, err = s.store.DismissTagReviewFiltered(r.Context(), id, excludeNsfw)
		}

		if err == nil {
			succeeded++
			processedIDs = append(processedIDs, id)
		} else if errors.Is(err, db.ErrNotFound) || errors.Is(err, db.ErrAlreadyResolved) {
			skipped++
		} else {
			failed++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"action":        action,
		"succeeded":     succeeded,
		"skipped":       skipped,
		"failed":        failed,
		"processed_ids": processedIDs,
	})
}

func (s *Server) handleBackfillTagging(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Limit  int      `json:"limit"`
		Layers []string `json:"layers"`
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

	result, err := s.tagger.Backfill(r.Context(), limit, body.Layers...)
	if err != nil {
		switch {
		case errors.Is(err, tagging.ErrInvalidLayer), errors.Is(err, tagging.ErrJevDisabled):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, tagging.ErrBackfillRunning):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to backfill bookmark tags")
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSuggestUntagged(w http.ResponseWriter, r *http.Request) {
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
		limit = 100
	}

	result, err := s.tagger.SuggestUntagged(r.Context(), limit)
	if err != nil {
		switch {
		case errors.Is(err, tagging.ErrJevDisabled):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, tagging.ErrBackfillRunning):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to suggest tags for untagged bookmarks")
		}
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
