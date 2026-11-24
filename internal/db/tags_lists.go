package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (s *Store) ListTags(ctx context.Context) ([]Tag, error) {
	return s.ListTagsFiltered(ctx, false)
}

func (s *Store) ListTagsFiltered(ctx context.Context, excludeNsfw bool) ([]Tag, error) {
	where := ""
	if excludeNsfw {
		where = `WHERE lower(t.name) != 'nsfw'`
	}
	query := fmt.Sprintf(`
		SELECT t.id, t.name, COUNT(bt.bookmark_id) AS cnt, t.created_at
		FROM tags t
		LEFT JOIN bookmark_tags bt ON bt.tag_id = t.id
		%s
		GROUP BY t.id
		ORDER BY cnt DESC, t.name COLLATE NOCASE ASC
	`, where)

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	tags := make([]Tag, 0)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Count, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter tags: %w", err)
	}
	return tags, nil
}

func (s *Store) CreateTag(ctx context.Context, rawName string) (*Tag, error) {
	name := strings.TrimSpace(rawName)
	if name == "" {
		return nil, fmt.Errorf("%w: tag name is required", ErrInvalidInput)
	}

	now := nowUTC()
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name, created_at) VALUES (?, ?)`, name, now); err != nil {
		return nil, fmt.Errorf("insert tag: %w", err)
	}

	var t Tag
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.name, COUNT(bt.bookmark_id), t.created_at
		FROM tags t
		LEFT JOIN bookmark_tags bt ON bt.tag_id = t.id
		WHERE t.name = ? COLLATE NOCASE
		GROUP BY t.id
	`, name).Scan(&t.ID, &t.Name, &t.Count, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("fetch created tag: %w", err)
	}
	return &t, nil
}

func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	return s.DeleteTagFiltered(ctx, id, false)
}

func (s *Store) DeleteTagFiltered(ctx context.Context, id int64, excludeNsfw bool) error {
	query := `DELETE FROM tags WHERE id = ?`
	if excludeNsfw {
		query = `DELETE FROM tags WHERE id = ? AND lower(name) != 'nsfw'`
	}
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
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

func (s *Store) ListLists(ctx context.Context) ([]List, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.name, l.description, COUNT(bl.bookmark_id) AS cnt, l.created_at
		FROM lists l
		LEFT JOIN bookmark_lists bl ON bl.list_id = l.id
		GROUP BY l.id
		ORDER BY l.name COLLATE NOCASE ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list lists: %w", err)
	}
	defer rows.Close()

	lists := make([]List, 0)
	for rows.Next() {
		var l List
		if err := rows.Scan(&l.ID, &l.Name, &l.Description, &l.Count, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan list: %w", err)
		}
		lists = append(lists, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter lists: %w", err)
	}
	return lists, nil
}

func (s *Store) CreateList(ctx context.Context, rawName, description string) (*List, error) {
	name := strings.TrimSpace(rawName)
	if name == "" {
		return nil, fmt.Errorf("%w: list name is required", ErrInvalidInput)
	}

	now := nowUTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO lists(name, description, created_at) VALUES (?, ?, ?)`, name, strings.TrimSpace(description), now)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("insert list: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return &List{
		ID:          id,
		Name:        name,
		Description: strings.TrimSpace(description),
		Count:       0,
		CreatedAt:   now,
	}, nil
}

func (s *Store) UpdateList(ctx context.Context, id int64, name, description *string) (*List, error) {
	var current List
	err := s.db.QueryRowContext(ctx, `
		SELECT l.id, l.name, l.description, COUNT(bl.bookmark_id), l.created_at
		FROM lists l
		LEFT JOIN bookmark_lists bl ON bl.list_id = l.id
		WHERE l.id = ?
		GROUP BY l.id
	`, id).Scan(&current.ID, &current.Name, &current.Description, &current.Count, &current.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("fetch list: %w", err)
	}

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: list name cannot be empty", ErrInvalidInput)
		}
		current.Name = trimmed
	}
	if description != nil {
		current.Description = strings.TrimSpace(*description)
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE lists SET name = ?, description = ? WHERE id = ?`, current.Name, current.Description, id); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("update list: %w", err)
	}
	return &current, nil
}

func (s *Store) DeleteList(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM lists WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete list: %w", err)
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
