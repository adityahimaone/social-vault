package crawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	maxBodyBytes = 2 << 20 // 2 MiB
	userAgent    = "SocialVault/2.0 (+https://github.com/social-vault)"
)

type Metadata struct {
	Title       string
	Description string
	FaviconURL  string
	SiteName    string
}

type Fetcher struct {
	client *http.Client
}

func NewFetcher(client *http.Client) *Fetcher {
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				return nil
			},
		}
	}
	return &Fetcher{client: client}
}

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (Metadata, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Metadata{}, fmt.Errorf("invalid http url: %s", rawURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Metadata{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL
	if finalURL == nil {
		finalURL = parsed
	}

	meta := Metadata{
		SiteName:   cleanHost(finalURL.Hostname()),
		FaviconURL: defaultFavicon(finalURL),
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return meta, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "html") && !strings.Contains(contentType, "xml") {
		return meta, nil
	}

	limited := io.LimitReader(resp.Body, maxBodyBytes)
	parsedMeta := parseHTML(limited, finalURL)
	if parsedMeta.Title != "" {
		meta.Title = parsedMeta.Title
	}
	if parsedMeta.Description != "" {
		meta.Description = parsedMeta.Description
	}
	if parsedMeta.SiteName != "" {
		meta.SiteName = parsedMeta.SiteName
	}
	if parsedMeta.FaviconURL != "" {
		meta.FaviconURL = parsedMeta.FaviconURL
	}

	return meta, nil
}

func parseHTML(r io.Reader, baseURL *url.URL) Metadata {
	var (
		meta      Metadata
		titleTag  strings.Builder
		inTitle   bool
		ogTitle   string
		twTitle   string
		ogDesc    string
		metaDesc  string
		twDesc    string
		iconHref  string
		appleIcon string
	)

	z := html.NewTokenizer(r)
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			goto done
		case html.StartTagToken, html.SelfClosingTagToken:
			t := z.Token()
			name := strings.ToLower(t.Data)
			switch name {
			case "title":
				if tt == html.StartTagToken {
					inTitle = true
				}
			case "meta":
				prop := strings.ToLower(attrVal(t.Attr, "property"))
				mname := strings.ToLower(attrVal(t.Attr, "name"))
				content := normalizeSpace(attrVal(t.Attr, "content"))
				if content == "" {
					continue
				}
				switch {
				case prop == "og:title" && ogTitle == "":
					ogTitle = content
				case mname == "twitter:title" && twTitle == "":
					twTitle = content
				case prop == "og:description" && ogDesc == "":
					ogDesc = content
				case mname == "description" && metaDesc == "":
					metaDesc = content
				case mname == "twitter:description" && twDesc == "":
					twDesc = content
				case prop == "og:site_name" && meta.SiteName == "":
					meta.SiteName = content
				}
			case "link":
				rel := strings.ToLower(attrVal(t.Attr, "rel"))
				href := strings.TrimSpace(attrVal(t.Attr, "href"))
				if href == "" {
					continue
				}
				if hasRelToken(rel, "icon") && iconHref == "" {
					iconHref = href
				} else if hasRelToken(rel, "apple-touch-icon") && appleIcon == "" {
					appleIcon = href
				}
			}
		case html.TextToken:
			if inTitle {
				titleTag.WriteString(z.Token().Data)
			}
		case html.EndTagToken:
			if strings.EqualFold(z.Token().Data, "title") {
				inTitle = false
			} else if strings.EqualFold(z.Token().Data, "head") {
				goto done
			}
		}
	}

done:
	meta.Title = firstNonEmpty(ogTitle, twTitle, normalizeSpace(titleTag.String()))
	meta.Description = firstNonEmpty(ogDesc, metaDesc, twDesc)

	chosenIcon := firstNonEmpty(iconHref, appleIcon)
	if chosenIcon != "" {
		if resolved := resolveURL(baseURL, chosenIcon); resolved != "" {
			meta.FaviconURL = resolved
		}
	}
	return meta
}

func attrVal(attrs []html.Attribute, key string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func hasRelToken(rel, target string) bool {
	for _, tok := range strings.Fields(rel) {
		if tok == target {
			return true
		}
	}
	return false
}

func resolveURL(base *url.URL, ref string) string {
	if strings.HasPrefix(ref, "data:") {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.String()
}

func defaultFavicon(u *url.URL) string {
	if u == nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/favicon.ico"
}

func cleanHost(host string) string {
	return strings.TrimPrefix(strings.ToLower(host), "www.")
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
