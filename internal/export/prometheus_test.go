package export

import (
	"strings"
	"testing"
	"time"
)

func TestPrometheusExporter_Format(t *testing.T) {
	exporter := &PrometheusExporter{}
	if got := exporter.Format(); got != "prometheus" {
		t.Errorf("Format() = %q, want %q", got, "prometheus")
	}
}

func TestPrometheusExporter_Export(t *testing.T) {
	exporter := &PrometheusExporter{}

	snapshot := MetricSnapshot{
		Timestamp:            time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC),
		ProjectName:          "claudewatch",
		ProjectHash:          "abc123",
		SessionCount:         42,
		TotalDurationMin:     120.5,
		AvgDurationMin:       2.87,
		ActiveMinutes:        95.3,
		FrictionRate:         0.35,
		FrictionByType:       map[string]int{"retry:Bash": 15, "tool_error": 8, "user_rejected": 3},
		AvgToolErrors:        2.1,
		TotalCommits:         28,
		AvgCommitsPerSession: 0.67,
		CommitAttemptRatio:   0.82,
		ZeroCommitRate:       0.12,
		TotalCostUSD:         12.45,
		AvgCostPerSession:    0.30,
		CostPerCommit:        0.44,
		ModelUsagePct:        map[string]float64{"claude-opus-4-6": 75.5, "claude-sonnet-4-5": 24.5},
		AgentSuccessRate:     0.89,
		AgentUsageRate:       0.45,
		AvgContextPressure:   0.62,
	}

	output, err := exporter.Export(snapshot)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	outputStr := string(output)

	// Verify format structure
	tests := []struct {
		name     string
		contains string
	}{
		{"has HELP for sessions", "# HELP claudewatch_sessions_total"},
		{"has TYPE for sessions", "# TYPE claudewatch_sessions_total counter"},
		{"has session count value", "claudewatch_sessions_total{project=\"claudewatch\"} 42"},
		{"has friction rate", "claudewatch_friction_rate{project=\"claudewatch\"} 0.35"},
		{"has cost total", "claudewatch_cost_usd_total{project=\"claudewatch\"} 12.45"},
		{"has commits", "claudewatch_commits_total{project=\"claudewatch\"} 28"},
		{"has agent success rate", "claudewatch_agent_success_rate{project=\"claudewatch\"} 0.89"},
		{"has friction by type", "claudewatch_friction_events_total{project=\"claudewatch\",type=\"retry:Bash\"} 15"},
		{"has model usage", "claudewatch_model_usage_percent{model=\"claude-opus-4-6\",project=\"claudewatch\"} 75.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(outputStr, tt.contains) {
				t.Errorf("Export() output missing expected string: %q", tt.contains)
			}
		})
	}
}

func TestPrometheusExporter_EmptySnapshot(t *testing.T) {
	exporter := &PrometheusExporter{}
	snapshot := MetricSnapshot{
		Timestamp: time.Now(),
	}

	output, err := exporter.Export(snapshot)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	// Should still produce valid output with zero values
	outputStr := string(output)
	if !strings.Contains(outputStr, "claudewatch_sessions_total") {
		t.Error("Export() with empty snapshot should still include metrics")
	}
	if !strings.Contains(outputStr, "# HELP") {
		t.Error("Export() with empty snapshot should include HELP lines")
	}
}

func TestPrometheusExporter_EscapeLabelValues(t *testing.T) {
	exporter := &PrometheusExporter{}

	snapshot := MetricSnapshot{
		Timestamp:      time.Now(),
		ProjectName:    `test"project\with\special"chars`,
		SessionCount:   1,
		FrictionByType: map[string]int{`error:with"quotes`: 5},
	}

	output, err := exporter.Export(snapshot)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	outputStr := string(output)

	// Verify escaping - backslashes and quotes should be escaped
	if !strings.Contains(outputStr, `test\"project\\with\\special\"chars`) {
		t.Errorf("Export() should escape special characters in label values\nGot output:\n%s", outputStr)
	}
}

func TestPrometheusExporter_NoProjectLabel(t *testing.T) {
	exporter := &PrometheusExporter{}

	snapshot := MetricSnapshot{
		Timestamp:    time.Now(),
		ProjectName:  "", // No project filter
		SessionCount: 10,
	}

	output, err := exporter.Export(snapshot)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	outputStr := string(output)

	// When no project is specified, metrics should not have project label
	if strings.Contains(outputStr, `project=`) {
		t.Error("Export() should not include project label when ProjectName is empty")
	}
	if !strings.Contains(outputStr, "claudewatch_sessions_total 10") {
		t.Error("Export() should include metrics without labels when ProjectName is empty")
	}
}

func TestPrometheusExporter_CardinalityLimit(t *testing.T) {
	exporter := &PrometheusExporter{}

	// Create snapshot with >10 friction types and >5 models
	frictionTypes := make(map[string]int)
	for i := 1; i <= 15; i++ {
		frictionTypes[string(rune('a'+i))] = i
	}

	modelUsage := make(map[string]float64)
	for i := 1; i <= 10; i++ {
		modelUsage[string(rune('A'+i))] = float64(i)
	}

	snapshot := MetricSnapshot{
		Timestamp:      time.Now(),
		ProjectName:    "test",
		FrictionByType: frictionTypes,
		ModelUsagePct:  modelUsage,
	}

	output, err := exporter.Export(snapshot)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	outputStr := string(output)

	// Count occurrences of friction_events_total and model_usage_percent
	frictionCount := strings.Count(outputStr, "claudewatch_friction_events_total{")
	modelCount := strings.Count(outputStr, "claudewatch_model_usage_percent{")

	if frictionCount > 10 {
		t.Errorf("Export() friction types count = %d, want ≤10 (cardinality limit)", frictionCount)
	}
	if modelCount > 5 {
		t.Errorf("Export() model count = %d, want ≤5 (cardinality limit)", modelCount)
	}
}

func TestGetExporter(t *testing.T) {
	tests := []struct {
		format  string
		wantErr bool
	}{
		{"prometheus", false},
		{"json", true},
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			_, err := GetExporter(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetExporter(%q) error = %v, wantErr %v", tt.format, err, tt.wantErr)
			}
		})
	}
}
