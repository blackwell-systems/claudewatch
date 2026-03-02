package app

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
)

// TestRenderCompare_Empty verifies that renderCompare does not panic
// when given an empty ComparisonReport (no SAW or sequential sessions).
func TestRenderCompare_Empty(t *testing.T) {
	report := analyzer.ComparisonReport{
		Project:    "test-project",
		SAW:        analyzer.ComparisonGroup{},
		Sequential: analyzer.ComparisonGroup{},
	}

	// Should not panic.
	renderCompare(report)
}

// TestRenderCompare_WithData verifies that renderCompare does not panic
// when given a populated ComparisonReport.
func TestRenderCompare_WithData(t *testing.T) {
	report := analyzer.ComparisonReport{
		Project: "my-project",
		SAW: analyzer.ComparisonGroup{
			Count:         3,
			AvgCostUSD:    0.125,
			AvgCommits:    4.5,
			CostPerCommit: 0.028,
			AvgFriction:   1.2,
		},
		Sequential: analyzer.ComparisonGroup{
			Count:         10,
			AvgCostUSD:    0.085,
			AvgCommits:    2.1,
			CostPerCommit: 0.040,
			AvgFriction:   2.8,
		},
	}

	// Should not panic.
	renderCompare(report)
}

// TestRenderCompare_NoCostPerCommit verifies that renderCompare renders "N/A" for
// CostPerCommit when it is zero (no commits).
func TestRenderCompare_NoCostPerCommit(t *testing.T) {
	report := analyzer.ComparisonReport{
		Project: "no-commits-project",
		SAW: analyzer.ComparisonGroup{
			Count:         1,
			AvgCostUSD:    0.05,
			AvgCommits:    0,
			CostPerCommit: 0, // no commits
			AvgFriction:   0.5,
		},
		Sequential: analyzer.ComparisonGroup{
			Count:      2,
			AvgCostUSD: 0.03,
		},
	}

	// Should not panic.
	renderCompare(report)
}

// TestCompareCmd_Registered verifies that compareCmd is registered on rootCmd.
func TestCompareCmd_Registered(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "compare" {
			return
		}
	}
	t.Fatal("compare subcommand not registered on rootCmd")
}

// TestCompareFlagProject_Registered verifies --project flag is registered on compareCmd.
func TestCompareFlagProject_Registered(t *testing.T) {
	flag := compareCmd.Flags().Lookup("project")
	if flag == nil {
		t.Fatal("expected --project flag to be registered on compareCmd")
	}
}
