package mcp

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
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
	Live          bool    `json:"live"` // true when data is from an active (in-progress) session
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
	s.registerTool(toolDef{
		Name:        "get_project_health",
		Description: "Project-specific health metrics: friction rate, agent success rate, zero-commit rate, top error types, and whether a CLAUDE.md exists. Call at session start to calibrate behavior for the current project.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project":{"type":"string","description":"Project name (e.g. 'commitmux'). Omit to use the current session's project."}},"additionalProperties":false}`),
		Handler:     s.handleGetProjectHealth,
	})
	s.registerTool(toolDef{
		Name:        "get_suggestions",
		Description: "Ranked improvement suggestions based on session history: missing CLAUDE.md, recurring friction, low agent success rates, parallelization opportunities. Returns top N by impact score.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project":{"type":"string","description":"Filter suggestions for a specific project name (optional)."},"limit":{"type":"integer","description":"Maximum suggestions to return (default 5, max 20)."}},"additionalProperties":false}`),
		Handler:     s.handleGetSuggestions,
	})
	s.registerTool(toolDef{
		Name:        "get_session_friction",
		Description: "Friction events recorded for a specific session. Pass the current session ID to see what friction patterns have been logged so far this session.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string","description":"Session ID to inspect. Use the current session ID from get_session_stats."}},"required":["session_id"],"additionalProperties":false}`),
		Handler:     s.handleGetSessionFriction,
	})
	s.registerTool(toolDef{
		Name:        "set_session_project",
		Description: "Override the project name attributed to a session. Use when the session was launched from a different directory than the project being worked on (e.g. SAW worktrees). Call with the session_id from get_session_stats and the correct project name.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string","description":"The session ID to tag. Use the session_id from get_session_stats."},"project_name":{"type":"string","description":"The project name to attribute this session to (e.g. 'brewprune')."}},"required":["session_id","project_name"],"additionalProperties":false}`),
		Handler:     s.handleSetSessionProject,
	})
	addAnalyticsTools(s)
	addCostTools(s)
	addVelocityTools(s)
	addLiveTools(s)
	addContextTools(s)
	addCostVelocityTools(s)
	addDashboardTools(s)
	addDriftTools(s)
	addTranscriptTools(s)
	addAnomalyTools(s)
	addRegressionTools(s)
	addCorrelateTools(s)
	addAttributionTools(s)
	addMultiProjectTools(s)
	s.registerTool(toolDef{
		Name:        "get_project_comparison",
		Description: "All projects compared side by side in a single call. Returns a ranked list of all projects with health score, friction rate, has_claude_md, agent success rate, and session count.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"min_sessions":{"type":"integer","description":"Minimum session count to include a project (default 0 = no filter)"}},"additionalProperties":false}`),
		Handler:     s.handleGetProjectComparison,
	})
	s.registerTool(toolDef{
		Name:        "get_stale_patterns",
		Description: "Chronic recurring friction: friction types that appear in >N% of recent sessions AND have no corresponding CLAUDE.md change in the past K sessions. Returns a ranked list by recurrence rate.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"threshold":{"type":"number","description":"Minimum recurrence rate to flag a pattern (default 0.3)"},"lookback":{"type":"integer","description":"Number of recent sessions to analyze (default 10)"}},"additionalProperties":false}`),
		Handler:     s.handleGetStalePatterns,
	})
}

// loadTags loads the session tag override store.
// Returns an empty map on any error (non-fatal: missing tag file is normal).
func (s *Server) loadTags() map[string]string {
	ts := store.NewSessionTagStore(s.tagStorePath)
	tags, err := ts.Load()
	if err != nil || tags == nil {
		return map[string]string{}
	}
	return tags
}

// loadProjectWeights loads the session project weights store.
// Returns an empty map on any error (non-fatal: missing weights file is normal).
func (s *Server) loadProjectWeights() map[string][]store.ProjectWeight {
	ws := store.NewSessionProjectWeightsStore(s.weightsStorePath)
	weights, err := ws.Load()
	if err != nil || weights == nil {
		return map[string][]store.ProjectWeight{}
	}
	return weights
}

// resolveProjectName returns tags[sessionID] if an override exists,
// falling back to filepath.Base(projectPath).
func resolveProjectName(sessionID, projectPath string, tags map[string]string) string {
	if name, ok := tags[sessionID]; ok && name != "" {
		return name
	}
	return filepath.Base(projectPath)
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
// It first checks for an active (live) session using FindActiveSessionPath;
// if one is found, it returns its stats with Live: true without touching the DB.
// Only if no active session is found does it fall through to the closed-session path.
func (s *Server) handleGetSessionStats(args json.RawMessage) (any, error) {
	ratio := s.loadCacheRatio()
	pricing := analyzer.DefaultPricing["sonnet"]

	// Step 1: check for an active (live) session.
	activePath, err := claude.FindActiveSessionPath(s.claudeHome)
	if err == nil && activePath != "" {
		// Step 2: parse the active session.
		meta, err := claude.ParseActiveSession(activePath)
		if err == nil && meta != nil {
			// Step 3: build and return the live result. Do NOT write to the DB.
			cost := analyzer.EstimateSessionCost(*meta, pricing, ratio)
			tags := s.loadTags()
			return SessionStatsResult{
				SessionID:     meta.SessionID,
				ProjectName:   resolveProjectName(meta.SessionID, meta.ProjectPath, tags),
				StartTime:     meta.StartTime,
				DurationMin:   meta.DurationMinutes, // 0 for live sessions — expected
				InputTokens:   meta.InputTokens,
				OutputTokens:  meta.OutputTokens,
				EstimatedCost: cost,
				Live:          true,
			}, nil
		}
		// ParseActiveSession error is non-fatal; fall through to closed-session path.
	}
	// FindActiveSessionPath error is non-fatal; fall through to closed-session path.

	// Step 4: closed-session fallback — existing logic.
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

	tags := s.loadTags()
	session := sessions[0]
	cost := analyzer.EstimateSessionCost(session, pricing, ratio)

	return SessionStatsResult{
		SessionID:     session.SessionID,
		ProjectName:   resolveProjectName(session.SessionID, session.ProjectPath, tags),
		StartTime:     session.StartTime,
		DurationMin:   session.DurationMinutes,
		InputTokens:   session.InputTokens,
		OutputTokens:  session.OutputTokens,
		EstimatedCost: cost,
		Live:          false,
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

	tags := s.loadTags()
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
			ProjectName:   resolveProjectName(session.SessionID, session.ProjectPath, tags),
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
	tags := s.loadTags()
	metaMap := make(map[string]string, len(metas))
	for _, meta := range metas {
		metaMap[meta.SessionID] = resolveProjectName(meta.SessionID, meta.ProjectPath, tags)
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
