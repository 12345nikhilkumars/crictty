package ui

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/12345nikhilkumars/crictui/internal/app"
	"github.com/12345nikhilkumars/crictui/internal/models"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type screen int

const (
	screenList screen = iota
	screenMatch
)

type tickMsg time.Time

type matchLoadedMsg struct {
	match      models.MatchInfo
	commentary map[uint32][]models.CommentaryEntry
	err        error
}

type matchRefreshedMsg struct {
	match models.MatchInfo
	err   error
}

type commentaryLoadedMsg struct {
	entries   []models.CommentaryEntry
	inningsID uint32
	err       error
}

var (
	keyUp    = key.NewBinding(key.WithKeys("up", "k"))
	keyDown  = key.NewBinding(key.WithKeys("down", "j"))
	keyEnter = key.NewBinding(key.WithKeys("enter"))
	keyEsc   = key.NewBinding(key.WithKeys("esc"))
	keyQuit  = key.NewBinding(key.WithKeys("q", "ctrl+c"))
	keyLeft  = key.NewBinding(key.WithKeys("left"))
	keyRight = key.NewBinding(key.WithKeys("right"))
)

type Model struct {
	app    *app.App
	screen screen

	cursor    int
	scrollOff int

	activeMatch    *models.MatchInfo
	loadingMatch   bool
	matchErr       string
	currentInnings int
	showBowling    bool
	showHelp       bool

	commentary          []models.CommentaryEntry
	commentaryByInnings map[uint32][]models.CommentaryEntry
	commentaryScroll    int

	keyBuf   string
	tickRate int
	width    int
	height   int
}

func NewModel(a *app.App, tickRate int) Model {
	return Model{app: a, screen: screenList, tickRate: tickRate}
}

func (m Model) Init() tea.Cmd { return tea.EnterAltScreen }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case matchLoadedMsg:
		m.loadingMatch = false
		if msg.err != nil {
			m.matchErr = msg.err.Error()
			return m, nil
		}
		m.activeMatch = &msg.match
		m.commentaryByInnings = msg.commentary
		if m.commentaryByInnings == nil {
			m.commentaryByInnings = make(map[uint32][]models.CommentaryEntry)
		}
		// Start on the latest innings
		innID := uint32(1)
		numInnings := len(m.activeMatch.Scorecard)
		if numInnings > 0 {
			innID = uint32(numInnings)
		}
		m.currentInnings = int(innID) - 1
		if m.currentInnings < 0 {
			m.currentInnings = 0
		}
		m.syncCommentaryToInnings()
		m.matchErr = ""
		m.showBowling = false
		m.showHelp = false
		m.keyBuf = ""
		m.screen = screenMatch
		return m, tickCmd(m.tickRate)
	case matchRefreshedMsg:
		if msg.err == nil && m.activeMatch != nil && msg.match.CricbuzzMatchID == m.activeMatch.CricbuzzMatchID {
			msg.match.MatchShortName = m.activeMatch.MatchShortName
			m.activeMatch = &msg.match
		}
		return m, tickCmd(m.tickRate)
	case commentaryLoadedMsg:
		if msg.err == nil {
			if m.commentaryByInnings == nil {
				m.commentaryByInnings = make(map[uint32][]models.CommentaryEntry)
			}
			// Cache commentary for this innings
			m.commentaryByInnings[msg.inningsID] = msg.entries
			// If this is the currently viewed innings, update the visible commentary
			if uint32(m.currentInnings+1) == msg.inningsID {
				m.commentary = msg.entries
				m.commentaryScroll = 0
			}
		}
		return m, nil
	case tickMsg:
		if m.screen == screenMatch && m.activeMatch != nil {
			matchID := m.activeMatch.CricbuzzMatchID
			innID := uint32(m.currentInnings + 1)
			return m, tea.Batch(
				func() tea.Msg {
					info, err := m.app.RefreshMatch(matchID)
					return matchRefreshedMsg{match: info, err: err}
				},
				func() tea.Msg {
					entries, err := m.app.LoadCommentary(matchID, innID)
					return commentaryLoadedMsg{entries: entries, inningsID: innID, err: err}
				},
			)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		m.showHelp = false
		m.keyBuf = ""
		return m, nil
	}
	switch {
	case key.Matches(msg, keyQuit):
		if m.screen == screenMatch {
			m.screen = screenList
			m.activeMatch = nil
			m.commentary = nil
			m.commentaryByInnings = nil
			m.matchErr = ""
			m.keyBuf = ""
			return m, nil
		}
		return m, tea.Quit
	case key.Matches(msg, keyEsc):
		if m.screen == screenMatch {
			m.screen = screenList
			m.activeMatch = nil
			m.commentary = nil
			m.commentaryByInnings = nil
			m.matchErr = ""
			m.keyBuf = ""
			return m, nil
		}
		return m, tea.Quit
	}
	switch m.screen {
	case screenList:
		return m.handleListKey(msg)
	case screenMatch:
		return m.handleMatchKey(msg)
	}
	return m, nil
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	total := m.app.TotalMatches()
	if total == 0 {
		return m, nil
	}
	keyStr := msg.String()
	// Dynamic key jump: 1-9, 0, a-z
	if idx := keyToIndex(keyStr); idx >= 0 && idx < total {
		m.cursor = idx
		m.adjustListScroll()
		return m, nil
	}
	// Help
	if keyStr == "h" {
		m.showHelp = true
		return m, nil
	}
	switch {
	case key.Matches(msg, keyUp):
		if m.cursor > 0 {
			m.cursor--
			m.adjustListScroll()
		}
	case key.Matches(msg, keyDown):
		if m.cursor < total-1 {
			m.cursor++
			m.adjustListScroll()
		}
	case key.Matches(msg, keyEnter):
		item, ok := m.app.MatchAtFlatIndex(m.cursor)
		if !ok {
			return m, nil
		}
		m.loadingMatch = true
		m.matchErr = ""
		shortName := item.ShortName
		matchID := item.MatchID
		return m, func() tea.Msg {
			info, err := m.app.LoadMatch(matchID)
			if err != nil {
				return matchLoadedMsg{err: err}
			}
			info.MatchShortName = shortName
			commMap := make(map[uint32][]models.CommentaryEntry)
			for i := 1; i <= len(info.Scorecard); i++ {
				c, _ := m.app.LoadCommentary(matchID, uint32(i))
				if len(c) > 0 {
					commMap[uint32(i)] = c
				}
			}
			if len(commMap) == 0 {
				for _, id := range []uint32{1, 2} {
					c, _ := m.app.LoadCommentary(matchID, id)
					if len(c) > 0 {
						commMap[id] = c
					}
				}
			}
			return matchLoadedMsg{match: info, commentary: commMap}
		}
	}
	return m, nil
}

