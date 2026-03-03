package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createTestJSONL creates dir/projects/<hash>/<sessionID>.jsonl with the given lines.
func createTestJSONL(t *testing.T, dir, hash, sessionID string, lines []string) string {
	t.Helper()
	projDir := filepath.Join(dir, "projects", hash)
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir projects/%s: %v", hash, err)
	}
	path := filepath.Join(projDir, sessionID+".jsonl")
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// minimalJSONL returns two lines that form a valid minimal session transcript.
func minimalJSONL(sessionID, cwd string) []string {
	return []string{
		`{"type":"user","sessionId":"` + sessionID + `","timestamp":"2026-01-15T10:00:00Z","cwd":"` + cwd + `","message":{"role":"user","content":[{"type":"text","text":"Hello world"}]}}`,
		`{"type":"assistant","sessionId":"` + sessionID + `","timestamp":"2026-01-15T10:01:00Z","message":{"role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50}}}`,
	}
}

// ---------- ParseAllSessionMeta tests ----------

func TestParseAllSessionMeta_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	createTestJSONL(t, dir, "hash1", "s1", minimalJSONL("s1", "/home/user/proj1"))
	createTestJSONL(t, dir, "hash2", "s2", minimalJSONL("s2", "/home/user/proj2"))

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
	// No projects dir created.
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
	// Valid JSONL.
	createTestJSONL(t, dir, "hash1", "good", minimalJSONL("good", "/home/user/proj"))
	// Empty file — no complete lines, will return a minimal struct (not nil).
	// But an empty file should produce a valid (if sparse) SessionMeta, not nil.
	// Truly skipped: files that cause os.ReadFile to fail (can't simulate in temp dir easily).
	// Use a JSONL file with only unparseable lines.
	badProjDir := filepath.Join(dir, "projects", "hash2")
	if err := os.MkdirAll(badProjDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(badProjDir, "bad.jsonl"), []byte(""), 0644); err != nil {
		t.Fatalf("write bad.jsonl: %v", err)
	}

	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "bad" with empty file returns a minimal struct (non-nil, non-error), so we get 2.
	// But "good" must be present.
	foundGood := false
	for _, m := range metas {
		if m.SessionID == "good" {
			foundGood = true
		}
	}
	if !foundGood {
		t.Errorf("expected session 'good' to be present in results %v", metas)
	}
}

func TestParseAllSessionMeta_SkipsNonJsonlFiles(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "projects", "hash1")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a valid .jsonl and a .txt that should be skipped.
	createTestJSONL(t, dir, "hash1", "s1", minimalJSONL("s1", "/home/user/proj"))
	if err := os.WriteFile(filepath.Join(projDir, "readme.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("write readme.txt: %v", err)
	}

	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta (txt skipped), got %d", len(metas))
	}
	if metas[0].SessionID != "s1" {
		t.Errorf("SessionID = %q, want %q", metas[0].SessionID, "s1")
	}
}

func TestParseAllSessionMeta_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	// Create the projects dir but leave it empty.
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0755); err != nil {
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

func TestParseAllSessionMeta_CacheHit(t *testing.T) {
	dir := t.TempDir()
	// Create a JSONL file with data that parses to sessionID "jsonl-session".
	jsonlPath := createTestJSONL(t, dir, "hash1", "sess1", minimalJSONL("jsonl-session", "/proj"))

	// Create the cache dir and write a cache file with DIFFERENT data
	// (wrong sessionID) so we can verify the cache is used, not the JSONL.
	cacheDir := filepath.Join(dir, "usage-data", "session-meta")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir cacheDir: %v", err)
	}
	cacheMeta := SessionMeta{SessionID: "cache-session", InputTokens: 9999}
	cacheData, _ := json.MarshalIndent(cacheMeta, "", "  ")
	cachePath := filepath.Join(cacheDir, "sess1.json")
	if err := os.WriteFile(cachePath, cacheData, 0644); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	// Set cache mtime AFTER jsonl mtime so the cache is fresh.
	jsonlMtime := time.Now().Add(-2 * time.Minute)
	cacheMtime := time.Now().Add(-1 * time.Minute)
	if err := os.Chtimes(jsonlPath, jsonlMtime, jsonlMtime); err != nil {
		t.Fatalf("chtimes jsonl: %v", err)
	}
	if err := os.Chtimes(cachePath, cacheMtime, cacheMtime); err != nil {
		t.Fatalf("chtimes cache: %v", err)
	}

	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
	// Should have loaded from cache (cache-session, not jsonl-session).
	if metas[0].SessionID != "cache-session" {
		t.Errorf("SessionID = %q, want %q (cache hit expected)", metas[0].SessionID, "cache-session")
	}
	if metas[0].InputTokens != 9999 {
		t.Errorf("InputTokens = %d, want 9999 (cache hit expected)", metas[0].InputTokens)
	}
}

