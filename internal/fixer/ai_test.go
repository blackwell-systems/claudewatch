package fixer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

func TestParseAIResponse_ValidJSON(t *testing.T) {
	response := `{
		"additions": [
			{
				"section": "## Build & Test",
				"content": "go build ./...",
				"reason": "High bash usage detected"
			},
			{
				"section": "## Conventions",
				"content": "- Do not plan",
				"reason": "Plan agents have high kill rate"
			}
		]
	}`

	additions, err := parseAIResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(additions) != 2 {
		t.Fatalf("expected 2 additions, got %d", len(additions))
	}
	if additions[0].Section != "## Build & Test" {
		t.Errorf("expected section '## Build & Test', got %q", additions[0].Section)
	}
	if additions[0].Content != "go build ./..." {
		t.Errorf("expected content 'go build ./...', got %q", additions[0].Content)
	}
	if additions[1].Section != "## Conventions" {
		t.Errorf("expected section '## Conventions', got %q", additions[1].Section)
	}
}

func TestParseAIResponse_JSONInCodeFences(t *testing.T) {
	response := "```json\n" + `{
		"additions": [
			{
				"section": "## Testing",
				"content": "npm test",
				"reason": "Tests found"
			}
		]
	}` + "\n```"

	additions, err := parseAIResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(additions) != 1 {
		t.Fatalf("expected 1 addition, got %d", len(additions))
	}
	if additions[0].Section != "## Testing" {
		t.Errorf("expected section '## Testing', got %q", additions[0].Section)
	}
}

func TestParseAIResponse_PlainCodeFences(t *testing.T) {
	response := "```\n" + `{
		"additions": [
			{
				"section": "## Architecture",
				"content": "Go project layout",
				"reason": "Large project"
			}
		]
	}` + "\n```"

	additions, err := parseAIResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(additions) != 1 {
		t.Fatalf("expected 1 addition, got %d", len(additions))
	}
}

func TestParseAIResponse_InvalidJSON(t *testing.T) {
	response := "this is not json at all"

	_, err := parseAIResponse(response)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parsing AI JSON response") {
		t.Errorf("expected parsing error, got: %v", err)
	}
}

func TestParseAIResponse_EmptyAdditions(t *testing.T) {
	response := `{"additions": []}`

	_, err := parseAIResponse(response)
	if err == nil {
		t.Fatal("expected error for empty additions")
	}
	if !strings.Contains(err.Error(), "no additions") {
		t.Errorf("expected 'no additions' error, got: %v", err)
	}
}

func TestParseAIResponse_SkipsEmptySections(t *testing.T) {
	response := `{
		"additions": [
			{"section": "", "content": "some content", "reason": "test"},
			{"section": "## Valid", "content": "", "reason": "test"},
			{"section": "## Good", "content": "real content", "reason": "test"}
		]
	}`

	additions, err := parseAIResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(additions) != 1 {
		t.Fatalf("expected 1 valid addition (others should be skipped), got %d", len(additions))
	}
	if additions[0].Section != "## Good" {
		t.Errorf("expected section '## Good', got %q", additions[0].Section)
	}
}

func TestBuildUserPrompt_ProducesNonEmptyString(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{
			Path:            "/tmp/test-project",
			Name:            "test-project",
			PrimaryLanguage: "Go",
			HasClaudeMD:     true,
			Score:           75,
		},
		Sessions: []claude.SessionMeta{
			{
				SessionID:             "s1",
				DurationMinutes:       30,
				UserMessageCount:      10,
				AssistantMessageCount: 15,
				ToolCounts:            map[string]int{"Bash": 5, "Read": 10},
				Languages:             map[string]int{"Go": 8},
				ToolErrors:            2,
			},
		},
		ExistingClaudeMD: "# Test Project\n\nSome content.",
		CommitAnalysis: &analyzer.CommitAnalysis{
			TotalSessions:        1,
			SessionsWithCommits:  1,
			ZeroCommitRate:       0,
			AvgCommitsPerSession: 2.0,
		},
		ToolProfile: &analyzer.ToolProfile{
			DominantTool:    "Read",
			BashRatio:       0.25,
			EditToReadRatio: 0.8,
		},
		ConversationData: &analyzer.ConversationAnalysis{
			AvgCorrectionRate:      0.15,
			AvgLongMsgRate:         0.10,
			HighCorrectionSessions: 0,
		},
	}

	prompt := buildUserPrompt(ctx)

	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}

	expectedSections := []string{
		"## Project Overview",
		"## Session Statistics",
		"## Commit Analysis",
		"## Tool Profile",
		"## Conversation Quality",
		"## Existing CLAUDE.md",
	}

	for _, section := range expectedSections {
		if !strings.Contains(prompt, section) {
			t.Errorf("expected prompt to contain %q", section)
		}
	}

	if !strings.Contains(prompt, "test-project") {
		t.Error("expected prompt to contain project name")
	}
	if !strings.Contains(prompt, "Go") {
		t.Error("expected prompt to contain primary language")
	}
}

