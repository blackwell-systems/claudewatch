package analyzer

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

func TestClassifySession_Exploration(t *testing.T) {
	s := claude.SessionMeta{
		SessionID:  "s1",
		ToolCounts: map[string]int{"Read": 20, "Grep": 10, "Glob": 5, "Edit": 3},
		GitCommits: 0,
	}
	sc := classifySession(s)
	if sc.Intent != IntentExploration {
		t.Errorf("expected exploration, got %q", sc.Intent)
	}
	if sc.ReadRatio < 0.9 {
		t.Errorf("expected high read ratio, got %.2f", sc.ReadRatio)
	}
}

func TestClassifySession_Implementation(t *testing.T) {
	s := claude.SessionMeta{
		SessionID:  "s2",
		ToolCounts: map[string]int{"Edit": 25, "Write": 10, "Read": 5},
		GitCommits: 4,
	}
	sc := classifySession(s)
	if sc.Intent != IntentImplementation {
		t.Errorf("expected implementation, got %q", sc.Intent)
	}
}

func TestClassifySession_Mixed(t *testing.T) {
	s := claude.SessionMeta{
		SessionID:  "s3",
		ToolCounts: map[string]int{"Read": 10, "Edit": 10, "Bash": 10},
		GitCommits: 2,
	}
	sc := classifySession(s)
	if sc.Intent != IntentMixed {
		t.Errorf("expected mixed, got %q", sc.Intent)
	}
}

func TestClassifySession_NoTools(t *testing.T) {
	s := claude.SessionMeta{SessionID: "s4"}
	sc := classifySession(s)
	if sc.TotalTools != 0 {
		t.Errorf("expected 0 total tools, got %d", sc.TotalTools)
	}
}

func TestAnalyzeConfidence_Empty(t *testing.T) {
	result := AnalyzeConfidence(nil)
	if len(result.Projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(result.Projects))
	}
}

func TestAnalyzeConfidence_HighConfidence(t *testing.T) {
	// Project with implementation-heavy sessions and lots of commits.
	sessions := []claude.SessionMeta{
		{
			SessionID: "s1", ProjectPath: "/proj/confident",
			ToolCounts: map[string]int{"Edit": 30, "Write": 10, "Read": 5},
			GitCommits: 5,
		},
		{
			SessionID: "s2", ProjectPath: "/proj/confident",
			ToolCounts: map[string]int{"Edit": 25, "Write": 8, "Read": 7},
			GitCommits: 4,
		},
		{
			SessionID: "s3", ProjectPath: "/proj/confident",
			ToolCounts: map[string]int{"Edit": 20, "Write": 12, "Read": 3, "Bash": 5},
			GitCommits: 3,
		},
	}

	result := AnalyzeConfidence(sessions)
	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}

	pc := result.Projects[0]
	if pc.ConfidenceScore < 60 {
		t.Errorf("expected high confidence (>=60), got %.0f", pc.ConfidenceScore)
	}
	if pc.ImplementationRate < 0.5 {
		t.Errorf("expected high implementation rate, got %.2f", pc.ImplementationRate)
	}
}

func TestAnalyzeConfidence_LowConfidence(t *testing.T) {
	// Project where Claude just reads and reads with no commits.
	sessions := []claude.SessionMeta{
		{
			SessionID: "s1", ProjectPath: "/proj/lost",
			ToolCounts: map[string]int{"Read": 30, "Grep": 15, "Glob": 10, "Edit": 2},
			GitCommits: 0,
		},
		{
			SessionID: "s2", ProjectPath: "/proj/lost",
			ToolCounts: map[string]int{"Read": 25, "Grep": 10, "Glob": 8},
			GitCommits: 0,
		},
		{
			SessionID: "s3", ProjectPath: "/proj/lost",
			ToolCounts: map[string]int{"Read": 20, "Grep": 12, "Edit": 1},
			GitCommits: 0,
		},
	}

	result := AnalyzeConfidence(sessions)
	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}

	pc := result.Projects[0]
	if pc.ConfidenceScore > 30 {
		t.Errorf("expected low confidence (<=30), got %.0f", pc.ConfidenceScore)
	}
	if pc.ExplorationRate < 0.9 {
		t.Errorf("expected high exploration rate, got %.2f", pc.ExplorationRate)
	}
	if pc.Signal != "low confidence — Claude spends most time reading, CLAUDE.md may need more context" {
		t.Errorf("unexpected signal: %q", pc.Signal)
	}
	if result.LowConfidenceCount != 1 {
		t.Errorf("expected 1 low-confidence project, got %d", result.LowConfidenceCount)
	}
}

func TestAnalyzeConfidence_MultipleProjects(t *testing.T) {
	sessions := []claude.SessionMeta{
		// High-confidence project.
		{SessionID: "s1", ProjectPath: "/proj/a", ToolCounts: map[string]int{"Edit": 20, "Read": 5}, GitCommits: 3},
		{SessionID: "s2", ProjectPath: "/proj/a", ToolCounts: map[string]int{"Edit": 15, "Write": 5, "Read": 3}, GitCommits: 4},
		// Low-confidence project.
		{SessionID: "s3", ProjectPath: "/proj/b", ToolCounts: map[string]int{"Read": 30, "Grep": 10}, GitCommits: 0},
		{SessionID: "s4", ProjectPath: "/proj/b", ToolCounts: map[string]int{"Read": 25, "Glob": 8}, GitCommits: 0},
	}

	result := AnalyzeConfidence(sessions)
	if len(result.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(result.Projects))
	}

	// Sorted by confidence ascending — low-confidence project first.
	if result.Projects[0].ProjectName != "b" {
		t.Errorf("expected 'b' first (lowest confidence), got %q", result.Projects[0].ProjectName)
	}
	if result.Projects[1].ProjectName != "a" {
		t.Errorf("expected 'a' second (highest confidence), got %q", result.Projects[1].ProjectName)
	}

	// b should score lower than a.
	if result.Projects[0].ConfidenceScore >= result.Projects[1].ConfidenceScore {
		t.Errorf("expected b (%.0f) < a (%.0f)",
			result.Projects[0].ConfidenceScore, result.Projects[1].ConfidenceScore)
	}
}

func TestAnalyzeConfidence_SingleSessionProject(t *testing.T) {
	// Only one session — should be excluded (need 2+ for signal).
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/proj/single", ToolCounts: map[string]int{"Read": 10}, GitCommits: 0},
	}

	result := AnalyzeConfidence(sessions)
	if len(result.Projects) != 0 {
		t.Errorf("expected 0 projects for single-session, got %d", len(result.Projects))
	}
}

func TestAnalyzeConfidence_ExplorationWithCommits(t *testing.T) {
	// Read-heavy but still producing commits — moderate confidence.
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/proj/careful", ToolCounts: map[string]int{"Read": 20, "Grep": 10, "Edit": 5}, GitCommits: 2},
		{SessionID: "s2", ProjectPath: "/proj/careful", ToolCounts: map[string]int{"Read": 15, "Grep": 8, "Edit": 4, "Write": 2}, GitCommits: 3},
	}

	result := AnalyzeConfidence(sessions)
	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}

	pc := result.Projects[0]
	// Should be moderate — reads a lot but still commits.
	if pc.ConfidenceScore < 20 || pc.ConfidenceScore > 60 {
		t.Errorf("expected moderate confidence (20-60), got %.0f", pc.ConfidenceScore)
	}
}