func TestParseAllSessionMeta_CacheMiss(t *testing.T) {
	dir := t.TempDir()
	createTestJSONL(t, dir, "hash1", "sess1", minimalJSONL("s1", "/home/user/proj"))
	// No cache file — should parse from JSONL and write cache.
	cacheDir := filepath.Join(dir, "usage-data", "session-meta")

	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
	if metas[0].SessionID != "s1" {
		t.Errorf("SessionID = %q, want %q", metas[0].SessionID, "s1")
	}

	// Verify cache file was written.
	cachePath := filepath.Join(cacheDir, "sess1.json")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Errorf("expected cache file to be written at %s", cachePath)
	}
}

func TestParseAllSessionMeta_StaleCache(t *testing.T) {
	dir := t.TempDir()
	// Create a JSONL with sessionID "jsonl-session".
	jsonlPath := createTestJSONL(t, dir, "hash1", "sess1", minimalJSONL("jsonl-session", "/proj"))

	// Create stale cache with different sessionID.
	cacheDir := filepath.Join(dir, "usage-data", "session-meta")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir cacheDir: %v", err)
	}
	staleMeta := SessionMeta{SessionID: "stale-session", InputTokens: 1}
	staleData, _ := json.MarshalIndent(staleMeta, "", "  ")
	cachePath := filepath.Join(cacheDir, "sess1.json")
	if err := os.WriteFile(cachePath, staleData, 0644); err != nil {
		t.Fatalf("write stale cache: %v", err)
	}

	// Set cache mtime BEFORE jsonl mtime so the cache is stale.
	cacheMtime := time.Now().Add(-2 * time.Minute)
	jsonlMtime := time.Now().Add(-1 * time.Minute)
	if err := os.Chtimes(cachePath, cacheMtime, cacheMtime); err != nil {
		t.Fatalf("chtimes cache: %v", err)
	}
	if err := os.Chtimes(jsonlPath, jsonlMtime, jsonlMtime); err != nil {
		t.Fatalf("chtimes jsonl: %v", err)
	}

	metas, err := ParseAllSessionMeta(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(metas))
	}
	// Should have re-parsed from JSONL (jsonl-session, not stale-session).
	if metas[0].SessionID != "jsonl-session" {
		t.Errorf("SessionID = %q, want %q (stale cache should be bypassed)", metas[0].SessionID, "jsonl-session")
	}

	// Cache file should be refreshed with new data.
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read refreshed cache: %v", err)
	}
	var refreshed SessionMeta
	if err := json.Unmarshal(data, &refreshed); err != nil {
		t.Fatalf("unmarshal refreshed cache: %v", err)
	}
	if refreshed.SessionID != "jsonl-session" {
		t.Errorf("refreshed cache SessionID = %q, want %q", refreshed.SessionID, "jsonl-session")
	}
}

// ---------- parseJSONLToSessionMeta tests ----------

func TestParseJSONLToSessionMeta_Basic(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := createTestJSONL(t, dir, "hash1", "sess-abc",
		minimalJSONL("sess-abc", "/home/user/myproject"))

	meta, err := parseJSONLToSessionMeta(jsonlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil meta")
	}
	if meta.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want %q", meta.SessionID, "sess-abc")
	}
	if meta.ProjectPath != "/home/user/myproject" {
		t.Errorf("ProjectPath = %q, want %q", meta.ProjectPath, "/home/user/myproject")
	}
	if meta.StartTime != "2026-01-15T10:00:00Z" {
		t.Errorf("StartTime = %q, want %q", meta.StartTime, "2026-01-15T10:00:00Z")
	}
	if meta.DurationMinutes != 1 {
		t.Errorf("DurationMinutes = %d, want 1", meta.DurationMinutes)
	}
	if meta.UserMessageCount != 1 {
		t.Errorf("UserMessageCount = %d, want 1", meta.UserMessageCount)
	}
	if meta.AssistantMessageCount != 1 {
		t.Errorf("AssistantMessageCount = %d, want 1", meta.AssistantMessageCount)
	}
	if meta.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", meta.InputTokens)
	}
	if meta.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", meta.OutputTokens)
	}
	if meta.FirstPrompt != "Hello world" {
		t.Errorf("FirstPrompt = %q, want %q", meta.FirstPrompt, "Hello world")
	}
}

