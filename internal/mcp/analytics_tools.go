package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// AgentPerformanceResult holds agent performance metrics returned by get_agent_performance.
type AgentPerformanceResult struct {
	TotalAgents       int                            `json:"total_agents"`
	SuccessRate       float64                        `json:"success_rate"`
	KillRate          float64                        `json:"kill_rate"`
	BackgroundRatio   float64                        `json:"background_ratio"`
	AvgDurationMs     float64                        `json:"avg_duration_ms"`
	AvgTokensPerAgent float64                        `json:"avg_tokens_per_agent"`
	ParallelSessions  int                            `json:"parallel_sessions"`
	ByType            map[string]AgentTypePerfDetail `json:"by_type"`
}

// AgentTypePerfDetail holds per-type agent performance stats.
type AgentTypePerfDetail struct {
	Count         int     `json:"count"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	AvgTokens     float64 `json:"avg_tokens"`
}

// EffectivenessEntry holds the CLAUDE.md effectiveness result for a single project.
type EffectivenessEntry struct {
	Project          string  `json:"project"`
	Verdict          string  `json:"verdict"`
	Score            int     `json:"score"`
	FrictionDelta    float64 `json:"friction_delta"`
	ToolErrorDelta   float64 `json:"tool_error_delta"`
	BeforeSessions   int     `json:"before_sessions"`
	AfterSessions    int     `json:"after_sessions"`
	ChangeDetectedAt string  `json:"change_detected_at"` // RFC3339
}

// AllEffectivenessResult holds effectiveness results for all qualifying projects.
type AllEffectivenessResult struct {
	Projects []EffectivenessEntry `json:"projects"`
}

// addAnalyticsTools registers the get_agent_performance and get_effectiveness handlers on s.
func addAnalyticsTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_agent_performance",
		Description: "Agent performance metrics across all session transcripts.",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetAgentPerformance,
	})
	s.registerTool(toolDef{
		Name:        "get_effectiveness",
		Description: "CLAUDE.md effectiveness scores for each project with before/after session data.",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetEffectiveness,
	})
}

// handleGetAgentPerformance returns agent performance metrics computed from session transcripts.
// Arguments are ignored (noArgsSchema).
func (s *Server) handleGetAgentPerformance(args json.RawMessage) (any, error) {
	tasks, err := claude.ParseAgentTasks(s.claudeHome)
	if err != nil {
		// Non-fatal: return zero-value result.
		tasks = nil
	}

	perf := analyzer.AnalyzeAgents(tasks)

	byType := make(map[string]AgentTypePerfDetail, len(perf.ByType))
	for agentType, stats := range perf.ByType {
		byType[agentType] = AgentTypePerfDetail{
			Count:         stats.Count,
			SuccessRate:   stats.SuccessRate,
			AvgDurationMs: stats.AvgDurationMs,
			AvgTokens:     stats.AvgTokens,
		}
	}

	return AgentPerformanceResult{
		TotalAgents:       perf.TotalAgents,
		SuccessRate:       perf.SuccessRate,
		KillRate:          perf.KillRate,
		BackgroundRatio:   perf.BackgroundRatio,
		AvgDurationMs:     perf.AvgDurationMs,
		AvgTokensPerAgent: perf.AvgTokensPerAgent,
		ParallelSessions:  perf.ParallelSessions,
		ByType:            byType,
	}, nil
}

// handleGetEffectiveness returns CLAUDE.md effectiveness scores for each qualifying project.
// Arguments are ignored (noArgsSchema).
func (s *Server) handleGetEffectiveness(args json.RawMessage) (any, error) {
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		sessions = nil
	}

	facets, err := claude.ParseAllFacets(s.claudeHome)
	if err != nil {
		facets = nil
	}

	pricing := analyzer.DefaultPricing["sonnet"]
	ratio := s.loadCacheRatio()

	// Discover unique project paths from sessions.
	projectSet := make(map[string]struct{})
	for _, sess := range sessions {
		normalized := claude.NormalizePath(sess.ProjectPath)
		if normalized != "" {
			projectSet[normalized] = struct{}{}
		}
	}

	entries := []EffectivenessEntry{}

	for projectPath := range projectSet {
		// Check for CLAUDE.md in the project directory.
		claudeMDPath := filepath.Join(projectPath, "CLAUDE.md")
		info, statErr := os.Stat(claudeMDPath)
		if statErr != nil {
			// No CLAUDE.md — skip this project.
			continue
		}
		modTime := info.ModTime()

		// Gather sessions for this project.
		var projectSessions []claude.SessionMeta
		for _, sess := range sessions {
			if claude.NormalizePath(sess.ProjectPath) == projectPath {
				projectSessions = append(projectSessions, sess)
			}
		}

		result := analyzer.AnalyzeEffectiveness(
			projectPath,
			modTime,
			projectSessions,
			facets,
			pricing,
			ratio,
		)

		// Skip projects with no before/after session data at all.
		if result.BeforeSessions == 0 && result.AfterSessions == 0 {
			continue
		}

		entries = append(entries, EffectivenessEntry{
			Project:          result.ProjectName,
			Verdict:          result.Verdict,
			Score:            result.Score,
			FrictionDelta:    result.FrictionDelta,
			ToolErrorDelta:   result.ToolErrorDelta,
			BeforeSessions:   result.BeforeSessions,
			AfterSessions:    result.AfterSessions,
			ChangeDetectedAt: result.ChangeDetectedAt.Format(time.RFC3339),
		})
	}

	return AllEffectivenessResult{Projects: entries}, nil
}