func (m Model) handleMatchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.activeMatch == nil {
		return m, nil
	}
	keyStr := msg.String()

	if m.keyBuf == "b" {
		m.keyBuf = ""
		switch keyStr {
		case "a":
			m.showBowling = false
			return m, nil
		case "o":
			m.showBowling = true
			return m, nil
		}
	}
	if keyStr == "b" {
		m.keyBuf = "b"
		return m, nil
	}
	m.keyBuf = ""

	if keyStr == "h" {
		m.showHelp = true
		return m, nil
	}

	// Manual refresh
	if keyStr == "r" {
		matchID := m.activeMatch.CricbuzzMatchID
		innID := uint32(m.currentInnings + 1)
		return m, tea.Batch(
			func() tea.Msg {
				info, err := m.app.RefreshMatch(matchID)
				return matchRefreshedMsg{match: info, err: err}
			},
			func() tea.Msg {
				entries, err := m.app.LoadCommentary(matchID, innID)
				return commentaryLoadedMsg{entries: entries, inningsID: innID, err: err}
			},
		)
	}

	switch {
	case key.Matches(msg, keyLeft):
		if m.currentInnings > 0 {
			m.currentInnings--
			m.syncCommentaryToInnings()
		}
	case key.Matches(msg, keyRight):
		if m.currentInnings < len(m.activeMatch.Scorecard)-1 {
			m.currentInnings++
			m.syncCommentaryToInnings()
		}
	case key.Matches(msg, keyUp):
		if m.commentaryScroll > 0 {
			m.commentaryScroll--
		}
	case key.Matches(msg, keyDown):
		if m.commentaryScroll < len(m.commentary)-1 {
			m.commentaryScroll++
		}
	default:
		if n, err := strconv.Atoi(keyStr); err == nil && n >= 1 && n <= len(m.activeMatch.Scorecard) {
			m.currentInnings = n - 1
			m.syncCommentaryToInnings()
		}
	}
	return m, nil
}

func (m *Model) syncCommentaryToInnings() {
	if m.commentaryByInnings == nil {
		m.commentary = nil
		m.commentaryScroll = 0
		return
	}
	innID := uint32(m.currentInnings + 1)
	if c, ok := m.commentaryByInnings[innID]; ok {
		m.commentary = c
	} else if m.activeMatch != nil {
		c, _ := m.app.LoadCommentary(m.activeMatch.CricbuzzMatchID, innID)
		if len(c) > 0 {
			m.commentaryByInnings[innID] = c
			m.commentary = c
		} else {
			m.commentary = nil
		}
	} else {
		m.commentary = nil
	}
	m.commentaryScroll = 0
}

