package mcp

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// SessionStatsResult holds token usage, cost, and duration for the most recent session.
type SessionStatsResult struct {
	SessionID     string  `json:"session_id"`
	ProjectName   string  `json:"project_name"`
	StartTime     string  `json:"start_time"`
	DurationMin   int     `json:"duration_minutes"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	EstimatedCost float64 `json:"estimated_cost_usd"`
}

// CostBudgetResult holds today's spend vs a configured daily budget.
type CostBudgetResult struct {
	TodaySpendUSD  float64 `json:"today_spend_usd"`
	DailyBudgetUSD float64 `json:"daily_budget_usd"`
	Remaining      float64 `json:"remaining_usd"`
	OverBudget     bool    `json:"over_budget"`
}

// RecentSessionsResult holds a list of recent sessions.
type RecentSessionsResult struct {
	Sessions []RecentSession `json:"sessions"`
}

// RecentSession holds summary data for a single session.
type RecentSession struct {
	SessionID     string  `json:"session_id"`
	ProjectName   string  `json:"project_name"`
	StartTime     string  `json:"start_time"`
	DurationMin   int     `json:"duration_minutes"`
	EstimatedCost float64 `json:"estimated_cost_usd"`
	FrictionScore int     `json:"friction_score"`
}

// SAWSessionsResult holds a list of SAW sessions.
type SAWSessionsResult struct {
	Sessions []SAWSessionSummary `json:"sessions"`
}

// SAWSessionSummary holds summary data for a single SAW session.
type SAWSessionSummary struct {
	SessionID   string `json:"session_id"`
	ProjectName string `json:"project_name"` // filepath.Base of ProjectPath, or ProjectHash if unknown
	StartTime   string `json:"start_time"`   // RFC3339 of earliest wave's StartedAt
	WaveCount   int    `json:"wave_count"`
	AgentCount  int    `json:"total_agents"`
}

// SAWWaveBreakdownResult holds per-wave timing and agent status for a SAW session.
type SAWWaveBreakdownResult struct {
	SessionID string          `json:"session_id"`
	Waves     []SAWWaveDetail `json:"waves"`
}

// SAWWaveDetail holds details for a single wave within a SAW session.
type SAWWaveDetail struct {
	Wave       int              `json:"wave"`
	AgentCount int              `json:"agent_count"`
	DurationMs int64            `json:"duration_ms"`
	StartedAt  string           `json:"started_at"` // RFC3339
	EndedAt    string           `json:"ended_at"`   // RFC3339
	Agents     []SAWAgentDetail `json:"agents"`
}

// SAWAgentDetail holds details for a single agent within a SAW wave.
type SAWAgentDetail struct {
	Agent      string `json:"agent"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
}

var (
	noArgsSchema  = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	recentNSchema = json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer","description":"Number of sessions to return (default 5)"}},"additionalProperties":false}`)
)

