package export

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// PrometheusExporter outputs metrics in Prometheus text format.
type PrometheusExporter struct{}

// Format returns "prometheus".
func (p *PrometheusExporter) Format() string {
	return "prometheus"
}

// Export renders the MetricSnapshot in Prometheus text format.
// Follows naming convention: claudewatch_<subsystem>_<name>_<unit>
func (p *PrometheusExporter) Export(snapshot MetricSnapshot) ([]byte, error) {
	var buf bytes.Buffer

	// Helper function to write metric with labels
	writeMetric := func(name, metricType, help string, value interface{}, labels map[string]string) {
		fmt.Fprintf(&buf, "# HELP %s %s\n", name, help)
		fmt.Fprintf(&buf, "# TYPE %s %s\n", name, metricType)

		labelStr := ""
		if len(labels) > 0 {
			var pairs []string
			for k, v := range labels {
				// Escape label values according to Prometheus spec
				escapedValue := strings.ReplaceAll(v, "\\", "\\\\")
				escapedValue = strings.ReplaceAll(escapedValue, "\"", "\\\"")
				escapedValue = strings.ReplaceAll(escapedValue, "\n", "\\n")
				pairs = append(pairs, fmt.Sprintf("%s=\"%s\"", k, escapedValue))
			}
			sort.Strings(pairs) // Stable output for testing
			labelStr = "{" + strings.Join(pairs, ",") + "}"
		}

		fmt.Fprintf(&buf, "%s%s %v\n\n", name, labelStr, value)
	}

	// Prepare labels
	labels := make(map[string]string)
	if snapshot.ProjectName != "" {
		labels["project"] = snapshot.ProjectName
	}

	// Session metrics
	writeMetric(
		"claudewatch_sessions_total",
		"counter",
		"Total number of Claude Code sessions",
		snapshot.SessionCount,
		labels,
	)

	writeMetric(
		"claudewatch_session_duration_minutes_total",
		"counter",
		"Total duration of all sessions in minutes",
		snapshot.TotalDurationMin,
		labels,
	)

	writeMetric(
		"claudewatch_session_duration_minutes_avg",
		"gauge",
		"Average session duration in minutes",
		snapshot.AvgDurationMin,
		labels,
	)

	writeMetric(
		"claudewatch_active_minutes_total",
		"counter",
		"Total active minutes across all sessions",
		snapshot.ActiveMinutes,
		labels,
	)

	// Friction metrics
	writeMetric(
		"claudewatch_friction_rate",
		"gauge",
		"Fraction of sessions with friction events (0.0-1.0)",
		snapshot.FrictionRate,
		labels,
	)

	// Friction by type (limit to top 10 to avoid cardinality explosion)
	if len(snapshot.FrictionByType) > 0 {
		// Sort by count descending
		type frictionEntry struct {
			name  string
			count int
		}
		var entries []frictionEntry
		for name, count := range snapshot.FrictionByType {
			entries = append(entries, frictionEntry{name, count})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].count > entries[j].count
		})

		// Take top 10
		limit := 10
		if len(entries) < limit {
			limit = len(entries)
		}

		for _, entry := range entries[:limit] {
			typeLabels := make(map[string]string)
			for k, v := range labels {
				typeLabels[k] = v
			}
			typeLabels["type"] = entry.name

			writeMetric(
				"claudewatch_friction_events_total",
				"counter",
				"Total friction events by type",
				entry.count,
				typeLabels,
			)
		}
	}

	writeMetric(
		"claudewatch_tool_errors_avg",
		"gauge",
		"Average tool errors per session",
		snapshot.AvgToolErrors,
		labels,
	)

	// Productivity metrics
	writeMetric(
		"claudewatch_commits_total",
		"counter",
		"Total number of git commits created",
		snapshot.TotalCommits,
		labels,
	)

	writeMetric(
		"claudewatch_commits_per_session_avg",
		"gauge",
		"Average commits per session",
		snapshot.AvgCommitsPerSession,
		labels,
	)

	writeMetric(
		"claudewatch_commit_attempt_ratio",
		"gauge",
		"Ratio of commits to code change attempts",
		snapshot.CommitAttemptRatio,
		labels,
	)

	writeMetric(
		"claudewatch_zero_commit_rate",
		"gauge",
		"Fraction of sessions with zero commits (0.0-1.0)",
		snapshot.ZeroCommitRate,
		labels,
	)

	// Cost metrics
	writeMetric(
		"claudewatch_cost_usd_total",
		"counter",
		"Total cost in USD",
		snapshot.TotalCostUSD,
		labels,
	)

	writeMetric(
		"claudewatch_cost_per_session_avg",
		"gauge",
		"Average cost per session in USD",
		snapshot.AvgCostPerSession,
		labels,
	)

	writeMetric(
		"claudewatch_cost_per_commit_avg",
		"gauge",
		"Average cost per commit in USD",
		snapshot.CostPerCommit,
		labels,
	)

	// Model usage (limit to top 5 models)
	if len(snapshot.ModelUsagePct) > 0 {
		type modelEntry struct {
			name string
			pct  float64
		}
		var entries []modelEntry
		for name, pct := range snapshot.ModelUsagePct {
			entries = append(entries, modelEntry{name, pct})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].pct > entries[j].pct
		})

		// Take top 5
		limit := 5
		if len(entries) < limit {
			limit = len(entries)
		}

		for _, entry := range entries[:limit] {
			modelLabels := make(map[string]string)
			for k, v := range labels {
				modelLabels[k] = v
			}
			modelLabels["model"] = entry.name

			writeMetric(
				"claudewatch_model_usage_percent",
				"gauge",
				"Percentage of sessions using this model (0-100)",
				entry.pct,
				modelLabels,
			)
		}
	}

	// Agent metrics
	writeMetric(
		"claudewatch_agent_success_rate",
		"gauge",
		"Agent task success rate (0.0-1.0)",
		snapshot.AgentSuccessRate,
		labels,
	)

	writeMetric(
		"claudewatch_agent_usage_rate",
		"gauge",
		"Fraction of sessions using agents (0.0-1.0)",
		snapshot.AgentUsageRate,
		labels,
	)

	// Context pressure
	writeMetric(
		"claudewatch_context_pressure_avg",
		"gauge",
		"Average context pressure (0.0-1.0)",
		snapshot.AvgContextPressure,
		labels,
	)

	return buf.Bytes(), nil
}

