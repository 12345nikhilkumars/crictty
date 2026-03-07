package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/12345nikhilkumars/crictui/internal/cache"
	"github.com/12345nikhilkumars/crictui/internal/cricbuzz"
	"github.com/12345nikhilkumars/crictui/internal/models"
)

const defaultCommentaryTTL = 8 * time.Second

type matchDataClient interface {
	GetLiveMatchSections() ([]models.MatchSection, error)
	GetMatchInfo(matchID uint32) (models.MatchInfo, error)
	GetMatchInfoWithContext(ctx context.Context, matchID uint32) (models.MatchInfo, error)
	GetOverSummaries(matchID uint32) (map[uint32][]models.OverSummary, error)
	GetOverSummariesWithContext(ctx context.Context, matchID uint32) (map[uint32][]models.OverSummary, error)
	GetFullCommentary(matchID, inningsID uint32) ([]models.CommentaryEntry, error)
	GetFullCommentaryWithContext(ctx context.Context, matchID, inningsID uint32) ([]models.CommentaryEntry, error)
}

type commentaryCacheKey struct {
	matchID   uint32
	inningsID uint32
}

type commentaryCacheEntry struct {
	entries   []models.CommentaryEntry
	expiresAt time.Time
}

type App struct {
	client   matchDataClient
	cache    *cache.Cache
	Sections []models.MatchSection

	commentaryTTL   time.Duration
	commentaryMu    sync.RWMutex
	commentaryCache map[commentaryCacheKey]commentaryCacheEntry
}

func New() (*App, error) {
	client := cricbuzz.NewClient()
	sections, err := client.GetLiveMatchSections()
	if err != nil {
		return nil, fmt.Errorf("failed to get live match list: %v", err)
	}
	c, err := cache.New()
	if err != nil {
		return nil, fmt.Errorf("failed to init cache: %v", err)
	}
	return &App{
		client:          client,
		cache:           c,
		Sections:        sections,
		commentaryTTL:   defaultCommentaryTTL,
		commentaryCache: make(map[commentaryCacheKey]commentaryCacheEntry),
	}, nil
}

func NewWithMatchID(matchID uint32) (*App, error) {
	client := cricbuzz.NewClient()
	info, err := client.GetMatchInfo(matchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get match info: %v", err)
	}
	shortName := fmt.Sprintf("%s vs %s",
		info.CricbuzzInfo.MatchHeader.Team1.ShortName,
		info.CricbuzzInfo.MatchHeader.Team2.ShortName)
	c, err := cache.New()
	if err != nil {
		return nil, fmt.Errorf("failed to init cache: %v", err)
	}
	format := info.CricbuzzInfo.MatchHeader.MatchFormat
	if format == "" {
		format = "-"
	} else {
		format = normalizeFormat(format)
	}
	matchType := "Domestic"
	if !info.CricbuzzInfo.MatchHeader.Domestic {
		matchType = "Intl"
	}
	miniScore := "-"
	if len(info.CricbuzzInfo.Miniscore.MatchScoreDetails.InningsScoreList) > 0 {
		inn := info.CricbuzzInfo.Miniscore.MatchScoreDetails.InningsScoreList[len(info.CricbuzzInfo.Miniscore.MatchScoreDetails.InningsScoreList)-1]
		miniScore = fmt.Sprintf("%d/%d", inn.Score, inn.Wickets)
	}
	return &App{
		client: client,
		cache:  c,
		Sections: []models.MatchSection{{
			Name: info.CricbuzzInfo.MatchHeader.SeriesName,
			Matches: []models.MatchListItem{{
				MatchID:     matchID,
				ShortName:   shortName,
				SectionName: info.CricbuzzInfo.MatchHeader.SeriesName,
				Format:      format,
				MatchType:   matchType,
				MiniScore:   miniScore,
			}},
		}},
		commentaryTTL:   defaultCommentaryTTL,
		commentaryCache: make(map[commentaryCacheKey]commentaryCacheEntry),
	}, nil
}

func applyMatchRuntimeFields(info models.MatchInfo, overSummaries map[uint32][]models.OverSummary, updatedAt time.Time) models.MatchInfo {
	info.OverSummaries = overSummaries
	info.LastUpdated = updatedAt
	return info
}

func normalizeFormat(f string) string {
	switch strings.ToUpper(strings.TrimSpace(f)) {
	case "TEST":
		return "Test"
	case "ODI":
		return "ODI"
	case "T20", "T20I":
		return "T20"
	case "FC", "FIRST CLASS", "FIRST-CLASS":
		// First-class multi-day games should appear under Test-style format
		return "Test"
	case "LIST A", "LISTA":
		// List A one-day games should appear under ODI-style format
		return "ODI"
	default:
		return f
	}
}

func (a *App) Close() error {
	if a.cache != nil {
		return a.cache.Close()
	}
	return nil
}

func (a *App) TotalMatches() int {
	n := 0
	for _, s := range a.Sections {
		n += len(s.Matches)
	}
	return n
}

func (a *App) MatchAtFlatIndex(idx int) (models.MatchListItem, bool) {
	if idx < 0 {
		return models.MatchListItem{}, false
	}
	for _, s := range a.Sections {
		if idx < len(s.Matches) {
			return s.Matches[idx], true
		}
		idx -= len(s.Matches)
	}
	return models.MatchListItem{}, false
}