// addTools registers all three MCP tool handlers on s.
func addTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_session_stats",
		Description: "Token usage, cost, and duration for the most recent Claude Code session.",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetSessionStats,
	})
	s.registerTool(toolDef{
		Name:        "get_cost_budget",
		Description: "Today's total spend vs configured daily budget in USD.",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetCostBudget,
	})
	s.registerTool(toolDef{
		Name:        "get_recent_sessions",
		Description: "Last N sessions with cost, friction score, and project name.",
		InputSchema: recentNSchema,
		Handler:     s.handleGetRecentSessions,
	})
	s.registerTool(toolDef{
		Name:        "get_saw_sessions",
		Description: "Recent Claude Code sessions that used Scout-and-Wave parallel agents, with wave count and agent count.",
		InputSchema: recentNSchema,
		Handler:     s.handleGetSAWSessions,
	})
	s.registerTool(toolDef{
		Name:        "get_saw_wave_breakdown",
		Description: "Per-wave timing and agent status breakdown for a SAW session.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string","description":"Session ID from get_saw_sessions"}},"required":["session_id"],"additionalProperties":false}`),
		Handler:     s.handleGetSAWWaveBreakdown,
	})
}

// loadCacheRatio loads the stats cache and returns a CacheRatio; falls back to NoCacheRatio on error.
func (s *Server) loadCacheRatio() analyzer.CacheRatio {
	sc, err := claude.ParseStatsCache(s.claudeHome)
	if err != nil || sc == nil {
		return analyzer.NoCacheRatio()
	}
	return analyzer.ComputeCacheRatio(*sc)
}

// handleGetSessionStats returns stats for the most recent session.
func (s *Server) handleGetSessionStats(args json.RawMessage) (any, error) {
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, errors.New("no sessions found")
	}

	// Sort descending by StartTime (lexicographic on RFC3339 works correctly).
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime > sessions[j].StartTime
	})

	session := sessions[0]
	ratio := s.loadCacheRatio()
	pricing := analyzer.DefaultPricing["sonnet"]
	cost := analyzer.EstimateSessionCost(session, pricing, ratio)

	return SessionStatsResult{
		SessionID:     session.SessionID,
		ProjectName:   filepath.Base(session.ProjectPath),
		StartTime:     session.StartTime,
		DurationMin:   session.DurationMinutes,
		InputTokens:   session.InputTokens,
		OutputTokens:  session.OutputTokens,
		EstimatedCost: cost,
	}, nil
}

// handleGetCostBudget returns today's total spend vs the configured daily budget.
func (s *Server) handleGetCostBudget(args json.RawMessage) (any, error) {
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		return nil, err
	}

	today := time.Now().UTC().Format("2006-01-02")
	ratio := s.loadCacheRatio()
	pricing := analyzer.DefaultPricing["sonnet"]

	var sum float64
	for _, session := range sessions {
		if len(session.StartTime) >= 10 && session.StartTime[:10] == today {
			sum += analyzer.EstimateSessionCost(session, pricing, ratio)
		}
	}

	remaining := 0.0
	overBudget := false
	if s.budgetUSD > 0 {
		remaining = s.budgetUSD - sum
		overBudget = sum > s.budgetUSD
	}

	return CostBudgetResult{
		TodaySpendUSD:  sum,
		DailyBudgetUSD: s.budgetUSD,
		Remaining:      remaining,
		OverBudget:     overBudget,
	}, nil
}

// handleGetRecentSessions returns the last N sessions with cost and friction data.
func (s *Server) handleGetRecentSessions(args json.RawMessage) (any, error) {
	// Parse optional n argument.
	n := 5
	if len(args) > 0 && string(args) != "null" {
		var params struct {
			N *int `json:"n"`
		}
		if err := json.Unmarshal(args, &params); err == nil && params.N != nil {
			n = *params.N
		}
	}
	if n <= 0 {
		n = 5
	}
	if n > 50 {
		n = 50
	}

	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		return nil, err
	}

	facets, err := claude.ParseAllFacets(s.claudeHome)
	if err != nil {
		return nil, err
	}

	// Index facets by session ID.
	facetMap := make(map[string]*claude.SessionFacet, len(facets))
	for i := range facets {
		facetMap[facets[i].SessionID] = &facets[i]
	}

	// Sort sessions descending by start time.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime > sessions[j].StartTime
	})

	// Take first N.
	if n < len(sessions) {
		sessions = sessions[:n]
	}

	ratio := s.loadCacheRatio()
	pricing := analyzer.DefaultPricing["sonnet"]

	result := make([]RecentSession, 0, len(sessions))
	for _, session := range sessions {
		cost := analyzer.EstimateSessionCost(session, pricing, ratio)

		frictionScore := 0
		if facet, ok := facetMap[session.SessionID]; ok {
			for _, count := range facet.FrictionCounts {
				frictionScore += count
			}
		}

		result = append(result, RecentSession{
			SessionID:     session.SessionID,
			ProjectName:   filepath.Base(session.ProjectPath),
			StartTime:     session.StartTime,
			DurationMin:   session.DurationMinutes,
			EstimatedCost: cost,
			FrictionScore: frictionScore,
		})
	}

	return RecentSessionsResult{Sessions: result}, nil
}

// handleGetSAWSessions returns the last N SAW sessions with wave and agent counts.
func (s *Server) handleGetSAWSessions(args json.RawMessage) (any, error) {
	// Parse optional n argument.
	n := 5
	if len(args) > 0 && string(args) != "null" {
		var params struct {
			N *int `json:"n"`
		}
		if err := json.Unmarshal(args, &params); err == nil && params.N != nil {
			n = *params.N
		}
	}
	if n <= 0 {
		n = 5
	}
	if n > 50 {
		n = 50
	}

	spans, err := claude.ParseSessionTranscripts(s.claudeHome)
	if err != nil {
		return nil, err
	}

	sawSessions := claude.ComputeSAWWaves(spans)

	// Build project name lookup from session meta.
	metas, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		return nil, err
	}
	metaMap := make(map[string]string, len(metas))
	for _, meta := range metas {
		metaMap[meta.SessionID] = filepath.Base(meta.ProjectPath)
	}

	result := make([]SAWSessionSummary, 0, len(sawSessions))
	for _, session := range sawSessions {
		projectName := session.ProjectHash
		if name, ok := metaMap[session.SessionID]; ok {
			projectName = name
		}

		startTime := ""
		if len(session.Waves) > 0 {
			startTime = session.Waves[0].StartedAt.Format(time.RFC3339)
		}

		result = append(result, SAWSessionSummary{
			SessionID:   session.SessionID,
			ProjectName: projectName,
			StartTime:   startTime,
			WaveCount:   len(session.Waves),
			AgentCount:  session.TotalAgents,
		})
	}

	// Sort descending by StartTime (lexicographic on RFC3339 works correctly).
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartTime > result[j].StartTime
	})

	// Take first n.
	if n < len(result) {
		result = result[:n]
	}

	return SAWSessionsResult{Sessions: result}, nil
}

// handleGetSAWWaveBreakdown returns per-wave timing and agent status for a SAW session.
func (s *Server) handleGetSAWWaveBreakdown(args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil || params.SessionID == "" {
		return nil, errors.New("session_id is required")
	}

	spans, err := claude.ParseSessionTranscripts(s.claudeHome)
	if err != nil {
		return nil, err
	}

	sawSessions := claude.ComputeSAWWaves(spans)

	// Find the matching session.
	var found *claude.SAWSession
	for i := range sawSessions {
		if sawSessions[i].SessionID == params.SessionID {
			found = &sawSessions[i]
			break
		}
	}
	if found == nil {
		return nil, errors.New("session not found: " + params.SessionID)
	}

	waves := make([]SAWWaveDetail, 0, len(found.Waves))
	for _, wave := range found.Waves {
		agents := make([]SAWAgentDetail, 0, len(wave.Agents))
		for _, agent := range wave.Agents {
			agents = append(agents, SAWAgentDetail{
				Agent:      agent.Agent,
				Status:     agent.Status,
				DurationMs: agent.DurationMs,
			})
		}
		waves = append(waves, SAWWaveDetail{
			Wave:       wave.Wave,
			AgentCount: len(wave.Agents),
			DurationMs: wave.DurationMs,
			StartedAt:  wave.StartedAt.Format(time.RFC3339),
			EndedAt:    wave.EndedAt.Format(time.RFC3339),
			Agents:     agents,
		})
	}

	return SAWWaveBreakdownResult{
		SessionID: found.SessionID,
		Waves:     waves,
	}, nil
}
