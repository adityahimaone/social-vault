package tagging_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"social-vault/internal/db"
	"social-vault/internal/tagging"
)

func TestServiceStatusTrackingAndBackfill(t *testing.T) {
	const stubKey = "stub-status-key"

	t.Run("jev disabled reports empty non-error status", func(t *testing.T) {
		store, err := db.Open(t.TempDir())
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer func() { _ = store.Close() }()

		svc := tagging.NewService(store, tagging.NewJevClient(""))
		st, err := svc.Status(context.Background())
		if err != nil {
			t.Fatalf("Status: %v", err)
		}
		if st.JevEnabled {
			t.Fatal("expected JevEnabled=false")
		}
		if st.ActiveJevJobs == nil || len(st.ActiveJevJobs) != 0 {
			t.Fatalf("expected non-nil empty ActiveJevJobs, got %#v", st.ActiveJevJobs)
		}
		if st.LastJevEvent != nil {
			t.Fatalf("expected nil LastJevEvent, got %+v", st.LastJevEvent)
		}
		if st.LastBackfill.Running || st.LastBackfill.Result != nil {
			t.Fatalf("expected idle null-result LastBackfill, got %+v", st.LastBackfill)
		}
		if st.PendingReview != 0 {
			t.Fatalf("expected PendingReview=0, got %d", st.PendingReview)
		}
	})

	t.Run("active job visibility, monotonic seq, and backfill isolation", func(t *testing.T) {
		store, err := db.Open(t.TempDir())
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer func() { _ = store.Close() }()

		var (
			mu          sync.Mutex
			startedCh   = make(chan struct{}, 4)
			releaseCh   chan struct{}
			nextStatus  = http.StatusOK
			nextPayload = `{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.92},"design":{"type":"noul","noul":0.85}}}`
		)
		setMode := func(gate bool, status int, payload string) {
			mu.Lock()
			defer mu.Unlock()
			if gate {
				releaseCh = make(chan struct{})
			} else {
				releaseCh = nil
			}
			nextStatus = status
			nextPayload = payload
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			ch := releaseCh
			status := nextStatus
			payload := nextPayload
			mu.Unlock()

			select {
			case startedCh <- struct{}{}:
			default:
			}
			if ch != nil {
				<-ch
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(payload))
		}))
		defer srv.Close()

		ctx := context.Background()
		svc := tagging.NewService(store, tagging.NewJevClientWithOptions(stubKey, srv.URL, nil))

		bm1, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://example.org/one",
			Title: "Opaque One",
		})
		if err != nil {
			t.Fatalf("create bm1: %v", err)
		}
		bm2, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://example.org/two",
			Title: "Opaque Two",
		})
		if err != nil {
			t.Fatalf("create bm2: %v", err)
		}
		bm3, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://example.org/three",
			Title: "Opaque Three",
		})
		if err != nil {
			t.Fatalf("create bm3: %v", err)
		}

		// 1. Async job with applied tags: visible while in flight, cleared after completion.
		setMode(true, http.StatusOK, `{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.92},"design":{"type":"noul","noul":0.85}}}`)
		svc.RunJevAsync(bm1)

		select {
		case <-startedCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for async jev request")
		}

		inFlight, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status in-flight: %v", err)
		}
		if !inFlight.JevEnabled {
			t.Fatal("expected JevEnabled=true")
		}
		if len(inFlight.ActiveJevJobs) != 1 || inFlight.ActiveJevJobs[0].BookmarkID != bm1.ID || inFlight.ActiveJevJobs[0].StartedAt == "" {
			t.Fatalf("unexpected ActiveJevJobs while running: %+v", inFlight.ActiveJevJobs)
		}

		mu.Lock()
		close(releaseCh)
		mu.Unlock()
		svc.Wait()

		after1, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status after1: %v", err)
		}
		if len(after1.ActiveJevJobs) != 0 {
			t.Fatalf("expected 0 ActiveJevJobs after Wait, got %+v", after1.ActiveJevJobs)
		}
		if after1.LastJevEvent == nil || after1.LastJevEvent.Seq != 1 || after1.LastJevEvent.BookmarkID != bm1.ID {
			t.Fatalf("unexpected LastJevEvent after1: %+v", after1.LastJevEvent)
		}
		if !slices.Equal(after1.LastJevEvent.AppliedTags, []string{"ai", "design"}) || after1.LastJevEvent.Queued || after1.LastJevEvent.Error != "" || after1.LastJevEvent.FinishedAt == "" {
			t.Fatalf("unexpected LastJevEvent payload after1: %+v", after1.LastJevEvent)
		}

		// 2. Async job with queued-only review tags increments seq to 2 and updates pending_review.
		setMode(false, http.StatusOK, `{"model":"jev-1.13.0","answers":{"security":{"type":"noul","noul":0.65}}}`)
		svc.RunJevAsync(bm2)
		svc.Wait()

		after2, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status after2: %v", err)
		}
		if after2.LastJevEvent == nil || after2.LastJevEvent.Seq != 2 || after2.LastJevEvent.BookmarkID != bm2.ID {
			t.Fatalf("unexpected LastJevEvent after2: %+v", after2.LastJevEvent)
		}
		if len(after2.LastJevEvent.AppliedTags) != 0 || !after2.LastJevEvent.Queued || after2.LastJevEvent.Error != "" {
			t.Fatalf("expected queued-only event after2, got %+v", after2.LastJevEvent)
		}
		if after2.PendingReview != 1 {
			t.Fatalf("expected PendingReview=1, got %d", after2.PendingReview)
		}

		// Verify AppliedTags marshals as [] rather than null.
		rawJSON, err := json.Marshal(after2)
		if err != nil {
			t.Fatalf("marshal status: %v", err)
		}
		if !strings.Contains(string(rawJSON), `"applied_tags":[]`) {
			t.Fatalf("expected applied_tags:[] in JSON, got %s", string(rawJSON))
		}

		// 3. Async job error increments seq to 3 and records short error without leaking key.
		setMode(false, http.StatusInternalServerError, `{"error":"upstream jev failure"}`)
		svc.RunJevAsync(bm3)
		svc.Wait()

		after3, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status after3: %v", err)
		}
		if after3.LastJevEvent == nil || after3.LastJevEvent.Seq != 3 || after3.LastJevEvent.BookmarkID != bm3.ID {
			t.Fatalf("unexpected LastJevEvent after3: %+v", after3.LastJevEvent)
		}
		if after3.LastJevEvent.Error == "" || strings.Contains(after3.LastJevEvent.Error, stubKey) {
			t.Fatalf("unexpected error string: %q", after3.LastJevEvent.Error)
		}
		if after3.LastJevEvent.Queued || len(after3.LastJevEvent.AppliedTags) != 0 {
			t.Fatalf("expected empty tags and queued=false on error, got %+v", after3.LastJevEvent)
		}

		// 4. Backfill status tracks running/result without altering LastJevEvent.Seq.
		// Drain startedCh first.
		for len(startedCh) > 0 {
			<-startedCh
		}
		setMode(true, http.StatusOK, `{"model":"jev-1.13.0","answers":{"finance":{"type":"noul","noul":0.91}}}`)

		type bfRes struct {
			res tagging.BackfillResult
			err error
		}
		bfDone := make(chan bfRes, 1)
		go func() {
			r, berr := svc.Backfill(ctx, 10)
			bfDone <- bfRes{res: r, err: berr}
		}()

		select {
		case <-startedCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for backfill jev request")
		}

		duringBF, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status during backfill: %v", err)
		}
		if !duringBF.LastBackfill.Running || duringBF.LastBackfill.StartedAt == "" {
			t.Fatalf("expected LastBackfill.Running=true while backfill in flight, got %+v", duringBF.LastBackfill)
		}

		mu.Lock()
		close(releaseCh)
		mu.Unlock()

		gotBF := <-bfDone
		if gotBF.err != nil {
			t.Fatalf("Backfill err: %v", gotBF.err)
		}
		if gotBF.res.Processed != 1 || gotBF.res.JevTagged != 1 {
			t.Fatalf("expected bm3 only (processed=1, jev_tagged=1), got %+v", gotBF.res)
		}

		afterBF, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status after backfill: %v", err)
		}
		if afterBF.LastBackfill.Running || afterBF.LastBackfill.FinishedAt == "" || afterBF.LastBackfill.Result == nil {
			t.Fatalf("expected completed LastBackfill with Result, got %+v", afterBF.LastBackfill)
		}
		if afterBF.LastBackfill.Result.Processed != 1 || afterBF.LastBackfill.Result.JevTagged != 1 {
			t.Fatalf("unexpected LastBackfill.Result: %+v", afterBF.LastBackfill.Result)
		}
		if afterBF.LastJevEvent == nil || afterBF.LastJevEvent.Seq != 3 {
			t.Fatalf("expected backfill NOT to modify LastJevEvent.Seq, got %+v", afterBF.LastJevEvent)
		}
	})
}

