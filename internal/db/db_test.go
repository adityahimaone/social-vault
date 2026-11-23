package db_test

import (
	"context"
	"errors"
	"testing"

	"social-vault/internal/db"
)

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestBookmarkCRUDAndSearch(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	created, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:         "https://example.com/posts/go-sqlite",
		Title:       "Go + SQLite FTS5 Notes",
		Description: "Lightweight bookmark vault architecture",
		Notes:       "foo-bar custom note",
		FaviconURL:  "https://example.com/favicon.ico",
		SiteName:    "example.com",
		Source:      "manual",
		Tags:        []string{"golang", "sqlite"},
		Lists:       []string{"Reading"},
	})
	if err != nil {
		t.Fatalf("create bookmark: %v", err)
	}
	if created.ID == 0 || len(created.Tags) != 2 || len(created.Lists) != 1 {
		t.Fatalf("unexpected created bookmark: %+v", created)
	}

	// Duplicate URL should fail with ErrDuplicateURL.
	if _, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL: "https://example.com/posts/go-sqlite",
	}); !errors.Is(err, db.ErrDuplicateURL) {
		t.Fatalf("expected ErrDuplicateURL, got %v", err)
	}

	// FTS search by title, notes with punctuation, and URL.
	for _, query := range []string{"SQLite", "foo-bar", "https://example.com", "vault"} {
		items, total, err := store.ListBookmarks(ctx, db.BookmarkFilter{Query: query})
		if err != nil {
			t.Fatalf("search %q: %v", query, err)
		}
		if total != 1 || len(items) != 1 || items[0].ID != created.ID {
			t.Fatalf("search %q expected 1 match, got total=%d len=%d", query, total, len(items))
		}
	}

	// Update title, notes, archive, and tags.
	newTitle := "Updated FTS5 Title"
	archived := true
	newTags := []string{"golang", "archive-pick"}
	updated, err := store.UpdateBookmark(ctx, created.ID, db.UpdateBookmarkInput{
		Title:    &newTitle,
		Archived: &archived,
		Tags:     &newTags,
	})
	if err != nil {
		t.Fatalf("update bookmark: %v", err)
	}
	if updated.Title != newTitle || !updated.Archived || len(updated.Tags) != 2 {
		t.Fatalf("unexpected updated bookmark: %+v", updated)
	}

	// Verify FTS index updated to match new title and no longer matches old-only if replaced.
	items, _, err := store.ListBookmarks(ctx, db.BookmarkFilter{Query: "Updated", Archived: "true", Tag: "archive-pick", List: "Reading"})
	if err != nil {
		t.Fatalf("filtered search: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 filtered match, got %d", len(items))
	}

	// Stats check.
	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.Total != 1 || stats.Archived != 1 || stats.Active != 0 || stats.ListsCount != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	// Delete bookmark.
	if err := store.DeleteBookmark(ctx, created.ID); err != nil {
		t.Fatalf("delete bookmark: %v", err)
	}
	if _, err := store.GetBookmark(ctx, created.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// Search after delete should return 0.
	items, total, err := store.ListBookmarks(ctx, db.BookmarkFilter{Query: "Updated"})
	if err != nil {
		t.Fatalf("search after delete: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("expected 0 results after delete, got %d", total)
	}
}

func TestBookmarkNullableImageURL(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	noImg, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.com/no-image",
		Title: "Without Image",
	})
	if err != nil {
		t.Fatalf("create without image: %v", err)
	}
	if noImg.ImageURL != "" {
		t.Fatalf("expected empty image_url, got %q", noImg.ImageURL)
	}

	withImg, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:      "https://example.com/with-image",
		Title:    "With Image",
		ImageURL: "https://example.com/cover.png",
	})
	if err != nil {
		t.Fatalf("create with image: %v", err)
	}
	if withImg.ImageURL != "https://example.com/cover.png" {
		t.Fatalf("expected image_url, got %q", withImg.ImageURL)
	}

	listed, total, err := store.ListBookmarks(ctx, db.BookmarkFilter{})
	if err != nil || total != 2 || len(listed) != 2 {
		t.Fatalf("list bookmarks: err=%v total=%d len=%d", err, total, len(listed))
	}
}
