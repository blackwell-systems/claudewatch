package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// writeJSONL writes a slice of objects as JSONL to the given path.
func writeJSONL(t *testing.T, path string, entries []any) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("failed to encode entry: %v", err)
		}
	}
}

// makeEntry builds a transcript JSONL entry for a user or assistant message.
func makeEntry(entryType string, role string, text string, timestamp string) map[string]any {
	entry := map[string]any{
		"type":      entryType,
		"timestamp": timestamp,
		"message": map[string]any{
			"role": role,
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
		},
	}
	return entry
}

// makeToolUseEntry builds an assistant entry with a tool_use content block.
func makeToolUseEntry(toolName string, input map[string]any, timestamp string) map[string]any {
	inputBytes, _ := json.Marshal(input)
	return map[string]any{
		"type":      "assistant",
		"timestamp": timestamp,
		"message": map[string]any{
			"role": "assistant",
			"content": []map[string]any{
				{"type": "tool_use", "name": toolName, "input": json.RawMessage(inputBytes)},
			},
		},
	}
}

func setupTranscriptDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	projectsDir := filepath.Join(dir, "projects", "abc123")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("failed to create projects dir: %v", err)
	}
	return dir
}

func TestAnalyzeConversations_Empty(t *testing.T) {
	dir := t.TempDir()
	// No projects directory at all.
	result, err := AnalyzeConversations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(result.Sessions))
	}
}

func TestAnalyzeConversations_MissingProjectsDir(t *testing.T) {
	dir := t.TempDir()
	result, err := AnalyzeConversations(dir)
	if err != nil {
		t.Fatalf("unexpected error for missing projects dir: %v", err)
	}
	if len(result.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(result.Sessions))
	}
}

func TestAnalyzeConversations_CorrectionDetection(t *testing.T) {
	dir := setupTranscriptDir(t)
	projectDir := filepath.Join(dir, "projects", "abc123")

	entries := []any{
		makeEntry("user", "human", "fix the login page", "2026-01-15T10:00:00Z"),
		makeEntry("assistant", "assistant", "I will fix it now.", "2026-01-15T10:00:05Z"),
		makeEntry("user", "human", "no, that's not what I meant", "2026-01-15T10:01:00Z"),
		makeEntry("assistant", "assistant", "Let me try again.", "2026-01-15T10:01:05Z"),
		makeEntry("user", "human", "revert that change please", "2026-01-15T10:02:00Z"),
		makeEntry("assistant", "assistant", "Reverted.", "2026-01-15T10:02:05Z"),
		makeEntry("user", "human", "I meant the other file", "2026-01-15T10:03:00Z"),
		makeEntry("assistant", "assistant", "Got it.", "2026-01-15T10:03:05Z"),
		makeEntry("user", "human", "wrong approach, try again", "2026-01-15T10:04:00Z"),
		makeEntry("assistant", "assistant", "Trying a different way.", "2026-01-15T10:04:05Z"),
	}
	writeJSONL(t, filepath.Join(projectDir, "session1.jsonl"), entries)

	result, err := AnalyzeConversations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result.Sessions))
	}

	session := result.Sessions[0]
	if session.UserMessages != 5 {
		t.Errorf("expected 5 user messages, got %d", session.UserMessages)
	}

	// "no, that's not what I meant" matches "no," and "i meant"
	// "revert that change" matches "revert"
	// "I meant the other file" matches "i meant"
	// "wrong approach" matches "wrong"
	if session.CorrectionCount != 4 {
		t.Errorf("expected 4 corrections, got %d", session.CorrectionCount)
	}

	expectedRate := 4.0 / 5.0
	if diff := session.CorrectionRate - expectedRate; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected correction rate ~%.2f, got %.2f", expectedRate, session.CorrectionRate)
	}
}

func TestAnalyzeConversations_LongMessageDetection(t *testing.T) {
	dir := setupTranscriptDir(t)
	projectDir := filepath.Join(dir, "projects", "abc123")

	longMsg := strings.Repeat("a", 501) // > 500 chars
	shortMsg := "short message"

	entries := []any{
		makeEntry("user", "human", longMsg, "2026-01-15T10:00:00Z"),
		makeEntry("assistant", "assistant", "ok", "2026-01-15T10:00:05Z"),
		makeEntry("user", "human", shortMsg, "2026-01-15T10:01:00Z"),
		makeEntry("assistant", "assistant", "ok", "2026-01-15T10:01:05Z"),
		makeEntry("user", "human", longMsg, "2026-01-15T10:02:00Z"),
		makeEntry("assistant", "assistant", "ok", "2026-01-15T10:02:05Z"),
	}
	writeJSONL(t, filepath.Join(projectDir, "session2.jsonl"), entries)

	result, err := AnalyzeConversations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result.Sessions))
	}

	session := result.Sessions[0]
	if session.LongMessageCount != 2 {
		t.Errorf("expected 2 long messages, got %d", session.LongMessageCount)
	}

	expectedRate := 2.0 / 3.0
	if diff := session.LongMessageRate - expectedRate; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected long message rate ~%.2f, got %.2f", expectedRate, session.LongMessageRate)
	}
}