// LoadMatch fetches match info, scorecard, and over summaries (with cache).
func (a *App) LoadMatch(matchID uint32) (models.MatchInfo, error) {
	return a.LoadMatchWithContext(context.Background(), matchID)
}

// RefreshMatch re-fetches live data for the currently viewed match.
func (a *App) RefreshMatch(matchID uint32) (models.MatchInfo, error) {
	return a.RefreshMatchWithContext(context.Background(), matchID)
}

// LoadMatchWithContext fetches match info with context cancellation support.
func (a *App) LoadMatchWithContext(ctx context.Context, matchID uint32) (models.MatchInfo, error) {
	return a.loadMatchInfoWithContext(ctx, matchID)
}

// RefreshMatchWithContext re-fetches live data with context cancellation support.
func (a *App) RefreshMatchWithContext(ctx context.Context, matchID uint32) (models.MatchInfo, error) {
	return a.loadMatchInfoWithContext(ctx, matchID)
}

func (a *App) loadMatchInfoWithContext(ctx context.Context, matchID uint32) (models.MatchInfo, error) {
	if a.client == nil {
		return models.MatchInfo{}, fmt.Errorf("match client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	info, err := a.client.GetMatchInfoWithContext(ctx, matchID)
	if err != nil {
		return models.MatchInfo{}, err
	}

	return applyMatchRuntimeFields(info, a.loadOverSummariesWithContext(ctx, matchID), time.Now()), nil
}

// LoadCommentary fetches full commentary for a specific innings.
func (a *App) LoadCommentary(matchID, inningsID uint32) ([]models.CommentaryEntry, error) {
	return a.LoadCommentaryWithContext(context.Background(), matchID, inningsID)
}

// LoadCommentaryWithContext fetches full commentary with short-lived caching.
func (a *App) LoadCommentaryWithContext(ctx context.Context, matchID, inningsID uint32) ([]models.CommentaryEntry, error) {
	if a.client == nil {
		return nil, fmt.Errorf("match client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	key := commentaryCacheKey{matchID: matchID, inningsID: inningsID}
	now := time.Now()
	if entries, ok := a.getCachedCommentary(key, now); ok {
		return entries, nil
	}

	entries, err := a.client.GetFullCommentaryWithContext(ctx, matchID, inningsID)
	if err != nil {
		return nil, err
	}
	if a.commentaryTTL > 0 {
		a.setCachedCommentary(key, entries, now.Add(a.commentaryTTL))
	}
	return cloneCommentaryEntries(entries), nil
}

// loadOverSummaries fetches over summaries, using cache for completed innings.
func (a *App) loadOverSummaries(matchID uint32) map[uint32][]models.OverSummary {
	return a.loadOverSummariesWithContext(context.Background(), matchID)
}

func (a *App) loadOverSummariesWithContext(ctx context.Context, matchID uint32) map[uint32][]models.OverSummary {
	if a.client == nil {
		return map[uint32][]models.OverSummary{}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	fresh, err := a.client.GetOverSummariesWithContext(ctx, matchID)
	if err != nil {
		// Fall back to whatever is in cache
		result := make(map[uint32][]models.OverSummary)
		if a.cache == nil {
			return result
		}
		for _, innID := range []uint32{1, 2, 3, 4} {
			if cached, ok := a.cache.GetOvers(matchID, innID); ok {
				result[innID] = cached
			}
		}
		return result
	}

	// Merge: prefer fresh data, but also pull any cached innings not in fresh
	if a.cache == nil {
		return fresh
	}
	for _, innID := range []uint32{1, 2, 3, 4} {
		if overs, ok := fresh[innID]; ok && len(overs) > 0 {
			_ = a.cache.PutOvers(matchID, innID, overs)
		} else if cached, ok := a.cache.GetOvers(matchID, innID); ok {
			fresh[innID] = cached
		}
	}

	return fresh
}

func cloneCommentaryEntries(entries []models.CommentaryEntry) []models.CommentaryEntry {
	if len(entries) == 0 {
		return nil
	}
	cloned := make([]models.CommentaryEntry, len(entries))
	copy(cloned, entries)
	return cloned
}

func (a *App) getCachedCommentary(key commentaryCacheKey, now time.Time) ([]models.CommentaryEntry, bool) {
	a.commentaryMu.RLock()
	entry, ok := a.commentaryCache[key]
	a.commentaryMu.RUnlock()
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		a.commentaryMu.Lock()
		if cur, exists := a.commentaryCache[key]; exists && now.After(cur.expiresAt) {
			delete(a.commentaryCache, key)
		}
		a.commentaryMu.Unlock()
		return nil, false
	}
	return cloneCommentaryEntries(entry.entries), true
}

func (a *App) setCachedCommentary(key commentaryCacheKey, entries []models.CommentaryEntry, expiresAt time.Time) {
	a.commentaryMu.Lock()
	a.commentaryCache[key] = commentaryCacheEntry{entries: cloneCommentaryEntries(entries), expiresAt: expiresAt}
	a.commentaryMu.Unlock()
}
