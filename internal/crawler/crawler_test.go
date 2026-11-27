package crawler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
  <meta property="og:image" content="/images/cover.jpg" />
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
	if meta.ImageURL != srv.URL+"/images/cover.jpg" {
		t.Fatalf("expected resolved og:image url, got %q", meta.ImageURL)
	}
}

func TestFetcherTwitterImageFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Tweet</title><meta name="twitter:image" content="https://pbs.twimg.com/media/demo.jpg" /></head><body></body></html>`))
	}))
	defer srv.Close()

	f := crawler.NewFetcher(srv.Client())
	meta, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch twitter image: %v", err)
	}
	if meta.ImageURL != "https://pbs.twimg.com/media/demo.jpg" {
		t.Fatalf("expected twitter:image url, got %q", meta.ImageURL)
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

func TestFetcherXFxTwitterAndOEmbedFallback(t *testing.T) {
	xStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>X</title></head><body>JavaScript is not available.</body></html>`))
	}))
	defer xStub.Close()

	fxStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"tweet": {
				"text": "just setting up my twttr",
				"author": {
					"name": "jack",
					"screen_name": "jack",
					"avatar_url": "https://pbs.twimg.com/avatar.jpg"
				}
			}
		}`))
	}))
	defer fxStub.Close()

	oembedStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"author_name": "jack",
			"html": "<blockquote><p>oembed fallback tweet text</p></blockquote>"
		}`))
	}))
	defer oembedStub.Close()

	// Point Fetcher at fxStub via custom transport or directly test URL rewrite using host?
	// parseXStatus checks host == x.com; test with unreachable direct HTML? No, use Transport redirect or test oEmbed when FxTwitter fails.
	// Let's test via a RoundTripper mapping x.com -> xStub.
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			if strings.EqualFold(req.URL.Hostname(), "x.com") {
				target, _ := http.NewRequestWithContext(req.Context(), req.Method, xStub.URL+req.URL.RequestURI(), req.Body)
				return http.DefaultTransport.RoundTrip(target)
			}
			return http.DefaultTransport.RoundTrip(clone)
		}),
	}

	f := crawler.NewFetcher(client).WithSocialEndpoints(fxStub.URL, oembedStub.URL)
	meta, err := f.Fetch(context.Background(), "https://x.com/jack/status/20")
	if err != nil {
		t.Fatalf("fetch X with fxtwitter fallback: %v", err)
	}
	if meta.Title != "jack (@jack) on X" || meta.Description != "just setting up my twttr" {
		t.Fatalf("unexpected fxtwitter fallback metadata: %+v", meta)
	}

	// If FxTwitter returns 500, oEmbed takes over
	fxFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fxFail.Close()

	f2 := crawler.NewFetcher(client).WithSocialEndpoints(fxFail.URL, oembedStub.URL)
	meta2, err := f2.Fetch(context.Background(), "https://x.com/jack/status/20")
	if err != nil {
		t.Fatalf("fetch X with oembed fallback: %v", err)
	}
	if meta2.Title != "jack on X" || meta2.Description != "oembed fallback tweet text" {
		t.Fatalf("unexpected oembed fallback metadata: %+v", meta2)
	}
}

func TestFetcherThreadsLoginWallSanitized(t *testing.T) {
	threadsStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head>
			<title>Threads</title>
			<meta property="og:title" content="Threads &#x2022; Log in" />
			<meta property="og:description" content="Join Threads to share ideas, ask questions, post random thoughts, find your people and more. Log in with your Instagram." />
			<meta property="og:image" content="https://static.cdninstagram.com/rsrc.php/yd/r/kHwIMM5b8PW.webp" />
		</head><body></body></html>`))
	}))
	defer threadsStub.Close()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.EqualFold(req.URL.Hostname(), "www.threads.com") {
				target, _ := http.NewRequestWithContext(req.Context(), req.Method, threadsStub.URL+req.URL.RequestURI(), req.Body)
				return http.DefaultTransport.RoundTrip(target)
			}
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	f := crawler.NewFetcher(client)
	meta, err := f.Fetch(context.Background(), "https://www.threads.com/@alice/post/Abc123")
	if err != nil {
		t.Fatalf("fetch threads login wall: %v", err)
	}
	if meta.Title != "@alice on Threads" || meta.Description != "" || meta.ImageURL != "" {
		t.Fatalf("expected sanitized threads fallback, got %+v", meta)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}
