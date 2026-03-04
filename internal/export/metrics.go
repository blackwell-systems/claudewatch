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

// CollectMetricsPerProject returns one MetricSnapshot per project.
func CollectMetricsPerProject(cfg *config.Config, days int) ([]MetricSnapshot, error) {
	// Load all session metadata
	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}

	// Filter by time window
	if days > 0 {
		sessions = analyzer.FilterSessionsByDays(sessions, days)
	}

	// Group sessions by project
	projectSessions := make(map[string][]claude.SessionMeta)
	for _, s := range sessions {
		projectName := filepath.Base(s.ProjectPath)
		projectSessions[projectName] = append(projectSessions[projectName], s)
	}

	// Collect metrics for each project
	var snapshots []MetricSnapshot
	for projectName, projSessions := range projectSessions {
		if len(projSessions) == 0 {
			continue
		}
		// Collect metrics for this project by calling CollectMetrics with project filter
		snapshot, err := CollectMetrics(cfg, projectName, days)
		if err != nil {
			return nil, fmt.Errorf("failed to collect metrics for project %s: %w", projectName, err)
		}
		snapshots = append(snapshots, snapshot)
	}

	// Sort by project name for stable output
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].ProjectName < snapshots[j].ProjectName
	})

	return snapshots, nil
}

// CollectMetricsPerDay returns one MetricSnapshot per day over the time window.
func CollectMetricsPerDay(cfg *config.Config, projectFilter string, days int) ([]MetricSnapshot, error) {
	if days <= 0 {
		days = 30 // Default to 30 days if not specified
	}

	// Load all session metadata
	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}

	// Filter by project if specified
	if projectFilter != "" {
		sessions = filterSessionsByProject(sessions, projectFilter)
	}

	// Filter by time window
	sessions = analyzer.FilterSessionsByDays(sessions, days)

	// Group sessions by day
	daySessions := make(map[string][]claude.SessionMeta)
	for _, s := range sessions {
		t := claude.ParseTimestamp(s.SessionID)
		if t.IsZero() {
			continue
		}
		dayKey := t.Format("2006-01-02")
		daySessions[dayKey] = append(daySessions[dayKey], s)
	}

	// Collect metrics for each day
	var snapshots []MetricSnapshot
	cutoff := time.Now().AddDate(0, 0, -days)
	for i := 0; i < days; i++ {
		day := cutoff.AddDate(0, 0, i)
		dayKey := day.Format("2006-01-02")

		snapshot := MetricSnapshot{
			Timestamp:      day,
			FrictionByType: make(map[string]int),
			ModelUsagePct:  make(map[string]float64),
		}

		if projectFilter != "" {
			snapshot.ProjectName = projectFilter
			snapshot.ProjectHash = hashProjectName(projectFilter)
		} else {
			snapshot.ProjectName = "all"
			snapshot.ProjectHash = "aggregate"
		}

		daySess := daySessions[dayKey]
		if len(daySess) == 0 {
			snapshots = append(snapshots, snapshot)
			continue
		}

		// Compute metrics for this day's sessions
		snapshot.SessionCount = len(daySess)
		snapshot.TotalDurationMin = 0
		for _, s := range daySess {
			snapshot.TotalDurationMin += float64(s.DurationMinutes)
			snapshot.TotalCommits += s.GitCommits
		}
		if snapshot.SessionCount > 0 {
			snapshot.AvgDurationMin = snapshot.TotalDurationMin / float64(snapshot.SessionCount)
		}

		// Compute friction metrics
		facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
		if err == nil {
			facets = filterFacetsBySessionIDs(facets, daySess)
			frictionThreshold := 0.30
			if cfg.Friction.RecurringThreshold > 0 {
				frictionThreshold = cfg.Friction.RecurringThreshold
			}
			frictionSummary := analyzer.AnalyzeFriction(facets, frictionThreshold)
			if frictionSummary.TotalSessions > 0 {
				snapshot.FrictionRate = float64(frictionSummary.SessionsWithFriction) / float64(frictionSummary.TotalSessions)
			}
			for fType, count := range frictionSummary.FrictionByType {
				snapshot.FrictionByType[fType] = count
			}
		}

		// Compute tool error metrics
		efficiencyMetrics := analyzer.AnalyzeEfficiency(daySess)
		snapshot.AvgToolErrors = efficiencyMetrics.AvgToolErrorsPerSession

		// Compute cost metrics
		pricing := analyzer.DefaultPricing["sonnet"]
		cacheRatio := analyzer.NoCacheRatio()
		outcomeAnalysis := analyzer.AnalyzeOutcomes(daySess, nil, pricing, cacheRatio)
		snapshot.TotalCostUSD = outcomeAnalysis.TotalCost
		snapshot.AvgCostPerSession = outcomeAnalysis.AvgCostPerSession
		if snapshot.TotalCommits > 0 {
			snapshot.CostPerCommit = snapshot.TotalCostUSD / float64(snapshot.TotalCommits)
		}

		snapshots = append(snapshots, snapshot)
	}

	// Sort by timestamp
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.Before(snapshots[j].Timestamp)
	})

	return snapshots, nil
}

