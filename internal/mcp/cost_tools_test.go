package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// writeLiveSessionJSONL writes a JSONL file that looks like a live Claude Code
// session transcript under {claudeHome}/projects/{hashDir}/{sessionID}.jsonl.
// It sets a fresh mtime so FindActiveSessionPath's mtime fallback picks it up.
// Returns the full path of the written file.
func writeLiveSessionJSONL(t *testing.T, claudeHome, hashDir, sessionID string, inputTokens, outputTokens int) string {
	t.Helper()
	dir := filepath.Join(claudeHome, "projects", hashDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir projects hashdir: %v", err)
	}

	msgJSON, err := json.Marshal(map[string]any{
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	})
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}

	todayTS := time.Now().UTC().Format("2006-01-02") + "T12:00:00Z"
	entries := []map[string]any{
		{"type": "user", "timestamp": todayTS, "sessionId": sessionID},
		{"type": "assistant", "timestamp": todayTS, "sessionId": sessionID, "message": json.RawMessage(msgJSON)},
	}

	path := filepath.Join(dir, sessionID+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create jsonl: %v", err)
	}
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			_ = f.Close()
			t.Fatalf("encode entry: %v", err)
		}
	}
	_ = f.Close()

	// Ensure mtime is fresh (within FindActiveSessionPath's 5-minute window).
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return path
}

// TestGetCostSummary_LiveSessionIncluded verifies that a live (in-progress)
// session that is NOT in the indexed sessions contributes to TodayUSD and
// AllTimeUSD.
func TestGetCostSummary_LiveSessionIncluded(t *testing.T) {
	dir := t.TempDir()

	// No indexed sessions; only a live session.
	writeLiveSessionJSONL(t, dir, "livehash", "live-session-001", 1_000_000, 100_000)

	s := newCostTestServer(dir)
	result, err := callTool(s, "get_cost_summary", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CostSummaryResult)
	if !ok {
		t.Fatalf("expected CostSummaryResult, got %T", result)
	}

	if r.AllTimeUSD <= 0 {
		t.Errorf("AllTimeUSD = %f, want > 0 (live session cost should be included)", r.AllTimeUSD)
	}
	if r.TodayUSD <= 0 {
		t.Errorf("TodayUSD = %f, want > 0 (live session started today)", r.TodayUSD)
	}
	if r.TodayUSD != r.AllTimeUSD {
		t.Errorf("TodayUSD (%f) != AllTimeUSD (%f), expected equal for single live session today",
			r.TodayUSD, r.AllTimeUSD)
	}
}

// TestGetCostSummary_LiveSessionNoDuplicate verifies that a session that appears
// in both the indexed session-meta AND as the active JSONL is counted only once.
func TestGetCostSummary_LiveSessionNoDuplicate(t *testing.T) {
	dir := t.TempDir()

	// Write the same session to both indexed store and as a live JSONL.
	sessionID := "dup-session-001"
	today := time.Now().UTC().Format("2006-01-02") + "T12:00:00Z"
	writeSessionMeta(t, dir, sessionID, today, "/home/user/myproject", 1_000_000, 100_000)
	writeLiveSessionJSONL(t, dir, "duphash", sessionID, 1_000_000, 100_000)

	s := newCostTestServer(dir)
	result, err := callTool(s, "get_cost_summary", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CostSummaryResult)
	if !ok {
		t.Fatalf("expected CostSummaryResult, got %T", result)
	}

	// Compute the expected cost for one session (not doubled).
	singleToday := time.Now().UTC().Format("2006-01-02") + "T12:00:00Z"
	_ = singleToday

	// AllTimeUSD should equal TodayUSD (single session) and neither should be doubled.
	if r.AllTimeUSD != r.TodayUSD {
		t.Errorf("AllTimeUSD (%f) != TodayUSD (%f), expected equal for single session (no duplicates)",
			r.AllTimeUSD, r.TodayUSD)
	}
	if r.AllTimeUSD <= 0 {
		t.Errorf("AllTimeUSD = %f, want > 0", r.AllTimeUSD)
	}
	// Ensure cost is not doubled: there should be exactly one session's worth of cost.
	// We can verify by checking ByProject session count — must be 1, not 2.
	if len(r.ByProject) == 0 {
		t.Fatal("expected ByProject to have entries")
	}
	totalSessions := 0
	for _, p := range r.ByProject {
		totalSessions += p.Sessions
	}
	if totalSessions != 1 {
		t.Errorf("total sessions across ByProject = %d, want 1 (no double-counting)", totalSessions)
	}
}

// TestGetCostSummary_LiveSessionByProject verifies that a live session's cost
// appears in the ByProject aggregate under the correct project name.
func TestGetCostSummary_LiveSessionByProject(t *testing.T) {
	dir := t.TempDir()

	// Live session under "liveproject" hash directory; no indexed sessions.
	writeLiveSessionJSONL(t, dir, "liveproject", "live-session-002", 500_000, 50_000)

	s := newCostTestServer(dir)
	result, err := callTool(s, "get_cost_summary", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CostSummaryResult)
	if !ok {
		t.Fatalf("expected CostSummaryResult, got %T", result)
	}

	if len(r.ByProject) == 0 {
		t.Fatal("expected ByProject to have entries for live session")
	}

	found := false
	for _, p := range r.ByProject {
		if p.Project == "liveproject" {
			found = true
			if p.TotalUSD <= 0 {
				t.Errorf("ByProject[liveproject].TotalUSD = %f, want > 0", p.TotalUSD)
			}
			if p.Sessions != 1 {
				t.Errorf("ByProject[liveproject].Sessions = %d, want 1", p.Sessions)
			}
		}
	}
	if !found {
		t.Errorf("ByProject does not contain \"liveproject\"; got: %v", r.ByProject)
	}
}
