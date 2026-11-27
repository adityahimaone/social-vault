package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"social-vault/internal/db"
)

const maxImportBytes = 25 << 20 // 25 MiB

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse multipart upload")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read uploaded file")
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.FormValue("format")))
	if format == "" || format == "auto" {
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			format = "karakeep"
		} else {
			format = "netscape"
		}
	}

	var entries []db.CreateBookmarkInput
	switch format {
	case "karakeep":
		entries, err = parseKarakeepJSON(data)
	case "netscape":
		entries, err = parseNetscapeHTML(data)
	default:
		writeError(w, http.StatusBadRequest, "unsupported import format (use auto, netscape, or karakeep)")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	nsfwUnlocked := s.auth.IsNsfwUnlocked(r)

	var imported, skipped, errCount int
	for _, entry := range entries {
		cleanURL, verr := validateHTTPURL(entry.URL)
		if verr != nil {
			errCount++
			continue
		}
		if !nsfwUnlocked && hasNsfwTagName(entry.Tags) {
			skipped++
			continue
		}
		if _, err := s.findExistingBookmark(r.Context(), cleanURL); err == nil {
			skipped++
			continue
		}
		entry.URL = cleanURL
		if entry.Title == "" {
			entry.Title = cleanURL
		}
		if entry.SiteName == "" {
			if parsed, perr := url.Parse(cleanURL); perr == nil {
				entry.SiteName = strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
			}
		}
		if entry.FaviconURL == "" {
			if parsed, perr := url.Parse(cleanURL); perr == nil {
				entry.FaviconURL = parsed.Scheme + "://" + parsed.Host + "/favicon.ico"
			}
		}

		_, cerr := s.store.CreateBookmark(r.Context(), entry)
		if cerr != nil {
			if errors.Is(cerr, db.ErrDuplicateURL) {
				skipped++
				continue
			}
			errCount++
			continue
		}
		imported++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"format":   format,
		"total":    len(entries),
		"imported": imported,
		"skipped":  skipped,
		"errors":   errCount,
	})
}

func parseNetscapeHTML(data []byte) ([]db.CreateBookmarkInput, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("invalid Netscape HTML bookmark file")
	}

	var out []db.CreateBookmarkInput
	var walk func(n *html.Node, folderStack []string)
	walk = func(n *html.Node, folderStack []string) {
		var currentFolder string
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			tag := strings.ToLower(c.Data)
			switch tag {
			case "dt":
				var h3Text string
				var anchor *html.Node
				var ddText string
				for sub := c.FirstChild; sub != nil; sub = sub.NextSibling {
					if sub.Type != html.ElementNode {
						continue
					}
					switch strings.ToLower(sub.Data) {
					case "h3":
						h3Text = strings.TrimSpace(textContent(sub))
					case "a":
						anchor = sub
					case "dd":
						ddText = strings.TrimSpace(textContent(sub))
					}
				}
				// Also check sibling DD directly after DT (common in browser exports)
				if ddText == "" && c.NextSibling != nil && c.NextSibling.Type == html.ElementNode && strings.EqualFold(c.NextSibling.Data, "dd") {
					ddText = strings.TrimSpace(textContent(c.NextSibling))
				}

				if h3Text != "" {
					currentFolder = h3Text
				}
				if anchor != nil {
					href := strings.TrimSpace(attrValue(anchor, "href"))
					if href != "" {
						title := strings.TrimSpace(textContent(anchor))
						tags := splitCommaTags(attrValue(anchor, "tags"))
						var lists []string
						if len(folderStack) > 0 {
							lists = []string{folderStack[len(folderStack)-1]}
						}
						createdAt := parseUnixEpochAttr(attrValue(anchor, "add_date"))
						out = append(out, db.CreateBookmarkInput{
							URL:       href,
							Title:     title,
							Notes:     ddText,
							Source:    "import:netscape",
							Tags:      tags,
							Lists:     lists,
							CreatedAt: createdAt,
						})
					}
				}
				// Recurse into nested DL inside DT
				nextStack := folderStack
				if h3Text != "" {
					nextStack = append(append([]string{}, folderStack...), h3Text)
				}
				walk(c, nextStack)
			case "dl":
				nextStack := folderStack
				if currentFolder != "" {
					nextStack = append(append([]string{}, folderStack...), currentFolder)
					currentFolder = ""
				}
				walk(c, nextStack)
			default:
				walk(c, folderStack)
			}
		}
	}

	walk(doc, nil)
	return out, nil
}