func TestBackfillLayerSelectionAndConcurrency(t *testing.T) {
	t.Run("rules-only backfill never calls Jev", func(t *testing.T) {
		store, err := db.Open(t.TempDir())
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer func() { _ = store.Close() }()

		var jevCalls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jevCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.95}}}`))
		}))
		defer srv.Close()

		ctx := context.Background()
		svc := tagging.NewService(store, tagging.NewJevClientWithOptions("stub-key", srv.URL, nil))

		bmRust, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://crates.io/crates/tokio",
			Title: "Tokio async runtime in Rust",
		})
		if err != nil {
			t.Fatalf("create bmRust: %v", err)
		}
		_, err = store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://example.org/opaque",
			Title: "Opaque Note",
		})
		if err != nil {
			t.Fatalf("create bmOpaque: %v", err)
		}

		stBefore, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status before: %v", err)
		}
		if stBefore.UntaggedCount != 2 {
			t.Fatalf("expected UntaggedCount=2, got %d", stBefore.UntaggedCount)
		}

		res, err := svc.Backfill(ctx, 10, tagging.LayerRules)
		if err != nil {
			t.Fatalf("Backfill rules-only: %v", err)
		}
		if res.Processed != 2 || res.RulesTagged != 1 || res.JevTagged != 0 || res.Queued != 0 || res.Skipped != 1 {
			t.Fatalf("unexpected rules-only result: %+v", res)
		}
		if jevCalls.Load() != 0 {
			t.Fatalf("expected 0 Jev calls on rules-only backfill, got %d", jevCalls.Load())
		}

		reloaded, err := store.GetBookmark(ctx, bmRust.ID)
		if err != nil {
			t.Fatalf("get bmRust: %v", err)
		}
		var tagNames []string
		for _, tg := range reloaded.Tags {
			tagNames = append(tagNames, tg.Name)
		}
		if !slices.Contains(tagNames, "rust") {
			t.Fatalf("expected rust tag on bmRust, got %v", tagNames)
		}

		stAfter, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status after: %v", err)
		}
		if stAfter.UntaggedCount != 1 {
			t.Fatalf("expected UntaggedCount=1 after rules-only backfill, got %d", stAfter.UntaggedCount)
		}
	})

	t.Run("jev-only backfill never applies rules", func(t *testing.T) {
		store, err := db.Open(t.TempDir())
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer func() { _ = store.Close() }()

		var jevCalls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jevCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.95}}}`))
		}))
		defer srv.Close()

		ctx := context.Background()
		svc := tagging.NewService(store, tagging.NewJevClientWithOptions("stub-key", srv.URL, nil))

		bmRust, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://crates.io/crates/serde",
			Title: "Serde serialization in Rust",
		})
		if err != nil {
			t.Fatalf("create bmRust: %v", err)
		}

		res, err := svc.Backfill(ctx, 10, tagging.LayerJev)
		if err != nil {
			t.Fatalf("Backfill jev-only: %v", err)
		}
		if res.Processed != 1 || res.RulesTagged != 0 || res.JevTagged != 1 {
			t.Fatalf("unexpected jev-only result: %+v", res)
		}
		if jevCalls.Load() != 1 {
			t.Fatalf("expected 1 Jev call, got %d", jevCalls.Load())
		}

		reloaded, err := store.GetBookmark(ctx, bmRust.ID)
		if err != nil {
			t.Fatalf("get bmRust: %v", err)
		}
		if len(reloaded.Tags) != 1 || reloaded.Tags[0].Name != "ai" {
			t.Fatalf("expected only Jev tag [ai] (no rule tag), got %+v", reloaded.Tags)
		}
	})

	t.Run("invalid layer and disabled Jev errors without partial run", func(t *testing.T) {
		store, err := db.Open(t.TempDir())
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer func() { _ = store.Close() }()

		ctx := context.Background()
		svc := tagging.NewService(store, tagging.NewJevClient(""))

		bmRust, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://crates.io/crates/clap",
			Title: "Clap CLI in Rust",
		})
		if err != nil {
			t.Fatalf("create bmRust: %v", err)
		}

		for _, bad := range []string{"mcp", ""} {
			if _, err := svc.Backfill(ctx, 10, bad); !errors.Is(err, tagging.ErrInvalidLayer) {
				t.Fatalf("expected ErrInvalidLayer for %q, got %v", bad, err)
			}
		}

		if _, err := svc.Backfill(ctx, 10, tagging.LayerJev); !errors.Is(err, tagging.ErrJevDisabled) {
			t.Fatalf("expected ErrJevDisabled for jev-only when disabled, got %v", err)
		}
		if _, err := svc.Backfill(ctx, 10, tagging.LayerRules, tagging.LayerJev); !errors.Is(err, tagging.ErrJevDisabled) {
			t.Fatalf("expected ErrJevDisabled for rules+jev when disabled, got %v", err)
		}

		reloaded, err := store.GetBookmark(ctx, bmRust.ID)
		if err != nil {
			t.Fatalf("get bmRust: %v", err)
		}
		if len(reloaded.Tags) != 0 {
			t.Fatalf("expected no partial tagging on error, got %+v", reloaded.Tags)
		}
	})

	t.Run("concurrent backfill returns ErrBackfillRunning", func(t *testing.T) {
		store, err := db.Open(t.TempDir())
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer func() { _ = store.Close() }()

		startedCh := make(chan struct{}, 1)
		releaseCh := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case startedCh <- struct{}{}:
			default:
			}
			<-releaseCh
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.92}}}`))
		}))
		defer srv.Close()

		ctx := context.Background()
		svc := tagging.NewService(store, tagging.NewJevClientWithOptions("stub-key", srv.URL, nil))
		_, err = store.CreateBookmark(ctx, db.CreateBookmarkInput{
			URL:   "https://example.org/concurrent",
			Title: "Opaque concurrent item",
		})
		if err != nil {
			t.Fatalf("create bookmark: %v", err)
		}

		doneCh := make(chan error, 1)
		go func() {
			_, berr := svc.Backfill(ctx, 10, tagging.LayerJev)
			doneCh <- berr
		}()

		select {
		case <-startedCh:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for first backfill to reach Jev stub")
		}

		if _, err := svc.Backfill(ctx, 10, tagging.LayerRules); !errors.Is(err, tagging.ErrBackfillRunning) {
			t.Fatalf("expected ErrBackfillRunning on concurrent call, got %v", err)
		}

		stDuring, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status during: %v", err)
		}
		if !stDuring.LastBackfill.Running {
			t.Fatal("expected LastBackfill.Running=true while first run is in flight")
		}

		close(releaseCh)
		if err := <-doneCh; err != nil {
			t.Fatalf("first backfill err: %v", err)
		}

		stAfter, err := svc.Status(ctx)
		if err != nil {
			t.Fatalf("Status after: %v", err)
		}
		if stAfter.LastBackfill.Running || stAfter.UntaggedCount != 0 {
			t.Fatalf("expected Running=false and UntaggedCount=0 after completion, got %+v", stAfter)
		}
	})
}
