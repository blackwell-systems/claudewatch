package suggest

import (
	"strings"
	"testing"
)

// --- MissingClaudeMD ---

func TestMissingClaudeMD_ProjectWithSessionsNoClaudeMD(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "myapp", SessionCount: 5, HasClaudeMD: false},
		},
	}
	suggestions := MissingClaudeMD(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	s := suggestions[0]
	if s.Category != "configuration" {
		t.Errorf("expected category %q, got %q", "configuration", s.Category)
	}
	if s.Priority != PriorityHigh {
		t.Errorf("expected priority %d, got %d", PriorityHigh, s.Priority)
	}
	if !strings.Contains(s.Title, "myapp") {
		t.Errorf("expected title to contain project name, got %q", s.Title)
	}
	if s.ImpactScore <= 0 {
		t.Errorf("expected positive impact score, got %f", s.ImpactScore)
	}
}

func TestMissingClaudeMD_ProjectWithClaudeMD(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "myapp", SessionCount: 5, HasClaudeMD: true},
		},
	}
	suggestions := MissingClaudeMD(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestMissingClaudeMD_ProjectWithZeroSessions(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "unused", SessionCount: 0, HasClaudeMD: false},
		},
	}
	suggestions := MissingClaudeMD(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for zero-session project, got %d", len(suggestions))
	}
}

func TestMissingClaudeMD_MultipleProjects(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "app1", SessionCount: 10, HasClaudeMD: false},
			{Name: "app2", SessionCount: 3, HasClaudeMD: true},
			{Name: "app3", SessionCount: 7, HasClaudeMD: false},
		},
	}
	suggestions := MissingClaudeMD(ctx)
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}
}