type karakeepWrapper struct {
	Bookmarks []json.RawMessage `json:"bookmarks"`
}

type karakeepItem struct {
	Title     string `json:"title"`
	Note      string `json:"note"`
	Notes     string `json:"notes"`
	Archived  bool   `json:"archived"`
	CreatedAt string `json:"createdAt"`
	URL       string `json:"url"`
	Content   *struct {
		Type        string `json:"type"`
		URL         string `json:"url"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Favicon     string `json:"favicon"`
		Text        string `json:"text"`
	} `json:"content"`
	Tags  []json.RawMessage `json:"tags"`
	Lists []json.RawMessage `json:"lists"`
}

func parseKarakeepJSON(data []byte) ([]db.CreateBookmarkInput, error) {
	trimmed := bytes.TrimSpace(data)
	var rawItems []json.RawMessage

	if bytes.HasPrefix(trimmed, []byte("{")) {
		var wrap karakeepWrapper
		if err := json.Unmarshal(trimmed, &wrap); err != nil {
			return nil, errors.New("invalid Karakeep JSON object")
		}
		rawItems = wrap.Bookmarks
	} else if bytes.HasPrefix(trimmed, []byte("[")) {
		if err := json.Unmarshal(trimmed, &rawItems); err != nil {
			return nil, errors.New("invalid Karakeep JSON array")
		}
	} else {
		return nil, errors.New("invalid Karakeep JSON format")
	}

	out := make([]db.CreateBookmarkInput, 0, len(rawItems))
	for _, raw := range rawItems {
		var item karakeepItem
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}

		itemURL := strings.TrimSpace(item.URL)
		var title, desc, favicon string
		if item.Content != nil {
			if itemURL == "" {
				itemURL = strings.TrimSpace(item.Content.URL)
			}
			title = strings.TrimSpace(item.Content.Title)
			desc = strings.TrimSpace(item.Content.Description)
			if desc == "" {
				desc = strings.TrimSpace(item.Content.Text)
			}
			favicon = strings.TrimSpace(item.Content.Favicon)
		}
		if strings.TrimSpace(item.Title) != "" {
			title = strings.TrimSpace(item.Title)
		}
		if itemURL == "" {
			continue
		}

		notes := strings.TrimSpace(item.Note)
		if notes == "" {
			notes = strings.TrimSpace(item.Notes)
		}

		out = append(out, db.CreateBookmarkInput{
			URL:         itemURL,
			Title:       title,
			Description: desc,
			Notes:       notes,
			FaviconURL:  favicon,
			Source:      "import:karakeep",
			Archived:    item.Archived,
			Tags:        extractNames(item.Tags),
			Lists:       extractNames(item.Lists),
			CreatedAt:   normalizeTimestamp(item.CreatedAt),
		})
	}
	return out, nil
}

func extractNames(raws []json.RawMessage) []string {
	var names []string
	for _, raw := range raws {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			if t := strings.TrimSpace(s); t != "" {
				names = append(names, t)
			}
			continue
		}
		var obj struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &obj); err == nil {
			if t := strings.TrimSpace(obj.Name); t != "" {
				names = append(names, t)
			}
		}
	}
	return names
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var rec func(*html.Node)
	rec = func(cur *html.Node) {
		if cur.Type == html.TextNode {
			b.WriteString(cur.Data)
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func splitCommaTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseUnixEpochAttr(raw string) string {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n <= 0 {
		return ""
	}
	return time.Unix(n, 0).UTC().Format(time.RFC3339Nano)
}

func normalizeTimestamp(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return ""
}