// ExportMultiple renders multiple MetricSnapshots in Prometheus format.
// Each snapshot's metrics include project/date labels to distinguish them.
func (p *PrometheusExporter) ExportMultiple(snapshots []MetricSnapshot) ([]byte, error) {
	var buf bytes.Buffer

	for _, snapshot := range snapshots {
		// Export each snapshot - they will have different label combinations
		snapshotBytes, err := p.Export(snapshot)
		if err != nil {
			return nil, err
		}
		buf.Write(snapshotBytes)
	}

	return buf.Bytes(), nil
}

// ExportDetailed renders per-session details in Prometheus format.
func (p *PrometheusExporter) ExportDetailed(details []SessionDetail) ([]byte, error) {
	var buf bytes.Buffer

	// Helper function to write metric with labels
	writeMetric := func(name, metricType, help string, value interface{}, labels map[string]string) {
		fmt.Fprintf(&buf, "# HELP %s %s\n", name, help)
		fmt.Fprintf(&buf, "# TYPE %s %s\n", name, metricType)

		labelStr := ""
		if len(labels) > 0 {
			var pairs []string
			for k, v := range labels {
				// Escape label values according to Prometheus spec
				escapedValue := strings.ReplaceAll(v, "\\", "\\\\")
				escapedValue = strings.ReplaceAll(escapedValue, "\"", "\\\"")
				escapedValue = strings.ReplaceAll(escapedValue, "\n", "\\n")
				pairs = append(pairs, fmt.Sprintf("%s=\"%s\"", k, escapedValue))
			}
			sort.Strings(pairs) // Stable output
			labelStr = "{" + strings.Join(pairs, ",") + "}"
		}

		fmt.Fprintf(&buf, "%s%s %v\n\n", name, labelStr, value)
	}

	// Write session-level metrics
	for _, detail := range details {
		labels := map[string]string{
			"session_id":   detail.SessionID,
			"project_name": detail.ProjectName,
			"model":        detail.Model,
		}

		writeMetric(
			"claudewatch_session_duration_minutes",
			"gauge",
			"Session duration in minutes",
			detail.DurationMin,
			labels,
		)

		writeMetric(
			"claudewatch_session_commits",
			"gauge",
			"Number of commits in session",
			detail.Commits,
			labels,
		)

		writeMetric(
			"claudewatch_session_tool_errors",
			"gauge",
			"Number of tool errors in session",
			detail.ToolErrors,
			labels,
		)

		writeMetric(
			"claudewatch_session_cost_usd",
			"gauge",
			"Session cost in USD",
			detail.CostUSD,
			labels,
		)

		writeMetric(
			"claudewatch_session_friction_events",
			"gauge",
			"Number of friction events in session",
			detail.FrictionEvents,
			labels,
		)

		writeMetric(
			"claudewatch_session_input_tokens",
			"gauge",
			"Input tokens consumed in session",
			detail.InputTokens,
			labels,
		)

		writeMetric(
			"claudewatch_session_output_tokens",
			"gauge",
			"Output tokens generated in session",
			detail.OutputTokens,
			labels,
		)

		sawValue := 0
		if detail.IsSAW {
			sawValue = 1
		}
		writeMetric(
			"claudewatch_session_is_saw",
			"gauge",
			"Whether session used Scout-and-Wave (1=yes, 0=no)",
			sawValue,
			labels,
		)
	}

	return buf.Bytes(), nil
}
