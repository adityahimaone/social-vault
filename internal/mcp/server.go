package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName            = "social-vault-mcp"
	serverVersion         = "0.2.0"
	defaultListLimit      = 20
	maxListLimit          = 100
	compactDescRuneLimit  = 240
	compactNotesRuneLimit = 240
)

// CompactBookmark is a concise representation returned by MCP tools.
type CompactBookmark struct {
	ID          int64    `json:"id"`
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Notes       string   `json:"notes,omitempty"`
	SiteName    string   `json:"site_name,omitempty"`
	Archived    bool     `json:"archived"`
	Tags        []string `json:"tags"`
	Lists       []string `json:"lists"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

// CompactTag is a concise tag summary.
type CompactTag struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// CompactList is a concise list summary.
type CompactList struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Count       int    `json:"count"`
}

// CompactStats is a concise summary of vault counts and top tags.
type CompactStats struct {
	Total      int          `json:"total"`
	Active     int          `json:"active"`
	Archived   int          `json:"archived"`
	TagsCount  int          `json:"tags_count"`
	ListsCount int          `json:"lists_count"`
	TopTags    []CompactTag `json:"top_tags"`
}

// SearchBookmarksInput defines parameters for search_bookmarks.
type SearchBookmarksInput struct {
	Query    string `json:"query" jsonschema:"Full-text search query across title, URL, description, notes, and tags"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum results to return (1-100, default 20)"`
	Archived string `json:"archived,omitempty" jsonschema:"Filter archive status: all, true, or false"`
	Tag      string `json:"tag,omitempty" jsonschema:"Filter by tag name or ID"`
	List     string `json:"list,omitempty" jsonschema:"Filter by list name or ID"`
}

// AddBookmarkInput defines parameters for add_bookmark.
type AddBookmarkInput struct {
	URL   string   `json:"url" jsonschema:"HTTP or HTTPS URL to save in Social Vault"`
	Title string   `json:"title,omitempty" jsonschema:"Optional custom title (auto-fetched if omitted)"`
	Notes string   `json:"notes,omitempty" jsonschema:"Optional personal notes for the bookmark"`
	Tags  []string `json:"tags,omitempty" jsonschema:"Optional tag names to attach"`
}

// ListBookmarksInput defines parameters for list_bookmarks.
type ListBookmarksInput struct {
	Limit    int    `json:"limit,omitempty" jsonschema:"Maximum bookmarks to return (1-100, default 20)"`
	Offset   int    `json:"offset,omitempty" jsonschema:"Zero-based pagination offset (default 0)"`
	Archived string `json:"archived,omitempty" jsonschema:"Filter archive status: all, true, or false"`
}

// GetBookmarkInput defines parameters for get_bookmark.
type GetBookmarkInput struct {
	ID int64 `json:"id" jsonschema:"Bookmark ID"`
}

// UpdateBookmarkInput defines parameters for update_bookmark.
type UpdateBookmarkInput struct {
	ID       int64     `json:"id" jsonschema:"Bookmark ID to update"`
	Title    *string   `json:"title,omitempty" jsonschema:"Updated bookmark title"`
	Notes    *string   `json:"notes,omitempty" jsonschema:"Updated bookmark notes"`
	Tags     *[]string `json:"tags,omitempty" jsonschema:"Full replacement list of tag names"`
	Archived *bool     `json:"archived,omitempty" jsonschema:"Set true to archive or false to unarchive"`
}

// EmptyInput represents a tool with no arguments.
type EmptyInput struct{}

// BookmarkListOutput is returned by search_bookmarks and list_bookmarks.
type BookmarkListOutput struct {
	Items  []CompactBookmark `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

// BookmarkOutput is returned by add_bookmark, get_bookmark, and update_bookmark.
type BookmarkOutput struct {
	Bookmark CompactBookmark `json:"bookmark"`
}

// TagListOutput is returned by list_tags.
type TagListOutput struct {
	Items []CompactTag `json:"items"`
	Total int          `json:"total"`
}

// ListCollectionsOutput is returned by list_lists.
type ListCollectionsOutput struct {
	Items []CompactList `json:"items"`
	Total int           `json:"total"`
}

// CompactTagReviewItem is a concise representation of a pending tag review entry.
type CompactTagReviewItem struct {
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

// ListTagReviewInput defines parameters for list_tag_review.
type ListTagReviewInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"Maximum pending review items to return (1-100, default 20)"`
}