// CollectMetricsPerModel returns metrics split by model type.
func CollectMetricsPerModel(cfg *config.Config, projectFilter string, days int) (map[string]MetricSnapshot, error) {
	// Load all session metadata
	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}

	// Filter by project if specified
	if projectFilter != "" {
		sessions = filterSessionsByProject(sessions, projectFilter)
	}

	// Filter by time window
	if days > 0 {
		sessions = analyzer.FilterSessionsByDays(sessions, days)
	}

	// Group sessions by primary model
	modelSessions := make(map[string][]claude.SessionMeta)
	for _, s := range sessions {
		// Determine primary model (model with most tokens)
		var primaryModel string
		var maxTokens int
		for modelName, stats := range s.ModelUsage {
			totalTokens := stats.InputTokens + stats.OutputTokens
			if totalTokens > maxTokens {
				maxTokens = totalTokens
				primaryModel = modelName
			}
		}
		if primaryModel == "" {
			primaryModel = "unknown"
		}
		// Normalize model name
		primaryModel = normalizeModelNameForGrouping(primaryModel)
		modelSessions[primaryModel] = append(modelSessions[primaryModel], s)
	}

	// Collect metrics for each model
	result := make(map[string]MetricSnapshot)
	for modelName, modelSess := range modelSessions {
		if len(modelSess) == 0 {
			continue
		}

		snapshot := MetricSnapshot{
			Timestamp:      time.Now(),
			FrictionByType: make(map[string]int),
			ModelUsagePct:  make(map[string]float64),
			ProjectName:    projectFilter,
		}
		if projectFilter == "" {
			snapshot.ProjectName = "all"
		}
		snapshot.ProjectHash = hashProjectName(snapshot.ProjectName)

		// Compute session metrics
		snapshot.SessionCount = len(modelSess)
		snapshot.TotalDurationMin = 0
		for _, s := range modelSess {
			snapshot.TotalDurationMin += float64(s.DurationMinutes)
			snapshot.TotalCommits += s.GitCommits
		}
		if snapshot.SessionCount > 0 {
			snapshot.AvgDurationMin = snapshot.TotalDurationMin / float64(snapshot.SessionCount)
		}

		// Compute friction metrics
		facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
		if err == nil {
			facets = filterFacetsBySessionIDs(facets, modelSess)
			frictionThreshold := 0.30
			if cfg.Friction.RecurringThreshold > 0 {
				frictionThreshold = cfg.Friction.RecurringThreshold
			}
			frictionSummary := analyzer.AnalyzeFriction(facets, frictionThreshold)
			if frictionSummary.TotalSessions > 0 {
				snapshot.FrictionRate = float64(frictionSummary.SessionsWithFriction) / float64(frictionSummary.TotalSessions)
			}
			for fType, count := range frictionSummary.FrictionByType {
				snapshot.FrictionByType[fType] = count
			}
		}

		// Compute tool error metrics
		efficiencyMetrics := analyzer.AnalyzeEfficiency(modelSess)
		snapshot.AvgToolErrors = efficiencyMetrics.AvgToolErrorsPerSession

		// Compute cost metrics
		pricing := analyzer.DefaultPricing["sonnet"]
		cacheRatio := analyzer.NoCacheRatio()
		outcomeAnalysis := analyzer.AnalyzeOutcomes(modelSess, nil, pricing, cacheRatio)
		snapshot.TotalCostUSD = outcomeAnalysis.TotalCost
		snapshot.AvgCostPerSession = outcomeAnalysis.AvgCostPerSession
		if snapshot.TotalCommits > 0 {
			snapshot.CostPerCommit = snapshot.TotalCostUSD / float64(snapshot.TotalCommits)
		}

		result[modelName] = snapshot
	}

	return result, nil
}

