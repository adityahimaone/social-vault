package db_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"social-vault/internal/db"
)

func TestManualAndSmartListsStore(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	b1, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:         "https://x.com/dev/status/1",
		Title:       "Go Tooling Thread",
		Description: "CLI utilities in Go",
		SiteName:    "X",
		Tags:        []string{"tools", "golang"},
	})
	if err != nil {
		t.Fatalf("create b1: %v", err)
	}
	b2, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:         "https://www.threads.com/@designer/post/2",
		Title:       "Design System Tokens",
		Description: "Typography and spacing",
		SiteName:    "Threads",
		Tags:        []string{"design", "tools"},
	})
	if err != nil {
		t.Fatalf("create b2: %v", err)
	}
	b3, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:      "https://example.org/untagged",
		Title:    "Untagged Reference",
		SiteName: "example.org",
	})
	if err != nil {
		t.Fatalf("create b3: %v", err)
	}

	// Manual list CRUD + add/remove bookmarks
	manual, err := store.CreateList(ctx, "Reading Queue", "Weekend reads")
	if err != nil {
		t.Fatalf("create manual list: %v", err)
	}

	added, err := store.AddBookmarksToList(ctx, manual.ID, []int64{b1.ID, b2.ID, b1.ID}, false)
	if err != nil || added != 2 {
		t.Fatalf("expected added=2, got added=%d err=%v", added, err)
	}

	items, total, err := store.ListBookmarks(ctx, db.BookmarkFilter{List: "Reading Queue"})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("list manual bookmarks: total=%d len=%d err=%v", total, len(items), err)
	}

	// Filter inside manual list by query
	items, total, err = store.ListBookmarks(ctx, db.BookmarkFilter{List: "Reading Queue", Query: "Typography"})
	if err != nil || total != 1 || items[0].ID != b2.ID {
		t.Fatalf("filtered manual list: total=%d items=%+v err=%v", total, items, err)
	}

	if err := store.RemoveBookmarkFromList(ctx, manual.ID, b1.ID); err != nil {
		t.Fatalf("remove bookmark from list: %v", err)
	}
	loadedManual, err := store.GetList(ctx, manual.ID, false)
	if err != nil || loadedManual.Count != 1 {
		t.Fatalf("expected manual count=1 after removal, got %+v err=%v", loadedManual, err)
	}

	// Smart list with tags (all) + domain
	smartAll, err := store.CreateListWithOptions(ctx, db.CreateListInput{
		Name:        "Go Tools on X",
		Description: "Auto-matched",
		IsSmart:     true,
		Rule: &db.SmartListRule{
			Tags:    []string{"tools", "golang"},
			TagMode: "all",
			Domain:  "https://x.com/",
		},
	})
	if err != nil {
		t.Fatalf("create smart list: %v", err)
	}

	items, total, err = store.ListBookmarks(ctx, db.BookmarkFilter{List: smartAll.Name})
	if err != nil || total != 1 || items[0].ID != b1.ID {
		t.Fatalf("expected only b1 in smartAll, got total=%d items=%+v err=%v", total, items, err)
	}

	// Adding manually to smart list must fail with ErrInvalidInput
	if _, err := store.AddBookmarksToList(ctx, smartAll.ID, []int64{b2.ID}, false); !errors.Is(err, db.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput adding to smart list, got %v", err)
	}

	// Smart list for untagged
	smartUntagged, err := store.CreateListWithOptions(ctx, db.CreateListInput{
		Name:    "Needs Tagging",
		IsSmart: true,
		Rule:    &db.SmartListRule{Untagged: true},
	})
	if err != nil {
		t.Fatalf("create untagged smart list: %v", err)
	}
	items, total, err = store.ListBookmarks(ctx, db.BookmarkFilter{List: smartUntagged.Name})
	if err != nil || total != 1 || items[0].ID != b3.ID {
		t.Fatalf("expected only b3 in untagged smart list, got total=%d err=%v", total, err)
	}

	// ListListsFiltered computes smart list counts accurately
	lists, err := store.ListListsFiltered(ctx, false)
	if err != nil || len(lists) != 3 {
		t.Fatalf("expected 3 lists, got %d err=%v", len(lists), err)
	}
	for _, l := range lists {
		if l.ID == smartAll.ID && l.Count != 1 {
			t.Fatalf("expected smartAll count=1, got %d", l.Count)
		}
		if l.ID == smartUntagged.ID && l.Count != 1 {
			t.Fatalf("expected smartUntagged count=1, got %d", l.Count)
		}
	}
}

func TestRenameAndMergeTagsStore(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	b1, _ := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.com/1",
		Title: "One",
		Tags:  []string{"devtools", "web"},
	})
	b2, _ := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.com/2",
		Title: "Two",
		Tags:  []string{"tools", "devtools"},
	})

	smart, err := store.CreateListWithOptions(ctx, db.CreateListInput{
		Name:    "Devtools Smart",
		IsSmart: true,
		Rule:    &db.SmartListRule{Tags: []string{"devtools"}},
	})
	if err != nil {
		t.Fatalf("create smart list: %v", err)
	}

	tags, _ := store.ListTags(ctx)
	var devtoolsID, toolsID int64
	for _, tg := range tags {
		switch tg.Name {
		case "devtools":
			devtoolsID = tg.ID
		case "tools":
			toolsID = tg.ID
		}
	}

	// Rename devtools -> dev-tools and check smart list rule follows
	renamed, err := store.RenameTag(ctx, devtoolsID, "dev-tools", false)
	if err != nil || renamed.Name != "dev-tools" || renamed.Count != 2 {
		t.Fatalf("rename tag failed: %+v err=%v", renamed, err)
	}

	reloadedSmart, err := store.GetList(ctx, smart.ID, false)
	if err != nil || !slices.Equal(reloadedSmart.Rule.Tags, []string{"dev-tools"}) {
		t.Fatalf("expected smart list tag updated to dev-tools, got %+v err=%v", reloadedSmart, err)
	}

	// Duplicate rename fails with ErrDuplicate
	if _, err := store.RenameTag(ctx, devtoolsID, "tools", false); !errors.Is(err, db.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate renaming onto existing tag, got %v", err)
	}

	// Merge dev-tools into tools
	mergedInto, moved, err := store.MergeTags(ctx, devtoolsID, toolsID, false)
	if err != nil {
		t.Fatalf("merge tags: %v", err)
	}
	if mergedInto.ID != toolsID || mergedInto.Count != 2 || moved != 2 {
		t.Fatalf("unexpected merge result: target=%+v moved=%d", mergedInto, moved)
	}

	reloadedB1, _ := store.GetBookmark(ctx, b1.ID)
	var b1TagNames []string
	for _, tg := range reloadedB1.Tags {
		b1TagNames = append(b1TagNames, tg.Name)
	}
	if !slices.Contains(b1TagNames, "tools") || slices.Contains(b1TagNames, "dev-tools") {
		t.Fatalf("expected b1 tags to contain tools without dev-tools, got %v", b1TagNames)
	}

	reloadedB2, _ := store.GetBookmark(ctx, b2.ID)
	if len(reloadedB2.Tags) != 1 || reloadedB2.Tags[0].Name != "tools" {
		t.Fatalf("expected b2 deduplicated to [tools], got %+v", reloadedB2.Tags)
	}

	reloadedSmartAfterMerge, _ := store.GetList(ctx, smart.ID, false)
	if !slices.Equal(reloadedSmartAfterMerge.Rule.Tags, []string{"tools"}) {
		t.Fatalf("expected smart list tag merged to tools, got %+v", reloadedSmartAfterMerge.Rule.Tags)
	}
}
