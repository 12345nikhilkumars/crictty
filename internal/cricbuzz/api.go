package cricbuzz

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/12345nikhilkumars/crictui/internal/models"
	"github.com/PuerkitoBio/goquery"
)

// URL constants for Cricbuzz API endpoints
const (
	CricbuzzLiveMatchesURL    = "https://www.cricbuzz.com/cricket-match/live-scores"
	CricbuzzMatchAPI          = "https://www.cricbuzz.com/api/mcenter/comm/"
	CricbuzzMatchScorecardAPI = "https://www.cricbuzz.com/api/mcenter/scorecard/"
	CricbuzzMatchOverviewsAPI = "https://www.cricbuzz.com/api/mcenter/over-refresh/"
	CricbuzzOverByOverAPI     = "https://www.cricbuzz.com/api/mcenter/over-by-over/"
	CricbuzzBaseAPI           = "https://www.cricbuzz.com"
)

// Client represents the Cricbuzz API client
type Client struct {
	httpClient *http.Client
}

// NewClient initializes a new Cricbuzz API client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{},
	}
}

const requestInterval = 1 * time.Second

var lastRequest time.Time

// makeRequest performs an HTTP GET request to the specified URL with rate limiting.
func (c *Client) makeRequest(url string) (*http.Response, error) {
	time.Sleep(time.Until(lastRequest.Add(requestInterval)))
	lastRequest = time.Now()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	return c.httpClient.Do(req)
}

// GetAllLiveMatches fetches all live and recently completed matches from the Cricbuzz live-scores page.
// Deprecated: use GetLiveMatchSections for sectioned list and faster load (no per-match API calls).
func (c *Client) GetAllLiveMatches() ([]models.MatchInfo, error) {
	sections, err := c.GetLiveMatchSections()
	if err != nil {
		return nil, err
	}
	var matches []models.MatchInfo
	seen := make(map[uint32]bool)
	for _, sec := range sections {
		for _, m := range sec.Matches {
			if seen[m.MatchID] {
				continue
			}
			seen[m.MatchID] = true
			info, err := c.GetMatchInfo(m.MatchID)
			if err != nil {
				continue
			}
			info.MatchShortName = m.ShortName
			matches = append(matches, info)
		}
	}
	return matches, nil
}

// GetLiveMatchSections fetches the live-scores page once and returns matches grouped by section
// (e.g. "ICC Men's T20 World Cup 2026", "CSA Provincial One-Day Challenge..."). No per-match API
// calls — use this for a fast list; call GetMatchInfo for the selected match when needed.
func (c *Client) GetLiveMatchSections() ([]models.MatchSection, error) {
	resp, err := c.makeRequest(CricbuzzLiveMatchesURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch live scores page: %v", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %v", err)
	}

	var sections []models.MatchSection
	currentSection := "Live & Recent"
	seenIDs := make(map[uint32]bool)

	// Statuses that mean the match is NOT live/in-progress
	completedSuffixes := []string{
		"Complete", "Won", "won", "Abandon", "No Result", "Draw", "Tied",
	}
	isCompleted := func(title string) bool {
		// title ends with "- <status> " — extract the part after the last " - "
		idx := strings.LastIndex(title, " - ")
		if idx < 0 {
			return false
		}
		status := strings.TrimSpace(title[idx+3:])
		for _, suffix := range completedSuffixes {
			if strings.Contains(status, suffix) {
				return true
			}
		}
		return false
	}

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		href = strings.TrimSpace(href)
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return
		}

		if strings.Contains(href, "/cricket-series/") {
			if len(text) > 1 && len(text) < 120 {
				currentSection = text
			}
			return
		}

		if !strings.Contains(href, "/live-cricket-scores/") {
			return
		}

		// Use the title attribute to determine match status and get a clean name
		title, _ := s.Attr("title")
		title = strings.TrimSpace(title)

		// Skip completed/abandoned matches
		if title != "" && isCompleted(title) {
			return
		}

		pathParts := strings.Split(strings.Trim(strings.TrimPrefix(strings.TrimPrefix(href, "https://"), "http://"), "/"), "/")
		idIdx := -1
		for j, p := range pathParts {
			if p == "live-cricket-scores" && j+1 < len(pathParts) {
				idIdx = j + 1
				break
			}
		}
		if idIdx < 0 || idIdx >= len(pathParts) {
			return
		}
		matchID, err := strconv.ParseUint(pathParts[idIdx], 10, 32)
		if err != nil {
			return
		}
		if seenIDs[uint32(matchID)] {
			return
		}
		seenIDs[uint32(matchID)] = true

		// Build short name from title (cleaner) or fall back to link text
		shortName := text
		if title != "" {
			cleanTitle := title
			if idx := strings.LastIndex(cleanTitle, " - "); idx > 0 {
				cleanTitle = strings.TrimSpace(cleanTitle[:idx])
			}
			if len(cleanTitle) > 2 {
				shortName = cleanTitle
			}
		}
		if idx := strings.Index(shortName, " - "); idx > 0 {
			shortName = strings.TrimSpace(shortName[:idx])
		}
		if len(shortName) < 2 {
			return
		}

		item := models.MatchListItem{MatchID: uint32(matchID), ShortName: shortName, SectionName: currentSection}
		if len(sections) == 0 || sections[len(sections)-1].Name != currentSection {
			sections = append(sections, models.MatchSection{Name: currentSection, Matches: []models.MatchListItem{item}})
		} else {
			sections[len(sections)-1].Matches = append(sections[len(sections)-1].Matches, item)
		}
	})

	// Remove empty sections
	var filtered []models.MatchSection
	for _, sec := range sections {
		if len(sec.Matches) > 0 {
			filtered = append(filtered, sec)
		}
	}

	return filtered, nil
}

