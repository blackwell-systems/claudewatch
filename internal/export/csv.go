package export

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CSVExporter outputs metrics in CSV format.
type CSVExporter struct{}

// Format returns "csv".
func (c *CSVExporter) Format() string {
	return "csv"
}

// Export renders the MetricSnapshot as CSV with headers.
// Friction and model data are serialized as semicolon-separated values within cells.
func (c *CSVExporter) Export(snapshot MetricSnapshot) ([]byte, error) {
	return c.ExportMultiple([]MetricSnapshot{snapshot})
}

// ExportMultiple renders multiple MetricSnapshots as CSV with headers.
func (c *CSVExporter) ExportMultiple(snapshots []MetricSnapshot) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Define headers
	headers := []string{
		"timestamp",
		"project_name",
		"project_hash",
		"session_count",
		"total_duration_min",
		"avg_duration_min",
		"active_minutes",
		"friction_rate",
		"friction_by_type",
		"avg_tool_errors",
		"total_commits",
		"avg_commits_per_session",
		"commit_attempt_ratio",
		"zero_commit_rate",
		"total_cost_usd",
		"avg_cost_per_session",
		"cost_per_commit",
		"model_usage_pct",
		"agent_success_rate",
		"agent_usage_rate",
		"avg_context_pressure",
	}

	if err := writer.Write(headers); err != nil {
		return nil, fmt.Errorf("failed to write CSV headers: %w", err)
	}

	// Write each snapshot as a row
	for _, snapshot := range snapshots {
		record := []string{
			snapshot.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			snapshot.ProjectName,
			snapshot.ProjectHash,
			strconv.Itoa(snapshot.SessionCount),
			formatFloat(snapshot.TotalDurationMin),
			formatFloat(snapshot.AvgDurationMin),
			formatFloat(snapshot.ActiveMinutes),
			formatFloat(snapshot.FrictionRate),
			formatMapIntSemicolon(snapshot.FrictionByType),
			formatFloat(snapshot.AvgToolErrors),
			strconv.Itoa(snapshot.TotalCommits),
			formatFloat(snapshot.AvgCommitsPerSession),
			formatFloat(snapshot.CommitAttemptRatio),
			formatFloat(snapshot.ZeroCommitRate),
			formatFloat(snapshot.TotalCostUSD),
			formatFloat(snapshot.AvgCostPerSession),
			formatFloat(snapshot.CostPerCommit),
			formatMapFloatSemicolon(snapshot.ModelUsagePct),
			formatFloat(snapshot.AgentSuccessRate),
			formatFloat(snapshot.AgentUsageRate),
			formatFloat(snapshot.AvgContextPressure),
		}

		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV writer error: %w", err)
	}

	return buf.Bytes(), nil
}

// ExportDetailed renders per-session details as CSV.
func (c *CSVExporter) ExportDetailed(details []SessionDetail) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Define headers for detailed export
	headers := []string{
		"session_id",
		"project_name",
		"timestamp",
		"duration_min",
		"commits",
		"tool_errors",
		"cost_usd",
		"model",
		"is_saw",
		"friction_events",
		"input_tokens",
		"output_tokens",
	}

	if err := writer.Write(headers); err != nil {
		return nil, fmt.Errorf("failed to write CSV headers: %w", err)
	}

	// Write each session detail as a row
	for _, detail := range details {
		record := []string{
			detail.SessionID,
			detail.ProjectName,
			detail.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			formatFloat(detail.DurationMin),
			strconv.Itoa(detail.Commits),
			strconv.Itoa(detail.ToolErrors),
			formatFloat(detail.CostUSD),
			detail.Model,
			strconv.FormatBool(detail.IsSAW),
			strconv.Itoa(detail.FrictionEvents),
			strconv.Itoa(detail.InputTokens),
			strconv.Itoa(detail.OutputTokens),
		}

		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("failed to write CSV record: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV writer error: %w", err)
	}

	return buf.Bytes(), nil
}

// formatFloat formats a float64 with 4 decimal places.
func formatFloat(f float64) string {
	return fmt.Sprintf("%.4f", f)
}

// formatMapIntSemicolon formats a map[string]int as "key1:val1;key2:val2".
// Keys are sorted for stable output.
func formatMapIntSemicolon(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}

	// Sort keys
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build semicolon-separated string
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, m[k]))
	}
	return strings.Join(parts, ";")
}

// formatMapFloatSemicolon formats a map[string]float64 as "key1:val1;key2:val2".
// Keys are sorted for stable output.
func formatMapFloatSemicolon(m map[string]float64) string {
	if len(m) == 0 {
		return ""
	}

	// Sort keys
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build semicolon-separated string
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%.2f", k, m[k]))
	}
	return strings.Join(parts, ";")
}
