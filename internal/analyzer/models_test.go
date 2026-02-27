package analyzer

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

func TestClassifyModelTier(t *testing.T) {
	tests := []struct {
		name string
		want ModelTier
	}{
		{"claude-opus-4-20250514", TierOpus},
		{"claude-sonnet-4-20250514", TierSonnet},
		{"claude-haiku-4-20250514", TierHaiku},
		{"claude-3-5-sonnet-20241022", TierSonnet},
		{"unknown-model", TierOther},
	}
	for _, tt := range tests {
		got := ClassifyModelTier(tt.name)
		if got != tt.want {
			t.Errorf("ClassifyModelTier(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestAnalyzeModels_Nil(t *testing.T) {
	result := AnalyzeModels(nil)
	if len(result.Models) != 0 {
		t.Errorf("expected 0 models for nil stats, got %d", len(result.Models))
	}
}

func TestAnalyzeModels_SingleModel(t *testing.T) {
	stats := &claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"claude-sonnet-4-20250514": {
				InputTokens:  1_000_000,
				OutputTokens: 200_000,
				CostUSD:      4.50,
			},
		},
	}

	result := AnalyzeModels(stats)

	if len(result.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(result.Models))
	}
	if result.Models[0].Tier != TierSonnet {
		t.Errorf("expected sonnet tier, got %q", result.Models[0].Tier)
	}
	if result.DominantTier != TierSonnet {
		t.Errorf("expected dominant tier sonnet, got %q", result.DominantTier)
	}
	if result.Models[0].CostPercent != 100 {
		t.Errorf("expected 100%% cost, got %.1f%%", result.Models[0].CostPercent)
	}
}

func TestAnalyzeModels_MultipleModels(t *testing.T) {
	stats := &claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"claude-opus-4-20250514": {
				InputTokens:  500_000,
				OutputTokens: 100_000,
				CostUSD:      15.00,
			},
			"claude-sonnet-4-20250514": {
				InputTokens:  2_000_000,
				OutputTokens: 400_000,
				CostUSD:      8.00,
			},
			"claude-haiku-4-20250514": {
				InputTokens:  300_000,
				OutputTokens: 50_000,
				CostUSD:      0.50,
			},
		},
	}

	result := AnalyzeModels(stats)

	if len(result.Models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(result.Models))
	}

	// Sorted by cost descending — Opus first.
	if result.Models[0].Tier != TierOpus {
		t.Errorf("expected first model to be opus, got %q", result.Models[0].Tier)
	}

	// Dominant tier by cost is Opus ($15).
	if result.DominantTier != TierOpus {
		t.Errorf("expected dominant tier opus, got %q", result.DominantTier)
	}

	// Total cost.
	expectedTotal := 23.50
	if diff := result.TotalCost - expectedTotal; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected total cost %.2f, got %.2f", expectedTotal, result.TotalCost)
	}

	// Opus cost percent: 15/23.50 ≈ 63.8%.
	if result.OpusCostPercent < 63 || result.OpusCostPercent > 64 {
		t.Errorf("expected ~63.8%% opus cost, got %.1f%%", result.OpusCostPercent)
	}

	// Potential savings should be positive (Opus at Sonnet rates is cheaper).
	if result.PotentialSavings <= 0 {
		t.Errorf("expected positive potential savings, got %.2f", result.PotentialSavings)
	}
}

func TestAnalyzeModels_PotentialSavings(t *testing.T) {
	// 1M input + 200K output on Opus.
	// Opus: 1M * $15/M + 200K * $75/M = $15 + $15 = $30
	// Sonnet: 1M * $3/M + 200K * $15/M = $3 + $3 = $6
	// Savings: $30 - $6 = $24
	stats := &claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"claude-opus-4-20250514": {
				InputTokens:  1_000_000,
				OutputTokens: 200_000,
				CostUSD:      30.00,
			},
		},
	}

	result := AnalyzeModels(stats)

	if diff := result.PotentialSavings - 24.0; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected $24.00 savings, got $%.2f", result.PotentialSavings)
	}
}

