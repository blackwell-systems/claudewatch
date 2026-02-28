package analyzer

import (
	"math"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

func TestEstimateCosts_DefaultModel(t *testing.T) {
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"claude-sonnet-4-20250514": {
				InputTokens:              1_000_000,
				OutputTokens:             500_000,
				CacheReadInputTokens:     200_000,
				CacheCreationInputTokens: 100_000,
			},
		},
		DailyActivity: []claude.DailyActivity{
			{Date: "2026-01-01"},
			{Date: "2026-01-02"},
		},
	}

	// Empty model string should default to sonnet pricing.
	est := EstimateCosts(stats, "", 10, 5)

	// Verify sonnet pricing was used.
	// Input: 1M * 3.0 / 1M = 3.0
	wantInput := 3.0
	if diff := est.InputCost - wantInput; diff > 0.001 || diff < -0.001 {
		t.Errorf("InputCost = %.4f, want %.4f", est.InputCost, wantInput)
	}

	// Output: 0.5M * 15.0 / 1M = 7.5
	wantOutput := 7.5
	if diff := est.OutputCost - wantOutput; diff > 0.001 || diff < -0.001 {
		t.Errorf("OutputCost = %.4f, want %.4f", est.OutputCost, wantOutput)
	}

	// CacheRead: 0.2M * 0.3 / 1M = 0.06
	wantCacheRead := 0.06
	if diff := est.CacheReadCost - wantCacheRead; diff > 0.001 || diff < -0.001 {
		t.Errorf("CacheReadCost = %.4f, want %.4f", est.CacheReadCost, wantCacheRead)
	}

	// CacheWrite: 0.1M * 3.75 / 1M = 0.375
	wantCacheWrite := 0.375
	if diff := est.CacheWriteCost - wantCacheWrite; diff > 0.001 || diff < -0.001 {
		t.Errorf("CacheWriteCost = %.4f, want %.4f", est.CacheWriteCost, wantCacheWrite)
	}

	// Total = 3.0 + 7.5 + 0.06 + 0.375 = 10.935
	wantTotal := 10.935
	if diff := est.TotalCost - wantTotal; diff > 0.001 || diff < -0.001 {
		t.Errorf("TotalCost = %.4f, want %.4f", est.TotalCost, wantTotal)
	}

	// CostPerSession = 10.935 / 10 = 1.0935
	wantPerSession := wantTotal / 10.0
	if diff := est.CostPerSession - wantPerSession; diff > 0.001 || diff < -0.001 {
		t.Errorf("CostPerSession = %.4f, want %.4f", est.CostPerSession, wantPerSession)
	}

	// CostPerCommit = 10.935 / 5 = 2.187
	wantPerCommit := wantTotal / 5.0
	if diff := est.CostPerCommit - wantPerCommit; diff > 0.001 || diff < -0.001 {
		t.Errorf("CostPerCommit = %.4f, want %.4f", est.CostPerCommit, wantPerCommit)
	}

	// CostPerDay = 10.935 / 2 = 5.4675
	wantPerDay := wantTotal / 2.0
	if diff := est.CostPerDay - wantPerDay; diff > 0.001 || diff < -0.001 {
		t.Errorf("CostPerDay = %.4f, want %.4f", est.CostPerDay, wantPerDay)
	}
}

func TestEstimateCosts_OpusPricing(t *testing.T) {
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"claude-opus-4": {
				InputTokens:  2_000_000,
				OutputTokens: 1_000_000,
			},
		},
	}

	est := EstimateCosts(stats, "opus", 1, 1)

	// Input: 2M * 15.0 / 1M = 30.0
	wantInput := 30.0
	if diff := est.InputCost - wantInput; diff > 0.001 || diff < -0.001 {
		t.Errorf("InputCost = %.4f, want %.4f (opus pricing)", est.InputCost, wantInput)
	}

	// Output: 1M * 75.0 / 1M = 75.0
	wantOutput := 75.0
	if diff := est.OutputCost - wantOutput; diff > 0.001 || diff < -0.001 {
		t.Errorf("OutputCost = %.4f, want %.4f (opus pricing)", est.OutputCost, wantOutput)
	}
}