// normalizeModelNameForGrouping simplifies model names for grouping.
func normalizeModelNameForGrouping(modelName string) string {
	if len(modelName) == 0 {
		return "unknown"
	}

	lower := modelName
	// Simple substring matching
	// Claude 4.6 series
	if containsSubstr(lower, "opus-4-6") || containsSubstr(lower, "opus-4.6") {
		return "opus-4.6"
	}
	if containsSubstr(lower, "sonnet-4-6") || containsSubstr(lower, "sonnet-4.6") {
		return "sonnet-4.6"
	}
	if containsSubstr(lower, "haiku-4-6") || containsSubstr(lower, "haiku-4.6") {
		return "haiku-4.6"
	}

	// Claude 4.5 series
	if containsSubstr(lower, "sonnet-4-5") || containsSubstr(lower, "sonnet-4.5") {
		return "sonnet-4.5"
	}
	if containsSubstr(lower, "haiku-4-5") || containsSubstr(lower, "haiku-4.5") {
		return "haiku-4.5"
	}

	return "other"
}

// containsSubstr checks if a string contains a substring.
func containsSubstr(s, substr string) bool {
	// Simple implementation without importing strings package again
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CollectSAWComparison returns two snapshots: one for SAW sessions, one for non-SAW.
func CollectSAWComparison(cfg *config.Config, days int) (saw MetricSnapshot, nonSAW MetricSnapshot, err error) {
	// Load all session metadata
	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return saw, nonSAW, fmt.Errorf("failed to load sessions: %w", err)
	}

	// Filter by time window
	if days > 0 {
		sessions = analyzer.FilterSessionsByDays(sessions, days)
	}

	// Split sessions into SAW and non-SAW
	var sawSessions, nonSAWSessions []claude.SessionMeta
	for _, s := range sessions {
		if isSAWSession(s) {
			sawSessions = append(sawSessions, s)
		} else {
			nonSAWSessions = append(nonSAWSessions, s)
		}
	}

	// Collect metrics for SAW sessions
	if len(sawSessions) > 0 {
		saw, err = collectMetricsForSessions(cfg, sawSessions, "saw", days)
		if err != nil {
			return saw, nonSAW, fmt.Errorf("failed to collect SAW metrics: %w", err)
		}
	} else {
		saw = MetricSnapshot{
			Timestamp:      time.Now(),
			ProjectName:    "saw",
			ProjectHash:    "saw",
			FrictionByType: make(map[string]int),
			ModelUsagePct:  make(map[string]float64),
		}
	}

	// Collect metrics for non-SAW sessions
	if len(nonSAWSessions) > 0 {
		nonSAW, err = collectMetricsForSessions(cfg, nonSAWSessions, "non-saw", days)
		if err != nil {
			return saw, nonSAW, fmt.Errorf("failed to collect non-SAW metrics: %w", err)
		}
	} else {
		nonSAW = MetricSnapshot{
			Timestamp:      time.Now(),
			ProjectName:    "non-saw",
			ProjectHash:    "non-saw",
			FrictionByType: make(map[string]int),
			ModelUsagePct:  make(map[string]float64),
		}
	}

	return saw, nonSAW, nil
}

// isSAWSession determines if a session used Scout-and-Wave.
func isSAWSession(s claude.SessionMeta) bool {
	// Check if any agent tasks were spawned (SAW uses agents)
	// This is a heuristic - may need refinement based on actual usage patterns
	return s.UsesTaskAgent
}

// collectMetricsForSessions is a helper to collect metrics for a specific set of sessions.
func collectMetricsForSessions(cfg *config.Config, sessions []claude.SessionMeta, name string, days int) (MetricSnapshot, error) {
	snapshot := MetricSnapshot{
		Timestamp:      time.Now(),
		ProjectName:    name,
		ProjectHash:    hashProjectName(name),
		FrictionByType: make(map[string]int),
		ModelUsagePct:  make(map[string]float64),
	}

	snapshot.SessionCount = len(sessions)
	snapshot.TotalDurationMin = 0
	for _, s := range sessions {
		snapshot.TotalDurationMin += float64(s.DurationMinutes)
		snapshot.TotalCommits += s.GitCommits
	}
	if snapshot.SessionCount > 0 {
		snapshot.AvgDurationMin = snapshot.TotalDurationMin / float64(snapshot.SessionCount)
	}

	// Load facets for friction analysis
	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err == nil {
		facets = filterFacetsBySessionIDs(facets, sessions)
		frictionThreshold := 0.30
		if cfg.Friction.RecurringThreshold > 0 {
			frictionThreshold = cfg.Friction.RecurringThreshold
		}
		frictionSummary := analyzer.AnalyzeFriction(facets, frictionThreshold)
		if frictionSummary.TotalSessions > 0 {
			snapshot.FrictionRate = float64(frictionSummary.SessionsWithFriction) / float64(frictionSummary.TotalSessions)
		}
		for fType, count := range frictionSummary.FrictionByType {
			snapshot.FrictionByType[fType] = count
		}
	}

	// Compute tool error metrics
	efficiencyMetrics := analyzer.AnalyzeEfficiency(sessions)
	snapshot.AvgToolErrors = efficiencyMetrics.AvgToolErrorsPerSession

	// Compute cost metrics
	pricing := analyzer.DefaultPricing["sonnet"]
	cacheRatio := analyzer.NoCacheRatio()
	outcomeAnalysis := analyzer.AnalyzeOutcomes(sessions, facets, pricing, cacheRatio)
	snapshot.TotalCostUSD = outcomeAnalysis.TotalCost
	snapshot.AvgCostPerSession = outcomeAnalysis.AvgCostPerSession
	if snapshot.TotalCommits > 0 {
		snapshot.CostPerCommit = snapshot.TotalCostUSD / float64(snapshot.TotalCommits)
	}

	return snapshot, nil
}