func TestAnalyzeModels_DailyTrend(t *testing.T) {
	stats := &claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"claude-sonnet-4-20250514": {InputTokens: 100, CostUSD: 1.0},
		},
		DailyModelTokens: []claude.DailyModelTokens{
			{
				Date: "2026-01-10",
				TokensByModel: map[string]int{
					"claude-sonnet-4-20250514": 50000,
					"claude-opus-4-20250514":   10000,
				},
			},
			{
				Date: "2026-01-11",
				TokensByModel: map[string]int{
					"claude-opus-4-20250514": 80000,
				},
			},
		},
	}

	result := AnalyzeModels(stats)

	if len(result.DailyTrend) != 2 {
		t.Fatalf("expected 2 daily entries, got %d", len(result.DailyTrend))
	}
	if result.DailyTrend[0].DominantTier != TierSonnet {
		t.Errorf("day 1: expected sonnet dominant, got %q", result.DailyTrend[0].DominantTier)
	}
	if result.DailyTrend[1].DominantTier != TierOpus {
		t.Errorf("day 2: expected opus dominant, got %q", result.DailyTrend[1].DominantTier)
	}
}

func TestAnalyzeTokens_Empty(t *testing.T) {
	ts := AnalyzeTokens(nil, nil)
	if ts.TotalTokens != 0 {
		t.Errorf("expected 0 tokens, got %d", ts.TotalTokens)
	}
}

func TestAnalyzeTokens_FromStats(t *testing.T) {
	stats := &claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"claude-sonnet-4-20250514": {
				InputTokens:              5_000_000,
				OutputTokens:             1_000_000,
				CacheReadInputTokens:     3_000_000,
				CacheCreationInputTokens: 500_000,
			},
		},
	}

	ts := AnalyzeTokens(stats, nil)

	if ts.TotalInput != 5_000_000 {
		t.Errorf("expected 5M input, got %d", ts.TotalInput)
	}
	if ts.TotalOutput != 1_000_000 {
		t.Errorf("expected 1M output, got %d", ts.TotalOutput)
	}
	if ts.TotalTokens != 6_000_000 {
		t.Errorf("expected 6M total, got %d", ts.TotalTokens)
	}
	if ts.TotalCacheReads != 3_000_000 {
		t.Errorf("expected 3M cache reads, got %d", ts.TotalCacheReads)
	}

	// Cache hit rate: 3M / (3M + 5M) = 37.5%.
	if ts.CacheHitRate < 37 || ts.CacheHitRate > 38 {
		t.Errorf("expected ~37.5%% cache hit rate, got %.1f%%", ts.CacheHitRate)
	}

	// I/O ratio: 5M / 1M = 5.0.
	if ts.InputOutputRatio < 4.9 || ts.InputOutputRatio > 5.1 {
		t.Errorf("expected ~5.0 I/O ratio, got %.1f", ts.InputOutputRatio)
	}
}

func TestAnalyzeTokens_PerSession(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", InputTokens: 100_000, OutputTokens: 20_000},
		{SessionID: "s2", InputTokens: 200_000, OutputTokens: 40_000},
	}

	ts := AnalyzeTokens(nil, sessions)

	if ts.TotalSessions != 2 {
		t.Errorf("expected 2 sessions, got %d", ts.TotalSessions)
	}
	// Avg input: 150K.
	if ts.AvgInputPerSession != 150_000 {
		t.Errorf("expected 150K avg input, got %d", ts.AvgInputPerSession)
	}
	// Avg output: 30K.
	if ts.AvgOutputPerSession != 30_000 {
		t.Errorf("expected 30K avg output, got %d", ts.AvgOutputPerSession)
	}
	// Avg total: 180K.
	if ts.AvgTotalPerSession != 180_000 {
		t.Errorf("expected 180K avg total, got %d", ts.AvgTotalPerSession)
	}
}

func TestAnalyzeModels_NoOpusSavings(t *testing.T) {
	// Only Sonnet usage — no potential savings.
	stats := &claude.StatsCache{
		ModelUsage: map[string]claude.ModelUsage{
			"claude-sonnet-4-20250514": {
				InputTokens:  1_000_000,
				OutputTokens: 200_000,
				CostUSD:      6.00,
			},
		},
	}

	result := AnalyzeModels(stats)

	if result.PotentialSavings != 0 {
		t.Errorf("expected $0 savings with no opus, got $%.2f", result.PotentialSavings)
	}
	if result.OpusCostPercent != 0 {
		t.Errorf("expected 0%% opus cost, got %.1f%%", result.OpusCostPercent)
	}
}
