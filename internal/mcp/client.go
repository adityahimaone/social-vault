package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultVaultURL    = "https://vault.adityahimaone.space"
	defaultHTTPTimeout = 20 * time.Second
	maxAPIErrorBytes   = 64 * 1024
)

// Config holds connection settings for the Social Vault HTTP API.
type Config struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// LoadConfigFromEnv reads VAULT_URL and VAULT_API_KEY using getenv.
func LoadConfigFromEnv(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, errors.New("getenv function is required")
	}

	rawURL := strings.TrimSpace(getenv("VAULT_URL"))
	if rawURL == "" {
		rawURL = DefaultVaultURL
	}
	baseURL, err := normalizeBaseURL(rawURL)
	if err != nil {
		return Config{}, fmt.Errorf("invalid VAULT_URL: %w", err)
	}

	apiKey := strings.TrimSpace(getenv("VAULT_API_KEY"))
	if apiKey == "" {
		return Config{}, errors.New("VAULT_API_KEY environment variable is required")
	}

	return Config{
		BaseURL: baseURL,
		APIKey:  apiKey,
	}, nil
}

func normalizeBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("must be a valid http or https URL")
	}
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

// Client communicates with a running Social Vault server over REST/JSON.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient constructs a validated HTTP API client for Social Vault.
func NewClient(cfg Config) (*Client, error) {
	rawURL := strings.TrimSpace(cfg.BaseURL)
	if rawURL == "" {
		rawURL = DefaultVaultURL
	}
	baseURL, err := normalizeBaseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("VAULT_API_KEY is required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultHTTPTimeout}
	}

	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: httpClient,
	}, nil
}

