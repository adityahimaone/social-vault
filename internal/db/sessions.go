package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Store) CreateSession(ctx context.Context, tokenHash string, ttl time.Duration) error {
	now := time.Now().UTC()
	expires := now.Add(ttl)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions(token_hash, created_at, expires_at)
		VALUES (?, ?, ?)
	`, tokenHash, now.Format(time.RFC3339Nano), expires.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *Store) ValidateSession(ctx context.Context, tokenHash string) (bool, error) {
	var expiresRaw string
	err := s.db.QueryRowContext(ctx, `SELECT expires_at FROM sessions WHERE token_hash = ?`, tokenHash).Scan(&expiresRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("validate session: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresRaw)
	if err != nil {
		_ = s.DeleteSession(ctx, tokenHash)
		return false, nil
	}
	if time.Now().UTC().After(expiresAt) {
		_ = s.DeleteSession(ctx, tokenHash)
		return false, nil
	}
	return true, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Store) PurgeExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, nowUTC())
	return err
}
