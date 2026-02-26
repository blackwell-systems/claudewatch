// Package claude provides types and parsers for Claude Code's local data files.
package claude

// HistoryEntry represents a single entry in ~/.claude/history.jsonl.
type HistoryEntry struct {
	Display        string         `json:"display"`
	PastedContents map[string]any `json:"pastedContents"`
	Timestamp      int64          `json:"timestamp"`
	Project        string         `json:"project"`
	SessionID      string         `json:"sessionId"`
}

// StatsCache represents the aggregate stats in ~/.claude/stats-cache.json.
type StatsCache struct {
	Version                    int                    `json:"version"`
	LastComputedDate           string                 `json:"lastComputedDate"`
	DailyActivity              []DailyActivity        `json:"dailyActivity"`
	DailyModelTokens           []DailyModelTokens     `json:"dailyModelTokens"`
	ModelUsage                 map[string]ModelUsage   `json:"modelUsage"`
	TotalSessions              int                    `json:"totalSessions"`
	TotalMessages              int                    `json:"totalMessages"`
	LongestSession             LongestSession         `json:"longestSession"`
	FirstSessionDate           string                 `json:"firstSessionDate"`
	HourCounts                 map[string]int         `json:"hourCounts"`
	TotalSpeculationTimeSavedMs int64                 `json:"totalSpeculationTimeSavedMs"`
}

// DailyActivity represents a single day's activity metrics.
type DailyActivity struct {
	Date          string `json:"date"`
	MessageCount  int    `json:"messageCount"`
	SessionCount  int    `json:"sessionCount"`
	ToolCallCount int    `json:"toolCallCount"`
}

// DailyModelTokens represents token usage by model for a single day.
type DailyModelTokens struct {
	Date          string         `json:"date"`
	TokensByModel map[string]int `json:"tokensByModel"`
}

// ModelUsage represents aggregate usage stats for a single model.
type ModelUsage struct {
	InputTokens              int64 `json:"inputTokens"`
	OutputTokens             int64 `json:"outputTokens"`
	CacheReadInputTokens     int64 `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int64 `json:"cacheCreationInputTokens"`
	WebSearchRequests        int   `json:"webSearchRequests"`
	CostUSD                  float64 `json:"costUSD"`
	ContextWindow            int   `json:"contextWindow"`
	MaxOutputTokens          int   `json:"maxOutputTokens"`
}

// LongestSession holds metadata about the longest recorded session.
type LongestSession struct {
	SessionID    string `json:"sessionId"`
	Duration     int64  `json:"duration"`
	MessageCount int    `json:"messageCount"`
	Timestamp    string `json:"timestamp"`
}

// SessionMeta represents per-session metadata from ~/.claude/usage-data/session-meta/*.json.
type SessionMeta struct {
	SessionID             string         `json:"session_id"`
	ProjectPath           string         `json:"project_path"`
	StartTime             string         `json:"start_time"`
	DurationMinutes       int            `json:"duration_minutes"`
	UserMessageCount      int            `json:"user_message_count"`
	AssistantMessageCount int            `json:"assistant_message_count"`
	ToolCounts            map[string]int `json:"tool_counts"`
	Languages             map[string]int `json:"languages"`
	GitCommits            int            `json:"git_commits"`
	GitPushes             int            `json:"git_pushes"`
	InputTokens           int            `json:"input_tokens"`
	OutputTokens          int            `json:"output_tokens"`
	FirstPrompt           string         `json:"first_prompt"`
	UserInterruptions     int            `json:"user_interruptions"`
	UserResponseTimes     []float64      `json:"user_response_times"`
	ToolErrors            int            `json:"tool_errors"`
	ToolErrorCategories   map[string]int `json:"tool_error_categories"`
	UsesTaskAgent         bool           `json:"uses_task_agent"`
	UsesMCP               bool           `json:"uses_mcp"`
	UsesWebSearch         bool           `json:"uses_web_search"`
	UsesWebFetch          bool           `json:"uses_web_fetch"`
	LinesAdded            int            `json:"lines_added"`
	LinesRemoved          int            `json:"lines_removed"`
	FilesModified         int            `json:"files_modified"`
	MessageHours          []int          `json:"message_hours"`
	UserMessageTimestamps []string       `json:"user_message_timestamps"`
}

// SessionFacet represents qualitative session analysis from ~/.claude/usage-data/facets/*.json.
type SessionFacet struct {
	UnderlyingGoal         string         `json:"underlying_goal"`
	GoalCategories         map[string]int `json:"goal_categories"`
	Outcome                string         `json:"outcome"`
	UserSatisfactionCounts map[string]int `json:"user_satisfaction_counts"`
	ClaudeHelpfulness      string         `json:"claude_helpfulness"`
	SessionType            string         `json:"session_type"`
	FrictionCounts         map[string]int `json:"friction_counts"`
	FrictionDetail         string         `json:"friction_detail"`
	PrimarySuccess         string         `json:"primary_success"`
	BriefSummary           string         `json:"brief_summary"`
	SessionID              string         `json:"session_id"`
}

// GlobalSettings represents ~/.claude/settings.json.
type GlobalSettings struct {
	IncludeCoAuthoredBy bool                   `json:"includeCoAuthoredBy"`
	Permissions         Permissions            `json:"permissions"`
	Hooks               map[string][]HookGroup `json:"hooks"`
	EnabledPlugins      map[string]bool        `json:"enabledPlugins"`
	Preferences         map[string]string      `json:"preferences"`
	EffortLevel         string                 `json:"effortLevel"`
}

// Permissions represents the permissions block in settings.json.
type Permissions struct {
	AllowBash  bool `json:"allow_bash"`
	AllowRead  bool `json:"allow_read"`
	AllowWrite bool `json:"allow_write"`
	AllowMCP   bool `json:"allow_mcp"`
}

// HookGroup represents a hook configuration entry.
type HookGroup struct {
	Matcher string `json:"matcher,omitempty"`
	Hooks   []Hook `json:"hooks"`
}

// Hook represents a single hook definition.
type Hook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// AgentTask represents a parsed agent task from /tmp/claude-*/tasks/*.output.
type AgentTask struct {
	AgentID     string `json:"agent_id"`
	AgentType   string `json:"agent_type"`
	Description string `json:"description"`
	SessionID   string `json:"session_id"`
	Status      string `json:"status"`
	DurationMs  int64  `json:"duration_ms"`
	TotalTokens int    `json:"total_tokens"`
	ToolUses    int    `json:"tool_uses"`
	Background  bool   `json:"background"`
	CreatedAt   string `json:"created_at"`
}

// CommandFile represents a custom slash command from ~/.claude/commands/*.md.
type CommandFile struct {
	Name    string
	Path    string
	Content string
}

// PluginEntry represents a plugin from ~/.claude/plugins/installed_plugins.json.
type PluginEntry struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Version string `json:"version"`
}

// ProjectDir represents a discovered project directory under ~/.claude/projects/.
type ProjectDir struct {
	Path string
	Name string
}