func TestMissingClaudeMD_EmptyProjects(t *testing.T) {
	ctx := &AnalysisContext{}
	suggestions := MissingClaudeMD(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

// --- RecurringFriction ---

func TestRecurringFriction_WithFrictionTypes(t *testing.T) {
	ctx := &AnalysisContext{
		RecurringFriction: []string{"wrong_tool", "permission_denied"},
		TotalSessions:     20,
	}
	suggestions := RecurringFriction(ctx)
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}
	for _, s := range suggestions {
		if s.Category != "friction" {
			t.Errorf("expected category %q, got %q", "friction", s.Category)
		}
		if s.Priority != PriorityHigh {
			t.Errorf("expected priority %d, got %d", PriorityHigh, s.Priority)
		}
	}
}

func TestRecurringFriction_NoFriction(t *testing.T) {
	ctx := &AnalysisContext{
		RecurringFriction: nil,
		TotalSessions:     20,
	}
	suggestions := RecurringFriction(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestRecurringFriction_EmptySlice(t *testing.T) {
	ctx := &AnalysisContext{
		RecurringFriction: []string{},
		TotalSessions:     20,
	}
	suggestions := RecurringFriction(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestRecurringFriction_DescriptionIncludesFrictionType(t *testing.T) {
	ctx := &AnalysisContext{
		RecurringFriction: []string{"file_not_found"},
		TotalSessions:     10,
	}
	suggestions := RecurringFriction(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if !strings.Contains(suggestions[0].Title, "file_not_found") {
		t.Errorf("expected title to contain friction type, got %q", suggestions[0].Title)
	}
}

// --- HookGaps ---

func TestHookGaps_ZeroHooks(t *testing.T) {
	ctx := &AnalysisContext{
		HookCount:     0,
		TotalSessions: 10,
	}
	suggestions := HookGaps(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	s := suggestions[0]
	if s.Category != "configuration" {
		t.Errorf("expected category %q, got %q", "configuration", s.Category)
	}
	if s.Priority != PriorityMedium {
		t.Errorf("expected priority %d, got %d", PriorityMedium, s.Priority)
	}
	if !strings.Contains(s.Title, "Configure") {
		t.Errorf("expected title about configuring hooks, got %q", s.Title)
	}
}

func TestHookGaps_FewHooks(t *testing.T) {
	ctx := &AnalysisContext{
		HookCount:     2,
		TotalSessions: 10,
	}
	suggestions := HookGaps(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	s := suggestions[0]
	if s.Priority != PriorityLow {
		t.Errorf("expected priority %d, got %d", PriorityLow, s.Priority)
	}
	if !strings.Contains(s.Title, "Expand") {
		t.Errorf("expected title about expanding hooks, got %q", s.Title)
	}
}

func TestHookGaps_OneHook(t *testing.T) {
	ctx := &AnalysisContext{
		HookCount:     1,
		TotalSessions: 10,
	}
	suggestions := HookGaps(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Priority != PriorityLow {
		t.Errorf("expected PriorityLow, got %d", suggestions[0].Priority)
	}
}

func TestHookGaps_ThreeOrMoreHooks(t *testing.T) {
	ctx := &AnalysisContext{
		HookCount:     3,
		TotalSessions: 10,
	}
	suggestions := HookGaps(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for 3+ hooks, got %d", len(suggestions))
	}
}

func TestHookGaps_ManyHooks(t *testing.T) {
	ctx := &AnalysisContext{
		HookCount:     10,
		TotalSessions: 10,
	}
	suggestions := HookGaps(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

// --- UnusedSkills ---

func TestUnusedSkills_LowAgentRatio(t *testing.T) {
	ctx := &AnalysisContext{
		CommandCount:  5,
		TotalSessions: 20,
		Projects: []ProjectContext{
			{AgentCount: 0},
			{AgentCount: 1},
		},
	}
	suggestions := UnusedSkills(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Category != "adoption" {
		t.Errorf("expected category %q, got %q", "adoption", suggestions[0].Category)
	}
}

func TestUnusedSkills_HighAgentRatio(t *testing.T) {
	ctx := &AnalysisContext{
		CommandCount:  5,
		TotalSessions: 10,
		Projects: []ProjectContext{
			{AgentCount: 5},
			{AgentCount: 3},
		},
	}
	suggestions := UnusedSkills(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions when agent ratio is high, got %d", len(suggestions))
	}
}

func TestUnusedSkills_NoCommands(t *testing.T) {
	ctx := &AnalysisContext{
		CommandCount:  0,
		TotalSessions: 20,
		Projects: []ProjectContext{
			{AgentCount: 0},
		},
	}
	suggestions := UnusedSkills(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions when no commands defined, got %d", len(suggestions))
	}
}

func TestUnusedSkills_TooFewSessions(t *testing.T) {
	ctx := &AnalysisContext{
		CommandCount:  5,
		TotalSessions: 3,
		Projects: []ProjectContext{
			{AgentCount: 0},
		},
	}
	suggestions := UnusedSkills(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions when too few sessions, got %d", len(suggestions))
	}
}

func TestUnusedSkills_ExactlyFiveSessions(t *testing.T) {
	ctx := &AnalysisContext{
		CommandCount:  3,
		TotalSessions: 5,
		Projects: []ProjectContext{
			{AgentCount: 0},
		},
	}
	// TotalSessions must be > 5, so 5 should not trigger
	suggestions := UnusedSkills(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions at exactly 5 sessions, got %d", len(suggestions))
	}
}

func TestUnusedSkills_BoundaryAgentRatio(t *testing.T) {
	// agentRatio = 1/10 = 0.1, which is NOT < 0.1, so no suggestion
	ctx := &AnalysisContext{
		CommandCount:  3,
		TotalSessions: 10,
		Projects: []ProjectContext{
			{AgentCount: 1},
		},
	}
	suggestions := UnusedSkills(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions at boundary agent ratio 0.1, got %d", len(suggestions))
	}
}

// --- HighErrorProjects ---

func TestHighErrorProjects_HighErrors(t *testing.T) {
	ctx := &AnalysisContext{
		AvgToolErrors: 2.0,
		Projects: []ProjectContext{
			{Name: "buggy", SessionCount: 5, ToolErrors: 30}, // avg=6.0, threshold=4.0
		},
	}
	suggestions := HighErrorProjects(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Category != "quality" {
		t.Errorf("expected category %q, got %q", "quality", suggestions[0].Category)
	}
	if !strings.Contains(suggestions[0].Title, "buggy") {
		t.Errorf("expected title to contain project name, got %q", suggestions[0].Title)
	}
}

func TestHighErrorProjects_NormalErrors(t *testing.T) {
	ctx := &AnalysisContext{
		AvgToolErrors: 5.0,
		Projects: []ProjectContext{
			{Name: "fine", SessionCount: 10, ToolErrors: 50}, // avg=5.0, threshold=10.0
		},
	}
	suggestions := HighErrorProjects(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestHighErrorProjects_ZeroAvgToolErrors(t *testing.T) {
	ctx := &AnalysisContext{
		AvgToolErrors: 0,
		Projects: []ProjectContext{
			{Name: "project", SessionCount: 10, ToolErrors: 100},
		},
	}
	suggestions := HighErrorProjects(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions when avg tool errors is 0, got %d", len(suggestions))
	}
}

func TestHighErrorProjects_NegativeAvgToolErrors(t *testing.T) {
	ctx := &AnalysisContext{
		AvgToolErrors: -1.0,
		Projects: []ProjectContext{
			{Name: "project", SessionCount: 10, ToolErrors: 100},
		},
	}
	suggestions := HighErrorProjects(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions when avg tool errors is negative, got %d", len(suggestions))
	}
}

func TestHighErrorProjects_ZeroSessionProject(t *testing.T) {
	ctx := &AnalysisContext{
		AvgToolErrors: 2.0,
		Projects: []ProjectContext{
			{Name: "empty", SessionCount: 0, ToolErrors: 0},
		},
	}
	suggestions := HighErrorProjects(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for zero-session project, got %d", len(suggestions))
	}
}

func TestHighErrorProjects_ExactlyAtThreshold(t *testing.T) {
	// avg = 4.0/1 = 4.0, threshold = 2.0*2 = 4.0, NOT > threshold
	ctx := &AnalysisContext{
		AvgToolErrors: 2.0,
		Projects: []ProjectContext{
			{Name: "borderline", SessionCount: 1, ToolErrors: 4},
		},
	}
	suggestions := HighErrorProjects(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions at exact threshold, got %d", len(suggestions))
	}
}

// --- AgentAdoption ---

func TestAgentAdoption_LowUsage(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 20,
		Projects: []ProjectContext{
			{AgentCount: 0},
			{AgentCount: 1},
		},
	}
	suggestions := AgentAdoption(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Category != "adoption" {
		t.Errorf("expected category %q, got %q", "adoption", suggestions[0].Category)
	}
	if suggestions[0].Priority != PriorityMedium {
		t.Errorf("expected priority %d, got %d", PriorityMedium, suggestions[0].Priority)
	}
}

func TestAgentAdoption_HighUsage(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 10,
		Projects: []ProjectContext{
			{AgentCount: 5},
			{AgentCount: 5},
		},
	}
	suggestions := AgentAdoption(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for high agent usage, got %d", len(suggestions))
	}
}

func TestAgentAdoption_TooFewSessions(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 4,
		Projects: []ProjectContext{
			{AgentCount: 0},
		},
	}
	suggestions := AgentAdoption(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions when TotalSessions < 5, got %d", len(suggestions))
	}
}

func TestAgentAdoption_ExactlyFiveSessions(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 5,
		Projects: []ProjectContext{
			{AgentCount: 0},
		},
	}
	suggestions := AgentAdoption(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion at exactly 5 sessions, got %d", len(suggestions))
	}
}

// --- InterruptionPattern ---

func TestInterruptionPattern_HighInterruptions(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "messy", SessionCount: 5, Interruptions: 20}, // avg=4.0 > 3.0
		},
	}
	suggestions := InterruptionPattern(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Category != "friction" {
		t.Errorf("expected category %q, got %q", "friction", suggestions[0].Category)
	}
	if !strings.Contains(suggestions[0].Title, "messy") {
		t.Errorf("expected title to contain project name, got %q", suggestions[0].Title)
	}
}

func TestInterruptionPattern_LowInterruptions(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "smooth", SessionCount: 10, Interruptions: 20}, // avg=2.0 <= 3.0
		},
	}
	suggestions := InterruptionPattern(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestInterruptionPattern_ExactlyAtThreshold(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "border", SessionCount: 10, Interruptions: 30}, // avg=3.0, NOT > 3.0
		},
	}
	suggestions := InterruptionPattern(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions at exact threshold, got %d", len(suggestions))
	}
}

func TestInterruptionPattern_ZeroSessions(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "empty", SessionCount: 0, Interruptions: 0},
		},
	}
	suggestions := InterruptionPattern(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for zero-session project, got %d", len(suggestions))
	}
}

func TestInterruptionPattern_MultipleProjects(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "bad", SessionCount: 2, Interruptions: 10},  // avg=5.0
			{Name: "good", SessionCount: 10, Interruptions: 5}, // avg=0.5
			{Name: "also_bad", SessionCount: 3, Interruptions: 15}, // avg=5.0
		},
	}
	suggestions := InterruptionPattern(ctx)
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}
}

