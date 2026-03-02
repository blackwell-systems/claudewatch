package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helper: write a JSONL file with the given lines (each line is JSON-marshalled).
func writeJSONLFile(t *testing.T, path string, entries []map[string]any) {
	t.Helper()
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
}

// --- FindActiveSessionPath tests ---

func TestFindActiveSessionPath_NoProjectsDir(t *testing.T) {
	claudeHome := t.TempDir()
	// projects/ directory intentionally absent
	got, err := FindActiveSessionPath(claudeHome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty path, got %q", got)
	}
}

func TestFindActiveSessionPath_EmptyProjectsDir(t *testing.T) {
	claudeHome := t.TempDir()
	// create projects/ but with no JSONL files
	if err := os.MkdirAll(filepath.Join(claudeHome, "projects", "hashdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindActiveSessionPath(claudeHome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty path, got %q", got)
	}
}

func TestFindActiveSessionPath_MtimeFallback_RecentFile(t *testing.T) {
	claudeHome := t.TempDir()
	hashDir := filepath.Join(claudeHome, "projects", "abc123")
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionFile := filepath.Join(hashDir, "session1.jsonl")
	writeJSONLFile(t, sessionFile, []map[string]any{
		{"type": "user", "timestamp": "2026-03-01T10:00:00Z", "sessionId": "session1"},
	})

	// Touch the file to ensure its mtime is now (within 5 minutes).
	now := time.Now()
	if err := os.Chtimes(sessionFile, now, now); err != nil {
		t.Fatal(err)
	}

	got, err := FindActiveSessionPath(claudeHome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sessionFile {
		t.Fatalf("expected %q, got %q", sessionFile, got)
	}
}

func TestFindActiveSessionPath_MtimeFallback_OldFile(t *testing.T) {
	claudeHome := t.TempDir()
	hashDir := filepath.Join(claudeHome, "projects", "abc123")
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sessionFile := filepath.Join(hashDir, "session1.jsonl")
	writeJSONLFile(t, sessionFile, []map[string]any{
		{"type": "user", "timestamp": "2026-03-01T10:00:00Z", "sessionId": "session1"},
	})

	// Set the mtime to >5 minutes ago.
	oldTime := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(sessionFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	got, err := FindActiveSessionPath(claudeHome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty path for old file, got %q", got)
	}
}

// --- ParseActiveSession tests ---

func TestParseActiveSession_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "myhash", "session-abc.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := ParseActiveSession(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil *SessionMeta")
	}
	if meta.UserMessageCount != 0 || meta.AssistantMessageCount != 0 {
		t.Fatalf("expected zero counts, got user=%d assistant=%d",
			meta.UserMessageCount, meta.AssistantMessageCount)
	}
}

func TestParseActiveSession_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	hashDir := filepath.Join(dir, "myhash")
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hashDir, "session1.jsonl")

	// One valid JSON line with no trailing newline.
	line := `{"type":"user","timestamp":"2026-03-01T10:00:00Z","sessionId":"session1"}`
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := ParseActiveSession(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil *SessionMeta")
	}
	// No trailing newline means no complete lines by our truncation logic,
	// so we get an empty struct (the empty-file path).
	// The spec says: if no '\n' found, return &SessionMeta{} immediately.
	if meta.UserMessageCount != 0 {
		t.Fatalf("expected 0 user messages (no trailing newline), got %d", meta.UserMessageCount)
	}
}

func TestParseActiveSession_PartialLastLine(t *testing.T) {
	dir := t.TempDir()
	hashDir := filepath.Join(dir, "myhash")
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hashDir, "session1.jsonl")

	// One complete line followed by a partial line.
	content := `{"type":"user","timestamp":"2026-03-01T10:00:00Z","sessionId":"session1"}` + "\n" +
		`{"type":"assistant","timestamp":"2026-03-01T10:01:00Z","sessionId":"session1","message":{"us`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := ParseActiveSession(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil *SessionMeta")
	}
	if meta.UserMessageCount != 1 {
		t.Fatalf("expected 1 user message, got %d", meta.UserMessageCount)
	}
	if meta.AssistantMessageCount != 0 {
		t.Fatalf("expected 0 assistant messages (partial line discarded), got %d", meta.AssistantMessageCount)
	}
}

func TestParseActiveSession_SessionIDFromEntry(t *testing.T) {
	dir := t.TempDir()
	hashDir := filepath.Join(dir, "myhash")
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hashDir, "filename-session.jsonl")
	writeJSONLFile(t, path, []map[string]any{
		{"type": "user", "timestamp": "2026-03-01T10:00:00Z", "sessionId": "entry-session-id"},
	})

	meta, err := ParseActiveSession(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.SessionID != "entry-session-id" {
		t.Fatalf("expected SessionID from entry %q, got %q", "entry-session-id", meta.SessionID)
	}
}

func TestParseActiveSession_SessionIDFromFilename(t *testing.T) {
	dir := t.TempDir()
	hashDir := filepath.Join(dir, "myhash")
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hashDir, "my-session-id.jsonl")
	// Entries have no sessionId field.
	writeJSONLFile(t, path, []map[string]any{
		{"type": "user", "timestamp": "2026-03-01T10:00:00Z"},
	})

	meta, err := ParseActiveSession(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.SessionID != "my-session-id" {
		t.Fatalf("expected SessionID from filename %q, got %q", "my-session-id", meta.SessionID)
	}
}

