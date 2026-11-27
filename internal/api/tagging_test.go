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
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

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

		// Backfill run 3: dismissed b3 must NOT re-enter the queue; only b4 is processed.
		bResp3 := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/backfill", `{"limit": 10}`)
		defer bResp3.Body.Close()
		if bResp3.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on backfill run 3, got %d", bResp3.StatusCode)
		}
		var res3 tagging.BackfillResult
		_ = json.NewDecoder(bResp3.Body).Decode(&res3)
		if res3.Processed != 1 || res3.Skipped != 1 || res3.Queued != 0 || res3.RulesTagged != 0 || res3.JevTagged != 0 {
			t.Fatalf("expected dismissed b3 to stay skipped in backfill run 3, got %+v", res3)
		}

		qRespAfter := doAPIRequest(t, http.MethodGet, ts.URL+"/api/tag-review", "")
		defer qRespAfter.Body.Close()
		var listAfter struct {
			Items []db.TagReviewItem `json:"items"`
		}
		_ = json.NewDecoder(qRespAfter.Body).Decode(&listAfter)
		if len(listAfter.Items) != 0 {
			t.Fatalf("expected 0 pending items after backfill run 3, got %d", len(listAfter.Items))
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

func TestTaggingStatusAPI(t *testing.T) {
	t.Run("requires auth and handles disabled Jev without error", func(t *testing.T) {
		ts, _, _ := setupTaggingTestServer(t, tagging.NewJevClient(""))

		unauthResp, err := http.Get(ts.URL + "/api/tagging/status")
		if err != nil {
			t.Fatalf("unauth status: %v", err)
		}
		unauthResp.Body.Close()
		if unauthResp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 unauth, got %d", unauthResp.StatusCode)
		}

		resp := doAPIRequest(t, http.MethodGet, ts.URL+"/api/tagging/status", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on tagging status, got %d", resp.StatusCode)
		}
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !strings.Contains(string(raw), `"active_jev_jobs":[]`) {
			t.Fatalf("expected empty JSON array for active_jev_jobs, got %s", string(raw))
		}
		var st tagging.Status
		if err := json.Unmarshal(raw, &st); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if st.JevEnabled || len(st.ActiveJevJobs) != 0 || st.LastJevEvent != nil || st.LastBackfill.Running || st.LastBackfill.Result != nil || st.PendingReview != 0 {
			t.Fatalf("unexpected disabled status: %+v", st)
		}
	})

	t.Run("reports async jev event, pending review, and last backfill", func(t *testing.T) {
		jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"model": "jev-1.13.0",
				"answers": {
					"ai": {"type": "noul", "noul": 0.91},
					"security": {"type": "noul", "noul": 0.64}
				}
			}`))
		}))
		defer jevSrv.Close()

		ts, _, tagger := setupTaggingTestServer(t, tagging.NewJevClientWithOptions("stub-key", jevSrv.URL, nil))

		saveResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/bookmarks", `{
			"url": "https://example.org/status-async",
			"title": "Opaque async status check",
			"description": "No keyword match"
		}`)
		saveResp.Body.Close()
		if saveResp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", saveResp.StatusCode)
		}

		tagger.Wait()

		stResp := doAPIRequest(t, http.MethodGet, ts.URL+"/api/tagging/status", "")
		defer stResp.Body.Close()
		if stResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on status, got %d", stResp.StatusCode)
		}
		var st tagging.Status
		if err := json.NewDecoder(stResp.Body).Decode(&st); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if !st.JevEnabled || len(st.ActiveJevJobs) != 0 {
			t.Fatalf("expected jev_enabled=true and no active jobs, got %+v", st)
		}
		if st.LastJevEvent == nil || st.LastJevEvent.Seq != 1 || !slices.Equal(st.LastJevEvent.AppliedTags, []string{"ai"}) || !st.LastJevEvent.Queued || st.LastJevEvent.Error != "" {
			t.Fatalf("unexpected LastJevEvent: %+v", st.LastJevEvent)
		}
		if st.PendingReview != 1 {
			t.Fatalf("expected PendingReview=1, got %d", st.PendingReview)
		}

		bfResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/backfill", `{}`)
		bfResp.Body.Close()
		if bfResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on backfill, got %d", bfResp.StatusCode)
		}

		stResp2 := doAPIRequest(t, http.MethodGet, ts.URL+"/api/tagging/status", "")
		defer stResp2.Body.Close()
		var st2 tagging.Status
		if err := json.NewDecoder(stResp2.Body).Decode(&st2); err != nil {
			t.Fatalf("decode status2: %v", err)
		}
		if st2.LastBackfill.Running || st2.LastBackfill.Result == nil || st2.LastBackfill.Result.Processed != 0 {
			t.Fatalf("unexpected LastBackfill after backfill: %+v", st2.LastBackfill)
		}
		if st2.LastJevEvent == nil || st2.LastJevEvent.Seq != 1 {
			t.Fatalf("expected backfill not to alter LastJevEvent.Seq, got %+v", st2.LastJevEvent)
		}
	})
}

func TestBackfillLayersAndGuardsAPI(t *testing.T) {
	t.Run("rules-only, jev-only, invalid layer 400, and untagged_count", func(t *testing.T) {
		var jevCalls atomic.Int32
		jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jevCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.92}}}`))
		}))
		defer jevSrv.Close()

		ts, store, _ := setupTaggingTestServer(t, tagging.NewJevClientWithOptions("stub-key", jevSrv.URL, nil))
		ctx := context.Background()

		// Seed two Rust bookmarks (rules would match both) and one opaque bookmark.
		bm1, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://crates.io/crates/axum",
			Title: "Axum Rust server",
		})
		if err != nil {
			t.Fatalf("create bm1: %v", err)
		}
		_, err = store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://example.org/opaque",
			Title: "Opaque item",
		})
		if err != nil {
			t.Fatalf("create bm2: %v", err)
		}

		// Invalid layer -> 400
		badResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/backfill", `{"layers":["mcp"]}`)
		badResp.Body.Close()
		if badResp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid layer, got %d", badResp.StatusCode)
		}

		// Status untagged_count == 2
		stResp1 := doAPIRequest(t, http.MethodGet, ts.URL+"/api/tagging/status", "")
		var st1 tagging.Status
		_ = json.NewDecoder(stResp1.Body).Decode(&st1)
		stResp1.Body.Close()
		if st1.UntaggedCount != 2 {
			t.Fatalf("expected untagged_count=2, got %d", st1.UntaggedCount)
		}

		// Rules-only backfill -> tags bm1, 0 Jev calls
		rResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/backfill", `{"layers":["rules"]}`)
		var rRes tagging.BackfillResult
		_ = json.NewDecoder(rResp.Body).Decode(&rRes)
		rResp.Body.Close()
		if rResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on rules-only backfill, got %d", rResp.StatusCode)
		}
		if rRes.Processed != 2 || rRes.RulesTagged != 1 || rRes.JevTagged != 0 || rRes.Skipped != 1 {
			t.Fatalf("unexpected rules-only result: %+v", rRes)
		}
		if jevCalls.Load() != 0 {
			t.Fatalf("expected 0 Jev calls on rules-only backfill, got %d", jevCalls.Load())
		}

		// Status untagged_count == 1
		stResp2 := doAPIRequest(t, http.MethodGet, ts.URL+"/api/tagging/status", "")
		var st2 tagging.Status
		_ = json.NewDecoder(stResp2.Body).Decode(&st2)
		stResp2.Body.Close()
		if st2.UntaggedCount != 1 {
			t.Fatalf("expected untagged_count=1 after rules-only, got %d", st2.UntaggedCount)
		}

		// Seed another Rust bookmark, clear tags on bm1 is unnecessary; jev-only should not run rules on bm3.
		bm3, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://crates.io/crates/tokio",
			Title: "Tokio Rust runtime",
		})
		if err != nil {
			t.Fatalf("create bm3: %v", err)
		}
		_ = bm1

		jResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/backfill", `{"layers":["jev"]}`)
		var jRes tagging.BackfillResult
		_ = json.NewDecoder(jResp.Body).Decode(&jRes)
		jResp.Body.Close()
		if jResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 on jev-only backfill, got %d", jResp.StatusCode)
		}
		if jRes.Processed != 2 || jRes.RulesTagged != 0 || jRes.JevTagged != 2 {
			t.Fatalf("unexpected jev-only result: %+v", jRes)
		}
		if jevCalls.Load() != 2 {
			t.Fatalf("expected 2 Jev calls on jev-only backfill, got %d", jevCalls.Load())
		}

		reloadedBM3, err := store.GetBookmark(ctx, bm3.ID)
		if err != nil {
			t.Fatalf("get bm3: %v", err)
		}
		if len(reloadedBM3.Tags) != 1 || reloadedBM3.Tags[0].Name != "ai" {
			t.Fatalf("expected only [ai] tag on bm3 after jev-only backfill, got %+v", reloadedBM3.Tags)
		}

		stResp3 := doAPIRequest(t, http.MethodGet, ts.URL+"/api/tagging/status", "")
		var st3 tagging.Status
		_ = json.NewDecoder(stResp3.Body).Decode(&st3)
		stResp3.Body.Close()
		if st3.UntaggedCount != 0 {
			t.Fatalf("expected untagged_count=0 after jev-only, got %d", st3.UntaggedCount)
		}
	})

	t.Run("jev disabled returns 400 without partial run", func(t *testing.T) {
		ts, store, _ := setupTaggingTestServer(t, tagging.NewJevClient(""))
		ctx := context.Background()

		bmRust, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://crates.io/crates/serde",
			Title: "Serde Rust",
		})
		if err != nil {
			t.Fatalf("create bmRust: %v", err)
		}

		for _, payload := range []string{`{"layers":["jev"]}`, `{"layers":["rules","jev"]}`} {
			resp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/backfill", payload)
			var errBody struct {
				Error string `json:"error"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&errBody)
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400 for %s with disabled Jev, got %d", payload, resp.StatusCode)
			}
			if !strings.Contains(errBody.Error, "Jev tidak aktif") {
				t.Fatalf("unexpected error message for %s: %q", payload, errBody.Error)
			}
		}

		reloaded, err := store.GetBookmark(ctx, bmRust.ID)
		if err != nil {
			t.Fatalf("get bmRust: %v", err)
		}
		if len(reloaded.Tags) != 0 {
			t.Fatalf("expected no partial run when Jev disabled, got %+v", reloaded.Tags)
		}
	})

	t.Run("concurrent backfill returns 409 Conflict", func(t *testing.T) {
		startedCh := make(chan struct{}, 1)
		releaseCh := make(chan struct{})
		jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case startedCh <- struct{}{}:
			default:
			}
			<-releaseCh
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.92}}}`))
		}))
		defer jevSrv.Close()

		ts, store, _ := setupTaggingTestServer(t, tagging.NewJevClientWithOptions("stub-key", jevSrv.URL, nil))
		ctx := context.Background()

		_, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://example.org/slow",
			Title: "Slow Jev item",
		})
		if err != nil {
			t.Fatalf("create bookmark: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(1)
		var firstStatus int
		go func() {
			defer wg.Done()
			resp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/backfill", `{"layers":["jev"]}`)
			firstStatus = resp.StatusCode
			resp.Body.Close()
		}()

		select {
		case <-startedCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for first backfill HTTP call")
		}

		secondResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/backfill", `{"layers":["rules"]}`)
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(secondResp.Body).Decode(&errBody)
		secondResp.Body.Close()
		if secondResp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409 Conflict on concurrent backfill, got %d (%s)", secondResp.StatusCode, errBody.Error)
		}
		if errBody.Error != "Backfill sedang berjalan" {
			t.Fatalf("unexpected 409 message: %q", errBody.Error)
		}

		close(releaseCh)
		wg.Wait()
		if firstStatus != http.StatusOK {
			t.Fatalf("expected first backfill to finish 200, got %d", firstStatus)
		}
	})
}