// TagReviewListOutput is returned by list_tag_review.
type TagReviewListOutput struct {
	Items []CompactTagReviewItem `json:"items"`
	Total int                    `json:"total"`
}

// ResolveTagReviewInput defines parameters for resolve_tag_review.
type ResolveTagReviewInput struct {
	ID     int64  `json:"id" jsonschema:"Tag review queue item ID"`
	Action string `json:"action" jsonschema:"Resolution action: apply or dismiss"`
}

// ResolveTagReviewOutput is returned by resolve_tag_review.
type ResolveTagReviewOutput struct {
	Item     CompactTagReviewItem `json:"item"`
	Bookmark *CompactBookmark     `json:"bookmark,omitempty"`
}

// NewServer registers all Social Vault MCP tools backed by the HTTP API client.
func NewServer(client *Client) *sdkmcp.Server {
	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    serverName,
		Version: serverVersion,
	}, nil)

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "search_bookmarks",
		Description: "Search bookmarks in Social Vault using full-text search with optional archive, tag, or list filters.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in SearchBookmarksInput) (*sdkmcp.CallToolResult, BookmarkListOutput, error) {
		query := strings.TrimSpace(in.Query)
		if query == "" {
			return nil, BookmarkListOutput{}, errors.New("query is required and cannot be empty")
		}
		limit, err := normalizeLimit(in.Limit)
		if err != nil {
			return nil, BookmarkListOutput{}, err
		}
		archived, err := normalizeArchived(in.Archived)
		if err != nil {
			return nil, BookmarkListOutput{}, err
		}

		res, err := client.ListBookmarks(ctx, ListBookmarksParams{
			Query:    query,
			Limit:    limit,
			Archived: archived,
			Tag:      strings.TrimSpace(in.Tag),
			List:     strings.TrimSpace(in.List),
		})
		if err != nil {
			return nil, BookmarkListOutput{}, err
		}

		return nil, toBookmarkListOutput(res), nil
	})

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "add_bookmark",
		Description: "Save a URL to Social Vault with optional title, notes, and tags. Missing title/description metadata is auto-fetched by the vault.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in AddBookmarkInput) (*sdkmcp.CallToolResult, BookmarkOutput, error) {
		cleanURL, err := validateHTTPURL(in.URL)
		if err != nil {
			return nil, BookmarkOutput{}, err
		}

		created, err := client.AddBookmark(ctx, AddBookmarkRequest{
			URL:    cleanURL,
			Title:  strings.TrimSpace(in.Title),
			Notes:  strings.TrimSpace(in.Notes),
			Tags:   cleanStringSlice(in.Tags),
			Source: "mcp",
		})
		if err != nil {
			return nil, BookmarkOutput{}, err
		}

		return nil, BookmarkOutput{Bookmark: toCompactBookmark(*created, false)}, nil
	})

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "list_bookmarks",
		Description: "List recent bookmarks in Social Vault with pagination and optional archive status filter.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListBookmarksInput) (*sdkmcp.CallToolResult, BookmarkListOutput, error) {
		limit, err := normalizeLimit(in.Limit)
		if err != nil {
			return nil, BookmarkListOutput{}, err
		}
		if in.Offset < 0 {
			return nil, BookmarkListOutput{}, errors.New("offset must be greater than or equal to 0")
		}
		archived, err := normalizeArchived(in.Archived)
		if err != nil {
			return nil, BookmarkListOutput{}, err
		}

		res, err := client.ListBookmarks(ctx, ListBookmarksParams{
			Limit:    limit,
			Offset:   in.Offset,
			Archived: archived,
		})
		if err != nil {
			return nil, BookmarkListOutput{}, err
		}

		return nil, toBookmarkListOutput(res), nil
	})

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_bookmark",
		Description: "Fetch full details for a single bookmark by ID.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in GetBookmarkInput) (*sdkmcp.CallToolResult, BookmarkOutput, error) {
		if in.ID <= 0 {
			return nil, BookmarkOutput{}, errors.New("id must be a positive integer")
		}
		b, err := client.GetBookmark(ctx, in.ID)
		if err != nil {
			return nil, BookmarkOutput{}, err
		}
		return nil, BookmarkOutput{Bookmark: toCompactBookmark(*b, true)}, nil
	})

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "list_tags",
		Description: "List all tags in Social Vault with bookmark counts.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ EmptyInput) (*sdkmcp.CallToolResult, TagListOutput, error) {
		tags, err := client.ListTags(ctx)
		if err != nil {
			return nil, TagListOutput{}, err
		}
		items := make([]CompactTag, 0, len(tags))
		for _, t := range tags {
			items = append(items, CompactTag{
				ID:    t.ID,
				Name:  t.Name,
				Count: t.Count,
			})
		}
		return nil, TagListOutput{Items: items, Total: len(items)}, nil
	})

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "list_lists",
		Description: "List all bookmark lists (collections) in Social Vault with bookmark counts.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ EmptyInput) (*sdkmcp.CallToolResult, ListCollectionsOutput, error) {
		lists, err := client.ListLists(ctx)
		if err != nil {
			return nil, ListCollectionsOutput{}, err
		}
		items := make([]CompactList, 0, len(lists))
		for _, l := range lists {
			items = append(items, CompactList{
				ID:          l.ID,
				Name:        l.Name,
				Description: l.Description,
				Count:       l.Count,
			})
		}
		return nil, ListCollectionsOutput{Items: items, Total: len(items)}, nil
	})

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "get_stats",
		Description: "Get Social Vault summary statistics (total, active, archived, tag/list counts, and top tags).",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, _ EmptyInput) (*sdkmcp.CallToolResult, CompactStats, error) {
		st, err := client.GetStats(ctx)
		if err != nil {
			return nil, CompactStats{}, err
		}
		topTags := make([]CompactTag, 0, len(st.TopTags))
		for _, t := range st.TopTags {
			topTags = append(topTags, CompactTag{
				ID:    t.ID,
				Name:  t.Name,
				Count: t.Count,
			})
		}
		return nil, CompactStats{
			Total:      st.Total,
			Active:     st.Active,
			Archived:   st.Archived,
			TagsCount:  st.TagsCount,
			ListsCount: st.ListsCount,
			TopTags:    topTags,
		}, nil
	})

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "update_bookmark",
		Description: "Update a bookmark's title, notes, tags, or archived status by ID.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in UpdateBookmarkInput) (*sdkmcp.CallToolResult, BookmarkOutput, error) {
		if in.ID <= 0 {
			return nil, BookmarkOutput{}, errors.New("id must be a positive integer")
		}
		if in.Title == nil && in.Notes == nil && in.Tags == nil && in.Archived == nil {
			return nil, BookmarkOutput{}, errors.New("provide at least one field to update: title, notes, tags, or archived")
		}

		req := UpdateBookmarkRequest{
			Title:    in.Title,
			Notes:    in.Notes,
			Archived: in.Archived,
		}
		if in.Tags != nil {
			cleaned := cleanStringSlice(*in.Tags)
			req.Tags = &cleaned
		}

		updated, err := client.UpdateBookmark(ctx, in.ID, req)
		if err != nil {
			return nil, BookmarkOutput{}, err
		}
		return nil, BookmarkOutput{Bookmark: toCompactBookmark(*updated, true)}, nil
	})

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "list_tag_review",
		Description: "List pending auto-tagging suggestions queued by Jev (scores 0.5-0.8) for agent review.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ListTagReviewInput) (*sdkmcp.CallToolResult, TagReviewListOutput, error) {
		limit, err := normalizeLimit(in.Limit)
		if err != nil {
			return nil, TagReviewListOutput{}, err
		}
		res, err := client.ListTagReview(ctx, limit)
		if err != nil {
			return nil, TagReviewListOutput{}, err
		}
		items := make([]CompactTagReviewItem, 0, len(res.Items))
		for _, item := range res.Items {
			items = append(items, toCompactTagReviewItem(item))
		}
		return nil, TagReviewListOutput{Items: items, Total: len(items)}, nil
	})

	sdkmcp.AddTool(srv, &sdkmcp.Tool{
		Name:        "resolve_tag_review",
		Description: "Apply or dismiss a pending tag review queue item by ID.",
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, in ResolveTagReviewInput) (*sdkmcp.CallToolResult, ResolveTagReviewOutput, error) {
		if in.ID <= 0 {
			return nil, ResolveTagReviewOutput{}, errors.New("id must be a positive integer")
		}
		action := strings.ToLower(strings.TrimSpace(in.Action))
		switch action {
		case "apply":
			res, err := client.ApplyTagReview(ctx, in.ID)
			if err != nil {
				return nil, ResolveTagReviewOutput{}, err
			}
			bm := toCompactBookmark(res.Bookmark, false)
			return nil, ResolveTagReviewOutput{
				Item:     toCompactTagReviewItem(res.Item),
				Bookmark: &bm,
			}, nil
		case "dismiss":
			res, err := client.DismissTagReview(ctx, in.ID)
			if err != nil {
				return nil, ResolveTagReviewOutput{}, err
			}
			return nil, ResolveTagReviewOutput{
				Item: toCompactTagReviewItem(res.Item),
			}, nil
		default:
			return nil, ResolveTagReviewOutput{}, errors.New("action must be 'apply' or 'dismiss'")
		}
	})

	return srv
}

