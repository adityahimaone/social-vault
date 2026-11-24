package tagging_test

import (
	"slices"
	"strings"
	"testing"

	"social-vault/internal/tagging"
)

func TestRulesVocabularyAndExcludedTags(t *testing.T) {
	rules := tagging.RuleKeywords()

	for tag := range rules {
		if !tagging.IsAllowedTag(tag) {
			t.Fatalf("rule tag %q is outside fixed 19-tag vocabulary", tag)
		}
	}

	for _, excluded := range []string{"finance", "security", "golang"} {
		if _, ok := rules[excluded]; ok {
			t.Fatalf("excluded tag %q must not have Layer 1 rules", excluded)
		}
	}

	// Noise words must never appear as standalone keywords.
	noise := map[string]bool{"free": true, "you": true, "your": true, "try": true, "out": true}
	for tag, kws := range rules {
		for _, kw := range kws {
			if noise[strings.ToLower(strings.TrimSpace(kw))] {
				t.Fatalf("tag %q contains standalone noise word %q", tag, kw)
			}
		}
	}
}

func TestRulesNeverTagFinanceSecurityGolangOrNoiseOnly(t *testing.T) {
	cases := []struct {
		name string
		in   tagging.Input
	}{
		{
			name: "finance post",
			in: tagging.Input{
				Title:       "Indonesian bond yield & stock market outlook",
				Description: "Suku bunga dan investor pasar modal minggu ini",
				SiteName:    "X",
				URL:         "https://x.com/user/status/1",
			},
		},
		{
			name: "security post",
			in: tagging.Input{
				Title:       "Zero-day CVE vulnerability and password breach report",
				Description: "Vaultwarden security hardening and encryption checklist",
				SiteName:    "X",
				URL:         "https://x.com/user/status/2",
			},
		},
		{
			name: "golang post",
			in: tagging.Input{
				Title:       "Golang concurrency patterns with Go channels",
				Description: "Building fast HTTP services in Go",
				SiteName:    "X",
				URL:         "https://x.com/user/status/3",
			},
		},
		{
			name: "noise words only",
			in: tagging.Input{
				Title:       "Try it out for your team",
				Description: "You can check out your free mind today",
				SiteName:    "X",
				URL:         "https://x.com/user/status/4",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tagging.MatchRules(tc.in)
			for _, forbidden := range []string{"finance", "security", "golang"} {
				if slices.Contains(got, forbidden) {
					t.Fatalf("MatchRules(%q) unexpectedly emitted %q: %v", tc.name, forbidden, got)
				}
			}
			if tc.name == "noise words only" && len(got) != 0 {
				t.Fatalf("expected 0 tags for noise-only input, got %v", got)
			}
		})
	}
}

func TestRulesPositiveMatchingAndBoundaries(t *testing.T) {
	tests := []struct {
		name      string
		in        tagging.Input
		want      []string
		notWanted []string
	}{
		{
			name: "rust crate on github",
			in: tagging.Input{
				Title:       "Building an async HTTP server in Rust with Tokio",
				Description: "Fast runtime benchmark",
				URL:         "https://github.com/tokio-rs/tokio",
			},
			want: []string{"development", "rust"},
		},
		{
			name: "ai and deals phrase",
			in: tagging.Input{
				Title:       "DeepSeek V4 Flash free credits via API",
				Description: "Claim free credits for LLM coding agents",
				URL:         "https://x.com/demo/status/10",
			},
			want: []string{"ai", "deals"},
		},
		{
			name: "design and web",
			in: tagging.Input{
				Title:       "Minimalist Figma UI/UX Landing Page inspiration",
				Description: "TailwindCSS and Shadcn UI components",
				URL:         "https://x.com/demo/status/11",
			},
			want: []string{"design", "web"},
		},
		{
			name: "kpop aespa",
			in: tagging.Input{
				Title:       "aespa #Ningning Instagram update",
				Description: "New comeback pics",
				URL:         "https://x.com/aespapic/status/12",
			},
			want: []string{"kpop"},
		},
		{
			name: "career cpns",
			in: tagging.Input{
				Title:       "Info formasi CPNS 2026 dan lowongan kerja remote",
				Description: "Cek instansi di SSCASN",
				URL:         "https://x.com/demo/status/13",
			},
			want: []string{"career"},
		},
		{
			name: "word boundary safety",
			in: tagging.Input{
				Title:       "How trust and faith paid off on a plain trail",
				Description: "Unrelated personal reflection",
				URL:         "https://example.com/essay-note",
			},
			notWanted: []string{"rust", "ai"},
		},
		{
			name: "url host matching",
			in: tagging.Input{
				Title:       "Crates registry",
				Description: "Package index",
				URL:         "https://crates.io/crates/serde",
			},
			want: []string{"rust"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tagging.MatchRules(tc.in)
			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Fatalf("expected tag %q in %v", w, got)
				}
			}
			for _, nw := range tc.notWanted {
				if slices.Contains(got, nw) {
					t.Fatalf("did not expect tag %q in %v", nw, got)
				}
			}
		})
	}
}

func TestMergeTagsPreservesUserTags(t *testing.T) {
	got := tagging.MergeTags([]string{"custom-user-tag", "Rust"}, []string{"rust", "development", "  ", "ai"})
	want := []string{"custom-user-tag", "Rust", "development", "ai"}
	if !slices.Equal(got, want) {
		t.Fatalf("MergeTags got %v, want %v", got, want)
	}
}
