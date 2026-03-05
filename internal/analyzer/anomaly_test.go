package analyzer

import (
	"math"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

var testPricingAnomaly = ModelPricing{
	InputPerMillion:      3.0,
	OutputPerMillion:     15.0,
	CacheReadPerMillion:  0.3,
	CacheWritePerMillion: 3.75,
}

// makeBaselineInput builds a BaselineInput with the given sessions.
func makeBaselineInput(sessions []claude.SessionMeta, facets []claude.SessionFacet, sawIDs map[string]bool) BaselineInput {
	return BaselineInput{
		Project:    "testproject",
		Sessions:   sessions,
		Facets:     facets,
		SAWIDs:     sawIDs,
		Pricing:    testPricingAnomaly,
		CacheRatio: NoCacheRatio(),
	}
}

func TestComputeProjectBaseline_TooFewSessions(t *testing.T) {
	input := makeBaselineInput(
		[]claude.SessionMeta{
			makeMeta("s1", "2026-01-01T10:00:00Z", 1_000_000, 100_000, 3, 0),
			makeMeta("s2", "2026-01-02T10:00:00Z", 2_000_000, 200_000, 5, 0),
		},
		nil,
		nil,
	)
	_, err := ComputeProjectBaseline(input)
	if err == nil {
		t.Error("expected error for fewer than 3 sessions, got nil")
	}
}

func TestComputeProjectBaseline_ZeroSessions(t *testing.T) {
	input := makeBaselineInput(nil, nil, nil)
	_, err := ComputeProjectBaseline(input)
	if err == nil {
		t.Error("expected error for 0 sessions, got nil")
	}
}

func TestComputeProjectBaseline_Basic(t *testing.T) {
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 1_000_000, 100_000, 3, 1),
		makeMeta("s2", "2026-01-02T10:00:00Z", 1_000_000, 100_000, 5, 2),
		makeMeta("s3", "2026-01-03T10:00:00Z", 1_000_000, 100_000, 4, 0),
		makeMeta("s4", "2026-01-04T10:00:00Z", 1_000_000, 100_000, 6, 3),
	}
	facets := []claude.SessionFacet{
		makeFacet("s1", map[string]int{"retry": 2}),
		makeFacet("s2", map[string]int{"retry": 1}),
	}
	sawIDs := map[string]bool{"s1": true, "s3": true}

	input := makeBaselineInput(sessions, facets, sawIDs)
	baseline, err := ComputeProjectBaseline(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if baseline.Project != "testproject" {
		t.Errorf("Project = %q, want %q", baseline.Project, "testproject")
	}
	if baseline.SessionCount != 4 {
		t.Errorf("SessionCount = %d, want 4", baseline.SessionCount)
	}
	if baseline.ComputedAt == "" {
		t.Error("ComputedAt should not be empty")
	}

	// SAW fraction: 2 out of 4 sessions.
	if math.Abs(baseline.SAWSessionFrac-0.5) > 1e-9 {
		t.Errorf("SAWSessionFrac = %f, want 0.5", baseline.SAWSessionFrac)
	}

	// AvgCommits: (3+5+4+6)/4 = 4.5
	if math.Abs(baseline.AvgCommits-4.5) > 1e-9 {
		t.Errorf("AvgCommits = %f, want 4.5", baseline.AvgCommits)
	}

	// Costs are identical for all 4 sessions (same tokens), so stddev should be 0.
	if baseline.StddevCostUSD != 0 {
		t.Errorf("StddevCostUSD = %f, want 0 (identical costs)", baseline.StddevCostUSD)
	}

	// AvgCostUSD should be > 0.
	if baseline.AvgCostUSD <= 0 {
		t.Error("AvgCostUSD should be > 0")
	}

	// Friction: s1 has facet (sum=2), s2 has facet (sum=1), s3 no facet (ToolErrors=0), s4 no facet (ToolErrors=3).
	// AvgFriction = (2+1+0+3)/4 = 1.5
	if math.Abs(baseline.AvgFriction-1.5) > 1e-9 {
		t.Errorf("AvgFriction = %f, want 1.5", baseline.AvgFriction)
	}
}

