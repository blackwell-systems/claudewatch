package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeActiveJSONLWithToolUse writes a JSONL file simulating an active session with
// assistant tool_use blocks. Entries are written under <claudeHome>/projects/<hash>/<sessionID>.jsonl.
func writeActiveJSONLWithToolUse(t *testing.T, claudeHome, hash, sessionID string, entries []map[string]any) string {
	t.Helper()
	projDir := filepath.Join(claudeHome, "projects", hash)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}

	path := filepath.Join(projDir, sessionID+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create jsonl: %v", err)
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("encode entry: %v", err)
		}
	}
	return path
}

// mkToolUseEntry builds a JSONL assistant entry with tool_use content blocks.
func mkToolUseEntry(sessionID, timestamp string, tools []map[string]any) map[string]any {
	var content []map[string]any
	for _, tool := range tools {
		block := map[string]any{
			"type": "tool_use",
			"id":   tool["id"],
			"name": tool["name"],
		}
		if inp, ok := tool["input"]; ok {
			block["input"] = inp
		} else {
			block["input"] = map[string]any{}
		}
		content = append(content, block)
	}
	msg, _ := json.Marshal(map[string]any{
		"role":    "assistant",
		"content": content,
		"usage": map[string]any{
			"input_tokens":  0,
			"output_tokens": 0,
		},
	})
	return map[string]any{
		"type":      "assistant",
		"sessionId": sessionID,
		"timestamp": timestamp,
		"message":   json.RawMessage(msg),
	}
}

// --- Token Velocity Tests ---

func TestGetTokenVelocity_NoActiveSession(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addVelocityTools(s)

	_, err := callTool(s, "get_token_velocity", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for no active session, got nil")
	}
}

func TestGetTokenVelocity_ActiveSession(t *testing.T) {
	dir := t.TempDir()
	writeActiveJSONL(t, dir, "proj-hash", "vel-sess-001", 2_000_000, 500_000)

	s := newTestServer(dir, 0)
	addVelocityTools(s)

	result, err := callTool(s, "get_token_velocity", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(TokenVelocityResult)
	if !ok {
		t.Fatalf("expected TokenVelocityResult, got %T", result)
	}

	if !r.Live {
		t.Error("Live = false, want true")
	}
	if r.SessionID != "vel-sess-001" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "vel-sess-001")
	}
	if r.TotalTokens != 2_500_000 {
		t.Errorf("TotalTokens = %d, want %d", r.TotalTokens, 2_500_000)
	}
	// Status must be one of the three valid values.
	validStatuses := map[string]bool{"flowing": true, "slow": true, "idle": true}
	if !validStatuses[r.Status] {
		t.Errorf("Status = %q, want one of flowing/slow/idle", r.Status)
	}
	if r.ElapsedMinutes <= 0 {
		t.Errorf("ElapsedMinutes = %f, want > 0", r.ElapsedMinutes)
	}
}

// --- Commit Attempt Ratio Tests ---

func TestGetCommitAttemptRatio_NoActiveSession(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)
	addVelocityTools(s)

	_, err := callTool(s, "get_commit_attempt_ratio", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for no active session, got nil")
	}
}

