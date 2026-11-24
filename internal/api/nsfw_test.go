package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"social-vault/internal/api"
	"social-vault/internal/auth"
	"social-vault/internal/crawler"
	"social-vault/internal/db"
)

const testNsfwPass = "test-nsfw-pass"

func setupNsfwTestServer(t *testing.T, nsfwPassword string) (*httptest.Server, *db.Store) {
	t.Helper()
	store, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	authenticator := auth.New(store, "vault-pass", "vault-key").WithNsfwPassword(nsfwPassword)
	fetcher := crawler.NewFetcher(nil)
	webFS := fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><html><title>Social Vault</title></html>")},
	}
	srv := api.NewServer(store, authenticator, fetcher, webFS, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, store
}

func newLoggedInClient(t *testing.T, baseURL string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{Jar: jar}
	resp, err := client.Post(baseURL+"/api/auth/login", "application/json", strings.NewReader(`{"password":"vault-pass"}`))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on login, got %d", resp.StatusCode)
	}
	return client
}

func TestNsfwLockEndpointsAndUnconfigured(t *testing.T) {
	ts, store := setupNsfwTestServer(t, "")
	ctx := context.Background()

	// Seed normal + nsfw bookmarks directly in store.
	if _, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.org/safe",
		Title: "Safe Item",
		Tags:  []string{"golang"},
	}); err != nil {
		t.Fatalf("seed safe: %v", err)
	}
	nsfwBM, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.org/spicy",
		Title: "Spicy Secret Item",
		Tags:  []string{"nsfw"},
	})
	if err != nil {
		t.Fatalf("seed nsfw: %v", err)
	}

	// Unauthenticated /api/nsfw/* -> 401.
	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/nsfw/status", ""},
		{http.MethodPost, "/api/nsfw/unlock", `{"password":"` + testNsfwPass + `"}`},
		{http.MethodPost, "/api/nsfw/lock", ""},
	} {
		req, _ := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(tc.body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 unauthenticated for %s %s, got %d", tc.method, tc.path, resp.StatusCode)
		}
	}

	// Logged-in cookie session when NSFW_TAG_PASSWORD is empty: fails closed.
	client := newLoggedInClient(t, ts.URL)

	stResp, err := client.Get(ts.URL + "/api/nsfw/status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var st struct {
		Configured bool `json:"configured"`
		Locked     bool `json:"locked"`
	}
	_ = json.NewDecoder(stResp.Body).Decode(&st)
	stResp.Body.Close()
	if st.Configured || !st.Locked {
		t.Fatalf("expected configured=false, locked=true for cookie session, got %+v", st)
	}

	// Unlock returns 503 when unconfigured.
	uResp, err := client.Post(ts.URL+"/api/nsfw/unlock", "application/json", strings.NewReader(`{"password":"`+testNsfwPass+`"}`))
	if err != nil {
		t.Fatalf("unlock unconfigured: %v", err)
	}
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(uResp.Body).Decode(&errBody)
	uResp.Body.Close()
	if uResp.StatusCode != http.StatusServiceUnavailable || errBody.Error != "NSFW lock tidak dikonfigurasi" {
		t.Fatalf("expected 503 'NSFW lock tidak dikonfigurasi', got %d (%q)", uResp.StatusCode, errBody.Error)
	}

	// Content remains hidden from cookie session.
	listResp, _ := client.Get(ts.URL + "/api/bookmarks")
	var listOut struct {
		Items []db.Bookmark `json:"items"`
		Total int           `json:"total"`
	}
	_ = json.NewDecoder(listResp.Body).Decode(&listOut)
	listResp.Body.Close()
	if listOut.Total != 1 || len(listOut.Items) != 1 || listOut.Items[0].ID == nsfwBM.ID {
		t.Fatalf("expected nsfw bookmark hidden when unconfigured, got %+v", listOut)
	}

	// API key sees everything even when unconfigured.
	apiReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/bookmarks", nil)
	apiReq.Header.Set("Authorization", "Bearer vault-key")
	apiResp, err := http.DefaultClient.Do(apiReq)
	if err != nil {
		t.Fatalf("api bookmarks: %v", err)
	}
	var apiList struct {
		Items []db.Bookmark `json:"items"`
		Total int           `json:"total"`
	}
	_ = json.NewDecoder(apiResp.Body).Decode(&apiList)
	apiResp.Body.Close()
	if apiList.Total != 2 || len(apiList.Items) != 2 {
		t.Fatalf("expected API key to see all 2 bookmarks, got %+v", apiList)
	}
}

