package mcp

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

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

	// Build a set of indexed session IDs for deduplication against the live session.
	indexedIDs := make(map[string]bool, len(sessions))
	for _, session := range sessions {
		indexedIDs[session.SessionID] = true
	}

	for _, session := range sessions {
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

		projectName := filepath.Base(session.ProjectPath)
		a, ok := byProject[projectName]
		if !ok {
			a = &projectAccum{}
			byProject[projectName] = a
		}
		a.totalUSD += cost
		a.sessions++
	}

	// Attempt to include the live (in-progress) session. Errors are non-fatal.
	activePath, activeErr := claude.FindActiveSessionPath(s.claudeHome)
	if activeErr == nil && activePath != "" {
		liveMeta, parseErr := claude.ParseActiveSession(activePath)
		if parseErr == nil && liveMeta != nil && !indexedIDs[liveMeta.SessionID] {
			liveCost := analyzer.EstimateSessionCost(*liveMeta, pricing, ratio)
			allTimeUSD += liveCost

			t := lastActiveTime(liveMeta.UserMessageTimestamps, liveMeta.StartTime)
			if !t.IsZero() {
				tUTC := t.UTC()
				if tUTC.Format("2006-01-02") == todayStr {
					todayUSD += liveCost
				}
				sessionYear, sessionWeek := tUTC.ISOWeek()
				if sessionYear == nowYear && sessionWeek == nowWeek {
					weekUSD += liveCost
				}
			}

			projectName := filepath.Base(liveMeta.ProjectPath)
			a, ok := byProject[projectName]
			if !ok {
				a = &projectAccum{}
				byProject[projectName] = a
			}
			a.totalUSD += liveCost
			a.sessions++
		}
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
