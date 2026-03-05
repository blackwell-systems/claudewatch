package analyzer

import (
	"strings"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// testPricingRegression is a simple pricing config for regression tests.
var testPricingRegression = ModelPricing{
	InputPerMillion:      3.0,
	OutputPerMillion:     15.0,
	CacheReadPerMillion:  0.3,
	CacheWritePerMillion: 3.75,
}

// makeBaseline builds a ProjectBaseline for testing.
func makeBaseline(avgCost, avgFriction float64) *store.ProjectBaseline {
	return &store.ProjectBaseline{
		Project:        "testproject",
		ComputedAt:     "2026-01-01T00:00:00Z",
		SessionCount:   10,
		AvgCostUSD:     avgCost,
		StddevCostUSD:  avgCost * 0.1,
		AvgFriction:    avgFriction,
		StddevFriction: avgFriction * 0.1,
	}
}

// makeRegressionInput builds a RegressionInput with sensible defaults.
func makeRegressionInput(sessions []claude.SessionMeta, facets []claude.SessionFacet, baseline *store.ProjectBaseline, threshold float64) RegressionInput {
	return RegressionInput{
		Project:        "testproject",
		Baseline:       baseline,
		RecentSessions: sessions,
		Facets:         facets,
		Pricing:        testPricingRegression,
		CacheRatio:     NoCacheRatio(),
		Threshold:      threshold,
	}
}

func TestComputeRegressionStatus_NoBaseline(t *testing.T) {
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 100_000, 50_000, 3, 0),
		makeMeta("s2", "2026-01-02T10:00:00Z", 100_000, 50_000, 3, 0),
		makeMeta("s3", "2026-01-03T10:00:00Z", 100_000, 50_000, 3, 0),
	}
	input := makeRegressionInput(sessions, nil, nil, 1.5)
	status := ComputeRegressionStatus(input)

	if status.HasBaseline {
		t.Error("HasBaseline should be false when Baseline is nil")
	}
	if status.Regressed {
		t.Error("Regressed should be false when no baseline")
	}
	if status.Project != "testproject" {
		t.Errorf("Project = %q, want %q", status.Project, "testproject")
	}
	if status.Message != "no baseline available" {
		t.Errorf("Message = %q, want %q", status.Message, "no baseline available")
	}
}

func TestComputeRegressionStatus_InsufficientData(t *testing.T) {
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 100_000, 50_000, 3, 0),
		makeMeta("s2", "2026-01-02T10:00:00Z", 100_000, 50_000, 3, 0),
	}
	baseline := makeBaseline(1.0, 0.3)
	input := makeRegressionInput(sessions, nil, baseline, 1.5)
	status := ComputeRegressionStatus(input)

	if !status.HasBaseline {
		t.Error("HasBaseline should be true")
	}
	if !status.InsufficientData {
		t.Error("InsufficientData should be true for 2 sessions")
	}
	if status.Regressed {
		t.Error("Regressed should be false when insufficient data")
	}
	if !strings.Contains(status.Message, "insufficient data") {
		t.Errorf("Message should mention insufficient data, got %q", status.Message)
	}
}

func TestComputeRegressionStatus_NoRegression(t *testing.T) {
	// Sessions with cost ~1.05 and no friction — baseline AvgCostUSD=1.0, AvgFriction=0.3
	// currentFrictionRate = 0/3 = 0.0, which is not > 1.5 * 0.3 = 0.45
	// currentAvgCost ~1.05, which is not > 1.5 * 1.0 = 1.5
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 100_000, 50_000, 3, 0),
		makeMeta("s2", "2026-01-02T10:00:00Z", 100_000, 50_000, 3, 0),
		makeMeta("s3", "2026-01-03T10:00:00Z", 100_000, 50_000, 3, 0),
	}
	baseline := makeBaseline(1.0, 0.3)
	input := makeRegressionInput(sessions, nil, baseline, 1.5)
	status := ComputeRegressionStatus(input)

	if status.Regressed {
		t.Error("Regressed should be false for sessions matching baseline")
	}
	if status.FrictionRegressed {
		t.Error("FrictionRegressed should be false")
	}
	if status.CostRegressed {
		t.Error("CostRegressed should be false")
	}
	if status.Message != "no regression detected" {
		t.Errorf("Message = %q, want %q", status.Message, "no regression detected")
	}
}

