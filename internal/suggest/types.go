// Package suggest provides the recommendation engine and rule types.
package suggest

// Priority levels for suggestions.
const (
	PriorityCritical = 1
	PriorityHigh     = 2
	PriorityMedium   = 3
	PriorityLow      = 4
)

// Suggestion represents an actionable improvement recommendation.
type Suggestion struct {
	Category    string  `json:"category"`
	Priority    int     `json:"priority"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	ImpactScore float64 `json:"impact_score"`
}

// AnalysisContext provides all data needed by suggest rules to generate
// recommendations. It is populated by the scan, metrics, and gaps commands
// before being passed to the suggest engine.
type AnalysisContext struct {
	// Projects is the list of all discovered projects with scores.
	Projects []ProjectContext `json:"projects"`

	// TotalSessions is the total number of sessions analyzed.
	TotalSessions int `json:"total_sessions"`

	// AvgToolErrors is the average tool errors per session across all sessions.
	AvgToolErrors float64 `json:"avg_tool_errors"`

	// RecurringFriction lists friction types appearing in >30% of sessions.
	RecurringFriction []string `json:"recurring_friction"`

	// HookCount is the number of configured hooks.
	HookCount int `json:"hook_count"`

	// CommandCount is the number of custom slash commands.
	CommandCount int `json:"command_count"`

	// PluginCount is the number of enabled plugins.
	PluginCount int `json:"plugin_count"`

	// AgentSuccessRate is the overall agent success rate.
	AgentSuccessRate float64 `json:"agent_success_rate"`

	// AgentTypeStats maps agent type to success rate.
	AgentTypeStats map[string]float64 `json:"agent_type_stats"`

	// CustomMetricTrends maps metric name to recent trend direction.
	CustomMetricTrends map[string]string `json:"custom_metric_trends"`
}

// ProjectContext provides project-level data for suggest rules.
type ProjectContext struct {
	Path            string  `json:"path"`
	Name            string  `json:"name"`
	HasClaudeMD     bool    `json:"has_claude_md"`
	SessionCount    int     `json:"session_count"`
	ToolErrors      int     `json:"tool_errors"`
	Interruptions   int     `json:"interruptions"`
	Score           float64 `json:"score"`
	HasFacets       bool    `json:"has_facets"`
	AgentCount      int     `json:"agent_count"`
	SequentialCount int     `json:"sequential_count"`
}

// Rule is a function that examines the analysis context and produces
// zero or more suggestions.
type Rule func(ctx *AnalysisContext) []Suggestion
