package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAgentTasks_CompletedTask(t *testing.T) {
	claudeDir := t.TempDir()
	projectDir := filepath.Join(claudeDir, "projects", "proj-hash")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_agent","name":"Task","input":{"subagent_type":"coder","description":"Implement feature","prompt":"Add login endpoint","run_in_background":false}}]}}`,
		`{"type":"user","timestamp":"2026-01-15T10:05:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_agent","content":"Feature implemented","is_error":false}]}}`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(projectDir, "session1.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	tasks, err := ParseAgentTasks(claudeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	task := tasks[0]
	if task.AgentID != "tu_agent" {
		t.Errorf("AgentID = %q, want %q", task.AgentID, "tu_agent")
	}
	if task.AgentType != "coder" {
		t.Errorf("AgentType = %q, want %q", task.AgentType, "coder")
	}
	if task.Status != "completed" {
		t.Errorf("Status = %q, want %q", task.Status, "completed")
	}
	if task.Description != "Implement feature" {
		t.Errorf("Description = %q, want %q", task.Description, "Implement feature")
	}
	if task.SessionID != "session1" {
		t.Errorf("SessionID = %q, want %q", task.SessionID, "session1")
	}
	if task.Background {
		t.Error("expected Background=false")
	}
	if task.DurationMs != 300000 {
		t.Errorf("DurationMs = %d, want 300000", task.DurationMs)
	}
}

func TestParseAgentTasks_FailedTask(t *testing.T) {
	claudeDir := t.TempDir()
	projectDir := filepath.Join(claudeDir, "projects", "proj-hash")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_fail","name":"Task","input":{"subagent_type":"builder","description":"Build","prompt":"Build all","run_in_background":false}}]}}`,
		`{"type":"user","timestamp":"2026-01-15T10:01:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_fail","content":"build error","is_error":true}]}}`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(projectDir, "sess-fail.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	tasks, err := ParseAgentTasks(claudeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Status != "failed" {
		t.Errorf("Status = %q, want %q", tasks[0].Status, "failed")
	}
}

func TestParseAgentTasks_KilledTask(t *testing.T) {
	claudeDir := t.TempDir()
	projectDir := filepath.Join(claudeDir, "projects", "proj-hash")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_kill","name":"Task","input":{"subagent_type":"runner","description":"Run","prompt":"Run forever","run_in_background":true}}]}}`,
		`{"type":"progress","parentToolUseID":"tu_kill","data":{"agentId":"agent-kill-id","type":"agent_progress"}}`,
		`{"type":"assistant","timestamp":"2026-01-15T10:01:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_stop","name":"TaskStop","input":{"task_id":"agent-kill-id"}}]}}`,
		`{"type":"user","timestamp":"2026-01-15T10:01:30Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_kill","content":"killed","is_error":false}]}}`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(projectDir, "sess-kill.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	tasks, err := ParseAgentTasks(claudeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Status != "killed" {
		t.Errorf("Status = %q, want %q", tasks[0].Status, "killed")
	}
	if !tasks[0].Background {
		t.Error("expected Background = true")
	}
}

func TestParseAgentTasks_NoProjects(t *testing.T) {
	claudeDir := t.TempDir()
	tasks, err := ParseAgentTasks(claudeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}
