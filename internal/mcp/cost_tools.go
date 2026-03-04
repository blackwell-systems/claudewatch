package mcp

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// loadAllWeightsCT loads per-session project weights from disk.
// Returns an empty map on any error (non-fatal: missing weights file is normal).
func loadAllWeightsCT(weightsPath string) map[string][]store.ProjectWeight {
	ws := store.NewSessionProjectWeightsStore(weightsPath)
	m, err := ws.Load()
	if err != nil || m == nil {
		return map[string][]store.ProjectWeight{}
	}
	return m
}

// CostSummaryResult holds aggregated cost data across time buckets and projects.
type CostSummaryResult struct {
	TodayUSD   float64        `json:"today_usd"`
	WeekUSD    float64        `json:"week_usd"`
	AllTimeUSD float64        `json:"all_time_usd"`
	ByProject  []ProjectSpend `json:"by_project"`
}

// ProjectSpend holds cost and session count aggregated for a single project.
type ProjectSpend struct {
	Project  string  `json:"project"`
	TotalUSD float64 `json:"total_usd"`
	Sessions int     `json:"sessions"`
}

// addCostTools registers the get_cost_summary handler on s.
func addCostTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_cost_summary",
		Description: "Aggregated cost data across today, this week, all time, and broken down by project.",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetCostSummary,
	})
}

// handleGetCostSummary returns aggregated cost data across today, this week,
// all time, and broken down by project.
func (s *Server) handleGetCostSummary(args json.RawMessage) (any, error) {
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		sessions = nil
	}

	tags := s.loadTags()
	weightsPath := s.weightsStorePath
	allWeights := loadAllWeightsCT(weightsPath)

	ratio := s.loadCacheRatio()
	pricing := analyzer.DefaultPricing["sonnet"]

	now := time.Now().UTC()
	todayStr := now.Format("2006-01-02")
	nowYear, nowWeek := now.ISOWeek()

	type projectAccum struct {
		totalUSD float64
		sessions int
	}
	byProject := make(map[string]*projectAccum)

	var todayUSD, weekUSD, allTimeUSD float64

	// Detect the live (in-progress) session so we can prefer its data over the
	// stale indexed version. The live session has current token counts and up-to-date
	// UserMessageTimestamps, while the indexed session-meta file may be days old.
	var liveMeta *claude.SessionMeta
	activePath, activeErr := claude.FindActiveSessionPath(s.claudeHome)
	if activeErr == nil && activePath != "" {
		parsed, parseErr := claude.ParseActiveSession(activePath)
		if parseErr == nil && parsed != nil {
			liveMeta = parsed
		}
	}

	// accumulate processes a single session's cost into the aggregate buckets.
	accumulate := func(session claude.SessionMeta) {
		cost := analyzer.EstimateSessionCost(session, pricing, ratio)
		allTimeUSD += cost

		t := lastActiveTime(session.UserMessageTimestamps, session.StartTime)
		if !t.IsZero() {
			tUTC := t.UTC()
			if tUTC.Format("2006-01-02") == todayStr {
				todayUSD += cost
			}
			sessionYear, sessionWeek := tUTC.ISOWeek()
			if sessionYear == nowYear && sessionWeek == nowWeek {
				weekUSD += cost
			}
		}

		var projectName string
		if w := allWeights[session.SessionID]; len(w) > 0 {
			projectName = sessionPrimaryProject(session.SessionID, session.ProjectPath, tags, w)
		} else {
			projectName = resolveProjectName(session.SessionID, session.ProjectPath, tags)
		}
		a, ok := byProject[projectName]
		if !ok {
			a = &projectAccum{}
			byProject[projectName] = a
		}
		a.totalUSD += cost
		a.sessions++
	}

	for _, session := range sessions {
		// Skip the indexed version of the live session — we'll use the live
		// data instead, which has current token counts and today's timestamps.
		if liveMeta != nil && session.SessionID == liveMeta.SessionID {
			continue
		}
		accumulate(session)
	}

	// Include the live session (replaces stale indexed version, or adds new).
	if liveMeta != nil {
		accumulate(*liveMeta)
	}

	if len(sessions) == 0 && len(byProject) == 0 {
		return CostSummaryResult{ByProject: []ProjectSpend{}}, nil
	}

	projectSpends := make([]ProjectSpend, 0, len(byProject))
	for name, a := range byProject {
		projectSpends = append(projectSpends, ProjectSpend{
			Project:  name,
			TotalUSD: a.totalUSD,
			Sessions: a.sessions,
		})
	}

	sort.Slice(projectSpends, func(i, j int) bool {
		return projectSpends[i].TotalUSD > projectSpends[j].TotalUSD
	})

	return CostSummaryResult{
		TodayUSD:   todayUSD,
		WeekUSD:    weekUSD,
		AllTimeUSD: allTimeUSD,
		ByProject:  projectSpends,
	}, nil
}

// lastActiveTime returns the timestamp of the most recent user message in the session,
// falling back to startTime if UserMessageTimestamps is empty. This avoids misclassifying
// long-running resumed sessions as inactive on their original start date.
func lastActiveTime(userMsgTimestamps []string, startTime string) time.Time {
	for i := len(userMsgTimestamps) - 1; i >= 0; i-- {
		t := claude.ParseTimestamp(userMsgTimestamps[i])
		if !t.IsZero() {
			return t
		}
	}
	return claude.ParseTimestamp(startTime)
}
