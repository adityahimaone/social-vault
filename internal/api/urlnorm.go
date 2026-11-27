package api

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	xStatusPathRe       = regexp.MustCompile(`^/(?:(i/web)|([A-Za-z0-9_]{1,30}))/status/(\d+)(?:/(?:photo|video)/\d+)?/?$`)
	threadsUserPostRe   = regexp.MustCompile(`^/@?([A-Za-z0-9._]{1,40})/post/([A-Za-z0-9_-]+)(?:/(?:media)?)?/?$`)
	threadsBarePostRe   = regexp.MustCompile(`^/(t|post)/([A-Za-z0-9_-]+)/?$`)
	trackingQueryParams = map[string]bool{
		"fbclid":  true,
		"gclid":   true,
		"igsh":    true,
		"ref_src": true,
		"ref_url": true,
		"xmt":     true,
	}
)

var xHostSet = map[string]bool{
	"x.com":              true,
	"www.x.com":          true,
	"mobile.x.com":       true,
	"twitter.com":        true,
	"www.twitter.com":    true,
	"mobile.twitter.com": true,
}

var threadsHostSet = map[string]bool{
	"threads.com":     true,
	"www.threads.com": true,
	"threads.net":     true,
	"www.threads.net": true,
}

func canonicalizeHTTPURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("url is required")
	}
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("url must be a valid http or https URL")
	}

	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	u.Fragment = ""

	if xHostSet[host] {
		if m := xStatusPathRe.FindStringSubmatch(u.Path); m != nil {
			u.Scheme = "https"
			u.Host = "x.com"
			u.RawQuery = ""
			if m[1] != "" {
				u.Path = "/i/web/status/" + m[3]
			} else {
				u.Path = "/" + m[2] + "/status/" + m[3]
			}
			return u.String(), nil
		}
	}

	if threadsHostSet[host] {
		if m := threadsUserPostRe.FindStringSubmatch(u.Path); m != nil {
			user := strings.ToLower(m[1])
			if !isThreadsReservedSegment(user) {
				u.Scheme = "https"
				u.Host = "www.threads.com"
				u.RawQuery = ""
				u.Path = "/@" + m[1] + "/post/" + m[2]
				return u.String(), nil
			}
		}
		if m := threadsBarePostRe.FindStringSubmatch(u.Path); m != nil {
			u.Scheme = "https"
			u.Host = "www.threads.com"
			u.RawQuery = ""
			u.Path = "/" + m[1] + "/" + m[2]
			return u.String(), nil
		}
	}

	if port != "" {
		u.Host = host + ":" + port
	} else {
		u.Host = host
	}

	if u.RawQuery != "" {
		q := u.Query()
		changed := false
		for key := range q {
			lower := strings.ToLower(key)
			if strings.HasPrefix(lower, "utm_") || trackingQueryParams[lower] {
				q.Del(key)
				changed = true
			}
		}
		if changed {
			u.RawQuery = q.Encode()
		}
	}

	return u.String(), nil
}

func alternateDuplicateURLs(canonical string) []string {
	u, err := url.Parse(canonical)
	if err != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	var candidates []string
	add := func(s string) {
		if s != "" && s != canonical {
			candidates = append(candidates, s)
		}
	}

	if host == "x.com" {
		for _, altHost := range []string{"twitter.com", "www.x.com", "www.twitter.com", "mobile.x.com", "mobile.twitter.com"} {
			add("https://" + altHost + u.Path)
			add("https://" + altHost + u.Path + "/")
		}
		add(canonical + "/")
	} else if host == "www.threads.com" {
		paths := []string{u.Path}
		if strings.HasPrefix(u.Path, "/@") {
			paths = append(paths, "/"+strings.TrimPrefix(u.Path, "/@"))
		}
		for _, altHost := range []string{"www.threads.net", "threads.com", "threads.net"} {
			for _, p := range paths {
				add("https://" + altHost + p)
				add("https://" + altHost + p + "/")
			}
		}
		for _, p := range paths {
			add("https://www.threads.com" + p)
			add("https://www.threads.com" + p + "/")
		}
	} else {
		if strings.HasSuffix(u.Path, "/") && u.Path != "/" && u.RawQuery == "" {
			trimmed := *u
			trimmed.Path = strings.TrimRight(u.Path, "/")
			add(trimmed.String())
		} else if !strings.HasSuffix(u.Path, "/") && u.RawQuery == "" {
			add(canonical + "/")
		}
	}

	return candidates
}

func isThreadsReservedSegment(seg string) bool {
	switch seg {
	case "login", "search", "activity", "settings", "api", "t", "post", "privacy", "terms", "saved", "liked", "likes":
		return true
	default:
		return false
	}
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
