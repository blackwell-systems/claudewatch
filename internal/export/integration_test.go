package export

import (
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExportPrometheus_EndToEnd validates the complete export pipeline:
// collect metrics → export to Prometheus → validate output format and privacy.
func TestExportPrometheus_EndToEnd(t *testing.T) {
	// 1. Create test configuration
	cfg := &config.Config{
		ClaudeHome: t.TempDir(), // Use temp dir for test isolation
		Friction: config.Friction{
			RecurringThreshold: 0.30,
		},
	}

	// 2. Collect metrics (will return empty snapshot for test with no real data)
	snapshot, err := CollectMetrics(cfg, "", 30)
	require.NoError(t, err, "CollectMetrics should not error on empty data")

	// Verify snapshot was created
	assert.NotZero(t, snapshot.Timestamp, "Snapshot should have timestamp")

	// 3. Get Prometheus exporter
	exporter, err := GetExporter("prometheus")
	require.NoError(t, err, "GetExporter should succeed for 'prometheus'")
	assert.Equal(t, "prometheus", exporter.Format())

	// 4. Export to Prometheus format
	output, err := exporter.Export(snapshot)
	require.NoError(t, err, "Export should not error")
	require.NotEmpty(t, output, "Export output should not be empty")

	outputStr := string(output)

	// 5. Validate Prometheus text format structure
	t.Run("ValidPrometheusFormat", func(t *testing.T) {
		// Should contain HELP and TYPE directives
		assert.Contains(t, outputStr, "# HELP claudewatch_sessions_total")
		assert.Contains(t, outputStr, "# TYPE claudewatch_sessions_total counter")
		assert.Contains(t, outputStr, "# HELP claudewatch_friction_rate")
		assert.Contains(t, outputStr, "# TYPE claudewatch_friction_rate gauge")
		assert.Contains(t, outputStr, "# HELP claudewatch_cost_usd_total")
		assert.Contains(t, outputStr, "# TYPE claudewatch_cost_usd_total counter")

		// Should contain actual metric lines
		assert.Contains(t, outputStr, "claudewatch_sessions_total")
		assert.Contains(t, outputStr, "claudewatch_friction_rate")
		assert.Contains(t, outputStr, "claudewatch_commits_total")
		assert.Contains(t, outputStr, "claudewatch_cost_usd_total")
	})

	// 6. Privacy validation - CRITICAL TEST
	t.Run("PrivacyValidation", func(t *testing.T) {
		// Should NEVER contain absolute file paths
		assert.NotContains(t, outputStr, "/Users/", "Output must not contain absolute paths")
		assert.NotContains(t, outputStr, "/home/", "Output must not contain absolute paths")
		assert.NotContains(t, outputStr, "C:\\", "Output must not contain absolute paths")

		// Should NEVER contain session IDs (UUIDs or timestamp-based IDs)
		// Note: metric names like "claudewatch_sessions_total" are safe and expected
		assert.NotRegexp(t, `sess-[a-f0-9]{8}`, outputStr, "Output must not contain session IDs")
		assert.NotRegexp(t, `[0-9]{13,}`, outputStr, "Output must not contain timestamp-based session IDs")

		// Should NEVER contain API keys or credentials
		assert.NotContains(t, outputStr, "sk-", "Output must not contain API keys")
		assert.NotContains(t, outputStr, "key=", "Output must not contain credentials")
		assert.NotContains(t, outputStr, "token=", "Output must not contain credentials")
		assert.NotContains(t, outputStr, "password", "Output must not contain credentials")

		// Should NEVER contain transcript content indicators
		assert.NotContains(t, outputStr, "user:", "Output must not contain transcript content")
		assert.NotContains(t, outputStr, "assistant:", "Output must not contain transcript content")
		assert.NotContains(t, outputStr, "tool_result", "Output must not contain tool results")
	})
}

// TestExportPrometheus_WithRealisticData validates export with realistic test data.
func TestExportPrometheus_WithRealisticData(t *testing.T) {
	// Create a realistic MetricSnapshot manually (simulating what CollectMetrics would return)
	snapshot := MetricSnapshot{
		Timestamp:            time.Now(),
		ProjectName:          "testproject",
		ProjectHash:          "a1b2c3d4",
		SessionCount:         42,
		TotalDurationMin:     840.5,
		AvgDurationMin:       20.0,
		ActiveMinutes:        720.0,
		FrictionRate:         0.35,
		FrictionByType:       map[string]int{"retry:Bash": 15, "buggy_code": 10, "excessive_analysis": 5},
		AvgToolErrors:        3.2,
		TotalCommits:         85,
		AvgCommitsPerSession: 2.02,
		CommitAttemptRatio:   0.75,
		ZeroCommitRate:       0.15,
		TotalCostUSD:         12.45,
		AvgCostPerSession:    0.296,
		CostPerCommit:        0.146,
		ModelUsagePct:        map[string]float64{"sonnet": 85.0, "opus": 15.0},
		AgentSuccessRate:     0.92,
		AgentUsageRate:       0.65,
		AvgContextPressure:   0.42,
	}

	// Export to Prometheus
	exporter := &PrometheusExporter{}
	output, err := exporter.Export(snapshot)
	require.NoError(t, err)

	outputStr := string(output)

	t.Run("ContainsExpectedMetrics", func(t *testing.T) {
		// Session metrics
		assert.Contains(t, outputStr, "claudewatch_sessions_total{project=\"testproject\"} 42")
		assert.Contains(t, outputStr, "claudewatch_session_duration_minutes_avg{project=\"testproject\"} 20")
		assert.Contains(t, outputStr, "claudewatch_active_minutes_total{project=\"testproject\"} 720")

		// Friction metrics
		assert.Contains(t, outputStr, "claudewatch_friction_rate{project=\"testproject\"} 0.35")
		assert.Contains(t, outputStr, "claudewatch_tool_errors_avg{project=\"testproject\"} 3.2")

		// Productivity metrics
		assert.Contains(t, outputStr, "claudewatch_commits_total{project=\"testproject\"} 85")
		assert.Contains(t, outputStr, "claudewatch_commits_per_session_avg{project=\"testproject\"} 2.02")
		assert.Contains(t, outputStr, "claudewatch_commit_attempt_ratio{project=\"testproject\"} 0.75")
		assert.Contains(t, outputStr, "claudewatch_zero_commit_rate{project=\"testproject\"} 0.15")

		// Cost metrics
		assert.Contains(t, outputStr, "claudewatch_cost_usd_total{project=\"testproject\"} 12.45")
		assert.Contains(t, outputStr, "claudewatch_cost_per_session_avg{project=\"testproject\"} 0.296")
		assert.Contains(t, outputStr, "claudewatch_cost_per_commit_avg{project=\"testproject\"} 0.146")

		// Agent metrics
		assert.Contains(t, outputStr, "claudewatch_agent_success_rate{project=\"testproject\"} 0.92")
		assert.Contains(t, outputStr, "claudewatch_agent_usage_rate{project=\"testproject\"} 0.65")

		// Context pressure
		assert.Contains(t, outputStr, "claudewatch_context_pressure_avg{project=\"testproject\"} 0.42")
	})

	t.Run("ContainsFrictionBreakdown", func(t *testing.T) {
		// Should contain top friction types with labels
		assert.Contains(t, outputStr, "claudewatch_friction_events_total")
		assert.Contains(t, outputStr, "type=\"retry:Bash\"")
		assert.Contains(t, outputStr, "type=\"buggy_code\"")
	})

	t.Run("ContainsModelUsage", func(t *testing.T) {
		// Should contain model usage percentages
		assert.Contains(t, outputStr, "claudewatch_model_usage_percent")
		assert.Contains(t, outputStr, "model=\"sonnet\"")
		assert.Contains(t, outputStr, "85") // 85% usage
	})
}

// TestExportPrometheus_CardinalityLimits validates that cardinality is limited
// to prevent label explosion in Prometheus.
func TestExportPrometheus_CardinalityLimits(t *testing.T) {
	// Create snapshot with many friction types (more than limit of 10)
	snapshot := MetricSnapshot{
		Timestamp:   time.Now(),
		ProjectName: "test",
		FrictionByType: map[string]int{
			"type1": 100, "type2": 90, "type3": 80, "type4": 70, "type5": 60,
			"type6": 50, "type7": 40, "type8": 30, "type9": 20, "type10": 10,
			"type11": 5, "type12": 4, "type13": 3, "type14": 2, "type15": 1,
		},
		ModelUsagePct: map[string]float64{
			"model1": 20, "model2": 18, "model3": 16, "model4": 14, "model5": 12,
			"model6": 10, "model7": 8, "model8": 6, "model9": 4, "model10": 2,
		},
	}

	exporter := &PrometheusExporter{}
	output, err := exporter.Export(snapshot)
	require.NoError(t, err)

	outputStr := string(output)

	t.Run("FrictionTypeLimitedToTop10", func(t *testing.T) {
		// Count how many friction type metrics are present
		frictionCount := strings.Count(outputStr, "claudewatch_friction_events_total{")
		assert.LessOrEqual(t, frictionCount, 10, "Should export at most 10 friction types")

		// Top types should be present
		assert.Contains(t, outputStr, "type=\"type1\"")
		assert.Contains(t, outputStr, "type=\"type10\"")

		// Bottom types should be excluded
		assert.NotContains(t, outputStr, "type=\"type15\"")
	})

	t.Run("ModelUsageLimitedToTop5", func(t *testing.T) {
		// Count how many model usage metrics are present
		modelCount := strings.Count(outputStr, "claudewatch_model_usage_percent{")
		assert.LessOrEqual(t, modelCount, 5, "Should export at most 5 models")

		// Top models should be present
		assert.Contains(t, outputStr, "model=\"model1\"")

		// Bottom models should be excluded
		assert.NotContains(t, outputStr, "model=\"model10\"")
	})
}

// TestExportPrometheus_EmptySnapshot validates graceful handling of empty data.
func TestExportPrometheus_EmptySnapshot(t *testing.T) {
	snapshot := MetricSnapshot{
		Timestamp:      time.Now(),
		ProjectName:    "empty",
		FrictionByType: make(map[string]int),
		ModelUsagePct:  make(map[string]float64),
	}

	exporter := &PrometheusExporter{}
	output, err := exporter.Export(snapshot)
	require.NoError(t, err, "Export should handle empty snapshot gracefully")
	require.NotEmpty(t, output, "Output should not be empty even with zero metrics")

	outputStr := string(output)

	// Should still contain metric definitions with zero values
	assert.Contains(t, outputStr, "claudewatch_sessions_total{project=\"empty\"} 0")
	assert.Contains(t, outputStr, "claudewatch_friction_rate{project=\"empty\"} 0")
	assert.Contains(t, outputStr, "claudewatch_commits_total{project=\"empty\"} 0")
}

// TestExportPrometheus_ProjectFiltering validates that project filtering works correctly.
func TestExportPrometheus_ProjectFiltering(t *testing.T) {
	// This test would require actual session data with different projects.
	// For now, we validate that the project label is correctly set.
	snapshot := MetricSnapshot{
		Timestamp:    time.Now(),
		ProjectName:  "myproject",
		SessionCount: 10,
	}

	exporter := &PrometheusExporter{}
	output, err := exporter.Export(snapshot)
	require.NoError(t, err)

	outputStr := string(output)

	// All metrics should have the project label
	assert.Contains(t, outputStr, "project=\"myproject\"")
	// Should not contain "all" when filtering to specific project
	assert.NotContains(t, outputStr, "project=\"all\"")
}

// TestExportPrometheus_AllProjects validates export without project filtering.
func TestExportPrometheus_AllProjects(t *testing.T) {
	snapshot := MetricSnapshot{
		Timestamp:    time.Now(),
		ProjectName:  "all",
		SessionCount: 100,
	}

	exporter := &PrometheusExporter{}
	output, err := exporter.Export(snapshot)
	require.NoError(t, err)

	outputStr := string(output)

	// Should use "all" as project label
	assert.Contains(t, outputStr, "project=\"all\"")
}

// TestGetExporter_InvalidFormat validates error handling for unsupported formats.
func TestGetExporter_InvalidFormat(t *testing.T) {
	_, err := GetExporter("yaml")
	require.Error(t, err, "Should error on unsupported format")
	assert.Contains(t, err.Error(), "unsupported format")
}

// TestExportPrivacyRules is a comprehensive privacy validation test.
// This is the CRITICAL test that ensures no sensitive data is exported.
func TestExportPrivacyRules(t *testing.T) {
	// Create a snapshot that might accidentally contain sensitive data
	// (this simulates a buggy implementation that we're testing against)
	snapshot := MetricSnapshot{
		Timestamp:            time.Now(),
		ProjectName:          "safe-project-name",
		ProjectHash:          "abc123",
		SessionCount:         10,
		TotalCostUSD:         5.0,
		FrictionByType:       map[string]int{"tool_error": 5},
		ModelUsagePct:        map[string]float64{"sonnet": 100.0},
		TotalCommits:         5,
		AvgCommitsPerSession: 0.5,
		FrictionRate:         0.2,
	}

	exporter := &PrometheusExporter{}
	output, err := exporter.Export(snapshot)
	require.NoError(t, err)

	outputStr := string(output)

	t.Run("NoAbsolutePaths", func(t *testing.T) {
		// Unix paths
		assert.NotContains(t, outputStr, "/Users/")
		assert.NotContains(t, outputStr, "/home/")
		assert.NotContains(t, outputStr, "/var/")
		assert.NotContains(t, outputStr, "/tmp/")

		// Windows paths
		assert.NotContains(t, outputStr, "C:\\")
		assert.NotContains(t, outputStr, "D:\\")
		assert.NotContains(t, outputStr, ":\\\\")
	})

	t.Run("NoSessionIDs", func(t *testing.T) {
		// Check for actual session ID patterns (UUIDs, timestamp IDs)
		// Metric names like "claudewatch_sessions_total" are safe
		assert.NotRegexp(t, `sess-[a-f0-9]{8}`, outputStr, "Output must not contain session IDs")
		assert.NotRegexp(t, `session_[0-9]{13,}`, outputStr, "Output must not contain timestamp-based session IDs")
		assert.NotContains(t, outputStr, "sid:", "Output must not contain session ID markers")
	})

	t.Run("NoAPIKeys", func(t *testing.T) {
		assert.NotContains(t, outputStr, "sk-")
		assert.NotContains(t, outputStr, "api_key")
		assert.NotContains(t, outputStr, "apikey")
	})

	t.Run("NoTranscriptContent", func(t *testing.T) {
		assert.NotContains(t, outputStr, "user:")
		assert.NotContains(t, outputStr, "assistant:")
		assert.NotContains(t, outputStr, "tool_result")
		assert.NotContains(t, outputStr, "function_call")
	})

	t.Run("NoFileContents", func(t *testing.T) {
		// Common code patterns that shouldn't appear
		assert.NotContains(t, outputStr, "package main")
		assert.NotContains(t, outputStr, "import (")
		assert.NotContains(t, outputStr, "func ")
		assert.NotContains(t, outputStr, "class ")
	})

	t.Run("OnlySafeAggregates", func(t *testing.T) {
		// Verify that what IS exported is safe
		assert.Contains(t, outputStr, "claudewatch_sessions_total")
		assert.Contains(t, outputStr, "claudewatch_cost_usd_total")
		assert.Contains(t, outputStr, "claudewatch_friction_rate")

		// Should contain counts and rates, not raw data
		assert.Contains(t, outputStr, "project=\"safe-project-name\"")
		// Should NOT contain project hash in labels (only safe name)
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "claudewatch_") && strings.Contains(line, "{") {
				// Metric line with labels
				assert.NotContains(t, line, "abc123", "Project hash should not appear in metric labels")
			}
		}
	})
}
