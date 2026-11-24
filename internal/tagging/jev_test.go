package tagging_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"social-vault/internal/tagging"
)

func TestJevClientClassifyThresholdsAndPayload(t *testing.T) {
	const stubKey = "stub-jev-secret-token"

	var captured struct {
		Auth      string
		Model     string `json:"model"`
		State     map[string]string
		Questions map[string]struct {
			Type         string            `json:"type"`
			Instructions string            `json:"instructions"`
			Criteria     map[string]string `json:"criteria"`
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		captured.Auth = r.Header.Get("Authorization")

		var body struct {
			Model     string            `json:"model"`
			State     map[string]string `json:"state"`
			Questions map[string]struct {
				Type         string            `json:"type"`
				Instructions string            `json:"instructions"`
				Criteria     map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		captured.Model = body.Model
		captured.State = body.State
		captured.Questions = body.Questions

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "jev-1.13.0",
			"answers": {
				"ai": {"type": "noul", "noul": 0.91},
				"golang": {"type": "noul", "noul": 0.8},
				"security": {"type": "noul", "noul": 0.79},
				"finance": {"type": "noul", "noul": 0.5},
				"news": {"type": "noul", "noul": 0.499},
				"invented_tag": {"type": "noul", "noul": 0.99}
			},
			"usage": {"input_tokens": 1800, "output_tokens": 19}
		}`))
	}))
	defer srv.Close()

	client := tagging.NewJevClientWithOptions(stubKey, srv.URL, nil)
	if !client.Enabled() {
		t.Fatal("expected client.Enabled() == true")
	}

	res, err := client.Classify(context.Background(), tagging.Input{
		URL:         "https://example.com/go-sec",
		Title:       "Go Security Notes",
		Description: "Hardening services",
		SiteName:    "example.com",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}

	if captured.Auth != "Bearer "+stubKey {
		t.Fatalf("unexpected Authorization header %q", captured.Auth)
	}
	if captured.Model != tagging.JevModel {
		t.Fatalf("expected pinned model %q, got %q", tagging.JevModel, captured.Model)
	}
	if len(captured.Questions) != len(tagging.Vocabulary) {
		t.Fatalf("expected %d questions, got %d", len(tagging.Vocabulary), len(captured.Questions))
	}
	for _, tag := range tagging.Vocabulary {
		q, ok := captured.Questions[tag]
		if !ok {
			t.Fatalf("missing noul question for %q", tag)
		}
		if q.Type != "noul" || strings.TrimSpace(q.Instructions) == "" || q.Criteria["true"] == "" || q.Criteria["false"] == "" {
			t.Fatalf("incomplete noul question for %q: %+v", tag, q)
		}
	}
	if captured.State["url"] != "https://example.com/go-sec" || captured.State["title"] != "Go Security Notes" {
		t.Fatalf("unexpected state: %+v", captured.State)
	}

	// Threshold assertions: >=0.8 auto, [0.5,0.8) review, <0.5 ignore, outside vocab ignored
	if !slices.Equal(res.AutoTags, []string{"ai", "golang"}) {
		t.Fatalf("unexpected AutoTags: %v", res.AutoTags)
	}
	if !slices.Equal(res.ReviewTags, []string{"finance", "security"}) {
		t.Fatalf("unexpected ReviewTags: %v", res.ReviewTags)
	}
	if res.ReviewScores["security"] != 0.79 || res.ReviewScores["finance"] != 0.5 {
		t.Fatalf("unexpected ReviewScores: %+v", res.ReviewScores)
	}
	if _, hasInvented := res.AllScores["invented_tag"]; hasInvented {
		t.Fatal("invented_tag must be ignored")
	}
}

func TestJevClientErrorsAndTimeout(t *testing.T) {
	const stubKey = "super-secret-jev-key"

	t.Run("disabled when empty key", func(t *testing.T) {
		c := tagging.NewJevClient("")
		if c.Enabled() {
			t.Fatal("expected Enabled() == false")
		}
		if _, err := c.Classify(context.Background(), tagging.Input{URL: "https://example.com"}); err == nil {
			t.Fatal("expected error when disabled")
		}
	})

	t.Run("non-2xx does not leak key", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"Unknown model"}`, http.StatusBadRequest)
		}))
		defer srv.Close()

		c := tagging.NewJevClientWithOptions(stubKey, srv.URL, nil)
		_, err := c.Classify(context.Background(), tagging.Input{URL: "https://example.com"})
		if err == nil {
			t.Fatal("expected error on 400")
		}
		if strings.Contains(err.Error(), stubKey) {
			t.Fatalf("error leaked API key: %v", err)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{not-json`))
		}))
		defer srv.Close()

		c := tagging.NewJevClientWithOptions(stubKey, srv.URL, nil)
		if _, err := c.Classify(context.Background(), tagging.Input{URL: "https://example.com"}); err == nil {
			t.Fatal("expected error on malformed JSON")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(150 * time.Millisecond)
			_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{"ai":{"type":"noul","noul":0.9}}}`))
		}))
		defer srv.Close()

		hc := &http.Client{Timeout: 30 * time.Millisecond}
		c := tagging.NewJevClientWithOptions(stubKey, srv.URL, hc)
		if _, err := c.Classify(context.Background(), tagging.Input{URL: "https://example.com"}); err == nil {
			t.Fatal("expected timeout error")
		}
	})
}