func TestEstimateCosts_HaikuPricing(t *testing.T) {
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"claude-haiku": {
				InputTokens:  1_000_000,
				OutputTokens: 1_000_000,
			},
		},
	}

	est := EstimateCosts(stats, "haiku", 1, 1)

	// Input: 1M * 0.25 / 1M = 0.25
	wantInput := 0.25
	if diff := est.InputCost - wantInput; diff > 0.001 || diff < -0.001 {
		t.Errorf("InputCost = %.4f, want %.4f (haiku pricing)", est.InputCost, wantInput)
	}

	// Output: 1M * 1.25 / 1M = 1.25
	wantOutput := 1.25
	if diff := est.OutputCost - wantOutput; diff > 0.001 || diff < -0.001 {
		t.Errorf("OutputCost = %.4f, want %.4f (haiku pricing)", est.OutputCost, wantOutput)
	}
}

func TestEstimateCosts_UnknownModelFallsBackToSonnet(t *testing.T) {
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"model-x": {
				InputTokens: 1_000_000,
			},
		},
	}

	est := EstimateCosts(stats, "unknown-model", 1, 1)

	// Should use sonnet pricing: 1M * 3.0 / 1M = 3.0
	wantInput := 3.0
	if diff := est.InputCost - wantInput; diff > 0.001 || diff < -0.001 {
		t.Errorf("InputCost = %.4f, want %.4f (fallback to sonnet)", est.InputCost, wantInput)
	}
}

func TestEstimateCosts_CacheSavings(t *testing.T) {
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"model": {
				CacheReadInputTokens: 1_000_000,
			},
		},
	}

	est := EstimateCosts(stats, "sonnet", 1, 1)

	// CacheRead cost: 1M * 0.3 / 1M = 0.3
	// Full input cost for those tokens: 1M * 3.0 / 1M = 3.0
	// Savings: 3.0 - 0.3 = 2.7
	wantSavings := 2.7
	if diff := est.CacheSavings - wantSavings; diff > 0.001 || diff < -0.001 {
		t.Errorf("CacheSavings = %.4f, want %.4f", est.CacheSavings, wantSavings)
	}

	// Savings percent: 2.7 / (0.3 + 2.7) * 100 = 90%
	wantPercent := 90.0
	if diff := est.CacheSavingsPercent - wantPercent; diff > 0.1 || diff < -0.1 {
		t.Errorf("CacheSavingsPercent = %.2f, want %.2f", est.CacheSavingsPercent, wantPercent)
	}
}

func TestEstimateCosts_ZeroSessionsAndCommits(t *testing.T) {
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"model": {
				InputTokens:  1_000_000,
				OutputTokens: 500_000,
			},
		},
	}

	est := EstimateCosts(stats, "sonnet", 0, 0)

	if est.CostPerSession != 0 {
		t.Errorf("CostPerSession should be 0 with 0 sessions, got %f", est.CostPerSession)
	}

	if !math.IsInf(est.CostPerCommit, 1) {
		t.Errorf("CostPerCommit should be +Inf with 0 commits, got %f", est.CostPerCommit)
	}

	if est.CostPerDay != 0 {
		t.Errorf("CostPerDay should be 0 with 0 daily activity, got %f", est.CostPerDay)
	}
}

func TestEstimateCosts_EmptyStats(t *testing.T) {
	stats := claude.StatsCache{}

	est := EstimateCosts(stats, "sonnet", 0, 0)

	if est.TotalCost != 0 {
		t.Errorf("TotalCost should be 0 for empty stats, got %f", est.TotalCost)
	}
	if est.InputCost != 0 {
		t.Errorf("InputCost should be 0, got %f", est.InputCost)
	}
	if est.OutputCost != 0 {
		t.Errorf("OutputCost should be 0, got %f", est.OutputCost)
	}
}

func TestEstimateCosts_MultipleModelsInStats(t *testing.T) {
	// Multiple model entries in stats should be aggregated.
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"model-a": {
				InputTokens:  500_000,
				OutputTokens: 200_000,
			},
			"model-b": {
				InputTokens:  500_000,
				OutputTokens: 300_000,
			},
		},
	}

	est := EstimateCosts(stats, "sonnet", 1, 1)

	// Total input: 1M, total output: 0.5M
	wantInput := 3.0  // 1M * 3.0/1M
	wantOutput := 7.5 // 0.5M * 15.0/1M
	wantTotal := wantInput + wantOutput

	if diff := est.TotalCost - wantTotal; diff > 0.001 || diff < -0.001 {
		t.Errorf("TotalCost = %.4f, want %.4f (aggregated from multiple models)", est.TotalCost, wantTotal)
	}
}