func tickCmd(tickRate int) tea.Cmd {
	return tea.Tick(time.Duration(tickRate)*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ════════════════════════════════════════════════════════════════
// VIEW
// ════════════════════════════════════════════════════════════════

func (m Model) View() string {
	switch m.screen {
	case screenList:
		return m.viewList()
	case screenMatch:
		return m.viewMatch()
	}
	return ""
}

func (m Model) viewList() string {
	total := m.app.TotalMatches()
	if total == 0 {
		return m.viewEmpty()
	}
	if m.cursor >= total {
		m.cursor = total - 1
	}
	if m.showHelp {
		return m.renderListHelpOverlay()
	}
	return m.renderListTable()
}

func (m Model) renderListTable() string {
	total := m.app.TotalMatches()
	if total == 0 {
		return ""
	}
	W := m.width
	if W < 60 {
		W = 60
	}
	H := m.height
	if H < 10 {
		H = 30
	}

	px := 2
	iW := W - px*2
	contentW := iW - 2

	const colNo = 4
	const colKey = 5
	const colCursor = 2
	const colFormat = 11
	const colType = 13
	fixedW := colNo + colKey + colCursor + colFormat + colType + 5 // 5 spaces between 6 cols
	nameW := contentW - fixedW - 3                                 // -3 buffer to prevent overflow
	if nameW < 18 {
		nameW = 18
	}

	box := dimText.Render
	topBorder := box("┌" + strings.Repeat("─", iW-2) + "┐")
	bottomBorder := box("└" + strings.Repeat("─", iW-2) + "┘")
	hLine := box("├" + strings.Repeat("─", iW-2) + "┤")
	wrap := func(s string) string { return box("│") + padToDisplayWidth(s, contentW) + box("│") }
	// Constrain row width to prevent overflow (lipgloss truncates with ellipsis, preserves ANSI)
	rowMaxW := lipgloss.NewStyle().MaxWidth(contentW)
	centerWrap := func(s string) string {
		rendered := lipgloss.NewStyle().Width(contentW).Align(lipgloss.Center).Render(s)
		return box("│") + padToDisplayWidth(rendered, contentW) + box("│")
	}

	// Table header row
	hdr := " " + tableHeaderStyle.Render(
		padCol("", colCursor)+" "+
			padCol("#", colNo)+" "+
			padCol("Key", colKey)+" "+
			padCol("Match", nameW)+" "+
			padCol("Format", colFormat)+" "+
			padCol("Type", colType))

	// Data rows
	var dataRows []string
	fi := 0
	for _, sec := range m.app.Sections {
		for _, item := range sec.Matches {
			keyLabel := indexToKey(fi)
			if keyLabel == "" {
				keyLabel = "-"
			}
			name := item.ShortName
			if runewidth.StringWidth(name) > nameW {
				name = runewidth.Truncate(name, nameW, "…")
			}
			format := item.Format
			if format == "" {
				format = "-"
			}
			matchType := item.MatchType
			if matchType == "" {
				matchType = "-"
			}
			style := rowStyle
			cursor := " "
			if fi == m.cursor {
				style = listItemActiveStyle
				cursor = "▸"
			}
			row := " " + style.Render(
				padCol(cursor, colCursor)+" "+
					padCol(fmt.Sprintf("%d", fi+1), colNo)+" "+
					padCol(keyLabel, colKey)+" "+
					padCol(name, nameW)+" "+
					padCol(format, colFormat)+" "+
					padCol(matchType, colType))
			dataRows = append(dataRows, row)
			fi++
		}
	}

	// Footer (help hints) — centered
	footer := hint("h", "Help") + "  " + hint("↑↓", "navigate") + "  " + hint("enter", "select") + "  " + hint("1-0/a-z", "jump") + "  " + hint("q", "quit")

	// Fixed lines: top(1) + title(1) + hLine(1) + header(1) + hLine(1) + hLine(1) + footer(1) + bottom(1) = 8
	fixedLines := 8
	bodyH := H - fixedLines - 1
	if bodyH < 3 {
		bodyH = 3
	}

	if m.cursor < m.scrollOff {
		m.scrollOff = m.cursor
	}
	if m.cursor >= m.scrollOff+bodyH {
		m.scrollOff = m.cursor - bodyH + 1
	}
	if m.scrollOff < 0 {
		m.scrollOff = 0
	}
	end := m.scrollOff + bodyH
	if end > len(dataRows) {
		end = len(dataRows)
	}

	var out []string
	out = append(out, topBorder)
	out = append(out, centerWrap(boldWhite.Render("Live Cricket Matches")))
	out = append(out, hLine)
	out = append(out, wrap(rowMaxW.Render(hdr)))
	out = append(out, hLine)

	visibleCount := end - m.scrollOff
	for i := m.scrollOff; i < end; i++ {
		out = append(out, wrap(rowMaxW.Render(dataRows[i])))
	}
	for i := visibleCount; i < bodyH; i++ {
		out = append(out, wrap(""))
	}

	out = append(out, hLine)
	if m.loadingMatch {
		out = append(out, centerWrap(greenText.Render("Loading match...")))
	} else if m.matchErr != "" {
		out = append(out, centerWrap(dimText.Render("Error: "+m.matchErr)))
	} else {
		out = append(out, centerWrap(footer))
	}
	out = append(out, bottomBorder)

	view := strings.Join(out, "\n")
	return lipgloss.NewStyle().MarginLeft(px).MarginRight(px).Render(view)
}

func padCol(s string, w int) string {
	n := runewidth.StringWidth(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func (m Model) renderListHelpOverlay() string {
	var b strings.Builder
	b.WriteString(boldWhite.Render("  Help — Live matches") + "\n\n")
	bindings := []struct{ key, desc string }{
		{"h", "Toggle this help"},
		{"↑ ↓ / j k", "Move selection"},
		{"1-9, 0, a-z", "Jump to match by key"},
		{"enter", "Open selected match"},
		{"q", "Quit"},
	}
	for _, bind := range bindings {
		b.WriteString(fmt.Sprintf("  %s  %s\n", hintKeyStyle.Render(fmt.Sprintf("%-14s", bind.key)), bind.desc))
	}
	b.WriteString("\n" + dimText.Render("  Press any key to close"))
	overlay := helpOverlayStyle.Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay, lipgloss.WithWhitespaceChars(" "))
}

func (m Model) viewEmpty() string {
	return "\n  No live matches found.\n\n" +
		dimText.Render("  Use --match-id to view a specific match.") + "\n\n" +
		dimText.Render("  Press 'q' to quit")
}

// adjustListScroll keeps the list scroll offset so the cursor stays visible.
func (m *Model) adjustListScroll() {
	visibleLines := m.height - 6
	if visibleLines < 5 {
		visibleLines = 20
	}
	cursorLine := m.cursor + 2
	if cursorLine < m.scrollOff {
		m.scrollOff = cursorLine
	}
	if cursorLine >= m.scrollOff+visibleLines {
		m.scrollOff = cursorLine - visibleLines + 1
	}
	if m.scrollOff < 0 {
		m.scrollOff = 0
	}
}

// ── Match View ─────────────────────────────────────────────────

func (m Model) viewMatch() string {
	if m.activeMatch == nil {
		return greenText.Render("  Loading...")
	}

	match := *m.activeMatch
	W := m.width
	if W < 60 {
		W = 80
	}
	H := m.height
	if H < 10 {
		H = 30
	}

	px := 2 // horizontal padding (left and right)
	iW := W - px*2

	header := m.renderHeader(match, iW)
	headerH := lipgloss.Height(header)

	footerStr := m.renderFooter(iW)
	footerH := 1

	bodyH := H - headerH - footerH
	if bodyH < 5 {
		bodyH = 15
	}

	leftW := iW * 45 / 100
	if leftW < 48 {
		leftW = 48
	}
	// Right column width so body (left + │ + right) fits iW-2; junction line uses leftW + 1 + rightW = iW - 2.
	rightW := iW - leftW - 3

	left := m.renderScorecard(match, leftW, bodyH)

	var divLines []string
	for i := 0; i < bodyH; i++ {
		divLines = append(divLines, dimText.Render("│"))
	}
	divider := strings.Join(divLines, "\n")

	right := m.renderRightPane(match, rightW, bodyH)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)

	view := wrapWithConnectedBorders(header, body, footerStr, iW, leftW, rightW)
	view = lipgloss.NewStyle().MarginLeft(px).MarginRight(px).Render(view)

	if m.showHelp {
		view = m.renderHelpOverlay(W)
	}

	return view
}

// ── Header ─────────────────────────────────────────────────────

func (m Model) renderHeader(match models.MatchInfo, W int) string {
	ms := match.CricbuzzInfo.Miniscore
	sd := ms.MatchScoreDetails
	hdr := match.CricbuzzInfo.MatchHeader

	p := 1 // inner padding from screen edges (left and right) — reduced to match visual balance
	iW := W - p*2
	contentW := iW - 2 - p*2 // leave room for left indent + right indent so row fits in box content width iW-2
	lCol := lipgloss.NewStyle().Width(contentW / 2).Align(lipgloss.Left)
	rCol := lipgloss.NewStyle().Width(contentW - contentW/2).Align(lipgloss.Right)
	center := func(s string) string {
		return lipgloss.NewStyle().Width(contentW).Align(lipgloss.Center).Render(s)
	}
	indent := strings.Repeat(" ", p)
	rightPad := strings.Repeat(" ", p)

	var rows []string

	// Row 1: title centered
	title := fmt.Sprintf("%s vs %s · %s, %s",
		hdr.Team1.ShortName, hdr.Team2.ShortName,
		hdr.MatchFormat, hdr.MatchDescription)
	rows = append(rows, indent+center(boldWhite.Render(title))+rightPad)

	// Row 2-3: scores at edges with 4s/6s below
	if len(sd.InningsScoreList) > 0 {
		type sb struct{ score, extra string }
		var blocks []sb
		team1Short := hdr.Team1.ShortName
		team2Short := hdr.Team2.ShortName
		t1Fours, t1Sixes, t2Fours, t2Sixes := teamBoundaryTotals(match.Scorecard, team1Short, team2Short)
		for i, inn := range sd.InningsScoreList {
			s := fmtInningsScore(inn)
			f, x := inningsBoundaryTotals(match.Scorecard, i)
			blocks = append(blocks, sb{
				score: cyanText.Render(s),
				extra: dimText.Render(fmt.Sprintf("4s: %d | 6s: %d", f, x)),
			})
		}
		// For Test matches (4+ innings), use team-aggregated 4s/6s across all innings
		if len(match.Scorecard) > 2 {
			if len(blocks) >= 1 {
				blocks[0].extra = dimText.Render(fmt.Sprintf("4s: %d | 6s: %d", t1Fours, t1Sixes))
			}
			if len(blocks) >= 2 {
				blocks[1].extra = dimText.Render(fmt.Sprintf("4s: %d | 6s: %d", t2Fours, t2Sixes))
			}
		}
		if len(blocks) == 1 {
			rows = append(rows, indent+blocks[0].score+rightPad)
			rows = append(rows, indent+blocks[0].extra+rightPad)
		} else {
			rows = append(rows, indent+lipgloss.JoinHorizontal(lipgloss.Top,
				lCol.Render(blocks[0].score), rCol.Render(blocks[1].score))+rightPad)
			rows = append(rows, indent+lipgloss.JoinHorizontal(lipgloss.Top,
				lCol.Render(blocks[0].extra), rCol.Render(blocks[1].extra))+rightPad)
		}
	}

	// Row 4: comparison centered
	comp := m.renderComparison(match)
	if comp != "" {
		rows = append(rows, indent+center(yellowText.Render(comp))+rightPad)
	}

	// Row 5: status + CRR on same line, centered
	var statusParts []string
	if ms.Status != "" {
		statusParts = append(statusParts, greenText.Render(ms.Status))
	}
	if ms.CurrentRunRate > 0 {
		statusParts = append(statusParts, dimText.Render(fmt.Sprintf("CRR: %.2f", ms.CurrentRunRate)))
	}
	if ms.RequiredRunRate > 0 {
		statusParts = append(statusParts, dimText.Render(fmt.Sprintf("RRR: %.2f", ms.RequiredRunRate)))
	}
	if len(statusParts) > 0 {
		rows = append(rows, indent+center(strings.Join(statusParts, "    "))+rightPad)
	}

	// Row 6: batsmen and bowler — name and score together (no gap), bowler right-aligned
	if ms.BatsmanStriker.BatName != "" {
		strikerStats := fmt.Sprintf("%d(%d)*", ms.BatsmanStriker.BatRuns, ms.BatsmanStriker.BatBalls)
		nonStrikerStats := fmt.Sprintf("%d(%d)", ms.BatsmanNonStriker.BatRuns, ms.BatsmanNonStriker.BatBalls)

		// Name and score directly adjacent so they read as one unit
		line1 := ms.BatsmanStriker.BatName + " " + strikerStats
		line2 := ms.BatsmanNonStriker.BatName + " " + nonStrikerStats

		bowlerName := ms.BowlerStriker.BowlName
		bowlFig := fmt.Sprintf("%d-%d (%.1f)", ms.BowlerStriker.BowlWkts, ms.BowlerStriker.BowlRuns, ms.BowlerStriker.BowlOvs)

		// Fixed width for batsmen block so bowler starts at same column; bowler right-aligned in remainder
		batsmenW := 32
		rw1 := runewidth.StringWidth(line1)
		rw2 := runewidth.StringWidth(line2)
		pad1 := batsmenW - rw1
		pad2 := batsmenW - rw2
		if pad1 < 0 {
			pad1 = 0
		}
		if pad2 < 0 {
			pad2 = 0
		}
		rightW := contentW - batsmenW
		bowlNameW := runewidth.StringWidth(bowlerName)
		bowlFigW := runewidth.StringWidth(bowlFig)
		bowlNamePad := rightW - bowlNameW
		bowlFigPad := rightW - bowlFigW
		if bowlNamePad < 0 {
			bowlNamePad = 0
		}
		if bowlFigPad < 0 {
			bowlFigPad = 0
		}

		row1 := indent + boldWhite.Render(line1) + strings.Repeat(" ", pad1) + strings.Repeat(" ", bowlNamePad) + rowStyle.Render(bowlerName) + rightPad
		row2 := indent + rowStyle.Render(line2) + strings.Repeat(" ", pad2) + strings.Repeat(" ", bowlFigPad) + rowStyle.Render(bowlFig) + rightPad

		rows = append(rows, row1)
		rows = append(rows, row2)
	}

	// Recent overs (last ~12 balls) centered just above the horizontal line
	if ms.RecentOvsStats != "" {
		rows = append(rows, indent+center(dimText.Render("Recent: ")+rowStyle.Render(ms.RecentOvsStats))+rightPad)
	}

	rows = append(rows, indent+dimText.Render(strings.Repeat("─", contentW))+rightPad)

	return strings.Join(rows, "\n") + "\n"
}

func (m Model) renderComparison(match models.MatchInfo) string {
	sd := match.CricbuzzInfo.Miniscore.MatchScoreDetails
	if len(sd.InningsScoreList) < 2 {
		return ""
	}
	var inn1, inn2 *models.InningsScore
	for i := range sd.InningsScoreList {
		switch sd.InningsScoreList[i].InningsID {
		case 1:
			inn1 = &sd.InningsScoreList[i]
		case 2:
			inn2 = &sd.InningsScoreList[i]
		}
	}
	if inn1 == nil || inn2 == nil || inn2.Overs <= 0 {
		return ""
	}
	overs1, ok := match.OverSummaries[1]
	if !ok || len(overs1) == 0 {
		return ""
	}
	overInt := int(math.Floor(float64(inn2.Overs)))
	var s, w int
	found := false
	for _, o := range overs1 {
		if o.Overs <= overInt {
			s = o.Score
			w = o.Wickets
			found = true
		}
	}
	if !found {
		return ""
	}
	return fmt.Sprintf("At %.1f ov: %s was %d/%d · %s is %d/%d",
		inn2.Overs, inn1.BatTeamName, s, w, inn2.BatTeamName, inn2.Score, inn2.Wickets)
}

func fmtInningsScore(inn models.InningsScore) string {
	if inn.IsDeclared {
		return fmt.Sprintf("%s %d/%d d (%.1f)", inn.BatTeamName, inn.Score, inn.Wickets, inn.Overs)
	}
	if inn.Wickets == 10 {
		return fmt.Sprintf("%s %d (%.1f)", inn.BatTeamName, inn.Score, inn.Overs)
	}
	return fmt.Sprintf("%s %d/%d (%.1f)", inn.BatTeamName, inn.Score, inn.Wickets, inn.Overs)
}

func inningsBoundaryTotals(scorecard []models.MatchInningsInfo, idx int) (fours, sixes int) {
	if idx < 0 || idx >= len(scorecard) {
		return 0, 0
	}
	for _, bat := range scorecard[idx].BatsmanDetails {
		f, _ := strconv.Atoi(bat.Fours)
		s, _ := strconv.Atoi(bat.Sixes)
		fours += f
		sixes += s
	}
	return
}

// teamBoundaryTotals returns 4s and 6s aggregated across all innings for each team (for Test matches).
func teamBoundaryTotals(scorecard []models.MatchInningsInfo, team1Short, team2Short string) (t1Fours, t1Sixes, t2Fours, t2Sixes int) {
	for _, inn := range scorecard {
		batTeam := inn.BatTeamShortName
		for _, bat := range inn.BatsmanDetails {
			f, _ := strconv.Atoi(bat.Fours)
			s, _ := strconv.Atoi(bat.Sixes)
			switch batTeam {
			case team1Short:
				t1Fours += f
				t1Sixes += s
			case team2Short:
				t2Fours += f
				t2Sixes += s
			}
		}
	}
	return
}

// ── Left pane: Scorecard ───────────────────────────────────────

func (m Model) renderScorecard(match models.MatchInfo, w, h int) string {
	if len(match.Scorecard) == 0 {
		return lipgloss.NewStyle().Width(w).Height(h).Render(dimText.Render(" No scorecard data"))
	}

	var b strings.Builder

	numInnings := len(match.Scorecard)
	// Innings tabs: one per available innings + Bat/Bowl toggle
	tabCount := numInnings + 2
	colW := w / tabCount
	if colW < 3 {
		colW = 3
	}
	colStyle := lipgloss.NewStyle().Width(colW).Align(lipgloss.Center)

	var cells []string
	for i := 0; i < numInnings; i++ {
		batTeam := match.Scorecard[i].BatTeamShortName
		if batTeam == "" {
			batTeam = "?"
		}
		label := ordinal(i+1) + " (" + batTeam + ")"
		if m.currentInnings == i {
			cells = append(cells, colStyle.Render(boldWhite.Render(label)))
		} else {
			cells = append(cells, colStyle.Render(dimText.Render(label)))
		}
	}
	batLabel := "Bat"
	bowlLabel := "Bowl"
	if !m.showBowling {
		batLabel = boldWhite.Render(batLabel)
		bowlLabel = dimText.Render(bowlLabel)
	} else {
		batLabel = dimText.Render(batLabel)
		bowlLabel = boldWhite.Render(bowlLabel)
	}
	cells = append(cells, colStyle.Render(batLabel))
	cells = append(cells, colStyle.Render(bowlLabel))
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, cells...) + "\n")

	b.WriteString(dimText.Render(strings.Repeat("─", w)) + "\n")

	tw := w - 1
	if tw < 30 {
		tw = 30
	}

	innings := match.Scorecard[m.currentInnings]
	batTeam := innings.BatTeamShortName
	bowlTeam := innings.BowlTeamShortName
	if batTeam == "" {
		batTeam = "?"
	}
	if bowlTeam == "" {
		bowlTeam = "?"
	}
	if m.showBowling {
		b.WriteString(dimText.Render(" Bowling: "+bowlTeam) + "\n")
	} else {
		b.WriteString(dimText.Render(" Batting: "+batTeam) + "\n")
	}
	b.WriteString(dimText.Render(strings.Repeat("─", tw)) + "\n")
	if m.showBowling {
		b.WriteString(renderBowlingTable(innings.BowlerDetails, tw))
	} else {
		b.WriteString(renderBattingTable(innings.BatsmanDetails, tw))
	}

	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(b.String())
}

