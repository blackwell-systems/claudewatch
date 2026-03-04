package export

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
)

// MetricSnapshot contains safe, aggregated metrics for export.
// No sensitive data (transcript content, file paths, credentials).
type MetricSnapshot struct {
	Timestamp time.Time

	// Project identity (hash or name, never absolute paths)
	ProjectName string
	ProjectHash string

	// Session metrics
	SessionCount     int
	TotalDurationMin float64
	AvgDurationMin   float64
	ActiveMinutes    float64

	// Friction metrics
	FrictionRate   float64 // sessions with friction / total sessions
	FrictionByType map[string]int
	AvgToolErrors  float64

	// Productivity metrics
	TotalCommits         int
	AvgCommitsPerSession float64
	CommitAttemptRatio   float64 // commits / (Edit+Write tool uses)
	ZeroCommitRate       float64

	// Cost metrics (USD)
	TotalCostUSD      float64
	AvgCostPerSession float64
	CostPerCommit     float64

	// Model usage (percentages, not token counts)
	ModelUsagePct map[string]float64 // model name → % of sessions

	// Agent metrics
	AgentSuccessRate float64
	AgentUsageRate   float64 // sessions with agents / total

	// Context pressure (aggregated status)
	AvgContextPressure float64 // 0.0-1.0
}

// CollectMetrics gathers safe, aggregated metrics for export.
// projectFilter: empty string = all projects, or specific project name
// days: time window (0 = all time)
func CollectMetrics(cfg *config.Config, projectFilter string, days int) (MetricSnapshot, error) {
	snapshot := MetricSnapshot{
		Timestamp:      time.Now(),
		FrictionByType: make(map[string]int),
		ModelUsagePct:  make(map[string]float64),
	}

	// Load all session metadata
	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return snapshot, fmt.Errorf("failed to load sessions: %w", err)
	}

	// Load facets for friction analysis
	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		return snapshot, fmt.Errorf("failed to load facets: %w", err)
	}

	// Load agent tasks for agent metrics
	agentTasks, err := claude.ParseAgentTasks(cfg.ClaudeHome)
	if err != nil {
		// Non-fatal - agent tasks are optional
		agentTasks = nil
	}

	// Filter sessions by project if specified
	if projectFilter != "" {
		sessions = filterSessionsByProject(sessions, projectFilter)
		// Set project name and hash for filtered export
		if len(sessions) > 0 {
			snapshot.ProjectName = projectFilter
			snapshot.ProjectHash = hashProjectName(projectFilter)
		}
	} else {
		// All projects - use aggregate identifier
		snapshot.ProjectName = "all"
		snapshot.ProjectHash = "aggregate"
	}

	// Filter by time window
	if days > 0 {
		sessions = analyzer.FilterSessionsByDays(sessions, days)
		facets = filterFacetsByDays(facets, days)
		if agentTasks != nil {
			agentTasks = filterAgentTasksByDays(agentTasks, sessions, days)
		}
	}

	// Early return if no sessions match filters
	if len(sessions) == 0 {
		return snapshot, nil
	}

	// Compute session metrics using existing analyzers
	velocityMetrics := analyzer.AnalyzeVelocity(sessions, days)
	snapshot.SessionCount = velocityMetrics.TotalSessions
	snapshot.AvgDurationMin = velocityMetrics.AvgDurationMinutes
	snapshot.TotalDurationMin = velocityMetrics.AvgDurationMinutes * float64(velocityMetrics.TotalSessions)
	snapshot.ActiveMinutes = computeActiveMinutes(sessions)

	// Compute friction metrics
	frictionThreshold := 0.30
	if cfg.Friction.RecurringThreshold > 0 {
		frictionThreshold = cfg.Friction.RecurringThreshold
	}
	frictionSummary := analyzer.AnalyzeFriction(facets, frictionThreshold)
	if frictionSummary.TotalSessions > 0 {
		snapshot.FrictionRate = float64(frictionSummary.SessionsWithFriction) / float64(frictionSummary.TotalSessions)
	}
	// Copy friction by type (safe aggregate counts)
	for fType, count := range frictionSummary.FrictionByType {
		snapshot.FrictionByType[fType] = count
	}

	// Compute tool error metrics
	efficiencyMetrics := analyzer.AnalyzeEfficiency(sessions)
	snapshot.AvgToolErrors = efficiencyMetrics.AvgToolErrorsPerSession

	// Compute commit metrics
	commitAnalysis := analyzer.AnalyzeCommits(sessions)
	snapshot.TotalCommits = 0
	for _, s := range sessions {
		snapshot.TotalCommits += s.GitCommits
	}
	snapshot.AvgCommitsPerSession = commitAnalysis.AvgCommitsPerSession
	snapshot.ZeroCommitRate = commitAnalysis.ZeroCommitRate
	snapshot.CommitAttemptRatio = computeCommitAttemptRatio(sessions)

	// Compute cost metrics
	// Use Sonnet pricing as default (most common)
	pricing := analyzer.DefaultPricing["sonnet"]
	cacheRatio := analyzer.NoCacheRatio() // Use no-cache ratio if stats unavailable
	outcomeAnalysis := analyzer.AnalyzeOutcomes(sessions, facets, pricing, cacheRatio)
	snapshot.TotalCostUSD = outcomeAnalysis.TotalCost
	snapshot.AvgCostPerSession = outcomeAnalysis.AvgCostPerSession
	if snapshot.TotalCommits > 0 {
		snapshot.CostPerCommit = snapshot.TotalCostUSD / float64(snapshot.TotalCommits)
	}

	// Compute model usage percentages (safe - no token counts)
	snapshot.ModelUsagePct = computeModelUsagePercent(sessions)

	// Compute agent metrics
	if len(agentTasks) > 0 {
		agentPerf := analyzer.AnalyzeAgents(agentTasks)
		snapshot.AgentSuccessRate = agentPerf.SuccessRate
		// Compute agent usage rate: sessions with agents / total sessions
		sessionsWithAgents := countSessionsWithAgents(agentTasks)
		if snapshot.SessionCount > 0 {
			snapshot.AgentUsageRate = float64(sessionsWithAgents) / float64(snapshot.SessionCount)
		}
	}

	// Compute average context pressure (estimate from token usage)
	snapshot.AvgContextPressure = computeAvgContextPressure(sessions)

	return snapshot, nil
}

