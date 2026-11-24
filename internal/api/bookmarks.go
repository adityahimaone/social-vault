package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"social-vault/internal/db"
	"social-vault/internal/tagging"
)

func (s *Server) handleListBookmarks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := parseIntDefault(q.Get("limit"), 50)
	offset := parseIntDefault(q.Get("offset"), 0)

	items, total, err := s.store.ListBookmarks(r.Context(), db.BookmarkFilter{
		Query:       q.Get("q"),
		Archived:    q.Get("archived"),
		Tag:         q.Get("tag"),
		List:        q.Get("list"),
		Limit:       limit,
		Offset:      offset,
		ExcludeNsfw: !s.auth.IsNsfwUnlocked(r),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list bookmarks")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL         string   `json:"url"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Notes       string   `json:"notes"`
		FaviconURL  string   `json:"favicon_url"`
		ImageURL    string   `json:"image_url"`
		SiteName    string   `json:"site_name"`
		Source      string   `json:"source"`
		Archived    bool     `json:"archived"`
		Tags        []string `json:"tags"`
		Lists       []string `json:"lists"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	nsfwUnlocked := s.auth.IsNsfwUnlocked(r)
	if !nsfwUnlocked && hasNsfwTagName(body.Tags) {
		writeError(w, http.StatusBadRequest, "nsfw tag is locked; unlock before assigning it")
		return
	}

	cleanURL, err := validateHTTPURL(body.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if existing, err := s.store.GetBookmarkByURL(r.Context(), cleanURL); err == nil {
		if !nsfwUnlocked && bookmarkHasNsfwTag(existing) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "bookmark already exists",
			})
			return
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":    "bookmark already exists",
			"bookmark": existing,
		})
		return
	}

	title := strings.TrimSpace(body.Title)
	desc := strings.TrimSpace(body.Description)
	favicon := strings.TrimSpace(body.FaviconURL)
	imageURL := strings.TrimSpace(body.ImageURL)
	if imageURL != "" {
		if validImg, ierr := validateHTTPURL(imageURL); ierr == nil {
			imageURL = validImg
		} else {
			imageURL = ""
		}
	}
	siteName := strings.TrimSpace(body.SiteName)

	if title == "" || desc == "" || favicon == "" {
		crawlCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		meta, fetchErr := s.fetcher.Fetch(crawlCtx, cleanURL)
		cancel()
		if fetchErr == nil {
			if title == "" {
				title = meta.Title
			}
			if desc == "" {
				desc = meta.Description
			}
			if favicon == "" {
				favicon = meta.FaviconURL
			}
			if imageURL == "" {
				imageURL = meta.ImageURL
			}
			if siteName == "" {
				siteName = meta.SiteName
			}
		} else if siteName == "" {
			if parsed, perr := url.Parse(cleanURL); perr == nil {
				siteName = strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
			}
		}
	}

	if title == "" {
		title = cleanURL
	}

	ruleTags := s.tagger.MatchRules(tagging.Input{
		URL:         cleanURL,
		Title:       title,
		Description: desc,
		SiteName:    siteName,
	})
	mergedTags := tagging.MergeTags(body.Tags, ruleTags)

	created, err := s.store.CreateBookmark(r.Context(), db.CreateBookmarkInput{
		URL:         cleanURL,
		Title:       title,
		Description: desc,
		Notes:       body.Notes,
		FaviconURL:  favicon,
		ImageURL:    imageURL,
		SiteName:    siteName,
		Source:      body.Source,
		Archived:    body.Archived,
		Tags:        mergedTags,
		Lists:       body.Lists,
	})
	if err != nil {
		if errors.Is(err, db.ErrDuplicateURL) {
			existing, _ := s.store.GetBookmarkByURL(r.Context(), cleanURL)
			if !nsfwUnlocked && bookmarkHasNsfwTag(existing) {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": "bookmark already exists",
				})
				return
			}
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":    "bookmark already exists",
				"bookmark": existing,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to save bookmark")
		return
	}

	if len(created.Tags) == 0 && s.tagger.JevEnabled() {
		s.tagger.RunJevAsync(created)
	}

	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetBookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid bookmark id")
		return
	}
	b, err := s.store.GetBookmark(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load bookmark")
		return
	}
	if !s.auth.IsNsfwUnlocked(r) && bookmarkHasNsfwTag(b) {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleUpdateBookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid bookmark id")
		return
	}

	var body struct {
		Title       *string   `json:"title"`
		Description *string   `json:"description"`
		Notes       *string   `json:"notes"`
		Archived    *bool     `json:"archived"`
		Tags        *[]string `json:"tags"`
		Lists       *[]string `json:"lists"`
		ListIDs     *[]int64  `json:"list_ids"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !s.auth.IsNsfwUnlocked(r) {
		if body.Tags != nil && hasNsfwTagName(*body.Tags) {
			writeError(w, http.StatusBadRequest, "nsfw tag is locked; unlock before assigning it")
			return
		}
		existing, err := s.store.GetBookmark(r.Context(), id)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeError(w, http.StatusNotFound, "bookmark not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to update bookmark")
			return
		}
		if bookmarkHasNsfwTag(existing) {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		}
	}

	updated, err := s.store.UpdateBookmark(r.Context(), id, db.UpdateBookmarkInput{
		Title:       body.Title,
		Description: body.Description,
		Notes:       body.Notes,
		Archived:    body.Archived,
		Tags:        body.Tags,
		Lists:       body.Lists,
		ListIDs:     body.ListIDs,
	})
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update bookmark")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid bookmark id")
		return
	}
	if !s.auth.IsNsfwUnlocked(r) {
		existing, err := s.store.GetBookmark(r.Context(), id)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeError(w, http.StatusNotFound, "bookmark not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to delete bookmark")
			return
		}
		if bookmarkHasNsfwTag(existing) {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		}
	}
	if err := s.store.DeleteBookmark(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete bookmark")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func validateHTTPURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("url is required")
	}
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("url must be a valid http or https URL")
	}
	return u.String(), nil
}

func parseIDParam(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func parseIntDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
