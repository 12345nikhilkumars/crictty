package app

import (
	"reflect"
	"testing"
	"time"

	"github.com/12345nikhilkumars/crictui/internal/models"
)

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
