package app

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/12345nikhilkumars/crictui/internal/models"
)

type mockMatchClient struct {
	getLiveMatchSectionsFn     func() ([]models.MatchSection, error)
	getMatchInfoFn             func(matchID uint32) (models.MatchInfo, error)
	getMatchInfoWithContextFn  func(ctx context.Context, matchID uint32) (models.MatchInfo, error)
	getOverSummariesFn         func(matchID uint32) (map[uint32][]models.OverSummary, error)
	getOverSummariesWithCtxFn  func(ctx context.Context, matchID uint32) (map[uint32][]models.OverSummary, error)
	getFullCommentaryFn        func(matchID, inningsID uint32) ([]models.CommentaryEntry, error)
	getFullCommentaryWithCtxFn func(ctx context.Context, matchID, inningsID uint32) ([]models.CommentaryEntry, error)
}

func (m *mockMatchClient) GetLiveMatchSections() ([]models.MatchSection, error) {
	if m.getLiveMatchSectionsFn != nil {
		return m.getLiveMatchSectionsFn()
	}
	return nil, nil
}

func (m *mockMatchClient) GetMatchInfo(matchID uint32) (models.MatchInfo, error) {
	if m.getMatchInfoFn != nil {
		return m.getMatchInfoFn(matchID)
	}
	if m.getMatchInfoWithContextFn != nil {
		return m.getMatchInfoWithContextFn(context.Background(), matchID)
	}
	return models.MatchInfo{}, nil
}

func (m *mockMatchClient) GetMatchInfoWithContext(ctx context.Context, matchID uint32) (models.MatchInfo, error) {
	if m.getMatchInfoWithContextFn != nil {
		return m.getMatchInfoWithContextFn(ctx, matchID)
	}
	if m.getMatchInfoFn != nil {
		return m.getMatchInfoFn(matchID)
	}
	return models.MatchInfo{}, nil
}

func (m *mockMatchClient) GetOverSummaries(matchID uint32) (map[uint32][]models.OverSummary, error) {
	if m.getOverSummariesFn != nil {
		return m.getOverSummariesFn(matchID)
	}
	if m.getOverSummariesWithCtxFn != nil {
		return m.getOverSummariesWithCtxFn(context.Background(), matchID)
	}
	return map[uint32][]models.OverSummary{}, nil
}

func (m *mockMatchClient) GetOverSummariesWithContext(ctx context.Context, matchID uint32) (map[uint32][]models.OverSummary, error) {
	if m.getOverSummariesWithCtxFn != nil {
		return m.getOverSummariesWithCtxFn(ctx, matchID)
	}
	if m.getOverSummariesFn != nil {
		return m.getOverSummariesFn(matchID)
	}
	return map[uint32][]models.OverSummary{}, nil
}

func (m *mockMatchClient) GetFullCommentary(matchID, inningsID uint32) ([]models.CommentaryEntry, error) {
	if m.getFullCommentaryFn != nil {
		return m.getFullCommentaryFn(matchID, inningsID)
	}
	if m.getFullCommentaryWithCtxFn != nil {
		return m.getFullCommentaryWithCtxFn(context.Background(), matchID, inningsID)
	}
	return nil, nil
}

func (m *mockMatchClient) GetFullCommentaryWithContext(ctx context.Context, matchID, inningsID uint32) ([]models.CommentaryEntry, error) {
	if m.getFullCommentaryWithCtxFn != nil {
		return m.getFullCommentaryWithCtxFn(ctx, matchID, inningsID)
	}
	if m.getFullCommentaryFn != nil {
		return m.getFullCommentaryFn(matchID, inningsID)
	}
	return nil, nil
}

