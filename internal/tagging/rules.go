package tagging

import (
	"net/url"
	"slices"
	"sort"
	"strings"
)

// Curated Layer 1 keyword/phrase rules derived from the 377 AI-tagged bookmarks.
// Intentionally excludes finance, security, and golang (zero keyword signal),
// and excludes standalone noise words (free, you, your, try, out).
var curatedRules = map[string][]string{
	"ai": {
		"ai", "llm", "llms", "claude", "anthropic", "openai", "chatgpt", "gpt-4", "gpt-5",
		"gemini", "deepseek", "opencode", "glm", "opus", "sonnet", "codex",
		"agent", "agents", "agentic", "mcp", "prompt", "vibe coding",
		"machine learning", "neural", "diffusion", "midjourney", "perplexity",
		"openclaw", "skills.sh", "npx skills", "ai coding",
	},
	"design": {
		"design", "designer", "designers", "design system", "design systems",
		"figma", "ui/ux", "ux", "typography", "wireframe", "prototype", "prototyping",
		"color palette", "branding", "illustration", "icon", "icons",
		"landing page", "framer", "dribbble", "behance", "design inspiration",
		"ui components", "shadcn", "kubai087",
	},
	"tools": {
		"mcp", "docker", "open-source", "browser extension",
		"extension", "automation", "self-hosted", "self hosted",
		"homebrew", "raycast", "obsidian", "vscode", "neovim",
		"terminal", "devtools", "desktop app", "mac app", "windows", "linux",
	},
	"development": {
		"github", "open-source", "docker", "kubernetes",
		"sdk", "typescript", "javascript", "python", "nextjs", "next.js",
		"svelte", "vue", "sqlite", "postgres", "postgresql", "database",
		"backend", "fullstack", "compiler", "linux", "terminal",
		"developer", "programming", "codebase", "repository", "rust",
	},
	"deals": {
		"free credits", "free api", "free access", "for free",
		"promo code", "coupon", "discount", "giveaway", "credits", "claim",
		"sign up", "free plan", "free pro", "free trial", "limited time",
		"voucher", "cashback", "promo",
	},
	"web": {
		"landing page", "html", "css", "tailwind", "tailwindcss",
		"webgl", "threejs", "browser", "shadcn", "next.js", "nextjs",
		"svelte", "astro", "vite", "ui components", "web app", "webapp",
		"ui sdk", "isometric", "templates",
	},
	"reading": {
		"ebook", "e-book", "newsletter", "essay", "essays", "longread",
		"cheatsheet", "cheat sheet", "handbook", "guide",
		"tutorial", "documentation", "whitepaper",
	},
	"career": {
		"cpns", "sscasn", "lowongan", "loker", "cari kerja", "lamaran",
		"interview", "resume", "hiring", "recruiter", "salary", "gaji",
		"career", "karir", "karier", "jobs", "remote work", "freelance",
		"formasi", "instansi", "portfolio", "vindikasi",
	},
	"news": {
		"palestine", "president", "presiden", "election", "pemilu",
		"geopolitics", "headline", "headlines",
	},
	"entertainment": {
		"movie", "movies", "films", "netflix", "anime",
		"manga", "tv show", "tv shows", "concert", "konser",
		"ishowspeed", "ishowspeedhq", "popbase", "pop base", "trailer",
		"soundtrack", "podcast", "comedy", "standup",
	},
	"lifestyle": {
		"recipe", "resep", "coffee", "kopi", "travel", "wisata", "hotel",
		"fitness", "workout", "gym", "skincare", "fashion", "outfit",
		"minimalist", "habit", "habits", "kereta",
	},
	"kpop": {
		"kpop", "k-pop", "aespa", "ningning", "#ningning", "aespapic",
		"imnotningning", "karina", "giselle", "blackpink", "bts",
		"newjeans", "le sserafim", "exo", "nct", "seventeen",
		"stray kids", "red velvet", "itzy", "illit", "babymonster",
	},
	"gaming": {
		"gta", "gta 6", "gta vi", "gaming", "gamer", "steam", "playstation",
		"ps5", "xbox", "nintendo", "esports", "valorant", "minecraft",
		"elden ring", "cyberpunk", "mmorpg", "indie game", "video game",
		"video games", "gameplay",
	},
	"nsfw": {
		"nsfw", "age-restricted", "adult content", "18+", "onlyfans", "bokep",
		"ngewe", "porn", "hentai",
	},
	"rust": {
		"rust", "rustlang", "rust-lang", "cargo", "crates.io", "tokio", "axum",
		"clippy", "borrow checker",
	},
	"sports": {
		"formula 1", "formula1", "#f1", "f1", "f1speed_indo", "antonelli",
		"verstappen", "hamilton", "ferrari", "mclaren", "grand prix", "#mexicogp",
		"motogp", "premier league", "champions league", "football", "soccer",
		"nba", "basketball", "badminton", "bulutangkis", "timnas", "fifa",
	},
}

// RuleKeywords returns a defensive copy of the Layer 1 keyword rules per tag.
func RuleKeywords() map[string][]string {
	out := make(map[string][]string, len(curatedRules))
	for tag, kws := range curatedRules {
		out[tag] = slices.Clone(kws)
	}
	return out
}

// MatchRules evaluates Layer 1 curated keyword rules synchronously against
// title, description, site_name, and URL host. Returns sorted allowed tags.
func MatchRules(in Input) []string {
	text := buildSearchText(in)
	if text == "" {
		return nil
	}

	var matched []string
	for tag, kws := range curatedRules {
		if !IsAllowedTag(tag) {
			continue
		}
		for _, kw := range kws {
			if containsKeyword(text, kw) {
				matched = append(matched, tag)
				break
			}
		}
	}
	sort.Strings(matched)
	return matched
}

func buildSearchText(in Input) string {
	host := ""
	if rawURL := strings.TrimSpace(in.URL); rawURL != "" {
		if parsed, err := url.Parse(rawURL); err == nil {
			host = strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
		}
	}

	parts := []string{
		strings.TrimSpace(in.Title),
		strings.TrimSpace(in.Description),
		strings.TrimSpace(in.SiteName),
		host,
	}
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.ToLower(strings.Join(nonEmpty, " "))
}

func containsKeyword(text, kw string) bool {
	if kw == "" {
		return false
	}
	start := 0
	for start <= len(text)-len(kw) {
		idx := strings.Index(text[start:], kw)
		if idx < 0 {
			return false
		}
		absStart := start + idx
		absEnd := absStart + len(kw)

		leftOK := true
		if isWordChar(kw[0]) || kw[0] == '#' {
			leftOK = absStart == 0 || !isWordChar(text[absStart-1])
		}

		rightOK := true
		if isWordChar(kw[len(kw)-1]) {
			rightOK = absEnd == len(text) || !isWordChar(text[absEnd])
		}

		if leftOK && rightOK {
			return true
		}
		start = absStart + 1
	}
	return false
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}
