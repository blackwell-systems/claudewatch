package mcp

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// addProjectComparisonTool registers the get_project_comparison tool on the server.
// Registration in tools.go is orchestrator-owned post-merge; this test helper includes
// the min_sessions schema so tests can exercise the filter.
func addProjectComparisonTool(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_project_comparison",
		Description: "Project-level health metrics comparison across all projects, ranked by health score.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"min_sessions":{"type":"integer","description":"Minimum session count for a project to be included (default 0, no filter)."}},"additionalProperties":false}`),
		Handler:     s.handleGetProjectComparison,
	})
}

// almostEqualPT returns true if a and b differ by less than epsilon.
// Named to avoid collision with almostEqual in health_tools_test.go.
func almostEqualPT(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// TestGetProjectComparison_NoProjects verifies that an empty dir returns empty Projects slice without error.
func TestGetProjectComparison_NoProjects(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addProjectComparisonTool(s)

	result, err := callTool(s, "get_project_comparison", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectComparisonResult)
	if !ok {
		t.Fatalf("expected ProjectComparisonResult, got %T", result)
	}

	if r.Projects == nil {
		t.Error("Projects is nil, want []ProjectSummary{}")
	}
	if len(r.Projects) != 0 {
		t.Errorf("len(Projects) = %d, want 0", len(r.Projects))
	}
}

// TestGetProjectComparison_SingleProject verifies that a single project with known data returns correct metrics.
func TestGetProjectComparison_SingleProject(t *testing.T) {
	dir := t.TempDir()

	// Write two sessions for "myproject": one with commits, one without.
	writeSessionMetaFull(t, dir, "sess-1", "2026-01-10T10:00:00Z", "/home/user/myproject", 0, 1)
	writeSessionMetaFull(t, dir, "sess-2", "2026-01-11T10:00:00Z", "/home/user/myproject", 0, 0)

	// Add friction for sess-1 only: FrictionRate = 0.5.
	writeFacet(t, dir, "sess-1", map[string]int{"wrong_approach": 2})

	s := newTestServer(dir, 0)
	addProjectComparisonTool(s)

	result, err := callTool(s, "get_project_comparison", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectComparisonResult)
	if !ok {
		t.Fatalf("expected ProjectComparisonResult, got %T", result)
	}

	if len(r.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(r.Projects))
	}

	p := r.Projects[0]
	if p.Project != "myproject" {
		t.Errorf("Project = %q, want %q", p.Project, "myproject")
	}
	if p.SessionCount != 2 {
		t.Errorf("SessionCount = %d, want 2", p.SessionCount)
	}
	// FrictionRate: 1 out of 2 sessions has friction => 0.5.
	if !almostEqualPT(p.FrictionRate, 0.5, 0.001) {
		t.Errorf("FrictionRate = %f, want 0.5", p.FrictionRate)
	}
	// ZeroCommitRate: 1 out of 2 sessions has 0 commits => 0.5.
	if !almostEqualPT(p.ZeroCommitRate, 0.5, 0.001) {
		t.Errorf("ZeroCommitRate = %f, want 0.5", p.ZeroCommitRate)
	}
	// TopFriction should contain the one friction type.
	if len(p.TopFriction) != 1 || p.TopFriction[0] != "wrong_approach" {
		t.Errorf("TopFriction = %v, want [wrong_approach]", p.TopFriction)
	}
}

