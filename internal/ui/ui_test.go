package ui

import (
	"strings"
	"testing"
	"time"

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