func TestComputeRegressionStatus_FrictionRegression(t *testing.T) {
	// 3 sessions all with high friction (frictionRate = 1.0)
	// baseline AvgFriction = 0.3, threshold = 1.5
	// frictionRegressed = 1.0 > 1.5 * 0.3 = 0.45 → true
	// cost: ~1.05 per session, baseline 1.0, 1.05 > 1.5 * 1.0 = 1.5 → false
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 100_000, 50_000, 3, 5),
		makeMeta("s2", "2026-01-02T10:00:00Z", 100_000, 50_000, 3, 3),
		makeMeta("s3", "2026-01-03T10:00:00Z", 100_000, 50_000, 3, 2),
	}
	baseline := makeBaseline(1.0, 0.3)
	input := makeRegressionInput(sessions, nil, baseline, 1.5)
	status := ComputeRegressionStatus(input)

	if !status.FrictionRegressed {
		t.Error("FrictionRegressed should be true")
	}
	if status.CostRegressed {
		t.Error("CostRegressed should be false")
	}
	if !status.Regressed {
		t.Error("Regressed should be true")
	}
	if status.CurrentFrictionRate != 1.0 {
		t.Errorf("CurrentFrictionRate = %f, want 1.0", status.CurrentFrictionRate)
	}
}

func TestComputeRegressionStatus_CostRegression(t *testing.T) {
	// 3 sessions with very high cost: input=2_000_000, output=200_000
	// cost = 2.0*3 + 0.2*15 = 6.0 + 3.0 = 9.0
	// currentAvgCost = 9.0, baseline AvgCostUSD = 1.0
	// 9.0 > 1.5 * 1.0 → true
	// no friction → frictionRate = 0.0, baseline AvgFriction = 0.3
	// 0.0 > 1.5 * 0.3 = 0.45 → false
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 2_000_000, 200_000, 3, 0),
		makeMeta("s2", "2026-01-02T10:00:00Z", 2_000_000, 200_000, 3, 0),
		makeMeta("s3", "2026-01-03T10:00:00Z", 2_000_000, 200_000, 3, 0),
	}
	baseline := makeBaseline(1.0, 0.3)
	input := makeRegressionInput(sessions, nil, baseline, 1.5)
	status := ComputeRegressionStatus(input)

	if !status.CostRegressed {
		t.Errorf("CostRegressed should be true (currentAvgCost=%.4f vs threshold*baseline=%.4f)",
			status.CurrentAvgCostUSD, 1.5*baseline.AvgCostUSD)
	}
	if status.FrictionRegressed {
		t.Error("FrictionRegressed should be false")
	}
	if !status.Regressed {
		t.Error("Regressed should be true")
	}
}

func TestComputeRegressionStatus_BothRegressed(t *testing.T) {
	// High cost AND high friction.
	// cost = 9.0 per session > 1.5 * 1.0
	// frictionRate = 1.0 > 1.5 * 0.3 = 0.45
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 2_000_000, 200_000, 3, 5),
		makeMeta("s2", "2026-01-02T10:00:00Z", 2_000_000, 200_000, 3, 3),
		makeMeta("s3", "2026-01-03T10:00:00Z", 2_000_000, 200_000, 3, 2),
	}
	baseline := makeBaseline(1.0, 0.3)
	input := makeRegressionInput(sessions, nil, baseline, 1.5)
	status := ComputeRegressionStatus(input)

	if !status.FrictionRegressed {
		t.Error("FrictionRegressed should be true")
	}
	if !status.CostRegressed {
		t.Error("CostRegressed should be true")
	}
	if !status.Regressed {
		t.Error("Regressed should be true")
	}
	if !strings.Contains(status.Message, "friction rate regressed") || !strings.Contains(status.Message, "cost regressed") {
		t.Errorf("Message should mention both regressions, got %q", status.Message)
	}
}

