package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	TagReviewStatusPending   = "pending"
	TagReviewStatusApplied   = "applied"
	TagReviewStatusDismissed = "dismissed"
)

// TagReviewItem represents one queued Layer 3 tag suggestion row.
type TagReviewItem struct {
	ID            int64              `json:"id"`
	BookmarkID    int64              `json:"bookmark_id"`
	BookmarkTitle string             `json:"bookmark_title"`
	BookmarkURL   string             `json:"bookmark_url"`
	SuggestedTags []string           `json:"suggested_tags"`
	JevScores     map[string]float64 `json:"jev_scores"`
	Source        string             `json:"source"`
	Status        string             `json:"status"`
	CreatedAt     string             `json:"created_at"`
	ResolvedAt    string             `json:"resolved_at,omitempty"`
}

// EnqueueTagReview creates a pending review queue item for bookmarkID.
// If a pending item already exists for bookmarkID, it returns that existing item with created=false.
func (s *Store) EnqueueTagReview(ctx context.Context, bookmarkID int64, suggestedTags []string, jevScores map[string]float64, source string) (*TagReviewItem, bool, error) {
	if bookmarkID <= 0 {
		return nil, false, fmt.Errorf("%w: bookmark_id must be positive", ErrInvalidInput)
	}
	cleanTags := dedupeTrimmedTags(suggestedTags)
	if len(cleanTags) == 0 {
		return nil, false, fmt.Errorf("%w: suggested_tags cannot be empty", ErrInvalidInput)
	}
	if jevScores == nil {
		jevScores = map[string]float64{}
	}
	src := strings.TrimSpace(source)
	if src == "" {
		src = "jev"
	}

	tagsJSON, err := json.Marshal(cleanTags)
	if err != nil {
		return nil, false, fmt.Errorf("marshal suggested_tags: %w", err)
	}
	scoresJSON, err := json.Marshal(jevScores)
	if err != nil {
		return nil, false, fmt.Errorf("marshal jev_scores: %w", err)
	}

	if existing, err := s.getPendingTagReviewByBookmark(ctx, bookmarkID); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}

	now := nowUTC()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO tag_review_queue (bookmark_id, suggested_tags, jev_scores, source, status, created_at)
		VALUES (?, ?, ?, ?, 'pending', ?)
	`, bookmarkID, string(tagsJSON), string(scoresJSON), src, now)
	if err != nil {
		if isUniqueViolation(err) {
			existing, gerr := s.getPendingTagReviewByBookmark(ctx, bookmarkID)
			if gerr != nil {
				return nil, false, gerr
			}
			return existing, false, nil
		}
		return nil, false, fmt.Errorf("insert tag review: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, false, fmt.Errorf("last insert id: %w", err)
	}
	item, err := s.GetTagReview(ctx, id)
	if err != nil {
		return nil, false, err
	}
	return item, true, nil
}

func (s *Store) getPendingTagReviewByBookmark(ctx context.Context, bookmarkID int64) (*TagReviewItem, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT q.id, q.bookmark_id, b.title, b.url, q.suggested_tags, q.jev_scores, q.source, q.status, q.created_at, q.resolved_at
		FROM tag_review_queue q
		JOIN bookmarks b ON b.id = q.bookmark_id
		WHERE q.bookmark_id = ? AND q.status = 'pending'
	`, bookmarkID)
	item, err := scanTagReviewItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get pending tag review: %w", err)
	}
	return &item, nil
}

