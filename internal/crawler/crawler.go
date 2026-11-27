package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	maxBodyBytes            = 2 << 20 // 2 MiB
	userAgent               = "SocialVault/2.0 (+https://github.com/social-vault)"
	defaultFxTwitterBaseURL = "https://api.fxtwitter.com"
	defaultTwitterOEmbedURL = "https://publish.twitter.com/oembed"
	xDefaultFavicon         = "https://abs.twimg.com/favicons/twitter.3.ico"
)

var (
	xStatusRe     = regexp.MustCompile(`^/(?:(i/web)|([A-Za-z0-9_]{1,30}))/status/(\d+)(?:/(?:photo|video)/\d+)?/?$`)
	threadsPostRe = regexp.MustCompile(`^/@?([A-Za-z0-9._]{1,40})/post/([A-Za-z0-9_-]+)(?:/(?:media)?)?/?$`)
)

type Metadata struct {
	Title       string
	Description string
	FaviconURL  string
	ImageURL    string
	SiteName    string
}

type Fetcher struct {
	client           *http.Client
	fxTwitterBaseURL string
	twitterOEmbedURL string
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
	return &Fetcher{
		client:           client,
		fxTwitterBaseURL: defaultFxTwitterBaseURL,
		twitterOEmbedURL: defaultTwitterOEmbedURL,
	}
}

// WithSocialEndpoints overrides the FxTwitter and oEmbed endpoints (used in tests).
func (f *Fetcher) WithSocialEndpoints(fxBase, oembedURL string) *Fetcher {
	clone := *f
	if strings.TrimSpace(fxBase) != "" {
		clone.fxTwitterBaseURL = strings.TrimRight(strings.TrimSpace(fxBase), "/")
	}
	if strings.TrimSpace(oembedURL) != "" {
		clone.twitterOEmbedURL = strings.TrimSpace(oembedURL)
	}
	return &clone
}

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (Metadata, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Metadata{}, fmt.Errorf("invalid http url: %s", rawURL)
	}

	meta, finalURL, fetchErr := f.fetchHTML(ctx, parsed)

	if handle, statusID, ok := parseXStatus(parsed); ok {
		if fetchErr != nil || meta.Description == "" || isGenericSocialTitle(meta.Title) {
			if fxMeta, fxErr := f.fetchFxTwitter(ctx, handle, statusID); fxErr == nil {
				mergeMetadata(&meta, fxMeta)
			}
			if meta.Description == "" || isGenericSocialTitle(meta.Title) {
				if oeMeta, oeErr := f.fetchTwitterOEmbed(ctx, parsed.String()); oeErr == nil {
					mergeMetadata(&meta, oeMeta)
				}
			}
			if meta.Description != "" || !isGenericSocialTitle(meta.Title) {
				return meta, nil
			}
		}
	}

	if handle, _, ok := parseThreadsPost(parsed); ok {
		if fetchErr == nil && isThreadsLoginWall(finalURL, meta) {
			meta.Description = ""
			if strings.Contains(meta.ImageURL, "cdninstagram.com/rsrc.php/") {
				meta.ImageURL = ""
			}
			meta.SiteName = "Threads"
			if handle != "" {
				meta.Title = "@" + handle + " on Threads"
			} else {
				meta.Title = ""
			}
		}
	}

	return meta, fetchErr
}

func (f *Fetcher) fetchHTML(ctx context.Context, parsed *url.URL) (Metadata, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return Metadata{}, parsed, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := f.client.Do(req)
	if err != nil {
		return Metadata{}, parsed, err
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
		return meta, finalURL, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "html") && !strings.Contains(contentType, "xml") {
		return meta, finalURL, nil
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
	if parsedMeta.ImageURL != "" {
		meta.ImageURL = parsedMeta.ImageURL
	}

	return meta, finalURL, nil
}

func (f *Fetcher) fetchFxTwitter(ctx context.Context, handle, statusID string) (Metadata, error) {
	base := strings.TrimRight(f.fxTwitterBaseURL, "/")
	if base == "" {
		return Metadata{}, errors.New("fxtwitter endpoint disabled")
	}
	if handle == "" {
		handle = "i"
	}
	endpoint := fmt.Sprintf("%s/%s/status/%s", base, url.PathEscape(handle), url.PathEscape(statusID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Metadata{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Metadata{}, fmt.Errorf("fxtwitter status %d", resp.StatusCode)
	}

	var decoded struct {
		Code  int `json:"code"`
		Tweet struct {
			Text   string `json:"text"`
			Author struct {
				Name       string `json:"name"`
				ScreenName string `json:"screen_name"`
				AvatarURL  string `json:"avatar_url"`
			} `json:"author"`
			Media struct {
				Photos []struct {
					URL string `json:"url"`
				} `json:"photos"`
			} `json:"media"`
		} `json:"tweet"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&decoded); err != nil {
		return Metadata{}, err
	}

	text := normalizeSpace(decoded.Tweet.Text)
	name := normalizeSpace(decoded.Tweet.Author.Name)
	screen := strings.TrimSpace(decoded.Tweet.Author.ScreenName)
	if text == "" && screen == "" {
		return Metadata{}, errors.New("fxtwitter returned empty tweet")
	}

	var title string
	switch {
	case name != "" && screen != "":
		title = fmt.Sprintf("%s (@%s) on X", name, screen)
	case screen != "":
		title = fmt.Sprintf("@%s on X", screen)
	case name != "":
		title = fmt.Sprintf("%s on X", name)
	}

	var imageURL string
	if len(decoded.Tweet.Media.Photos) > 0 {
		imageURL = strings.TrimSpace(decoded.Tweet.Media.Photos[0].URL)
	}
	if imageURL == "" {
		imageURL = strings.TrimSpace(decoded.Tweet.Author.AvatarURL)
	}

	return Metadata{
		Title:       title,
		Description: text,
		ImageURL:    imageURL,
		FaviconURL:  xDefaultFavicon,
		SiteName:    "X",
	}, nil
}

func (f *Fetcher) fetchTwitterOEmbed(ctx context.Context, rawPostURL string) (Metadata, error) {
	if strings.TrimSpace(f.twitterOEmbedURL) == "" {
		return Metadata{}, errors.New("oembed endpoint disabled")
	}
	endpoint := f.twitterOEmbedURL + "?omit_script=true&url=" + url.QueryEscape(rawPostURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Metadata{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return Metadata{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Metadata{}, fmt.Errorf("oembed status %d", resp.StatusCode)
	}

	var decoded struct {
		AuthorName string `json:"author_name"`
		HTML       string `json:"html"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&decoded); err != nil {
		return Metadata{}, err
	}

	author := normalizeSpace(decoded.AuthorName)
	desc := extractOEmbedText(decoded.HTML)
	if author == "" && desc == "" {
		return Metadata{}, errors.New("empty oembed response")
	}

	title := ""
	if author != "" {
		title = author + " on X"
	}

	return Metadata{
		Title:       title,
		Description: desc,
		FaviconURL:  xDefaultFavicon,
		SiteName:    "X",
	}, nil
}

func extractOEmbedText(rawHTML string) string {
	if strings.TrimSpace(rawHTML) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return ""
	}
	var pText strings.Builder
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inP bool) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "p") {
			inP = true
		}
		if n.Type == html.TextNode && inP {
			pText.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inP)
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "p") {
			pText.WriteByte(' ')
		}
	}
	walk(doc, false)
	return normalizeSpace(pText.String())
}

