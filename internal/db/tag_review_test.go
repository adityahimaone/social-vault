package db_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"social-vault/internal/db"
)

func TestTagReviewQueueLifecycleAndBackfillQuery(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	bm1, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.com/1",
		Title: "Bookmark One",
		Tags:  []string{"existing-tag"},
	})
	if err != nil {
		t.Fatalf("create bm1: %v", err)
	}

	bm2, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.com/2",
		Title: "Bookmark Two",
	})
	if err != nil {
		t.Fatalf("create bm2: %v", err)
	}

	bm3, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.com/3",
		Title: "Bookmark Three",
	})
	if err != nil {
		t.Fatalf("create bm3: %v", err)
	}

	// Enqueue bm1 twice: second call should be idempotent and return existing pending row.
	item1, created, err := store.EnqueueTagReview(ctx, bm1.ID, []string{"finance", "news"}, map[string]float64{"finance": 0.72, "news": 0.58}, "jev")
	if err != nil || !created {
		t.Fatalf("enqueue bm1: created=%v err=%v", created, err)
	}
	item1Dup, created2, err := store.EnqueueTagReview(ctx, bm1.ID, []string{"reading"}, map[string]float64{"reading": 0.6}, "jev")
	if err != nil || created2 || item1Dup.ID != item1.ID {
		t.Fatalf("expected idempotent pending enqueue, got item=%+v created=%v err=%v", item1Dup, created2, err)
	}

	// Enqueue bm2.
	item2, created3, err := store.EnqueueTagReview(ctx, bm2.ID, []string{"security"}, map[string]float64{"security": 0.65}, "jev")
	if err != nil || !created3 {
		t.Fatalf("enqueue bm2: created=%v err=%v", created3, err)
	}

	// List pending returns both with bookmark title & URL.
	pending, err := store.ListPendingTagReviews(ctx, 50)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending items, got %d", len(pending))
	}
	if pending[0].BookmarkURL == "" || pending[0].BookmarkTitle == "" {
		t.Fatalf("expected bookmark title and url on pending item, got %+v", pending[0])
	}

	// Untaggable backfill query should exclude bm1 (has tags + pending), bm2 (has pending),
	// and return only bm3.
	backfillList, err := store.ListUntaggedBookmarksForBackfill(ctx, 50)
	if err != nil {
		t.Fatalf("list backfill: %v", err)
	}
	if len(backfillList) != 1 || backfillList[0].ID != bm3.ID {
		t.Fatalf("expected only bm3 for backfill, got %+v", backfillList)
	}

	// Apply item1: merges tags without removing "existing-tag", marks applied.
	appliedItem, updatedBM1, err := store.ApplyTagReview(ctx, item1.ID)
	if err != nil {
		t.Fatalf("apply item1: %v", err)
	}
	if appliedItem.Status != db.TagReviewStatusApplied || appliedItem.ResolvedAt == "" {
		t.Fatalf("expected applied status & resolved_at, got %+v", appliedItem)
	}
	var gotTags []string
	for _, tg := range updatedBM1.Tags {
		gotTags = append(gotTags, tg.Name)
	}
	for _, want := range []string{"existing-tag", "finance", "news"} {
		if !slices.Contains(gotTags, want) {
			t.Fatalf("expected merged tag %q in %v", want, gotTags)
		}
	}

	// Second apply returns ErrAlreadyResolved.
	if _, _, err := store.ApplyTagReview(ctx, item1.ID); !errors.Is(err, db.ErrAlreadyResolved) {
		t.Fatalf("expected ErrAlreadyResolved on re-apply, got %v", err)
	}

	// Dismiss item2: marks dismissed, does not add tags, and allows bm2 to be eligible again if untagged.
	dismissedItem, err := store.DismissTagReview(ctx, item2.ID)
	if err != nil {
		t.Fatalf("dismiss item2: %v", err)
	}
	if dismissedItem.Status != db.TagReviewStatusDismissed || dismissedItem.ResolvedAt == "" {
		t.Fatalf("expected dismissed status, got %+v", dismissedItem)
	}
	reloadedBM2, err := store.GetBookmark(ctx, bm2.ID)
	if err != nil {
		t.Fatalf("reload bm2: %v", err)
	}
	if len(reloadedBM2.Tags) != 0 {
		t.Fatalf("expected bm2 tags to remain empty after dismiss, got %+v", reloadedBM2.Tags)
	}

	if _, err := store.DismissTagReview(ctx, item2.ID); !errors.Is(err, db.ErrAlreadyResolved) {
		t.Fatalf("expected ErrAlreadyResolved on re-dismiss, got %v", err)
	}

	pendingAfter, err := store.ListPendingTagReviews(ctx, 50)
	if err != nil || len(pendingAfter) != 0 {
		t.Fatalf("expected 0 pending items after resolve, got %d (err=%v)", len(pendingAfter), err)
	}
}
