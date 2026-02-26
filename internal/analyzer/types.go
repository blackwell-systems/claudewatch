// Package analyzer provides friction, velocity, satisfaction, and efficiency analysis.
package analyzer

// AnalysisResult is the top-level result of analyzing all session data.
type AnalysisResult struct {
	Friction     FrictionSummary   `json:"friction"`
	Velocity     VelocityMetrics   `json:"velocity"`
	Satisfaction SatisfactionScore `json:"satisfaction"`
	Efficiency   EfficiencyMetrics `json:"efficiency"`
}

// FrictionSummary aggregates friction patterns across sessions.
type FrictionSummary struct {
	// TotalFrictionEvents is the total count of all friction events.
	TotalFrictionEvents int `json:"total_friction_events"`

	// FrictionByType maps friction type (e.g. "wrong_approach") to count.
	FrictionByType map[string]int `json:"friction_by_type"`

	// FrictionByProject maps project path to total friction count.
	FrictionByProject map[string]int `json:"friction_by_project"`

	// RecurringFriction lists friction types appearing in >30% of sessions.
	RecurringFriction []string `json:"recurring_friction"`

	// SessionsWithFriction is the count of sessions that had any friction.
	SessionsWithFriction int `json:"sessions_with_friction"`

	// TotalSessions is the total number of sessions analyzed.
	TotalSessions int `json:"total_sessions"`
}

// VelocityMetrics captures productivity indicators.
type VelocityMetrics struct {
	// AvgLinesAddedPerSession is the mean lines added across sessions.
	AvgLinesAddedPerSession float64 `json:"avg_lines_added_per_session"`

	// AvgCommitsPerSession is the mean git commits per session.
	AvgCommitsPerSession float64 `json:"avg_commits_per_session"`

	// AvgFilesModifiedPerSession is the mean files modified per session.
	AvgFilesModifiedPerSession float64 `json:"avg_files_modified_per_session"`

	// AvgDurationMinutes is the mean session duration.
	AvgDurationMinutes float64 `json:"avg_duration_minutes"`

	// AvgMessagesPerSession is the mean message count per session.
	AvgMessagesPerSession float64 `json:"avg_messages_per_session"`

	// TotalSessions is the number of sessions analyzed.
	TotalSessions int `json:"total_sessions"`
}

// SatisfactionScore captures user satisfaction from facet data.
type SatisfactionScore struct {
	// WeightedScore is the overall satisfaction score (0-100).
	WeightedScore float64 `json:"weighted_score"`

	// SatisfactionCounts maps satisfaction level to count.
	SatisfactionCounts map[string]int `json:"satisfaction_counts"`

	// OutcomeCounts maps outcome type to count.
	OutcomeCounts map[string]int `json:"outcome_counts"`

	// HelpfulnessCounts maps helpfulness rating to count.
	HelpfulnessCounts map[string]int `json:"helpfulness_counts"`

	// TotalFacets is the number of facets analyzed.
	TotalFacets int `json:"total_facets"`
}

// EfficiencyMetrics captures tool usage efficiency indicators.
type EfficiencyMetrics struct {
	// AvgToolErrorsPerSession is the mean tool errors per session.
	AvgToolErrorsPerSession float64 `json:"avg_tool_errors_per_session"`

	// AvgInterruptionsPerSession is the mean user interruptions per session.
	AvgInterruptionsPerSession float64 `json:"avg_interruptions_per_session"`

	// ErrorCategoryTotals maps error category to total count.
	ErrorCategoryTotals map[string]int `json:"error_category_totals"`

	// ToolUsageTotals maps tool name to total usage count.
	ToolUsageTotals map[string]int `json:"tool_usage_totals"`

	// FeatureAdoption tracks adoption rates for advanced features.
	FeatureAdoption FeatureAdoption `json:"feature_adoption"`

	// AvgTokensPerSession is the mean total tokens per session.
	AvgTokensPerSession float64 `json:"avg_tokens_per_session"`

	// TotalSessions is the number of sessions analyzed.
	TotalSessions int `json:"total_sessions"`
}

