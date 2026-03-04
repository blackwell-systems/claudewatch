package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

// newMultiProjectTestServer creates a Server with multi-project tools registered,
// pointing at the given tmpDir.
func newMultiProjectTestServer(tmpDir string) *Server {
	s := newTestServer(tmpDir, 0)
	addMultiProjectTools(s)
	return s
}

// writeMultiProjectJSONL writes a JSONL transcript under
// <claudeHome>/projects/<hash>/<sessionID>.jsonl with the given lines.
func writeMultiProjectJSONL(t *testing.T, claudeHome, hash, sessionID string, lines []string) {
	t.Helper()
	projDir := filepath.Join(claudeHome, "projects", hash)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}

	var content string
	for _, line := range lines {
		content += line + "\n"
	}

	path := filepath.Join(projDir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write multi-project JSONL: %v", err)
	}
}

// TestHandleGetSessionProjects_NoFilePaths verifies that a transcript with only
// text entries (no tool_use blocks) falls back to the project path with weight 1.0.
func TestHandleGetSessionProjects_NoFilePaths(t *testing.T) {
	dir := t.TempDir()

	sessionID := "no-filepaths-sess"
	projectPath := "/tmp/myproject"

	// Write a session meta file so ParseAllSessionMeta finds it.
	writeSessionMeta(t, dir, sessionID, "2026-01-15T10:00:00Z", projectPath, 1000, 500)

	// Write a transcript with only user/assistant text entries — no tool_use.
	lines := []string{
		`{"type":"user","sessionId":"no-filepaths-sess","timestamp":"2026-01-15T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
		`{"type":"assistant","sessionId":"no-filepaths-sess","timestamp":"2026-01-15T10:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"world"}]}}`,
	}
	// Use a hash dir that sorts after sessionID so the stub written by writeSessionMeta
	// (projects/<sessionID>/<sessionID>.jsonl with correct cwd) is found first by
	// ParseAllSessionMeta when looking up the project path.
	writeMultiProjectJSONL(t, dir, "z-hash-abc", sessionID, lines)

	s := newMultiProjectTestServer(dir)
	argsJSON, _ := json.Marshal(map[string]string{"session_id": sessionID})
	result, err := callTool(s, "get_session_projects", json.RawMessage(argsJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionProjectsResult)
	if !ok {
		t.Fatalf("expected SessionProjectsResult, got %T", result)
	}

	if r.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", r.SessionID, sessionID)
	}
	if len(r.Projects) != 1 {
		t.Fatalf("expected 1 project (fallback), got %d", len(r.Projects))
	}
	if r.Projects[0].Weight != 1.0 {
		t.Errorf("Weight = %f, want 1.0", r.Projects[0].Weight)
	}
	if r.Projects[0].Project != "myproject" {
		t.Errorf("Project = %q, want %q", r.Projects[0].Project, "myproject")
	}
	if r.PrimaryProject != "myproject" {
		t.Errorf("PrimaryProject = %q, want %q", r.PrimaryProject, "myproject")
	}
}

// TestHandleGetSessionProjects_CachedWeights verifies that pre-populated
// weights store data is returned without re-computing from transcript.
func TestHandleGetSessionProjects_CachedWeights(t *testing.T) {
	dir := t.TempDir()

	sessionID := "cached-weights-sess"

	// Write a session meta file so ParseAllSessionMeta finds it.
	writeSessionMeta(t, dir, sessionID, "2026-01-15T10:00:00Z", "/tmp/cachedproject", 1000, 500)

	// Pre-populate the weights store (placed alongside the tag store).
	weightsStorePath := filepath.Join(dir, "session-project-weights.json")
	ws := store.NewSessionProjectWeightsStore(weightsStorePath)
	cachedWeights := []store.ProjectWeight{
		{Project: "cached-repo", RepoRoot: "/tmp/cached-repo", Weight: 0.75, ToolCalls: 15},
		{Project: "other-repo", RepoRoot: "/tmp/other-repo", Weight: 0.25, ToolCalls: 5},
	}
	if err := ws.Set(sessionID, cachedWeights); err != nil {
		t.Fatalf("failed to pre-populate weights store: %v", err)
	}

	s := newMultiProjectTestServer(dir)
	argsJSON, _ := json.Marshal(map[string]string{"session_id": sessionID})
	result, err := callTool(s, "get_session_projects", json.RawMessage(argsJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(SessionProjectsResult)
	if !ok {
		t.Fatalf("expected SessionProjectsResult, got %T", result)
	}

	if r.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", r.SessionID, sessionID)
	}
	if len(r.Projects) != 2 {
		t.Fatalf("expected 2 projects from cache, got %d", len(r.Projects))
	}
	if r.Projects[0].Project != "cached-repo" {
		t.Errorf("Projects[0].Project = %q, want %q", r.Projects[0].Project, "cached-repo")
	}
	if r.Projects[0].Weight != 0.75 {
		t.Errorf("Projects[0].Weight = %f, want 0.75", r.Projects[0].Weight)
	}
	if r.PrimaryProject != "cached-repo" {
		t.Errorf("PrimaryProject = %q, want %q", r.PrimaryProject, "cached-repo")
	}
}

// TestHandleGetSessionProjects_NoSession verifies that when no session is found,
// the handler returns an empty result without error.
func TestHandleGetSessionProjects_NoSession(t *testing.T) {
	dir := t.TempDir()

	s := newMultiProjectTestServer(dir)
	// Call with no args — no session exists.
	result, err := callTool(s, "get_session_projects", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected no error for missing session, got %v", err)
	}

	r, ok := result.(SessionProjectsResult)
	if !ok {
		t.Fatalf("expected SessionProjectsResult, got %T", result)
	}

	if r.SessionID != "" {
		t.Errorf("SessionID = %q, want empty", r.SessionID)
	}
	if r.Projects == nil {
		t.Error("Projects is nil, want non-nil empty slice")
	}
	if len(r.Projects) != 0 {
		t.Errorf("Projects length = %d, want 0", len(r.Projects))
	}
}
