package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Tag struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Count     int    `json:"count,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type List struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Count       int    `json:"count,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

type Bookmark struct {
	ID          int64  `json:"id"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Notes       string `json:"notes"`
	FaviconURL  string `json:"favicon_url"`
	SiteName    string `json:"site_name"`
	Source      string `json:"source"`
	Archived    bool   `json:"archived"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Tags        []Tag  `json:"tags"`
	Lists       []List `json:"lists"`
}

type Stats struct {
	Total      int   `json:"total"`
	Active     int   `json:"active"`
	Archived   int   `json:"archived"`
	TagsCount  int   `json:"tags_count"`
	ListsCount int   `json:"lists_count"`
	TopTags    []Tag `json:"top_tags"`
}

type CreateBookmarkInput struct {
	URL         string
	Title       string
	Description string
	Notes       string
	FaviconURL  string
	SiteName    string
	Source      string
	Archived    bool
	Tags        []string
	Lists       []string
	CreatedAt   string
}

type UpdateBookmarkInput struct {
	Title       *string
	Description *string
	Notes       *string
	Archived    *bool
	Tags        *[]string
	Lists       *[]string
	ListIDs     *[]int64
}

type BookmarkFilter struct {
	Query    string
	Archived string // "all", "true", "false"
	Tag      string
	List     string
	Limit    int
	Offset   int
}

func (s *Store) CreateBookmark(ctx context.Context, in CreateBookmarkInput) (*Bookmark, error) {
	urlStr := strings.TrimSpace(in.URL)
	if urlStr == "" {
		return nil, fmt.Errorf("%w: url is required", ErrInvalidInput)
	}

	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "manual"
	}

	now := nowUTC()
	createdAt := strings.TrimSpace(in.CreatedAt)
	if createdAt == "" {
		createdAt = now
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO bookmarks (url, title, description, notes, favicon_url, site_name, source, archived, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		urlStr,
		strings.TrimSpace(in.Title),
		strings.TrimSpace(in.Description),
		strings.TrimSpace(in.Notes),
		strings.TrimSpace(in.FaviconURL),
		strings.TrimSpace(in.SiteName),
		source,
		boolToInt(in.Archived),
		createdAt,
		now,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicateURL
		}
		return nil, fmt.Errorf("insert bookmark: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	if len(in.Tags) > 0 {
		if err := setBookmarkTagsTx(ctx, tx, id, in.Tags, now); err != nil {
			return nil, err
		}
	}
	if len(in.Lists) > 0 {
		if err := setBookmarkListsByNameTx(ctx, tx, id, in.Lists, now); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return s.GetBookmark(ctx, id)
}

func (s *Store) GetBookmark(ctx context.Context, id int64) (*Bookmark, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, url, title, description, notes, favicon_url, site_name, source, archived, created_at, updated_at
		FROM bookmarks
		WHERE id = ?
	`, id)

	b, err := scanBookmark(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get bookmark: %w", err)
	}

	if err := s.hydrateBookmarks(ctx, []*Bookmark{&b}); err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) GetBookmarkByURL(ctx context.Context, rawURL string) (*Bookmark, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, url, title, description, notes, favicon_url, site_name, source, archived, created_at, updated_at
		FROM bookmarks
		WHERE url = ?
	`, strings.TrimSpace(rawURL))

	b, err := scanBookmark(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get bookmark by url: %w", err)
	}

	if err := s.hydrateBookmarks(ctx, []*Bookmark{&b}); err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) ListBookmarks(ctx context.Context, f BookmarkFilter) ([]Bookmark, int, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	var joins []string
	var where []string
	var args []any

	ftsQuery := buildFTSQuery(f.Query)
	if ftsQuery != "" {
		joins = append(joins, `JOIN bookmarks_fts ON bookmarks_fts.rowid = b.id`)
		where = append(where, `bookmarks_fts MATCH ?`)
		args = append(args, ftsQuery)
	}

	switch strings.ToLower(strings.TrimSpace(f.Archived)) {
	case "true", "1":
		where = append(where, `b.archived = 1`)
	case "false", "0":
		where = append(where, `b.archived = 0`)
	}

	if tag := strings.TrimSpace(f.Tag); tag != "" {
		joins = append(joins, `JOIN bookmark_tags bt ON bt.bookmark_id = b.id JOIN tags t ON t.id = bt.tag_id`)
		if id, err := strconv.ParseInt(tag, 10, 64); err == nil && id > 0 {
			where = append(where, `(t.id = ? OR t.name = ? COLLATE NOCASE)`)
			args = append(args, id, tag)
		} else {
			where = append(where, `t.name = ? COLLATE NOCASE`)
			args = append(args, tag)
		}
	}

	if list := strings.TrimSpace(f.List); list != "" {
		joins = append(joins, `JOIN bookmark_lists bl ON bl.bookmark_id = b.id JOIN lists l ON l.id = bl.list_id`)
		if id, err := strconv.ParseInt(list, 10, 64); err == nil && id > 0 {
			where = append(where, `(l.id = ? OR l.name = ? COLLATE NOCASE)`)
			args = append(args, id, list)
		} else {
			where = append(where, `l.name = ? COLLATE NOCASE`)
			args = append(args, list)
		}
	}

	joinClause := strings.Join(joins, " ")
	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	countSQL := fmt.Sprintf(`SELECT COUNT(DISTINCT b.id) FROM bookmarks b %s %s`, joinClause, whereClause)
	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count bookmarks: %w", err)
	}

	orderBy := `ORDER BY b.created_at DESC, b.id DESC`
	if ftsQuery != "" {
		orderBy = `ORDER BY bm25(bookmarks_fts), b.created_at DESC, b.id DESC`
	}

	selectSQL := fmt.Sprintf(`
		SELECT DISTINCT b.id, b.url, b.title, b.description, b.notes, b.favicon_url, b.site_name, b.source, b.archived, b.created_at, b.updated_at
		FROM bookmarks b
		%s
		%s
		%s
		LIMIT ? OFFSET ?
	`, joinClause, whereClause, orderBy)

	selectArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, selectSQL, selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list bookmarks: %w", err)
	}
	defer rows.Close()

	items := make([]Bookmark, 0)
	for rows.Next() {
		b, err := scanBookmark(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan bookmark: %w", err)
		}
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate bookmarks: %w", err)
	}

	if len(items) > 0 {
		ptrs := make([]*Bookmark, len(items))
		for i := range items {
			ptrs[i] = &items[i]
		}
		if err := s.hydrateBookmarks(ctx, ptrs); err != nil {
			return nil, 0, err
		}
	}

	return items, total, nil
}