// APITag mirrors the tag object returned by the Social Vault REST API.
type APITag struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Count     int    `json:"count,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// APIList mirrors the list object returned by the Social Vault REST API.
type APIList struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Count       int    `json:"count,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// APIBookmark mirrors the bookmark object returned by the Social Vault REST API.
type APIBookmark struct {
	ID          int64     `json:"id"`
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Notes       string    `json:"notes"`
	FaviconURL  string    `json:"favicon_url"`
	SiteName    string    `json:"site_name"`
	Source      string    `json:"source"`
	Archived    bool      `json:"archived"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
	Tags        []APITag  `json:"tags"`
	Lists       []APIList `json:"lists"`
}

// APIStats mirrors GET /api/stats.
type APIStats struct {
	Total      int      `json:"total"`
	Active     int      `json:"active"`
	Archived   int      `json:"archived"`
	TagsCount  int      `json:"tags_count"`
	ListsCount int      `json:"lists_count"`
	TopTags    []APITag `json:"top_tags"`
}

// BookmarkListResponse mirrors GET /api/bookmarks.
type BookmarkListResponse struct {
	Items  []APIBookmark `json:"items"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

// APITagReviewItem mirrors one item returned by GET /api/tag-review.
type APITagReviewItem struct {
	ID            int64              `json:"id"`
	BookmarkID    int64              `json:"bookmark_id"`
	BookmarkTitle string             `json:"bookmark_title"`
	BookmarkURL   string             `json:"bookmark_url"`
	SuggestedTags []string           `json:"suggested_tags"`
	JevScores     map[string]float64 `json:"jev_scores"`
	Source        string             `json:"source"`
	Status        string             `json:"status"`
	CreatedAt     string             `json:"created_at"`
	ResolvedAt    string             `json:"resolved_at,omitempty"`
}

// TagReviewListResponse mirrors GET /api/tag-review.
type TagReviewListResponse struct {
	Items []APITagReviewItem `json:"items"`
	Total int                `json:"total"`
}

// ApplyTagReviewResponse mirrors POST /api/tag-review/{id}/apply.
type ApplyTagReviewResponse struct {
	Item     APITagReviewItem `json:"item"`
	Bookmark APIBookmark      `json:"bookmark"`
}

// DismissTagReviewResponse mirrors POST /api/tag-review/{id}/dismiss.
type DismissTagReviewResponse struct {
	Item APITagReviewItem `json:"item"`
}

// ListBookmarksParams configures GET /api/bookmarks query parameters.
type ListBookmarksParams struct {
	Query    string
	Limit    int
	Offset   int
	Archived string
	Tag      string
	List     string
}

// AddBookmarkRequest is sent to POST /api/bookmarks.
type AddBookmarkRequest struct {
	URL    string   `json:"url"`
	Title  string   `json:"title,omitempty"`
	Notes  string   `json:"notes,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Source string   `json:"source,omitempty"`
}

// UpdateBookmarkRequest is sent to PATCH /api/bookmarks/{id}.
type UpdateBookmarkRequest struct {
	Title    *string   `json:"title,omitempty"`
	Notes    *string   `json:"notes,omitempty"`
	Tags     *[]string `json:"tags,omitempty"`
	Archived *bool     `json:"archived,omitempty"`
}

type apiErrorBody struct {
	Error    string       `json:"error"`
	Bookmark *APIBookmark `json:"bookmark,omitempty"`
}

// ListBookmarks queries GET /api/bookmarks.
func (c *Client) ListBookmarks(ctx context.Context, p ListBookmarksParams) (*BookmarkListResponse, error) {
	q := url.Values{}
	if v := strings.TrimSpace(p.Query); v != "" {
		q.Set("q", v)
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Offset > 0 {
		q.Set("offset", strconv.Itoa(p.Offset))
	}
	if v := strings.TrimSpace(p.Archived); v != "" {
		q.Set("archived", v)
	}
	if v := strings.TrimSpace(p.Tag); v != "" {
		q.Set("tag", v)
	}
	if v := strings.TrimSpace(p.List); v != "" {
		q.Set("list", v)
	}

	path := "/api/bookmarks"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var out BookmarkListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AddBookmark creates a bookmark via POST /api/bookmarks.
func (c *Client) AddBookmark(ctx context.Context, req AddBookmarkRequest) (*APIBookmark, error) {
	if strings.TrimSpace(req.Source) == "" {
		req.Source = "mcp"
	}
	var out APIBookmark
	if err := c.doJSON(ctx, http.MethodPost, "/api/bookmarks", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetBookmark fetches a single bookmark via GET /api/bookmarks/{id}.
func (c *Client) GetBookmark(ctx context.Context, id int64) (*APIBookmark, error) {
	var out APIBookmark
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/bookmarks/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateBookmark updates a bookmark via PATCH /api/bookmarks/{id}.
func (c *Client) UpdateBookmark(ctx context.Context, id int64, req UpdateBookmarkRequest) (*APIBookmark, error) {
	var out APIBookmark
	if err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/api/bookmarks/%d", id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTags fetches all tags via GET /api/tags.
func (c *Client) ListTags(ctx context.Context) ([]APITag, error) {
	var out struct {
		Items []APITag `json:"items"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/tags", nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// ListLists fetches all lists via GET /api/lists.
func (c *Client) ListLists(ctx context.Context) ([]APIList, error) {
	var out struct {
		Items []APIList `json:"items"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/lists", nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// GetStats fetches vault summary counts via GET /api/stats.
func (c *Client) GetStats(ctx context.Context) (*APIStats, error) {
	var out APIStats
	if err := c.doJSON(ctx, http.MethodGet, "/api/stats", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTagReview fetches pending tag review items via GET /api/tag-review.
func (c *Client) ListTagReview(ctx context.Context, limit int) (*TagReviewListResponse, error) {
	path := "/api/tag-review"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var out TagReviewListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ApplyTagReview applies suggested tags for a review item via POST /api/tag-review/{id}/apply.
func (c *Client) ApplyTagReview(ctx context.Context, id int64) (*ApplyTagReviewResponse, error) {
	var out ApplyTagReviewResponse
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/tag-review/%d/apply", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DismissTagReview dismisses a review item via POST /api/tag-review/{id}/dismiss.
func (c *Client) DismissTagReview(ctx context.Context, id int64) (*DismissTagReviewResponse, error) {
	var out DismissTagReviewResponse
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/tag-review/%d/dismiss", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody any, dst any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		raw, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vault request to %s failed: %w", c.baseURL, sanitizeNetworkError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return formatHTTPError(resp.StatusCode, resp.Body)
	}

	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("decode vault response: %w", err)
	}
	return nil
}

func formatHTTPError(status int, body io.Reader) error {
	raw, _ := io.ReadAll(io.LimitReader(body, maxAPIErrorBytes))
	var parsed apiErrorBody
	_ = json.Unmarshal(raw, &parsed)
	msg := strings.TrimSpace(parsed.Error)

	switch status {
	case http.StatusUnauthorized:
		return errors.New("authentication failed (HTTP 401): check that VAULT_API_KEY is valid")
	case http.StatusForbidden:
		return errors.New("access denied (HTTP 403): API key does not have permission")
	case http.StatusNotFound:
		if msg != "" {
			return fmt.Errorf("not found (HTTP 404): %s", msg)
		}
		return errors.New("not found (HTTP 404)")
	case http.StatusConflict:
		if parsed.Bookmark != nil && parsed.Bookmark.ID > 0 {
			return fmt.Errorf("bookmark already exists (id=%d, title=%q)", parsed.Bookmark.ID, parsed.Bookmark.Title)
		}
		if msg != "" {
			return fmt.Errorf("conflict (HTTP 409): %s", msg)
		}
		return errors.New("conflict (HTTP 409)")
	default:
		if msg != "" {
			return fmt.Errorf("vault API error (HTTP %d): %s", status, msg)
		}
		return fmt.Errorf("vault API error (HTTP %d)", status)
	}
}

func sanitizeNetworkError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err
	}
	return err
}
