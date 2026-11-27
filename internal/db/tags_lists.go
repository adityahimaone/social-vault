package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type CreateListInput struct {
	Name        string
	Description string
	IsSmart     bool
	Rule        *SmartListRule
}

type UpdateListInput struct {
	Name        *string
	Description *string
	IsSmart     *bool
	Rule        *SmartListRule
}

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

func (s *Store) RenameTag(ctx context.Context, id int64, rawNewName string, excludeNsfw bool) (*Tag, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: invalid tag id", ErrInvalidInput)
	}
	newName := strings.TrimSpace(rawNewName)
	if newName == "" {
		return nil, fmt.Errorf("%w: tag name is required", ErrInvalidInput)
	}
	if excludeNsfw && strings.EqualFold(newName, "nsfw") {
		return nil, fmt.Errorf("%w: nsfw tag is locked; unlock before assigning it", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var oldName string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM tags WHERE id = ?`, id).Scan(&oldName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load tag: %w", err)
	}
	if excludeNsfw && strings.EqualFold(oldName, "nsfw") {
		return nil, ErrNotFound
	}

	var dupID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ? COLLATE NOCASE AND id != ?`, newName, id).Scan(&dupID)
	if err == nil {
		return nil, ErrDuplicate
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check duplicate tag: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE tags SET name = ? WHERE id = ?`, newName, id); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("rename tag: %w", err)
	}

	if err := rewriteSmartListTagTx(ctx, tx, oldName, newName); err != nil {
		return nil, err
	}

	var updated Tag
	if err := tx.QueryRowContext(ctx, `
		SELECT t.id, t.name, COUNT(bt.bookmark_id), t.created_at
		FROM tags t
		LEFT JOIN bookmark_tags bt ON bt.tag_id = t.id
		WHERE t.id = ?
		GROUP BY t.id
	`, id).Scan(&updated.ID, &updated.Name, &updated.Count, &updated.CreatedAt); err != nil {
		return nil, fmt.Errorf("reload renamed tag: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit rename tag: %w", err)
	}
	return &updated, nil
}

func (s *Store) MergeTags(ctx context.Context, sourceID, targetID int64, excludeNsfw bool) (*Tag, int, error) {
	if sourceID <= 0 || targetID <= 0 {
		return nil, 0, fmt.Errorf("%w: source_id and target_id must be positive", ErrInvalidInput)
	}
	if sourceID == targetID {
		return nil, 0, fmt.Errorf("%w: cannot merge a tag into itself", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sourceName, targetName string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM tags WHERE id = ?`, sourceID).Scan(&sourceName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("load source tag: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT name FROM tags WHERE id = ?`, targetID).Scan(&targetName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("load target tag: %w", err)
	}

	if excludeNsfw && (strings.EqualFold(sourceName, "nsfw") || strings.EqualFold(targetName, "nsfw")) {
		return nil, 0, ErrNotFound
	}

	var moved int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM bookmark_tags WHERE tag_id = ?`, sourceID).Scan(&moved); err != nil {
		return nil, 0, fmt.Errorf("count source bookmark tags: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO bookmark_tags(bookmark_id, tag_id)
		SELECT bookmark_id, ? FROM bookmark_tags WHERE tag_id = ?
	`, targetID, sourceID); err != nil {
		return nil, 0, fmt.Errorf("copy bookmark tags: %w", err)
	}

	now := nowUTC()
	if _, err := tx.ExecContext(ctx, `
		UPDATE bookmarks
		SET updated_at = ?
		WHERE id IN (SELECT bookmark_id FROM bookmark_tags WHERE tag_id = ?)
	`, now, sourceID); err != nil {
		return nil, 0, fmt.Errorf("touch merged bookmarks: %w", err)
	}

	if err := rewriteSmartListTagTx(ctx, tx, sourceName, targetName); err != nil {
		return nil, 0, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, sourceID); err != nil {
		return nil, 0, fmt.Errorf("delete source tag: %w", err)
	}

	var target Tag
	if err := tx.QueryRowContext(ctx, `
		SELECT t.id, t.name, COUNT(bt.bookmark_id), t.created_at
		FROM tags t
		LEFT JOIN bookmark_tags bt ON bt.tag_id = t.id
		WHERE t.id = ?
		GROUP BY t.id
	`, targetID).Scan(&target.ID, &target.Name, &target.Count, &target.CreatedAt); err != nil {
		return nil, 0, fmt.Errorf("reload target tag: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, 0, fmt.Errorf("commit merge tags: %w", err)
	}
	return &target, moved, nil
}

func rewriteSmartListTagTx(ctx context.Context, tx *sql.Tx, oldName, newName string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, filter_rule FROM lists WHERE is_smart = 1 AND filter_rule != ''`)
	if err != nil {
		return fmt.Errorf("load smart lists for tag update: %w", err)
	}
	type pendingUpdate struct {
		id       int64
		ruleJSON string
	}
	var updates []pendingUpdate
	for rows.Next() {
		var id int64
		var rawRule string
		if err := rows.Scan(&id, &rawRule); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan smart list rule: %w", err)
		}
		rule := decodeSmartListRule(rawRule)
		if rule == nil || len(rule.Tags) == 0 {
			continue
		}
		changed := false
		replaced := make([]string, 0, len(rule.Tags))
		for _, t := range rule.Tags {
			if strings.EqualFold(t, oldName) {
				replaced = append(replaced, newName)
				changed = true
			} else {
				replaced = append(replaced, t)
			}
		}
		if !changed {
			continue
		}
		rule.Tags = dedupeTrimmedTags(replaced)
		encoded, err := json.Marshal(rule)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("encode updated smart list rule: %w", err)
		}
		updates = append(updates, pendingUpdate{id: id, ruleJSON: string(encoded)})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iter smart list rules: %w", err)
	}
	_ = rows.Close()

	for _, upd := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE lists SET filter_rule = ? WHERE id = ?`, upd.ruleJSON, upd.id); err != nil {
			return fmt.Errorf("update smart list rule %d: %w", upd.id, err)
		}
	}
	return nil
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
	return s.ListListsFiltered(ctx, false)
}