func TestNsfwLockLifecycleAndFiltering(t *testing.T) {
	ts, store := setupNsfwTestServer(t, testNsfwPass)
	ctx := context.Background()

	safeBM, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.org/safe-post",
		Title: "Golang Concurrency Patterns",
		Tags:  []string{"golang"},
	})
	if err != nil {
		t.Fatalf("create safeBM: %v", err)
	}
	nsfwBM1, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.org/nsfw-one",
		Title: "Hidden Spicy Artwork",
		Notes: "unique-nsfw-token-xyz",
		Tags:  []string{"nsfw"},
	})
	if err != nil {
		t.Fatalf("create nsfwBM1: %v", err)
	}
	nsfwBM2, err := store.CreateBookmark(ctx, db.CreateBookmarkInput{
		URL:   "https://example.org/nsfw-two",
		Title: "Second Hidden Item",
		Tags:  []string{"NSFW", "art"},
	})
	if err != nil {
		t.Fatalf("create nsfwBM2: %v", err)
	}

	// Enqueue one review item on safeBM and one on nsfwBM1.
	if _, _, err := store.EnqueueTagReview(ctx, safeBM.ID, []string{"reading"}, map[string]float64{"reading": 0.65}, "jev"); err != nil {
		t.Fatalf("enqueue safe review: %v", err)
	}
	if _, _, err := store.EnqueueTagReview(ctx, nsfwBM1.ID, []string{"entertainment"}, map[string]float64{"entertainment": 0.62}, "jev"); err != nil {
		t.Fatalf("enqueue nsfw review: %v", err)
	}

	// Find nsfw tag ID via store.
	allTags, err := store.ListTags(ctx)
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	var nsfwTagID int64
	for _, tg := range allTags {
		if strings.EqualFold(tg.Name, "nsfw") {
			nsfwTagID = tg.ID
			break
		}
	}
	if nsfwTagID == 0 {
		t.Fatalf("expected nsfw tag to exist in store")
	}

	client := newLoggedInClient(t, ts.URL)

	// 1. Locked cookie session: status -> configured=true, locked=true.
	stResp, _ := client.Get(ts.URL + "/api/nsfw/status")
	var st struct {
		Configured bool `json:"configured"`
		Locked     bool `json:"locked"`
	}
	_ = json.NewDecoder(stResp.Body).Decode(&st)
	stResp.Body.Close()
	if !st.Configured || !st.Locked {
		t.Fatalf("expected configured=true, locked=true, got %+v", st)
	}

	// 2. List, FTS search, ?tag=nsfw, ?tag=<id> all hide nsfw bookmarks.
	for _, path := range []string{
		"/api/bookmarks",
		"/api/bookmarks?q=unique-nsfw-token-xyz",
		"/api/bookmarks?tag=nsfw",
		"/api/bookmarks?tag=" + strconvI64(nsfwTagID),
	} {
		resp, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		var out struct {
			Items []db.Bookmark `json:"items"`
			Total int           `json:"total"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		for _, item := range out.Items {
			if item.ID == nsfwBM1.ID || item.ID == nsfwBM2.ID {
				t.Fatalf("locked GET %s leaked nsfw bookmark %d", path, item.ID)
			}
		}
		if path != "/api/bookmarks" && out.Total != 0 {
			t.Fatalf("expected total=0 for locked %s, got %d", path, out.Total)
		}
		if path == "/api/bookmarks" && out.Total != 1 {
			t.Fatalf("expected total=1 for locked list, got %d", out.Total)
		}
	}

	// 3. Get bookmark by ID: nsfw -> 404, safe -> 200.
	getNsfwResp, _ := client.Get(ts.URL + "/api/bookmarks/" + strconvI64(nsfwBM1.ID))
	getNsfwResp.Body.Close()
	if getNsfwResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for locked nsfw bookmark, got %d", getNsfwResp.StatusCode)
	}
	getSafeResp, _ := client.Get(ts.URL + "/api/bookmarks/" + strconvI64(safeBM.ID))
	getSafeResp.Body.Close()
	if getSafeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for safe bookmark, got %d", getSafeResp.StatusCode)
	}

	// 4. List tags excludes nsfw.
	tagsResp, _ := client.Get(ts.URL + "/api/tags")
	var tagsOut struct {
		Items []db.Tag `json:"items"`
	}
	_ = json.NewDecoder(tagsResp.Body).Decode(&tagsOut)
	tagsResp.Body.Close()
	for _, tg := range tagsOut.Items {
		if strings.EqualFold(tg.Name, "nsfw") {
			t.Fatalf("locked GET /api/tags leaked nsfw tag: %+v", tg)
		}
	}

	// 5. Stats: top_tags excludes nsfw, while total/active stay unchanged (3).
	statsResp, _ := client.Get(ts.URL + "/api/stats")
	var statsOut db.Stats
	_ = json.NewDecoder(statsResp.Body).Decode(&statsOut)
	statsResp.Body.Close()
	if statsOut.Total != 3 || statsOut.Active != 3 {
		t.Fatalf("expected unchanged total=3 active=3, got %+v", statsOut)
	}
	for _, tg := range statsOut.TopTags {
		if strings.EqualFold(tg.Name, "nsfw") {
			t.Fatalf("locked GET /api/stats leaked nsfw in top_tags: %+v", statsOut.TopTags)
		}
	}

	// 6. Tag-review queue hides nsfwBM1's review item, keeps safeBM's.
	revResp, _ := client.Get(ts.URL + "/api/tag-review")
	var revOut struct {
		Items []db.TagReviewItem `json:"items"`
		Total int                `json:"total"`
	}
	_ = json.NewDecoder(revResp.Body).Decode(&revOut)
	revResp.Body.Close()
	if revOut.Total != 1 || len(revOut.Items) != 1 || revOut.Items[0].BookmarkID != safeBM.ID {
		t.Fatalf("expected only safe review item while locked, got %+v", revOut)
	}

	// 7. Create/update bookmark with nsfw tag -> 400; update/delete existing nsfw bookmark -> 404.
	createNsfwResp, _ := client.Post(ts.URL+"/api/bookmarks", "application/json", strings.NewReader(`{"url":"https://example.org/new-spicy","title":"New","tags":["NSFW"]}`))
	createNsfwResp.Body.Close()
	if createNsfwResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 creating bookmark with nsfw tag while locked, got %d", createNsfwResp.StatusCode)
	}

	patchSafeReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/bookmarks/"+strconvI64(safeBM.ID), strings.NewReader(`{"tags":["golang","nsfw"]}`))
	patchSafeReq.Header.Set("Content-Type", "application/json")
	patchSafeResp, _ := client.Do(patchSafeReq)
	patchSafeResp.Body.Close()
	if patchSafeResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 assigning nsfw tag via PATCH while locked, got %d", patchSafeResp.StatusCode)
	}

	patchNsfwReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/bookmarks/"+strconvI64(nsfwBM1.ID), strings.NewReader(`{"notes":"tamper"}`))
	patchNsfwReq.Header.Set("Content-Type", "application/json")
	patchNsfwResp, _ := client.Do(patchNsfwReq)
	patchNsfwResp.Body.Close()
	if patchNsfwResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 patching locked nsfw bookmark, got %d", patchNsfwResp.StatusCode)
	}

	// 8. Delete nsfw tag ID while locked -> 404 and tag still exists.
	delTagReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/tags/"+strconvI64(nsfwTagID), nil)
	delTagResp, _ := client.Do(delTagReq)
	delTagResp.Body.Close()
	if delTagResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 deleting locked nsfw tag, got %d", delTagResp.StatusCode)
	}

	// 8b. While cookie session is locked, API-key requests still bypass the lock across all endpoints.
	doAPIKeyGet := func(path string) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		req.Header.Set("Authorization", "Bearer vault-key")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("API-key GET %s: %v", path, err)
		}
		return resp
	}

	apiStResp := doAPIKeyGet("/api/nsfw/status")
	var apiSt struct {
		Configured bool `json:"configured"`
		Locked     bool `json:"locked"`
	}
	_ = json.NewDecoder(apiStResp.Body).Decode(&apiSt)
	apiStResp.Body.Close()
	if !apiSt.Configured || apiSt.Locked {
		t.Fatalf("expected API-key /api/nsfw/status configured=true locked=false, got %+v", apiSt)
	}

	apiBmResp := doAPIKeyGet("/api/bookmarks")
	var apiBmOut struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(apiBmResp.Body).Decode(&apiBmOut)
	apiBmResp.Body.Close()
	if apiBmOut.Total != 3 {
		t.Fatalf("expected API-key GET /api/bookmarks total=3, got %d", apiBmOut.Total)
	}

	apiGetNsfwResp := doAPIKeyGet("/api/bookmarks/" + strconvI64(nsfwBM1.ID))
	apiGetNsfwResp.Body.Close()
	if apiGetNsfwResp.StatusCode != http.StatusOK {
		t.Fatalf("expected API-key GET nsfw bookmark 200, got %d", apiGetNsfwResp.StatusCode)
	}

	apiTagsResp := doAPIKeyGet("/api/tags")
	var apiTagsOut struct {
		Items []db.Tag `json:"items"`
	}
	_ = json.NewDecoder(apiTagsResp.Body).Decode(&apiTagsOut)
	apiTagsResp.Body.Close()
	var apiTagNames []string
	for _, tg := range apiTagsOut.Items {
		apiTagNames = append(apiTagNames, strings.ToLower(tg.Name))
	}
	if !slices.Contains(apiTagNames, "nsfw") {
		t.Fatalf("expected API-key GET /api/tags to include nsfw, got %v", apiTagNames)
	}

	apiStatsResp := doAPIKeyGet("/api/stats")
	var apiStatsOut db.Stats
	_ = json.NewDecoder(apiStatsResp.Body).Decode(&apiStatsOut)
	apiStatsResp.Body.Close()
	var apiTopTagNames []string
	for _, tg := range apiStatsOut.TopTags {
		apiTopTagNames = append(apiTopTagNames, strings.ToLower(tg.Name))
	}
	if !slices.Contains(apiTopTagNames, "nsfw") {
		t.Fatalf("expected API-key GET /api/stats top_tags to include nsfw, got %v", apiTopTagNames)
	}

	apiRevResp := doAPIKeyGet("/api/tag-review")
	var apiRevOut struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(apiRevResp.Body).Decode(&apiRevOut)
	apiRevResp.Body.Close()
	if apiRevOut.Total != 2 {
		t.Fatalf("expected API-key GET /api/tag-review total=2, got %d", apiRevOut.Total)
	}

	// 9. Tampered sv_nsfw cookie stays locked.
	parsedURL, _ := url.Parse(ts.URL)
	client.Jar.SetCookies(parsedURL, []*http.Cookie{{Name: "sv_nsfw", Value: "deadbeef", Path: "/"}})
	tamperedStResp, _ := client.Get(ts.URL + "/api/nsfw/status")
	var tamperedSt struct {
		Locked bool `json:"locked"`
	}
	_ = json.NewDecoder(tamperedStResp.Body).Decode(&tamperedSt)
	tamperedStResp.Body.Close()
	if !tamperedSt.Locked {
		t.Fatalf("expected tampered sv_nsfw cookie to stay locked")
	}

	// 10. Unlock wrong/empty password -> 401 "Password salah".
	for _, body := range []string{`{"password":"wrong-password"}`, `{"password":""}`} {
		uResp, _ := client.Post(ts.URL+"/api/nsfw/unlock", "application/json", strings.NewReader(body))
		var uErr struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(uResp.Body).Decode(&uErr)
		uResp.Body.Close()
		if uResp.StatusCode != http.StatusUnauthorized || uErr.Error != "Password salah" {
			t.Fatalf("expected 401 'Password salah' for %s, got %d (%q)", body, uResp.StatusCode, uErr.Error)
		}
	}

	// 11. Unlock right password -> 200 + sv_nsfw cookie -> session sees all nsfw content.
	okUnlockResp, _ := client.Post(ts.URL+"/api/nsfw/unlock", "application/json", strings.NewReader(`{"password":"`+testNsfwPass+`"}`))
	okUnlockBody, _ := io.ReadAll(okUnlockResp.Body)
	okUnlockResp.Body.Close()
	if okUnlockResp.StatusCode != http.StatusOK || !strings.Contains(string(okUnlockBody), `"unlocked":true`) {
		t.Fatalf("expected 200 unlocked=true, got %d: %s", okUnlockResp.StatusCode, string(okUnlockBody))
	}

	hasCookie := false
	for _, ck := range client.Jar.Cookies(parsedURL) {
		if ck.Name == "sv_nsfw" && ck.Value != "" && ck.Value != "1" {
			hasCookie = true
		}
	}
	if !hasCookie {
		t.Fatalf("expected non-trivial sv_nsfw cookie in jar after unlock")
	}

	// Now list, search, get, tags, stats, and review queue all include nsfw content.
	uListResp, _ := client.Get(ts.URL + "/api/bookmarks")
	var uListOut struct {
		Items []db.Bookmark `json:"items"`
		Total int           `json:"total"`
	}
	_ = json.NewDecoder(uListResp.Body).Decode(&uListOut)
	uListResp.Body.Close()
	if uListOut.Total != 3 || len(uListOut.Items) != 3 {
		t.Fatalf("expected all 3 bookmarks after unlock, got total=%d len=%d", uListOut.Total, len(uListOut.Items))
	}

	uSearchResp, _ := client.Get(ts.URL + "/api/bookmarks?q=unique-nsfw-token-xyz")
	var uSearchOut struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(uSearchResp.Body).Decode(&uSearchOut)
	uSearchResp.Body.Close()
	if uSearchOut.Total != 1 {
		t.Fatalf("expected FTS search to find nsfw bookmark after unlock, got total=%d", uSearchOut.Total)
	}

	uGetResp, _ := client.Get(ts.URL + "/api/bookmarks/" + strconvI64(nsfwBM1.ID))
	uGetResp.Body.Close()
	if uGetResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 getting nsfw bookmark after unlock, got %d", uGetResp.StatusCode)
	}

	uTagsResp, _ := client.Get(ts.URL + "/api/tags")
	var uTagsOut struct {
		Items []db.Tag `json:"items"`
	}
	_ = json.NewDecoder(uTagsResp.Body).Decode(&uTagsOut)
	uTagsResp.Body.Close()
	var tagNames []string
	for _, tg := range uTagsOut.Items {
		tagNames = append(tagNames, strings.ToLower(tg.Name))
	}
	if !slices.Contains(tagNames, "nsfw") {
		t.Fatalf("expected nsfw tag visible after unlock, got %v", tagNames)
	}

	uStatsResp, _ := client.Get(ts.URL + "/api/stats")
	var uStatsOut db.Stats
	_ = json.NewDecoder(uStatsResp.Body).Decode(&uStatsOut)
	uStatsResp.Body.Close()
	var topTagNames []string
	for _, tg := range uStatsOut.TopTags {
		topTagNames = append(topTagNames, strings.ToLower(tg.Name))
	}
	if !slices.Contains(topTagNames, "nsfw") {
		t.Fatalf("expected nsfw in top_tags after unlock, got %v", topTagNames)
	}

	uRevResp, _ := client.Get(ts.URL + "/api/tag-review")
	var uRevOut struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(uRevResp.Body).Decode(&uRevOut)
	uRevResp.Body.Close()
	if uRevOut.Total != 2 {
		t.Fatalf("expected 2 review items after unlock, got %d", uRevOut.Total)
	}

	// Unlocked session can also create a bookmark with nsfw tag.
	createUnlockedResp, _ := client.Post(ts.URL+"/api/bookmarks", "application/json", strings.NewReader(`{"url":"https://example.org/new-spicy-ok","title":"Allowed Now","description":"Safe test","favicon_url":"https://example.org/favicon.ico","tags":["nsfw"]}`))
	createUnlockedResp.Body.Close()
	if createUnlockedResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 creating nsfw bookmark while unlocked, got %d", createUnlockedResp.StatusCode)
	}

	// 12. Lock again -> hidden again.
	lockResp, _ := client.Post(ts.URL+"/api/nsfw/lock", "application/json", nil)
	lockResp.Body.Close()
	if lockResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on lock, got %d", lockResp.StatusCode)
	}

	afterLockResp, _ := client.Get(ts.URL + "/api/bookmarks")
	var afterLockOut struct {
		Total int `json:"total"`
	}
	_ = json.NewDecoder(afterLockResp.Body).Decode(&afterLockOut)
	afterLockResp.Body.Close()
	if afterLockOut.Total != 1 {
		t.Fatalf("expected only 1 safe bookmark after re-locking, got total=%d", afterLockOut.Total)
	}

	// 13. Unlock then logout clears sv_nsfw cookie (MaxAge < 0).
	_, _ = client.Post(ts.URL+"/api/nsfw/unlock", "application/json", strings.NewReader(`{"password":"`+testNsfwPass+`"}`))
	logoutResp, _ := client.Post(ts.URL+"/api/auth/logout", "application/json", nil)
	logoutResp.Body.Close()
	clearedNsfw := false
	for _, ck := range logoutResp.Cookies() {
		if ck.Name == "sv_nsfw" && ck.MaxAge < 0 {
			clearedNsfw = true
		}
	}
	if !clearedNsfw {
		t.Fatalf("expected logout to clear sv_nsfw cookie, got cookies=%+v", logoutResp.Cookies())
	}
}

func TestNsfwImportFiltering(t *testing.T) {
	ts, store := setupNsfwTestServer(t, testNsfwPass)
	client := newLoggedInClient(t, ts.URL)

	karakeep := `{"bookmarks":[
		{"id":"1","archived":false,"createdAt":"2026-01-10T10:00:00Z","content":{"type":"link","url":"https://example.org/import-safe","title":"Safe Import"},"tags":["golang"]},
		{"id":"2","archived":false,"createdAt":"2026-01-11T10:00:00Z","content":{"type":"link","url":"https://example.org/import-nsfw","title":"Spicy Import"},"tags":["NSFW"]}
	]}`

	// Cookie session while locked skips the nsfw entry and counts it as skipped.
	resp := uploadImportWithClient(t, client, ts.URL, "karakeep.json", "auto", karakeep, "")
	if resp["imported"] != float64(1) || resp["skipped"] != float64(1) {
		t.Fatalf("expected locked cookie import to import 1 and skip 1, got %+v", resp)
	}

	// API key import still imports the nsfw entry cleanly.
	apiRes := uploadImportWithClient(t, http.DefaultClient, ts.URL, "karakeep.json", "auto", karakeep, "vault-key")
	if apiRes["imported"] != float64(1) || apiRes["skipped"] != float64(1) {
		t.Fatalf("expected API-key import to import the remaining nsfw item and skip the duplicate safe item, got %+v", apiRes)
	}

	bm, err := store.GetBookmarkByURL(context.Background(), "https://example.org/import-nsfw")
	if err != nil || len(bm.Tags) != 1 || !strings.EqualFold(bm.Tags[0].Name, "nsfw") {
		t.Fatalf("expected API-key import to save nsfw item, got bm=%+v err=%v", bm, err)
	}
}

func uploadImportWithClient(t *testing.T, client *http.Client, baseURL, filename, format, content, apiKey string) map[string]any {
	t.Helper()
	// Build a simple multipart body using uploadImport's format via net/http.
	boundary := "----nsfwtestboundary"
	body := "--" + boundary + "\r\n" +
		`Content-Disposition: form-data; name="format"` + "\r\n\r\n" + format + "\r\n" +
		"--" + boundary + "\r\n" +
		`Content-Disposition: form-data; name="file"; filename="` + filename + `"` + "\r\n" +
		"Content-Type: application/json\r\n\r\n" + content + "\r\n" +
		"--" + boundary + "--\r\n"

	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	if apiKey != "" {
		req.Header.Set("X-Api-Key", apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 on import, got %d: %s", resp.StatusCode, string(b))
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	return out
}
