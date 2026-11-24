package tagging

import (
	"context"
	"log"
	"strings"
	"sync"

	"social-vault/internal/db"
)

const (
	DefaultBackfillLimit = 50
	MaxBackfillLimit     = 500
)

// BackfillResult summarizes one POST /api/tagging/backfill run.
type BackfillResult struct {
	Processed   int `json:"processed"`
	RulesTagged int `json:"rules_tagged"`
	JevTagged   int `json:"jev_tagged"`
	Queued      int `json:"queued"`
	Skipped     int `json:"skipped"`
	Failed      int `json:"failed"`
}

// Service coordinates Layer 1 rules, Layer 2 Jev classification, and Layer 3 review queueing.
type Service struct {
	store *db.Store
	jev   *JevClient
	wg    sync.WaitGroup
}

// NewService constructs a tagging Service. jev may be nil when JEV_API_KEY is unset.
func NewService(store *db.Store, jev *JevClient) *Service {
	return &Service{
		store: store,
		jev:   jev,
	}
}

// MatchRules executes Layer 1 keyword rules synchronously.
func (s *Service) MatchRules(in Input) []string {
	return MatchRules(in)
}

// JevEnabled reports whether Layer 2 (Jev) is configured and active.
func (s *Service) JevEnabled() bool {
	return s != nil && s.jev != nil && s.jev.Enabled()
}

// Wait blocks until all in-flight background Layer 2 goroutines finish.
func (s *Service) Wait() {
	if s == nil {
		return
	}
	s.wg.Wait()
}

// RunJevAsync triggers Layer 2 Jev classification in a background goroutine
// with a 15-second timeout. Errors are logged and never affect bookmark saves.
func (s *Service) RunJevAsync(b *db.Bookmark) {
	if !s.JevEnabled() || b == nil {
		return
	}

	bookmarkCopy := *b
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), DefaultJevTimeout)
		defer cancel()

		if _, _, err := s.RunJevForBookmark(ctx, &bookmarkCopy); err != nil {
			log.Printf("tagging: layer 2 jev failed for bookmark %d: %v", bookmarkCopy.ID, err)
		}
	}()
}

// RunJevForBookmark classifies one bookmark via Jev, merges >=0.8 tags, and
// enqueues [0.5, 0.8) tags into the review queue.
func (s *Service) RunJevForBookmark(ctx context.Context, b *db.Bookmark) (autoApplied int, queued bool, err error) {
	if !s.JevEnabled() || b == nil {
		return 0, false, nil
	}

	res, err := s.jev.Classify(ctx, Input{
		URL:         b.URL,
		Title:       b.Title,
		Description: b.Description,
		SiteName:    b.SiteName,
	})
	if err != nil {
		return 0, false, err
	}

	current, err := s.store.GetBookmark(ctx, b.ID)
	if err != nil {
		return 0, false, err
	}

	existing := bookmarkTagNames(current)
	existingSet := make(map[string]struct{}, len(existing))
	for _, t := range existing {
		existingSet[strings.ToLower(t)] = struct{}{}
	}

	if len(res.AutoTags) > 0 {
		merged := MergeTags(existing, res.AutoTags)
		if len(merged) > len(existing) {
			if _, err := s.store.UpdateBookmark(ctx, b.ID, db.UpdateBookmarkInput{Tags: &merged}); err != nil {
				return 0, false, err
			}
			autoApplied = len(merged) - len(existing)
			for _, t := range merged {
				existingSet[strings.ToLower(t)] = struct{}{}
			}
		}
	}

	var remainingReview []string
	remainingScores := make(map[string]float64)
	for _, t := range res.ReviewTags {
		if _, already := existingSet[strings.ToLower(t)]; already {
			continue
		}
		remainingReview = append(remainingReview, t)
		if sc, ok := res.ReviewScores[t]; ok {
			remainingScores[t] = sc
		}
	}

	if len(remainingReview) > 0 {
		_, created, err := s.store.EnqueueTagReview(ctx, b.ID, remainingReview, remainingScores, "jev")
		if err != nil {
			return autoApplied, false, err
		}
		queued = created
	}

	return autoApplied, queued, nil
}

// Backfill runs Layer 1 rules and (if configured) Layer 2 Jev over untagged bookmarks
// that do not already have a pending review queue row.
func (s *Service) Backfill(ctx context.Context, limit int) (BackfillResult, error) {
	if limit <= 0 {
		limit = DefaultBackfillLimit
	}
	if limit > MaxBackfillLimit {
		limit = MaxBackfillLimit
	}

	bookmarks, err := s.store.ListUntaggedBookmarksForBackfill(ctx, limit)
	if err != nil {
		return BackfillResult{}, err
	}

	var out BackfillResult
	for i := range bookmarks {
		b := &bookmarks[i]
		out.Processed++

		ruleMatched := false
		ruleTags := s.MatchRules(Input{
			URL:         b.URL,
			Title:       b.Title,
			Description: b.Description,
			SiteName:    b.SiteName,
		})
		if len(ruleTags) > 0 {
			merged := MergeTags(bookmarkTagNames(b), ruleTags)
			updated, uerr := s.store.UpdateBookmark(ctx, b.ID, db.UpdateBookmarkInput{Tags: &merged})
			if uerr != nil {
				log.Printf("tagging: backfill rule update failed for bookmark %d: %v", b.ID, uerr)
				out.Failed++
				continue
			}
			b = updated
			out.RulesTagged++
			ruleMatched = true
		}

		jevApplied := 0
		jevQueued := false
		if s.JevEnabled() {
			applied, queued, jerr := s.RunJevForBookmark(ctx, b)
			if jerr != nil {
				log.Printf("tagging: backfill jev failed for bookmark %d: %v", b.ID, jerr)
				out.Failed++
				continue
			}
			if applied > 0 {
				out.JevTagged++
				jevApplied = applied
			}
			if queued {
				out.Queued++
				jevQueued = true
			}
		}

		if !ruleMatched && jevApplied == 0 && !jevQueued {
			out.Skipped++
		}
	}

	return out, nil
}

func bookmarkTagNames(b *db.Bookmark) []string {
	if b == nil || len(b.Tags) == 0 {
		return nil
	}
	names := make([]string, 0, len(b.Tags))
	for _, t := range b.Tags {
		names = append(names, t.Name)
	}
	return names
}
