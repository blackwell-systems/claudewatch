package export

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestCSVExporter_Export(t *testing.T) {
	exporter := &CSVExporter{}

	// Test with populated snapshot
	snapshot := MetricSnapshot{
		Timestamp:            time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
		ProjectName:          "test-project",
		ProjectHash:          "abc123",
		SessionCount:         10,
		TotalDurationMin:     120.5,
		AvgDurationMin:       12.05,
		ActiveMinutes:        100.0,
		FrictionRate:         0.25,
		FrictionByType:       map[string]int{"retry:Bash": 5, "permission_denied": 2},
		AvgToolErrors:        2.5,
		TotalCommits:         15,
		AvgCommitsPerSession: 1.5,
		CommitAttemptRatio:   0.75,
		ZeroCommitRate:       0.1,
		TotalCostUSD:         12.34,
		AvgCostPerSession:    1.234,
		CostPerCommit:        0.823,
		ModelUsagePct:        map[string]float64{"claude-opus-4-6": 80.0, "claude-sonnet-4-6": 20.0},
		AgentSuccessRate:     0.95,
		AgentUsageRate:       0.3,
		AvgContextPressure:   0.45,
	}

	output, err := exporter.Export(snapshot)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Parse CSV
	reader := csv.NewReader(strings.NewReader(string(output)))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to parse CSV: %v", err)
	}

	// Should have 2 rows: header + data
	if len(records) != 2 {
		t.Fatalf("Expected 2 rows (header + data), got %d", len(records))
	}

	// Check header
	headers := records[0]
	if len(headers) != 21 {
		t.Errorf("Expected 21 columns, got %d", len(headers))
	}
	if headers[0] != "timestamp" {
		t.Errorf("First header = %s, want timestamp", headers[0])
	}

	// Check data row
	dataRow := records[1]
	if len(dataRow) != 21 {
		t.Errorf("Expected 21 data fields, got %d", len(dataRow))
	}
	if dataRow[1] != "test-project" {
		t.Errorf("project_name = %s, want test-project", dataRow[1])
	}
	if dataRow[3] != "10" {
		t.Errorf("session_count = %s, want 10", dataRow[3])
	}
}

func TestCSVExporter_EmptySnapshot(t *testing.T) {
	exporter := &CSVExporter{}

	// Test with empty snapshot
	snapshot := MetricSnapshot{
		Timestamp:      time.Now(),
		FrictionByType: make(map[string]int),
		ModelUsagePct:  make(map[string]float64),
	}

	output, err := exporter.Export(snapshot)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Parse CSV
	reader := csv.NewReader(strings.NewReader(string(output)))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("Failed to parse CSV: %v", err)
	}

	// Should still have 2 rows with zero values
	if len(records) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(records))
	}
}

func TestCSVExporter_Format(t *testing.T) {
	exporter := &CSVExporter{}
	if exporter.Format() != "csv" {
		t.Errorf("Format() = %s, want csv", exporter.Format())
	}
}

func TestFormatMapIntSemicolon(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]int
		expected string
	}{
		{
			name:     "empty map",
			input:    map[string]int{},
			expected: "",
		},
		{
			name:     "single entry",
			input:    map[string]int{"retry:Bash": 5},
			expected: "retry:Bash:5",
		},
		{
			name:     "multiple entries (sorted)",
			input:    map[string]int{"z": 1, "a": 2, "m": 3},
			expected: "a:2;m:3;z:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMapIntSemicolon(tt.input)
			if result != tt.expected {
				t.Errorf("formatMapIntSemicolon(%v) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatMapFloatSemicolon(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]float64
		expected string
	}{
		{
			name:     "empty map",
			input:    map[string]float64{},
			expected: "",
		},
		{
			name:     "single entry",
			input:    map[string]float64{"opus": 75.5},
			expected: "opus:75.50",
		},
		{
			name:     "multiple entries (sorted)",
			input:    map[string]float64{"z": 10.1, "a": 20.2, "m": 30.3},
			expected: "a:20.20;m:30.30;z:10.10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMapFloatSemicolon(tt.input)
			if result != tt.expected {
				t.Errorf("formatMapFloatSemicolon(%v) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}
