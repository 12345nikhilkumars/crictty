package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	appcore "github.com/12345nikhilkumars/crictui/internal/app"
	"github.com/12345nikhilkumars/crictui/internal/models"
	tea "github.com/charmbracelet/bubbletea"
)

func TestCleanCommTextSanitizesTerminalControlSequences(t *testing.T) {
	raw := "<b>Ball</b> \x1b[31mFOUR\x1b[0m\x1b]0;owned\x07 keep\tthis\nline\x00\x1f\x7f B0$"

	cleaned := cleanCommText(raw)

	if strings.Contains(cleaned, "\x1b") {
		t.Fatalf("expected ESC sequences removed, got %q", cleaned)
	}
	if strings.Contains(cleaned, "\x07") || strings.Contains(cleaned, "\x00") || strings.Contains(cleaned, "\x1f") || strings.Contains(cleaned, "\x7f") {
		t.Fatalf("expected C0 controls removed, got %q", cleaned)
	}
	if !strings.Contains(cleaned, "Ball FOUR keep\tthis") {
		t.Fatalf("expected printable text preserved, got %q", cleaned)
	}
	if !strings.Contains(cleaned, "line") {
		t.Fatalf("expected newline content preserved, got %q", cleaned)
	}
}

func TestInningsSwitchReturnsAsyncCommentaryCmd(t *testing.T) {
	m := Model{
		screen:         screenMatch,
		currentInnings: 0,
		activeMatch: &models.MatchInfo{
			CricbuzzMatchID: 99,
			Scorecard:       []models.MatchInningsInfo{{}, {}},
		},
		commentaryByInnings: map[uint32][]models.CommentaryEntry{
			1: {{CommText: "cached"}},
		},
	}

	nextModel, cmd := m.handleMatchKey(tea.KeyMsg{Type: tea.KeyRight})
	if cmd == nil {
		t.Fatal("expected innings switch to return a command")
	}

	next := nextModel.(Model)
	if next.currentInnings != 1 {
		t.Fatalf("expected innings index 1, got %d", next.currentInnings)
	}

	msg := cmd()
	req, ok := msg.(commentaryLoadRequestedMsg)
	if !ok {
		t.Fatalf("expected commentaryLoadRequestedMsg, got %T", msg)
	}
	if req.matchID != 99 || req.inningsID != 2 {
		t.Fatalf("unexpected request payload: %+v", req)
	}
}

func TestTickDoesNotTriggerCommentaryFetch(t *testing.T) {
	m := Model{
		screen: screenMatch,
		activeMatch: &models.MatchInfo{
			CricbuzzMatchID: 42,
			Scorecard:       []models.MatchInningsInfo{{}},
		},
	}

	nextModel, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("expected tick to schedule refresh command")
	}

	_ = nextModel
	msg := cmd()
	if _, ok := msg.(commentaryLoadRequestedMsg); ok {
		t.Fatalf("tick should not request commentary fetch, got %T", msg)
	}
	if _, ok := msg.(matchRefreshRequestedMsg); !ok {
		t.Fatalf("expected matchRefreshRequestedMsg, got %T", msg)
	}
}

func TestListEnterStartsAsyncMatchLoadFlow(t *testing.T) {
	a := &appcore.App{
		Sections: []models.MatchSection{{
			Name: "Series",
			Matches: []models.MatchListItem{{
				MatchID:   123,
				ShortName: "IND vs AUS",
			}},
		}},
	}

	m := Model{app: a, screen: screenList}
	nextModel, cmd := m.handleListKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected async match load command")
	}

	next := nextModel.(Model)
	if !next.loadingMatch {
		t.Fatal("expected loadingMatch to be true after enter")
	}

	msg := cmd()
	req, ok := msg.(matchLoadRequestedMsg)
	if !ok {
		t.Fatalf("expected matchLoadRequestedMsg, got %T", msg)
	}
	if req.matchID != 123 || req.shortName != "IND vs AUS" {
		t.Fatalf("unexpected load request payload: %+v", req)
	}
}

func TestCommentaryErrorStateOnFailureAndRecovery(t *testing.T) {
	m := Model{
		screen:         screenMatch,
		currentInnings: 0,
		activeMatch: &models.MatchInfo{
			CricbuzzMatchID: 77,
			Scorecard:       []models.MatchInningsInfo{{}},
		},
	}

	nextModel, _ := m.Update(commentaryLoadedMsg{inningsID: 1, err: errors.New("upstream timeout")})
	next := nextModel.(Model)
	if next.commentaryErr == "" {
		t.Fatal("expected commentary error to be surfaced")
	}

	recoveryModel, _ := next.Update(commentaryLoadedMsg{inningsID: 1, entries: []models.CommentaryEntry{{CommText: "ok"}}})
	recovered := recoveryModel.(Model)
	if recovered.commentaryErr != "" {
		t.Fatalf("expected commentary error cleared on success, got %q", recovered.commentaryErr)
	}
	if len(recovered.commentary) != 1 {
		t.Fatalf("expected commentary to update on success, got %d entries", len(recovered.commentary))
	}
}

func TestEscFromMatchCancelsInFlightRequests(t *testing.T) {
	refreshCanceled := false
	commentaryCanceled := false

	m := Model{
		screen: screenMatch,
		activeMatch: &models.MatchInfo{
			CricbuzzMatchID: 88,
			Scorecard:       []models.MatchInningsInfo{{}},
		},
		matchRefreshCancel: func() { refreshCanceled = true },
		commentaryCancel:   func() { commentaryCanceled = true },
	}

	nextModel, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("expected no command when escaping to list, got %T", cmd)
	}

	next := nextModel.(Model)
	if next.screen != screenList {
		t.Fatalf("expected list screen after esc, got %v", next.screen)
	}
	if !refreshCanceled || !commentaryCanceled {
		t.Fatalf("expected in-flight requests canceled, refresh=%v commentary=%v", refreshCanceled, commentaryCanceled)
	}
}
