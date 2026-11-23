package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"social-vault/internal/api"
	"social-vault/internal/auth"
	"social-vault/internal/crawler"
	"social-vault/internal/db"
	"social-vault/internal/mcp"
)

func setupVaultAndMCP(t *testing.T, apiKey string) (*sdkmcp.ClientSession, *db.Store, string) {
	t.Helper()

	store, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	authenticator := auth.New(store, "test-pass", "valid-api-key")
	fetcher := crawler.NewFetcher(nil)
	webFS := fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}
	apiSrv := api.NewServer(store, authenticator, fetcher, webFS)

	httpSrv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(httpSrv.Close)

	targetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head>
			<title>Example Crawled Article</title>
			<meta name="description" content="Helpful notes on Go MCP servers" />
		</head><body>ok</body></html>`))
	}))
	t.Cleanup(targetSrv.Close)

	client, err := mcp.NewClient(mcp.Config{
		BaseURL: httpSrv.URL,
		APIKey:  apiKey,
	})
	if err != nil {
		t.Fatalf("new mcp client: %v", err)
	}

	mcpSrv := mcp.NewServer(client)
	mcpClient := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)

	ctx := context.Background()
	t1, t2 := sdkmcp.NewInMemoryTransports()
	ss, err := mcpSrv.Connect(ctx, t1, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	cs, err := mcpClient.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	return cs, store, targetSrv.URL
}

func TestLoadConfigFromEnv(t *testing.T) {
	if _, err := mcp.LoadConfigFromEnv(func(string) string { return "" }); err == nil {
		t.Fatal("expected error when VAULT_API_KEY is missing")
	}

	cfg, err := mcp.LoadConfigFromEnv(func(k string) string {
		if k == "VAULT_API_KEY" {
			return "dummy-key"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != mcp.DefaultVaultURL {
		t.Fatalf("expected default URL %q, got %q", mcp.DefaultVaultURL, cfg.BaseURL)
	}

	if _, err := mcp.LoadConfigFromEnv(func(k string) string {
		switch k {
		case "VAULT_API_KEY":
			return "dummy-key"
		case "VAULT_URL":
			return "ftp://invalid.local"
		default:
			return ""
		}
	}); err == nil {
		t.Fatal("expected error for non-http VAULT_URL")
	}
}

func TestMCPToolsEndToEnd(t *testing.T) {
	cs, store, targetURL := setupVaultAndMCP(t, "valid-api-key")
	ctx := context.Background()

	if _, err := store.CreateList(ctx, "Reading Queue", "To read later"); err != nil {
		t.Fatalf("create list: %v", err)
	}

	// 1. Verify tools/list
	listRes, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expectedTools := []string{
		"search_bookmarks",
		"add_bookmark",
		"list_bookmarks",
		"get_bookmark",
		"list_tags",
		"list_lists",
		"get_stats",
		"update_bookmark",
	}
	gotNames := make([]string, 0, len(listRes.Tools))
	toolMap := make(map[string]*sdkmcp.Tool, len(listRes.Tools))
	for _, tl := range listRes.Tools {
		gotNames = append(gotNames, tl.Name)
		toolMap[tl.Name] = tl
		if strings.TrimSpace(tl.Description) == "" {
			t.Fatalf("tool %q missing description", tl.Name)
		}
		if tl.InputSchema == nil {
			t.Fatalf("tool %q missing inputSchema", tl.Name)
		}
	}
	for _, name := range expectedTools {
		if !slices.Contains(gotNames, name) {
			t.Fatalf("missing expected tool %q in %v", name, gotNames)
		}
	}

	// 2. add_bookmark via tools/call
	addRes, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "add_bookmark",
		Arguments: map[string]any{
			"url":   targetURL + "/article",
			"notes": "Read before Friday",
			"tags":  []string{"golang", "mcp"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool add_bookmark: %v", err)
	}
	if addRes.IsError {
		t.Fatalf("add_bookmark returned tool error: %+v", addRes.Content)
	}

	var added mcp.BookmarkOutput
	decodeToolText(t, addRes, &added)
	if added.Bookmark.ID <= 0 {
		t.Fatalf("expected positive bookmark ID, got %+v", added.Bookmark)
	}
	if added.Bookmark.Title != "Example Crawled Article" {
		t.Fatalf("expected crawled title, got %q", added.Bookmark.Title)
	}
	if !slices.Contains(added.Bookmark.Tags, "golang") || !slices.Contains(added.Bookmark.Tags, "mcp") {
		t.Fatalf("expected tags [golang mcp], got %v", added.Bookmark.Tags)
	}

	// Duplicate add returns IsError with human-readable conflict message
	dupRes, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "add_bookmark",
		Arguments: map[string]any{
			"url": targetURL + "/article",
		},
	})
	if err != nil {
		t.Fatalf("CallTool duplicate add_bookmark: %v", err)
	}
	if !dupRes.IsError {
		t.Fatal("expected IsError=true on duplicate bookmark")
	}

	// 3. search_bookmarks via tools/call
	searchRes, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "search_bookmarks",
		Arguments: map[string]any{
			"query": "Crawled",
			"tag":   "golang",
		},
	})
	if err != nil {
		t.Fatalf("CallTool search_bookmarks: %v", err)
	}
	if searchRes.IsError {
		t.Fatalf("search_bookmarks returned tool error: %+v", searchRes.Content)
	}
	var searchOut mcp.BookmarkListOutput
	decodeToolText(t, searchRes, &searchOut)
	if searchOut.Total != 1 || len(searchOut.Items) != 1 || searchOut.Items[0].ID != added.Bookmark.ID {
		t.Fatalf("unexpected search output: %+v", searchOut)
	}

	// 4. list_bookmarks, get_bookmark, update_bookmark
	listBookmarksRes, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "list_bookmarks",
		Arguments: map[string]any{"limit": 10},
	})
	if err != nil || listBookmarksRes.IsError {
		t.Fatalf("list_bookmarks failed: err=%v res=%+v", err, listBookmarksRes)
	}

	updateRes, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "update_bookmark",
		Arguments: map[string]any{
			"id":       added.Bookmark.ID,
			"archived": true,
			"notes":    "Updated note",
		},
	})
	if err != nil || updateRes.IsError {
		t.Fatalf("update_bookmark failed: err=%v res=%+v", err, updateRes)
	}
	var updated mcp.BookmarkOutput
	decodeToolText(t, updateRes, &updated)
	if !updated.Bookmark.Archived || updated.Bookmark.Notes != "Updated note" {
		t.Fatalf("unexpected update result: %+v", updated.Bookmark)
	}

	getRes, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "get_bookmark",
		Arguments: map[string]any{"id": added.Bookmark.ID},
	})
	if err != nil || getRes.IsError {
		t.Fatalf("get_bookmark failed: err=%v res=%+v", err, getRes)
	}

	missingRes, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "get_bookmark",
		Arguments: map[string]any{"id": 99999},
	})
	if err != nil || !missingRes.IsError {
		t.Fatalf("expected IsError=true for missing bookmark, err=%v res=%+v", err, missingRes)
	}

	// 5. list_tags, list_lists, get_stats
	tagsRes, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "list_tags", Arguments: map[string]any{}})
	if err != nil || tagsRes.IsError {
		t.Fatalf("list_tags failed: err=%v res=%+v", err, tagsRes)
	}
	var tagsOut mcp.TagListOutput
	decodeToolText(t, tagsRes, &tagsOut)
	if tagsOut.Total != 2 {
		t.Fatalf("expected 2 tags, got %+v", tagsOut)
	}

	listsRes, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "list_lists", Arguments: map[string]any{}})
	if err != nil || listsRes.IsError {
		t.Fatalf("list_lists failed: err=%v res=%+v", err, listsRes)
	}
	var listsOut mcp.ListCollectionsOutput
	decodeToolText(t, listsRes, &listsOut)
	if listsOut.Total != 1 || listsOut.Items[0].Name != "Reading Queue" {
		t.Fatalf("unexpected lists output: %+v", listsOut)
	}

	statsRes, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{Name: "get_stats", Arguments: map[string]any{}})
	if err != nil || statsRes.IsError {
		t.Fatalf("get_stats failed: err=%v res=%+v", err, statsRes)
	}
	var statsOut mcp.CompactStats
	decodeToolText(t, statsRes, &statsOut)
	if statsOut.Total != 1 || statsOut.Archived != 1 || statsOut.TagsCount != 2 || statsOut.ListsCount != 1 {
		t.Fatalf("unexpected stats: %+v", statsOut)
	}
}

func TestMCPUnauthorizedAPIKey(t *testing.T) {
	cs, _, _ := setupVaultAndMCP(t, "wrong-api-key")
	res, err := cs.CallTool(context.Background(), &sdkmcp.CallToolParams{
		Name:      "get_stats",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool failed at protocol layer: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true for invalid API key")
	}
	text := res.Content[0].(*sdkmcp.TextContent).Text
	if !strings.Contains(text, "401") {
		t.Fatalf("expected 401 message, got %q", text)
	}
}

func decodeToolText(t *testing.T, res *sdkmcp.CallToolResult, dst any) {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("tool result had empty Content")
	}
	tc, ok := res.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	if err := json.Unmarshal([]byte(tc.Text), dst); err != nil {
		t.Fatalf("unmarshal tool text %q: %v", tc.Text, err)
	}
}