func TestComputeProjectBaseline_SAWFrac(t *testing.T) {
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-01T10:00:00Z", 1_000_000, 100_000, 3, 0),
		makeMeta("s2", "2026-01-02T10:00:00Z", 1_000_000, 100_000, 3, 0),
		makeMeta("s3", "2026-01-03T10:00:00Z", 1_000_000, 100_000, 3, 0),
	}
	// All SAW.
	sawIDs := map[string]bool{"s1": true, "s2": true, "s3": true}
	input := makeBaselineInput(sessions, nil, sawIDs)
	baseline, err := ComputeProjectBaseline(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(baseline.SAWSessionFrac-1.0) > 1e-9 {
		t.Errorf("SAWSessionFrac = %f, want 1.0", baseline.SAWSessionFrac)
	}
}

// --- DetectAnomalies tests ---

func buildTestBaseline() store.ProjectBaseline {
	return store.ProjectBaseline{
		Project:        "testproject",
		ComputedAt:     "2026-01-01T00:00:00Z",
		SessionCount:   10,
		AvgCostUSD:     1.0,
		StddevCostUSD:  0.1,
		AvgFriction:    2.0,
		StddevFriction: 0.5,
		AvgCommits:     4.0,
		SAWSessionFrac: 0.3,
	}
}

func TestDetectAnomalies_NormalSession(t *testing.T) {
	// Session within normal range — no anomaly.
	// Cost: 1.0 (avg=1.0, stddev=0.1) -> z=0
	// Friction: 2 (avg=2.0, stddev=0.5) -> z=0
	sessions := []claude.SessionMeta{
		// 1M input tokens * 3.0/M = $3.00 — that's actually 3x higher than baseline avg=1.0
		// Let's use tokens that produce cost ~1.0:
		// cost = input/1M * inputPerM + output/1M * outputPerM
		// 1.0 = (100000/1M)*3 + (50000/1M)*15 = 0.3 + 0.75 = 1.05, close enough
		makeMeta("normal", "2026-01-10T10:00:00Z", 100_000, 50_000, 3, 2),
	}
	facets := []claude.SessionFacet{
		makeFacet("normal", map[string]int{"retry": 2}),
	}

	baseline := buildTestBaseline()
	results := DetectAnomalies(sessions, facets, baseline, testPricingAnomaly, NoCacheRatio(), 2.0)

	// z for cost = (1.05 - 1.0)/0.1 = 0.5, not anomalous
	// z for friction = (2 - 2.0)/0.5 = 0
	if len(results) != 0 {
		t.Errorf("expected 0 anomalies for normal session, got %d", len(results))
	}
}

func TestDetectAnomalies_HighCostAnomaly(t *testing.T) {
	// Session with very high cost: z = (5.0 - 1.0) / 0.1 = 40 -> critical
	// Cost ~5.0: input=1M -> cost = 1M/1M*3 + 1M/1M*15 = 18... too high.
	// Let's compute carefully for cost ~5.0:
	// Use input=1_500_000 (1.5M), output=50_000:
	// cost = 1.5*3 + 0.05*15 = 4.5 + 0.75 = 5.25 -> z = (5.25-1.0)/0.1 = 42.5 -> critical
	sessions := []claude.SessionMeta{
		makeMeta("high-cost", "2026-01-10T10:00:00Z", 1_500_000, 50_000, 3, 2),
	}
	facets := []claude.SessionFacet{
		makeFacet("high-cost", map[string]int{"retry": 2}),
	}

	baseline := buildTestBaseline()
	results := DetectAnomalies(sessions, facets, baseline, testPricingAnomaly, NoCacheRatio(), 2.0)

	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if results[0].SessionID != "high-cost" {
		t.Errorf("SessionID = %q, want %q", results[0].SessionID, "high-cost")
	}
	if results[0].Severity != "critical" {
		t.Errorf("Severity = %q, want %q", results[0].Severity, "critical")
	}
	if results[0].CostZScore <= 2.0 {
		t.Errorf("CostZScore = %f, should be > 2.0", results[0].CostZScore)
	}
}

