package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseHistory_ValidEntries(t *testing.T) {
	dir := t.TempDir()
	data := `{"display":"help me refactor","timestamp":1700000000,"project":"/home/user/proj","sessionId":"s1"}
{"display":"fix the bug","timestamp":1700001000,"project":"/home/user/proj","sessionId":"s2"}
{"display":"write tests","timestamp":1700002000,"project":"/home/user/other","sessionId":"s3"}
`
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, err := ParseHistory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Display != "help me refactor" {
		t.Errorf("entries[0].Display = %q, want %q", entries[0].Display, "help me refactor")
	}
	if entries[1].SessionID != "s2" {
		t.Errorf("entries[1].SessionID = %q, want %q", entries[1].SessionID, "s2")
	}
	if entries[2].Timestamp != 1700002000 {
		t.Errorf("entries[2].Timestamp = %d, want 1700002000", entries[2].Timestamp)
	}
}

func TestParseHistory_MissingFile(t *testing.T) {
	dir := t.TempDir()
	entries, err := ParseHistory(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries, got %v", entries)
	}
}

func TestParseHistory_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(""), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, err := ParseHistory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseHistory_SkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	data := `{"display":"good line","timestamp":1700000000,"sessionId":"s1"}
not valid json
{"display":"another good","timestamp":1700001000,"sessionId":"s2"}
`
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, err := ParseHistory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (malformed skipped), got %d", len(entries))
	}
}

func TestParseHistory_SkipsEmptyLines(t *testing.T) {
	dir := t.TempDir()
	data := `{"display":"first","timestamp":1700000000,"sessionId":"s1"}

{"display":"second","timestamp":1700001000,"sessionId":"s2"}
`
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	entries, err := ParseHistory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestLatestSessionID_ReturnsLatest(t *testing.T) {
	dir := t.TempDir()
	data := `{"display":"old","timestamp":1700000000,"sessionId":"s-old"}
{"display":"newest","timestamp":1700009000,"sessionId":"s-newest"}
{"display":"middle","timestamp":1700005000,"sessionId":"s-middle"}
`
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	id, err := LatestSessionID(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "s-newest" {
		t.Errorf("LatestSessionID = %q, want %q", id, "s-newest")
	}
}

func TestLatestSessionID_EmptyHistory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(""), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	id, err := LatestSessionID(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

func TestLatestSessionID_MissingFile(t *testing.T) {
	dir := t.TempDir()
	id, err := LatestSessionID(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

func TestLatestSessionID_SingleEntry(t *testing.T) {
	dir := t.TempDir()
	data := `{"display":"only","timestamp":1700000000,"sessionId":"s-only"}`
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	id, err := LatestSessionID(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "s-only" {
		t.Errorf("LatestSessionID = %q, want %q", id, "s-only")
	}
}