func TestComputeRegressionStatus_ThresholdDefault(t *testing.T) {
	// Threshold=0 should default to 1.5.
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 100_000, 50_000, 3, 0),
		makeMeta("s2", "2026-01-02T10:00:00Z", 100_000, 50_000, 3, 0),
		makeMeta("s3", "2026-01-03T10:00:00Z", 100_000, 50_000, 3, 0),
	}
	baseline := makeBaseline(1.0, 0.3)
	input := makeRegressionInput(sessions, nil, baseline, 0)
	status := ComputeRegressionStatus(input)

	if status.Threshold != 1.5 {
		t.Errorf("Threshold = %f, want 1.5 (default)", status.Threshold)
	}
}

func TestComputeRegressionStatus_ZeroBaselineSkipped(t *testing.T) {
	// baseline AvgFriction=0: friction comparison should be skipped,
	// so even with 100% friction rate, no friction regression.
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 100_000, 50_000, 3, 5),
		makeMeta("s2", "2026-01-02T10:00:00Z", 100_000, 50_000, 3, 3),
		makeMeta("s3", "2026-01-03T10:00:00Z", 100_000, 50_000, 3, 2),
	}
	// AvgFriction=0 and AvgCostUSD=0: both comparisons skipped.
	baseline := makeBaseline(0.0, 0.0)
	input := makeRegressionInput(sessions, nil, baseline, 1.5)
	status := ComputeRegressionStatus(input)

	if status.FrictionRegressed {
		t.Error("FrictionRegressed should be false when baseline AvgFriction=0")
	}
	if status.CostRegressed {
		t.Error("CostRegressed should be false when baseline AvgCostUSD=0")
	}
	if status.Regressed {
		t.Error("Regressed should be false when both baselines are zero")
	}
}

func TestComputeRegressionStatus_MessageContent(t *testing.T) {
	t.Run("no_regression", func(t *testing.T) {
		sessions := []claude.SessionMeta{
			makeMeta("s1", "2026-01-01T10:00:00Z", 100_000, 50_000, 3, 0),
			makeMeta("s2", "2026-01-02T10:00:00Z", 100_000, 50_000, 3, 0),
			makeMeta("s3", "2026-01-03T10:00:00Z", 100_000, 50_000, 3, 0),
		}
		baseline := makeBaseline(1.0, 0.3)
		status := ComputeRegressionStatus(makeRegressionInput(sessions, nil, baseline, 1.5))
		if status.Message != "no regression detected" {
			t.Errorf("Message = %q, want %q", status.Message, "no regression detected")
		}
	})

	t.Run("friction_only", func(t *testing.T) {
		// frictionRate = 1.0, baseline 0.3, threshold 1.5 → regressed
		// cost = ~1.05, baseline 2.0, 1.5*2.0=3.0 → not regressed
		sessions := []claude.SessionMeta{
			makeMeta("s1", "2026-01-01T10:00:00Z", 100_000, 50_000, 3, 5),
			makeMeta("s2", "2026-01-02T10:00:00Z", 100_000, 50_000, 3, 3),
			makeMeta("s3", "2026-01-03T10:00:00Z", 100_000, 50_000, 3, 2),
		}
		baseline := makeBaseline(2.0, 0.3)
		status := ComputeRegressionStatus(makeRegressionInput(sessions, nil, baseline, 1.5))
		if !strings.Contains(status.Message, "friction rate regressed") {
			t.Errorf("Message should mention friction regression, got %q", status.Message)
		}
		if strings.Contains(status.Message, "cost regressed") {
			t.Errorf("Message should not mention cost regression for friction-only, got %q", status.Message)
		}
		if !strings.Contains(status.Message, "threshold") {
			t.Errorf("Message should include threshold for single regression, got %q", status.Message)
		}
	})

	t.Run("cost_only", func(t *testing.T) {
		// cost = ~9.0, baseline 1.0, 1.5*1.0=1.5 → regressed
		// frictionRate = 0.0, baseline 0.3, 1.5*0.3=0.45 → not regressed
		sessions := []claude.SessionMeta{
			makeMeta("s1", "2026-01-01T10:00:00Z", 2_000_000, 200_000, 3, 0),
			makeMeta("s2", "2026-01-02T10:00:00Z", 2_000_000, 200_000, 3, 0),
			makeMeta("s3", "2026-01-03T10:00:00Z", 2_000_000, 200_000, 3, 0),
		}
		baseline := makeBaseline(1.0, 0.3)
		status := ComputeRegressionStatus(makeRegressionInput(sessions, nil, baseline, 1.5))
		if !strings.Contains(status.Message, "cost regressed") {
			t.Errorf("Message should mention cost regression, got %q", status.Message)
		}
		if strings.Contains(status.Message, "friction rate regressed") {
			t.Errorf("Message should not mention friction regression for cost-only, got %q", status.Message)
		}
		if !strings.Contains(status.Message, "threshold") {
			t.Errorf("Message should include threshold for single regression, got %q", status.Message)
		}
	})

	t.Run("both_regressed", func(t *testing.T) {
		sessions := []claude.SessionMeta{
			makeMeta("s1", "2026-01-01T10:00:00Z", 2_000_000, 200_000, 3, 5),
			makeMeta("s2", "2026-01-02T10:00:00Z", 2_000_000, 200_000, 3, 3),
			makeMeta("s3", "2026-01-03T10:00:00Z", 2_000_000, 200_000, 3, 2),
		}
		baseline := makeBaseline(1.0, 0.3)
		status := ComputeRegressionStatus(makeRegressionInput(sessions, nil, baseline, 1.5))
		if !strings.Contains(status.Message, "friction rate regressed") {
			t.Errorf("Message should mention friction regression, got %q", status.Message)
		}
		if !strings.Contains(status.Message, "cost regressed") {
			t.Errorf("Message should mention cost regression, got %q", status.Message)
		}
		// Both-regressed message does NOT include threshold per spec.
		if strings.Contains(status.Message, "threshold") {
			t.Errorf("Both-regressed message should not include threshold, got %q", status.Message)
		}
	})
}