func TestGetCommitAttemptRatio_WithData(t *testing.T) {
	dir := t.TempDir()

	bashInput, _ := json.Marshal(map[string]any{
		"command": "git commit -m 'add feature'",
	})

	// 5 edits, 1 commit -> ratio = 0.2 -> "normal"
	entries := []map[string]any{
		{
			"type":      "user",
			"sessionId": "commit-sess-001",
			"timestamp": "2026-03-01T10:00:00Z",
		},
		mkToolUseEntry("commit-sess-001", "2026-03-01T10:00:01Z", []map[string]any{
			{"id": "tu1", "name": "Edit"},
			{"id": "tu2", "name": "Write"},
			{"id": "tu3", "name": "Edit"},
			{"id": "tu5", "name": "Edit"},
			{"id": "tu6", "name": "Write"},
		}),
		mkToolUseEntry("commit-sess-001", "2026-03-01T10:00:02Z", []map[string]any{
			{"id": "tu4", "name": "Bash", "input": json.RawMessage(bashInput)},
		}),
	}

	writeActiveJSONLWithToolUse(t, dir, "proj-hash", "commit-sess-001", entries)

	s := newTestServer(dir, 0)
	addVelocityTools(s)

	result, err := callTool(s, "get_commit_attempt_ratio", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CommitAttemptResult)
	if !ok {
		t.Fatalf("expected CommitAttemptResult, got %T", result)
	}

	if !r.Live {
		t.Error("Live = false, want true")
	}
	if r.SessionID != "commit-sess-001" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "commit-sess-001")
	}
	if r.EditWriteAttempts != 5 {
		t.Errorf("EditWriteAttempts = %d, want 5", r.EditWriteAttempts)
	}
	if r.GitCommits != 1 {
		t.Errorf("GitCommits = %d, want 1", r.GitCommits)
	}
	expectedRatio := 1.0 / 5.0
	if r.Ratio < expectedRatio-0.001 || r.Ratio > expectedRatio+0.001 {
		t.Errorf("Ratio = %f, want ~%f", r.Ratio, expectedRatio)
	}
	if r.Assessment != "normal" {
		t.Errorf("Assessment = %q, want %q", r.Assessment, "normal")
	}
}

func TestGetCommitAttemptRatio_NoChanges(t *testing.T) {
	dir := t.TempDir()

	// Session with only Read tool uses — no Edit/Write, no commits.
	entries := []map[string]any{
		{
			"type":      "user",
			"sessionId": "nochange-sess",
			"timestamp": "2026-03-01T10:00:00Z",
		},
		mkToolUseEntry("nochange-sess", "2026-03-01T10:00:01Z", []map[string]any{
			{"id": "tu1", "name": "Read"},
		}),
	}

	writeActiveJSONLWithToolUse(t, dir, "proj-hash", "nochange-sess", entries)

	s := newTestServer(dir, 0)
	addVelocityTools(s)

	result, err := callTool(s, "get_commit_attempt_ratio", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(CommitAttemptResult)
	if !ok {
		t.Fatalf("expected CommitAttemptResult, got %T", result)
	}

	if r.EditWriteAttempts != 0 {
		t.Errorf("EditWriteAttempts = %d, want 0", r.EditWriteAttempts)
	}
	if r.GitCommits != 0 {
		t.Errorf("GitCommits = %d, want 0", r.GitCommits)
	}
	if r.Assessment != "no_changes" {
		t.Errorf("Assessment = %q, want %q", r.Assessment, "no_changes")
	}
}

func TestGetCommitAttemptRatio_Efficient(t *testing.T) {
	dir := t.TempDir()

	bashInput, _ := json.Marshal(map[string]any{
		"command": "git commit -m 'done'",
	})

	// 2 edits, 1 commit -> ratio = 0.5 -> "efficient"
	entries := []map[string]any{
		{
			"type":      "user",
			"sessionId": "eff-sess",
			"timestamp": "2026-03-01T10:00:00Z",
		},
		mkToolUseEntry("eff-sess", "2026-03-01T10:00:01Z", []map[string]any{
			{"id": "tu1", "name": "Edit"},
			{"id": "tu2", "name": "Write"},
		}),
		mkToolUseEntry("eff-sess", "2026-03-01T10:00:02Z", []map[string]any{
			{"id": "tu3", "name": "Bash", "input": json.RawMessage(bashInput)},
		}),
	}

	writeActiveJSONLWithToolUse(t, dir, "proj-hash", "eff-sess", entries)

	s := newTestServer(dir, 0)
	addVelocityTools(s)

	result, err := callTool(s, "get_commit_attempt_ratio", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := result.(CommitAttemptResult)
	if r.Assessment != "efficient" {
		t.Errorf("Assessment = %q, want %q (ratio=%f)", r.Assessment, "efficient", r.Ratio)
	}
}