func TestEstimateCosts_PerDayCost(t *testing.T) {
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"model": {
				InputTokens: 1_000_000,
			},
		},
		DailyActivity: []claude.DailyActivity{
			{Date: "2026-01-01"},
			{Date: "2026-01-02"},
			{Date: "2026-01-03"},
			{Date: "2026-01-04"},
			{Date: "2026-01-05"},
		},
	}

	est := EstimateCosts(stats, "sonnet", 5, 5)

	// Total = 3.0 (input only), 5 days -> 0.6 per day
	wantPerDay := 3.0 / 5.0
	if diff := est.CostPerDay - wantPerDay; diff > 0.001 || diff < -0.001 {
		t.Errorf("CostPerDay = %.4f, want %.4f", est.CostPerDay, wantPerDay)
	}
}

func TestTokensToCost(t *testing.T) {
	tests := []struct {
		name       string
		tokens     int64
		perMillion float64
		want       float64
	}{
		{"1M tokens at $3/M", 1_000_000, 3.0, 3.0},
		{"500K tokens at $15/M", 500_000, 15.0, 7.5},
		{"0 tokens", 0, 3.0, 0.0},
		{"1 token", 1, 1_000_000.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokensToCost(tt.tokens, tt.perMillion)
			if diff := got - tt.want; diff > 0.001 || diff < -0.001 {
				t.Errorf("tokensToCost(%d, %.2f) = %.4f, want %.4f", tt.tokens, tt.perMillion, got, tt.want)
			}
		})
	}
}

func TestEstimateCosts_CaseInsensitiveModel(t *testing.T) {
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"model": {InputTokens: 1_000_000},
		},
	}

	// "OPUS" should be normalized to "opus".
	est := EstimateCosts(stats, "OPUS", 1, 1)

	// Opus input: 1M * 15.0/1M = 15.0
	wantInput := 15.0
	if diff := est.InputCost - wantInput; diff > 0.001 || diff < -0.001 {
		t.Errorf("InputCost = %.4f, want %.4f (case-insensitive model)", est.InputCost, wantInput)
	}
}

func TestComputeCacheRatio(t *testing.T) {
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"model-a": {
				InputTokens:              100,
				CacheReadInputTokens:     800,
				CacheCreationInputTokens: 100,
			},
			"model-b": {
				InputTokens:              100,
				CacheReadInputTokens:     400,
				CacheCreationInputTokens: 100,
			},
		},
	}

	ratio := ComputeCacheRatio(stats)

	// Uncached: 200, CacheRead: 1200, CacheWrite: 200
	// CacheReadMultiplier = 1200/200 = 6.0
	// CacheWriteMultiplier = 200/200 = 1.0
	if diff := ratio.CacheReadMultiplier - 6.0; diff > 0.01 || diff < -0.01 {
		t.Errorf("CacheReadMultiplier = %.2f, want 6.0", ratio.CacheReadMultiplier)
	}
	if diff := ratio.CacheWriteMultiplier - 1.0; diff > 0.01 || diff < -0.01 {
		t.Errorf("CacheWriteMultiplier = %.2f, want 1.0", ratio.CacheWriteMultiplier)
	}
}

func TestComputeCacheRatio_Empty(t *testing.T) {
	ratio := ComputeCacheRatio(claude.StatsCache{})
	if ratio.CacheReadMultiplier != 0 || ratio.CacheWriteMultiplier != 0 {
		t.Errorf("expected zero multipliers for empty stats, got read=%.2f write=%.2f",
			ratio.CacheReadMultiplier, ratio.CacheWriteMultiplier)
	}
}

func TestComputeCacheRatio_ZeroUncached(t *testing.T) {
	stats := claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"model": {
				InputTokens:          0,
				CacheReadInputTokens: 1000,
			},
		},
	}
	ratio := ComputeCacheRatio(stats)
	if ratio.CacheReadMultiplier != 0 {
		t.Errorf("expected zero multiplier when uncached=0, got %.2f", ratio.CacheReadMultiplier)
	}
}

func TestNoCacheRatio(t *testing.T) {
	ratio := NoCacheRatio()
	if ratio.CacheReadMultiplier != 0 || ratio.CacheWriteMultiplier != 0 {
		t.Errorf("NoCacheRatio should have zero multipliers")
	}
}