func (s *Store) ListListsFiltered(ctx context.Context, excludeNsfw bool) ([]List, error) {
	countFilter := ""
	if excludeNsfw {
		countFilter = `AND NOT EXISTS (
			SELECT 1 FROM bookmark_tags bt_nsfw
			JOIN tags t_nsfw ON t_nsfw.id = bt_nsfw.tag_id
			WHERE bt_nsfw.bookmark_id = bl.bookmark_id AND lower(t_nsfw.name) = 'nsfw'
		)`
	}

	query := fmt.Sprintf(`
		SELECT l.id, l.name, l.description, l.is_smart, l.filter_rule,
			COUNT(CASE WHEN bl.bookmark_id IS NOT NULL %s THEN 1 END) AS cnt,
			l.created_at
		FROM lists l
		LEFT JOIN bookmark_lists bl ON bl.list_id = l.id
		GROUP BY l.id
		ORDER BY l.name COLLATE NOCASE ASC
	`, countFilter)

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list lists: %w", err)
	}
	defer rows.Close()

	lists := make([]List, 0)
	for rows.Next() {
		l, err := scanListRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan list: %w", err)
		}
		lists = append(lists, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter lists: %w", err)
	}

	for i := range lists {
		if lists[i].IsSmart && lists[i].Rule != nil {
			cnt, err := s.countSmartListBookmarks(ctx, lists[i].Rule, excludeNsfw)
			if err != nil {
				return nil, err
			}
			lists[i].Count = cnt
		}
	}
	return lists, nil
}

func (s *Store) GetList(ctx context.Context, id int64, excludeNsfw bool) (*List, error) {
	if id <= 0 {
		return nil, ErrNotFound
	}
	countFilter := ""
	if excludeNsfw {
		countFilter = `AND NOT EXISTS (
			SELECT 1 FROM bookmark_tags bt_nsfw
			JOIN tags t_nsfw ON t_nsfw.id = bt_nsfw.tag_id
			WHERE bt_nsfw.bookmark_id = bl.bookmark_id AND lower(t_nsfw.name) = 'nsfw'
		)`
	}
	query := fmt.Sprintf(`
		SELECT l.id, l.name, l.description, l.is_smart, l.filter_rule,
			COUNT(CASE WHEN bl.bookmark_id IS NOT NULL %s THEN 1 END) AS cnt,
			l.created_at
		FROM lists l
		LEFT JOIN bookmark_lists bl ON bl.list_id = l.id
		WHERE l.id = ?
		GROUP BY l.id
	`, countFilter)

	l, err := scanListRow(s.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get list: %w", err)
	}
	if l.IsSmart && l.Rule != nil {
		cnt, err := s.countSmartListBookmarks(ctx, l.Rule, excludeNsfw)
		if err != nil {
			return nil, err
		}
		l.Count = cnt
	}
	return &l, nil
}