// FeatureAdoption tracks how many sessions used advanced features.
type FeatureAdoption struct {
	TaskAgentSessions int `json:"task_agent_sessions"`
	MCPSessions       int `json:"mcp_sessions"`
	WebSearchSessions int `json:"web_search_sessions"`
	WebFetchSessions  int `json:"web_fetch_sessions"`
	TotalSessions     int `json:"total_sessions"`
}

// AgentPerformance captures agent-level performance metrics.
type AgentPerformance struct {
	// TotalAgents is the total number of agent tasks.
	TotalAgents int `json:"total_agents"`

	// SuccessRate is the fraction of agents that completed successfully.
	SuccessRate float64 `json:"success_rate"`

	// KillRate is the fraction of agents that were killed via TaskStop.
	KillRate float64 `json:"kill_rate"`

	// BackgroundRatio is the fraction of agents that ran in background.
	BackgroundRatio float64 `json:"background_ratio"`

	// AvgDurationMs is the mean agent task duration.
	AvgDurationMs float64 `json:"avg_duration_ms"`

	// AvgTokensPerAgent is the mean tokens per agent task.
	AvgTokensPerAgent float64 `json:"avg_tokens_per_agent"`

	// ParallelSessions is the count of sessions with 2+ concurrent agents.
	ParallelSessions int `json:"parallel_sessions"`

	// ByType maps agent type to per-type performance stats.
	ByType map[string]AgentTypeStats `json:"by_type"`
}

// AgentTypeStats captures performance stats for a single agent type.
type AgentTypeStats struct {
	Count         int     `json:"count"`
	SuccessRate   float64 `json:"success_rate"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	AvgTokens     float64 `json:"avg_tokens"`
}

// ToolProfile captures per-project tool usage patterns.
type ToolProfile struct {
	// ProjectPath is the absolute filesystem path to the project.
	ProjectPath string `json:"project_path"`

	// ProjectName is the short directory name of the project.
	ProjectName string `json:"project_name"`

	// TotalSessions is the number of sessions associated with this project.
	TotalSessions int `json:"total_sessions"`

	// ToolCounts maps tool name to total uses across all sessions.
	ToolCounts map[string]int `json:"tool_counts"`

	// ToolPerSession maps tool name to average uses per session.
	ToolPerSession map[string]float64 `json:"tool_per_session"`

	// DominantTool is the most-used tool across all sessions.
	DominantTool string `json:"dominant_tool"`

	// EditToReadRatio is the ratio of Edit to Read tool calls.
	// High values suggest productive editing; low values suggest exploration.
	EditToReadRatio float64 `json:"edit_to_read_ratio"`

	// BashRatio is the ratio of Bash calls to total tool calls.
	// High values in Go projects suggest frequent builds/tests.
	BashRatio float64 `json:"bash_ratio"`
}

// ToolAnalysis is the top-level result of per-project tool usage analysis.
type ToolAnalysis struct {
	// Projects contains tool profiles for each project with session data.
	Projects []ToolProfile `json:"projects"`

	// Anomalies lists projects where tool usage deviates from expected norms.
	Anomalies []ToolAnomaly `json:"anomalies"`
}

// ToolAnomaly describes a project where tool usage deviates significantly
// from what is expected for its language or project type.
type ToolAnomaly struct {
	// ProjectName is the short directory name of the project.
	ProjectName string `json:"project_name"`

	// Tool is the tool name involved in the anomaly.
	Tool string `json:"tool"`

	// Expected is the expected ratio or count based on project type.
	Expected float64 `json:"expected"`

	// Actual is the observed ratio or count.
	Actual float64 `json:"actual"`

	// Message is a human-readable explanation of the anomaly.
	Message string `json:"message"`
}
