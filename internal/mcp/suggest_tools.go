package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/suggest"
)

// SuggestionItem is the MCP-exposed shape of a single suggestion.
type SuggestionItem struct {
	Category    string  `json:"category"`
	Priority    int     `json:"priority"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	ImpactScore float64 `json:"impact_score"`
}

// SuggestionsResult is the MCP response for get_suggestions.
type SuggestionsResult struct {
	Suggestions []SuggestionItem `json:"suggestions"`
	TotalCount  int              `json:"total_count"`
	Project     string           `json:"project,omitempty"`
}

const (
	defaultSuggestLimit = 5
	maxSuggestLimit     = 20
	recurringThreshold  = 0.3
)

// handleGetSuggestions implements the get_suggestions MCP tool.
// It accepts optional "project" (string) and "limit" (int, default 5, max 20) arguments.
func (s *Server) handleGetSuggestions(args json.RawMessage) (any, error) {
	// Parse optional arguments.
	var params struct {
		Project *string `json:"project"`
		Limit   *int    `json:"limit"`
	}
	if len(args) > 0 && string(args) != "null" {
		_ = json.Unmarshal(args, &params)
	}

	limit := defaultSuggestLimit
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
	}
	if limit > maxSuggestLimit {
		limit = maxSuggestLimit
	}

	project := ""
	if params.Project != nil {
		project = *params.Project
	}

	// Build analysis context — non-fatal errors use zero values.
	ctx := s.buildSuggestContext()

	// Run the suggestion engine.
	engine := suggest.NewEngine()
	raw := engine.Run(ctx)

	// Filter by project if specified.
	if project != "" {
		raw = filterSuggestionsByProject(raw, project)
	}

	totalCount := len(raw)

	// Apply limit.
	if limit < totalCount {
		raw = raw[:limit]
	}

	// Convert to MCP result type.
	items := make([]SuggestionItem, 0, len(raw))
	for _, r := range raw {
		items = append(items, SuggestionItem{
			Category:    r.Category,
			Priority:    r.Priority,
			Title:       r.Title,
			Description: r.Description,
			ImpactScore: r.ImpactScore,
		})
	}

	return SuggestionsResult{
		Suggestions: items,
		TotalCount:  totalCount,
		Project:     project,
	}, nil
}

// buildSuggestContext constructs the AnalysisContext inline from session metadata and
// related data, without importing internal/app.
func (s *Server) buildSuggestContext() *suggest.AnalysisContext {
	// --- Sessions ---
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		sessions = nil
	}

	// --- Facets ---
	facets, err := claude.ParseAllFacets(s.claudeHome)
	if err != nil {
		facets = nil
	}

	// --- Settings (for hook count) ---
	settings, err := claude.ParseSettings(s.claudeHome)
	if err != nil {
		settings = nil
	}

	// --- Commands ---
	commands, err := claude.ListCommands(s.claudeHome)
	if err != nil {
		commands = nil
	}

	// --- Plugins ---
	plugins, err := claude.ParsePlugins(s.claudeHome)
	if err != nil {
		plugins = nil
	}

	// --- Agent tasks ---
	agentTasks, err := claude.ParseAgentTasks(s.claudeHome)
	if err != nil {
		agentTasks = nil
	}

	// Average tool errors across sessions.
	var totalToolErrors int
	for _, sess := range sessions {
		totalToolErrors += sess.ToolErrors
	}
	avgToolErrors := 0.0
	if len(sessions) > 0 {
		avgToolErrors = float64(totalToolErrors) / float64(len(sessions))
	}

	// Build session -> project index for cross-referencing.
	sessionProject := make(map[string]string, len(sessions))
	for _, sess := range sessions {
		sessionProject[sess.SessionID] = sess.ProjectPath
	}

	// Recurring friction (>30% of faceted sessions).
	frictionSessionCount := make(map[string]int)
	for _, f := range facets {
		for frictionType := range f.FrictionCounts {
			frictionSessionCount[frictionType]++
		}
	}
	var recurringFriction []string
	if len(facets) > 0 {
		for frictionType, count := range frictionSessionCount {
			if float64(count)/float64(len(facets)) > recurringThreshold {
				recurringFriction = append(recurringFriction, frictionType)
			}
		}
	}

	// Hook count.
	hookCount := 0
	if settings != nil {
		for _, groups := range settings.Hooks {
			hookCount += len(groups)
		}
	}

	// Agent stats.
	agentTypeStats := make(map[string]float64)
	agentOverallSuccess := 0.0
	if len(agentTasks) > 0 {
		typeCount := make(map[string]int)
		typeSuccess := make(map[string]int)
		totalSuccess := 0
		for _, task := range agentTasks {
			typeCount[task.AgentType]++
			if task.Status == "completed" {
				typeSuccess[task.AgentType]++
				totalSuccess++
			}
		}
		agentOverallSuccess = float64(totalSuccess) / float64(len(agentTasks))
		for agentType, count := range typeCount {
			agentTypeStats[agentType] = float64(typeSuccess[agentType]) / float64(count)
		}
	}

	// Build project contexts from session metadata (no scanner).
	projectSessions := make(map[string][]claude.SessionMeta)
	for _, sess := range sessions {
		key := claude.NormalizePath(sess.ProjectPath)
		projectSessions[key] = append(projectSessions[key], sess)
	}

	projectContexts := make([]suggest.ProjectContext, 0, len(projectSessions))
	for projPath, projSessions := range projectSessions {
		var toolErrors, interruptions, agentCount, sequentialCount int
		hasFacets := false

		for _, sess := range projSessions {
			toolErrors += sess.ToolErrors
			interruptions += sess.UserInterruptions
		}

		// Check facets for this project.
		for _, f := range facets {
			if claude.NormalizePath(sessionProject[f.SessionID]) == projPath {
				hasFacets = true
			}
		}

		// Count agent tasks for this project.
		for _, task := range agentTasks {
			if claude.NormalizePath(sessionProject[task.SessionID]) == projPath {
				agentCount++
				if !task.Background {
					sequentialCount++
				}
			}
		}

		// Check if CLAUDE.md exists in the project directory.
		claudeMDPath := filepath.Join(projPath, "CLAUDE.md")
		hasClaudeMD := false
		if _, statErr := os.Stat(claudeMDPath); statErr == nil {
			hasClaudeMD = true
		}

		projectContexts = append(projectContexts, suggest.ProjectContext{
			Path:          projPath,
			Name:          filepath.Base(projPath),
			HasClaudeMD:   hasClaudeMD,
			SessionCount:  len(projSessions),
			ToolErrors:    toolErrors,
			Interruptions: interruptions,
			Score:         0.0, // not available without scanner
			HasFacets:     hasFacets,
			AgentCount:    agentCount,
			SequentialCount: sequentialCount,
		})
	}

	// Commit analysis for zero-commit rate.
	commitAnalysis := analyzer.AnalyzeCommits(sessions)

	// Cost analysis for cache savings.
	var cacheSavingsPercent, totalCost float64
	statsCache, err := claude.ParseStatsCache(s.claudeHome)
	if err == nil && statsCache != nil {
		totalCommits := 0
		for _, sess := range sessions {
			totalCommits += sess.GitCommits
		}
		costEst := analyzer.EstimateCosts(*statsCache, "", len(sessions), totalCommits)
		cacheSavingsPercent = costEst.CacheSavingsPercent
		totalCost = costEst.TotalCost
	}

	return &suggest.AnalysisContext{
		Projects:            projectContexts,
		TotalSessions:       len(sessions),
		AvgToolErrors:       avgToolErrors,
		RecurringFriction:   recurringFriction,
		HookCount:           hookCount,
		CommandCount:        len(commands),
		PluginCount:         len(plugins),
		AgentSuccessRate:    agentOverallSuccess,
		AgentTypeStats:      agentTypeStats,
		CustomMetricTrends:  make(map[string]string),
		// ClaudeMDSectionCorrelation is left nil (no project scanner available)
		ZeroCommitRate:      commitAnalysis.ZeroCommitRate,
		CacheSavingsPercent: cacheSavingsPercent,
		TotalCost:           totalCost,
	}
}

// filterSuggestionsByProject keeps only suggestions whose Title or Description
// contains the given project name.
func filterSuggestionsByProject(suggestions []suggest.Suggestion, project string) []suggest.Suggestion {
	var filtered []suggest.Suggestion
	for _, s := range suggestions {
		if strings.Contains(s.Title, project) || strings.Contains(s.Description, project) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