func TestComputeRegressionStatus_ExactlyAtThreshold(t *testing.T) {
	// current == threshold * baseline should NOT trigger regression (must be strictly greater).
	// We need: currentFrictionRate = threshold * baseline.AvgFriction exactly.
	// Use baseline AvgFriction=0.4, threshold=2.5 → threshold*baseline = 1.0
	// With 3 sessions all having friction, frictionRate = 1.0. Exactly at threshold. Should NOT regress.
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 100_000, 50_000, 3, 5),
		makeMeta("s2", "2026-01-02T10:00:00Z", 100_000, 50_000, 3, 3),
		makeMeta("s3", "2026-01-03T10:00:00Z", 100_000, 50_000, 3, 2),
	}
	// AvgFriction=0.4, threshold=2.5 → threshold*AvgFriction=1.0 exactly
	// frictionRate = 3/3 = 1.0, which is == 1.0, NOT strictly greater, so no regression.
	baseline := &store.ProjectBaseline{
		Project:      "testproject",
		ComputedAt:   "2026-01-01T00:00:00Z",
		SessionCount: 10,
		AvgCostUSD:   100.0, // very high baseline cost to avoid cost regression
		AvgFriction:  0.4,
	}
	input := RegressionInput{
		Project:        "testproject",
		Baseline:       baseline,
		RecentSessions: sessions,
		Facets:         nil,
		Pricing:        testPricingRegression,
		CacheRatio:     NoCacheRatio(),
		Threshold:      2.5,
	}
	status := ComputeRegressionStatus(input)

	if status.FrictionRegressed {
		t.Errorf("FrictionRegressed should be false when current (%.4f) == threshold*baseline (%.4f), must be strictly greater",
			status.CurrentFrictionRate, 2.5*baseline.AvgFriction)
	}
}