// GetMatchInfo fetches detailed match information for a given match ID
func (c *Client) GetMatchInfo(matchID uint32) (models.MatchInfo, error) {
	// Construct the URL for the match API
	url := fmt.Sprintf("%s%d", CricbuzzMatchAPI, matchID)
	resp, err := c.makeRequest(url)
	if err != nil {
		return models.MatchInfo{}, fmt.Errorf("failed to fetch match info: %v", err)
	}
	defer resp.Body.Close()

	// Check if the response status is OK
	var cricbuzzJSON models.CricbuzzJSON
	if err := json.NewDecoder(resp.Body).Decode(&cricbuzzJSON); err != nil {
		return models.MatchInfo{}, fmt.Errorf("failed to decode JSON: %v", err)
	}

	// Check if the match is complete
	scorecard, err := c.GetScorecard(matchID)
	if err != nil {
		scorecard = []models.MatchInningsInfo{}
	}

	return models.MatchInfo{
		CricbuzzMatchID:      matchID,
		CricbuzzMatchAPILink: url,
		CricbuzzInfo:         cricbuzzJSON,
		Scorecard:            scorecard,
	}, nil
}

// GetScorecard fetches the scorecard for a given match ID
func (c *Client) GetScorecard(matchID uint32) ([]models.MatchInningsInfo, error) {
	// Construct the URL for the scorecard API
	url := fmt.Sprintf("%s%d", CricbuzzMatchScorecardAPI, matchID)
	resp, err := c.makeRequest(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scorecard: %v", err)
	}
	defer resp.Body.Close()

	// Parse JSON response
	var scorecardJSON models.ScorecardJSON
	if err := json.NewDecoder(resp.Body).Decode(&scorecardJSON); err != nil {
		return nil, fmt.Errorf("failed to decode scorecard JSON: %v", err)
	}

	var scorecard []models.MatchInningsInfo

	// Convert each innings from the JSON to MatchInningsInfo
	for _, innings := range scorecardJSON.ScoreCard {
		matchInnings := models.MatchInningsInfo{}

		// Parse batsmen data - sort by batId to maintain batting order
		var batsmenList []models.BatsmanData
		for _, batsman := range innings.BatTeamDetails.BatsmenData {
			// Skip players who haven't batted (0 balls, no dismissal)
			if batsman.Balls == 0 && batsman.OutDesc == "" {
				continue
			}
			batsmenList = append(batsmenList, batsman)
		}
		// Sort by batId to maintain batting order
		sort.Slice(batsmenList, func(i, j int) bool {
			return batsmenList[i].BatID < batsmenList[j].BatID
		})

		for _, batsman := range batsmenList {
			status := batsman.OutDesc
			if status == "batting" || status == "not out" {
				status = "not out"
			}

			matchInnings.BatsmanDetails = append(matchInnings.BatsmanDetails, models.BatsmanInfo{
				Name:       batsman.BatName,
				Status:     status,
				Runs:       fmt.Sprintf("%d", batsman.Runs),
				Balls:      fmt.Sprintf("%d", batsman.Balls),
				Fours:      fmt.Sprintf("%d", batsman.Fours),
				Sixes:      fmt.Sprintf("%d", batsman.Sixes),
				StrikeRate: fmt.Sprintf("%.2f", batsman.StrikeRate),
			})
		}

		// Parse bowlers data
		for _, bowler := range innings.BowlTeamDetails.BowlersData {
			// Skip bowlers who haven't bowled (0 overs)
			if bowler.Overs == 0 {
				continue
			}

			matchInnings.BowlerDetails = append(matchInnings.BowlerDetails, models.BowlerInfo{
				Name:    bowler.BowlName,
				Overs:   fmt.Sprintf("%.1f", bowler.Overs),
				Maidens: fmt.Sprintf("%d", bowler.Maidens),
				Runs:    fmt.Sprintf("%d", bowler.Runs),
				Wickets: fmt.Sprintf("%d", bowler.Wickets),
				NoBalls: fmt.Sprintf("%d", bowler.NoBalls),
				Wides:   fmt.Sprintf("%d", bowler.Wides),
				Economy: fmt.Sprintf("%.1f", bowler.Economy),
			})
		}

		scorecard = append(scorecard, matchInnings)
	}

	return scorecard, nil
}

