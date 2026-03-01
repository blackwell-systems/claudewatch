package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// callSuggestions is a convenience wrapper that calls handleGetSuggestions
// and type-asserts to SuggestionsResult.
func callSuggestions(t *testing.T, s *Server, args json.RawMessage) SuggestionsResult {
	t.Helper()
	result, err := s.handleGetSuggestions(args)
	if err != nil {
		t.Fatalf("handleGetSuggestions error: %v", err)
	}
	r, ok := result.(SuggestionsResult)
	if !ok {
		t.Fatalf("expected SuggestionsResult, got %T", result)
	}
	return r
}

// TestGetSuggestions_EmptyData verifies that no sessions/facets returns
// suggestions without error, with a non-nil slice and consistent TotalCount.
// Note: some rules (e.g. HookGaps) fire even with zero sessions, so we
// only check structural invariants, not that the count is exactly zero.
func TestGetSuggestions_EmptyData(t *testing.T) {
	dir := t.TempDir()
	s := &Server{claudeHome: dir}

	r := callSuggestions(t, s, json.RawMessage(`{}`))

	if r.Suggestions == nil {
		t.Error("Suggestions must be non-nil (expected slice, got nil)")
	}
	// TotalCount must equal total available suggestions (>= items returned after limit).
	if r.TotalCount < len(r.Suggestions) {
		t.Errorf("TotalCount (%d) < len(Suggestions) (%d), inconsistent", r.TotalCount, len(r.Suggestions))
	}
}

// TestGetSuggestions_DefaultLimit verifies that when more than 5 suggestions are
// available the default limit of 5 is applied.
func TestGetSuggestions_DefaultLimit(t *testing.T) {
	dir := t.TempDir()

	// Create 10 distinct projects, each without CLAUDE.md so MissingClaudeMD
	// fires for all of them — giving us at least 10 suggestions.
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("sess-%02d", i)
		projPath := fmt.Sprintf("/home/user/project%02d", i)
		writeSessionMeta(t, dir, id, "2026-01-15T10:00:00Z", projPath, 1000, 500)
	}

	s := &Server{claudeHome: dir}
	r := callSuggestions(t, s, json.RawMessage(`{}`))

	if len(r.Suggestions) != defaultSuggestLimit {
		t.Errorf("len(Suggestions) = %d, want %d (default limit)", len(r.Suggestions), defaultSuggestLimit)
	}
	if r.TotalCount < defaultSuggestLimit {
		t.Errorf("TotalCount = %d, should be >= %d", r.TotalCount, defaultSuggestLimit)
	}
}

// TestGetSuggestions_CustomLimit verifies that passing limit=2 returns at most 2 items.
func TestGetSuggestions_CustomLimit(t *testing.T) {
	dir := t.TempDir()

	// Create 8 distinct projects without CLAUDE.md.
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("sess-%02d", i)
		projPath := fmt.Sprintf("/home/user/project%02d", i)
		writeSessionMeta(t, dir, id, "2026-01-15T10:00:00Z", projPath, 1000, 500)
	}

	s := &Server{claudeHome: dir}
	r := callSuggestions(t, s, json.RawMessage(`{"limit":2}`))

	if len(r.Suggestions) > 2 {
		t.Errorf("len(Suggestions) = %d, want <= 2", len(r.Suggestions))
	}
}

// TestGetSuggestions_ProjectFilter verifies that the project filter reduces results
// to project-specific items.
func TestGetSuggestions_ProjectFilter(t *testing.T) {
	dir := t.TempDir()

	// Create sessions for two different projects.
	writeSessionMeta(t, dir, "sess-alpha", "2026-01-15T10:00:00Z", "/home/user/alpha", 1000, 500)
	writeSessionMeta(t, dir, "sess-beta", "2026-01-15T11:00:00Z", "/home/user/beta", 1000, 500)

	s := &Server{claudeHome: dir}
	// Filter for "alpha" — should only get suggestions mentioning alpha.
	r := callSuggestions(t, s, json.RawMessage(`{"project":"alpha","limit":20}`))

	// Every returned suggestion must mention "alpha" in title or description.
	for _, item := range r.Suggestions {
		if !strings.Contains(item.Title, "alpha") && !strings.Contains(item.Description, "alpha") {
			t.Errorf("suggestion %q does not mention project 'alpha'", item.Title)
		}
	}
}

// TestGetSuggestions_SortedByImpact verifies that suggestions are returned in
// descending impact order.
func TestGetSuggestions_SortedByImpact(t *testing.T) {
	dir := t.TempDir()

	// Create several distinct projects without CLAUDE.md to generate multiple suggestions.
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("sess-%02d", i)
		projPath := fmt.Sprintf("/home/user/project%02d", i)
		writeSessionMeta(t, dir, id, "2026-01-15T10:00:00Z", projPath, 1000, 500)
	}

	s := &Server{claudeHome: dir}
	r := callSuggestions(t, s, json.RawMessage(`{"limit":20}`))

	for i := 1; i < len(r.Suggestions); i++ {
		if r.Suggestions[i].ImpactScore > r.Suggestions[i-1].ImpactScore {
			t.Errorf(
				"suggestions not sorted by impact: index %d (%.4f) > index %d (%.4f)",
				i, r.Suggestions[i].ImpactScore,
				i-1, r.Suggestions[i-1].ImpactScore,
			)
		}
	}
}

// TestGetSuggestions_MissingClaudeMD verifies that a session with no CLAUDE.md
// in the project directory generates a MissingClaudeMD suggestion.
func TestGetSuggestions_MissingClaudeMD(t *testing.T) {
	dir := t.TempDir()

	// Create a session pointing to a temp project dir that has no CLAUDE.md.
	projDir := filepath.Join(dir, "myproject")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir projDir: %v", err)
	}
	// Deliberately do NOT create CLAUDE.md in projDir.

	writeSessionMeta(t, dir, "sess-noclaudemd", "2026-01-15T10:00:00Z", projDir, 1000, 500)

	s := &Server{claudeHome: dir}
	r := callSuggestions(t, s, json.RawMessage(`{"limit":20}`))

	// There should be at least one suggestion about missing CLAUDE.md.
	found := false
	for _, item := range r.Suggestions {
		if strings.Contains(item.Title, "CLAUDE.md") || strings.Contains(item.Description, "CLAUDE.md") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a CLAUDE.md suggestion for project without CLAUDE.md, got %d suggestions: %v",
			len(r.Suggestions), r.Suggestions)
	}
}
