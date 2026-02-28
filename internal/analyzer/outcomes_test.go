package analyzer

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

var testPricing = ModelPricing{
	InputPerMillion:      3.0,
	OutputPerMillion:     15.0,
	CacheReadPerMillion:  0.3,
	CacheWritePerMillion: 3.75,
}

func TestAnalyzeOutcomes_Empty(t *testing.T) {
	result := AnalyzeOutcomes(nil, nil, testPricing, NoCacheRatio())
	if len(result.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(result.Sessions))
	}
}

func TestAnalyzeOutcomes_BasicCosts(t *testing.T) {
	sessions := []claude.SessionMeta{
		{
			SessionID:     "s1",
			ProjectPath:   "/home/user/proj",
			StartTime:     "2026-01-10T10:00:00Z",
			InputTokens:   1_000_000, // $3.00
			OutputTokens:  100_000,   // $1.50
			GitCommits:    3,
			FilesModified: 5,
			LinesAdded:    200,
		},
	}

	result := AnalyzeOutcomes(sessions, nil, testPricing, NoCacheRatio())

	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result.Sessions))
	}

	// $3.00 input + $1.50 output = $4.50
	expectedCost := 4.50
	if diff := result.TotalCost - expectedCost; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected total cost %.2f, got %.2f", expectedCost, result.TotalCost)
	}

	// $4.50 / 3 commits = $1.50
	if diff := result.AvgCostPerCommit - 1.50; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected cost/commit $1.50, got $%.2f", result.AvgCostPerCommit)
	}

	if result.TotalCommits != 3 {
		t.Errorf("expected 3 commits, got %d", result.TotalCommits)
	}
}

func TestAnalyzeOutcomes_GoalAchievement(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-10T10:00:00Z", InputTokens: 100_000},
		{SessionID: "s2", StartTime: "2026-01-11T10:00:00Z", InputTokens: 100_000},
		{SessionID: "s3", StartTime: "2026-01-12T10:00:00Z", InputTokens: 100_000},
	}
	facets := []claude.SessionFacet{
		{SessionID: "s1", Outcome: "achieved"},
		{SessionID: "s2", Outcome: "mostly_achieved"},
		{SessionID: "s3", Outcome: "not_achieved"},
	}

	result := AnalyzeOutcomes(sessions, facets, testPricing, NoCacheRatio())

	// 2 out of 3 achieved/mostly_achieved
	if result.GoalAchievementRate < 0.66 || result.GoalAchievementRate > 0.67 {
		t.Errorf("expected ~66%% goal rate, got %.2f", result.GoalAchievementRate)
	}
}

func TestAnalyzeOutcomes_Trend(t *testing.T) {
	// Earlier sessions: expensive per commit. Later: cheaper.
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-01T10:00:00Z", InputTokens: 2_000_000, GitCommits: 1},
		{SessionID: "s2", StartTime: "2026-01-02T10:00:00Z", InputTokens: 2_000_000, GitCommits: 1},
		{SessionID: "s3", StartTime: "2026-01-10T10:00:00Z", InputTokens: 500_000, GitCommits: 2},
		{SessionID: "s4", StartTime: "2026-01-11T10:00:00Z", InputTokens: 500_000, GitCommits: 2},
	}

	result := AnalyzeOutcomes(sessions, nil, testPricing, NoCacheRatio())

	if result.CostPerCommitTrend != "improving" {
		t.Errorf("expected improving trend, got %q", result.CostPerCommitTrend)
	}
}

func TestAnalyzeOutcomes_TrendInsufficientData(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-01T10:00:00Z", InputTokens: 100_000, GitCommits: 1},
	}

	result := AnalyzeOutcomes(sessions, nil, testPricing, NoCacheRatio())

	if result.CostPerCommitTrend != "insufficient_data" {
		t.Errorf("expected insufficient_data, got %q", result.CostPerCommitTrend)
	}
}

