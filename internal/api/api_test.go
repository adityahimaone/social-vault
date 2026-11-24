package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"social-vault/internal/api"
	"social-vault/internal/auth"
	"social-vault/internal/crawler"
	"social-vault/internal/db"
)

func setupTestServer(t *testing.T) (*httptest.Server, *db.Store) {
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
	srv := api.NewServer(store, authenticator, fetcher, webFS, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, store
}

func TestEndToEndAPI(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Target HTML server to test crawling without external network dependency
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head>
			<title>Local Crawled Page</title>
			<meta name="description" content="Crawled description text" />
			<meta property="og:image" content="/og-cover.png" />
			<link rel="icon" href="/favicon.svg" />
		</head><body>ok</body></html>`))
	}))
	defer target.Close()

	// 1. Health -> 200
	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on health, got %d", resp.StatusCode)
	}

	// 2. Unauthenticated bookmarks -> 401
	resp, err = http.Get(ts.URL + "/api/bookmarks")
	if err != nil {
		t.Fatalf("unauth bookmarks: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthenticated, got %d", resp.StatusCode)
	}

	// 3. Login wrong password -> 401; right password -> cookie session works
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	resp, err = client.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(`{"password":"wrong"}`))
	if err != nil {
		t.Fatalf("bad login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", resp.StatusCode)
	}

	resp, err = client.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(`{"password":"vault-pass"}`))
	if err != nil {
		t.Fatalf("good login: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for valid login, got %d", resp.StatusCode)
	}

	// 4. Create bookmark via API key -> 201 + crawled metadata
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/bookmarks", strings.NewReader(`{"url":"`+target.URL+`/post","tags":["threads"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer vault-key")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create bookmark: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var created db.Bookmark
	if err := json.Unmarshal(bodyBytes, &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.Title != "Local Crawled Page" || created.Description != "Crawled description text" {
		t.Fatalf("unexpected crawled metadata: %+v", created)
	}
	if created.ImageURL != target.URL+"/og-cover.png" {
		t.Fatalf("expected crawled og:image %q, got %q", target.URL+"/og-cover.png", created.ImageURL)
	}

	// 5. Search via session cookie -> finds created bookmark
	resp, err = client.Get(ts.URL + "/api/bookmarks?q=Crawled")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var listRes struct {
		Items []db.Bookmark `json:"items"`
		Total int           `json:"total"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&listRes)
	resp.Body.Close()
	if listRes.Total != 1 || len(listRes.Items) != 1 {
		t.Fatalf("expected 1 search result, got %+v", listRes)
	}

	// 6. PATCH bookmark
	patchReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/bookmarks/"+strconvI64(created.ID), strings.NewReader(`{"notes":"saved note","archived":true}`))
	patchReq.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(patchReq)
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	var patched db.Bookmark
	_ = json.NewDecoder(resp.Body).Decode(&patched)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !patched.Archived || patched.Notes != "saved note" {
		t.Fatalf("unexpected patch result: status=%d body=%+v", resp.StatusCode, patched)
	}

	// 7. DELETE bookmark -> 200, subsequent GET -> 404
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/bookmarks/"+strconvI64(created.ID), nil)
	resp, err = client.Do(delReq)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d", resp.StatusCode)
	}

	resp, err = client.Get(ts.URL + "/api/bookmarks/" + strconvI64(created.ID))
	if err != nil {
		t.Fatalf("get deleted: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestImportNetscapeAndKarakeep(t *testing.T) {
	ts, _ := setupTestServer(t)

	netscape := `<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
  <DT><H3>X Harvest</H3>
  <DL><p>
    <DT><A HREF="https://x.com/user/status/1" ADD_DATE="1700000000" TAGS="go,sqlite">Thread about SQLite</A>
    <DD>Great benchmark notes
  </DL><p>
</DL><p>`

	res := uploadImport(t, ts.URL, "bookmarks.html", "auto", netscape)
	if res["format"] != "netscape" || res["imported"] != float64(1) {
		t.Fatalf("unexpected netscape import result: %+v", res)
	}

	karakeep := `{"bookmarks":[
		{"id":"1","archived":false,"createdAt":"2026-01-10T10:00:00Z","note":"Duplicate URL test","content":{"type":"link","url":"https://x.com/user/status/1","title":"Duplicate"},"tags":["go"]},
		{"id":"2","archived":true,"createdAt":"2026-01-11T10:00:00Z","note":"Threads post","content":{"type":"link","url":"https://www.threads.net/@demo/post/2","title":"Threads Note","description":"Saved thread"},"tags":[{"name":"threads"}]}
	]}`

	res = uploadImport(t, ts.URL, "karakeep.json", "auto", karakeep)
	if res["format"] != "karakeep" || res["imported"] != float64(1) || res["skipped"] != float64(1) {
		t.Fatalf("unexpected karakeep import result: %+v", res)
	}
}

func uploadImport(t *testing.T, baseURL, filename, format, content string) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("format", format)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = fw.Write([]byte(content))
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Api-Key", "vault-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("import request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("import status %d: %s", resp.StatusCode, string(b))
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	return out
}

func strconvI64(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
