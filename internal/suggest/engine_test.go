package suggest

import (
	"math"
	"testing"
)

// --- Engine.Run ---

func TestEngineRun_EmptyContext(t *testing.T) {
	engine := NewEngine()
	ctx := &AnalysisContext{}
	suggestions := engine.Run(ctx)
	// With an empty context, most rules should produce nothing.
	// HookGaps triggers on HookCount==0 even with empty context.
	for _, s := range suggestions {
		if s.Title == "" {
			t.Error("got suggestion with empty title")
		}
		if s.Category == "" {
			t.Error("got suggestion with empty category")
		}
	}
}

func TestEngineRun_AllNilFields(t *testing.T) {
	engine := NewEngine()
	ctx := &AnalysisContext{
		Projects:                   nil,
		RecurringFriction:          nil,
		AgentTypeStats:             nil,
		CustomMetricTrends:         nil,
		ClaudeMDSectionCorrelation: nil,
	}
	// Should not panic with nil maps and slices.
	suggestions := engine.Run(ctx)
	_ = suggestions
}

func TestEngineRun_ReturnsSortedByImpactScore(t *testing.T) {
	engine := NewEngine()
	ctx := &AnalysisContext{
		TotalSessions: 20,
		Projects: []ProjectContext{
			{Name: "app1", SessionCount: 10, HasClaudeMD: false},
			{Name: "app2", SessionCount: 50, HasClaudeMD: false, Interruptions: 500},
		},
		RecurringFriction: []string{"wrong_tool"},
	}
	suggestions := engine.Run(ctx)
	if len(suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}
	// Verify descending order of ImpactScore.
	for i := 1; i < len(suggestions); i++ {
		if suggestions[i].ImpactScore > suggestions[i-1].ImpactScore {
			t.Errorf("suggestions not sorted: index %d (%.2f) > index %d (%.2f)",
				i, suggestions[i].ImpactScore, i-1, suggestions[i-1].ImpactScore)
		}
	}
}

func TestEngineRun_ProducesSuggestionsFromMultipleRules(t *testing.T) {
	engine := NewEngine()
	ctx := &AnalysisContext{
		TotalSessions: 20,
		AvgToolErrors: 2.0,
		Projects: []ProjectContext{
			{
				Name:         "buggy",
				SessionCount: 5,
				HasClaudeMD:  false,
				ToolErrors:   50, // avg=10, threshold=4
			},
		},
		RecurringFriction: []string{"timeout"},
	}
	suggestions := engine.Run(ctx)

	categories := make(map[string]bool)
	for _, s := range suggestions {
		categories[s.Category] = true
	}

	// We should see at least "configuration" (MissingClaudeMD, HookGaps),
	// "friction" (RecurringFriction), and "quality" (HighErrorProjects).
	expectedCategories := []string{"configuration", "friction", "quality"}
	for _, cat := range expectedCategories {
		if !categories[cat] {
			t.Errorf("expected category %q in suggestions, got categories: %v", cat, categories)
		}
	}
}

func TestEngineRun_NoRules(t *testing.T) {
	engine := &Engine{rules: nil}
	ctx := &AnalysisContext{TotalSessions: 10}
	suggestions := engine.Run(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions from engine with no rules, got %d", len(suggestions))
	}
}

func TestEngineRun_CustomRule(t *testing.T) {
	customRule := func(ctx *AnalysisContext) []Suggestion {
		return []Suggestion{
			{
				Category:    "custom",
				Priority:    PriorityCritical,
				Title:       "Custom suggestion",
				Description: "This is a custom rule",
				ImpactScore: 100.0,
			},
		}
	}
	engine := &Engine{rules: []Rule{customRule}}
	ctx := &AnalysisContext{}
	suggestions := engine.Run(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Category != "custom" {
		t.Errorf("expected category %q, got %q", "custom", suggestions[0].Category)
	}
}

// --- NewEngine ---

func TestNewEngine_HasAllRules(t *testing.T) {
	engine := NewEngine()
	// NewEngine registers 13 built-in rules.
	expectedCount := 13
	if len(engine.rules) != expectedCount {
		t.Errorf("expected %d rules, got %d", expectedCount, len(engine.rules))
	}
}

// --- RankSuggestions ---

func TestRankSuggestions_SortedDescending(t *testing.T) {
	input := []Suggestion{
		{Title: "low", ImpactScore: 1.0},
		{Title: "high", ImpactScore: 10.0},
		{Title: "mid", ImpactScore: 5.0},
	}
	sorted := RankSuggestions(input)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 suggestions, got %d", len(sorted))
	}
	if sorted[0].Title != "high" {
		t.Errorf("expected first to be %q, got %q", "high", sorted[0].Title)
	}
	if sorted[1].Title != "mid" {
		t.Errorf("expected second to be %q, got %q", "mid", sorted[1].Title)
	}
	if sorted[2].Title != "low" {
		t.Errorf("expected third to be %q, got %q", "low", sorted[2].Title)
	}
}

func TestRankSuggestions_DoesNotMutateInput(t *testing.T) {
	input := []Suggestion{
		{Title: "low", ImpactScore: 1.0},
		{Title: "high", ImpactScore: 10.0},
	}
	_ = RankSuggestions(input)
	// Original order should be preserved.
	if input[0].Title != "low" {
		t.Error("RankSuggestions mutated the input slice")
	}
}

func TestRankSuggestions_EmptySlice(t *testing.T) {
	sorted := RankSuggestions(nil)
	if len(sorted) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(sorted))
	}
}

func TestRankSuggestions_SingleElement(t *testing.T) {
	input := []Suggestion{{Title: "only", ImpactScore: 5.0}}
	sorted := RankSuggestions(input)
	if len(sorted) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(sorted))
	}
	if sorted[0].Title != "only" {
		t.Errorf("expected %q, got %q", "only", sorted[0].Title)
	}
}

