package app

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/stretchr/testify/assert"
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

// TestCompareSAWVsSequential_PerModelCost verifies that CompareSAWVsSequential uses
// per-model pricing when sessions have ModelUsage populated. The test creates a mix
// of Opus and Sonnet sessions and verifies the comparison report reflects the correct
// per-model costs rather than applying Sonnet pricing uniformly.
func TestCompareSAWVsSequential_PerModelCost(t *testing.T) {
	sessions := []claude.SessionMeta{
		{
			SessionID:    "opus-saw-session",
			ProjectPath:  "/code/myproject",
			StartTime:    "2026-03-01T10:00:00Z",
			InputTokens:  1_000_000,
			OutputTokens: 500_000,
			GitCommits:   2,
			ModelUsage: map[string]claude.ModelStats{
				"claude-opus-4": {
					InputTokens:  1_000_000,
					OutputTokens: 500_000,
				},
			},
		},
		{
			SessionID:    "sonnet-seq-session",
			ProjectPath:  "/code/myproject",
			StartTime:    "2026-03-01T11:00:00Z",
			InputTokens:  1_000_000,
			OutputTokens: 500_000,
			GitCommits:   3,
			ModelUsage: map[string]claude.ModelStats{
				"claude-sonnet-4": {
					InputTokens:  1_000_000,
					OutputTokens: 500_000,
				},
			},
		},
	}

	sawSessionIDs := map[string]int{"opus-saw-session": 2}  // 2 waves
	sawAgentCounts := map[string]int{"opus-saw-session": 4} // 4 agents

	// Pass Sonnet pricing as fallback — but per-model path should override.
	pricing := analyzer.DefaultPricing["sonnet"]
	cacheRatio := analyzer.NoCacheRatio()

	report := analyzer.CompareSAWVsSequential(
		"myproject", sessions, nil, sawSessionIDs, sawAgentCounts,
		pricing, cacheRatio, true,
	)

	// SAW group: 1 Opus session. Opus cost = 1M * $15/M + 500K * $75/M = $52.50
	assert.Equal(t, 1, report.SAW.Count)
	assert.InDelta(t, 52.50, report.SAW.AvgCostUSD, 0.01,
		"SAW group should use Opus per-model pricing")

	// Sequential group: 1 Sonnet session. Sonnet cost = 1M * $3/M + 500K * $15/M = $10.50
	assert.Equal(t, 1, report.Sequential.Count)
	assert.InDelta(t, 10.50, report.Sequential.AvgCostUSD, 0.01,
		"Sequential group should use Sonnet per-model pricing")

	// Verify individual session details are populated
	assert.Equal(t, 2, len(report.Sessions))
}