func TestAnalyzeOutcomes_ByProject(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/proj/a", StartTime: "2026-01-01T10:00:00Z", InputTokens: 1_000_000, GitCommits: 2},
		{SessionID: "s2", ProjectPath: "/proj/a", StartTime: "2026-01-02T10:00:00Z", InputTokens: 1_000_000, GitCommits: 3},
		{SessionID: "s3", ProjectPath: "/proj/b", StartTime: "2026-01-03T10:00:00Z", InputTokens: 500_000, GitCommits: 1},
	}

	result := AnalyzeOutcomes(sessions, nil, testPricing, NoCacheRatio())

	if len(result.ByProject) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(result.ByProject))
	}

	// Sorted by total cost descending — /proj/a should be first.
	if result.ByProject[0].ProjectName != "a" {
		t.Errorf("expected first project 'a', got %q", result.ByProject[0].ProjectName)
	}
}

func TestCostPerGoal(t *testing.T) {
	outcomes := OutcomeAnalysis{
		Sessions: []SessionOutcome{
			{Cost: 5.00, GoalAchieved: true, Outcome: "achieved"},
			{Cost: 3.00, GoalAchieved: true, Outcome: "achieved"},
			{Cost: 8.00, GoalAchieved: false, Outcome: "not_achieved"},
		},
	}

	achieved, notAchieved := CostPerGoal(outcomes)

	if diff := achieved - 4.00; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected achieved avg $4.00, got $%.2f", achieved)
	}
	if diff := notAchieved - 8.00; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected not-achieved avg $8.00, got $%.2f", notAchieved)
	}
}

func TestMedianFloat64(t *testing.T) {
	tests := []struct {
		name string
		vals []float64
		want float64
	}{
		{"empty", nil, 0},
		{"single", []float64{5.0}, 5.0},
		{"odd", []float64{1.0, 3.0, 2.0}, 2.0},
		{"even", []float64{1.0, 2.0, 3.0, 4.0}, 2.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := medianFloat64(tt.vals)
			if got != tt.want {
				t.Errorf("medianFloat64(%v) = %v, want %v", tt.vals, got, tt.want)
			}
		})
	}
}

func TestEstimateSessionCost_Exported(t *testing.T) {
	s := claude.SessionMeta{
		InputTokens:  1_000_000, // $3.00 at $3/M
		OutputTokens: 100_000,   // $1.50 at $15/M
	}

	got := EstimateSessionCost(s, testPricing, NoCacheRatio())

	// Expected: uncached input $3.00 + output $1.50 = $4.50 (no cache costs with NoCacheRatio)
	expected := 4.50
	if diff := got - expected; diff > 0.001 || diff < -0.001 {
		t.Errorf("EstimateSessionCost() = %.4f, want %.4f", got, expected)
	}
}

func TestAnalyzeOutcomes_WithCacheRatio(t *testing.T) {
	// Simulate a cache ratio where cache reads are 2900x uncached input
	// and cache writes are 395x uncached input (realistic from stats-cache data).
	ratio := CacheRatio{
		CacheReadMultiplier:  2900.0,
		CacheWriteMultiplier: 395.0,
	}
	sessions := []claude.SessionMeta{
		{
			SessionID:    "s1",
			StartTime:    "2026-01-10T10:00:00Z",
			InputTokens:  1000, // 1K uncached input tokens
			OutputTokens: 100_000,
			GitCommits:   1,
		},
	}

	withCache := AnalyzeOutcomes(sessions, nil, testPricing, ratio)
	withoutCache := AnalyzeOutcomes(sessions, nil, testPricing, NoCacheRatio())

	// With cache ratio, cost should be higher (includes estimated cache costs).
	if withCache.TotalCost <= withoutCache.TotalCost {
		t.Errorf("cache-adjusted cost ($%.4f) should be > no-cache cost ($%.4f)",
			withCache.TotalCost, withoutCache.TotalCost)
	}

	// Verify the math:
	// uncached: 1000/1M * 3.0 = 0.003
	// cache read: (1000 * 2900) / 1M * 0.3 = 0.87
	// cache write: (1000 * 395) / 1M * 3.75 = 1.48125
	// output: 100_000/1M * 15.0 = 1.50
	// total ~= 3.854
	expected := 0.003 + 0.87 + 1.48125 + 1.50
	if diff := withCache.TotalCost - expected; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected total cost ~$%.4f, got $%.4f", expected, withCache.TotalCost)
	}
}
