package mcp

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGetStalePatterns_NoSessions verifies that an empty dir returns empty Patterns with no error.
func TestGetStalePatterns_NoSessions(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)

	result, err := s.handleGetStalePatterns(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(StalePatternsResult)
	if !ok {
		t.Fatalf("expected StalePatternsResult, got %T", result)
	}

	if r.Patterns == nil {
		t.Error("Patterns must not be nil (want empty slice)")
	}
	if len(r.Patterns) != 0 {
		t.Errorf("len(Patterns) = %d, want 0", len(r.Patterns))
	}
	if r.TotalSessions != 0 {
		t.Errorf("TotalSessions = %d, want 0", r.TotalSessions)
	}
	if r.WindowSessions != 0 {
		t.Errorf("WindowSessions = %d, want 0", r.WindowSessions)
	}
}

// TestGetStalePatterns_DefaultParams verifies that omitting args uses threshold=0.3 and lookback=10.
func TestGetStalePatterns_DefaultParams(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(dir, 0)

	result, err := s.handleGetStalePatterns(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(StalePatternsResult)
	if !ok {
		t.Fatalf("expected StalePatternsResult, got %T", result)
	}

	if r.Threshold != 0.3 {
		t.Errorf("Threshold = %f, want 0.3", r.Threshold)
	}
	if r.ClaudeMDLookback != 10 {
		t.Errorf("ClaudeMDLookback = %d, want 10", r.ClaudeMDLookback)
	}
}

// TestGetStalePatterns_RecurrenceRate verifies that 2 of 3 sessions with "wrong_approach" → rate=0.667.
func TestGetStalePatterns_RecurrenceRate(t *testing.T) {
	dir := t.TempDir()

	writeSessionMeta(t, dir, "sess-1", "2026-01-03T10:00:00Z", "/proj/a", 1000, 500)
	writeSessionMeta(t, dir, "sess-2", "2026-01-02T10:00:00Z", "/proj/a", 1000, 500)
	writeSessionMeta(t, dir, "sess-3", "2026-01-01T10:00:00Z", "/proj/a", 1000, 500)

	writeFacet(t, dir, "sess-1", map[string]int{"wrong_approach": 2})
	writeFacet(t, dir, "sess-2", map[string]int{"wrong_approach": 1})
	// sess-3 has no friction

	s := newTestServer(dir, 0)
	result, err := s.handleGetStalePatterns(json.RawMessage(`{"lookback":10}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(StalePatternsResult)
	if !ok {
		t.Fatalf("expected StalePatternsResult, got %T", result)
	}

	if r.WindowSessions != 3 {
		t.Errorf("WindowSessions = %d, want 3", r.WindowSessions)
	}

	found := false
	for _, p := range r.Patterns {
		if p.FrictionType == "wrong_approach" {
			found = true
			want := 2.0 / 3.0
			if math.Abs(p.RecurrenceRate-want) > 0.001 {
				t.Errorf("RecurrenceRate = %f, want ~%f", p.RecurrenceRate, want)
			}
			if p.SessionCount != 2 {
				t.Errorf("SessionCount = %d, want 2", p.SessionCount)
			}
		}
	}
	if !found {
		t.Error("expected 'wrong_approach' pattern, not found")
	}
}

// TestGetStalePatterns_IsStale verifies that recurrenceRate > threshold AND no CLAUDE.md → IsStale: true.
func TestGetStalePatterns_IsStale(t *testing.T) {
	dir := t.TempDir()

	// Project path with no CLAUDE.md.
	projPath := filepath.Join(dir, "project_no_claudemd")
	if err := os.MkdirAll(projPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeSessionMeta(t, dir, "sess-1", "2026-01-03T10:00:00Z", projPath, 1000, 500)
	writeSessionMeta(t, dir, "sess-2", "2026-01-02T10:00:00Z", projPath, 1000, 500)
	writeSessionMeta(t, dir, "sess-3", "2026-01-01T10:00:00Z", projPath, 1000, 500)

	// All 3 sessions have "wrong_approach" → recurrenceRate = 1.0 > 0.3 threshold.
	writeFacet(t, dir, "sess-1", map[string]int{"wrong_approach": 1})
	writeFacet(t, dir, "sess-2", map[string]int{"wrong_approach": 1})
	writeFacet(t, dir, "sess-3", map[string]int{"wrong_approach": 1})

	s := newTestServer(dir, 0)
	result, err := s.handleGetStalePatterns(json.RawMessage(`{"threshold":0.3,"lookback":10}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(StalePatternsResult)
	if !ok {
		t.Fatalf("expected StalePatternsResult, got %T", result)
	}

	found := false
	for _, p := range r.Patterns {
		if p.FrictionType == "wrong_approach" {
			found = true
			if !p.IsStale {
				t.Errorf("IsStale = false, want true (recurrenceRate=%f, threshold=%f, no CLAUDE.md)", p.RecurrenceRate, r.Threshold)
			}
		}
	}
	if !found {
		t.Error("expected 'wrong_approach' pattern, not found")
	}
}

// TestGetStalePatterns_NotStaleWithRecentClaudeMD verifies that a recent CLAUDE.md update → IsStale: false.
func TestGetStalePatterns_NotStaleWithRecentClaudeMD(t *testing.T) {
	dir := t.TempDir()

	projPath := filepath.Join(dir, "project_with_claudemd")
	if err := os.MkdirAll(projPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	writeSessionMeta(t, dir, "sess-1", "2026-01-03T10:00:00Z", projPath, 1000, 500)
	writeSessionMeta(t, dir, "sess-2", "2026-01-02T10:00:00Z", projPath, 1000, 500)
	writeSessionMeta(t, dir, "sess-3", "2026-01-01T10:00:00Z", projPath, 1000, 500)

	writeFacet(t, dir, "sess-1", map[string]int{"wrong_approach": 1})
	writeFacet(t, dir, "sess-2", map[string]int{"wrong_approach": 1})
	writeFacet(t, dir, "sess-3", map[string]int{"wrong_approach": 1})

	// Write CLAUDE.md with a modification time AFTER the oldest session (2026-01-01).
	claudeMDPath := filepath.Join(projPath, "CLAUDE.md")
	if err := os.WriteFile(claudeMDPath, []byte("# CLAUDE.md"), 0644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}
	// Set mtime to 2026-01-02 (after oldest session 2026-01-01).
	recentTime := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(claudeMDPath, recentTime, recentTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	s := newTestServer(dir, 0)
	result, err := s.handleGetStalePatterns(json.RawMessage(`{"threshold":0.3,"lookback":10}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(StalePatternsResult)
	if !ok {
		t.Fatalf("expected StalePatternsResult, got %T", result)
	}

	found := false
	for _, p := range r.Patterns {
		if p.FrictionType == "wrong_approach" {
			found = true
			if p.IsStale {
				t.Errorf("IsStale = true, want false (CLAUDE.md modified after oldest window session)")
			}
		}
	}
	if !found {
		t.Error("expected 'wrong_approach' pattern, not found")
	}
}

// TestGetStalePatterns_SortedByRecurrence verifies patterns are sorted descending by RecurrenceRate.
func TestGetStalePatterns_SortedByRecurrence(t *testing.T) {
	dir := t.TempDir()

	writeSessionMeta(t, dir, "sess-1", "2026-01-03T10:00:00Z", "/proj/a", 1000, 500)
	writeSessionMeta(t, dir, "sess-2", "2026-01-02T10:00:00Z", "/proj/a", 1000, 500)
	writeSessionMeta(t, dir, "sess-3", "2026-01-01T10:00:00Z", "/proj/a", 1000, 500)

	// "type_b" appears in 3 sessions (rate=1.0), "type_a" in 1 session (rate=0.333).
	writeFacet(t, dir, "sess-1", map[string]int{"type_a": 1, "type_b": 1})
	writeFacet(t, dir, "sess-2", map[string]int{"type_b": 1})
	writeFacet(t, dir, "sess-3", map[string]int{"type_b": 1})

	s := newTestServer(dir, 0)
	result, err := s.handleGetStalePatterns(json.RawMessage(`{"threshold":0.0,"lookback":10}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(StalePatternsResult)
	if !ok {
		t.Fatalf("expected StalePatternsResult, got %T", result)
	}

	if len(r.Patterns) < 2 {
		t.Fatalf("expected at least 2 patterns, got %d", len(r.Patterns))
	}

	// First pattern should have higher or equal recurrence rate.
	for i := 1; i < len(r.Patterns); i++ {
		if r.Patterns[i].RecurrenceRate > r.Patterns[i-1].RecurrenceRate {
			t.Errorf("patterns not sorted: patterns[%d].RecurrenceRate=%f > patterns[%d].RecurrenceRate=%f",
				i, r.Patterns[i].RecurrenceRate, i-1, r.Patterns[i-1].RecurrenceRate)
		}
	}

	// Verify type_b is first.
	if r.Patterns[0].FrictionType != "type_b" {
		t.Errorf("first pattern = %q, want %q", r.Patterns[0].FrictionType, "type_b")
	}
}

// TestGetStalePatterns_LookbackLimit verifies that lookback=2 uses only the 2 most recent sessions.
func TestGetStalePatterns_LookbackLimit(t *testing.T) {
	dir := t.TempDir()

	// 3 sessions; only 2 most recent should be used with lookback=2.
	writeSessionMeta(t, dir, "sess-1", "2026-01-03T10:00:00Z", "/proj/a", 1000, 500)
	writeSessionMeta(t, dir, "sess-2", "2026-01-02T10:00:00Z", "/proj/a", 1000, 500)
	writeSessionMeta(t, dir, "sess-3", "2026-01-01T10:00:00Z", "/proj/a", 1000, 500)

	// Only sess-3 (oldest, outside window) has "old_friction".
	// sess-1 and sess-2 both have "new_friction".
	writeFacet(t, dir, "sess-1", map[string]int{"new_friction": 1})
	writeFacet(t, dir, "sess-2", map[string]int{"new_friction": 1})
	writeFacet(t, dir, "sess-3", map[string]int{"old_friction": 1})

	s := newTestServer(dir, 0)
	result, err := s.handleGetStalePatterns(json.RawMessage(`{"lookback":2}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, ok := result.(StalePatternsResult)
	if !ok {
		t.Fatalf("expected StalePatternsResult, got %T", result)
	}

	if r.WindowSessions != 2 {
		t.Errorf("WindowSessions = %d, want 2", r.WindowSessions)
	}

	// "old_friction" should NOT be present (it's only in sess-3, outside the window).
	for _, p := range r.Patterns {
		if p.FrictionType == "old_friction" {
			t.Error("'old_friction' should not be in results — it's outside the lookback window")
		}
	}

	// "new_friction" should be present with rate=1.0 (2 of 2 sessions).
	found := false
	for _, p := range r.Patterns {
		if p.FrictionType == "new_friction" {
			found = true
			if math.Abs(p.RecurrenceRate-1.0) > 0.001 {
				t.Errorf("RecurrenceRate = %f, want 1.0", p.RecurrenceRate)
			}
		}
	}
	if !found {
		t.Error("expected 'new_friction' pattern, not found")
	}
}