func renderBattingTable(batsmen []models.BatsmanInfo, w int) string {
	if len(batsmen) == 0 {
		return dimText.Render(" No batting data")
	}

	nameW := w - 28
	if nameW < 10 {
		nameW = 10
	}

	var b strings.Builder
	hdr := fmt.Sprintf("%-*s %4s %4s %3s %3s %7s", nameW, "Batsman", "R", "B", "4s", "6s", "S/R")
	b.WriteString(tableHeaderStyle.Render(hdr) + "\n")
	b.WriteString(dimText.Render(strings.Repeat("─", w)) + "\n")

	for _, bat := range batsmen {
		name := bat.Name
		if len(name) > nameW-3 {
			name = name[:nameW-5] + ".."
		}
		isOut := bat.Status != "" && !isNotOut(bat.Status)
		if !isOut {
			name += " *"
		}
		row := fmt.Sprintf("%-*s %4s %4s %3s %3s %7s", nameW, name, bat.Runs, bat.Balls, bat.Fours, bat.Sixes, bat.StrikeRate)
		b.WriteString(rowStyle.Render(row) + "\n")

		if isOut && strings.TrimSpace(bat.Status) != "" {
			d := strings.TrimSpace(bat.Status)
			if len(d) > w-4 {
				d = d[:w-7] + "..."
			}
			b.WriteString(dismissalStyle.Render("  "+d) + "\n")
		} else {
			b.WriteString(dismissalStyle.Render("  not out") + "\n")
		}
	}
	return b.String()
}

