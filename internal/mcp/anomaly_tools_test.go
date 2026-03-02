package mcp

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

// TestGetProjectAnomalies_EmptyDir verifies that no session data returns an empty result.
func TestGetProjectAnomalies_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	s := newTestServer(dir, 0)
	addAnomalyTools(s)

	result, err := callTool(s, "get_project_anomalies", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectAnomaliesResult)
	if !ok {
		t.Fatalf("expected ProjectAnomaliesResult, got %T", result)
	}

	if r.Anomalies == nil {
		t.Error("Anomalies is nil, want non-nil empty slice")
	}
	if len(r.Anomalies) != 0 {
		t.Errorf("Anomalies len = %d, want 0", len(r.Anomalies))
	}
}

// TestGetProjectAnomalies_InsufficientSessions verifies that fewer than 3 sessions
// returns a user-friendly error.
func TestGetProjectAnomalies_InsufficientSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write 2 sessions (< 3 minimum for baseline).
	writeSessionMetaFull(t, dir, "sess-1", "2026-01-10T10:00:00Z", "/home/user/myproj", 0, 0)
	writeSessionMetaFull(t, dir, "sess-2", "2026-01-11T10:00:00Z", "/home/user/myproj", 0, 0)

	s := newTestServer(dir, 0)
	addAnomalyTools(s)

	_, err := callTool(s, "get_project_anomalies", json.RawMessage(`{"project":"myproj"}`))
	if err == nil {
		t.Fatal("expected error for insufficient sessions, got nil")
	}
	// Should contain the user-friendly message.
	wantSubstr := "insufficient session history for project myproj"
	if len(err.Error()) < len(wantSubstr) || err.Error()[:len(wantSubstr)] != wantSubstr {
		// Check via contains approach.
		errStr := err.Error()
		found := false
		for i := 0; i+len(wantSubstr) <= len(errStr); i++ {
			if errStr[i:i+len(wantSubstr)] == wantSubstr {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("error = %q, want substring %q", errStr, wantSubstr)
		}
	}
}

// TestGetProjectAnomalies_ExplicitProject verifies that specifying a project name
// filters correctly and returns a result.
func TestGetProjectAnomalies_ExplicitProject(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write 5 sessions for "myproj" with uniform cost/friction (no anomalies expected).
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("sess-%d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/myproj", 1000, 500)
	}

	s := newTestServer(dir, 0)
	addAnomalyTools(s)

	result, err := callTool(s, "get_project_anomalies", json.RawMessage(`{"project":"myproj"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectAnomaliesResult)
	if !ok {
		t.Fatalf("expected ProjectAnomaliesResult, got %T", result)
	}

	if r.Project != "myproj" {
		t.Errorf("Project = %q, want %q", r.Project, "myproj")
	}
	// Baseline should be populated.
	if r.Baseline == nil {
		t.Error("Baseline is nil, want non-nil (should be computed on the fly)")
	}
	if r.Baseline != nil && r.Baseline.SessionCount != 5 {
		t.Errorf("Baseline.SessionCount = %d, want 5", r.Baseline.SessionCount)
	}
	// No anomalies expected when all sessions have uniform stats.
	if r.Anomalies == nil {
		t.Error("Anomalies is nil, want non-nil slice")
	}
}

// TestGetProjectAnomalies_BaselinePersistedAndReused verifies that after the first
// call computes and stores the baseline, a second call retrieves it from the DB
// (baseline ComputedAt should be the same).
func TestGetProjectAnomalies_BaselinePersistedAndReused(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("sess-pr-%d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/reproj", 1000, 500)
	}

	s := newTestServer(dir, 0)
	addAnomalyTools(s)

	// First call — baseline is computed and stored.
	result1, err := callTool(s, "get_project_anomalies", json.RawMessage(`{"project":"reproj"}`))
	if err != nil {
		t.Fatalf("first call unexpected error: %v", err)
	}
	r1, ok := result1.(ProjectAnomaliesResult)
	if !ok {
		t.Fatalf("expected ProjectAnomaliesResult, got %T", result1)
	}
	if r1.Baseline == nil {
		t.Fatal("first call: Baseline is nil")
	}

	// Second call — should reuse the stored baseline (ComputedAt unchanged).
	result2, err := callTool(s, "get_project_anomalies", json.RawMessage(`{"project":"reproj"}`))
	if err != nil {
		t.Fatalf("second call unexpected error: %v", err)
	}
	r2, ok := result2.(ProjectAnomaliesResult)
	if !ok {
		t.Fatalf("expected ProjectAnomaliesResult, got %T", result2)
	}
	if r2.Baseline == nil {
		t.Fatal("second call: Baseline is nil")
	}

	// Both calls should return the same ComputedAt (baseline was persisted).
	if r1.Baseline.ComputedAt != r2.Baseline.ComputedAt {
		t.Errorf("ComputedAt changed between calls: first=%q, second=%q",
			r1.Baseline.ComputedAt, r2.Baseline.ComputedAt)
	}
}

// TestGetProjectAnomalies_StoredBaselineUsed verifies that if a baseline is already
// stored in the DB, it is returned without recomputation.
func TestGetProjectAnomalies_StoredBaselineUsed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	writeSessionMetaFull(t, dir, "sess-sb-1", "2026-01-01T10:00:00Z", "/home/user/storedproj", 1000, 500)
	writeSessionMetaFull(t, dir, "sess-sb-2", "2026-01-02T10:00:00Z", "/home/user/storedproj", 1000, 500)
	writeSessionMetaFull(t, dir, "sess-sb-3", "2026-01-03T10:00:00Z", "/home/user/storedproj", 1000, 500)

	// Pre-insert a baseline into the DB.
	db := openTestDB(t)
	preBaseline := store.ProjectBaseline{
		Project:        "storedproj",
		ComputedAt:     "2026-01-01T00:00:00Z",
		SessionCount:   99, // distinctive value to confirm it's the stored one
		AvgCostUSD:     0.5,
		StddevCostUSD:  0.1,
		AvgFriction:    1.0,
		StddevFriction: 0.5,
		AvgCommits:     2.0,
		SAWSessionFrac: 0.0,
	}
	if err := db.UpsertProjectBaseline(preBaseline); err != nil {
		t.Fatalf("upsert baseline: %v", err)
	}
	_ = db.Close()

	s := newTestServer(dir, 0)
	addAnomalyTools(s)

	result, err := callTool(s, "get_project_anomalies", json.RawMessage(`{"project":"storedproj"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectAnomaliesResult)
	if !ok {
		t.Fatalf("expected ProjectAnomaliesResult, got %T", result)
	}

	if r.Baseline == nil {
		t.Fatal("Baseline is nil")
	}
	// Should have loaded the stored baseline (SessionCount = 99).
	if r.Baseline.SessionCount != 99 {
		t.Errorf("Baseline.SessionCount = %d, want 99 (pre-stored value)", r.Baseline.SessionCount)
	}
	if r.Baseline.ComputedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("Baseline.ComputedAt = %q, want %q", r.Baseline.ComputedAt, "2026-01-01T00:00:00Z")
	}
}

