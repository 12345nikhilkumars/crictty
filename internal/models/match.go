package models

import "time"

// BowlerInfo contains bowling statistics for a player in a single innings
type BowlerInfo struct {
	Name    string
	Overs   string
	Maidens string
	Runs    string
	Wickets string
	NoBalls string
	Wides   string
	Economy string
}

// BatsmanInfo contains batting statistics for a player in a single innings
type BatsmanInfo struct {
	Name       string
	Status     string
	Runs       string
	Balls      string
	Fours      string
	Sixes      string
	StrikeRate string
}

// MatchInningsInfo holds all batting and bowling details for an innings
type MatchInningsInfo struct {
	BatTeamShortName  string
	BowlTeamShortName string
	BatsmanDetails    []BatsmanInfo
	YetToBat          string
	BowlerDetails     []BowlerInfo
}

// CricbuzzMiniscore contains live match summary information.
type CricbuzzMiniscore struct {
	InningsID         uint32            `json:"inningsId"`
	BatsmanStriker    Batsman           `json:"batsmanStriker"`
	BatsmanNonStriker Batsman           `json:"batsmanNonStriker"`
	BowlerStriker     Bowler            `json:"bowlerStriker"`
	BowlerNonStriker  Bowler            `json:"bowlerNonStriker"`
	Overs             float32           `json:"overs"`
	RecentOvsStats    string            `json:"recentOvsStats"`
	CurrentRunRate    float32           `json:"currentRunRate"`
	RequiredRunRate   float32           `json:"requiredRunRate"`
	LastWicket        *string           `json:"lastWicket"`
	MatchScoreDetails MatchScoreDetails `json:"matchScoreDetails"`
	OversRem          *float32          `json:"oversRem"`
	Status            string            `json:"status"`
}

// Batsman represents batting stats for a player as received from the API
type Batsman struct {
	BatBalls      uint32 `json:"balls"`
	BatDots       uint32 `json:"dots"`
	BatFours      uint32 `json:"fours"`
	BatID         uint32 `json:"id"`
	BatName       string `json:"name"`
	BatMins       uint32 `json:"mins"`
	BatSixes      uint32 `json:"sixes"`
	BatStrikeRate string `json:"strikeRate"`
	BatRuns       uint32 `json:"runs"`
}

// Bowler represents bowling stats for a player as received from the API
type Bowler struct {
	BowlID      uint32  `json:"id"`
	BowlName    string  `json:"name"`
	BowlMaidens uint32  `json:"maidens"`
	BowlNoballs uint32  `json:"noballs"`
	BowlOvs     float32 `json:"overs"`
	BowlRuns    uint32  `json:"runs"`
	BowlWides   uint32  `json:"wides"`
	BowlWkts    uint32  `json:"wickets"`
	BowlEcon    float32 `json:"economy"`
}

// MatchScoreDetails contains overall match score and innings summary
type MatchScoreDetails struct {
	MatchID          uint32         `json:"matchId"`
	MatchFormat      string         `json:"matchFormat"`
	State            string         `json:"state"`
	CustomStatus     string         `json:"customStatus"`
	InningsScoreList []InningsScore `json:"inningsScoreList"`
}

// InningsScore contains summary of runs, wickets, and overs for an innings
type InningsScore struct {
	InningsID   uint32  `json:"inningsId"`
	BatTeamID   uint32  `json:"batTeamId"`
	BatTeamName string  `json:"batTeamName"`
	Score       uint32  `json:"score"`
	Wickets     uint32  `json:"wickets"`
	Overs       float32 `json:"overs"`
	IsDeclared  bool    `json:"isDeclared"`
	IsFollowOn  bool    `json:"isFollowOn"`
}

// MatchHeader contains metadata about the match
type MatchHeader struct {
	MatchID                uint32  `json:"matchId"`
	MatchDescription       string  `json:"matchDescription"`
	MatchFormat            string  `json:"matchFormat"`
	MatchType              string  `json:"matchType"`
	Complete               bool    `json:"complete"`
	Domestic               bool    `json:"domestic"`
	MatchStartTimestamp    uint64  `json:"matchStartTimestamp"`
	MatchCompleteTimestamp uint64  `json:"matchCompleteTimestamp"`
	DayNight               *bool   `json:"dayNight"`
	Year                   uint32  `json:"year"`
	DayNumber              *uint32 `json:"dayNumber"`
	State                  string  `json:"state"`
	Status                 string  `json:"status"`
	Team1                  Team    `json:"team1"`
	Team2                  Team    `json:"team2"`
	SeriesDesc             string  `json:"seriesDesc"`
	SeriesID               uint32  `json:"seriesId"`
	SeriesName             string  `json:"seriesName"`
}