func renderBowlingTable(bowlers []models.BowlerInfo, w int) string {
	if len(bowlers) == 0 {
		return dimText.Render(" No bowling data")
	}

	nameW := w - 34
	if nameW < 8 {
		nameW = 8
	}

	var b strings.Builder
	hdr := fmt.Sprintf("%-*s %5s %2s %4s %2s %3s %3s %6s", nameW, "Bowler", "O", "M", "R", "W", "Nb", "Wd", "Econ")
	b.WriteString(tableHeaderStyle.Render(hdr) + "\n")
	b.WriteString(dimText.Render(strings.Repeat("─", w)) + "\n")

	for _, bowl := range bowlers {
		name := bowl.Name
		if len(name) > nameW-1 {
			name = name[:nameW-3] + ".."
		}
		row := fmt.Sprintf("%-*s %5s %2s %4s %2s %3s %3s %6s", nameW, name, bowl.Overs, bowl.Maidens, bowl.Runs, bowl.Wickets, bowl.NoBalls, bowl.Wides, bowl.Economy)
		b.WriteString(rowStyle.Render(row) + "\n")
	}
	return b.String()
}

// ── Right pane: Commentary ─────────────────────────────────────

// renderRightPane renders the right pane with just commentary.
func (m Model) renderRightPane(_ models.MatchInfo, w, h int) string {
	return m.renderCommentaryPane(w, h)
}