func (s *Store) GetListByRef(ctx context.Context, ref string) (*List, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return nil, ErrNotFound
	}
	var row *sql.Row
	if id, err := strconv.ParseInt(trimmed, 10, 64); err == nil && id > 0 {
		row = s.db.QueryRowContext(ctx, `
			SELECT id, name, description, is_smart, filter_rule, 0, created_at
			FROM lists
			WHERE id = ? OR name = ? COLLATE NOCASE
			LIMIT 1
		`, id, trimmed)
	} else {
		row = s.db.QueryRowContext(ctx, `
			SELECT id, name, description, is_smart, filter_rule, 0, created_at
			FROM lists
			WHERE name = ? COLLATE NOCASE
			LIMIT 1
		`, trimmed)
	}
	l, err := scanListRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get list by ref: %w", err)
	}
	return &l, nil
}

func (s *Store) countSmartListBookmarks(ctx context.Context, rule *SmartListRule, excludeNsfw bool) (int, error) {
	var joins []string
	var where []string
	var args []any

	if ftsQuery := applySmartListRule(rule, &where, &args); ftsQuery != "" {
		joins = append(joins, `JOIN bookmarks_fts ON bookmarks_fts.rowid = b.id`)
		where = append([]string{`bookmarks_fts MATCH ?`}, where...)
		args = append([]any{ftsQuery}, args...)
	}
	if excludeNsfw {
		where = append(where, `NOT EXISTS (
			SELECT 1 FROM bookmark_tags bt_nsfw
			JOIN tags t_nsfw ON t_nsfw.id = bt_nsfw.tag_id
			WHERE bt_nsfw.bookmark_id = b.id AND lower(t_nsfw.name) = 'nsfw'
		)`)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}
	countSQL := fmt.Sprintf(`SELECT COUNT(DISTINCT b.id) FROM bookmarks b %s %s`, strings.Join(joins, " "), whereClause)
	var count int
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count smart list bookmarks: %w", err)
	}
	return count, nil
}

func (s *Store) CreateList(ctx context.Context, rawName, description string) (*List, error) {
	return s.CreateListWithOptions(ctx, CreateListInput{
		Name:        rawName,
		Description: description,
	})
}

func (s *Store) CreateListWithOptions(ctx context.Context, in CreateListInput) (*List, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: list name is required", ErrInvalidInput)
	}

	var (
		normRule *SmartListRule
		ruleJSON string
		err      error
	)
	if in.IsSmart {
		normRule, err = NormalizeSmartListRule(in.Rule)
		if err != nil {
			return nil, err
		}
		raw, merr := json.Marshal(normRule)
		if merr != nil {
			return nil, fmt.Errorf("marshal smart list rule: %w", merr)
		}
		ruleJSON = string(raw)
	}

	now := nowUTC()
	desc := strings.TrimSpace(in.Description)
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO lists(name, description, is_smart, filter_rule, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, name, desc, boolToInt(in.IsSmart), ruleJSON, now)
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
		Description: desc,
		IsSmart:     in.IsSmart,
		Rule:        normRule,
		Count:       0,
		CreatedAt:   now,
	}, nil
}

func (s *Store) UpdateList(ctx context.Context, id int64, name, description *string) (*List, error) {
	return s.UpdateListWithOptions(ctx, id, UpdateListInput{
		Name:        name,
		Description: description,
	})
}