func TestParseJSONLToSessionMeta_ToolCounts(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		`{"type":"user","sessionId":"s1","timestamp":"2026-01-15T10:00:00Z","cwd":"/proj","message":{"role":"user","content":[{"type":"text","text":"do stuff"}]}}`,
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-01-15T10:01:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash"},{"type":"tool_use","id":"t2","name":"Task"},{"type":"tool_use","id":"t3","name":"mcp__myserver__tool"}],"usage":{"input_tokens":200,"output_tokens":80}}}`,
	}
	jsonlPath := createTestJSONL(t, dir, "hash1", "s1", lines)

	meta, err := parseJSONLToSessionMeta(jsonlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.ToolCounts["Bash"] != 1 {
		t.Errorf("ToolCounts[Bash] = %d, want 1", meta.ToolCounts["Bash"])
	}
	if meta.ToolCounts["Task"] != 1 {
		t.Errorf("ToolCounts[Task] = %d, want 1", meta.ToolCounts["Task"])
	}
	if meta.ToolCounts["mcp__myserver__tool"] != 1 {
		t.Errorf("ToolCounts[mcp__myserver__tool] = %d, want 1", meta.ToolCounts["mcp__myserver__tool"])
	}
	if !meta.UsesTaskAgent {
		t.Error("expected UsesTaskAgent = true")
	}
	if !meta.UsesMCP {
		t.Error("expected UsesMCP = true")
	}
}

func TestParseJSONLToSessionMeta_ToolErrors(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		`{"type":"user","sessionId":"s1","timestamp":"2026-01-15T10:00:00Z","cwd":"/proj","message":{"role":"user","content":[{"type":"text","text":"run something"}]}}`,
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-01-15T10:01:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash"}],"usage":{"input_tokens":50,"output_tokens":20}}}`,
		`{"type":"user","sessionId":"s1","timestamp":"2026-01-15T10:02:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"command not found"}]}}`,
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-01-15T10:03:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t2","name":"Read"}],"usage":{"input_tokens":60,"output_tokens":25}}}`,
		`{"type":"user","sessionId":"s1","timestamp":"2026-01-15T10:04:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t2","is_error":true,"content":"file not found"}]}}`,
	}
	jsonlPath := createTestJSONL(t, dir, "hash1", "s1", lines)

	meta, err := parseJSONLToSessionMeta(jsonlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.ToolErrors != 2 {
		t.Errorf("ToolErrors = %d, want 2", meta.ToolErrors)
	}
}

func TestParseJSONLToSessionMeta_WebFlags(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		`{"type":"user","sessionId":"s1","timestamp":"2026-01-15T10:00:00Z","cwd":"/proj","message":{"role":"user","content":[{"type":"text","text":"search and fetch"}]}}`,
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-01-15T10:01:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"WebSearch"},{"type":"tool_use","id":"t2","name":"WebFetch"}],"usage":{"input_tokens":100,"output_tokens":40}}}`,
	}
	jsonlPath := createTestJSONL(t, dir, "hash1", "s1", lines)

	meta, err := parseJSONLToSessionMeta(jsonlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !meta.UsesWebSearch {
		t.Error("expected UsesWebSearch = true")
	}
	if !meta.UsesWebFetch {
		t.Error("expected UsesWebFetch = true")
	}
}

func TestParseJSONLToSessionMeta_DurationMinutes(t *testing.T) {
	dir := t.TempDir()
	lines := []string{
		`{"type":"user","sessionId":"s1","timestamp":"2026-01-15T10:00:00Z","cwd":"/proj","message":{"role":"user","content":[{"type":"text","text":"start"}]}}`,
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-01-15T10:00:30Z","message":{"role":"assistant","content":[],"usage":{"input_tokens":10,"output_tokens":5}}}`,
		`{"type":"user","sessionId":"s1","timestamp":"2026-01-15T10:45:00Z","message":{"role":"user","content":[{"type":"text","text":"done"}]}}`,
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-01-15T11:30:00Z","message":{"role":"assistant","content":[],"usage":{"input_tokens":20,"output_tokens":10}}}`,
	}
	jsonlPath := createTestJSONL(t, dir, "hash1", "s1", lines)

	meta, err := parseJSONLToSessionMeta(jsonlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// From 10:00:00 to 11:30:00 = 90 minutes.
	if meta.DurationMinutes != 90 {
		t.Errorf("DurationMinutes = %d, want 90", meta.DurationMinutes)
	}
}

func TestParseJSONLToSessionMeta_FallbackIDs(t *testing.T) {
	dir := t.TempDir()
	// JSONL entries without sessionId or cwd — IDs should fall back to filename/dirname.
	lines := []string{
		`{"type":"user","timestamp":"2026-01-15T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`,
		`{"type":"assistant","timestamp":"2026-01-15T10:01:00Z","message":{"role":"assistant","content":[],"usage":{"input_tokens":5,"output_tokens":3}}}`,
	}
	jsonlPath := createTestJSONL(t, dir, "myhash", "mysession", lines)

	meta, err := parseJSONLToSessionMeta(jsonlPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.SessionID != "mysession" {
		t.Errorf("SessionID = %q, want %q (fallback from filename)", meta.SessionID, "mysession")
	}
	if meta.ProjectPath != "myhash" {
		t.Errorf("ProjectPath = %q, want %q (fallback from dirname)", meta.ProjectPath, "myhash")
	}
}

// ---------- ParseSessionMeta tests (existing, kept) ----------

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