func (s *Store) UpdateBookmark(ctx context.Context, id int64, in UpdateBookmarkInput) (*Bookmark, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM bookmarks WHERE id = ?`, id).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("check bookmark: %w", err)
	}

	now := nowUTC()
	var sets []string
	var args []any

	if in.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, strings.TrimSpace(*in.Title))
	}
	if in.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, strings.TrimSpace(*in.Description))
	}
	if in.Notes != nil {
		sets = append(sets, "notes = ?")
		args = append(args, strings.TrimSpace(*in.Notes))
	}
	if in.Archived != nil {
		sets = append(sets, "archived = ?")
		args = append(args, boolToInt(*in.Archived))
	}

	sets = append(sets, "updated_at = ?")
	args = append(args, now, id)

	query := fmt.Sprintf(`UPDATE bookmarks SET %s WHERE id = ?`, strings.Join(sets, ", "))
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("update bookmark: %w", err)
	}

	if in.Tags != nil {
		if err := setBookmarkTagsTx(ctx, tx, id, *in.Tags, now); err != nil {
			return nil, err
		}
	}
	if in.Lists != nil {
		if err := setBookmarkListsByNameTx(ctx, tx, id, *in.Lists, now); err != nil {
			return nil, err
		}
	} else if in.ListIDs != nil {
		if err := setBookmarkListsByIDTx(ctx, tx, id, *in.ListIDs); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return s.GetBookmark(ctx, id)
}

func (s *Store) DeleteBookmark(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM bookmarks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete bookmark: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetStats(ctx context.Context) (*Stats, error) {
	st := &Stats{TopTags: make([]Tag, 0)}

	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN archived = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN archived = 1 THEN 1 ELSE 0 END), 0)
		FROM bookmarks
	`).Scan(&st.Total, &st.Active, &st.Archived)
	if err != nil {
		return nil, fmt.Errorf("bookmark counts: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags`).Scan(&st.TagsCount); err != nil {
		return nil, fmt.Errorf("tags count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM lists`).Scan(&st.ListsCount); err != nil {
		return nil, fmt.Errorf("lists count: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.name, COUNT(bt.bookmark_id) AS cnt, t.created_at
		FROM tags t
		JOIN bookmark_tags bt ON bt.tag_id = t.id
		GROUP BY t.id
		ORDER BY cnt DESC, t.name COLLATE NOCASE ASC
		LIMIT 5
	`)
	if err != nil {
		return nil, fmt.Errorf("top tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Count, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan top tag: %w", err)
		}
		st.TopTags = append(st.TopTags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter top tags: %w", err)
	}

	return st, nil
}

func (s *Store) hydrateBookmarks(ctx context.Context, items []*Bookmark) error {
	if len(items) == 0 {
		return nil
	}

	byID := make(map[int64]*Bookmark, len(items))
	placeholders := make([]string, len(items))
	args := make([]any, len(items))

	for i, b := range items {
		b.Tags = make([]Tag, 0)
		b.Lists = make([]List, 0)
		byID[b.ID] = b
		placeholders[i] = "?"
		args[i] = b.ID
	}

	inClause := strings.Join(placeholders, ",")

	tagRows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT bt.bookmark_id, t.id, t.name, t.created_at
		FROM bookmark_tags bt
		JOIN tags t ON t.id = bt.tag_id
		WHERE bt.bookmark_id IN (%s)
		ORDER BY t.name COLLATE NOCASE ASC
	`, inClause), args...)
	if err != nil {
		return fmt.Errorf("hydrate tags: %w", err)
	}
	defer tagRows.Close()

	for tagRows.Next() {
		var bid int64
		var t Tag
		if err := tagRows.Scan(&bid, &t.ID, &t.Name, &t.CreatedAt); err != nil {
			return fmt.Errorf("scan bookmark tag: %w", err)
		}
		if b, ok := byID[bid]; ok {
			b.Tags = append(b.Tags, t)
		}
	}
	if err := tagRows.Err(); err != nil {
		return fmt.Errorf("iter bookmark tags: %w", err)
	}

	listRows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT bl.bookmark_id, l.id, l.name, l.description, l.created_at
		FROM bookmark_lists bl
		JOIN lists l ON l.id = bl.list_id
		WHERE bl.bookmark_id IN (%s)
		ORDER BY l.name COLLATE NOCASE ASC
	`, inClause), args...)
	if err != nil {
		return fmt.Errorf("hydrate lists: %w", err)
	}
	defer listRows.Close()

	for listRows.Next() {
		var bid int64
		var l List
		if err := listRows.Scan(&bid, &l.ID, &l.Name, &l.Description, &l.CreatedAt); err != nil {
			return fmt.Errorf("scan bookmark list: %w", err)
		}
		if b, ok := byID[bid]; ok {
			b.Lists = append(b.Lists, l)
		}
	}
	if err := listRows.Err(); err != nil {
		return fmt.Errorf("iter bookmark lists: %w", err)
	}

	return nil
}

func setBookmarkTagsTx(ctx context.Context, tx *sql.Tx, bookmarkID int64, names []string, now string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM bookmark_tags WHERE bookmark_id = ?`, bookmarkID); err != nil {
		return fmt.Errorf("clear bookmark tags: %w", err)
	}

	seen := make(map[string]struct{}, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name, created_at) VALUES (?, ?)`, name, now); err != nil {
			return fmt.Errorf("upsert tag %q: %w", name, err)
		}
		var tagID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ? COLLATE NOCASE`, name).Scan(&tagID); err != nil {
			return fmt.Errorf("find tag %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO bookmark_tags(bookmark_id, tag_id) VALUES (?, ?)`, bookmarkID, tagID); err != nil {
			return fmt.Errorf("link tag %q: %w", name, err)
		}
	}
	return nil
}