// ── Commentary (recent at top) ─────────────────────────────────

func (m Model) renderCommentaryPane(w, h int) string {
	var b strings.Builder

	innLabel := ordinal(m.currentInnings+1) + " Innings"
	if m.activeMatch != nil && m.currentInnings < len(m.activeMatch.Scorecard) {
		batTeam := m.activeMatch.Scorecard[m.currentInnings].BatTeamShortName
		if batTeam != "" {
			innLabel = innLabel + " (" + batTeam + ")"
		}
	}
	b.WriteString(" " + boldWhite.Render("Commentary") + "  " + dimText.Render(innLabel) + "\n")

	if len(m.commentary) == 0 {
		b.WriteString(" " + dimText.Render("No commentary available for this innings.") + "\n")
		return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(b.String())
	}

	headerLines := 1
	footerLines := 1
	availH := h - headerLines - footerLines
	if availH < 3 {
		availH = 10
	}

	lineW := w - 2
	if lineW < 20 {
		lineW = 20
	}

	// Reverse commentary so most recent is at the top
	reversed := make([]models.CommentaryEntry, len(m.commentary))
	for i, e := range m.commentary {
		reversed[len(m.commentary)-1-i] = e
	}

	type wrappedLine struct {
		text     string
		entryIdx int
	}
	var allLines []wrappedLine
	for ei, entry := range reversed {
		lines := renderCommentaryLines(entry, lineW)
		for _, l := range lines {
			allLines = append(allLines, wrappedLine{text: l, entryIdx: ei})
		}
	}

	if len(allLines) == 0 {
		b.WriteString(" " + dimText.Render("No commentary available.") + "\n")
		return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(b.String())
	}

	// Map scroll position to line index
	targetLine := 0
	for i, wl := range allLines {
		if wl.entryIdx >= m.commentaryScroll {
			targetLine = i
			break
		}
	}

	start := targetLine
	endLine := start + availH
	if endLine > len(allLines) {
		endLine = len(allLines)
		start = max(0, endLine-availH)
	}

	for i := start; i < endLine; i++ {
		b.WriteString(" " + allLines[i].text + "\n")
	}

	pct := 0.0
	if len(m.commentary) > 1 {
		pct = float64(m.commentaryScroll) / float64(len(m.commentary)-1) * 100
	}
	b.WriteString(dimText.Render(fmt.Sprintf(" [%.0f%%] ↑↓ scroll", pct)))

	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(b.String())
}

