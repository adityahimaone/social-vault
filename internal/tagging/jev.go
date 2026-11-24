package tagging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	DefaultJevEndpoint    = "https://api.typesafe.ai/v1/systemone"
	JevModel              = "jev-1.13.0"
	DefaultJevTimeout     = 15 * time.Second
	JevAutoApplyThreshold = 0.8
	JevReviewThreshold    = 0.5
	maxJevErrorBytes      = 32 * 1024
)

type noulCriteria struct {
	True  string `json:"true"`
	False string `json:"false"`
}

type noulQuestion struct {
	Type         string       `json:"type"`
	Instructions string       `json:"instructions"`
	Criteria     noulCriteria `json:"criteria"`
}

type jevState struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	SiteName    string `json:"site_name"`
	URL         string `json:"url"`
}

type jevRequest struct {
	Model     string                  `json:"model"`
	State     jevState                `json:"state"`
	Questions map[string]noulQuestion `json:"questions"`
}

type jevAnswer struct {
	Type string   `json:"type"`
	Noul *float64 `json:"noul"`
}

// JevUsage holds token usage reported by the System One API.
type JevUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type jevResponse struct {
	Model   string               `json:"model"`
	Answers map[string]jevAnswer `json:"answers"`
	Usage   JevUsage             `json:"usage"`
}

// JevResult represents the threshold-routed classification output from Jev.
type JevResult struct {
	AutoTags     []string           `json:"auto_tags"`
	ReviewTags   []string           `json:"review_tags"`
	ReviewScores map[string]float64 `json:"review_scores"`
	AllScores    map[string]float64 `json:"all_scores"`
	Usage        JevUsage           `json:"usage"`
}

// JevClient calls TypeSafe AI's System One API with the pinned Jev model.
type JevClient struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

// NewJevClient constructs a JevClient pointing at the production endpoint with a 15s timeout.
func NewJevClient(apiKey string) *JevClient {
	return NewJevClientWithOptions(apiKey, DefaultJevEndpoint, nil)
}

// NewJevClientWithOptions constructs a JevClient with custom endpoint and HTTP client (for tests).
func NewJevClientWithOptions(apiKey, endpoint string, httpClient *http.Client) *JevClient {
	ep := strings.TrimSpace(endpoint)
	if ep == "" {
		ep = DefaultJevEndpoint
	}
	hc := httpClient
	if hc == nil {
		hc = &http.Client{Timeout: DefaultJevTimeout}
	} else if hc.Timeout <= 0 {
		clone := *hc
		clone.Timeout = DefaultJevTimeout
		hc = &clone
	}
	return &JevClient{
		apiKey:     strings.TrimSpace(apiKey),
		endpoint:   ep,
		httpClient: hc,
	}
}

// Enabled reports whether a non-empty Jev API key is configured.
func (c *JevClient) Enabled() bool {
	return c != nil && c.apiKey != ""
}

// Classify sends one System One request with 19 parallel noul questions and partitions
// the resulting scores by threshold: >=0.8 auto-apply, [0.5, 0.8) review queue, <0.5 ignore.
func (c *JevClient) Classify(ctx context.Context, in Input) (*JevResult, error) {
	if !c.Enabled() {
		return nil, errors.New("jev client is disabled (missing JEV_API_KEY)")
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultJevTimeout)
		defer cancel()
	}

	payload := jevRequest{
		Model: JevModel,
		State: jevState{
			Title:       strings.TrimSpace(in.Title),
			Description: strings.TrimSpace(in.Description),
			SiteName:    strings.TrimSpace(in.SiteName),
			URL:         strings.TrimSpace(in.URL),
		},
		Questions: buildJevQuestions(),
	}

	rawBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal jev request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("build jev request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jev request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxJevErrorBytes*4))
	if err != nil {
		return nil, fmt.Errorf("read jev response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(respBytes))
		if len(snippet) > 256 {
			snippet = snippet[:256]
		}
		if snippet != "" {
			return nil, fmt.Errorf("jev API error (HTTP %d): %s", resp.StatusCode, snippet)
		}
		return nil, fmt.Errorf("jev API error (HTTP %d)", resp.StatusCode)
	}

	var decoded jevResponse
	if err := json.Unmarshal(respBytes, &decoded); err != nil {
		return nil, fmt.Errorf("decode jev response: %w", err)
	}
	if len(decoded.Answers) == 0 {
		return nil, errors.New("malformed jev response: empty answers")
	}

	res := &JevResult{
		ReviewScores: make(map[string]float64),
		AllScores:    make(map[string]float64),
		Usage:        decoded.Usage,
	}

	for rawTag, ans := range decoded.Answers {
		tag := strings.ToLower(strings.TrimSpace(rawTag))
		if !IsAllowedTag(tag) || ans.Noul == nil {
			continue
		}
		score := *ans.Noul
		if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
			continue
		}

		res.AllScores[tag] = score
		switch {
		case score >= JevAutoApplyThreshold:
			res.AutoTags = append(res.AutoTags, tag)
		case score >= JevReviewThreshold:
			res.ReviewTags = append(res.ReviewTags, tag)
			res.ReviewScores[tag] = score
		}
	}

	sort.Strings(res.AutoTags)
	sort.Strings(res.ReviewTags)
	return res, nil
}