// --- AgentTypeEffectiveness ---

func TestAgentTypeEffectiveness_LowSuccessRate(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 10,
		AgentTypeStats: map[string]float64{
			"research": 0.50,
		},
	}
	suggestions := AgentTypeEffectiveness(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	s := suggestions[0]
	if s.Category != "agents" {
		t.Errorf("expected category %q, got %q", "agents", s.Category)
	}
	if !strings.Contains(s.Title, "research") {
		t.Errorf("expected title to contain agent type, got %q", s.Title)
	}
}

func TestAgentTypeEffectiveness_HighSuccessRate(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 10,
		AgentTypeStats: map[string]float64{
			"coding": 0.90,
		},
	}
	suggestions := AgentTypeEffectiveness(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for high success rate, got %d", len(suggestions))
	}
}

func TestAgentTypeEffectiveness_ExactlyAtThreshold(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 10,
		AgentTypeStats: map[string]float64{
			"testing": 0.70, // NOT < 0.70
		},
	}
	suggestions := AgentTypeEffectiveness(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions at exactly 0.70, got %d", len(suggestions))
	}
}

func TestAgentTypeEffectiveness_MultipleTypes(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 10,
		AgentTypeStats: map[string]float64{
			"research": 0.40,
			"coding":   0.90,
			"testing":  0.55,
		},
	}
	suggestions := AgentTypeEffectiveness(ctx)
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}
}

