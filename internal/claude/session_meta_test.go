package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSessionMeta_ValidFile(t *testing.T) {
	dir := t.TempDir()
	data := `{
		"session_id": "sess-001",
		"project_path": "/home/user/project",
		"start_time": "2026-01-15T10:00:00Z",
		"duration_minutes": 45,
		"user_message_count": 12,
		"assistant_message_count": 11,
		"tool_counts": {"Bash": 5, "Read": 3},
		"languages": {"go": 10, "python": 2},
		"git_commits": 3,
		"git_pushes": 1,
		"input_tokens": 50000,
		"output_tokens": 15000,
		"first_prompt": "Help me refactor this module",
		"user_interruptions": 2,
		"user_response_times": [1.5, 3.2, 0.8],
		"tool_errors": 1,
		"tool_error_categories": {"permission_denied": 1},
		"uses_task_agent": true,
		"uses_mcp": false,
		"uses_web_search": true,
		"uses_web_fetch": false,
		"lines_added": 200,
		"lines_removed": 50,
		"files_modified": 8,
		"message_hours": [10, 10, 11, 11, 11],
		"user_message_timestamps": ["2026-01-15T10:00:00Z", "2026-01-15T10:05:00Z"]
	}`
	path := filepath.Join(dir, "sess-001.json")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	meta, err := ParseSessionMeta(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil meta")
	}
	if meta.SessionID != "sess-001" {
		t.Errorf("SessionID = %q, want %q", meta.SessionID, "sess-001")
	}
	if meta.ProjectPath != "/home/user/project" {
		t.Errorf("ProjectPath = %q, want %q", meta.ProjectPath, "/home/user/project")
	}
	if meta.DurationMinutes != 45 {
		t.Errorf("DurationMinutes = %d, want 45", meta.DurationMinutes)
	}
	if meta.ToolCounts["Bash"] != 5 {
		t.Errorf("ToolCounts[Bash] = %d, want 5", meta.ToolCounts["Bash"])
	}
	if meta.Languages["go"] != 10 {
		t.Errorf("Languages[go] = %d, want 10", meta.Languages["go"])
	}
	if meta.InputTokens != 50000 {
		t.Errorf("InputTokens = %d, want 50000", meta.InputTokens)
	}
	if !meta.UsesTaskAgent {
		t.Error("expected UsesTaskAgent = true")
	}
	if meta.UsesMCP {
		t.Error("expected UsesMCP = false")
	}
	if meta.LinesAdded != 200 {
		t.Errorf("LinesAdded = %d, want 200", meta.LinesAdded)
	}
	if meta.FilesModified != 8 {
		t.Errorf("FilesModified = %d, want 8", meta.FilesModified)
	}
	if len(meta.UserResponseTimes) != 3 {
		t.Errorf("UserResponseTimes length = %d, want 3", len(meta.UserResponseTimes))
	}
}

func TestParseSessionMeta_MissingFile(t *testing.T) {
	meta, err := ParseSessionMeta("/nonexistent/path/file.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if meta != nil {
		t.Errorf("expected nil meta, got %+v", meta)
	}
}

func TestParseSessionMeta_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	meta, err := ParseSessionMeta(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if meta != nil {
		t.Errorf("expected nil meta, got %+v", meta)
	}
}

func TestParseAllSessionMeta_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	metaDir := filepath.Join(dir, "usage-data", "session-meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	file1 := `{"session_id":"s1","duration_minutes":10,"input_tokens":100}`
	file2 := `{"session_id":"s2","duration_minutes":20,"input_tokens":200}`
	if err := os.WriteFile(filepath.Join(metaDir, "s1.json"), []byte(file1), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "s2.json"), []byte(file2), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("expected 2 metas, got %d", len(metas))
	}

	ids := map[string]bool{}
	for _, m := range metas {
		ids[m.SessionID] = true
	}
	if !ids["s1"] || !ids["s2"] {
		t.Errorf("expected sessions s1 and s2, got %v", ids)
	}
}

func TestParseAllSessionMeta_MissingDir(t *testing.T) {
	dir := t.TempDir()
	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if metas != nil {
		t.Errorf("expected nil metas, got %v", metas)
	}
}

func TestParseAllSessionMeta_SkipsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	metaDir := filepath.Join(dir, "usage-data", "session-meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	valid := `{"session_id":"good","duration_minutes":5}`
	invalid := `not valid json`
	if err := os.WriteFile(filepath.Join(metaDir, "good.json"), []byte(valid), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "bad.json"), []byte(invalid), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta (invalid skipped), got %d", len(metas))
	}
	if metas[0].SessionID != "good" {
		t.Errorf("SessionID = %q, want %q", metas[0].SessionID, "good")
	}
}

func TestParseAllSessionMeta_SkipsNonJsonFiles(t *testing.T) {
	dir := t.TempDir()
	metaDir := filepath.Join(dir, "usage-data", "session-meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	valid := `{"session_id":"only-json"}`
	if err := os.WriteFile(filepath.Join(metaDir, "data.json"), []byte(valid), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaDir, "readme.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
}

func TestParseAllSessionMeta_SkipsSubdirectories(t *testing.T) {
	dir := t.TempDir()
	metaDir := filepath.Join(dir, "usage-data", "session-meta")
	if err := os.MkdirAll(filepath.Join(metaDir, "subdir"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	valid := `{"session_id":"top-level"}`
	if err := os.WriteFile(filepath.Join(metaDir, "top.json"), []byte(valid), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
}

func TestParseAllSessionMeta_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	metaDir := filepath.Join(dir, "usage-data", "session-meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("expected 0 metas, got %d", len(metas))
	}
}