func buildJevQuestions() map[string]noulQuestion {
	specs := map[string]struct {
		instr     string
		trueCrit  string
		falseCrit string
	}{
		"ai": {
			instr:     "Is this bookmark primarily about artificial intelligence, LLMs, AI agents, prompts, or machine learning?",
			trueCrit:  "Mentions AI models, LLMs, AI coding assistants, agents, prompts, or ML tools.",
			falseCrit: "Only incidental or no meaningful connection to AI or machine learning.",
		},
		"design": {
			instr:     "Is this bookmark about UI/UX design, visual design, typography, design systems, or Figma?",
			trueCrit:  "Covers UI/UX craft, visual inspiration, Figma, typography, branding, or design systems.",
			falseCrit: "Not focused on visual or product design.",
		},
		"tools": {
			instr:     "Is this bookmark about a software tool, utility, CLI, browser extension, or productivity app?",
			trueCrit:  "Showcases or discusses an actionable app, CLI, MCP server, extension, or developer utility.",
			falseCrit: "Not introducing or discussing a reusable software tool or app.",
		},
		"development": {
			instr:     "Is this bookmark about software engineering, programming, APIs, databases, or developer workflows?",
			trueCrit:  "Discusses coding, repositories, architecture, frameworks, databases, or engineering practices.",
			falseCrit: "Unrelated to software development or programming.",
		},
		"deals": {
			instr:     "Is this bookmark about a free tier, free credits, promo, discount, giveaway, or limited-time deal?",
			trueCrit:  "Highlights free API credits, giveaways, discounts, promo codes, or free plan offers.",
			falseCrit: "Not promoting a deal, discount, giveaway, or free credit offer.",
		},
		"web": {
			instr:     "Is this bookmark about web development, websites, frontend frameworks, CSS, or browser tech?",
			trueCrit:  "Covers web apps, landing pages, HTML/CSS/Tailwind, React/Next/Svelte, or browser capabilities.",
			falseCrit: "Not specifically about the web platform or frontend web engineering.",
		},
		"reading": {
			instr:     "Is this bookmark an essay, longread, guide, book, documentation, or educational read to revisit?",
			trueCrit:  "Long-form article, essay, tutorial, guide, documentation, or book recommendation.",
			falseCrit: "Short status update, bare link, or non-reading item.",
		},
		"career": {
			instr:     "Is this bookmark about jobs, hiring, resumes, interviews, salaries, CPNS, or career advice?",
			trueCrit:  "Discusses job openings, CPNS/SSCASN, hiring, interviews, freelance income, or career growth.",
			falseCrit: "Unrelated to jobs, employment, or career topics.",
		},
		"news": {
			instr:     "Is this bookmark news, current events, politics, policy, or breaking reporting?",
			trueCrit:  "Reports current events, geopolitics, government policy, elections, or breaking news.",
			falseCrit: "Not news or current-affairs reporting.",
		},
		"entertainment": {
			instr:     "Is this bookmark about movies, TV shows, music, pop culture, streamers, or entertainment media?",
			trueCrit:  "Covers films, series, anime, music, creators/streamers, or pop-culture entertainment.",
			falseCrit: "Not focused on entertainment media or pop culture.",
		},
		"lifestyle": {
			instr:     "Is this bookmark about daily life, personal habits, travel, food, coffee, fitness, or lifestyle?",
			trueCrit:  "Relates to everyday living, travel, food/coffee, hobbies, health, or personal reflections.",
			falseCrit: "Not about personal lifestyle or everyday living.",
		},
		"kpop": {
			instr:     "Is this bookmark about K-pop idols, K-pop groups, fan updates, or Korean pop music?",
			trueCrit:  "Mentions K-pop groups, idols (such as aespa/Ningning), comebacks, or K-pop fandom content.",
			falseCrit: "Not about K-pop artists or music.",
		},
		"finance": {
			instr:     "Is this bookmark about personal finance, investing, stocks, macroeconomics, markets, or money?",
			trueCrit:  "Discusses investments, markets, yields, interest rates, taxes, or financial management.",
			falseCrit: "Not related to finance, investing, or economics.",
		},
		"gaming": {
			instr:     "Is this bookmark about video games, game releases, consoles, esports, or gaming culture?",
			trueCrit:  "Covers video games (e.g., GTA, Steam, PlayStation, Nintendo), gameplay, or esports.",
			falseCrit: "Not about video games or gaming.",
		},
		"nsfw": {
			instr:     "Is this bookmark adult, sexually explicit, or age-restricted NSFW content?",
			trueCrit:  "Contains explicit adult themes, age-restricted media warnings, or NSFW material.",
			falseCrit: "Safe for work and non-explicit.",
		},
		"security": {
			instr:     "Is this bookmark about cybersecurity, privacy, vulnerabilities, breaches, encryption, or password managers?",
			trueCrit:  "Covers hacks, breaches, CVEs, infosec, auth hardening, Vaultwarden, or privacy/security.",
			falseCrit: "Not focused on security, privacy, or infosec.",
		},
		"rust": {
			instr:     "Is this bookmark specifically about the Rust programming language or Rust ecosystem?",
			trueCrit:  "Mentions Rust language, Cargo, crates.io, Tokio, Axum, or Rust tooling.",
			falseCrit: "Not about the Rust programming language.",
		},
		"sports": {
			instr:     "Is this bookmark about sports, motorsports (F1/MotoGP), football, basketball, or athletic competitions?",
			trueCrit:  "Covers Formula 1, football, badminton, NBA, or other competitive sports.",
			falseCrit: "Not about sports or athletic events.",
		},
		"golang": {
			instr:     "Is this bookmark specifically about the Go (Golang) programming language or Go ecosystem?",
			trueCrit:  "Mentions Go/Golang, Go standard library, goroutines, or Go libraries/tooling.",
			falseCrit: "Not about the Go programming language.",
		},
	}

	out := make(map[string]noulQuestion, len(Vocabulary))
	for _, tag := range Vocabulary {
		s := specs[tag]
		out[tag] = noulQuestion{
			Type:         "noul",
			Instructions: s.instr,
			Criteria: noulCriteria{
				True:  s.trueCrit,
				False: s.falseCrit,
			},
		}
	}
	return out
}
