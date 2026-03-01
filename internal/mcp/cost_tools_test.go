package mcp

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// newCostTestServer creates a Server with the cost summary tool registered.
func newCostTestServer(tmpDir string) *Server {
	s := newTestServer(tmpDir, 0)
	addCostTools(s)
	return s
}

// TestGetCostSummary_NoSessions verifies that an empty directory returns a
// zero-value result without error.
func TestGetCostSummary_NoSessions(t *testing.T) {
	dir := t.TempDir()
	s := newCostTestServer(dir)

	result, err := callTool(s, "get_cost_summary", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CostSummaryResult)
	if !ok {
		t.Fatalf("expected CostSummaryResult, got %T", result)
	}

	if r.TodayUSD != 0 {
		t.Errorf("TodayUSD = %f, want 0", r.TodayUSD)
	}
	if r.WeekUSD != 0 {
		t.Errorf("WeekUSD = %f, want 0", r.WeekUSD)
	}
	if r.AllTimeUSD != 0 {
		t.Errorf("AllTimeUSD = %f, want 0", r.AllTimeUSD)
	}
	if len(r.ByProject) != 0 {
		t.Errorf("ByProject len = %d, want 0", len(r.ByProject))
	}
}

// TestGetCostSummary_TodayBucket verifies that a session from today contributes
// to TodayUSD, WeekUSD, and AllTimeUSD.
func TestGetCostSummary_TodayBucket(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().UTC().Format("2006-01-02") + "T12:00:00Z"
	writeSessionMeta(t, dir, "today-sess", today, "/home/user/myproject", 1_000_000, 100_000)

	s := newCostTestServer(dir)

	result, err := callTool(s, "get_cost_summary", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CostSummaryResult)
	if !ok {
		t.Fatalf("expected CostSummaryResult, got %T", result)
	}

	if r.TodayUSD <= 0 {
		t.Errorf("TodayUSD = %f, want > 0", r.TodayUSD)
	}
	if r.WeekUSD <= 0 {
		t.Errorf("WeekUSD = %f, want > 0", r.WeekUSD)
	}
	if r.AllTimeUSD <= 0 {
		t.Errorf("AllTimeUSD = %f, want > 0", r.AllTimeUSD)
	}
	if r.TodayUSD != r.WeekUSD {
		t.Errorf("TodayUSD (%f) != WeekUSD (%f), expected equal for single session today", r.TodayUSD, r.WeekUSD)
	}
	if r.TodayUSD != r.AllTimeUSD {
		t.Errorf("TodayUSD (%f) != AllTimeUSD (%f), expected equal for single session today", r.TodayUSD, r.AllTimeUSD)
	}
}

// TestGetCostSummary_ByProjectSorted verifies that multiple projects are sorted
// descending by TotalUSD.
func TestGetCostSummary_ByProjectSorted(t *testing.T) {
	dir := t.TempDir()

	// Write sessions for two projects with different token counts.
	// projectA gets large tokens so it costs more.
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("projecta-sess-%d", i)
		writeSessionMeta(t, dir, id, "2026-01-15T10:00:00Z", "/home/user/projectA", 2_000_000, 200_000)
	}
	// projectB gets small tokens so it costs less.
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("projectb-sess-%d", i)
		writeSessionMeta(t, dir, id, "2026-01-15T10:00:00Z", "/home/user/projectB", 100_000, 10_000)
	}

	s := newCostTestServer(dir)

	result, err := callTool(s, "get_cost_summary", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CostSummaryResult)
	if !ok {
		t.Fatalf("expected CostSummaryResult, got %T", result)
	}

	if len(r.ByProject) != 2 {
		t.Fatalf("ByProject len = %d, want 2", len(r.ByProject))
	}

	if r.ByProject[0].Project != "projectA" {
		t.Errorf("ByProject[0].Project = %q, want %q", r.ByProject[0].Project, "projectA")
	}
	if r.ByProject[1].Project != "projectB" {
		t.Errorf("ByProject[1].Project = %q, want %q", r.ByProject[1].Project, "projectB")
	}
	if r.ByProject[0].TotalUSD <= r.ByProject[1].TotalUSD {
		t.Errorf("ByProject not sorted descending: [0]=%f, [1]=%f", r.ByProject[0].TotalUSD, r.ByProject[1].TotalUSD)
	}
	if r.ByProject[0].Sessions != 3 {
		t.Errorf("ByProject[0].Sessions = %d, want 3", r.ByProject[0].Sessions)
	}
}

// TestGetCostSummary_NonNilByProject verifies that empty sessions returns
// ByProject as []ProjectSpend{} rather than nil.
func TestGetCostSummary_NonNilByProject(t *testing.T) {
	dir := t.TempDir()
	s := newCostTestServer(dir)

	result, err := callTool(s, "get_cost_summary", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CostSummaryResult)
	if !ok {
		t.Fatalf("expected CostSummaryResult, got %T", result)
	}

	if r.ByProject == nil {
		t.Error("ByProject is nil, want []ProjectSpend{}")
	}
}