func TestSuggestUntaggedAndBatchReviewAPI(t *testing.T) {
	jevSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			State struct {
				Title string `json:"title"`
			} `json:"state"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(req.State.Title, "Alpha") {
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.93},"tools":{"type":"noul","noul":0.64}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"design":{"type":"noul","noul":0.71}}}`))
	}))
	defer jevSrv.Close()

	ts, store, _ := setupTaggingTestServer(t, tagging.NewJevClientWithOptions("stub-key", jevSrv.URL, nil))
	ctx := context.Background()

	bm1, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.org/alpha",
		Title: "Alpha Untapped Tool",
	})
	if err != nil {
		t.Fatalf("create bm1: %v", err)
	}
	bm2, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.org/beta",
		Title: "Beta Design Sketch",
	})
	if err != nil {
		t.Fatalf("create bm2: %v", err)
	}

	// Suggest untagged must queue suggestions (>=0.5, including >=0.8) WITHOUT auto-applying tags.
	sugResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tagging/suggest-untagged", `{"limit":50}`)
	var sugOut tagging.SuggestResult
	_ = json.NewDecoder(sugResp.Body).Decode(&sugOut)
	sugResp.Body.Close()
	if sugResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on suggest-untagged, got %d", sugResp.StatusCode)
	}
	if sugOut.Processed != 2 || sugOut.Queued != 2 || len(sugOut.Items) != 2 {
		t.Fatalf("unexpected suggest-untagged output: %+v", sugOut)
	}

	reloaded1, _ := store.GetBookmark(ctx, bm1.ID)
	reloaded2, _ := store.GetBookmark(ctx, bm2.ID)
	if len(reloaded1.Tags) != 0 || len(reloaded2.Tags) != 0 {
		t.Fatalf("expected zero auto-applied tags before review, got bm1=%+v bm2=%+v", reloaded1.Tags, reloaded2.Tags)
	}

	// Apply bm1 with subset override ["tools"] instead of ["ai", "tools"]
	var alphaItemID, betaItemID int64
	for _, it := range sugOut.Items {
		if it.BookmarkID == bm1.ID {
			alphaItemID = it.ID
		} else if it.BookmarkID == bm2.ID {
			betaItemID = it.ID
		}
	}

	appResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tag-review/"+strconvI64(alphaItemID)+"/apply", `{"tags":["tools"]}`)
	var appOut struct {
		Bookmark db.Bookmark `json:"bookmark"`
	}
	_ = json.NewDecoder(appResp.Body).Decode(&appOut)
	appResp.Body.Close()
	if appResp.StatusCode != http.StatusOK || len(appOut.Bookmark.Tags) != 1 || appOut.Bookmark.Tags[0].Name != "tools" {
		t.Fatalf("expected subset apply [tools], got status=%d bm=%+v", appResp.StatusCode, appOut.Bookmark)
	}

	// Batch apply remaining item(s)
	batchResp := doAPIRequest(t, http.MethodPost, ts.URL+"/api/tag-review/batch", `{"action":"apply","ids":[`+strconvI64(betaItemID)+`]}`)
	var batchOut struct {
		Succeeded int `json:"succeeded"`
	}
	_ = json.NewDecoder(batchResp.Body).Decode(&batchOut)
	batchResp.Body.Close()
	if batchResp.StatusCode != http.StatusOK || batchOut.Succeeded != 1 {
		t.Fatalf("expected batch apply succeeded=1, got status=%d out=%+v", batchResp.StatusCode, batchOut)
	}

	reloaded2After, _ := store.GetBookmark(ctx, bm2.ID)
	if len(reloaded2After.Tags) != 1 || reloaded2After.Tags[0].Name != "design" {
		t.Fatalf("expected bm2 tagged [design] after batch apply, got %+v", reloaded2After.Tags)
	}
}