// filterFacetsBySessionIDs keeps only facets matching the given sessions.
func filterFacetsBySessionIDs(facets []claude.SessionFacet, sessions []claude.SessionMeta) []claude.SessionFacet {
	if len(sessions) == 0 {
		return nil
	}
	sessionIDs := make(map[string]bool)
	for _, s := range sessions {
		sessionIDs[s.SessionID] = true
	}
	var filtered []claude.SessionFacet
	for _, f := range facets {
		if sessionIDs[f.SessionID] {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// SessionDetail represents per-session data for detailed export.
type SessionDetail struct {
	SessionID       string    `json:"session_id"`
	ProjectName     string    `json:"project_name"`
	Timestamp       time.Time `json:"timestamp"`
	DurationMin     float64   `json:"duration_min"`
	Commits         int       `json:"commits"`
	ToolErrors      int       `json:"tool_errors"`
	CostUSD         float64   `json:"cost_usd"`
	Model           string    `json:"model"`
	IsSAW           bool      `json:"is_saw"`
	FrictionEvents  int       `json:"friction_events"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
}

// CollectDetailedMetrics returns per-session details.
func CollectDetailedMetrics(cfg *config.Config, projectFilter string, days int) ([]SessionDetail, error) {
	// Load all session metadata
	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}

	// Filter by project if specified
	if projectFilter != "" {
		sessions = filterSessionsByProject(sessions, projectFilter)
	}

	// Filter by time window
	if days > 0 {
		sessions = analyzer.FilterSessionsByDays(sessions, days)
	}

	// Load facets for friction counts
	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		facets = nil // Non-fatal
	}
	frictionBySession := make(map[string]int)
	for _, f := range facets {
		count := 0
		for _, c := range f.FrictionCounts {
			count += c
		}
		frictionBySession[f.SessionID] = count
	}

	// Build detailed records
	var details []SessionDetail
	for _, s := range sessions {
		timestamp := claude.ParseTimestamp(s.SessionID)

		// Determine primary model
		var primaryModel string
		var maxTokens int
		for modelName, stats := range s.ModelUsage {
			totalTokens := stats.InputTokens + stats.OutputTokens
			if totalTokens > maxTokens {
				maxTokens = totalTokens
				primaryModel = modelName
			}
		}
		if primaryModel == "" {
			primaryModel = "unknown"
		}
		primaryModel = normalizeModelNameForGrouping(primaryModel)

		// Estimate cost
		pricing := analyzer.DefaultPricing["sonnet"]
		cacheRatio := analyzer.NoCacheRatio()
		cost := analyzer.EstimateSessionCost(s, pricing, cacheRatio)

		detail := SessionDetail{
			SessionID:      s.SessionID,
			ProjectName:    filepath.Base(s.ProjectPath),
			Timestamp:      timestamp,
			DurationMin:    float64(s.DurationMinutes),
			Commits:        s.GitCommits,
			ToolErrors:     s.ToolErrors,
			CostUSD:        cost,
			Model:          primaryModel,
			IsSAW:          isSAWSession(s),
			FrictionEvents: frictionBySession[s.SessionID],
			InputTokens:    s.InputTokens,
			OutputTokens:   s.OutputTokens,
		}
		details = append(details, detail)
	}

	// Sort by timestamp descending (most recent first)
	sort.Slice(details, func(i, j int) bool {
		return details[i].Timestamp.After(details[j].Timestamp)
	})

	return details, nil
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
