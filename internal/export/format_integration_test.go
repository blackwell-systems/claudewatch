package export

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllFormats_WithRealisticData validates all exporters work with realistic data.
func TestAllFormats_WithRealisticData(t *testing.T) {
	snapshot := MetricSnapshot{
		Timestamp:            time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
		ProjectName:          "test-project",
		ProjectHash:          "abc123",
		SessionCount:         100,
		TotalDurationMin:     1500.5,
		AvgDurationMin:       15.005,
		ActiveMinutes:        1200.0,
		FrictionRate:         0.25,
		FrictionByType:       map[string]int{"retry:Bash": 15, "permission_denied": 5},
		AvgToolErrors:        2.5,
		TotalCommits:         150,
		AvgCommitsPerSession: 1.5,
		CommitAttemptRatio:   0.75,
		ZeroCommitRate:       0.2,
		TotalCostUSD:         250.75,
		AvgCostPerSession:    2.5075,
		CostPerCommit:        1.671666,
		ModelUsagePct:        map[string]float64{"claude-opus-4-6": 80.0, "claude-sonnet-4-6": 20.0},
		AgentSuccessRate:     0.95,
		AgentUsageRate:       0.3,
		AvgContextPressure:   0.45,
	}

	t.Run("JSON", func(t *testing.T) {
		exporter := &JSONExporter{}
		output, err := exporter.Export(snapshot)
		require.NoError(t, err)

		// Verify valid JSON
		var decoded MetricSnapshot
		err = json.Unmarshal(output, &decoded)
		require.NoError(t, err)

		// Verify key fields
		assert.Equal(t, 100, decoded.SessionCount)
		assert.Equal(t, "test-project", decoded.ProjectName)
		assert.Equal(t, 150, decoded.TotalCommits)
		assert.InDelta(t, 0.25, decoded.FrictionRate, 0.001)
	})

	t.Run("CSV", func(t *testing.T) {
		exporter := &CSVExporter{}
		output, err := exporter.Export(snapshot)
		require.NoError(t, err)

		// Parse CSV
		reader := csv.NewReader(strings.NewReader(string(output)))
		records, err := reader.ReadAll()
		require.NoError(t, err)
		require.Len(t, records, 2, "Should have header + data row")

		headers := records[0]
		dataRow := records[1]

		// Verify structure
		assert.Equal(t, "timestamp", headers[0])
		assert.Equal(t, "project_name", headers[1])
		assert.Equal(t, "session_count", headers[3])

		// Verify data
		assert.Equal(t, "test-project", dataRow[1])
		assert.Equal(t, "100", dataRow[3])
	})

	t.Run("Prometheus", func(t *testing.T) {
		exporter := &PrometheusExporter{}
		output, err := exporter.Export(snapshot)
		require.NoError(t, err)

		outputStr := string(output)

		// Verify Prometheus format
		assert.Contains(t, outputStr, "# HELP claudewatch_sessions_total")
		assert.Contains(t, outputStr, "# TYPE claudewatch_sessions_total counter")
		assert.Contains(t, outputStr, `claudewatch_sessions_total{project="test-project"} 100`)
		assert.Contains(t, outputStr, "claudewatch_commits_total")
		assert.Contains(t, outputStr, "claudewatch_friction_rate")
	})
}

// TestAllFormats_EmptySnapshot verifies all formats handle empty data correctly.
func TestAllFormats_EmptySnapshot(t *testing.T) {
	snapshot := MetricSnapshot{
		Timestamp:      time.Now(),
		FrictionByType: make(map[string]int),
		ModelUsagePct:  make(map[string]float64),
	}

	t.Run("JSON", func(t *testing.T) {
		exporter := &JSONExporter{}
		output, err := exporter.Export(snapshot)
		require.NoError(t, err)

		// Should still be valid JSON
		var decoded MetricSnapshot
		err = json.Unmarshal(output, &decoded)
		require.NoError(t, err)
	})

	t.Run("CSV", func(t *testing.T) {
		exporter := &CSVExporter{}
		output, err := exporter.Export(snapshot)
		require.NoError(t, err)

		// Should still have valid CSV structure
		reader := csv.NewReader(strings.NewReader(string(output)))
		records, err := reader.ReadAll()
		require.NoError(t, err)
		require.Len(t, records, 2, "Should have header + data row")
	})

	t.Run("Prometheus", func(t *testing.T) {
		exporter := &PrometheusExporter{}
		output, err := exporter.Export(snapshot)
		require.NoError(t, err)

		// Should still be valid Prometheus format
		outputStr := string(output)
		assert.Contains(t, outputStr, "# HELP")
		assert.Contains(t, outputStr, "# TYPE")
	})
}

