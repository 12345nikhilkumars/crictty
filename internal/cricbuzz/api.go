package cricbuzz

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	httpClient       *http.Client
	limiterMu        sync.Mutex
	lastRequest      time.Time
	requestInterval  time.Duration
	maxRetries       int
	retryBaseBackoff time.Duration
}

// NewClient initializes a new Cricbuzz API client
func NewClient() *Client {
	return &Client{
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		requestInterval:  1 * time.Second,
		maxRetries:       2,
		retryBaseBackoff: 150 * time.Millisecond,
	}
}

const errorBodySnippetLimit = 256

// HTTPStatusError is returned when the upstream API returns a non-success status code.
type HTTPStatusError struct {
	StatusCode  int
	Status      string
	URL         string
	BodySnippet string
}

func (e *HTTPStatusError) Error() string {
	if e.BodySnippet == "" {
		return fmt.Sprintf("http request failed: status=%d (%s) url=%s", e.StatusCode, e.Status, e.URL)
	}
	return fmt.Sprintf("http request failed: status=%d (%s) url=%s body=%q", e.StatusCode, e.Status, e.URL, e.BodySnippet)
}

func isTransientStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) reserveRequestSlot() time.Duration {
	c.limiterMu.Lock()
	defer c.limiterMu.Unlock()

	now := time.Now()
	next := c.lastRequest
	if c.requestInterval > 0 {
		next = c.lastRequest.Add(c.requestInterval)
	}
	if now.Before(next) {
		c.lastRequest = next
		return next.Sub(now)
	}
	c.lastRequest = now
	return 0
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(value); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

func (c *Client) retryDelay(attempt int, retryAfter time.Duration) time.Duration {
	const maxRetryDelay = 2 * time.Second
	if retryAfter > 0 {
		if retryAfter > maxRetryDelay {
			return maxRetryDelay
		}
		return retryAfter
	}
	d := c.retryBaseBackoff * time.Duration(1<<attempt)
	if d > maxRetryDelay {
		return maxRetryDelay
	}
	return d
}

func validateHTTPStatus(resp *http.Response, requestURL string) error {
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodySnippetLimit))
	snippet := strings.TrimSpace(strings.ReplaceAll(string(body), "\n", " "))
	return &HTTPStatusError{
		StatusCode:  resp.StatusCode,
		Status:      resp.Status,
		URL:         requestURL,
		BodySnippet: snippet,
	}
}

// makeRequest performs an HTTP GET request to the specified URL with rate limiting.
func (c *Client) makeRequest(url string) (*http.Response, error) {
	return c.makeRequestWithContext(context.Background(), url)
}

func (c *Client) makeRequestWithContext(ctx context.Context, requestURL string) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	for attempt := 0; ; attempt++ {
		if err := sleepWithContext(ctx, c.reserveRequestSlot()); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < c.maxRetries {
				if err := sleepWithContext(ctx, c.retryDelay(attempt, 0)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}

		if isTransientStatus(resp.StatusCode) && attempt < c.maxRetries {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
			resp.Body.Close()
			if err := sleepWithContext(ctx, c.retryDelay(attempt, retryAfter)); err != nil {
				return nil, err
			}
			continue
		}

		return resp, nil
	}
}

// GetAllLiveMatches fetches all live and recently completed matches from the Cricbuzz live-scores page.
// Deprecated: use GetLiveMatchSections for sectioned list and faster load (no per-match API calls).
func (c *Client) GetAllLiveMatches() ([]models.MatchInfo, error) {
	return c.GetAllLiveMatchesWithContext(context.Background())
}

// GetAllLiveMatchesWithContext fetches all live and recently completed matches with context support.
func (c *Client) GetAllLiveMatchesWithContext(ctx context.Context) ([]models.MatchInfo, error) {
	sections, err := c.GetLiveMatchSectionsWithContext(ctx)
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
			info, err := c.GetMatchInfoWithContext(ctx, m.MatchID)
			if err != nil {
				continue
			}
			info.MatchShortName = m.ShortName
			matches = append(matches, info)
		}
	}
	return matches, nil
}

