// Package store provides SQLite database access for claudewatch metrics and snapshots.
package store

import "time"

// Snapshot represents a point-in-time capture of all metrics.
type Snapshot struct {
	ID      int64     `json:"id"`
	TakenAt time.Time `json:"taken_at"`
	Command string    `json:"command"`
	Version string    `json:"version"`
}

// ProjectScore represents a project's readiness score within a snapshot.
type ProjectScore struct {
	ID               int64   `json:"id"`
	SnapshotID       int64   `json:"snapshot_id"`
	Project          string  `json:"project"`
	Score            float64 `json:"score"`
	HasClaudeMD      bool    `json:"has_claude_md"`
	HasDotClaude     bool    `json:"has_dot_claude"`
	HasLocalSettings bool    `json:"has_local_settings"`
	SessionCount     int     `json:"session_count"`
	LastSessionDate  string  `json:"last_session_date,omitempty"`
	PrimaryLanguage  string  `json:"primary_language,omitempty"`
	GitCommit30D     int     `json:"git_commit_30d"`
}

// AggregateMetric represents a named metric value within a snapshot.
type AggregateMetric struct {
	ID          int64   `json:"id"`
	SnapshotID  int64   `json:"snapshot_id"`
	MetricName  string  `json:"metric_name"`
	MetricValue float64 `json:"metric_value"`
	Detail      string  `json:"detail,omitempty"`
}

// FrictionEvent represents a friction occurrence within a snapshot.
type FrictionEvent struct {
	ID           int64  `json:"id"`
	SnapshotID   int64  `json:"snapshot_id"`
	SessionID    string `json:"session_id"`
	FrictionType string `json:"friction_type"`
	Count        int    `json:"count"`
	Detail       string `json:"detail,omitempty"`
	Project      string `json:"project,omitempty"`
	SessionDate  string `json:"session_date,omitempty"`
}

// Suggestion represents an actionable improvement recommendation.
type Suggestion struct {
	ID          int64   `json:"id"`
	SnapshotID  int64   `json:"snapshot_id"`
	Category    string  `json:"category"`
	Priority    int     `json:"priority"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	ImpactScore float64 `json:"impact_score"`
	Status      string  `json:"status"`
}

// AgentTaskRow represents an agent task record in the database.
type AgentTaskRow struct {
	ID               int64  `json:"id"`
	SnapshotID       int64  `json:"snapshot_id"`
	SessionID        string `json:"session_id"`
	AgentID          string `json:"agent_id"`
	AgentType        string `json:"agent_type"`
	Description      string `json:"description,omitempty"`
	Status           string `json:"status"`
	DurationMs       int64  `json:"duration_ms,omitempty"`
	TotalTokens      int    `json:"total_tokens,omitempty"`
	ToolUses         int    `json:"tool_uses,omitempty"`
	Background       bool   `json:"background"`
	NeededCorrection bool   `json:"needed_correction"`
	CreatedAt        string `json:"created_at"`
}

// CustomMetricRow represents a user-injected custom metric.
type CustomMetricRow struct {
	ID          int64   `json:"id"`
	LoggedAt    string  `json:"logged_at"`
	SessionID   string  `json:"session_id,omitempty"`
	Project     string  `json:"project,omitempty"`
	MetricName  string  `json:"metric_name"`
	MetricValue float64 `json:"metric_value"`
	Tags        string  `json:"tags,omitempty"`
	Note        string  `json:"note,omitempty"`
}

// MetricRow is a generic metric name-value pair used in queries.
type MetricRow struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Detail string  `json:"detail,omitempty"`
}

// SnapshotDiff represents the comparison between two snapshots.
type SnapshotDiff struct {
	Previous *Snapshot     `json:"previous"`
	Current  *Snapshot     `json:"current"`
	Deltas   []MetricDelta `json:"deltas"`
}

// MetricDelta represents the change in a single metric between snapshots.
type MetricDelta struct {
	Name      string  `json:"name"`
	Previous  float64 `json:"previous"`
	Current   float64 `json:"current"`
	Delta     float64 `json:"delta"`
	Direction string  `json:"direction"` // "improved", "regressed", "unchanged"
}