// GetTagReview fetches one review queue item joined with its bookmark title and URL.
func (s *Store) GetTagReview(ctx context.Context, id int64) (*TagReviewItem, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT q.id, q.bookmark_id, b.title, b.url, q.suggested_tags, q.jev_scores, q.source, q.status, q.created_at, q.resolved_at
		FROM tag_review_queue q
		JOIN bookmarks b ON b.id = q.bookmark_id
		WHERE q.id = ?
	`, id)
	item, err := scanTagReviewItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get tag review: %w", err)
	}
	return &item, nil
}

// ListPendingTagReviews lists pending review queue rows newest-first with bookmark title and URL.
func (s *Store) ListPendingTagReviews(ctx context.Context, limit int) ([]TagReviewItem, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT q.id, q.bookmark_id, b.title, b.url, q.suggested_tags, q.jev_scores, q.source, q.status, q.created_at, q.resolved_at
		FROM tag_review_queue q
		JOIN bookmarks b ON b.id = q.bookmark_id
		WHERE q.status = 'pending'
		ORDER BY q.created_at DESC, q.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending tag reviews: %w", err)
	}
	defer rows.Close()

	items := make([]TagReviewItem, 0)
	for rows.Next() {
		item, err := scanTagReviewItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tag review: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter tag reviews: %w", err)
	}
	return items, nil
}

// ApplyTagReview merges the pending suggested tags into the bookmark without removing
// any existing tags, and marks the queue item applied.
func (s *Store) ApplyTagReview(ctx context.Context, id int64) (*TagReviewItem, *Bookmark, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var bookmarkID int64
	var rawSuggested, status string
	err = tx.QueryRowContext(ctx, `
		SELECT bookmark_id, suggested_tags, status
		FROM tag_review_queue
		WHERE id = ?
	`, id).Scan(&bookmarkID, &rawSuggested, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("load tag review: %w", err)
	}
	if status != TagReviewStatusPending {
		return nil, nil, ErrAlreadyResolved
	}

	var suggested []string
	if err := json.Unmarshal([]byte(rawSuggested), &suggested); err != nil {
		return nil, nil, fmt.Errorf("decode suggested_tags: %w", err)
	}

	existingRows, err := tx.QueryContext(ctx, `
		SELECT t.name
		FROM bookmark_tags bt
		JOIN tags t ON t.id = bt.tag_id
		WHERE bt.bookmark_id = ?
		ORDER BY t.name COLLATE NOCASE ASC
	`, bookmarkID)
	if err != nil {
		return nil, nil, fmt.Errorf("load existing tags: %w", err)
	}
	var existing []string
	for existingRows.Next() {
		var name string
		if err := existingRows.Scan(&name); err != nil {
			_ = existingRows.Close()
			return nil, nil, fmt.Errorf("scan existing tag: %w", err)
		}
		existing = append(existing, name)
	}
	if err := existingRows.Err(); err != nil {
		_ = existingRows.Close()
		return nil, nil, fmt.Errorf("iter existing tags: %w", err)
	}
	_ = existingRows.Close()

	merged := dedupeTrimmedTags(append(existing, suggested...))
	now := nowUTC()
	if err := setBookmarkTagsTx(ctx, tx, bookmarkID, merged, now); err != nil {
		return nil, nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE bookmarks SET updated_at = ? WHERE id = ?`, now, bookmarkID); err != nil {
		return nil, nil, fmt.Errorf("touch bookmark updated_at: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE tag_review_queue
		SET status = 'applied', resolved_at = ?
		WHERE id = ? AND status = 'pending'
	`, now, id); err != nil {
		return nil, nil, fmt.Errorf("mark tag review applied: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit apply tag review: %w", err)
	}

	item, err := s.GetTagReview(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	bookmark, err := s.GetBookmark(ctx, bookmarkID)
	if err != nil {
		return nil, nil, err
	}
	return item, bookmark, nil
}

// DismissTagReview marks a pending queue item dismissed without altering bookmark tags.
func (s *Store) DismissTagReview(ctx context.Context, id int64) (*TagReviewItem, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM tag_review_queue WHERE id = ?`, id).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load tag review status: %w", err)
	}
	if status != TagReviewStatusPending {
		return nil, ErrAlreadyResolved
	}

	now := nowUTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE tag_review_queue
		SET status = 'dismissed', resolved_at = ?
		WHERE id = ? AND status = 'pending'
	`, now, id)
	if err != nil {
		return nil, fmt.Errorf("dismiss tag review: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return nil, ErrAlreadyResolved
	}
	return s.GetTagReview(ctx, id)
}

// CountPendingTagReviews returns the number of pending rows in tag_review_queue.
func (s *Store) CountPendingTagReviews(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tag_review_queue WHERE status = 'pending'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pending tag reviews: %w", err)
	}
	return count, nil
}

// CountUntaggedBookmarks returns the number of bookmarks that have zero tags.
func (s *Store) CountUntaggedBookmarks(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM bookmarks b
		WHERE NOT EXISTS (
			SELECT 1 FROM bookmark_tags bt WHERE bt.bookmark_id = b.id
		)
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count untagged bookmarks: %w", err)
	}
	return count, nil
}

// ListUntaggedBookmarksForBackfill returns bookmarks with zero tags and no review queue row (pending, applied, or dismissed).
func (s *Store) ListUntaggedBookmarksForBackfill(ctx context.Context, limit int) ([]Bookmark, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id, b.url, b.title, b.description, b.notes, b.favicon_url, b.image_url, b.site_name, b.source, b.archived, b.created_at, b.updated_at
		FROM bookmarks b
		WHERE NOT EXISTS (
			SELECT 1 FROM bookmark_tags bt WHERE bt.bookmark_id = b.id
		)
		AND NOT EXISTS (
			SELECT 1 FROM tag_review_queue q WHERE q.bookmark_id = b.id
		)
		ORDER BY b.created_at DESC, b.id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list untagged bookmarks for backfill: %w", err)
	}
	defer rows.Close()

	items := make([]Bookmark, 0)
	for rows.Next() {
		b, err := scanBookmark(rows)
		if err != nil {
			return nil, fmt.Errorf("scan backfill bookmark: %w", err)
		}
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter backfill bookmarks: %w", err)
	}

	if len(items) > 0 {
		ptrs := make([]*Bookmark, len(items))
		for i := range items {
			ptrs[i] = &items[i]
		}
		if err := s.hydrateBookmarks(ctx, ptrs); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func scanTagReviewItem(r rowScanner) (TagReviewItem, error) {
	var item TagReviewItem
	var rawTags, rawScores string
	var resolvedAt sql.NullString
	err := r.Scan(
		&item.ID,
		&item.BookmarkID,
		&item.BookmarkTitle,
		&item.BookmarkURL,
		&rawTags,
		&rawScores,
		&item.Source,
		&item.Status,
		&item.CreatedAt,
		&resolvedAt,
	)
	if err != nil {
		return TagReviewItem{}, err
	}
	item.ResolvedAt = resolvedAt.String
	item.SuggestedTags = make([]string, 0)
	item.JevScores = make(map[string]float64)
	if strings.TrimSpace(rawTags) != "" {
		_ = json.Unmarshal([]byte(rawTags), &item.SuggestedTags)
	}
	if strings.TrimSpace(rawScores) != "" {
		_ = json.Unmarshal([]byte(rawScores), &item.JevScores)
	}
	return item, nil
}

func dedupeTrimmedTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, raw := range tags {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return slices.Clip(out)
}