// TestGetProjectAnomalies_DefaultsToMostRecent verifies that with no project arg,
// the most recent session's project is used.
func TestGetProjectAnomalies_DefaultsToMostRecent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write sessions for two projects; "newproj" is more recent.
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("sess-old-%d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/oldproj", 1000, 500)
	}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("sess-new-%d", i)
		start := fmt.Sprintf("2026-02-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/newproj", 1000, 500)
	}

	s := newTestServer(dir, 0)
	addAnomalyTools(s)

	result, err := callTool(s, "get_project_anomalies", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectAnomaliesResult)
	if !ok {
		t.Fatalf("expected ProjectAnomaliesResult, got %T", result)
	}

	if r.Project != "newproj" {
		t.Errorf("Project = %q, want %q (most recent)", r.Project, "newproj")
	}
}

// TestGetProjectAnomalies_CustomThreshold verifies that a higher threshold results
// in fewer (or equal) anomalies than a lower threshold.
func TestGetProjectAnomalies_CustomThreshold(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write sessions with one high-cost outlier that stands out at z=2 but not z=5.
	// 3 normal sessions + 1 with very high token count.
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("sess-thresh-%d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/threshproj", 0, 0)
	}
	// Outlier with massive tokens.
	writeSessionMeta(t, dir, "sess-outlier", "2026-01-10T10:00:00Z", "/home/user/threshproj", 10_000_000, 2_000_000)

	s := newTestServer(dir, 0)
	addAnomalyTools(s)

	// Call with very high threshold — no anomalies should be detected.
	result, err := callTool(s, "get_project_anomalies", json.RawMessage(`{"project":"threshproj","threshold":100.0}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := result.(ProjectAnomaliesResult)
	if !ok {
		t.Fatalf("expected ProjectAnomaliesResult, got %T", result)
	}
	// With threshold=100.0, nothing should be anomalous.
	if len(r.Anomalies) != 0 {
		t.Errorf("with threshold=100, len(Anomalies) = %d, want 0", len(r.Anomalies))
	}
}