// deriveFormatAndType infers format (Test/ODI/T20) and match type (International/Domestic/Women)
// from section, match name and the original title attribute.
func deriveFormatAndType(section, matchName, title string) (format, matchType string) {
	s := strings.ToLower(section + " " + matchName + " " + title)
	switch {
	case strings.Contains(s, "t20") || strings.Contains(s, "t20i") ||
		strings.Contains(s, "twenty20") || strings.Contains(s, "twenty 20"):
		format = "T20"
	case strings.Contains(s, "odi") || strings.Contains(s, "one-day international") || (strings.Contains(s, "one-day") && (strings.Contains(s, "icc") || strings.Contains(s, "world cup"))):
		format = "ODI"
	case strings.Contains(s, "one-day") || strings.Contains(s, "one day") || strings.Contains(s, "list a"):
		// Domestic one-day or list A → treat as ODI-format game
		format = "ODI"
	case strings.Contains(s, "test ") || strings.Contains(s, "test,") || strings.Contains(s, "test match"):
		format = "Test"
	case strings.Contains(s, "plunket") || strings.Contains(s, "shield") || strings.Contains(s, "county") ||
		strings.Contains(s, "first-class") || strings.Contains(s, "first class") ||
		strings.Contains(s, "ranji") || strings.Contains(s, "trophy"):
		// Multi-day domestic comps → Test-style format
		format = "Test"
	default:
		format = "-"
	}
	matchType = classifyMatchType(s)
	return format, matchType
}

// classifyMatchType tries to distinguish International, Domestic, and Women matches.
func classifyMatchType(s string) string {
	if strings.Contains(s, "women") || strings.Contains(s, "women's") {
		return "Women"
	}
	if strings.Contains(s, "icc") || strings.Contains(s, "world cup") ||
		strings.Contains(s, "asia cup") || strings.Contains(s, "championship") {
		return "International"
	}

	countryKeywords := []string{
		"india", "australia", "new zealand", "south africa", "england",
		"pakistan", "sri lanka", "bangladesh", "afghanistan", "west indies",
		"ireland", "zimbabwe", "netherlands", "scotland", "namibia",
		"nepal", "uae", "united arab emirates", "oman", "usa",
	}
	count := 0
	for _, kw := range countryKeywords {
		if strings.Contains(s, kw) {
			count++
		}
	}
	if count >= 2 {
		return "International"
	}
	return "Domestic"
}

// GetLiveMatchSections fetches the live-scores page once and returns matches grouped by section
// (e.g. "ICC Men's T20 World Cup 2026", "CSA Provincial One-Day Challenge..."). No per-match API
// calls — use this for a fast list; call GetMatchInfo for the selected match when needed.
func (c *Client) GetLiveMatchSections() ([]models.MatchSection, error) {
	return c.GetLiveMatchSectionsWithContext(context.Background())
}

// GetLiveMatchSectionsWithContext fetches grouped live matches with context support.
func (c *Client) GetLiveMatchSectionsWithContext(ctx context.Context) ([]models.MatchSection, error) {
	resp, err := c.makeRequestWithContext(ctx, CricbuzzLiveMatchesURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch live scores page: %v", err)
	}
	defer resp.Body.Close()
	if err := validateHTTPStatus(resp, CricbuzzLiveMatchesURL); err != nil {
		return nil, fmt.Errorf("failed to fetch live scores page: %w", err)
	}

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

		format, matchType := deriveFormatAndType(currentSection, shortName, title)

		// If we still couldn't detect the format (e.g. some T20Is), fall back to the match API once
		// for this match and derive from Cricbuzz's structured header.
		if format == "-" {
			if info, err := c.GetMatchInfoWithContext(ctx, uint32(matchID)); err == nil {
				hdr := info.CricbuzzInfo.MatchHeader
				switch strings.ToUpper(strings.TrimSpace(hdr.MatchFormat)) {
				case "T20", "T20I":
					format = "T20"
				case "ODI":
					format = "ODI"
				case "TEST":
					format = "Test"
				}
				// Trust header fields more than HTML heuristics for match type.
				lowerType := strings.ToLower(hdr.MatchType)
				if strings.Contains(lowerType, "women") {
					matchType = "Women"
				} else if !hdr.Domestic {
					matchType = "International"
				} else if matchType == "" {
					matchType = "Domestic"
				}
			}
		}
		item := models.MatchListItem{
			MatchID:     uint32(matchID),
			ShortName:   shortName,
			SectionName: currentSection,
			Format:      format,
			MatchType:   matchType,
			MiniScore:   "-",
		}
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
	return c.GetMatchInfoWithContext(context.Background(), matchID)
}