func TestParseActiveSession_TokenAccumulation(t *testing.T) {
	dir := t.TempDir()
	hashDir := filepath.Join(dir, "myhash")
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hashDir, "session1.jsonl")

	// Two assistant entries each with usage tokens.
	entry1Msg, _ := json.Marshal(map[string]any{
		"usage": map[string]any{"input_tokens": 100, "output_tokens": 50},
	})
	entry2Msg, _ := json.Marshal(map[string]any{
		"usage": map[string]any{"input_tokens": 200, "output_tokens": 75},
	})

	writeJSONLFile(t, path, []map[string]any{
		{"type": "assistant", "timestamp": "2026-03-01T10:00:00Z", "sessionId": "session1", "message": json.RawMessage(entry1Msg)},
		{"type": "assistant", "timestamp": "2026-03-01T10:01:00Z", "sessionId": "session1", "message": json.RawMessage(entry2Msg)},
	})

	meta, err := ParseActiveSession(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.InputTokens != 300 {
		t.Fatalf("expected InputTokens=300, got %d", meta.InputTokens)
	}
	if meta.OutputTokens != 125 {
		t.Fatalf("expected OutputTokens=125, got %d", meta.OutputTokens)
	}
}

func TestParseActiveSession_MessageCounts(t *testing.T) {
	dir := t.TempDir()
	hashDir := filepath.Join(dir, "myhash")
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hashDir, "session1.jsonl")

	writeJSONLFile(t, path, []map[string]any{
		{"type": "user", "timestamp": "2026-03-01T10:00:00Z", "sessionId": "session1"},
		{"type": "assistant", "timestamp": "2026-03-01T10:01:00Z", "sessionId": "session1"},
		{"type": "user", "timestamp": "2026-03-01T10:02:00Z", "sessionId": "session1"},
		{"type": "assistant", "timestamp": "2026-03-01T10:03:00Z", "sessionId": "session1"},
		{"type": "user", "timestamp": "2026-03-01T10:04:00Z", "sessionId": "session1"},
	})

	meta, err := ParseActiveSession(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.UserMessageCount != 3 {
		t.Fatalf("expected UserMessageCount=3, got %d", meta.UserMessageCount)
	}
	if meta.AssistantMessageCount != 2 {
		t.Fatalf("expected AssistantMessageCount=2, got %d", meta.AssistantMessageCount)
	}
}

func TestParseActiveSession_ProjectPathIsHash(t *testing.T) {
	// When no cwd field is present, ProjectPath falls back to the hash dir name.
	dir := t.TempDir()
	hashDir := filepath.Join(dir, "the-project-hash")
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hashDir, "session1.jsonl")
	writeJSONLFile(t, path, []map[string]any{
		{"type": "user", "timestamp": "2026-03-01T10:00:00Z", "sessionId": "session1"},
	})

	meta, err := ParseActiveSession(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.ProjectPath != "the-project-hash" {
		t.Fatalf("expected ProjectPath=%q, got %q", "the-project-hash", meta.ProjectPath)
	}
}

func TestParseActiveSession_ProjectPathFromCwd(t *testing.T) {
	// When a cwd field is present (SessionStart progress entry), ProjectPath
	// is set to the real filesystem path rather than the hash dir name.
	dir := t.TempDir()
	hashDir := filepath.Join(dir, "-Users-dayna-blackwell-code-commitmux")
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hashDir, "session1.jsonl")
	writeJSONLFile(t, path, []map[string]any{
		{"type": "progress", "cwd": "/Users/dayna.blackwell/code/commitmux", "sessionId": "session1", "timestamp": "2026-03-01T10:00:00Z"},
		{"type": "user", "timestamp": "2026-03-01T10:01:00Z", "sessionId": "session1"},
	})

	meta, err := ParseActiveSession(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.ProjectPath != "/Users/dayna.blackwell/code/commitmux" {
		t.Fatalf("expected ProjectPath=%q, got %q", "/Users/dayna.blackwell/code/commitmux", meta.ProjectPath)
	}
}

func TestParseActiveSession_ReadError(t *testing.T) {
	meta, err := ParseActiveSession("/nonexistent/path/to/session.jsonl")
	if err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}
	if meta != nil {
		t.Fatalf("expected nil *SessionMeta on error, got %+v", meta)
	}
}

func TestActiveSessionInfo_EmbedsMeta(t *testing.T) {
	meta := SessionMeta{
		SessionID:        "test-session",
		UserMessageCount: 5,
	}
	info := ActiveSessionInfo{
		SessionMeta: meta,
		Path:        "/some/path/session.jsonl",
		IsLive:      true,
	}

	if !info.IsLive {
		t.Fatal("expected IsLive == true")
	}
	if info.SessionID != "test-session" {
		t.Fatalf("expected embedded SessionID=%q, got %q", "test-session", info.SessionID)
	}
	if info.UserMessageCount != 5 {
		t.Fatalf("expected embedded UserMessageCount=5, got %d", info.UserMessageCount)
	}
	if info.Path != "/some/path/session.jsonl" {
		t.Fatalf("expected Path=%q, got %q", "/some/path/session.jsonl", info.Path)
	}
}