var (
	htmlTagRe = regexp.MustCompile(`<[^>]*>`)
	b0Re      = regexp.MustCompile(`B0\$`)
)

func cleanCommText(raw string) string {
	text := htmlTagRe.ReplaceAllString(raw, "")
	text = b0Re.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\\n", "\n")
	var cleaned []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

func renderCommentaryLines(entry models.CommentaryEntry, w int) []string {
	text := cleanCommText(entry.CommText)
	if text == "" {
		return nil
	}

	if entry.Event == "over-break" {
		sep := fmt.Sprintf("─── End of Over %s ───", entry.OverNumber)
		return []string{"", commentaryOverSepStyle.Render(sep), ""}
	}

	prefix := ""
	if entry.OverNumber != "" {
		prefix = entry.OverNumber + " "
	}

	styleFn := func(s string) string {
		switch {
		case entry.Event == "WICKET" || strings.Contains(strings.ToLower(entry.Event), "wicket"):
			return commentaryWicketStyle.Render(s)
		case entry.Event == "FOUR" || entry.Event == "SIX":
			return commentaryBoundaryStyle.Render(s)
		default:
			return commentaryBallStyle.Render(s)
		}
	}

	var result []string
	paragraphs := strings.Split(text, "\n")
	for i, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		line := para
		if i == 0 && prefix != "" {
			line = prefix + para
		}
		wrappedLines := wordWrap(line, w)
		for j, wrapped := range wrappedLines {
			justified := wrapped
			if j < len(wrappedLines)-1 {
				justified = justifyLine(wrapped, w)
			}
			result = append(result, styleFn(justified))
		}
	}
	return result
}

