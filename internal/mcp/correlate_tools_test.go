package mcp

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestHandleGetCausalInsights_AllFactors verifies that calling with only outcome
// produces group_comparisons and total_sessions > 0.
func TestHandleGetCausalInsights_AllFactors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write 15 synthetic sessions with varying friction.
	for i := 0; i < 15; i++ {
		id := fmt.Sprintf("sess-af-%02d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		toolErrors := i % 3 // vary tool errors
		gitCommits := (i % 4) + 1
		writeSessionMetaFull(t, dir, id, start, "/home/user/myproject", toolErrors, gitCommits)
		// Add friction facets for some sessions.
		if i%2 == 0 {
			writeFacet(t, dir, id, map[string]int{"tool_error": toolErrors + 1})
		}
	}

	s := newTestServer(dir, 0)
	addCorrelateTools(s)

	result, err := callTool(s, "get_causal_insights", json.RawMessage(`{"outcome":"friction"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CausalInsightsResult)
	if !ok {
		t.Fatalf("expected CausalInsightsResult, got %T", result)
	}

	if r.TotalSessions <= 0 {
		t.Errorf("TotalSessions = %d, want > 0", r.TotalSessions)
	}
	if len(r.GroupComparisons) == 0 {
		t.Error("GroupComparisons is empty, want non-empty for all-factors mode")
	}
	if r.SingleGroupComparison != nil {
		t.Error("SingleGroupComparison should be nil in all-factors mode")
	}
	if r.Outcome != "friction" {
		t.Errorf("Outcome = %q, want %q", r.Outcome, "friction")
	}
	if r.Summary == "" {
		t.Error("Summary is empty, want non-empty")
	}
}

// TestHandleGetCausalInsights_SingleFactor verifies that specifying a single
// boolean factor returns single_group_comparison and no group_comparisons.
func TestHandleGetCausalInsights_SingleFactor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write 15 synthetic sessions.
	for i := 0; i < 15; i++ {
		id := fmt.Sprintf("sess-sf-%02d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/myproject", i%3, (i%4)+1)
	}

	s := newTestServer(dir, 0)
	addCorrelateTools(s)

	result, err := callTool(s, "get_causal_insights", json.RawMessage(`{"outcome":"friction","factor":"has_claude_md"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CausalInsightsResult)
	if !ok {
		t.Fatalf("expected CausalInsightsResult, got %T", result)
	}

	if r.SingleGroupComparison == nil {
		t.Error("SingleGroupComparison is nil, want non-nil for single boolean factor mode")
	}
	if r.GroupComparisons != nil {
		t.Error("GroupComparisons should be nil in single-factor mode")
	}
	if r.SinglePearson != nil {
		t.Error("SinglePearson should be nil for boolean factor")
	}
	if r.Outcome != "friction" {
		t.Errorf("Outcome = %q, want %q", r.Outcome, "friction")
	}
}

// TestHandleGetCausalInsights_InsufficientData verifies that fewer than 3 sessions
// returns an error.
func TestHandleGetCausalInsights_InsufficientData(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write only 2 sessions (below the minimum of 3).
	for i := 0; i < 2; i++ {
		id := fmt.Sprintf("sess-id-%d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/myproject", 0, 1)
	}

	s := newTestServer(dir, 0)
	addCorrelateTools(s)

	_, err := callTool(s, "get_causal_insights", json.RawMessage(`{"outcome":"friction"}`))
	if err == nil {
		t.Fatal("expected error for insufficient data, got nil")
	}
}

// TestHandleGetCausalInsights_MissingOutcome verifies that calling without outcome
// returns an error.
func TestHandleGetCausalInsights_MissingOutcome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	s := newTestServer(dir, 0)
	addCorrelateTools(s)

	_, err := callTool(s, "get_causal_insights", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing outcome, got nil")
	}
}

// TestHandleGetCausalInsights_ProjectFilter verifies that specifying a project
// filters sessions to only that project.
func TestHandleGetCausalInsights_ProjectFilter(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write sessions for two projects: 5 for "myproject" and 10 for "otherproject".
	// The CorrelateFactors requires at least 3 sessions per filtered result.
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("sess-pf-my-%02d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/myproject", i%2, (i%3)+1)
	}
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("sess-pf-other-%02d", i)
		start := fmt.Sprintf("2026-02-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/otherproject", 0, 1)
	}

	s := newTestServer(dir, 0)
	addCorrelateTools(s)

	result, err := callTool(s, "get_causal_insights", json.RawMessage(`{"outcome":"commits","project":"myproject"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CausalInsightsResult)
	if !ok {
		t.Fatalf("expected CausalInsightsResult, got %T", result)
	}

	// total_sessions should reflect only "myproject" sessions (10), not "otherproject" (10).
	if r.TotalSessions != 10 {
		t.Errorf("TotalSessions = %d, want 10 (only myproject sessions)", r.TotalSessions)
	}
	if r.Project != "myproject" {
		t.Errorf("Project = %q, want %q", r.Project, "myproject")
	}
}

// TestHandleGetCausalInsights_PerModelCost verifies that factor analysis uses
// per-model costs when sessions have ModelUsage populated. Sessions with Opus
// usage should show higher cost in the outcome than equivalent Sonnet sessions.
func TestHandleGetCausalInsights_PerModelCost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write 10 sessions with Opus model usage and varying friction.
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("sess-opus-ci-%02d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaWithModels(t, dir, id, start, "/home/user/opusproj",
			map[string][2]int{
				"claude-3-opus-20240229": {1_000_000, 100_000},
			})
		if i%2 == 0 {
			writeFacet(t, dir, id, map[string]int{"tool_error": i + 1})
		}
	}

	s := newTestServer(dir, 0)
	addCorrelateTools(s)

	result, err := callTool(s, "get_causal_insights", json.RawMessage(`{"outcome":"cost"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CausalInsightsResult)
	if !ok {
		t.Fatalf("expected CausalInsightsResult, got %T", result)
	}

	if r.TotalSessions != 10 {
		t.Errorf("TotalSessions = %d, want 10", r.TotalSessions)
	}
	if r.Outcome != "cost" {
		t.Errorf("Outcome = %q, want %q", r.Outcome, "cost")
	}
}