// GetMatchInfoWithContext fetches detailed match information for a given match ID.
func (c *Client) GetMatchInfoWithContext(ctx context.Context, matchID uint32) (models.MatchInfo, error) {
	// Construct the URL for the match API
	url := fmt.Sprintf("%s%d", CricbuzzMatchAPI, matchID)
	resp, err := c.makeRequestWithContext(ctx, url)
	if err != nil {
		return models.MatchInfo{}, fmt.Errorf("failed to fetch match info: %v", err)
	}
	defer resp.Body.Close()
	if err := validateHTTPStatus(resp, url); err != nil {
		return models.MatchInfo{}, fmt.Errorf("failed to fetch match info: %w", err)
	}

	var cricbuzzJSON models.CricbuzzJSON
	if err := json.NewDecoder(resp.Body).Decode(&cricbuzzJSON); err != nil {
		return models.MatchInfo{}, fmt.Errorf("failed to decode JSON: %v", err)
	}

	// Check if the match is complete
	scorecard, err := c.GetScorecardWithContext(ctx, matchID)
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
	return c.GetScorecardWithContext(context.Background(), matchID)
}

// GetScorecardWithContext fetches the scorecard for a given match ID with context support.
func (c *Client) GetScorecardWithContext(ctx context.Context, matchID uint32) ([]models.MatchInningsInfo, error) {
	// Construct the URL for the scorecard API
	url := fmt.Sprintf("%s%d", CricbuzzMatchScorecardAPI, matchID)
	resp, err := c.makeRequestWithContext(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scorecard: %v", err)
	}
	defer resp.Body.Close()
	if err := validateHTTPStatus(resp, url); err != nil {
		return nil, fmt.Errorf("failed to fetch scorecard: %w", err)
	}

	// Parse JSON response
	var scorecardJSON models.ScorecardJSON
	if err := json.NewDecoder(resp.Body).Decode(&scorecardJSON); err != nil {
		return nil, fmt.Errorf("failed to decode scorecard JSON: %v", err)
	}

	var scorecard []models.MatchInningsInfo

	// Convert each innings from the JSON to MatchInningsInfo
	for _, innings := range scorecardJSON.ScoreCard {
		matchInnings := models.MatchInningsInfo{
			BatTeamShortName:  innings.BatTeamDetails.BatTeamShortName,
			BowlTeamShortName: innings.BowlTeamDetails.BowlTeamShortName,
		}

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
	return c.GetOverSummariesWithContext(context.Background(), matchID)
}

// GetOverSummariesWithContext fetches over-by-over summaries with context support.
func (c *Client) GetOverSummariesWithContext(ctx context.Context, matchID uint32) (map[uint32][]models.OverSummary, error) {
	result := make(map[uint32][]models.OverSummary)

	// Step 1: fetch recent overs from over-refresh
	refreshURL := fmt.Sprintf("%s%d", CricbuzzMatchOverviewsAPI, matchID)
	resp, err := c.makeRequestWithContext(ctx, refreshURL)
	if err != nil {
		return nil, fmt.Errorf("over-refresh request failed: %v", err)
	}
	defer resp.Body.Close()
	if err := validateHTTPStatus(resp, refreshURL); err != nil {
		return nil, fmt.Errorf("over-refresh request failed: %w", err)
	}

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
		pResp, err := c.makeRequestWithContext(ctx, nextURL)
		if err != nil {
			break
		}
		if err := validateHTTPStatus(pResp, nextURL); err != nil {
			pResp.Body.Close()
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
	return c.GetFullCommentaryWithContext(context.Background(), matchID, inningsID)
}

// GetFullCommentaryWithContext fetches complete ball-by-ball commentary for a single innings.
func (c *Client) GetFullCommentaryWithContext(ctx context.Context, matchID uint32, inningsID uint32) ([]models.CommentaryEntry, error) {
	url := fmt.Sprintf("%s/api/mcenter/%d/full-commentary/%d", CricbuzzBaseAPI, matchID, inningsID)
	resp, err := c.makeRequestWithContext(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("full-commentary request failed: %v", err)
	}
	defer resp.Body.Close()
	if err := validateHTTPStatus(resp, url); err != nil {
		return nil, fmt.Errorf("full-commentary request failed: %w", err)
	}

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
