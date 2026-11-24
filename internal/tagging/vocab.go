package tagging

import (
	"slices"
	"strings"
)

// Vocabulary is the fixed set of 19 allowed auto-tagging labels.
var Vocabulary = []string{
	"ai",
	"design",
	"tools",
	"development",
	"deals",
	"web",
	"reading",
	"career",
	"news",
	"entertainment",
	"lifestyle",
	"kpop",
	"finance",
	"gaming",
	"nsfw",
	"security",
	"rust",
	"sports",
	"golang",
}

var allowedTagSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(Vocabulary))
	for _, t := range Vocabulary {
		m[t] = struct{}{}
	}
	return m
}()

// IsAllowedTag reports whether name belongs to the fixed 19-tag vocabulary (case-insensitive).
func IsAllowedTag(name string) bool {
	_, ok := allowedTagSet[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// Input contains the bookmark metadata used by Layer 1 rules and Layer 2 Jev.
type Input struct {
	URL         string
	Title       string
	Description string
	SiteName    string
}

// MergeTags preserves every existing tag in order and appends deduplicated
// (case-insensitive) non-empty tags from added without dropping any user tag.
func MergeTags(existing, added []string) []string {
	out := make([]string, 0, len(existing)+len(added))
	seen := make(map[string]struct{}, len(existing)+len(added))

	appendList := func(items []string) {
		for _, raw := range items {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, trimmed)
		}
	}

	appendList(existing)
	appendList(added)
	return slices.Clip(out)
}
