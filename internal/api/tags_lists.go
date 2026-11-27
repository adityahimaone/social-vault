package api

import (
	"errors"
	"net/http"

	"social-vault/internal/db"
)

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetStatsFiltered(r.Context(), !s.auth.IsNsfwUnlocked(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.ListTagsFiltered(r.Context(), !s.auth.IsNsfwUnlocked(r))
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
	if !s.auth.IsNsfwUnlocked(r) && isNsfwTagName(body.Name) {
		writeError(w, http.StatusBadRequest, "nsfw tag is locked; unlock before creating it")
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

func (s *Server) handleRenameTag(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tag id")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tag, err := s.store.RenameTag(r.Context(), id, body.Name, !s.auth.IsNsfwUnlocked(r))
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "tag not found")
		case errors.Is(err, db.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, db.ErrDuplicate):
			writeError(w, http.StatusConflict, "tag already exists")
		default:
			writeError(w, http.StatusInternalServerError, "failed to rename tag")
		}
		return
	}
	writeJSON(w, http.StatusOK, tag)
}

func (s *Server) handleMergeTags(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SourceID int64 `json:"source_id"`
		TargetID int64 `json:"target_id"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	target, moved, err := s.store.MergeTags(r.Context(), body.SourceID, body.TargetID, !s.auth.IsNsfwUnlocked(r))
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "tag not found")
		case errors.Is(err, db.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to merge tags")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"merged_into":     target,
		"moved_bookmarks": moved,
		"deleted_tag_id":  body.SourceID,
	})
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tag id")
		return
	}
	if err := s.store.DeleteTagFiltered(r.Context(), id, !s.auth.IsNsfwUnlocked(r)); err != nil {
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
	lists, err := s.store.ListListsFiltered(r.Context(), !s.auth.IsNsfwUnlocked(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list collections")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": lists})
}

func (s *Server) handleGetList(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid list id")
		return
	}
	list, err := s.store.GetList(r.Context(), id, !s.auth.IsNsfwUnlocked(r))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "list not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load list")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		IsSmart     bool              `json:"is_smart"`
		Rule        *db.SmartListRule `json:"rule"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	excludeNsfw := !s.auth.IsNsfwUnlocked(r)
	if excludeNsfw && body.IsSmart && body.Rule != nil && hasNsfwTagName(body.Rule.Tags) {
		writeError(w, http.StatusBadRequest, "nsfw tag is locked; unlock before referencing it")
		return
	}

	list, err := s.store.CreateListWithOptions(r.Context(), db.CreateListInput{
		Name:        body.Name,
		Description: body.Description,
		IsSmart:     body.IsSmart,
		Rule:        body.Rule,
	})
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
	if hydrated, herr := s.store.GetList(r.Context(), list.ID, excludeNsfw); herr == nil {
		list = hydrated
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
		Name        *string           `json:"name"`
		Description *string           `json:"description"`
		IsSmart     *bool             `json:"is_smart"`
		Rule        *db.SmartListRule `json:"rule"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	excludeNsfw := !s.auth.IsNsfwUnlocked(r)
	if excludeNsfw && body.Rule != nil && hasNsfwTagName(body.Rule.Tags) {
		writeError(w, http.StatusBadRequest, "nsfw tag is locked; unlock before referencing it")
		return
	}

	list, err := s.store.UpdateListWithOptions(r.Context(), id, db.UpdateListInput{
		Name:        body.Name,
		Description: body.Description,
		IsSmart:     body.IsSmart,
		Rule:        body.Rule,
	})
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
	if hydrated, herr := s.store.GetList(r.Context(), list.ID, excludeNsfw); herr == nil {
		list = hydrated
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

func (s *Server) handleAddListBookmarks(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid list id")
		return
	}
	var body struct {
		BookmarkIDs []int64 `json:"bookmark_ids"`
		BookmarkID  int64   `json:"bookmark_id"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids := make([]int64, 0, len(body.BookmarkIDs)+1)
	if body.BookmarkID > 0 {
		ids = append(ids, body.BookmarkID)
	}
	ids = append(ids, body.BookmarkIDs...)

	excludeNsfw := !s.auth.IsNsfwUnlocked(r)
	added, err := s.store.AddBookmarksToList(r.Context(), id, ids, excludeNsfw)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "list not found")
		case errors.Is(err, db.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to add bookmarks to list")
		}
		return
	}
	list, err := s.store.GetList(r.Context(), id, excludeNsfw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load updated list")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"list":  list,
		"added": added,
	})
}

func (s *Server) handleRemoveListBookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid list id")
		return
	}
	bookmarkID, ok := parseIDParam(r, "bookmarkId")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid bookmark id")
		return
	}
	if err := s.store.RemoveBookmarkFromList(r.Context(), id, bookmarkID); err != nil {
		switch {
		case errors.Is(err, db.ErrNotFound):
			writeError(w, http.StatusNotFound, "bookmark membership or list not found")
		case errors.Is(err, db.ErrInvalidInput):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to remove bookmark from list")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"removed": true})
}