func TestComputeRegressionStatus_PerModelCost(t *testing.T) {
	// Verify regression detection uses per-model pricing when ModelUsage is populated.
	// Sessions with Opus ModelUsage: cost should be much higher than Sonnet baseline.
	// Opus: 100K input * $15/M = $1.50, 50K output * $75/M = $3.75 → total $5.25 per session
	// Baseline avg cost = $1.00, threshold = 1.5 → threshold*baseline = $1.50
	// $5.25 > $1.50 → cost regressed
	sessions := []claude.SessionMeta{
		{
			SessionID:    "s1",
			StartTime:    "2026-01-01T10:00:00Z",
			InputTokens:  100_000,
			OutputTokens: 50_000,
			GitCommits:   3,
			ModelUsage: map[string]claude.ModelStats{
				"claude-3-opus-20240229": {InputTokens: 100_000, OutputTokens: 50_000},
			},
		},
		{
			SessionID:    "s2",
			StartTime:    "2026-01-02T10:00:00Z",
			InputTokens:  100_000,
			OutputTokens: 50_000,
			GitCommits:   3,
			ModelUsage: map[string]claude.ModelStats{
				"claude-3-opus-20240229": {InputTokens: 100_000, OutputTokens: 50_000},
			},
		},
		{
			SessionID:    "s3",
			StartTime:    "2026-01-03T10:00:00Z",
			InputTokens:  100_000,
			OutputTokens: 50_000,
			GitCommits:   3,
			ModelUsage: map[string]claude.ModelStats{
				"claude-3-opus-20240229": {InputTokens: 100_000, OutputTokens: 50_000},
			},
		},
	}
	baseline := makeBaseline(1.0, 0.3)
	input := makeRegressionInput(sessions, nil, baseline, 1.5)
	status := ComputeRegressionStatus(input)

	if !status.CostRegressed {
		t.Errorf("CostRegressed should be true with Opus per-model pricing (currentAvgCost=%.4f vs threshold=%.4f)",
			status.CurrentAvgCostUSD, 1.5*baseline.AvgCostUSD)
	}
	// Verify cost is Opus-priced, not Sonnet-priced.
	// Sonnet would give: 100K*3/M + 50K*15/M = 0.30 + 0.75 = 1.05
	// Opus gives: 100K*15/M + 50K*75/M = 1.50 + 3.75 = 5.25
	if status.CurrentAvgCostUSD < 5.0 {
		t.Errorf("CurrentAvgCostUSD = %.4f, expected ~5.25 (Opus pricing), not ~1.05 (Sonnet)", status.CurrentAvgCostUSD)
	}
}

func TestComputeRegressionStatus_FacetsUsed(t *testing.T) {
	// Verify that facets are used to compute friction (overriding ToolErrors).
	// s1 has ToolErrors=0 but facet with friction=5 → friction>0
	// s2 has ToolErrors=0, no facet → friction=0
	// s3 has ToolErrors=0, no facet → friction=0
	// frictionRate = 1/3 ≈ 0.333
	// baseline AvgFriction=0.1, threshold=1.5 → 1.5*0.1=0.15
	// 0.333 > 0.15 → regressed
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 100_000, 50_000, 3, 0),
		makeMeta("s2", "2026-01-02T10:00:00Z", 100_000, 50_000, 3, 0),
		makeMeta("s3", "2026-01-03T10:00:00Z", 100_000, 50_000, 3, 0),
	}
	facets := []claude.SessionFacet{
		makeFacet("s1", map[string]int{"retry": 5}),
	}
	baseline := makeBaseline(2.0, 0.1) // high cost baseline so no cost regression
	input := makeRegressionInput(sessions, facets, baseline, 1.5)
	status := ComputeRegressionStatus(input)

	// frictionRate should reflect the facet-based friction, not ToolErrors.
	expectedRate := 1.0 / 3.0
	if status.CurrentFrictionRate < expectedRate-1e-9 || status.CurrentFrictionRate > expectedRate+1e-9 {
		t.Errorf("CurrentFrictionRate = %f, want %f (facet-based)", status.CurrentFrictionRate, expectedRate)
	}
	if !status.FrictionRegressed {
		t.Errorf("FrictionRegressed should be true (%.4f > %.4f)", status.CurrentFrictionRate, 1.5*baseline.AvgFriction)
	}
}