func TestRankSuggestions_EqualScores(t *testing.T) {
	input := []Suggestion{
		{Title: "a", ImpactScore: 5.0},
		{Title: "b", ImpactScore: 5.0},
		{Title: "c", ImpactScore: 5.0},
	}
	sorted := RankSuggestions(input)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 suggestions, got %d", len(sorted))
	}
	// All scores should be equal.
	for _, s := range sorted {
		if s.ImpactScore != 5.0 {
			t.Errorf("expected score 5.0, got %f", s.ImpactScore)
		}
	}
}

// --- ComputeImpact ---

func TestComputeImpact_BasicFormula(t *testing.T) {
	// (10 * 0.5 * 3.0) / 5.0 = 3.0
	result := ComputeImpact(10, 0.5, 3.0, 5.0)
	if math.Abs(result-3.0) > 0.001 {
		t.Errorf("expected 3.0, got %f", result)
	}
}

func TestComputeImpact_ZeroEffort(t *testing.T) {
	result := ComputeImpact(10, 0.5, 3.0, 0.0)
	if result != 0 {
		t.Errorf("expected 0 for zero effort, got %f", result)
	}
}

func TestComputeImpact_NegativeEffort(t *testing.T) {
	result := ComputeImpact(10, 0.5, 3.0, -1.0)
	if result != 0 {
		t.Errorf("expected 0 for negative effort, got %f", result)
	}
}

func TestComputeImpact_ZeroSessions(t *testing.T) {
	result := ComputeImpact(0, 0.5, 3.0, 5.0)
	if result != 0 {
		t.Errorf("expected 0 for zero sessions, got %f", result)
	}
}

func TestComputeImpact_ZeroFrequency(t *testing.T) {
	result := ComputeImpact(10, 0.0, 3.0, 5.0)
	if result != 0 {
		t.Errorf("expected 0 for zero frequency, got %f", result)
	}
}

func TestComputeImpact_ZeroTimeSaved(t *testing.T) {
	result := ComputeImpact(10, 0.5, 0.0, 5.0)
	if result != 0 {
		t.Errorf("expected 0 for zero time saved, got %f", result)
	}
}

func TestComputeImpact_LargeValues(t *testing.T) {
	// (1000 * 1.0 * 60.0) / 10.0 = 6000.0
	result := ComputeImpact(1000, 1.0, 60.0, 10.0)
	if math.Abs(result-6000.0) > 0.001 {
		t.Errorf("expected 6000.0, got %f", result)
	}
}

// --- Priority Constants ---

func TestPriorityOrdering(t *testing.T) {
	if PriorityCritical >= PriorityHigh {
		t.Error("PriorityCritical should be numerically less than PriorityHigh")
	}
	if PriorityHigh >= PriorityMedium {
		t.Error("PriorityHigh should be numerically less than PriorityMedium")
	}
	if PriorityMedium >= PriorityLow {
		t.Error("PriorityMedium should be numerically less than PriorityLow")
	}
}

// --- Integration: Engine with full triggering context ---

func TestEngineRun_FullContext(t *testing.T) {
	engine := NewEngine()
	ctx := &AnalysisContext{
		TotalSessions:       30,
		AvgToolErrors:       2.0,
		HookCount:           0,
		CommandCount:        5,
		PluginCount:         2,
		ZeroCommitRate:      0.60,
		CacheSavingsPercent: 5.0,
		TotalCost:           200.0,
		AgentSuccessRate:    0.50,
		RecurringFriction:   []string{"timeout", "wrong_file"},
		AgentTypeStats: map[string]float64{
			"research": 0.40,
		},
		CustomMetricTrends: map[string]string{
			"build_time": "regressing",
		},
		ClaudeMDSectionCorrelation: map[string]float64{
			"testing": 30.0,
		},
		Projects: []ProjectContext{
			{
				Name:                    "main-app",
				SessionCount:            20,
				HasClaudeMD:             true,
				ToolErrors:              100, // avg=5.0, threshold=4.0
				Interruptions:           100, // avg=5.0 > 3.0
				AgentCount:              0,
				SequentialCount:         5,
				ClaudeMDMissingSections: []string{"testing"},
			},
			{
				Name:         "side-project",
				SessionCount: 5,
				HasClaudeMD:  false,
				ToolErrors:   5,
				AgentCount:   0,
			},
		},
	}

	suggestions := engine.Run(ctx)
	if len(suggestions) == 0 {
		t.Fatal("expected multiple suggestions from full context")
	}

	// Verify sorted by impact score descending.
	for i := 1; i < len(suggestions); i++ {
		if suggestions[i].ImpactScore > suggestions[i-1].ImpactScore {
			t.Errorf("not sorted at index %d: %.2f > %.2f",
				i, suggestions[i].ImpactScore, suggestions[i-1].ImpactScore)
		}
	}

	// Verify we got suggestions from many different categories.
	categories := make(map[string]bool)
	for _, s := range suggestions {
		categories[s.Category] = true
	}
	if len(categories) < 4 {
		t.Errorf("expected at least 4 categories, got %d: %v", len(categories), categories)
	}
}

// --- Edge case: HookGaps with zero TotalSessions ---

func TestHookGaps_ZeroSessions(t *testing.T) {
	ctx := &AnalysisContext{
		HookCount:     0,
		TotalSessions: 0,
	}
	suggestions := HookGaps(ctx)
	// HookGaps checks HookCount, not TotalSessions, so it still fires.
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	// But impact score should be 0 because affectedSessions=0.
	if suggestions[0].ImpactScore != 0 {
		t.Errorf("expected impact score 0 with zero sessions, got %f", suggestions[0].ImpactScore)
	}
}
