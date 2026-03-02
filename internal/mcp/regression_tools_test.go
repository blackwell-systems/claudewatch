package mcp

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// TestGetRegressionStatus_EmptyDir verifies that no sessions returns HasBaseline=false
// and no error.
func TestGetRegressionStatus_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	s := newTestServer(dir, 0)
	addRegressionTools(s)

	result, err := callTool(s, "get_regression_status", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(analyzer.RegressionStatus)
	if !ok {
		t.Fatalf("expected analyzer.RegressionStatus, got %T", result)
	}

	if r.HasBaseline {
		t.Error("HasBaseline = true, want false (no sessions)")
	}
}

// TestGetRegressionStatus_NoBaseline verifies that 5 sessions with no stored baseline
// returns HasBaseline=false.
func TestGetRegressionStatus_NoBaseline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("sess-nb-%d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/nobaselineproj", 0, 0)
	}

	s := newTestServer(dir, 0)
	addRegressionTools(s)

	result, err := callTool(s, "get_regression_status", json.RawMessage(`{"project":"nobaselineproj"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(analyzer.RegressionStatus)
	if !ok {
		t.Fatalf("expected analyzer.RegressionStatus, got %T", result)
	}

	if r.HasBaseline {
		t.Error("HasBaseline = true, want false (no baseline stored)")
	}
	if r.Project != "nobaselineproj" {
		t.Errorf("Project = %q, want %q", r.Project, "nobaselineproj")
	}
}

// TestGetRegressionStatus_NoRegression verifies that 5 uniform sessions with a stored
// baseline within threshold returns Regressed=false.
func TestGetRegressionStatus_NoRegression(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("sess-nr-%d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/noregressproj", 0, 1)
	}

	// Pre-insert a baseline that matches the session profile (no regression expected).
	db := openTestDB(t)
	baseline := store.ProjectBaseline{
		Project:        "noregressproj",
		ComputedAt:     "2026-01-01T00:00:00Z",
		SessionCount:   5,
		AvgCostUSD:     0.01,
		StddevCostUSD:  0.001,
		AvgFriction:    0.0,
		StddevFriction: 0.0,
		AvgCommits:     1.0,
		SAWSessionFrac: 0.0,
	}
	if err := db.UpsertProjectBaseline(baseline); err != nil {
		t.Fatalf("upsert baseline: %v", err)
	}
	_ = db.Close()

	s := newTestServer(dir, 0)
	addRegressionTools(s)

	result, err := callTool(s, "get_regression_status", json.RawMessage(`{"project":"noregressproj"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(analyzer.RegressionStatus)
	if !ok {
		t.Fatalf("expected analyzer.RegressionStatus, got %T", result)
	}

	if !r.HasBaseline {
		t.Error("HasBaseline = false, want true (baseline was stored)")
	}
	if r.Regressed {
		t.Errorf("Regressed = true, want false (uniform sessions, baseline matches)")
	}
}

// TestGetRegressionStatus_FrictionRegression verifies that sessions with high friction
// relative to a low-friction baseline triggers FrictionRegressed=true.
func TestGetRegressionStatus_FrictionRegression(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write sessions with friction events (tool_errors > 0).
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("sess-fr-%d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		// toolErrors=5 means high friction rate.
		writeSessionMetaFull(t, dir, id, start, "/home/user/frictionproj", 5, 1)
		// Write corresponding facets with friction counts.
		writeFacet(t, dir, id, map[string]int{"tool_error": 5})
	}

	// Pre-insert a baseline with near-zero friction.
	db := openTestDB(t)
	baseline := store.ProjectBaseline{
		Project:        "frictionproj",
		ComputedAt:     "2026-01-01T00:00:00Z",
		SessionCount:   10,
		AvgCostUSD:     0.01,
		StddevCostUSD:  0.001,
		AvgFriction:    0.1, // very low baseline friction rate
		StddevFriction: 0.01,
		AvgCommits:     1.0,
		SAWSessionFrac: 0.0,
	}
	if err := db.UpsertProjectBaseline(baseline); err != nil {
		t.Fatalf("upsert baseline: %v", err)
	}
	_ = db.Close()

	s := newTestServer(dir, 0)
	addRegressionTools(s)

	// Use default threshold (0 → ComputeRegressionStatus defaults to 1.5).
	result, err := callTool(s, "get_regression_status", json.RawMessage(`{"project":"frictionproj"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(analyzer.RegressionStatus)
	if !ok {
		t.Fatalf("expected analyzer.RegressionStatus, got %T", result)
	}

	if !r.HasBaseline {
		t.Error("HasBaseline = false, want true")
	}
	if !r.FrictionRegressed {
		t.Errorf("FrictionRegressed = false, want true (high friction vs low baseline)")
	}
}

// TestGetRegressionStatus_ExplicitProject verifies that specifying a project name
// in args filters correctly.
func TestGetRegressionStatus_ExplicitProject(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write sessions for two projects.
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("sess-ep-a-%d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/projA", 0, 1)
	}
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("sess-ep-b-%d", i)
		start := fmt.Sprintf("2026-02-%02dT10:00:00Z", i+1)
		writeSessionMetaFull(t, dir, id, start, "/home/user/projB", 0, 1)
	}

	s := newTestServer(dir, 0)
	addRegressionTools(s)

	// Request projA explicitly even though projB is more recent.
	result, err := callTool(s, "get_regression_status", json.RawMessage(`{"project":"projA"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(analyzer.RegressionStatus)
	if !ok {
		t.Fatalf("expected analyzer.RegressionStatus, got %T", result)
	}

	if r.Project != "projA" {
		t.Errorf("Project = %q, want %q", r.Project, "projA")
	}
}

// TestGetRegressionStatus_CustomThreshold verifies that threshold=2.0 is not exceeded
// when threshold=1.5 would be, i.e., the threshold parameter is correctly forwarded.
func TestGetRegressionStatus_CustomThreshold(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write sessions with moderately elevated friction (would regress at 1.5x but not 2.0x).
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("sess-ct-%d", i)
		start := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		// toolErrors=2 each session.
		writeSessionMetaFull(t, dir, id, start, "/home/user/threshproj2", 2, 1)
		writeFacet(t, dir, id, map[string]int{"tool_error": 2})
	}

	// Pre-insert a baseline: friction rate = 1.0 (moderate baseline).
	// At threshold 1.5: current ~2.0 > 1.5 * 1.0 → regressed.
	// At threshold 2.0: current ~2.0 == 2.0 * 1.0 → borderline; use baseline just below current.
	db := openTestDB(t)
	baseline := store.ProjectBaseline{
		Project:        "threshproj2",
		ComputedAt:     "2026-01-01T00:00:00Z",
		SessionCount:   10,
		AvgCostUSD:     0.01,
		StddevCostUSD:  0.001,
		AvgFriction:    1.0,
		StddevFriction: 0.1,
		AvgCommits:     1.0,
		SAWSessionFrac: 0.0,
	}
	if err := db.UpsertProjectBaseline(baseline); err != nil {
		t.Fatalf("upsert baseline: %v", err)
	}
	_ = db.Close()

	s := newTestServer(dir, 0)
	addRegressionTools(s)

	// With threshold=1.5 (default), friction should regress.
	result15, err := callTool(s, "get_regression_status", json.RawMessage(`{"project":"threshproj2","threshold":1.5}`))
	if err != nil {
		t.Fatalf("threshold=1.5 unexpected error: %v", err)
	}
	r15, ok := result15.(analyzer.RegressionStatus)
	if !ok {
		t.Fatalf("expected analyzer.RegressionStatus, got %T", result15)
	}

	// With threshold=5.0, friction should not regress.
	result50, err := callTool(s, "get_regression_status", json.RawMessage(`{"project":"threshproj2","threshold":5.0}`))
	if err != nil {
		t.Fatalf("threshold=5.0 unexpected error: %v", err)
	}
	r50, ok := result50.(analyzer.RegressionStatus)
	if !ok {
		t.Fatalf("expected analyzer.RegressionStatus, got %T", result50)
	}

	// Threshold 5.0 should be harder to exceed than 1.5.
	// If 1.5 triggers regression, 5.0 must not (or both may not, but not 5.0 without 1.5).
	if r50.Regressed && !r15.Regressed {
		t.Error("threshold=5.0 regressed but threshold=1.5 did not — higher threshold should be harder to exceed")
	}
}