func TestAnalyzeConversations_MalformedJSONL(t *testing.T) {
	dir := setupTranscriptDir(t)
	projectDir := filepath.Join(dir, "projects", "abc123")

	// Write a file with some valid and some invalid lines.
	content := `{"type":"user","timestamp":"2026-01-15T10:00:00Z","message":{"role":"human","content":[{"type":"text","text":"hello"}]}}
not valid json at all
{"type":"assistant","timestamp":"2026-01-15T10:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}
`
	if err := os.WriteFile(filepath.Join(projectDir, "session5.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	result, err := AnalyzeConversations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still process the valid lines.
	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result.Sessions))
	}
	if result.Sessions[0].TotalMessages != 2 {
		t.Errorf("expected 2 total messages (skipping malformed line), got %d", result.Sessions[0].TotalMessages)
	}
}

func TestAnalyzeConversations_AggregateMetrics(t *testing.T) {
	dir := setupTranscriptDir(t)
	projectDir := filepath.Join(dir, "projects", "abc123")

	// Session 1: has corrections, no commits.
	entries1 := []any{
		makeEntry("user", "human", "do something", "2026-01-15T10:00:00Z"),
		makeEntry("assistant", "assistant", "ok", "2026-01-15T10:00:05Z"),
		makeEntry("user", "human", "no, revert that", "2026-01-15T10:01:00Z"),
		makeEntry("assistant", "assistant", "reverted", "2026-01-15T10:01:05Z"),
	}
	writeJSONL(t, filepath.Join(projectDir, "sess_a.jsonl"), entries1)

	// Session 2: no corrections, has commit.
	entries2 := []any{
		makeEntry("user", "human", "add a button", "2026-01-15T11:00:00Z"),
		makeEntry("assistant", "assistant", "done", "2026-01-15T11:00:05Z"),
		makeToolUseEntry("Bash", map[string]any{"command": "git commit -m 'add button'"}, "2026-01-15T11:00:10Z"),
	}
	writeJSONL(t, filepath.Join(projectDir, "sess_b.jsonl"), entries2)

	result, err := AnalyzeConversations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(result.Sessions))
	}

}

func TestIsCorrection(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"starts with no comma", "no, do it differently", true},
		{"contains revert", "please revert that change", true},
		{"contains wrong", "that's wrong", true},
		{"contains I meant", "I meant the other function", true},
		{"contains undo", "undo that", true},
		{"contains try again", "try again with a different approach", true},
		{"contains go back", "go back to the previous version", true},
		{"normal message", "looks good, ship it", false},
		{"empty", "", false},
		{"case insensitive", "REVERT the change", true},
		{"contains stop", "stop doing that", true},
		{"dont", "dont change that file", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCorrection(tt.text)
			if got != tt.want {
				t.Errorf("isCorrection(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseTimestamp_FromConversation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		zero  bool
	}{
		{"RFC3339", "2026-01-15T10:00:00Z", false},
		{"RFC3339Nano", "2026-01-15T10:00:00.123456Z", false},
		{"empty", "", true},
		{"invalid", "not-a-timestamp", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := claude.ParseTimestamp(tt.input)
			if tt.zero && !result.IsZero() {
				t.Errorf("expected zero time for %q, got %v", tt.input, result)
			}
			if !tt.zero && result.IsZero() {
				t.Errorf("expected non-zero time for %q", tt.input)
			}
		})
	}
}

func TestAnalyzeConversations_ProjectHash(t *testing.T) {
	dir := setupTranscriptDir(t)
	projectDir := filepath.Join(dir, "projects", "abc123")

	entries := []any{
		makeEntry("user", "human", "hello", "2026-01-15T10:00:00Z"),
		makeEntry("assistant", "assistant", "hi", "2026-01-15T10:00:05Z"),
	}
	writeJSONL(t, filepath.Join(projectDir, "session_x.jsonl"), entries)

	result, err := AnalyzeConversations(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result.Sessions))
	}
	if result.Sessions[0].ProjectHash != "abc123" {
		t.Errorf("expected project hash 'abc123', got %q", result.Sessions[0].ProjectHash)
	}
	if result.Sessions[0].SessionID != "session_x" {
		t.Errorf("expected session ID 'session_x', got %q", result.Sessions[0].SessionID)
	}
}
