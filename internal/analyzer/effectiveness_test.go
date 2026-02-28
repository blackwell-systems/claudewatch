package analyzer

import (
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

func TestAnalyzeEffectiveness_InsufficientData(t *testing.T) {
	result := AnalyzeEffectiveness("/proj", time.Time{}, nil, nil, testPricing, NoCacheRatio())
	if result.Verdict != "insufficient_data" {
		t.Errorf("expected insufficient_data, got %q", result.Verdict)
	}
}

func TestAnalyzeEffectiveness_TooFewSessions(t *testing.T) {
	changeTime := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	sessions := []claude.SessionMeta{
		{SessionID: "s1", StartTime: "2026-01-10T10:00:00Z"},
		{SessionID: "s2", StartTime: "2026-01-20T10:00:00Z"},
	}

	result := AnalyzeEffectiveness("/proj", changeTime, sessions, nil, testPricing, NoCacheRatio())
	if result.Verdict != "insufficient_data" {
		t.Errorf("expected insufficient_data (1 per side), got %q", result.Verdict)
	}
}

func TestAnalyzeEffectiveness_FrictionImproved(t *testing.T) {
	changeTime := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	sessions := []claude.SessionMeta{
		// Before: high friction
		{SessionID: "s1", StartTime: "2026-01-10T10:00:00Z", ToolErrors: 5, UserInterruptions: 3},
		{SessionID: "s2", StartTime: "2026-01-11T10:00:00Z", ToolErrors: 4, UserInterruptions: 2},
		{SessionID: "s3", StartTime: "2026-01-12T10:00:00Z", ToolErrors: 6, UserInterruptions: 4},
		// After: low friction
		{SessionID: "s4", StartTime: "2026-01-20T10:00:00Z", ToolErrors: 1, UserInterruptions: 0},
		{SessionID: "s5", StartTime: "2026-01-21T10:00:00Z", ToolErrors: 0, UserInterruptions: 1},
		{SessionID: "s6", StartTime: "2026-01-22T10:00:00Z", ToolErrors: 1, UserInterruptions: 0},
	}

	facets := []claude.SessionFacet{
		{SessionID: "s1", FrictionCounts: map[string]int{"wrong_approach": 3}},
		{SessionID: "s2", FrictionCounts: map[string]int{"wrong_approach": 2}},
		{SessionID: "s3", FrictionCounts: map[string]int{"wrong_approach": 4}},
		{SessionID: "s4", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s5", FrictionCounts: map[string]int{}},
		{SessionID: "s6", FrictionCounts: map[string]int{"wrong_approach": 1}},
	}

	result := AnalyzeEffectiveness("/proj", changeTime, sessions, facets, testPricing, NoCacheRatio())

	if result.BeforeSessions != 3 {
		t.Errorf("expected 3 before sessions, got %d", result.BeforeSessions)
	}
	if result.AfterSessions != 3 {
		t.Errorf("expected 3 after sessions, got %d", result.AfterSessions)
	}

	// Friction should have decreased.
	if result.FrictionDelta >= 0 {
		t.Errorf("expected negative friction delta, got %.2f", result.FrictionDelta)
	}

	// Tool errors should have decreased.
	if result.ToolErrorDelta >= 0 {
		t.Errorf("expected negative tool error delta, got %.2f", result.ToolErrorDelta)
	}

	if result.Score <= 0 {
		t.Errorf("expected positive score, got %d", result.Score)
	}
	if result.Verdict != "effective" {
		t.Errorf("expected 'effective', got %q", result.Verdict)
	}
}

func TestAnalyzeEffectiveness_Regression(t *testing.T) {
	changeTime := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	sessions := []claude.SessionMeta{
		// Before: low friction
		{SessionID: "s1", StartTime: "2026-01-10T10:00:00Z", ToolErrors: 1},
		{SessionID: "s2", StartTime: "2026-01-11T10:00:00Z", ToolErrors: 0},
		// After: high friction
		{SessionID: "s3", StartTime: "2026-01-20T10:00:00Z", ToolErrors: 8},
		{SessionID: "s4", StartTime: "2026-01-21T10:00:00Z", ToolErrors: 6},
	}

	facets := []claude.SessionFacet{
		{SessionID: "s1", FrictionCounts: map[string]int{"wrong_approach": 1}},
		{SessionID: "s2", FrictionCounts: map[string]int{}},
		{SessionID: "s3", FrictionCounts: map[string]int{"wrong_approach": 5}},
		{SessionID: "s4", FrictionCounts: map[string]int{"wrong_approach": 4}},
	}

	result := AnalyzeEffectiveness("/proj", changeTime, sessions, facets, testPricing, NoCacheRatio())

	if result.Score >= 0 {
		t.Errorf("expected negative score for regression, got %d", result.Score)
	}
	if result.Verdict != "regression" {
		t.Errorf("expected 'regression', got %q", result.Verdict)
	}
}

func TestEffectivenessTimeline_MultipleProjects(t *testing.T) {
	changes := []ClaudeMDChange{
		{ProjectPath: "/proj/a", ModifiedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
		{ProjectPath: "/proj/b", ModifiedAt: time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)},
	}

	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/proj/a", StartTime: "2026-01-10T10:00:00Z", ToolErrors: 5},
		{SessionID: "s2", ProjectPath: "/proj/a", StartTime: "2026-01-11T10:00:00Z", ToolErrors: 4},
		{SessionID: "s3", ProjectPath: "/proj/a", StartTime: "2026-01-20T10:00:00Z", ToolErrors: 1},
		{SessionID: "s4", ProjectPath: "/proj/a", StartTime: "2026-01-21T10:00:00Z", ToolErrors: 0},
	}

	results := EffectivenessTimeline(changes, sessions, nil, testPricing, NoCacheRatio())

	// Only /proj/a has sessions, /proj/b should be skipped.
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ProjectName != "a" {
		t.Errorf("expected project 'a', got %q", results[0].ProjectName)
	}
}

func TestComputeEffectivenessScore_AllZeroBefore(t *testing.T) {
	r := EffectivenessResult{
		BeforeFrictionRate:  0,
		AfterFrictionRate:   2.0,
		BeforeToolErrors:    0,
		AfterToolErrors:     3.0,
		BeforeInterruptions: 0,
		AfterInterruptions:  1.0,
		BeforeGoalRate:      0,
		AfterGoalRate:       0.8,
		BeforeCostPerCommit: 0,
		AfterCostPerCommit:  5.0,
	}

	score, verdict := computeEffectivenessScore(r)

	// With all zero baselines, no percentage changes can be computed.
	// Score should be 0 (neutral).
	if score != 0 {
		t.Errorf("expected 0 score with zero baselines, got %d", score)
	}
	if verdict != "neutral" {
		t.Errorf("expected 'neutral', got %q", verdict)
	}
}