// filterSessionsByProject returns only sessions matching the given project name.
func filterSessionsByProject(sessions []claude.SessionMeta, projectName string) []claude.SessionMeta {
	var filtered []claude.SessionMeta
	for _, s := range sessions {
		// Match by directory name
		dirName := filepath.Base(s.ProjectPath)
		if dirName == projectName {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// filterFacetsByDays returns facets matching sessions from the last N days.
func filterFacetsByDays(facets []claude.SessionFacet, days int) []claude.SessionFacet {
	if days <= 0 {
		return facets
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	var filtered []claude.SessionFacet
	for _, f := range facets {
		t := claude.ParseTimestamp(f.SessionID) // SessionID contains timestamp
		if !t.IsZero() && t.After(cutoff) {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// filterAgentTasksByDays returns agent tasks matching sessions from the last N days.
func filterAgentTasksByDays(tasks []claude.AgentTask, sessions []claude.SessionMeta, days int) []claude.AgentTask {
	if days <= 0 {
		return tasks
	}
	// Build set of valid session IDs
	validSessions := make(map[string]bool)
	for _, s := range sessions {
		validSessions[s.SessionID] = true
	}
	var filtered []claude.AgentTask
	for _, t := range tasks {
		if validSessions[t.SessionID] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// computeActiveMinutes estimates active working time (vs idle time) from sessions.
// Uses message timestamps to detect gaps > 10 min.
func computeActiveMinutes(sessions []claude.SessionMeta) float64 {
	var totalActive float64
	for _, s := range sessions {
		// Simple heuristic: if duration is reasonable, count it as active
		// More sophisticated: analyze message gaps
		if s.DurationMinutes > 0 && s.DurationMinutes < 480 { // < 8 hours
			totalActive += float64(s.DurationMinutes)
		}
	}
	return totalActive
}

// computeCommitAttemptRatio calculates commits / (Edit + Write tool uses).
// Represents success rate of code changes resulting in commits.
func computeCommitAttemptRatio(sessions []claude.SessionMeta) float64 {
	var totalCommits int
	var totalEditWrites int
	for _, s := range sessions {
		totalCommits += s.GitCommits
		editCount := s.ToolCounts["Edit"]
		writeCount := s.ToolCounts["Write"]
		totalEditWrites += (editCount + writeCount)
	}
	if totalEditWrites == 0 {
		return 0
	}
	return float64(totalCommits) / float64(totalEditWrites)
}

// computeModelUsagePercent returns the percentage of sessions using each model.
// This is safe to export (percentages, not token counts or content).
func computeModelUsagePercent(sessions []claude.SessionMeta) map[string]float64 {
	if len(sessions) == 0 {
		return nil
	}

	modelSessionCount := make(map[string]int)
	totalSessions := len(sessions)

	for _, s := range sessions {
		// Track which models were used in this session
		modelsInSession := make(map[string]bool)
		for modelName := range s.ModelUsage {
			modelsInSession[modelName] = true
		}
		for modelName := range modelsInSession {
			modelSessionCount[modelName]++
		}
	}

	// Convert to percentages
	result := make(map[string]float64)
	for modelName, count := range modelSessionCount {
		result[modelName] = (float64(count) / float64(totalSessions)) * 100.0
	}

	return result
}

// countSessionsWithAgents returns the number of unique sessions that have agent tasks.
func countSessionsWithAgents(tasks []claude.AgentTask) int {
	sessionSet := make(map[string]bool)
	for _, t := range tasks {
		sessionSet[t.SessionID] = true
	}
	return len(sessionSet)
}

// computeAvgContextPressure estimates average context window usage.
// Uses a simple heuristic: token count / estimated max context size.
func computeAvgContextPressure(sessions []claude.SessionMeta) float64 {
	if len(sessions) == 0 {
		return 0
	}

	var totalPressure float64
	const estimatedMaxContext = 200000.0 // Conservative estimate for Claude models

	for _, s := range sessions {
		totalTokens := float64(s.InputTokens + s.OutputTokens)
		pressure := totalTokens / estimatedMaxContext
		if pressure > 1.0 {
			pressure = 1.0 // Cap at 100%
		}
		totalPressure += pressure
	}

	return totalPressure / float64(len(sessions))
}

// hashProjectName creates a stable, privacy-preserving hash of a project name.
func hashProjectName(name string) string {
	h := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%x", h[:8]) // First 8 bytes (16 hex chars)
}

// AnalyzeEfficiency computes tool usage efficiency indicators.
// This is a convenience wrapper for the analyzer package function.
func AnalyzeEfficiency(sessions []claude.SessionMeta) analyzer.EfficiencyMetrics {
	metrics := analyzer.EfficiencyMetrics{
		ErrorCategoryTotals: make(map[string]int),
		ToolUsageTotals:     make(map[string]int),
		TotalSessions:       len(sessions),
	}

	if len(sessions) == 0 {
		return metrics
	}

	var totalToolErrors, totalInterruptions, totalTokens int

	for _, s := range sessions {
		totalToolErrors += s.ToolErrors
		totalInterruptions += s.UserInterruptions
		totalTokens += (s.InputTokens + s.OutputTokens)

		// Aggregate error categories
		for category, count := range s.ToolErrorCategories {
			metrics.ErrorCategoryTotals[category] += count
		}

		// Aggregate tool usage
		for tool, count := range s.ToolCounts {
			metrics.ToolUsageTotals[tool] += count
		}

		// Track feature adoption
		if s.UsesTaskAgent {
			metrics.FeatureAdoption.TaskAgentSessions++
		}
		if s.UsesMCP {
			metrics.FeatureAdoption.MCPSessions++
		}
		if s.UsesWebSearch {
			metrics.FeatureAdoption.WebSearchSessions++
		}
		if s.UsesWebFetch {
			metrics.FeatureAdoption.WebFetchSessions++
		}
	}

	n := float64(len(sessions))
	metrics.AvgToolErrorsPerSession = float64(totalToolErrors) / n
	metrics.AvgInterruptionsPerSession = float64(totalInterruptions) / n
	metrics.AvgTokensPerSession = float64(totalTokens) / n
	metrics.FeatureAdoption.TotalSessions = len(sessions)

	return metrics
}

// LimitFrictionTypes returns the top N friction types by count to prevent
// label explosion in Prometheus exports.
func LimitFrictionTypes(frictionByType map[string]int, limit int) map[string]int {
	if len(frictionByType) <= limit {
		return frictionByType
	}

	type entry struct {
		name  string
		count int
	}

	var entries []entry
	for name, count := range frictionByType {
		entries = append(entries, entry{name, count})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})

	result := make(map[string]int)
	for i := 0; i < limit && i < len(entries); i++ {
		result[entries[i].name] = entries[i].count
	}

	return result
}
