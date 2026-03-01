package mcp

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// writeSessionMetaFull writes a session-meta JSON file with all relevant fields.
func writeSessionMetaFull(t *testing.T, dir, id, startTime, projectPath string, toolErrors, gitCommits int) {
	t.Helper()
	metaDir := filepath.Join(dir, "usage-data", "session-meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatalf("mkdir session-meta: %v", err)
	}
	data := fmt.Sprintf(`{
		"session_id": %q,
		"project_path": %q,
		"start_time": %q,
		"duration_minutes": 30,
		"input_tokens": 1000,
		"output_tokens": 500,
		"tool_errors": %d,
		"git_commits": %d
	}`, id, projectPath, startTime, toolErrors, gitCommits)
	if err := os.WriteFile(filepath.Join(metaDir, id+".json"), []byte(data), 0644); err != nil {
		t.Fatalf("write session meta: %v", err)
	}
}

// writeAgentTaskTranscript writes a SAW-like transcript file so ParseAgentTasks can
// pick up agent data. Each taskSpec is {sessionID, toolUseID, agentType, status}.
// status "completed" => is_error:false, success; "failed" => is_error:true.
func writeAgentTaskTranscript(t *testing.T, dir, projectHash string, tasks []struct {
	SessionID string
	ToolUseID string
	AgentType string
	Status    string
}) {
	t.Helper()
	if len(tasks) == 0 {
		return
	}

	// Group tasks by sessionID to write per-session transcript files.
	bySession := make(map[string][]struct {
		SessionID string
		ToolUseID string
		AgentType string
		Status    string
	})
	for _, task := range tasks {
		bySession[task.SessionID] = append(bySession[task.SessionID], task)
	}

	for sessionID, sessionTasks := range bySession {
		projDir := filepath.Join(dir, "projects", projectHash)
		if err := os.MkdirAll(projDir, 0755); err != nil {
			t.Fatalf("mkdir projects/%s: %v", projectHash, err)
		}

		var lines string
		for _, task := range sessionTasks {
			isError := "false"
			if task.Status == "failed" {
				isError = "true"
			}
			// Write an assistant line (tool_use) followed by user line (tool_result).
			assistantLine := fmt.Sprintf(`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","sessionId":%q,"message":{"role":"assistant","content":[{"type":"tool_use","id":%q,"name":"Task","input":{"subagent_type":%q,"description":"task description","prompt":"do work"}}]}}`,
				task.SessionID, task.ToolUseID, task.AgentType)
			userLine := fmt.Sprintf(`{"type":"user","timestamp":"2026-01-15T10:05:00Z","sessionId":%q,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":%q,"content":"done","is_error":%s}]}}`,
				task.SessionID, task.ToolUseID, isError)
			lines += assistantLine + "\n" + userLine + "\n"
		}

		path := filepath.Join(projDir, sessionID+".jsonl")
		if err := os.WriteFile(path, []byte(lines), 0644); err != nil {
			t.Fatalf("write transcript: %v", err)
		}
	}
}

// almostEqual returns true if a and b differ by less than epsilon.
func almostEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// TestGetProjectHealth_EmptyDir verifies that no session data returns an empty result without error.
func TestGetProjectHealth_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addHealthTool(s)

	result, err := callTool(s, "get_project_health", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectHealthResult)
	if !ok {
		t.Fatalf("expected ProjectHealthResult, got %T", result)
	}

	if r.SessionCount != 0 {
		t.Errorf("SessionCount = %d, want 0", r.SessionCount)
	}
	if r.TopFriction == nil {
		t.Error("TopFriction is nil, want []string{}")
	}
	if r.ByAgentType == nil {
		t.Error("ByAgentType is nil, want empty map")
	}
}

