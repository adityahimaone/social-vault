package tagging

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"social-vault/internal/db"
)

const (
	DefaultBackfillLimit = 50
	MaxBackfillLimit     = 500
	maxShortJevErrorLen  = 160

	LayerRules = "rules"
	LayerJev   = "jev"
)

var (
	ErrBackfillRunning = errors.New("Backfill sedang berjalan")
	ErrInvalidLayer    = errors.New("invalid backfill layer")
	ErrJevDisabled     = errors.New("Jev tidak aktif (JEV_API_KEY kosong)")
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

// SuggestResult summarizes one POST /api/tagging/suggest-untagged run.
type SuggestResult struct {
	Processed int                `json:"processed"`
	Queued    int                `json:"queued"`
	Skipped   int                `json:"skipped"`
	Failed    int                `json:"failed"`
	Items     []db.TagReviewItem `json:"items"`
}

// ActiveJevJob describes one in-flight background Layer 2 Jev classification job.
type ActiveJevJob struct {
	BookmarkID int64  `json:"bookmark_id"`
	StartedAt  string `json:"started_at"`
}

// JevEvent describes the most recently completed async Layer 2 Jev job.
type JevEvent struct {
	Seq         uint64   `json:"seq"`
	BookmarkID  int64    `json:"bookmark_id"`
	AppliedTags []string `json:"applied_tags"`
	Queued      bool     `json:"queued"`
	Error       string   `json:"error"`
	FinishedAt  string   `json:"finished_at"`
}

// BackfillStatus describes the state and latest outcome of Service.Backfill.
type BackfillStatus struct {
	Running    bool            `json:"running"`
	StartedAt  string          `json:"started_at"`
	FinishedAt string          `json:"finished_at"`
	Result     *BackfillResult `json:"result"`
}

// Status is the snapshot returned by GET /api/tagging/status.
type Status struct {
	JevEnabled    bool           `json:"jev_enabled"`
	ActiveJevJobs []ActiveJevJob `json:"active_jev_jobs"`
	LastJevEvent  *JevEvent      `json:"last_jev_event"`
	LastBackfill  BackfillStatus `json:"last_backfill"`
	PendingReview int            `json:"pending_review"`
	UntaggedCount int            `json:"untagged_count"`
}

// Service coordinates Layer 1 rules, Layer 2 Jev classification, and Layer 3 review queueing.
type Service struct {
	store *db.Store
	jev   *JevClient
	wg    sync.WaitGroup

	mu           sync.Mutex
	nextJobID    uint64
	activeJobs   map[uint64]ActiveJevJob
	jevSeq       uint64
	lastJevEvent *JevEvent
	lastBackfill BackfillStatus
}

// NewService constructs a tagging Service. jev may be nil when JEV_API_KEY is unset.
func NewService(store *db.Store, jev *JevClient) *Service {
	return &Service{
		store:      store,
		jev:        jev,
		activeJobs: make(map[uint64]ActiveJevJob),
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

// Status returns a live snapshot of background Jev jobs, the latest async Jev outcome,
// backfill status, and pending review queue count. It never blocks on Jev.
func (s *Service) Status(ctx context.Context) (Status, error) {
	s.mu.Lock()
	keys := make([]uint64, 0, len(s.activeJobs))
	for k := range s.activeJobs {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	active := make([]ActiveJevJob, 0, len(keys))
	for _, k := range keys {
		active = append(active, s.activeJobs[k])
	}

	var lastEvent *JevEvent
	if s.lastJevEvent != nil {
		evCopy := *s.lastJevEvent
		evCopy.AppliedTags = make([]string, len(s.lastJevEvent.AppliedTags))
		copy(evCopy.AppliedTags, s.lastJevEvent.AppliedTags)
		lastEvent = &evCopy
	}

	lastBackfill := s.lastBackfill
	if s.lastBackfill.Result != nil {
		resCopy := *s.lastBackfill.Result
		lastBackfill.Result = &resCopy
	}
	s.mu.Unlock()

	pending, err := s.store.CountPendingTagReviews(ctx)
	if err != nil {
		return Status{}, err
	}

	untagged, err := s.store.CountUntaggedBookmarks(ctx)
	if err != nil {
		return Status{}, err
	}

	return Status{
		JevEnabled:    s.JevEnabled(),
		ActiveJevJobs: active,
		LastJevEvent:  lastEvent,
		LastBackfill:  lastBackfill,
		PendingReview: pending,
		UntaggedCount: untagged,
	}, nil
}

// RunJevAsync triggers Layer 2 Jev classification in a background goroutine
// with a 15-second timeout. Errors are logged and never affect bookmark saves.
func (s *Service) RunJevAsync(b *db.Bookmark) {
	if !s.JevEnabled() || b == nil {
		return
	}

	bookmarkCopy := *b

	s.mu.Lock()
	s.nextJobID++
	jobID := s.nextJobID
	if s.activeJobs == nil {
		s.activeJobs = make(map[uint64]ActiveJevJob)
	}
	s.activeJobs[jobID] = ActiveJevJob{
		BookmarkID: bookmarkCopy.ID,
		StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		var (
			appliedTags []string
			queued      bool
			runErr      error
		)
		defer func() {
			tagsCopy := make([]string, len(appliedTags))
			copy(tagsCopy, appliedTags)

			s.mu.Lock()
			delete(s.activeJobs, jobID)
			s.jevSeq++
			s.lastJevEvent = &JevEvent{
				Seq:         s.jevSeq,
				BookmarkID:  bookmarkCopy.ID,
				AppliedTags: tagsCopy,
				Queued:      queued,
				Error:       shortJevError(runErr),
				FinishedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			}
			s.mu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), DefaultJevTimeout)
		defer cancel()

		appliedTags, queued, runErr = s.runJevForBookmark(ctx, &bookmarkCopy)
		if runErr != nil {
			log.Printf("tagging: layer 2 jev failed for bookmark %d: %v", bookmarkCopy.ID, runErr)
		}
	}()
}

// RunJevForBookmark classifies one bookmark via Jev, merges >=0.8 tags, and
// enqueues [0.5, 0.8) tags into the review queue.
func (s *Service) RunJevForBookmark(ctx context.Context, b *db.Bookmark) (autoApplied int, queued bool, err error) {
	appliedTags, queued, err := s.runJevForBookmark(ctx, b)
	return len(appliedTags), queued, err
}

func (s *Service) runJevForBookmark(ctx context.Context, b *db.Bookmark) (appliedTags []string, queued bool, err error) {
	if !s.JevEnabled() || b == nil {
		return nil, false, nil
	}

	res, err := s.jev.Classify(ctx, Input{
		URL:         b.URL,
		Title:       b.Title,
		Description: b.Description,
		SiteName:    b.SiteName,
	})
	if err != nil {
		return nil, false, err
	}

	current, err := s.store.GetBookmark(ctx, b.ID)
	if err != nil {
		return nil, false, err
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
				return nil, false, err
			}
			for _, t := range merged {
				lower := strings.ToLower(t)
				if _, already := existingSet[lower]; !already {
					appliedTags = append(appliedTags, t)
					existingSet[lower] = struct{}{}
				}
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
			return appliedTags, false, err
		}
		queued = created
	}

	return appliedTags, queued, nil
}

func resolveBackfillLayers(layers []string, jevEnabled bool) (runRules, runJev bool, err error) {
	if len(layers) == 0 {
		return true, jevEnabled, nil
	}
	for _, raw := range layers {
		switch raw {
		case LayerRules:
			runRules = true
		case LayerJev:
			runJev = true
		default:
			return false, false, fmt.Errorf("%w: %q (allowed: rules, jev)", ErrInvalidLayer, raw)
		}
	}
	if runJev && !jevEnabled {
		return false, false, ErrJevDisabled
	}
	return runRules, runJev, nil
}

// Backfill runs selected tagging layers ("rules", "jev", or both when omitted) over
// untagged bookmarks that do not already have a review queue row (pending, applied, or dismissed).
func (s *Service) Backfill(ctx context.Context, limit int, layers ...string) (out BackfillResult, err error) {
	runRules, runJev, err := resolveBackfillLayers(layers, s.JevEnabled())
	if err != nil {
		return BackfillResult{}, err
	}

	s.mu.Lock()
	if s.lastBackfill.Running {
		s.mu.Unlock()
		return BackfillResult{}, ErrBackfillRunning
	}
	s.lastBackfill.Running = true
	s.lastBackfill.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.lastBackfill.FinishedAt = ""
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.lastBackfill.Running = false
		s.lastBackfill.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err == nil {
			resCopy := out
			s.lastBackfill.Result = &resCopy
		}
		s.mu.Unlock()
	}()

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

	for i := range bookmarks {
		b := &bookmarks[i]
		out.Processed++

		ruleMatched := false
		if runRules {
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
		}

		jevApplied := 0
		jevQueued := false
		if runJev && s.JevEnabled() {
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

// SuggestUntagged runs Jev over untagged bookmarks without pending review rows
// and enqueues all suggestions >= JevReviewThreshold (0.5) into tag_review_queue
// without auto-applying any tags to bookmarks.
func (s *Service) SuggestUntagged(ctx context.Context, limit int) (SuggestResult, error) {
	if !s.JevEnabled() {
		return SuggestResult{}, ErrJevDisabled
	}

	s.mu.Lock()
	if s.lastBackfill.Running {
		s.mu.Unlock()
		return SuggestResult{}, ErrBackfillRunning
	}
	s.lastBackfill.Running = true
	s.lastBackfill.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	s.lastBackfill.FinishedAt = ""
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.lastBackfill.Running = false
		s.lastBackfill.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		s.mu.Unlock()
	}()

	if limit <= 0 {
		limit = 100
	}
	if limit > MaxBackfillLimit {
		limit = MaxBackfillLimit
	}

	bookmarks, err := s.store.ListUntaggedBookmarksWithoutPendingReview(ctx, limit)
	if err != nil {
		return SuggestResult{}, err
	}

	out := SuggestResult{
		Items: make([]db.TagReviewItem, 0),
	}

	for i := range bookmarks {
		b := &bookmarks[i]
		out.Processed++

		res, jerr := s.jev.Classify(ctx, Input{
			URL:         b.URL,
			Title:       b.Title,
			Description: b.Description,
			SiteName:    b.SiteName,
		})
		if jerr != nil {
			log.Printf("tagging: suggest-untagged jev failed for bookmark %d: %v", b.ID, jerr)
			out.Failed++
			continue
		}

		suggested := make([]string, 0)
		scores := make(map[string]float64)
		for tag, score := range res.AllScores {
			if score >= JevReviewThreshold {
				suggested = append(suggested, tag)
				scores[tag] = score
			}
		}

		if len(suggested) == 0 {
			out.Skipped++
			continue
		}

		sort.Slice(suggested, func(a, b int) bool {
			sa, sb := scores[suggested[a]], scores[suggested[b]]
			if sa != sb {
				return sa > sb
			}
			return suggested[a] < suggested[b]
		})

		item, created, qerr := s.store.EnqueueTagReview(ctx, b.ID, suggested, scores, "jev")
		if qerr != nil {
			log.Printf("tagging: suggest-untagged enqueue failed for bookmark %d: %v", b.ID, qerr)
			out.Failed++
			continue
		}
		if created && item != nil {
			out.Queued++
			out.Items = append(out.Items, *item)
		} else {
			out.Skipped++
		}
	}

	return out, nil
}

func shortJevError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if len(msg) > maxShortJevErrorLen {
		return msg[:maxShortJevErrorLen-3] + "..."
	}
	return msg
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