func TestBuildUserPrompt_MinimalContext(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{
			Path: "/tmp/minimal",
			Name: "minimal",
		},
	}

	prompt := buildUserPrompt(ctx)
	if prompt == "" {
		t.Fatal("expected non-empty prompt even with minimal context")
	}
	if !strings.Contains(prompt, "## Project Overview") {
		t.Error("expected Project Overview section")
	}
	if !strings.Contains(prompt, "Total sessions: 0") {
		t.Error("expected 0 total sessions")
	}
}

func TestBuildUserPrompt_WithFrictionPatterns(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{
			Path: "/tmp/friction",
			Name: "friction",
		},
		FrictionPatterns: &analyzer.PersistenceAnalysis{
			StaleCount:     1,
			ImprovingCount: 2,
			WorseningCount: 0,
			Patterns: []analyzer.FrictionPersistence{
				{
					FrictionType:     "wrong_approach",
					Frequency:        0.45,
					WeeklyTrend:      "stable",
					ConsecutiveWeeks: 4,
					Stale:            true,
					OccurrenceCount:  8,
				},
			},
		},
	}

	prompt := buildUserPrompt(ctx)
	if !strings.Contains(prompt, "## Friction Patterns") {
		t.Error("expected Friction Patterns section")
	}
	if !strings.Contains(prompt, "wrong_approach") {
		t.Error("expected friction type in prompt")
	}
}

func TestBuildUserPrompt_WithAgentTasks(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{
			Path: "/tmp/agents",
			Name: "agents",
		},
		AgentTasks: []claude.AgentTask{
			{AgentType: "Plan", Status: "killed", DurationMs: 60000},
			{AgentType: "Plan", Status: "completed", DurationMs: 120000},
		},
	}

	prompt := buildUserPrompt(ctx)
	if !strings.Contains(prompt, "## Agent Tasks") {
		t.Error("expected Agent Tasks section")
	}
	if !strings.Contains(prompt, "Plan") {
		t.Error("expected Plan agent type in prompt")
	}
}

func TestScanProjectStructure_TempDir(t *testing.T) {
	dir := t.TempDir()

	// Create some known files and directories.
	if err := os.MkdirAll(filepath.Join(dir, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Hidden files should be skipped.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.o\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := scanProjectStructure(dir)

	if result == "" {
		t.Fatal("expected non-empty project structure")
	}
	if !strings.Contains(result, "cmd/ (directory)") {
		t.Error("expected cmd/ directory in output")
	}
	if !strings.Contains(result, "internal/ (directory)") {
		t.Error("expected internal/ directory in output")
	}
	if !strings.Contains(result, "main.go") {
		t.Error("expected main.go in output")
	}
	if !strings.Contains(result, "go.mod") {
		t.Error("expected go.mod content in output")
	}
	if strings.Contains(result, ".gitignore") {
		t.Error("expected hidden files to be excluded")
	}
}

func TestScanProjectStructure_NonexistentDir(t *testing.T) {
	result := scanProjectStructure("/nonexistent/path/that/does/not/exist")
	if result != "" {
		t.Errorf("expected empty string for nonexistent dir, got %q", result)
	}
}