// TestGetProjectHealth_DefaultsToMostRecent verifies that omitting project uses the most recent session's project.
func TestGetProjectHealth_DefaultsToMostRecent(t *testing.T) {
	dir := t.TempDir()
	// Write two sessions from different projects; the more recent one is "projectB".
	writeSessionMetaFull(t, dir, "sess-old", "2026-01-10T10:00:00Z", "/home/user/projectA", 0, 1)
	writeSessionMetaFull(t, dir, "sess-new", "2026-01-20T10:00:00Z", "/home/user/projectB", 0, 1)

	s := newTestServer(dir, 0)
	addHealthTool(s)

	result, err := callTool(s, "get_project_health", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectHealthResult)
	if !ok {
		t.Fatalf("expected ProjectHealthResult, got %T", result)
	}

	if r.Project != "projectB" {
		t.Errorf("Project = %q, want %q", r.Project, "projectB")
	}
	if r.SessionCount != 1 {
		t.Errorf("SessionCount = %d, want 1", r.SessionCount)
	}
}

// TestGetProjectHealth_FiltersByProject verifies that sessions from two projects only return target project data.
func TestGetProjectHealth_FiltersByProject(t *testing.T) {
	dir := t.TempDir()
	writeSessionMetaFull(t, dir, "sess-a1", "2026-01-10T10:00:00Z", "/home/user/projectA", 2, 0)
	writeSessionMetaFull(t, dir, "sess-a2", "2026-01-11T10:00:00Z", "/home/user/projectA", 4, 0)
	writeSessionMetaFull(t, dir, "sess-b1", "2026-01-12T10:00:00Z", "/home/user/projectB", 0, 1)

	s := newTestServer(dir, 0)
	addHealthTool(s)

	result, err := callTool(s, "get_project_health", json.RawMessage(`{"project":"projectA"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectHealthResult)
	if !ok {
		t.Fatalf("expected ProjectHealthResult, got %T", result)
	}

	if r.Project != "projectA" {
		t.Errorf("Project = %q, want %q", r.Project, "projectA")
	}
	if r.SessionCount != 2 {
		t.Errorf("SessionCount = %d, want 2", r.SessionCount)
	}
	// AvgToolErrors = (2+4)/2 = 3.0
	if !almostEqual(r.AvgToolErrors, 3.0, 0.001) {
		t.Errorf("AvgToolErrors = %f, want 3.0", r.AvgToolErrors)
	}
}

// TestGetProjectHealth_FrictionRate verifies 2 sessions with 1 having friction yields rate 0.5.
func TestGetProjectHealth_FrictionRate(t *testing.T) {
	dir := t.TempDir()
	writeSessionMetaFull(t, dir, "sess-1", "2026-01-10T10:00:00Z", "/home/user/myproject", 0, 0)
	writeSessionMetaFull(t, dir, "sess-2", "2026-01-11T10:00:00Z", "/home/user/myproject", 0, 0)
	// Only sess-1 has friction.
	writeFacet(t, dir, "sess-1", map[string]int{"wrong_approach": 2})

	s := newTestServer(dir, 0)
	addHealthTool(s)

	result, err := callTool(s, "get_project_health", json.RawMessage(`{"project":"myproject"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectHealthResult)
	if !ok {
		t.Fatalf("expected ProjectHealthResult, got %T", result)
	}

	if !almostEqual(r.FrictionRate, 0.5, 0.001) {
		t.Errorf("FrictionRate = %f, want 0.5", r.FrictionRate)
	}
}

// TestGetProjectHealth_AgentSuccessRate verifies 3 agents: 2 completed, 1 failed => 0.667.
func TestGetProjectHealth_AgentSuccessRate(t *testing.T) {
	dir := t.TempDir()
	writeSessionMetaFull(t, dir, "sess-x", "2026-01-10T10:00:00Z", "/home/user/agentproj", 0, 0)

	tasks := []struct {
		SessionID string
		ToolUseID string
		AgentType string
		Status    string
	}{
		{"sess-x", "tu-1", "general-purpose", "completed"},
		{"sess-x", "tu-2", "general-purpose", "completed"},
		{"sess-x", "tu-3", "general-purpose", "failed"},
	}
	writeAgentTaskTranscript(t, dir, "proj-hash-x", tasks)

	s := newTestServer(dir, 0)
	addHealthTool(s)

	result, err := callTool(s, "get_project_health", json.RawMessage(`{"project":"agentproj"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectHealthResult)
	if !ok {
		t.Fatalf("expected ProjectHealthResult, got %T", result)
	}

	expected := 2.0 / 3.0
	if !almostEqual(r.AgentSuccessRate, expected, 0.001) {
		t.Errorf("AgentSuccessRate = %f, want %f", r.AgentSuccessRate, expected)
	}
}

// TestGetProjectHealth_TopFriction verifies returns top 3 friction types sorted by frequency.
func TestGetProjectHealth_TopFriction(t *testing.T) {
	dir := t.TempDir()
	writeSessionMetaFull(t, dir, "sess-f", "2026-01-10T10:00:00Z", "/home/user/frictionproj", 0, 0)
	// friction_a:5, friction_b:3, friction_c:2, friction_d:1
	writeFacet(t, dir, "sess-f", map[string]int{
		"friction_a": 5,
		"friction_b": 3,
		"friction_c": 2,
		"friction_d": 1,
	})

	s := newTestServer(dir, 0)
	addHealthTool(s)

	result, err := callTool(s, "get_project_health", json.RawMessage(`{"project":"frictionproj"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectHealthResult)
	if !ok {
		t.Fatalf("expected ProjectHealthResult, got %T", result)
	}

	if len(r.TopFriction) != 3 {
		t.Fatalf("len(TopFriction) = %d, want 3", len(r.TopFriction))
	}
	// The top should be friction_a (count=5).
	if r.TopFriction[0] != "friction_a" {
		t.Errorf("TopFriction[0] = %q, want %q", r.TopFriction[0], "friction_a")
	}
	// Second should be friction_b (count=3).
	if r.TopFriction[1] != "friction_b" {
		t.Errorf("TopFriction[1] = %q, want %q", r.TopFriction[1], "friction_b")
	}
	// Third should be friction_c (count=2).
	if r.TopFriction[2] != "friction_c" {
		t.Errorf("TopFriction[2] = %q, want %q", r.TopFriction[2], "friction_c")
	}
}

// TestGetProjectHealth_UnknownProject verifies that an unknown project returns a zero-value result.
func TestGetProjectHealth_UnknownProject(t *testing.T) {
	dir := t.TempDir()
	writeSessionMetaFull(t, dir, "sess-known", "2026-01-10T10:00:00Z", "/home/user/knownproject", 0, 0)

	s := newTestServer(dir, 0)
	addHealthTool(s)

	result, err := callTool(s, "get_project_health", json.RawMessage(`{"project":"nonexistent"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(ProjectHealthResult)
	if !ok {
		t.Fatalf("expected ProjectHealthResult, got %T", result)
	}

	if r.Project != "nonexistent" {
		t.Errorf("Project = %q, want %q", r.Project, "nonexistent")
	}
	if r.SessionCount != 0 {
		t.Errorf("SessionCount = %d, want 0", r.SessionCount)
	}
	if r.FrictionRate != 0 {
		t.Errorf("FrictionRate = %f, want 0", r.FrictionRate)
	}
	if r.TopFriction == nil {
		t.Error("TopFriction is nil, want []string{}")
	}
	if len(r.TopFriction) != 0 {
		t.Errorf("len(TopFriction) = %d, want 0", len(r.TopFriction))
	}
}

// addHealthTool registers the get_project_health tool on the server.
// This is needed because tools.go's addTools does not yet include it
// (registration is orchestrator-owned post-merge).
func addHealthTool(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_project_health",
		Description: "Aggregate health metrics for a project.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project":{"type":"string","description":"Project name (filepath.Base of project path). Defaults to most recent session's project."}},"additionalProperties":false}`),
		Handler:     s.handleGetProjectHealth,
	})
}
