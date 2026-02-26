package fixer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

// FixContext holds all data needed by fix rules to generate proposed additions.
type FixContext struct {
	// Project is the target project metadata.
	Project scanner.Project

	// Sessions contains all session metadata for this project.
	Sessions []claude.SessionMeta

	// Facets contains all qualitative facets for this project's sessions.
	Facets []claude.SessionFacet

	// AgentTasks contains all agent tasks for this project's sessions.
	AgentTasks []claude.AgentTask

	// ExistingClaudeMD is the current CLAUDE.md content (empty if none exists).
	ExistingClaudeMD string

	// Pre-computed analysis results.
	ClaudeMDQuality  *analyzer.ClaudeMDQuality
	FrictionPatterns *analyzer.PersistenceAnalysis
	CommitAnalysis   *analyzer.CommitAnalysis
	ToolProfile      *analyzer.ToolProfile
	ConversationData *analyzer.ConversationAnalysis
}

// BuildFixContext loads all session data and runs pre-computed analyses
// for a single project. It returns a FixContext ready for rule evaluation.
func BuildFixContext(project scanner.Project, cfg *config.Config) (*FixContext, error) {
	ctx := &FixContext{
		Project: project,
	}

	// Load all session metadata.
	allSessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("parsing session meta: %w", err)
	}

	// Filter sessions for this project.
	ctx.Sessions = filterSessionsByProject(allSessions, project.Path)

	// Load all facets.
	allFacets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("parsing facets: %w", err)
	}

	// Filter facets for this project's sessions.
	ctx.Facets = filterFacetsByProject(allFacets, ctx.Sessions)

	// Load agent tasks.
	allTasks, err := claude.ParseAgentTasks(cfg.ClaudeHome)
	if err != nil {
		// Non-fatal: agent tasks may not exist.
		allTasks = nil
	}
	ctx.AgentTasks = filterAgentTasksByProject(allTasks, ctx.Sessions)

	// Read existing CLAUDE.md content.
	claudeMDPath := filepath.Join(project.Path, "CLAUDE.md")
	if data, err := os.ReadFile(claudeMDPath); err == nil {
		ctx.ExistingClaudeMD = string(data)
	}

	// Run pre-computed analyses.

	// ClaudeMD quality for this project.
	projects := []scanner.Project{project}
	claudeMDAnalysis := analyzer.AnalyzeClaudeMDEffectiveness(projects, ctx.Facets)
	if len(claudeMDAnalysis.Projects) > 0 {
		quality := claudeMDAnalysis.Projects[0]
		ctx.ClaudeMDQuality = &quality
	}

	// Friction persistence.
	persistence := analyzer.AnalyzeFrictionPersistence(ctx.Facets, ctx.Sessions)
	ctx.FrictionPatterns = &persistence

	// Commit analysis.
	commits := analyzer.AnalyzeCommits(ctx.Sessions)
	ctx.CommitAnalysis = &commits

	// Tool profile.
	toolAnalysis := analyzer.AnalyzeToolUsage(ctx.Sessions, projects)
	if len(toolAnalysis.Projects) > 0 {
		profile := toolAnalysis.Projects[0]
		ctx.ToolProfile = &profile
	}

	// Conversation analysis.
	convAnalysis, err := analyzer.AnalyzeConversations(cfg.ClaudeHome)
	if err == nil {
		// Filter conversation metrics to this project's sessions.
		filtered := filterConversationsByProject(convAnalysis, ctx.Sessions)
		ctx.ConversationData = &filtered
	}

	return ctx, nil
}

// hasSection checks whether the existing CLAUDE.md content contains a
// section header matching any of the given keywords (case-insensitive).
func hasSection(content string, keywords ...string) bool {
	lower := strings.ToLower(content)
	for _, kw := range keywords {
		// Check for ## header containing the keyword.
		lines := strings.Split(lower, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "##") && strings.Contains(trimmed, strings.ToLower(kw)) {
				return true
			}
		}
	}
	return false
}

// confidenceFromSessionCount returns a confidence score based on the number
// of sessions: 0.9 for >10, 0.7 for 5-10, 0.5 for <5.
func confidenceFromSessionCount(n int) float64 {
	switch {
	case n > 10:
		return 0.9
	case n >= 5:
		return 0.7
	default:
		return 0.5
	}
}

// filterSessionsByProject returns sessions whose ProjectPath matches the given path.
func filterSessionsByProject(sessions []claude.SessionMeta, projectPath string) []claude.SessionMeta {
	normalized := strings.TrimRight(projectPath, "/")
	var result []claude.SessionMeta
	for _, s := range sessions {
		if strings.TrimRight(s.ProjectPath, "/") == normalized {
			result = append(result, s)
		}
	}
	return result
}

// filterFacetsByProject returns facets whose session ID matches a session in the list.
func filterFacetsByProject(facets []claude.SessionFacet, sessions []claude.SessionMeta) []claude.SessionFacet {
	ids := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		ids[s.SessionID] = true
	}
	var result []claude.SessionFacet
	for _, f := range facets {
		if ids[f.SessionID] {
			result = append(result, f)
		}
	}
	return result
}

// filterAgentTasksByProject returns agent tasks whose session ID matches a
// session in the list.
func filterAgentTasksByProject(tasks []claude.AgentTask, sessions []claude.SessionMeta) []claude.AgentTask {
	ids := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		ids[s.SessionID] = true
	}
	var result []claude.AgentTask
	for _, t := range tasks {
		if ids[t.SessionID] {
			result = append(result, t)
		}
	}
	return result
}

// filterConversationsByProject returns a ConversationAnalysis containing only
// metrics for sessions that belong to the target project.
func filterConversationsByProject(full analyzer.ConversationAnalysis, sessions []claude.SessionMeta) analyzer.ConversationAnalysis {
	ids := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		ids[s.SessionID] = true
	}

	var filtered []analyzer.ConversationMetrics
	for _, cm := range full.Sessions {
		if ids[cm.SessionID] {
			filtered = append(filtered, cm)
		}
	}

	// Recompute aggregates for the filtered set.
	result := analyzer.ConversationAnalysis{
		Sessions: filtered,
	}

	if len(filtered) == 0 {
		return result
	}

	var totalCorrectionRate, totalLongMsgRate float64
	for _, s := range filtered {
		totalCorrectionRate += s.CorrectionRate
		totalLongMsgRate += s.LongMessageRate
		if s.CorrectionRate > 0.3 {
			result.HighCorrectionSessions++
		}
	}

	n := float64(len(filtered))
	result.AvgCorrectionRate = totalCorrectionRate / n
	result.AvgLongMsgRate = totalLongMsgRate / n

	return result
}
