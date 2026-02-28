package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseTodoFilename(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		wantSID   string
		wantAgent string
	}{
		{
			"matching IDs",
			"abc-123-agent-abc-123.json",
			"abc-123",
			"abc-123",
		},
		{
			"different IDs",
			"sess-1-agent-agent-2.json",
			"sess-1",
			"agent-2",
		},
		{
			"no agent separator",
			"just-a-session.json",
			"just-a-session",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid, aid := parseTodoFilename(tt.filename)
			if sid != tt.wantSID {
				t.Errorf("sessionID = %q, want %q", sid, tt.wantSID)
			}
			if aid != tt.wantAgent {
				t.Errorf("agentID = %q, want %q", aid, tt.wantAgent)
			}
		})
	}
}

func TestParseAllTodos(t *testing.T) {
	dir := t.TempDir()
	todosDir := filepath.Join(dir, "todos")
	os.MkdirAll(todosDir, 0o755)

	// Write a non-empty todo file.
	tasks := []TodoTask{
		{Content: "Fix the bug", Status: "completed", ID: "1"},
		{Content: "Add tests", Status: "pending", ID: "2"},
	}
	data, _ := json.Marshal(tasks)
	os.WriteFile(filepath.Join(todosDir, "sess1-agent-sess1.json"), data, 0o644)

	// Write an empty array file (should be skipped).
	os.WriteFile(filepath.Join(todosDir, "sess2-agent-sess2.json"), []byte("[]"), 0o644)

	results, err := ParseAllTodos(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}
	if results[0].SessionID != "sess1" {
		t.Errorf("sessionID = %q, want %q", results[0].SessionID, "sess1")
	}
	if len(results[0].Tasks) != 2 {
		t.Errorf("tasks = %d, want 2", len(results[0].Tasks))
	}
	if results[0].Tasks[0].Status != "completed" {
		t.Errorf("first task status = %q, want completed", results[0].Tasks[0].Status)
	}
}

func TestParseAllTodos_MissingDir(t *testing.T) {
	results, err := ParseAllTodos("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