// TestAllFormats_Privacy verifies no sensitive data in any format.
func TestAllFormats_Privacy(t *testing.T) {
	snapshot := MetricSnapshot{
		Timestamp:            time.Now(),
		ProjectName:          "safe-name",
		ProjectHash:          "hash123",
		SessionCount:         10,
		TotalCommits:         5,
		FrictionByType:       map[string]int{"retry:Bash": 2},
		ModelUsagePct:        map[string]float64{"claude-opus-4-6": 100.0},
		AvgCostPerSession:    1.5,
		TotalCostUSD:         15.0,
		FrictionRate:         0.2,
		AvgCommitsPerSession: 0.5,
	}

	sensitivePatterns := []string{
		"/Users/",
		"/home/",
		"session-",
		"transcript",
		"api_key",
		"sk-ant-",
	}

	t.Run("JSON", func(t *testing.T) {
		exporter := &JSONExporter{}
		output, err := exporter.Export(snapshot)
		require.NoError(t, err)

		outputStr := string(output)
		for _, pattern := range sensitivePatterns {
			assert.NotContains(t, outputStr, pattern,
				"JSON output should not contain sensitive pattern: %s", pattern)
		}
	})

	t.Run("CSV", func(t *testing.T) {
		exporter := &CSVExporter{}
		output, err := exporter.Export(snapshot)
		require.NoError(t, err)

		outputStr := string(output)
		for _, pattern := range sensitivePatterns {
			assert.NotContains(t, outputStr, pattern,
				"CSV output should not contain sensitive pattern: %s", pattern)
		}
	})

	t.Run("Prometheus", func(t *testing.T) {
		exporter := &PrometheusExporter{}
		output, err := exporter.Export(snapshot)
		require.NoError(t, err)

		outputStr := string(output)
		for _, pattern := range sensitivePatterns {
			assert.NotContains(t, outputStr, pattern,
				"Prometheus output should not contain sensitive pattern: %s", pattern)
		}
	})
}

// TestJSONExporter_JQCompatibility verifies JSON output can be parsed by jq-like tools.
func TestJSONExporter_JQCompatibility(t *testing.T) {
	exporter := &JSONExporter{}

	snapshot := MetricSnapshot{
		Timestamp:            time.Now(),
		ProjectName:          "test",
		SessionCount:         42,
		TotalCommits:         100,
		FrictionByType:       map[string]int{"retry:Bash": 5},
		ModelUsagePct:        map[string]float64{"claude-opus-4-6": 100.0},
		AvgCostPerSession:    1.5,
		TotalCostUSD:         63.0,
		FrictionRate:         0.12,
		AvgCommitsPerSession: 2.38,
	}

	output, err := exporter.Export(snapshot)
	require.NoError(t, err)

	// Parse as generic map to verify structure
	var data map[string]interface{}
	err = json.Unmarshal(output, &data)
	require.NoError(t, err)

	// Verify key fields are accessible as expected by jq
	assert.Equal(t, float64(42), data["SessionCount"])
	assert.Equal(t, float64(100), data["TotalCommits"])
	assert.Equal(t, "test", data["ProjectName"])
}

// TestCSVExporter_SpreadsheetCompatibility verifies CSV is importable to spreadsheets.
func TestCSVExporter_SpreadsheetCompatibility(t *testing.T) {
	exporter := &CSVExporter{}

	snapshot := MetricSnapshot{
		Timestamp:            time.Date(2026, 3, 4, 10, 30, 0, 0, time.UTC),
		ProjectName:          "test-project",
		ProjectHash:          "abc123",
		SessionCount:         50,
		TotalDurationMin:     500.0,
		AvgDurationMin:       10.0,
		ActiveMinutes:        450.0,
		FrictionRate:         0.2,
		FrictionByType:       map[string]int{"retry:Bash": 3, "permission": 2},
		AvgToolErrors:        1.5,
		TotalCommits:         25,
		AvgCommitsPerSession: 0.5,
		CommitAttemptRatio:   0.6,
		ZeroCommitRate:       0.5,
		TotalCostUSD:         50.0,
		AvgCostPerSession:    1.0,
		CostPerCommit:        2.0,
		ModelUsagePct:        map[string]float64{"opus": 60.0, "sonnet": 40.0},
		AgentSuccessRate:     0.9,
		AgentUsageRate:       0.2,
		AvgContextPressure:   0.35,
	}

	output, err := exporter.Export(snapshot)
	require.NoError(t, err)

	// Parse CSV
	reader := csv.NewReader(strings.NewReader(string(output)))
	records, err := reader.ReadAll()
	require.NoError(t, err)

	// Verify structure suitable for spreadsheet import
	require.Len(t, records, 2, "Should have exactly header + one data row")
	headers := records[0]
	dataRow := records[1]

	// Verify all columns are present
	assert.Len(t, headers, 21, "Should have 21 columns")
	assert.Len(t, dataRow, 21, "Data row should have 21 values")

	// Verify numeric columns are properly formatted
	assert.NotContains(t, dataRow[3], "e+", "SessionCount should not use scientific notation")
	assert.NotContains(t, dataRow[14], "e+", "TotalCostUSD should not use scientific notation")
}