// TestGetProjectComparison_RankedByHealthScore verifies two projects are sorted descending by health score.
func TestGetProjectComparison_RankedByHealthScore(t *testing.T) {
	dir := t.TempDir()

	// "goodproject": 2 sessions, commits in both, no friction => high health score.
	writeSessionMetaFull(t, dir, "good-1", "2026-01-10T10:00:00Z", "/home/user/goodproject", 0, 2)
	writeSessionMetaFull(t, dir, "good-2", "2026-01-11T10:00:00Z", "/home/user/goodproject", 0, 1)

	// "badproject": 2 sessions, no commits in either, friction in both => low health score.
	writeSessionMetaFull(t, dir, "bad-1", "2026-01-10T10:00:00Z", "/home/user/badproject", 0, 0)
	writeSessionMetaFull(t, dir, "bad-2", "2026-01-11T10:00:00Z", "/home/user/badproject", 0, 0)
	writeFacet(t, dir, "bad-1", map[string]int{"wrong_approach": 3})
	writeFacet(t, dir, "bad-2", map[string]int{"off_track": 2})

	s := newTestServer(dir, 0)
	addProjectComparisonTool(s)

	result, err := callTool(s, "get_project_comparison", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectComparisonResult)
	if !ok {
		t.Fatalf("expected ProjectComparisonResult, got %T", result)
	}

	if len(r.Projects) != 2 {
		t.Fatalf("len(Projects) = %d, want 2", len(r.Projects))
	}

	// First project should have higher health score.
	if r.Projects[0].HealthScore < r.Projects[1].HealthScore {
		t.Errorf("Projects not sorted descending by HealthScore: [0]=%d, [1]=%d",
			r.Projects[0].HealthScore, r.Projects[1].HealthScore)
	}
	// The first project should be "goodproject".
	if r.Projects[0].Project != "goodproject" {
		t.Errorf("Projects[0].Project = %q, want %q", r.Projects[0].Project, "goodproject")
	}
	// The second project should be "badproject".
	if r.Projects[1].Project != "badproject" {
		t.Errorf("Projects[1].Project = %q, want %q", r.Projects[1].Project, "badproject")
	}
}