func (s *Store) UpdateListWithOptions(ctx context.Context, id int64, in UpdateListInput) (*List, error) {
	current, err := s.GetList(ctx, id, false)
	if err != nil {
		return nil, err
	}

	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: list name cannot be empty", ErrInvalidInput)
		}
		current.Name = trimmed
	}
	if in.Description != nil {
		current.Description = strings.TrimSpace(*in.Description)
	}
	if in.IsSmart != nil {
		current.IsSmart = *in.IsSmart
	}
	if current.IsSmart {
		ruleToValidate := current.Rule
		if in.Rule != nil {
			ruleToValidate = in.Rule
		}
		norm, rerr := NormalizeSmartListRule(ruleToValidate)
		if rerr != nil {
			return nil, rerr
		}
		current.Rule = norm
	} else {
		current.Rule = nil
	}

	ruleJSON := ""
	if current.IsSmart && current.Rule != nil {
		raw, merr := json.Marshal(current.Rule)
		if merr != nil {
			return nil, fmt.Errorf("marshal smart list rule: %w", merr)
		}
		ruleJSON = string(raw)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE lists
		SET name = ?, description = ?, is_smart = ?, filter_rule = ?
		WHERE id = ?
	`, current.Name, current.Description, boolToInt(current.IsSmart), ruleJSON, id); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("update list: %w", err)
	}

	if current.IsSmart {
		if _, err := tx.ExecContext(ctx, `DELETE FROM bookmark_lists WHERE list_id = ?`, id); err != nil {
			return nil, fmt.Errorf("clear manual memberships for smart list: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update list: %w", err)
	}

	return s.GetList(ctx, id, false)
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

func (s *Store) AddBookmarksToList(ctx context.Context, listID int64, bookmarkIDs []int64, excludeNsfw bool) (int, error) {
	list, err := s.GetList(ctx, listID, excludeNsfw)
	if err != nil {
		return 0, err
	}
	if list.IsSmart {
		return 0, fmt.Errorf("%w: cannot manually add bookmarks to a smart list", ErrInvalidInput)
	}

	seen := make(map[int64]struct{}, len(bookmarkIDs))
	cleanIDs := make([]int64, 0, len(bookmarkIDs))
	for _, bid := range bookmarkIDs {
		if bid <= 0 {
			continue
		}
		if _, ok := seen[bid]; ok {
			continue
		}
		seen[bid] = struct{}{}
		cleanIDs = append(cleanIDs, bid)
	}
	if len(cleanIDs) == 0 {
		return 0, fmt.Errorf("%w: at least one valid bookmark_id is required", ErrInvalidInput)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	added := 0
	for _, bid := range cleanIDs {
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM bookmarks WHERE id = ?`, bid).Scan(&exists)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return 0, fmt.Errorf("check bookmark %d: %w", bid, err)
		}
		if excludeNsfw {
			var hasNsfw int
			err := tx.QueryRowContext(ctx, `
				SELECT 1 FROM bookmark_tags bt
				JOIN tags t ON t.id = bt.tag_id
				WHERE bt.bookmark_id = ? AND lower(t.name) = 'nsfw'
				LIMIT 1
			`, bid).Scan(&hasNsfw)
			if err == nil {
				continue
			} else if !errors.Is(err, sql.ErrNoRows) {
				return 0, fmt.Errorf("check nsfw bookmark %d: %w", bid, err)
			}
		}

		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO bookmark_lists(bookmark_id, list_id) VALUES (?, ?)`, bid, listID)
		if err != nil {
			return 0, fmt.Errorf("insert bookmark_list (%d, %d): %w", bid, listID, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added += int(n)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit add bookmarks to list: %w", err)
	}
	return added, nil
}

func (s *Store) RemoveBookmarkFromList(ctx context.Context, listID, bookmarkID int64) error {
	if listID <= 0 || bookmarkID <= 0 {
		return fmt.Errorf("%w: list_id and bookmark_id must be positive", ErrInvalidInput)
	}
	list, err := s.GetList(ctx, listID, false)
	if err != nil {
		return err
	}
	if list.IsSmart {
		return fmt.Errorf("%w: cannot manually remove bookmarks from a smart list", ErrInvalidInput)
	}

	res, err := s.db.ExecContext(ctx, `DELETE FROM bookmark_lists WHERE list_id = ? AND bookmark_id = ?`, listID, bookmarkID)
	if err != nil {
		return fmt.Errorf("remove bookmark from list: %w", err)
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

func scanListRow(r rowScanner) (List, error) {
	var l List
	var isSmartInt int
	var rawRule string
	err := r.Scan(&l.ID, &l.Name, &l.Description, &isSmartInt, &rawRule, &l.Count, &l.CreatedAt)
	if err != nil {
		return List{}, err
	}
	l.IsSmart = isSmartInt != 0
	if l.IsSmart {
		l.Rule = decodeSmartListRule(rawRule)
	}
	return l, nil
}
