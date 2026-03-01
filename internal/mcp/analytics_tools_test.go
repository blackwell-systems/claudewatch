package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newAnalyticsTestServer creates a Server with both standard and analytics tools registered.
func newAnalyticsTestServer(tmpDir string) *Server {
	s := newTestServer(tmpDir, 0)
	addAnalyticsTools(s)
	return s
}

// agentTranscriptLines returns two JSONL lines representing a completed agent Task span.
func agentTranscriptLines(sessionID, toolUseID, agentType, launchedAt, completedAt string, isError bool) []string {
	errStr := "false"
	if isError {
		errStr = "true"
	}
	assistantLine := `{"type":"assistant","timestamp":"` + launchedAt + `","sessionId":"` + sessionID + `","message":{"role":"assistant","content":[{"type":"tool_use","id":"` + toolUseID + `","name":"Task","input":{"subagent_type":"` + agentType + `","description":"agent task","prompt":"do work","run_in_background":false}}]}}`
	userLine := `{"type":"user","timestamp":"` + completedAt + `","sessionId":"` + sessionID + `","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + toolUseID + `","content":"result","is_error":` + errStr + `}]}}`
	return []string{assistantLine, userLine}
}

// TestGetAgentPerformance_NoAgents verifies that no transcript data returns a zero-value result.
func TestGetAgentPerformance_NoAgents(t *testing.T) {
	dir := t.TempDir()
	s := newAnalyticsTestServer(dir)

	result, err := callTool(s, "get_agent_performance", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(AgentPerformanceResult)
	if !ok {
		t.Fatalf("expected AgentPerformanceResult, got %T", result)
	}

	if r.TotalAgents != 0 {
		t.Errorf("TotalAgents = %d, want 0", r.TotalAgents)
	}
	if r.SuccessRate != 0 {
		t.Errorf("SuccessRate = %f, want 0", r.SuccessRate)
	}
	if r.ByType == nil {
		t.Error("ByType is nil, want non-nil map")
	}
	if len(r.ByType) != 0 {
		t.Errorf("len(ByType) = %d, want 0", len(r.ByType))
	}
}

// TestGetAgentPerformance_SuccessRate verifies that 3 agents: 2 completed, 1 failed → SuccessRate ~0.667.
func TestGetAgentPerformance_SuccessRate(t *testing.T) {
	dir := t.TempDir()

	// 2 completed agents.
	lines1 := agentTranscriptLines("sess-1", "tu-1", "coder", "2026-01-15T10:00:00Z", "2026-01-15T10:05:00Z", false)
	lines2 := agentTranscriptLines("sess-1", "tu-2", "coder", "2026-01-15T10:06:00Z", "2026-01-15T10:10:00Z", false)
	// 1 failed agent.
	lines3 := agentTranscriptLines("sess-1", "tu-3", "coder", "2026-01-15T10:11:00Z", "2026-01-15T10:12:00Z", true)

	allLines := append(lines1, lines2...)
	allLines = append(allLines, lines3...)
	writeTranscriptJSONL(t, dir, "proj-hash-1", "sess-1", allLines)

	s := newAnalyticsTestServer(dir)
	result, err := callTool(s, "get_agent_performance", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(AgentPerformanceResult)
	if !ok {
		t.Fatalf("expected AgentPerformanceResult, got %T", result)
	}

	if r.TotalAgents != 3 {
		t.Errorf("TotalAgents = %d, want 3", r.TotalAgents)
	}

	// 2 out of 3 completed → SuccessRate ≈ 0.667
	wantSuccessRate := 2.0 / 3.0
	epsilon := 0.001
	if r.SuccessRate < wantSuccessRate-epsilon || r.SuccessRate > wantSuccessRate+epsilon {
		t.Errorf("SuccessRate = %f, want ~%f", r.SuccessRate, wantSuccessRate)
	}
}

// TestGetAgentPerformance_ByType verifies agents of different types produce correct ByType map.
func TestGetAgentPerformance_ByType(t *testing.T) {
	dir := t.TempDir()

	// 2 coder agents (1 success, 1 fail), 1 builder agent (success).
	coderSuccess := agentTranscriptLines("sess-bt", "tu-c1", "coder", "2026-01-15T10:00:00Z", "2026-01-15T10:05:00Z", false)
	coderFail := agentTranscriptLines("sess-bt", "tu-c2", "coder", "2026-01-15T10:06:00Z", "2026-01-15T10:07:00Z", true)
	builderSuccess := agentTranscriptLines("sess-bt", "tu-b1", "builder", "2026-01-15T10:08:00Z", "2026-01-15T10:12:00Z", false)

	allLines := append(coderSuccess, coderFail...)
	allLines = append(allLines, builderSuccess...)
	writeTranscriptJSONL(t, dir, "proj-hash-bt", "sess-bt", allLines)

	s := newAnalyticsTestServer(dir)
	result, err := callTool(s, "get_agent_performance", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(AgentPerformanceResult)
	if !ok {
		t.Fatalf("expected AgentPerformanceResult, got %T", result)
	}

	if r.TotalAgents != 3 {
		t.Errorf("TotalAgents = %d, want 3", r.TotalAgents)
	}

	coderStats, ok := r.ByType["coder"]
	if !ok {
		t.Fatal("ByType missing 'coder' key")
	}
	if coderStats.Count != 2 {
		t.Errorf("coder Count = %d, want 2", coderStats.Count)
	}
	epsilon := 0.001
	if coderStats.SuccessRate < 0.5-epsilon || coderStats.SuccessRate > 0.5+epsilon {
		t.Errorf("coder SuccessRate = %f, want 0.5", coderStats.SuccessRate)
	}

	builderStats, ok := r.ByType["builder"]
	if !ok {
		t.Fatal("ByType missing 'builder' key")
	}
	if builderStats.Count != 1 {
		t.Errorf("builder Count = %d, want 1", builderStats.Count)
	}
	if builderStats.SuccessRate < 1.0-epsilon {
		t.Errorf("builder SuccessRate = %f, want 1.0", builderStats.SuccessRate)
	}
}

// TestGetEffectiveness_NoProjects verifies that no session data returns empty Projects list.
func TestGetEffectiveness_NoProjects(t *testing.T) {
	dir := t.TempDir()
	s := newAnalyticsTestServer(dir)

	result, err := callTool(s, "get_effectiveness", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(AllEffectivenessResult)
	if !ok {
		t.Fatalf("expected AllEffectivenessResult, got %T", result)
	}

	if r.Projects == nil {
		t.Error("Projects is nil, want non-nil empty slice")
	}
	if len(r.Projects) != 0 {
		t.Errorf("len(Projects) = %d, want 0", len(r.Projects))
	}
}

// TestGetEffectiveness_NoCLAUDEMD verifies that a project without CLAUDE.md is excluded.
func TestGetEffectiveness_NoCLAUDEMD(t *testing.T) {
	dir := t.TempDir()

	// Create a project directory without CLAUDE.md.
	projectPath := filepath.Join(dir, "myproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	// Write sessions referencing this project.
	writeSessionMeta(t, dir, "no-claude-md-sess", "2026-01-10T10:00:00Z", projectPath, 1000, 500)

	s := newAnalyticsTestServer(dir)
	result, err := callTool(s, "get_effectiveness", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(AllEffectivenessResult)
	if !ok {
		t.Fatalf("expected AllEffectivenessResult, got %T", result)
	}

	// Project without CLAUDE.md should be excluded.
	if len(r.Projects) != 0 {
		t.Errorf("len(Projects) = %d, want 0 (project has no CLAUDE.md)", len(r.Projects))
	}
}

// TestGetEffectiveness_ReturnsVerdict verifies that a project with sessions before/after
// CLAUDE.md modification is included in results with a non-empty verdict.
func TestGetEffectiveness_ReturnsVerdict(t *testing.T) {
	dir := t.TempDir()

	// Create a project directory with CLAUDE.md.
	projectPath := filepath.Join(dir, "effectiveproject")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	// Write CLAUDE.md with a modification time in the middle of the session range.
	claudeMDPath := filepath.Join(projectPath, "CLAUDE.md")
	if err := os.WriteFile(claudeMDPath, []byte("# Project guidelines\n"), 0644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	// Set CLAUDE.md mod time to a fixed past time so we can place sessions before and after it.
	changeTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(claudeMDPath, changeTime, changeTime); err != nil {
		t.Fatalf("chtimes CLAUDE.md: %v", err)
	}

	// Write sessions before the CLAUDE.md change (need at least 2 for valid analysis).
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("before-sess-%d", i)
		startTime := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+1)
		writeSessionMeta(t, dir, id, startTime, projectPath, 1000, 500)
	}

	// Write sessions after the CLAUDE.md change (need at least 2 for valid analysis).
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("after-sess-%d", i)
		startTime := fmt.Sprintf("2026-01-%02dT10:00:00Z", i+20)
		writeSessionMeta(t, dir, id, startTime, projectPath, 1000, 500)
	}

	s := newAnalyticsTestServer(dir)
	result, err := callTool(s, "get_effectiveness", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(AllEffectivenessResult)
	if !ok {
		t.Fatalf("expected AllEffectivenessResult, got %T", result)
	}

	if len(r.Projects) == 0 {
		t.Fatal("expected at least 1 project entry, got 0")
	}

	entry := r.Projects[0]
	if entry.Verdict == "" {
		t.Error("Verdict is empty, want non-empty string")
	}
	if entry.ChangeDetectedAt == "" {
		t.Error("ChangeDetectedAt is empty, want RFC3339 string")
	}
	if entry.BeforeSessions == 0 && entry.AfterSessions == 0 {
		t.Error("both BeforeSessions and AfterSessions are 0, want at least one > 0")
	}
	if entry.Project == "" {
		t.Error("Project name is empty")
	}
}