func TestDetectAnomalies_HighFrictionAnomaly(t *testing.T) {
	// Session with high friction: z = (10 - 2.0) / 0.5 = 16 -> critical
	// Cost near baseline: use input=100_000, output=50_000 -> cost ~1.05
	sessions := []claude.SessionMeta{
		makeMeta("high-friction", "2026-01-10T10:00:00Z", 100_000, 50_000, 3, 10),
	}
	// No facet -> friction falls back to ToolErrors=10.

	baseline := buildTestBaseline()
	results := DetectAnomalies(sessions, nil, baseline, testPricingAnomaly, NoCacheRatio(), 2.0)

	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if results[0].SessionID != "high-friction" {
		t.Errorf("SessionID = %q, want %q", results[0].SessionID, "high-friction")
	}
	if results[0].Severity != "critical" {
		t.Errorf("Severity = %q, want %q", results[0].Severity, "critical")
	}
	if results[0].FrictionZScore <= 2.0 {
		t.Errorf("FrictionZScore = %f, should be > 2.0", results[0].FrictionZScore)
	}
	if results[0].Friction != 10 {
		t.Errorf("Friction = %d, want 10", results[0].Friction)
	}
}

func TestDetectAnomalies_WarningThreshold(t *testing.T) {
	// Session with modest friction: z = (3.1 - 2.0) / 0.5 = 2.2 -> warning (not critical)
	// friction = 3.1 is not integer-possible from int ToolErrors, use friction counts.
	// friction = 3 -> z = (3-2.0)/0.5 = 2.0, exactly at threshold = not anomalous for >, need >
	// Use friction=4 -> z = (4-2.0)/0.5 = 4.0 -> critical
	// Use friction=3 with threshold=1.5 -> z = (3-2.0)/0.5 = 2.0 > 1.5 -> warning (not >=3)
	sessions := []claude.SessionMeta{
		makeMeta("warn-sess", "2026-01-10T10:00:00Z", 100_000, 50_000, 3, 3),
	}

	baseline := buildTestBaseline()
	results := DetectAnomalies(sessions, nil, baseline, testPricingAnomaly, NoCacheRatio(), 1.5)

	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if results[0].Severity != "warning" {
		t.Errorf("Severity = %q, want %q", results[0].Severity, "warning")
	}
}

func TestDetectAnomalies_DefaultThreshold(t *testing.T) {
	// threshold <= 0 should default to 2.0.
	sessions := []claude.SessionMeta{
		makeMeta("normal2", "2026-01-10T10:00:00Z", 100_000, 50_000, 3, 2),
	}
	facets := []claude.SessionFacet{
		makeFacet("normal2", map[string]int{"retry": 2}),
	}

	baseline := buildTestBaseline()
	// With threshold=0 -> default 2.0, cost z~0.5, friction z=0 -> no anomaly.
	results := DetectAnomalies(sessions, facets, baseline, testPricingAnomaly, NoCacheRatio(), 0)
	if len(results) != 0 {
		t.Errorf("expected 0 anomalies with default threshold, got %d", len(results))
	}
}

func TestDetectAnomalies_ZeroStddev(t *testing.T) {
	// Baseline with 0 stddev — z-score should be 0 regardless, no anomalies.
	baseline := store.ProjectBaseline{
		Project:        "testproject",
		AvgCostUSD:     1.0,
		StddevCostUSD:  0,
		AvgFriction:    2.0,
		StddevFriction: 0,
	}
	sessions := []claude.SessionMeta{
		makeMeta("s1", "2026-01-10T10:00:00Z", 100_000, 50_000, 3, 100), // high friction but z=0
	}

	results := DetectAnomalies(sessions, nil, baseline, testPricingAnomaly, NoCacheRatio(), 2.0)
	if len(results) != 0 {
		t.Errorf("expected 0 anomalies when stddev=0, got %d", len(results))
	}
}

