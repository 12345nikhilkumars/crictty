package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/12345nikhilkumars/crictui/internal/cache"
	"github.com/12345nikhilkumars/crictui/internal/cricbuzz"
	"github.com/12345nikhilkumars/crictui/internal/models"
)

type App struct {
	client   *cricbuzz.Client
	cache    *cache.Cache
	Sections []models.MatchSection
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
	return &App{client: client, cache: c, Sections: sections}, nil
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
				MatchID:      matchID,
				ShortName:   shortName,
				SectionName: info.CricbuzzInfo.MatchHeader.SeriesName,
				Format:      format,
				MatchType:   matchType,
				MiniScore:   miniScore,
			}},
		}},
	}, nil
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
	info, err := a.client.GetMatchInfo(matchID)
	if err != nil {
		return models.MatchInfo{}, err
	}

	info.OverSummaries = a.loadOverSummaries(matchID)
	info.LastUpdated = time.Now()
	return info, nil
}

// RefreshMatch re-fetches live data for the currently viewed match.
func (a *App) RefreshMatch(matchID uint32) (models.MatchInfo, error) {
	info, err := a.client.GetMatchInfo(matchID)
	if err != nil {
		return models.MatchInfo{}, err
	}

	info.OverSummaries = a.loadOverSummaries(matchID)
	info.LastUpdated = time.Now()
	return info, nil
}

// LoadCommentary fetches full commentary for a specific innings.
func (a *App) LoadCommentary(matchID, inningsID uint32) ([]models.CommentaryEntry, error) {
	return a.client.GetFullCommentary(matchID, inningsID)
}

// loadOverSummaries fetches over summaries, using cache for completed innings.
func (a *App) loadOverSummaries(matchID uint32) map[uint32][]models.OverSummary {
	fresh, err := a.client.GetOverSummaries(matchID)
	if err != nil {
		// Fall back to whatever is in cache
		result := make(map[uint32][]models.OverSummary)
		for _, innID := range []uint32{1, 2, 3, 4} {
			if cached, ok := a.cache.GetOvers(matchID, innID); ok {
				result[innID] = cached
			}
		}
		return result
	}

	// Merge: prefer fresh data, but also pull any cached innings not in fresh
	for _, innID := range []uint32{1, 2, 3, 4} {
		if overs, ok := fresh[innID]; ok && len(overs) > 0 {
			_ = a.cache.PutOvers(matchID, innID, overs)
		} else if cached, ok := a.cache.GetOvers(matchID, innID); ok {
			fresh[innID] = cached
		}
	}

	return fresh
}
