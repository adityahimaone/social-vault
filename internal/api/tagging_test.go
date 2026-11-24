package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"social-vault/internal/api"
	"social-vault/internal/auth"
	"social-vault/internal/crawler"
	"social-vault/internal/db"
	"social-vault/internal/tagging"
)

func setupTaggingTestServer(t *testing.T, jev *tagging.JevClient) (*httptest.Server, *db.Store, *tagging.Service) {
	t.Helper()
	store, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	authenticator := auth.New(store, "vault-pass", "vault-key")
	fetcher := crawler.NewFetcher(nil)
	webFS := fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><html><title>Social Vault</title></html>")},
	}
	tagger := tagging.NewService(store, jev)
	srv := api.NewServer(store, authenticator, fetcher, webFS, tagger)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, store, tagger
}

func doAPIRequest(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = bytes.NewBufferString(body)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer vault-key")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	return resp
}

func TestHybridAutoTaggingAPI(t *testing.T) {
	t.Run("layer 1 synchronous rules preserve user tags and skip Jev", func(t *testing.T) {
		var jevCalls atomic.Int32
		jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jevCalls.Add(1)
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.9}}}`))
		}))
		defer jevSrv.Close()

		ts, _, tagger := setupTaggingTestServer(t, tagging.NewJevClientWithOptions("stub-key", jevSrv.URL, nil))

		resp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/bookmarks", `{
			"url": "https://crates.io/crates/axum",
			"title": "Axum web framework in Rust",
			"description": "Ergonomic Rust HTTP server",
			"tags": ["my-pick"]
		}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}
		var bm db.Bookmark
		_ = json.NewDecoder(resp.Body).Decode(&bm)

		tagger.Wait()

		var names []string
		for _, tg := range bm.Tags {
			names = append(names, tg.Name)
		}
		for _, want := range []string{"my-pick", "rust"} {
			if !slices.Contains(names, want) {
				t.Fatalf("expected tag %q in %v", want, names)
			}
		}
		if jevCalls.Load() != 0 {
			t.Fatalf("expected Jev not to be called when Layer 1 tagged bookmark, got %d calls", jevCalls.Load())
		}
	})

	t.Run("layer 2 async Jev auto-applies >=0.8 and queues [0.5,0.8)", func(t *testing.T) {
		jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"model": "jev-1.13.0",
				"answers": {
					"golang": {"type": "noul", "noul": 0.88},
					"security": {"type": "noul", "noul": 0.64},
					"sports": {"type": "noul", "noul": 0.12}
				}
			}`))
		}))
		defer jevSrv.Close()

		ts, store, tagger := setupTaggingTestServer(t, tagging.NewJevClientWithOptions("stub-key", jevSrv.URL, nil))

		resp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/bookmarks", `{
			"url": "https://example.org/post/42",
			"title": "Minimal post title",
			"description": "Short opaque note"
		}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}
		var created db.Bookmark
		_ = json.NewDecoder(resp.Body).Decode(&created)

		tagger.Wait()

		reloaded, err := store.GetBookmark(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("get bookmark: %v", err)
		}
		if len(reloaded.Tags) != 1 || reloaded.Tags[0].Name != "golang" {
			t.Fatalf("expected async auto-tag [golang], got %+v", reloaded.Tags)
		}

		// Review queue should contain security@0.64.
		qResp := doAPIRequest(t, http.MethodGet, ts.URL+"/api/tag-review", "")
		defer qResp.Body.Close()
		if qResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on list review, got %d", qResp.StatusCode)
		}
		var listOut struct {
			Items []db.TagReviewItem `json:"items"`
			Total int                `json:"total"`
		}
		_ = json.NewDecoder(qResp.Body).Decode(&listOut)
		if listOut.Total != 1 || !slices.Equal(listOut.Items[0].SuggestedTags, []string{"security"}) {
			t.Fatalf("unexpected review queue: %+v", listOut)
		}

		// Apply review item -> merges "security" with "golang"
		appResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tag-review/"+strconvI64(listOut.Items[0].ID)+"/apply", "")
		defer appResp.Body.Close()
		if appResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on apply, got %d", appResp.StatusCode)
		}
		var appOut struct {
			Item     db.TagReviewItem `json:"item"`
			Bookmark db.Bookmark      `json:"bookmark"`
		}
		_ = json.NewDecoder(appResp.Body).Decode(&appOut)
		if appOut.Item.Status != "applied" || len(appOut.Bookmark.Tags) != 2 {
			t.Fatalf("unexpected apply response: %+v", appOut)
		}

		// Re-applying returns 409 Conflict
		dupAppResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tag-review/"+strconvI64(listOut.Items[0].ID)+"/apply", "")
		dupAppResp.Body.Close()
		if dupAppResp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409 on double apply, got %d", dupAppResp.StatusCode)
		}
	})

	t.Run("no JEV_API_KEY still succeeds and leaves review queue empty", func(t *testing.T) {
		ts, _, tagger := setupTaggingTestServer(t, tagging.NewJevClient(""))
		resp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/bookmarks", `{
			"url": "https://example.org/opaque",
			"title": "Opaque note",
			"description": "No rules match here"
		}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.StatusCode)
		}
		tagger.Wait()

		qResp := doAPIRequest(t, http.MethodGet, ts.URL+"/api/tag-review", "")
		defer qResp.Body.Close()
		var listOut struct {
			Total int `json:"total"`
		}
		_ = json.NewDecoder(qResp.Body).Decode(&listOut)
		if listOut.Total != 0 {
			t.Fatalf("expected empty queue when JEV_API_KEY unset, got %d", listOut.Total)
		}
	})

	t.Run("backfill counts and idempotency plus dismiss", func(t *testing.T) {
		jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				State struct {
					Title string `json:"title"`
				} `json:"state"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.Contains(req.State.Title, "Rust"):
				_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"rust":{"type":"noul","noul":0.95}}}`))
			case strings.Contains(req.State.Title, "Opaque Finance"):
				_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"finance":{"type":"noul","noul":0.9}}}`))
			case strings.Contains(req.State.Title, "Uncertain Security"):
				_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"security":{"type":"noul","noul":0.66}}}`))
			default:
				_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.1}}}`))
			}
		}))
		defer jevSrv.Close()

		ts, store, _ := setupTaggingTestServer(t, tagging.NewJevClientWithOptions("stub-key", jevSrv.URL, nil))
		ctx := context.Background()

		// Seed 4 untagged bookmarks directly in DB (bypassing POST auto-tagger)
		_, _ = store.CreateBookmark(ctx, db.CreateBookmarkInput{URL: "https://example.org/b1", Title: "Rust Ownership Deep Dive"})
		_, _ = store.CreateBookmark(ctx, db.CreateBookmarkInput{URL: "https://example.org/b2", Title: "Opaque Finance Post"})
		_, _ = store.CreateBookmark(ctx, db.CreateBookmarkInput{URL: "https://example.org/b3", Title: "Uncertain Security Post"})
		_, _ = store.CreateBookmark(ctx, db.CreateBookmarkInput{URL: "https://example.org/b4", Title: "Bare Emoji Post"})

		// Backfill run 1
		bResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/backfill", `{}`)
		defer bResp.Body.Close()
		if bResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on backfill, got %d", bResp.StatusCode)
		}
		var res1 tagging.BackfillResult
		_ = json.NewDecoder(bResp.Body).Decode(&res1)
		if res1.Processed != 4 || res1.RulesTagged != 1 || res1.JevTagged != 1 || res1.Queued != 1 || res1.Skipped != 1 || res1.Failed != 0 {
			t.Fatalf("unexpected backfill result 1: %+v", res1)
		}

		// Backfill run 2: only b4 remains eligible (b1, b2 have tags; b3 has pending review)
		bResp2 := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/backfill", `{"limit": 10}`)
		defer bResp2.Body.Close()
		var res2 tagging.BackfillResult
		_ = json.NewDecoder(bResp2.Body).Decode(&res2)
		if res2.Processed != 1 || res2.Skipped != 1 || res2.RulesTagged != 0 || res2.JevTagged != 0 || res2.Queued != 0 {
			t.Fatalf("unexpected backfill result 2: %+v", res2)
		}

		// Dismiss the queued review for b3
		qResp := doAPIRequest(t, http.MethodGet, ts.URL+"/api/tag-review", "")
		defer qResp.Body.Close()
		var listOut struct {
			Items []db.TagReviewItem `json:"items"`
		}
		_ = json.NewDecoder(qResp.Body).Decode(&listOut)
		if len(listOut.Items) != 1 {
			t.Fatalf("expected 1 queued item, got %d", len(listOut.Items))
		}
		dResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tag-review/"+strconvI64(listOut.Items[0].ID)+"/dismiss", "")
		defer dResp.Body.Close()
		if dResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on dismiss, got %d", dResp.StatusCode)
		}

		// Unauthenticated request to /api/tag-review returns 401
		unauthResp, err := http.Get(ts.URL + "/api/tag-review")
		if err != nil {
			t.Fatalf("unauth review: %v", err)
		}
		unauthResp.Body.Close()
		if unauthResp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 unauth, got %d", unauthResp.StatusCode)
		}
	})
}
