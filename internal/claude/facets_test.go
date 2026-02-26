package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAllFacets_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	facetDir := filepath.Join(dir, "usage-data", "facets")
	if err := os.MkdirAll(facetDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	f1 := `{
		"session_id": "sess-a",
		"underlying_goal": "refactor authentication module",
		"goal_categories": {"refactoring": 1},
		"outcome": "success",
		"user_satisfaction_counts": {"satisfied": 1},
		"claude_helpfulness": "very helpful",
		"session_type": "coding",
		"friction_counts": {},
		"friction_detail": "",
		"primary_success": "completed refactoring",
		"brief_summary": "Refactored auth module to use JWT"
	}`
	f2 := `{
		"session_id": "sess-b",
		"underlying_goal": "debug memory leak",
		"goal_categories": {"debugging": 1},
		"outcome": "partial",
		"user_satisfaction_counts": {"neutral": 1},
		"claude_helpfulness": "somewhat helpful",
		"session_type": "debugging",
		"friction_counts": {"tool_error": 2},
		"friction_detail": "File read permissions failed twice",
		"primary_success": "identified leak source",
		"brief_summary": "Found memory leak in cache layer"
	}`

	if err := os.WriteFile(filepath.Join(facetDir, "sess-a.json"), []byte(f1), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(facetDir, "sess-b.json"), []byte(f2), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	facets, err := ParseAllFacets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facets) != 2 {
		t.Fatalf("expected 2 facets, got %d", len(facets))
	}

	found := map[string]SessionFacet{}
	for _, f := range facets {
		found[f.SessionID] = f
	}

	a, ok := found["sess-a"]
	if !ok {
		t.Fatal("missing facet for sess-a")
	}
	if a.UnderlyingGoal != "refactor authentication module" {
		t.Errorf("UnderlyingGoal = %q", a.UnderlyingGoal)
	}
	if a.Outcome != "success" {
		t.Errorf("Outcome = %q, want %q", a.Outcome, "success")
	}
	if a.SessionType != "coding" {
		t.Errorf("SessionType = %q, want %q", a.SessionType, "coding")
	}

	b, ok := found["sess-b"]
	if !ok {
		t.Fatal("missing facet for sess-b")
	}
	if b.FrictionCounts["tool_error"] != 2 {
		t.Errorf("FrictionCounts[tool_error] = %d, want 2", b.FrictionCounts["tool_error"])
	}
	if b.FrictionDetail != "File read permissions failed twice" {
		t.Errorf("FrictionDetail = %q", b.FrictionDetail)
	}
}

func TestParseAllFacets_MissingDir(t *testing.T) {
	dir := t.TempDir()
	facets, err := ParseAllFacets(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if facets != nil {
		t.Errorf("expected nil facets, got %v", facets)
	}
}

func TestParseAllFacets_SkipsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	facetDir := filepath.Join(dir, "usage-data", "facets")
	if err := os.MkdirAll(facetDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	valid := `{"session_id":"good-facet","outcome":"success"}`
	invalid := `{broken json`
	if err := os.WriteFile(filepath.Join(facetDir, "good.json"), []byte(valid), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(facetDir, "bad.json"), []byte(invalid), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	facets, err := ParseAllFacets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facets) != 1 {
		t.Fatalf("expected 1 facet (invalid skipped), got %d", len(facets))
	}
	if facets[0].SessionID != "good-facet" {
		t.Errorf("SessionID = %q, want %q", facets[0].SessionID, "good-facet")
	}
}

func TestParseAllFacets_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	facetDir := filepath.Join(dir, "usage-data", "facets")
	if err := os.MkdirAll(facetDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	facets, err := ParseAllFacets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facets) != 0 {
		t.Errorf("expected 0 facets, got %d", len(facets))
	}
}
