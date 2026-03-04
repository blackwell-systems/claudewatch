package analyzer

import (
	"fmt"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// DashboardStats holds the computed session efficiency dashboard metrics.
type DashboardStats struct {
	// CostUSD is the total cost of the session so far in USD.
	CostUSD float64 `json:"cost_usd"`
	// Commits is the count of git commits completed in the session.
	Commits int `json:"commits"`
	// CostPerCommit is the cost divided by commits (0 if commits == 0).
	CostPerCommit float64 `json:"cost_per_commit"`
	// ToolErrors is the total count of tool errors in the session.
	ToolErrors int `json:"tool_errors"`
	// DurationMinutes is the session duration so far.
	DurationMinutes float64 `json:"duration_minutes"`
	// ToolCallCount is the total number of tool calls in the session.
	ToolCallCount int `json:"tool_call_count"`
	// DriftPercent is the percentage of recent tool calls that are reads (0-100).
	DriftPercent float64 `json:"drift_percent"`
	// Status is one of "efficient", "adequate", or "struggling".
	Status string `json:"status"`
	// StatusEmoji is the emoji representing the status: 🟢 / 🟡 / 🔴.
	StatusEmoji string `json:"status_emoji"`
}

// ComputeSessionDashboard reads the active session at activePath and computes
// a session efficiency dashboard using the provided pricing model.
// Returns an error only on I/O failure or parse errors.
func ComputeSessionDashboard(activePath string, pricing claude.CostPricing) (*DashboardStats, error) {
	// Parse the active session metadata (full parse for tool counts, commits, errors).
	meta, err := claude.ParseJSONLToSessionMeta(activePath)
	if err != nil {
		return nil, fmt.Errorf("parse active session: %w", err)
	}

	// Compute cost.
	costUSD := (float64(meta.InputTokens)/1_000_000)*pricing.InputPerMillion +
		(float64(meta.OutputTokens)/1_000_000)*pricing.OutputPerMillion

	// Parse duration from start time.
	var durationMinutes float64
	if meta.StartTime != "" {
		t, err := time.Parse(time.RFC3339, meta.StartTime)
		if err == nil {
			durationMinutes = time.Since(t).Minutes()
		}
	}

	// Count tool calls.
	toolCallCount := 0
	for _, count := range meta.ToolCounts {
		toolCallCount += count
	}

	// Compute cost per commit.
	var costPerCommit float64
	if meta.GitCommits > 0 {
		costPerCommit = costUSD / float64(meta.GitCommits)
	}

	// Parse drift percentage.
	drift, err := claude.ParseLiveDriftSignal(activePath, 15)
	if err != nil {
		drift = &claude.LiveDriftStats{} // Default to zero on parse failure.
	}
	driftPercent := 0.0
	if drift.WindowN > 0 {
		driftPercent = (float64(drift.ReadCalls) / float64(drift.WindowN)) * 100
	}

	// Compute status.
	status, emoji := computeStatus(costPerCommit, meta.ToolErrors, driftPercent)

	return &DashboardStats{
		CostUSD:         costUSD,
		Commits:         meta.GitCommits,
		CostPerCommit:   costPerCommit,
		ToolErrors:      meta.ToolErrors,
		DurationMinutes: durationMinutes,
		ToolCallCount:   toolCallCount,
		DriftPercent:    driftPercent,
		Status:          status,
		StatusEmoji:     emoji,
	}, nil
}

// computeStatus determines the session status and emoji based on the metrics.
// Status thresholds:
//   - Efficient (🟢): cost/commit < $1.00, errors < 10, drift < 20%
//   - Struggling (🔴): cost/commit > $2.00 OR errors > 20 OR drift > 30%
//   - Adequate (🟡): everything else
func computeStatus(costPerCommit float64, toolErrors int, driftPercent float64) (string, string) {
	// Efficient thresholds.
	if costPerCommit < 1.0 && toolErrors < 10 && driftPercent < 20.0 {
		return "efficient", "🟢"
	}

	// Struggling thresholds.
	if costPerCommit > 2.0 || toolErrors > 20 || driftPercent > 30.0 {
		return "struggling", "🔴"
	}

	// Default to adequate.
	return "adequate", "🟡"
}

// FormatDashboard formats a DashboardStats struct as a human-readable string
// suitable for display in the PostToolUse hook.
func FormatDashboard(stats *DashboardStats) string {
	return fmt.Sprintf(
		"%s Session Efficiency Dashboard [%d tool calls]\n"+
			"  Cost: $%.3f | Commits: %d | Cost/commit: $%.2f\n"+
			"  Errors: %d | Duration: %.1f min | Drift: %.0f%%\n"+
			"  Status: %s",
		stats.StatusEmoji,
		stats.ToolCallCount,
		stats.CostUSD,
		stats.Commits,
		stats.CostPerCommit,
		stats.ToolErrors,
		stats.DurationMinutes,
		stats.DriftPercent,
		stats.Status,
	)
}