// Team contains team identification and names
type Team struct {
	ID        uint32 `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
}

// CricbuzzJSON contains match header, miniscore, and page info
type CricbuzzJSON struct {
	MatchHeader MatchHeader       `json:"matchHeader"`
	Miniscore   CricbuzzMiniscore `json:"miniscore"`
	Page        string            `json:"page"`
}

// MatchListItem is a match entry in a section list (no full details).
type MatchListItem struct {
	MatchID     uint32
	ShortName   string
	SectionName string
	Format      string // e.g. "Test", "ODI", "T20"
	MatchType   string // e.g. "Intl", "Domestic", "Women"
	MiniScore   string // e.g. "401/6" or "-" when unknown
}

// MatchSection is a named group of matches (e.g. "ICC Men's T20 World Cup 2026").
type MatchSection struct {
	Name    string
	Matches []MatchListItem
}

// OverSummary holds per-over score progression for an innings
type OverSummary struct {
	InningsID   uint32 `json:"inningsId"`
	Overs       int    `json:"overs"`
	Runs        int    `json:"runs"`
	Score       int    `json:"score"`
	Wickets     int    `json:"wickets"`
	OvrSummary  string `json:"ovrSummary"`
	Timestamp   int64  `json:"timestamp"`
	BatTeamName string `json:"batTeamName"`
}

// CommentaryEntry is a single ball/event from the full-commentary API
type CommentaryEntry struct {
	CommText    string `json:"commText"`
	OverNumber  string `json:"overNumber"`
	InningsID   uint32 `json:"inningsId"`
	Event       string `json:"event"`
	Timestamp   int64  `json:"timestamp"`
	BatTeamName string `json:"batTeamName"`
}

// MatchInfo contains match metadata, live data, scorecard, over summaries, and commentary
type MatchInfo struct {
	MatchShortName       string
	CricbuzzMatchID      uint32
	CricbuzzMatchAPILink string
	CricbuzzInfo         CricbuzzJSON
	Scorecard            []MatchInningsInfo
	OverSummaries        map[uint32][]OverSummary // inningsId -> sorted over summaries
	Commentary           []CommentaryEntry        // commentary for the viewed innings
	LastUpdated          time.Time
}

// ScorecardJSON represents the new JSON structure from the scorecard API
type ScorecardJSON struct {
	ScoreCard   []ScorecardInnings `json:"scoreCard"`
	MatchHeader MatchHeader        `json:"matchHeader"`
	Status      string             `json:"status"`
}

// ScorecardInnings represents a single innings in the scorecard
type ScorecardInnings struct {
	MatchID         uint32          `json:"matchId"`
	InningsID       uint32          `json:"inningsId"`
	BatTeamDetails  BatTeamDetails  `json:"batTeamDetails"`
	BowlTeamDetails BowlTeamDetails `json:"bowlTeamDetails"`
	ScoreDetails    ScoreDetails    `json:"scoreDetails"`
}

// BatTeamDetails contains batting team info and batsmen data
type BatTeamDetails struct {
	BatTeamID        uint32                 `json:"batTeamId"`
	BatTeamName      string                 `json:"batTeamName"`
	BatTeamShortName string                 `json:"batTeamShortName"`
	BatsmenData      map[string]BatsmanData `json:"batsmenData"`
}

// BatsmanData represents individual batsman statistics
type BatsmanData struct {
	BatID        uint32  `json:"batId"`
	BatName      string  `json:"batName"`
	BatShortName string  `json:"batShortName"`
	Runs         uint32  `json:"runs"`
	Balls        uint32  `json:"balls"`
	Dots         uint32  `json:"dots"`
	Fours        uint32  `json:"fours"`
	Sixes        uint32  `json:"sixes"`
	Mins         uint32  `json:"mins"`
	StrikeRate   float64 `json:"strikeRate"`
	OutDesc      string  `json:"outDesc"`
	IsCaptain    bool    `json:"isCaptain"`
	IsKeeper     bool    `json:"isKeeper"`
}

// BowlTeamDetails contains bowling team info and bowlers data
type BowlTeamDetails struct {
	BowlTeamID        uint32                `json:"bowlTeamId"`
	BowlTeamName      string                `json:"bowlTeamName"`
	BowlTeamShortName string                `json:"bowlTeamShortName"`
	BowlersData       map[string]BowlerData `json:"bowlersData"`
}

// BowlerData represents individual bowler statistics
type BowlerData struct {
	BowlerID      uint32  `json:"bowlerId"`
	BowlName      string  `json:"bowlName"`
	BowlShortName string  `json:"bowlShortName"`
	Overs         float64 `json:"overs"`
	Maidens       uint32  `json:"maidens"`
	Runs          uint32  `json:"runs"`
	Wickets       uint32  `json:"wickets"`
	Economy       float64 `json:"economy"`
	NoBalls       uint32  `json:"no_balls"`
	Wides         uint32  `json:"wides"`
	Dots          uint32  `json:"dots"`
	Balls         uint32  `json:"balls"`
	IsCaptain     bool    `json:"isCaptain"`
	IsKeeper      bool    `json:"isKeeper"`
}

// ScoreDetails contains innings summary
type ScoreDetails struct {
	BallNbr      uint32  `json:"ballNbr"`
	Overs        float64 `json:"overs"`
	RevisedOvers uint32  `json:"revisedOvers"`
	RunRate      float64 `json:"runRate"`
	Runs         uint32  `json:"runs"`
	Wickets      uint32  `json:"wickets"`
	RunsPerBall  float64 `json:"runsPerBall"`
	IsDeclared   bool    `json:"isDeclared"`
	IsFollowOn   bool    `json:"isFollowOn"`
}