func TestAgentTypeEffectiveness_NilMap(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions:  10,
		AgentTypeStats: nil,
	}
	suggestions := AgentTypeEffectiveness(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for nil map, got %d", len(suggestions))
	}
}

func TestAgentTypeEffectiveness_EmptyMap(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions:  10,
		AgentTypeStats: map[string]float64{},
	}
	suggestions := AgentTypeEffectiveness(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for empty map, got %d", len(suggestions))
	}
}

// --- ParallelizationOpportunity ---

func TestParallelizationOpportunity_HighSequentialCount(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "serial", SessionCount: 5, SequentialCount: 5},
		},
	}
	suggestions := ParallelizationOpportunity(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Category != "agents" {
		t.Errorf("expected category %q, got %q", "agents", suggestions[0].Category)
	}
	if !strings.Contains(suggestions[0].Title, "serial") {
		t.Errorf("expected title to contain project name, got %q", suggestions[0].Title)
	}
}

func TestParallelizationOpportunity_LowSequentialCount(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "fine", SessionCount: 5, SequentialCount: 2},
		},
	}
	suggestions := ParallelizationOpportunity(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for SequentialCount <= 2, got %d", len(suggestions))
	}
}

func TestParallelizationOpportunity_ExactlyTwo(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "borderline", SessionCount: 5, SequentialCount: 2},
		},
	}
	suggestions := ParallelizationOpportunity(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions at exactly 2, got %d", len(suggestions))
	}
}

func TestParallelizationOpportunity_ExactlyThree(t *testing.T) {
	ctx := &AnalysisContext{
		Projects: []ProjectContext{
			{Name: "threshold", SessionCount: 5, SequentialCount: 3},
		},
	}
	suggestions := ParallelizationOpportunity(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion at exactly 3, got %d", len(suggestions))
	}
}

func TestParallelizationOpportunity_NoProjects(t *testing.T) {
	ctx := &AnalysisContext{}
	suggestions := ParallelizationOpportunity(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

// --- CustomMetricRegression ---

func TestCustomMetricRegression_RegressingMetric(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 10,
		CustomMetricTrends: map[string]string{
			"build_time": "regressing",
		},
	}
	suggestions := CustomMetricRegression(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Category != "custom_metrics" {
		t.Errorf("expected category %q, got %q", "custom_metrics", suggestions[0].Category)
	}
	if !strings.Contains(suggestions[0].Title, "build_time") {
		t.Errorf("expected title to contain metric name, got %q", suggestions[0].Title)
	}
}

func TestCustomMetricRegression_ImprovingMetric(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 10,
		CustomMetricTrends: map[string]string{
			"test_coverage": "improving",
		},
	}
	suggestions := CustomMetricRegression(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for improving metric, got %d", len(suggestions))
	}
}

func TestCustomMetricRegression_StableMetric(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 10,
		CustomMetricTrends: map[string]string{
			"latency": "stable",
		},
	}
	suggestions := CustomMetricRegression(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for stable metric, got %d", len(suggestions))
	}
}