func mergeMetadata(dst *Metadata, src Metadata) {
	if src.Title != "" && (dst.Title == "" || isGenericSocialTitle(dst.Title)) {
		dst.Title = src.Title
	}
	if src.Description != "" && dst.Description == "" {
		dst.Description = src.Description
	}
	if src.ImageURL != "" && dst.ImageURL == "" {
		dst.ImageURL = src.ImageURL
	}
	if src.FaviconURL != "" && (dst.FaviconURL == "" || strings.HasSuffix(dst.FaviconURL, "/favicon.ico")) {
		dst.FaviconURL = src.FaviconURL
	}
	if src.SiteName != "" && (dst.SiteName == "" || dst.SiteName == "x.com" || dst.SiteName == "twitter.com") {
		dst.SiteName = src.SiteName
	}
}

func parseXStatus(u *url.URL) (handle string, statusID string, ok bool) {
	if u == nil {
		return "", "", false
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "x.com", "www.x.com", "mobile.x.com", "twitter.com", "www.twitter.com", "mobile.twitter.com":
	default:
		return "", "", false
	}
	m := xStatusRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	return m[2], m[3], true
}

func parseThreadsPost(u *url.URL) (handle string, postID string, ok bool) {
	if u == nil {
		return "", "", false
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "threads.com", "www.threads.com", "threads.net", "www.threads.net":
	default:
		return "", "", false
	}
	m := threadsPostRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	lower := strings.ToLower(m[1])
	switch lower {
	case "login", "search", "activity", "settings", "api", "t", "post", "privacy", "terms", "saved", "liked", "likes":
		return "", "", false
	}
	return m[1], m[2], true
}

func isThreadsLoginWall(finalURL *url.URL, meta Metadata) bool {
	if finalURL != nil && strings.Contains(finalURL.RawQuery, "error=invalid_post") {
		return true
	}
	lowerTitle := strings.ToLower(strings.TrimSpace(meta.Title))
	if lowerTitle == "threads" || strings.Contains(lowerTitle, "log in") {
		return true
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(meta.Description)), "join threads to share ideas") {
		return true
	}
	return false
}

func isGenericSocialTitle(title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	switch t {
	case "", "x", "twitter", "threads", "threads • log in", "home / x", "log in to x", "javascript is not available.":
		return true
	default:
		return false
	}
}

func parseHTML(r io.Reader, baseURL *url.URL) Metadata {
	var (
		meta          Metadata
		titleTag      strings.Builder
		inTitle       bool
		ogTitle       string
		twTitle       string
		ogDesc        string
		metaDesc      string
		twDesc        string
		ogImage       string
		ogImageSecure string
		twImage       string
		iconHref      string
		appleIcon     string
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
				case (prop == "og:image" || mname == "og:image") && ogImage == "":
					ogImage = content
				case (prop == "og:image:secure_url" || mname == "og:image:secure_url") && ogImageSecure == "":
					ogImageSecure = content
				case (prop == "twitter:image" || mname == "twitter:image" || prop == "twitter:image:src" || mname == "twitter:image:src") && twImage == "":
					twImage = content
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

	chosenImage := firstNonEmpty(ogImage, ogImageSecure, twImage)
	if chosenImage != "" {
		if resolved := resolveURL(baseURL, chosenImage); resolved != "" {
			meta.ImageURL = resolved
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