func TestNormalizeFormat(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "test", in: "TEST", want: "Test"},
		{name: "odi", in: "ODI", want: "ODI"},
		{name: "t20i", in: " t20i ", want: "T20"},
		{name: "first class", in: "First Class", want: "Test"},
		{name: "list a", in: "LIST A", want: "ODI"},
		{name: "unknown unchanged", in: "The Hundred", want: "The Hundred"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFormat(tt.in)
			if got != tt.want {
				t.Fatalf("normalizeFormat(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestApplyMatchRuntimeFields(t *testing.T) {
	original := models.MatchInfo{
		MatchShortName:  "IND vs AUS",
		CricbuzzMatchID: 123,
	}
	overs := map[uint32][]models.OverSummary{
		1: {{InningsID: 1, Overs: 10, Score: 88, Wickets: 2}},
	}
	updatedAt := time.Date(2026, time.March, 7, 10, 30, 0, 0, time.UTC)

	got := applyMatchRuntimeFields(original, overs, updatedAt)

	if got.LastUpdated != updatedAt {
		t.Fatalf("LastUpdated = %v, want %v", got.LastUpdated, updatedAt)
	}
	if !reflect.DeepEqual(got.OverSummaries, overs) {
		t.Fatalf("OverSummaries = %#v, want %#v", got.OverSummaries, overs)
	}
	if got.MatchShortName != original.MatchShortName || got.CricbuzzMatchID != original.CricbuzzMatchID {
		t.Fatalf("non-runtime fields changed: got %#v original %#v", got, original)
	}

	if original.LastUpdated != (time.Time{}) {
		t.Fatalf("original LastUpdated mutated: %v", original.LastUpdated)
	}
	if original.OverSummaries != nil {
		t.Fatalf("original OverSummaries mutated: %#v", original.OverSummaries)
	}
}

func TestLoadCommentaryWithContext_UsesTTLCache(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)

	mock := &mockMatchClient{
		getFullCommentaryWithCtxFn: func(ctx context.Context, matchID, inningsID uint32) ([]models.CommentaryEntry, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return []models.CommentaryEntry{{CommText: "first-ball", InningsID: inningsID}}, nil
		},
	}

	a := &App{
		client:          mock,
		commentaryTTL:   40 * time.Millisecond,
		commentaryCache: make(map[commentaryCacheKey]commentaryCacheEntry),
	}

	first, err := a.LoadCommentaryWithContext(context.Background(), 99, 1)
	if err != nil {
		t.Fatalf("first load failed: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("expected one entry, got %d", len(first))
	}

	first[0].CommText = "mutated"

	second, err := a.LoadCommentaryWithContext(context.Background(), 99, 1)
	if err != nil {
		t.Fatalf("second load failed: %v", err)
	}
	if len(second) != 1 || second[0].CommText != "first-ball" {
		t.Fatalf("expected cached copy with original text, got %#v", second)
	}

	mu.Lock()
	if calls != 1 {
		t.Fatalf("expected one upstream call within TTL, got %d", calls)
	}
	mu.Unlock()

	time.Sleep(55 * time.Millisecond)

	_, err = a.LoadCommentaryWithContext(context.Background(), 99, 1)
	if err != nil {
		t.Fatalf("third load after ttl failed: %v", err)
	}

	mu.Lock()
	if calls != 2 {
		t.Fatalf("expected second upstream call after TTL expiry, got %d", calls)
	}
	mu.Unlock()
}

func TestLoadCommentaryWithContext_RespectsCancellation(t *testing.T) {
	mock := &mockMatchClient{
		getFullCommentaryWithCtxFn: func(ctx context.Context, matchID, inningsID uint32) ([]models.CommentaryEntry, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	a := &App{
		client:          mock,
		commentaryTTL:   defaultCommentaryTTL,
		commentaryCache: make(map[commentaryCacheKey]commentaryCacheEntry),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := a.LoadCommentaryWithContext(ctx, 42, 2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}

func TestLoadMatchWithContext_RespectsCancellation(t *testing.T) {
	mock := &mockMatchClient{
		getMatchInfoWithContextFn: func(ctx context.Context, matchID uint32) (models.MatchInfo, error) {
			<-ctx.Done()
			return models.MatchInfo{}, ctx.Err()
		},
	}

	a := &App{
		client:          mock,
		commentaryTTL:   defaultCommentaryTTL,
		commentaryCache: make(map[commentaryCacheKey]commentaryCacheEntry),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := a.LoadMatchWithContext(ctx, 7)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
}
