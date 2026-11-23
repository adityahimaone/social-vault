package api

import (
	"errors"
	"net/http"

	"social-vault/internal/db"
)

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTags(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tags")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": tags})
}

func (s *Server) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tag, err := s.store.CreateTag(r.Context(), body.Name)
	if err != nil {
		if errors.Is(err, db.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create tag")
		return
	}
	writeJSON(w, http.StatusCreated, tag)
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tag id")
		return
	}
	if err := s.store.DeleteTag(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "tag not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete tag")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleListLists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.store.ListLists(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list collections")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": lists})
}

func (s *Server) handleCreateList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := s.store.CreateList(r.Context(), body.Name, body.Description)
	if err != nil {
		if errors.Is(err, db.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, db.ErrDuplicate) {
			writeError(w, http.StatusConflict, "list already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create list")
		return
	}
	writeJSON(w, http.StatusCreated, list)
}

func (s *Server) handleUpdateList(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid list id")
		return
	}
	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := s.store.UpdateList(r.Context(), id, body.Name, body.Description)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "list not found")
			return
		}
		if errors.Is(err, db.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, db.ErrDuplicate) {
			writeError(w, http.StatusConflict, "list already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update list")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleDeleteList(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid list id")
		return
	}
	if err := s.store.DeleteList(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "list not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete list")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