// TestGetProjectComparison_HasClaudeMD verifies that a project with CLAUDE.md gets has_claude_md: true and health_score bonus.
func TestGetProjectComparison_HasClaudeMD(t *testing.T) {
	dir := t.TempDir()

	// Create a real project directory with CLAUDE.md.
	projectDir := t.TempDir()
	claudeMDPath := filepath.Join(projectDir, "CLAUDE.md")
	if err := os.WriteFile(claudeMDPath, []byte("# CLAUDE\n"), 0644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	// Write sessions for the project with CLAUDE.md.
	// Give it 50% zero-commit rate so the baseline health score is not 100 (clamped).
	// healthScore = 100 - int(0 * 40) - int(0.5 * 30) + int(0 * 20) + 10 = 100 - 0 - 15 + 0 + 10 = 95
	writeSessionMetaFull(t, dir, "cmd-sess-1", "2026-01-10T10:00:00Z", projectDir, 0, 1)
	writeSessionMetaFull(t, dir, "cmd-sess-2", "2026-01-11T10:00:00Z", projectDir, 0, 0)

	// Write sessions for a project WITHOUT CLAUDE.md with the same 50% zero-commit rate.
	// healthScore = 100 - int(0 * 40) - int(0.5 * 30) + int(0 * 20) + 0 = 100 - 0 - 15 + 0 + 0 = 85
	writeSessionMetaFull(t, dir, "nocmd-sess-1", "2026-01-10T10:00:00Z", "/home/user/nomdproject", 0, 1)
	writeSessionMetaFull(t, dir, "nocmd-sess-2", "2026-01-11T10:00:00Z", "/home/user/nomdproject", 0, 0)

	s := newTestServer(dir, 0)
	addProjectComparisonTool(s)

	result, err := callTool(s, "get_project_comparison", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectComparisonResult)
	if !ok {
		t.Fatalf("expected ProjectComparisonResult, got %T", result)
	}

	if len(r.Projects) != 2 {
		t.Fatalf("len(Projects) = %d, want 2", len(r.Projects))
	}

	// Find the project that has CLAUDE.md (by project name = base of projectDir).
	projectName := filepath.Base(projectDir)
	var withClaude, withoutClaude *ProjectSummary
	for i := range r.Projects {
		if r.Projects[i].Project == projectName {
			withClaude = &r.Projects[i]
		} else {
			withoutClaude = &r.Projects[i]
		}
	}

	if withClaude == nil {
		t.Fatalf("could not find project %q in results", projectName)
	}
	if !withClaude.HasClaudeMD {
		t.Errorf("HasClaudeMD = false, want true for project with CLAUDE.md")
	}
	if withoutClaude != nil && withoutClaude.HasClaudeMD {
		t.Errorf("HasClaudeMD = true, want false for project without CLAUDE.md")
	}

	// Project with CLAUDE.md should have a higher health score (10-point bonus).
	// Expected: withClaude=95, withoutClaude=85.
	if withClaude.HealthScore <= withoutClaude.HealthScore {
		t.Errorf("Project with CLAUDE.md HealthScore=%d should be > project without CLAUDE.md HealthScore=%d",
			withClaude.HealthScore, withoutClaude.HealthScore)
	}
}

// TestGetProjectComparison_MinSessionsFilter verifies that projects with fewer sessions
// than min_sessions are excluded from the result.
func TestGetProjectComparison_MinSessionsFilter(t *testing.T) {
	dir := t.TempDir()

	// "alpha": 1 session — should be filtered out with min_sessions=3.
	writeSessionMetaFull(t, dir, "alpha-1", "2026-01-10T10:00:00Z", "/home/user/alpha", 0, 1)

	// "beta": 3 sessions — should pass filter (exactly meets threshold).
	writeSessionMetaFull(t, dir, "beta-1", "2026-01-10T10:00:00Z", "/home/user/beta", 0, 1)
	writeSessionMetaFull(t, dir, "beta-2", "2026-01-11T10:00:00Z", "/home/user/beta", 0, 1)
	writeSessionMetaFull(t, dir, "beta-3", "2026-01-12T10:00:00Z", "/home/user/beta", 0, 1)

	// "gamma": 5 sessions — should pass filter.
	writeSessionMetaFull(t, dir, "gamma-1", "2026-01-10T10:00:00Z", "/home/user/gamma", 0, 1)
	writeSessionMetaFull(t, dir, "gamma-2", "2026-01-11T10:00:00Z", "/home/user/gamma", 0, 1)
	writeSessionMetaFull(t, dir, "gamma-3", "2026-01-12T10:00:00Z", "/home/user/gamma", 0, 1)
	writeSessionMetaFull(t, dir, "gamma-4", "2026-01-13T10:00:00Z", "/home/user/gamma", 0, 1)
	writeSessionMetaFull(t, dir, "gamma-5", "2026-01-14T10:00:00Z", "/home/user/gamma", 0, 1)

	s := newTestServer(dir, 0)
	addProjectComparisonTool(s)

	result, err := callTool(s, "get_project_comparison", json.RawMessage(`{"min_sessions":3}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectComparisonResult)
	if !ok {
		t.Fatalf("expected ProjectComparisonResult, got %T", result)
	}

	if len(r.Projects) != 2 {
		t.Fatalf("len(Projects) = %d, want 2 (alpha with 1 session should be filtered out)", len(r.Projects))
	}

	for _, p := range r.Projects {
		if p.Project == "alpha" {
			t.Errorf("project %q with 1 session should have been filtered out by min_sessions=3", p.Project)
		}
		if p.SessionCount < 3 {
			t.Errorf("project %q has SessionCount=%d, want >= 3", p.Project, p.SessionCount)
		}
	}
}

// TestGetProjectComparison_MinSessionsZeroNoFilter verifies that min_sessions=0 returns all projects.
func TestGetProjectComparison_MinSessionsZeroNoFilter(t *testing.T) {
	dir := t.TempDir()

	writeSessionMetaFull(t, dir, "p1-sess-1", "2026-01-10T10:00:00Z", "/home/user/project1", 0, 1)
	writeSessionMetaFull(t, dir, "p2-sess-1", "2026-01-10T10:00:00Z", "/home/user/project2", 0, 1)
	writeSessionMetaFull(t, dir, "p2-sess-2", "2026-01-11T10:00:00Z", "/home/user/project2", 0, 1)
	writeSessionMetaFull(t, dir, "p2-sess-3", "2026-01-12T10:00:00Z", "/home/user/project2", 0, 1)
	writeSessionMetaFull(t, dir, "p3-sess-1", "2026-01-10T10:00:00Z", "/home/user/project3", 0, 1)
	writeSessionMetaFull(t, dir, "p3-sess-2", "2026-01-11T10:00:00Z", "/home/user/project3", 0, 1)
	writeSessionMetaFull(t, dir, "p3-sess-3", "2026-01-12T10:00:00Z", "/home/user/project3", 0, 1)
	writeSessionMetaFull(t, dir, "p3-sess-4", "2026-01-13T10:00:00Z", "/home/user/project3", 0, 1)
	writeSessionMetaFull(t, dir, "p3-sess-5", "2026-01-14T10:00:00Z", "/home/user/project3", 0, 1)

	s := newTestServer(dir, 0)
	addProjectComparisonTool(s)

	result, err := callTool(s, "get_project_comparison", json.RawMessage(`{"min_sessions":0}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectComparisonResult)
	if !ok {
		t.Fatalf("expected ProjectComparisonResult, got %T", result)
	}

	if len(r.Projects) != 3 {
		t.Fatalf("len(Projects) = %d, want 3 (min_sessions=0 should not filter anything)", len(r.Projects))
	}
}

// TestGetProjectComparison_MinSessionsDefaultNoFilter verifies that omitting min_sessions returns all projects.
func TestGetProjectComparison_MinSessionsDefaultNoFilter(t *testing.T) {
	dir := t.TempDir()

	writeSessionMetaFull(t, dir, "d1-sess-1", "2026-01-10T10:00:00Z", "/home/user/delta1", 0, 1)
	writeSessionMetaFull(t, dir, "d2-sess-1", "2026-01-10T10:00:00Z", "/home/user/delta2", 0, 1)
	writeSessionMetaFull(t, dir, "d2-sess-2", "2026-01-11T10:00:00Z", "/home/user/delta2", 0, 1)
	writeSessionMetaFull(t, dir, "d2-sess-3", "2026-01-12T10:00:00Z", "/home/user/delta2", 0, 1)
	writeSessionMetaFull(t, dir, "d3-sess-1", "2026-01-10T10:00:00Z", "/home/user/delta3", 0, 1)
	writeSessionMetaFull(t, dir, "d3-sess-2", "2026-01-11T10:00:00Z", "/home/user/delta3", 0, 1)
	writeSessionMetaFull(t, dir, "d3-sess-3", "2026-01-12T10:00:00Z", "/home/user/delta3", 0, 1)
	writeSessionMetaFull(t, dir, "d3-sess-4", "2026-01-13T10:00:00Z", "/home/user/delta3", 0, 1)
	writeSessionMetaFull(t, dir, "d3-sess-5", "2026-01-14T10:00:00Z", "/home/user/delta3", 0, 1)

	s := newTestServer(dir, 0)
	addProjectComparisonTool(s)

	result, err := callTool(s, "get_project_comparison", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectComparisonResult)
	if !ok {
		t.Fatalf("expected ProjectComparisonResult, got %T", result)
	}

	if len(r.Projects) != 3 {
		t.Fatalf("len(Projects) = %d, want 3 (absent min_sessions should not filter anything)", len(r.Projects))
	}
}

// TestGetProjectComparison_FrictionRate verifies 2 sessions, 1 with friction yields friction_rate = 0.5.
func TestGetProjectComparison_FrictionRate(t *testing.T) {
	dir := t.TempDir()

	writeSessionMetaFull(t, dir, "fr-sess-1", "2026-01-10T10:00:00Z", "/home/user/frproject", 0, 1)
	writeSessionMetaFull(t, dir, "fr-sess-2", "2026-01-11T10:00:00Z", "/home/user/frproject", 0, 1)
	// Only fr-sess-1 has friction.
	writeFacet(t, dir, "fr-sess-1", map[string]int{"wrong_approach": 1})

	s := newTestServer(dir, 0)
	addProjectComparisonTool(s)

	result, err := callTool(s, "get_project_comparison", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectComparisonResult)
	if !ok {
		t.Fatalf("expected ProjectComparisonResult, got %T", result)
	}

	if len(r.Projects) != 1 {
		t.Fatalf("len(Projects) = %d, want 1", len(r.Projects))
	}

	p := r.Projects[0]
	if p.Project != "frproject" {
		t.Errorf("Project = %q, want %q", p.Project, "frproject")
	}
	if !almostEqualPT(p.FrictionRate, 0.5, 0.001) {
		t.Errorf("FrictionRate = %f, want 0.5", p.FrictionRate)
	}
}
