package fixer

import (
	"strings"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

func TestGenerateFix_NilContext(t *testing.T) {
	_, err := GenerateFix(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil context, got nil")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("expected error to mention nil, got: %s", err.Error())
	}
}

func TestGenerateFix_NilOptions(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{
			Path:  "/tmp/test-project",
			Name:  "test-project",
			Score: 50,
		},
	}

	fix, err := GenerateFix(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fix == nil {
		t.Fatal("expected non-nil fix")
	}
	if fix.ProjectPath != "/tmp/test-project" {
		t.Errorf("expected project path /tmp/test-project, got %s", fix.ProjectPath)
	}
	if fix.ProjectName != "test-project" {
		t.Errorf("expected project name test-project, got %s", fix.ProjectName)
	}
	if fix.CurrentScore != 50 {
		t.Errorf("expected score 50, got %d", fix.CurrentScore)
	}
}

func TestGenerateFix_EmptyContext(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{
			Path: "/tmp/empty",
			Name: "empty",
		},
	}

	fix, err := GenerateFix(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fix == nil {
		t.Fatal("expected non-nil fix")
	}
	// With empty context, no rules should trigger.
	if len(fix.Additions) != 0 {
		t.Errorf("expected 0 additions for empty context, got %d", len(fix.Additions))
	}
}

func TestMergeAdditions_SameSectionMerged(t *testing.T) {
	additions := []Addition{
		{
			Section:    "## Conventions",
			Content:    "Rule 1",
			Reason:     "Reason 1",
			Impact:     "Impact 1",
			Source:     "source_a",
			Confidence: 0.5,
		},
		{
			Section:    "## Conventions",
			Content:    "Rule 2",
			Reason:     "Reason 2",
			Impact:     "Impact 2",
			Source:     "source_b",
			Confidence: 0.9,
		},
	}

	merged := mergeAdditions(additions)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged addition, got %d", len(merged))
	}

	m := merged[0]
	if m.Section != "## Conventions" {
		t.Errorf("expected section '## Conventions', got %q", m.Section)
	}
	if !strings.Contains(m.Content, "Rule 1") || !strings.Contains(m.Content, "Rule 2") {
		t.Errorf("expected merged content to contain both rules, got %q", m.Content)
	}
	if !strings.Contains(m.Reason, "Reason 1") || !strings.Contains(m.Reason, "Reason 2") {
		t.Errorf("expected merged reason to contain both reasons, got %q", m.Reason)
	}
	if !strings.Contains(m.Source, "source_a") || !strings.Contains(m.Source, "source_b") {
		t.Errorf("expected merged source to contain both sources, got %q", m.Source)
	}
	if m.Confidence != 0.9 {
		t.Errorf("expected best confidence 0.9, got %f", m.Confidence)
	}
}

func TestMergeAdditions_DifferentSectionsPreserved(t *testing.T) {
	additions := []Addition{
		{Section: "## Build & Test", Content: "build stuff", Source: "a"},
		{Section: "## Conventions", Content: "convention stuff", Source: "b"},
	}

	merged := mergeAdditions(additions)
	if len(merged) != 2 {
		t.Fatalf("expected 2 additions, got %d", len(merged))
	}
	if merged[0].Section != "## Build & Test" {
		t.Errorf("expected first section to be Build & Test, got %q", merged[0].Section)
	}
	if merged[1].Section != "## Conventions" {
		t.Errorf("expected second section to be Conventions, got %q", merged[1].Section)
	}
}

func TestRenderMarkdown_NoExisting(t *testing.T) {
	fix := &ProposedFix{
		ProjectName: "my-project",
		Additions: []Addition{
			{
				Section: "## Build & Test",
				Content: "```bash\ngo build ./...\n```",
			},
		},
	}

	md := RenderMarkdown(fix, false)

	if !strings.HasPrefix(md, "# my-project") {
		t.Errorf("expected markdown to start with project header, got: %s", md[:min(50, len(md))])
	}
	if !strings.Contains(md, "Claude Code instructions") {
		t.Error("expected starter file text when no existing CLAUDE.md")
	}
	if !strings.Contains(md, "## Build & Test") {
		t.Error("expected Build & Test section in output")
	}
	if !strings.Contains(md, "go build") {
		t.Error("expected build command in output")
	}
}

func TestRenderMarkdown_WithExisting(t *testing.T) {
	fix := &ProposedFix{
		ProjectName: "my-project",
		Additions: []Addition{
			{
				Section: "## Conventions",
				Content: "- Do not plan",
			},
		},
	}

	md := RenderMarkdown(fix, true)

	if strings.HasPrefix(md, "# my-project") {
		t.Error("should not include project header when existing CLAUDE.md present")
	}
	if strings.Contains(md, "Claude Code instructions") {
		t.Error("should not include starter text when existing CLAUDE.md present")
	}
	if !strings.Contains(md, "## Conventions") {
		t.Error("expected Conventions section in output")
	}
}

func TestRenderMarkdown_NoAdditions(t *testing.T) {
	fix := &ProposedFix{
		ProjectName: "empty-project",
		Additions:   nil,
	}

	md := RenderMarkdown(fix, true)
	// With existing file and no additions, should be empty.
	if md != "" {
		t.Errorf("expected empty string for no additions with existing file, got %q", md)
	}
}

func TestGenerateFix_RulesProduceOutput(t *testing.T) {
	// Set up a context that triggers multiple rules.
	ctx := &FixContext{
		Project: scanner.Project{
			Path:            "/tmp/test",
			Name:            "test",
			Score:           30,
			PrimaryLanguage: "Go",
		},
		ToolProfile: &analyzer.ToolProfile{
			BashRatio: 0.40,
		},
		Sessions: []claude.SessionMeta{
			{
				SessionID:   "s1",
				Languages:   map[string]int{"Go": 5},
				ToolCounts:  map[string]int{"Bash": 10},
				FirstPrompt: "build the thing",
			},
		},
		CommitAnalysis: &analyzer.CommitAnalysis{
			ZeroCommitRate: 0.60,
		},
	}

	fix, err := GenerateFix(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fix.Additions) == 0 {
		t.Error("expected at least one addition from triggered rules")
	}
}