func toCompactTagReviewItem(item APITagReviewItem) CompactTagReviewItem {
	return CompactTagReviewItem{
		ID:            item.ID,
		BookmarkID:    item.BookmarkID,
		BookmarkTitle: item.BookmarkTitle,
		BookmarkURL:   item.BookmarkURL,
		SuggestedTags: item.SuggestedTags,
		JevScores:     item.JevScores,
		Source:        item.Source,
		Status:        item.Status,
		CreatedAt:     item.CreatedAt,
		ResolvedAt:    item.ResolvedAt,
	}
}

func toBookmarkListOutput(res *BookmarkListResponse) BookmarkListOutput {
	items := make([]CompactBookmark, 0, len(res.Items))
	for _, b := range res.Items {
		items = append(items, toCompactBookmark(b, false))
	}
	return BookmarkListOutput{
		Items:  items,
		Total:  res.Total,
		Limit:  res.Limit,
		Offset: res.Offset,
	}
}

func toCompactBookmark(b APIBookmark, full bool) CompactBookmark {
	tags := make([]string, 0, len(b.Tags))
	for _, t := range b.Tags {
		tags = append(tags, t.Name)
	}
	lists := make([]string, 0, len(b.Lists))
	for _, l := range b.Lists {
		lists = append(lists, l.Name)
	}

	desc := b.Description
	notes := b.Notes
	updatedAt := ""
	if !full {
		desc = truncateRunes(desc, compactDescRuneLimit)
		notes = truncateRunes(notes, compactNotesRuneLimit)
	} else {
		updatedAt = b.UpdatedAt
	}

	return CompactBookmark{
		ID:          b.ID,
		URL:         b.URL,
		Title:       b.Title,
		Description: desc,
		Notes:       notes,
		SiteName:    b.SiteName,
		Archived:    b.Archived,
		Tags:        tags,
		Lists:       lists,
		CreatedAt:   b.CreatedAt,
		UpdatedAt:   updatedAt,
	}
}

func normalizeLimit(raw int) (int, error) {
	if raw == 0 {
		return defaultListLimit, nil
	}
	if raw < 1 || raw > maxListLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", maxListLimit)
	}
	return raw, nil
}

func normalizeArchived(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "", "all", "true", "false":
		return v, nil
	default:
		return "", errors.New("archived must be one of: all, true, false")
	}
}

func validateHTTPURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("url is required")
	}
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("url must be a valid http or https URL")
	}
	return u.String(), nil
}

func cleanStringSlice(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	var b strings.Builder
	count := 0
	for _, r := range s {
		if count >= max-1 {
			break
		}
		b.WriteRune(r)
		count++
	}
	b.WriteRune('…')
	return b.String()
}