func setBookmarkListsByNameTx(ctx context.Context, tx *sql.Tx, bookmarkID int64, names []string, now string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM bookmark_lists WHERE bookmark_id = ?`, bookmarkID); err != nil {
		return fmt.Errorf("clear bookmark lists: %w", err)
	}

	seen := make(map[string]struct{}, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO lists(name, description, created_at) VALUES (?, '', ?)`, name, now); err != nil {
			return fmt.Errorf("upsert list %q: %w", name, err)
		}
		var listID int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM lists WHERE name = ? COLLATE NOCASE`, name).Scan(&listID); err != nil {
			return fmt.Errorf("find list %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO bookmark_lists(bookmark_id, list_id) VALUES (?, ?)`, bookmarkID, listID); err != nil {
			return fmt.Errorf("link list %q: %w", name, err)
		}
	}
	return nil
}

func setBookmarkListsByIDTx(ctx context.Context, tx *sql.Tx, bookmarkID int64, listIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM bookmark_lists WHERE bookmark_id = ?`, bookmarkID); err != nil {
		return fmt.Errorf("clear bookmark lists: %w", err)
	}
	seen := make(map[int64]struct{}, len(listIDs))
	for _, lid := range listIDs {
		if lid <= 0 {
			continue
		}
		if _, ok := seen[lid]; ok {
			continue
		}
		seen[lid] = struct{}{}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO bookmark_lists(bookmark_id, list_id) VALUES (?, ?)`, bookmarkID, lid); err != nil {
			return fmt.Errorf("link list id %d: %w", lid, err)
		}
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBookmark(r rowScanner) (Bookmark, error) {
	var b Bookmark
	var archivedInt int
	err := r.Scan(
		&b.ID,
		&b.URL,
		&b.Title,
		&b.Description,
		&b.Notes,
		&b.FaviconURL,
		&b.SiteName,
		&b.Source,
		&archivedInt,
		&b.CreatedAt,
		&b.UpdatedAt,
	)
	if err != nil {
		return Bookmark{}, err
	}
	b.Archived = archivedInt != 0
	b.Tags = make([]Tag, 0)
	b.Lists = make([]List, 0)
	return b, nil
}

func buildFTSQuery(raw string) string {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return ""
	}
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		clean := strings.ReplaceAll(f, `"`, "")
		clean = strings.TrimSpace(clean)
		if clean == "" {
			continue
		}
		terms = append(terms, `"`+clean+`"*`)
	}
	return strings.Join(terms, " ")
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