func TestDetectAnomalies_PerModelCost(t *testing.T) {
	// Session with Opus ModelUsage should be costed at Opus rates.
	// Opus: 1.5M input * $15/M = $22.50, 50K output * $75/M = $3.75 → total $26.25
	// Baseline avg cost = $1.00, stddev = $0.10
	// z = (26.25 - 1.0) / 0.1 = 252.5 → critical anomaly
	sessions := []claude.SessionMeta{
		{
			SessionID:    "opus-session",
			StartTime:    "2026-01-10T10:00:00Z",
			InputTokens:  1_500_000,
			OutputTokens: 50_000,
			GitCommits:   3,
			ToolErrors:   2,
			ModelUsage: map[string]claude.ModelStats{
				"claude-3-opus-20240229": {InputTokens: 1_500_000, OutputTokens: 50_000},
			},
		},
	}
	facets := []claude.SessionFacet{
		makeFacet("opus-session", map[string]int{"retry": 2}),
	}

	baseline := buildTestBaseline()
	results := DetectAnomalies(sessions, facets, baseline, testPricingAnomaly, NoCacheRatio(), 2.0)

	if len(results) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(results))
	}
	if results[0].SessionID != "opus-session" {
		t.Errorf("SessionID = %q, want %q", results[0].SessionID, "opus-session")
	}
	if results[0].Severity != "critical" {
		t.Errorf("Severity = %q, want %q", results[0].Severity, "critical")
	}
	// Verify the cost reflects Opus pricing, not Sonnet.
	// Sonnet would give: 1.5M*3/M + 50K*15/M = 4.50 + 0.75 = 5.25
	// Opus gives: 1.5M*15/M + 50K*75/M = 22.50 + 3.75 = 26.25
	if results[0].CostUSD < 20.0 {
		t.Errorf("CostUSD = %.4f, expected ~26.25 (Opus pricing), got Sonnet-level cost", results[0].CostUSD)
	}
}

func TestDetectAnomalies_EmptySessions(t *testing.T) {
	baseline := buildTestBaseline()
	results := DetectAnomalies(nil, nil, baseline, testPricingAnomaly, NoCacheRatio(), 2.0)
	if len(results) != 0 {
		t.Errorf("expected nil/empty results for empty sessions, got %d", len(results))
	}
}

func TestMean(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		want   float64
	}{
		{"empty", nil, 0},
		{"single", []float64{5}, 5},
		{"multiple", []float64{1, 2, 3, 4}, 2.5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mean(tc.values)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("mean(%v) = %f, want %f", tc.values, got, tc.want)
			}
		})
	}
}

func TestPopulationStddev(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		avg    float64
		want   float64
	}{
		{"empty", nil, 0, 0},
		{"single", []float64{5}, 5, 0},
		{"uniform", []float64{2, 2, 2}, 2, 0},
		// variance = ((1-2)^2 + (2-2)^2 + (3-2)^2) / 3 = 2/3, stddev = sqrt(2/3)
		{"three_values", []float64{1, 2, 3}, 2, math.Sqrt(2.0 / 3.0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := populationStddev(tc.values, tc.avg)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("populationStddev(%v, %f) = %f, want %f", tc.values, tc.avg, got, tc.want)
			}
		})
	}
}

func TestZScore(t *testing.T) {
	tests := []struct {
		value, avg, stddev, want float64
	}{
		{5, 5, 1, 0},
		{7, 5, 1, 2},
		{3, 5, 1, -2},
		{10, 5, 0, 0}, // zero stddev -> 0
	}
	for _, tc := range tests {
		got := zScore(tc.value, tc.avg, tc.stddev)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("zScore(%f, %f, %f) = %f, want %f", tc.value, tc.avg, tc.stddev, got, tc.want)
		}
	}
}
