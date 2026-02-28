package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseVersionedFilename(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		wantHash    string
		wantVersion int
	}{
		{"v1", "abc123@v1", "abc123", 1},
		{"v25", "01601367db25a77e@v25", "01601367db25a77e", 25},
		{"no version", "plainfile", "", 0},
		{"invalid version", "abc@vXYZ", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, version := parseVersionedFilename(tt.filename)
			if hash != tt.wantHash {
				t.Errorf("hash = %q, want %q", hash, tt.wantHash)
			}
			if version != tt.wantVersion {
				t.Errorf("version = %d, want %d", version, tt.wantVersion)
			}
		})
	}
}

func TestParseAllFileHistory(t *testing.T) {
	dir := t.TempDir()
	fhDir := filepath.Join(dir, "file-history", "session-abc")
	if err := os.MkdirAll(fhDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create versioned files.
	if err := os.WriteFile(filepath.Join(fhDir, "hash1@v1"), []byte("content v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fhDir, "hash1@v2"), []byte("content v2 longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fhDir, "hash2@v1"), []byte("other file"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := ParseAllFileHistory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results))
	}

	r := results[0]
	if r.SessionID != "session-abc" {
		t.Errorf("sessionID = %q, want session-abc", r.SessionID)
	}
	if r.UniqueFiles != 2 {
		t.Errorf("uniqueFiles = %d, want 2", r.UniqueFiles)
	}
	if r.TotalEdits != 3 {
		t.Errorf("totalEdits = %d, want 3", r.TotalEdits)
	}
	if r.MaxVersion != 2 {
		t.Errorf("maxVersion = %d, want 2", r.MaxVersion)
	}
	if r.TotalBytes == 0 {
		t.Error("totalBytes should be > 0")
	}
}

func TestParseAllFileHistory_MissingDir(t *testing.T) {
	results, err := ParseAllFileHistory("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestParseAllFileHistory_EmptySession(t *testing.T) {
	dir := t.TempDir()
	fhDir := filepath.Join(dir, "file-history", "empty-session")
	if err := os.MkdirAll(fhDir, 0o755); err != nil {
		t.Fatal(err)
	}

	results, err := ParseAllFileHistory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty session dir, got %d", len(results))
	}
}