func TestCustomMetricRegression_MixedTrends(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions: 10,
		CustomMetricTrends: map[string]string{
			"latency":       "regressing",
			"test_coverage": "improving",
			"build_time":    "regressing",
			"uptime":        "stable",
		},
	}
	suggestions := CustomMetricRegression(ctx)
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}
}

func TestCustomMetricRegression_NilMap(t *testing.T) {
	ctx := &AnalysisContext{
		TotalSessions:     10,
		CustomMetricTrends: nil,
	}
	suggestions := CustomMetricRegression(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for nil map, got %d", len(suggestions))
	}
}

// --- ClaudeMDSectionSuggestions ---

func TestClaudeMDSectionSuggestions_MissingSection(t *testing.T) {
	ctx := &AnalysisContext{
		ClaudeMDSectionCorrelation: map[string]float64{
			"testing": 25.0,
		},
		Projects: []ProjectContext{
			{
				Name:                    "myapp",
				HasClaudeMD:             true,
				SessionCount:            10,
				ClaudeMDMissingSections: []string{"testing"},
			},
		},
	}
	suggestions := ClaudeMDSectionSuggestions(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Category != "quality" {
		t.Errorf("expected category %q, got %q", "quality", suggestions[0].Category)
	}
	if !strings.Contains(suggestions[0].Title, "testing") {
		t.Errorf("expected title to contain section name, got %q", suggestions[0].Title)
	}
	if !strings.Contains(suggestions[0].Title, "myapp") {
		t.Errorf("expected title to contain project name, got %q", suggestions[0].Title)
	}
}

func TestClaudeMDSectionSuggestions_NoMissingSections(t *testing.T) {
	ctx := &AnalysisContext{
		ClaudeMDSectionCorrelation: map[string]float64{
			"testing": 25.0,
		},
		Projects: []ProjectContext{
			{
				Name:                    "myapp",
				HasClaudeMD:             true,
				SessionCount:            10,
				ClaudeMDMissingSections: []string{},
			},
		},
	}
	suggestions := ClaudeMDSectionSuggestions(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions, got %d", len(suggestions))
	}
}

func TestClaudeMDSectionSuggestions_NoClaudeMD(t *testing.T) {
	ctx := &AnalysisContext{
		ClaudeMDSectionCorrelation: map[string]float64{
			"testing": 25.0,
		},
		Projects: []ProjectContext{
			{
				Name:                    "myapp",
				HasClaudeMD:             false,
				SessionCount:            10,
				ClaudeMDMissingSections: []string{"testing"},
			},
		},
	}
	suggestions := ClaudeMDSectionSuggestions(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for project without CLAUDE.md, got %d", len(suggestions))
	}
}

func TestClaudeMDSectionSuggestions_ZeroOrNegativeReduction(t *testing.T) {
	ctx := &AnalysisContext{
		ClaudeMDSectionCorrelation: map[string]float64{
			"testing": 0.0,
			"linting": -5.0,
		},
		Projects: []ProjectContext{
			{
				Name:                    "myapp",
				HasClaudeMD:             true,
				SessionCount:            10,
				ClaudeMDMissingSections: []string{"testing", "linting"},
			},
		},
	}
	suggestions := ClaudeMDSectionSuggestions(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for zero/negative reduction, got %d", len(suggestions))
	}
}

func TestClaudeMDSectionSuggestions_NilCorrelationMap(t *testing.T) {
	ctx := &AnalysisContext{
		ClaudeMDSectionCorrelation: nil,
		Projects: []ProjectContext{
			{
				Name:                    "myapp",
				HasClaudeMD:             true,
				SessionCount:            10,
				ClaudeMDMissingSections: []string{"testing"},
			},
		},
	}
	suggestions := ClaudeMDSectionSuggestions(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for nil correlation map, got %d", len(suggestions))
	}
}

func TestClaudeMDSectionSuggestions_MultipleProjectsMultipleSections(t *testing.T) {
	ctx := &AnalysisContext{
		ClaudeMDSectionCorrelation: map[string]float64{
			"testing":    25.0,
			"formatting": 15.0,
		},
		Projects: []ProjectContext{
			{
				Name:                    "app1",
				HasClaudeMD:             true,
				SessionCount:            10,
				ClaudeMDMissingSections: []string{"testing", "formatting"},
			},
			{
				Name:                    "app2",
				HasClaudeMD:             true,
				SessionCount:            5,
				ClaudeMDMissingSections: []string{"testing"},
			},
		},
	}
	suggestions := ClaudeMDSectionSuggestions(ctx)
	// app1 missing testing + formatting = 2, app2 missing testing = 1 => 3
	if len(suggestions) != 3 {
		t.Fatalf("expected 3 suggestions, got %d", len(suggestions))
	}
}

// --- ZeroCommitRateSuggestion ---

func TestZeroCommitRateSuggestion_HighRate(t *testing.T) {
	ctx := &AnalysisContext{
		ZeroCommitRate: 0.60,
		TotalSessions:  10,
	}
	suggestions := ZeroCommitRateSuggestion(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Category != "quality" {
		t.Errorf("expected category %q, got %q", "quality", suggestions[0].Category)
	}
	if suggestions[0].Priority != PriorityHigh {
		t.Errorf("expected priority %d, got %d", PriorityHigh, suggestions[0].Priority)
	}
}

func TestZeroCommitRateSuggestion_LowRate(t *testing.T) {
	ctx := &AnalysisContext{
		ZeroCommitRate: 0.20,
		TotalSessions:  10,
	}
	suggestions := ZeroCommitRateSuggestion(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for low zero-commit rate, got %d", len(suggestions))
	}
}

func TestZeroCommitRateSuggestion_ExactlyAtThreshold(t *testing.T) {
	ctx := &AnalysisContext{
		ZeroCommitRate: 0.40,
		TotalSessions:  10,
	}
	suggestions := ZeroCommitRateSuggestion(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions at exactly 0.40, got %d", len(suggestions))
	}
}

func TestZeroCommitRateSuggestion_TooFewSessions(t *testing.T) {
	ctx := &AnalysisContext{
		ZeroCommitRate: 0.80,
		TotalSessions:  4,
	}
	suggestions := ZeroCommitRateSuggestion(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions when TotalSessions < 5, got %d", len(suggestions))
	}
}

func TestZeroCommitRateSuggestion_ExactlyFiveSessions(t *testing.T) {
	ctx := &AnalysisContext{
		ZeroCommitRate: 0.80,
		TotalSessions:  5,
	}
	suggestions := ZeroCommitRateSuggestion(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion at exactly 5 sessions, got %d", len(suggestions))
	}
}

// --- CostOptimizationSuggestion ---

func TestCostOptimizationSuggestion_LowCacheSavings(t *testing.T) {
	ctx := &AnalysisContext{
		CacheSavingsPercent: 5.0,
		TotalCost:           100.0,
		TotalSessions:       10,
	}
	suggestions := CostOptimizationSuggestion(ctx)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	if suggestions[0].Category != "configuration" {
		t.Errorf("expected category %q, got %q", "configuration", suggestions[0].Category)
	}
	if !strings.Contains(suggestions[0].Description, "100.00") {
		t.Errorf("expected description to contain total cost, got %q", suggestions[0].Description)
	}
}

func TestCostOptimizationSuggestion_HighCacheSavings(t *testing.T) {
	ctx := &AnalysisContext{
		CacheSavingsPercent: 25.0,
		TotalCost:           100.0,
		TotalSessions:       10,
	}
	suggestions := CostOptimizationSuggestion(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for high cache savings, got %d", len(suggestions))
	}
}

func TestCostOptimizationSuggestion_ExactlyAtThreshold(t *testing.T) {
	ctx := &AnalysisContext{
		CacheSavingsPercent: 20.0,
		TotalCost:           100.0,
		TotalSessions:       10,
	}
	suggestions := CostOptimizationSuggestion(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions at exactly 20%%, got %d", len(suggestions))
	}
}

func TestCostOptimizationSuggestion_ZeroCost(t *testing.T) {
	ctx := &AnalysisContext{
		CacheSavingsPercent: 5.0,
		TotalCost:           0.0,
		TotalSessions:       10,
	}
	suggestions := CostOptimizationSuggestion(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions when TotalCost is 0, got %d", len(suggestions))
	}
}

func TestCostOptimizationSuggestion_NegativeCost(t *testing.T) {
	ctx := &AnalysisContext{
		CacheSavingsPercent: 5.0,
		TotalCost:           -10.0,
		TotalSessions:       10,
	}
	suggestions := CostOptimizationSuggestion(ctx)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions when TotalCost is negative, got %d", len(suggestions))
	}
}
