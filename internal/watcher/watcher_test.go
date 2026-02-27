package watcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshot_MissingDirectory(t *testing.T) {
	w := New("/nonexistent/path/to/claude", 5*time.Minute, nil)

	// ParseAllSessionMeta returns empty slice for missing dirs, so Snapshot
	// succeeds with zero sessions rather than returning an error.
	state, err := w.Snapshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.SessionCount != 0 {
		t.Errorf("expected 0 sessions, got %d", state.SessionCount)
	}
}

// createSessionMetaFile writes a minimal session-meta JSON file.
func createSessionMetaFile(t *testing.T, dir string, sessionID string, projectPath string, commits int, startTime string) {
	t.Helper()
	metaDir := filepath.Join(dir, "usage-data", "session-meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("failed to create session-meta dir: %v", err)
	}

	meta := map[string]any{
		"session_id":              sessionID,
		"project_path":            projectPath,
		"start_time":              startTime,
		"duration_minutes":        15,
		"user_message_count":      5,
		"assistant_message_count": 8,
		"tool_counts":             map[string]int{"Bash": 3, "Read": 5},
		"languages":               map[string]int{"Go": 4},
		"git_commits":             commits,
		"input_tokens":            1000,
		"output_tokens":           2000,
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("failed to marshal session meta: %v", err)
	}

	filePath := filepath.Join(metaDir, sessionID+".json")
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		t.Fatalf("failed to write session meta file: %v", err)
	}
}

func TestSnapshot_WithSyntheticData(t *testing.T) {
	dir := t.TempDir()

	createSessionMetaFile(t, dir, "session-1", "/tmp/project-a", 3, "2026-01-15T10:00:00Z")
	createSessionMetaFile(t, dir, "session-2", "/tmp/project-a", 0, "2026-01-16T10:00:00Z")
	createSessionMetaFile(t, dir, "session-3", "/tmp/project-b", 1, "2026-01-17T10:00:00Z")

	w := New(dir, 5*time.Minute, nil)
	state, err := w.Snapshot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if state.SessionCount != 3 {
		t.Errorf("expected 3 sessions, got %d", state.SessionCount)
	}
	if state.TotalSessions != 3 {
		t.Errorf("expected 3 total sessions, got %d", state.TotalSessions)
	}
	if state.ZeroCommitCount != 1 {
		t.Errorf("expected 1 zero-commit session, got %d", state.ZeroCommitCount)
	}
	if state.LastSessionID != "session-3" {
		t.Errorf("expected last session to be session-3, got %s", state.LastSessionID)
	}
	if state.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestCheck_ReturnsAlerts(t *testing.T) {
	dir := t.TempDir()

	// Initial snapshot with one session.
	createSessionMetaFile(t, dir, "session-1", "/tmp/project-a", 2, "2026-01-15T10:00:00Z")

	var received []Alert
	w := New(dir, 5*time.Minute, func(a Alert) {
		received = append(received, a)
	})

	// Take initial snapshot.
	initial, err := w.Snapshot()
	if err != nil {
		t.Fatalf("initial snapshot error: %v", err)
	}
	w.previous = initial

	// Add a new session.
	createSessionMetaFile(t, dir, "session-2", "/tmp/project-a", 1, "2026-01-16T10:00:00Z")

	// Check should detect the new session.
	alerts := w.Check()

	// Should have at least an info alert for new session.
	hasInfo := false
	for _, a := range alerts {
		if a.Level == "info" {
			hasInfo = true
		}
	}
	if !hasInfo {
		t.Error("expected at least one info alert for new session")
	}
}

func TestNew_SetsFields(t *testing.T) {
	called := false
	fn := func(a Alert) { called = true }

	w := New("/some/dir", 10*time.Minute, fn)

	if w.claudeDir != "/some/dir" {
		t.Errorf("expected claudeDir '/some/dir', got %q", w.claudeDir)
	}
	if w.interval != 10*time.Minute {
		t.Errorf("expected interval 10m, got %v", w.interval)
	}
	if w.alertFn == nil {
		t.Error("expected non-nil alertFn")
	}

	// Verify the function is the one we passed.
	w.alertFn(Alert{})
	if !called {
		t.Error("expected alertFn to be called")
	}
}
