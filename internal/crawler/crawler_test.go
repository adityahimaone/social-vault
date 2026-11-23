package crawler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"social-vault/internal/crawler"
)

func TestFetcherExtractsMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
<head>
  <title>Fallback Title</title>
  <meta property="og:title" content="OpenGraph   Title" />
  <meta property="og:description" content="Helpful summary for Social Vault." />
  <meta property="og:site_name" content="Example Blog" />
  <link rel="icon" href="/assets/icon.png" />
</head>
<body><p>Hello</p></body>
</html>`))
	}))
	defer srv.Close()

	f := crawler.NewFetcher(srv.Client())
	meta, err := f.Fetch(context.Background(), srv.URL+"/article")
	if err != nil {
		t.Fatalf("fetch metadata: %v", err)
	}
	if meta.Title != "OpenGraph Title" {
		t.Fatalf("expected og:title, got %q", meta.Title)
	}
	if meta.Description != "Helpful summary for Social Vault." {
		t.Fatalf("unexpected description: %q", meta.Description)
	}
	if meta.SiteName != "Example Blog" {
		t.Fatalf("unexpected site name: %q", meta.SiteName)
	}
	if meta.FaviconURL != srv.URL+"/assets/icon.png" {
		t.Fatalf("expected resolved favicon url, got %q", meta.FaviconURL)
	}
}

func TestFetcherTitleFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Plain Page Title</title></head><body></body></html>`))
	}))
	defer srv.Close()

	f := crawler.NewFetcher(srv.Client())
	meta, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch fallback: %v", err)
	}
	if meta.Title != "Plain Page Title" {
		t.Fatalf("expected title fallback, got %q", meta.Title)
	}
	if meta.FaviconURL != srv.URL+"/favicon.ico" {
		t.Fatalf("expected default favicon, got %q", meta.FaviconURL)
	}
}
