package export

import (
	"encoding/json"
	"testing"
	"time"
)

func TestJSONExporter_Export(t *testing.T) {
	exporter := &JSONExporter{}

	// Test with populated snapshot
	snapshot := MetricSnapshot{
		Timestamp:            time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
		ProjectName:          "test-project",
		ProjectHash:          "abc123",
		SessionCount:         10,
		TotalDurationMin:     120.5,
		FrictionRate:         0.25,
		FrictionByType:       map[string]int{"retry:Bash": 5, "permission_denied": 2},
		ModelUsagePct:        map[string]float64{"claude-opus-4-6": 80.0, "claude-sonnet-4-6": 20.0},
		TotalCommits:         15,
		AvgCommitsPerSession: 1.5,
		TotalCostUSD:         12.34,
	}

	output, err := exporter.Export(snapshot)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify output is valid JSON
	var decoded MetricSnapshot
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	// Verify key fields
	if decoded.SessionCount != 10 {
		t.Errorf("SessionCount = %d, want 10", decoded.SessionCount)
	}
	if decoded.ProjectName != "test-project" {
		t.Errorf("ProjectName = %s, want test-project", decoded.ProjectName)
	}
	if decoded.FrictionRate != 0.25 {
		t.Errorf("FrictionRate = %f, want 0.25", decoded.FrictionRate)
	}
}

func TestJSONExporter_EmptySnapshot(t *testing.T) {
	exporter := &JSONExporter{}

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

	// Verify output is valid JSON
	var decoded MetricSnapshot
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}
}

func TestJSONExporter_Format(t *testing.T) {
	exporter := &JSONExporter{}
	if exporter.Format() != "json" {
		t.Errorf("Format() = %s, want json", exporter.Format())
	}
}