func wordWrap(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	var lines []string
	words := strings.Fields(s)
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
		} else if len(current)+1+len(word) <= w {
			current += " " + word
		} else {
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

// justifyLine distributes spaces between words so the line length equals width.
// Single-word or empty lines are left as-is (left-aligned).
func justifyLine(line string, width int) string {
	words := strings.Fields(line)
	if len(words) <= 1 || width <= 0 {
		return line
	}
	totalChars := 0
	for _, w := range words {
		totalChars += len(w)
	}
	spacesNeeded := width - totalChars
	gaps := len(words) - 1
	if gaps == 0 {
		return line
	}
	spacePerGap := spacesNeeded / gaps
	extra := spacesNeeded % gaps
	var b strings.Builder
	for i, w := range words {
		if i > 0 {
			n := spacePerGap
			if i <= extra {
				n++
			}
			b.WriteString(strings.Repeat(" ", n))
		}
		b.WriteString(w)
	}
	return b.String()
}

// ── Footer ─────────────────────────────────────────────────────

func (m Model) renderFooter(W int) string {
	hints := hint("h", "Help") + "  " +
		hint("r", "Refresh") + "  " +
		hint("ba", "Bat") + "  " +
		hint("bo", "Bowl") + "  " +
		hint("←→", "Innings") + "  " +
		hint("↑↓", "Scroll") + "  " +
		hint("esc", "Back") + "  " +
		hint("q", "Quit")
	return lipgloss.NewStyle().Width(W).PaddingLeft(1).Render(hints)
}

func hint(k, label string) string {
	return hintKeyStyle.Render("("+k+")") + " " + hintLabelStyle.Render(label)
}

// indexToKey returns the key label for the given flat match index (1-9, 0, a-z).
func indexToKey(idx int) string {
	if idx < 0 {
		return ""
	}
	if idx <= 8 {
		return string(rune('1' + idx))
	}
	if idx == 9 {
		return "0"
	}
	if idx <= 35 {
		return string(rune('a' + idx - 10))
	}
	return ""
}

// keyToIndex returns the flat match index for a key (1-9 -> 0-8, 0 -> 9, a-z -> 10-35), or -1 if invalid.
func keyToIndex(k string) int {
	if len(k) != 1 {
		return -1
	}
	c := k[0]
	if c >= '1' && c <= '9' {
		return int(c - '1')
	}
	if c == '0' {
		return 9
	}
	if c >= 'a' && c <= 'z' {
		return 10 + int(c-'a')
	}
	return -1
}

// ── Help Overlay ───────────────────────────────────────────────

func (m Model) renderHelpOverlay(width int) string {
	var b strings.Builder
	b.WriteString(boldWhite.Render("  Keybindings") + "\n\n")

	bindings := []struct{ key, desc string }{
		{"h", "Toggle this help"},
		{"r", "Refresh score / commentary now"},
		{"ba", "Show batting scorecard"},
		{"bo", "Show bowling scorecard"},
		{"← →", "Switch innings"},
		{"↑ ↓ / j k", "Scroll commentary"},
		{"1-4", "Jump to innings"},
		{"esc", "Back to match list"},
		{"q", "Quit"},
	}
	for _, bind := range bindings {
		b.WriteString(fmt.Sprintf("  %s  %s\n",
			hintKeyStyle.Render(fmt.Sprintf("%-12s", bind.key)), bind.desc))
	}
	b.WriteString("\n" + dimText.Render("  Press any key to close"))

	overlay := helpOverlayStyle.Render(b.String())
	return lipgloss.Place(width, m.height, lipgloss.Center, lipgloss.Center, overlay,
		lipgloss.WithWhitespaceChars(" "))
}

// ── Helpers ────────────────────────────────────────────────────

func ordinal(n int) string {
	switch n {
	case 1:
		return "1st"
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	default:
		return fmt.Sprintf("%dth", n)
	}
}

func isNotOut(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "not out" || s == "batting" || s == "*" || s == ""
}

// padToDisplayWidth pads s with spaces to display width w (ignores ANSI for width).
func padToDisplayWidth(s string, w int) string {
	stripped := ansiStripRegex.ReplaceAllString(s, "")
	n := runewidth.StringWidth(stripped)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

var ansiStripRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// wrapWithConnectedBorders builds a full view with connected box-drawing: ┌─┐ │ ├┼┤ └─┘.
func wrapWithConnectedBorders(header, body, footerStr string, iW, leftW, rightW int) string {
	box := dimText.Render
	topBorder := box("┌" + strings.Repeat("─", iW-2) + "┐")
	bottomBorder := box("└" + strings.Repeat("─", iW-2) + "┘")
	headerSep := box("├" + strings.Repeat("─", iW-2) + "┤")
	hLineWithJunction := box("├" + strings.Repeat("─", leftW) + "┼" + strings.Repeat("─", rightW) + "┤")

	contentW := iW - 2
	wrap := func(s string) string { return box("│") + padToDisplayWidth(s, contentW) + box("│") }

	headerLines := strings.Split(strings.TrimSuffix(header, "\n"), "\n")
	if len(headerLines) > 0 && strings.Contains(headerLines[len(headerLines)-1], "─") {
		headerLines = headerLines[:len(headerLines)-1]
	}

	var out []string
	out = append(out, topBorder)
	for _, line := range headerLines {
		out = append(out, wrap(line))
	}
	out = append(out, headerSep)

	bodyLines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	for _, line := range bodyLines {
		out = append(out, wrap(line))
	}
	out = append(out, hLineWithJunction)
	out = append(out, wrap(footerStr))
	out = append(out, bottomBorder)

	return strings.Join(out, "\n")
}