// GetOverSummaries fetches all over-by-over summaries for every innings of a match.
// It starts with the over-refresh endpoint (recent overs) then paginates backwards
// via the over-by-over endpoint until all overs are collected.
func (c *Client) GetOverSummaries(matchID uint32) (map[uint32][]models.OverSummary, error) {
	result := make(map[uint32][]models.OverSummary)

	// Step 1: fetch recent overs from over-refresh
	refreshURL := fmt.Sprintf("%s%d", CricbuzzMatchOverviewsAPI, matchID)
	resp, err := c.makeRequest(refreshURL)
	if err != nil {
		return nil, fmt.Errorf("over-refresh request failed: %v", err)
	}
	defer resp.Body.Close()

	var refreshResp struct {
		OverSummaryList []models.OverSummary `json:"overSummaryList"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		return nil, fmt.Errorf("over-refresh decode failed: %v", err)
	}

	for _, o := range refreshResp.OverSummaryList {
		result[o.InningsID] = append(result[o.InningsID], o)
	}

	if len(refreshResp.OverSummaryList) == 0 {
		return result, nil
	}

	// Step 2: find the earliest timestamp and its innings to start pagination
	earliest := refreshResp.OverSummaryList[0]
	for _, o := range refreshResp.OverSummaryList {
		if o.Timestamp < earliest.Timestamp {
			earliest = o
		}
	}

	// Paginate backwards using over-by-over, starting from the earliest innings
	nextURL := fmt.Sprintf("%s%d/%d/%d", CricbuzzOverByOverAPI, matchID, earliest.InningsID, earliest.Timestamp)
	for nextURL != "" {
		pResp, err := c.makeRequest(nextURL)
		if err != nil {
			break
		}

		var page struct {
			PaginatedData     []models.OverSummary `json:"paginatedData"`
			NextPaginationURL string               `json:"nextPaginationURL"`
		}
		if err := json.NewDecoder(pResp.Body).Decode(&page); err != nil {
			pResp.Body.Close()
			break
		}
		pResp.Body.Close()

		if len(page.PaginatedData) == 0 {
			break
		}

		for _, o := range page.PaginatedData {
			result[o.InningsID] = append(result[o.InningsID], o)
		}

		if page.NextPaginationURL == "" {
			break
		}
		nextURL = CricbuzzBaseAPI + page.NextPaginationURL
	}

	// Deduplicate and sort each innings by over number ascending
	for innID, overs := range result {
		seen := make(map[int]bool)
		var deduped []models.OverSummary
		for _, o := range overs {
			if !seen[o.Overs] {
				seen[o.Overs] = true
				deduped = append(deduped, o)
			}
		}
		sort.Slice(deduped, func(i, j int) bool {
			return deduped[i].Overs < deduped[j].Overs
		})
		result[innID] = deduped
	}

	return result, nil
}

// GetFullCommentary fetches the complete ball-by-ball commentary for a single innings.
func (c *Client) GetFullCommentary(matchID uint32, inningsID uint32) ([]models.CommentaryEntry, error) {
	url := fmt.Sprintf("%s/api/mcenter/%d/full-commentary/%d", CricbuzzBaseAPI, matchID, inningsID)
	resp, err := c.makeRequest(url)
	if err != nil {
		return nil, fmt.Errorf("full-commentary request failed: %v", err)
	}
	defer resp.Body.Close()

	// overNumber can be number (e.g. 19.6) or string in API response
	var raw struct {
		Commentary []struct {
			InningsID      uint32 `json:"inningsId"`
			CommentaryList []struct {
				CommText    string      `json:"commText"`
				OverNumber  interface{} `json:"overNumber"`
				InningsID   uint32      `json:"inningsId"`
				Event       string      `json:"event"`
				Timestamp   int64       `json:"timestamp"`
				BatTeamName string      `json:"batTeamName"`
			} `json:"commentaryList"`
		} `json:"commentary"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("full-commentary decode failed: %v", err)
	}

	var entries []models.CommentaryEntry
	for _, inn := range raw.Commentary {
		for _, c := range inn.CommentaryList {
			overNumStr := ""
			if c.OverNumber != nil {
				overNumStr = fmt.Sprint(c.OverNumber)
			}
			entries = append(entries, models.CommentaryEntry{
				CommText:    c.CommText,
				OverNumber:  overNumStr,
				InningsID:   c.InningsID,
				Event:       c.Event,
				Timestamp:   c.Timestamp,
				BatTeamName: c.BatTeamName,
			})
		}
	}

	// Reverse so oldest ball is first (API returns newest first)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	return entries, nil
}
