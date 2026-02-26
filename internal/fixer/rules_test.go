package fixer

import (
	"strings"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

// helper to create a minimal FixContext with sessions containing a given language.
func ctxWithGoSessions(n int) *FixContext {
	sessions := make([]claude.SessionMeta, n)
	for i := range sessions {
		sessions[i] = claude.SessionMeta{
			SessionID:  "s" + string(rune('0'+i)),
			Languages:  map[string]int{"Go": 3},
			ToolCounts: map[string]int{"Bash": 5},
		}
	}
	return &FixContext{
		Project: scanner.Project{
			Path:            "/tmp/test",
			Name:            "test",
			PrimaryLanguage: "Go",
		},
		Sessions: sessions,
	}
}

// --- ruleMissingBuildCommands ---

func TestRuleMissingBuildCommands_Triggers(t *testing.T) {
	ctx := ctxWithGoSessions(3)
	ctx.ToolProfile = &analyzer.ToolProfile{BashRatio: 0.30}

	additions := ruleMissingBuildCommands(ctx)
	if len(additions) == 0 {
		t.Fatal("expected additions when build section missing and high bash ratio")
	}
	if additions[0].Section != "## Build & Test" {
		t.Errorf("expected section '## Build & Test', got %q", additions[0].Section)
	}
	if !strings.Contains(additions[0].Content, "go build") {
		t.Error("expected Go build commands in content")
	}
	if additions[0].Source != "missing_build_commands" {
		t.Errorf("expected source 'missing_build_commands', got %q", additions[0].Source)
	}
}

func TestRuleMissingBuildCommands_SkipsWhenBuildSectionExists(t *testing.T) {
	ctx := ctxWithGoSessions(3)
	ctx.ToolProfile = &analyzer.ToolProfile{BashRatio: 0.30}
	ctx.ExistingClaudeMD = "# Project\n\n## Build & Test\n\ngo build ./...\n"

	additions := ruleMissingBuildCommands(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions when build section exists, got %d", len(additions))
	}
}

func TestRuleMissingBuildCommands_SkipsLowBashRatio(t *testing.T) {
	ctx := ctxWithGoSessions(3)
	ctx.ToolProfile = &analyzer.ToolProfile{BashRatio: 0.05}

	additions := ruleMissingBuildCommands(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with low bash ratio, got %d", len(additions))
	}
}

func TestRuleMissingBuildCommands_SkipsNilToolProfile(t *testing.T) {
	ctx := ctxWithGoSessions(3)
	// ToolProfile is nil by default.

	additions := ruleMissingBuildCommands(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with nil tool profile, got %d", len(additions))
	}
}

// --- rulePlanModeWarning ---

func TestRulePlanModeWarning_Triggers(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
		Sessions: []claude.SessionMeta{
			{SessionID: "s1"},
		},
		AgentTasks: []claude.AgentTask{
			{AgentType: "Plan", Status: "killed"},
			{AgentType: "Plan", Status: "killed"},
			{AgentType: "Plan", Status: "completed"},
			{AgentType: "Plan", Status: "killed"},
		},
	}

	additions := rulePlanModeWarning(ctx)
	if len(additions) == 0 {
		t.Fatal("expected additions when plan kill rate > 30%")
	}
	if additions[0].Section != "## Conventions" {
		t.Errorf("expected section '## Conventions', got %q", additions[0].Section)
	}
	if !strings.Contains(additions[0].Content, "plan mode") {
		t.Error("expected plan mode warning in content")
	}
	if additions[0].Source != "plan_mode_warning" {
		t.Errorf("expected source 'plan_mode_warning', got %q", additions[0].Source)
	}
}

func TestRulePlanModeWarning_NoTriggerLowKillRate(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
		AgentTasks: []claude.AgentTask{
			{AgentType: "Plan", Status: "completed"},
			{AgentType: "Plan", Status: "completed"},
			{AgentType: "Plan", Status: "completed"},
			{AgentType: "Plan", Status: "killed"},
		},
	}

	additions := rulePlanModeWarning(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with 25%% kill rate, got %d", len(additions))
	}
}

func TestRulePlanModeWarning_NoTriggerNoAgents(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
	}

	additions := rulePlanModeWarning(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with no agent tasks, got %d", len(additions))
	}
}

// --- ruleKnownFrictionPatterns ---

func TestRuleKnownFrictionPatterns_Triggers(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
		Sessions: []claude.SessionMeta{
			{SessionID: "s1"},
		},
		FrictionPatterns: &analyzer.PersistenceAnalysis{
			StaleCount: 1,
			Patterns: []analyzer.FrictionPersistence{
				{
					FrictionType:     "wrong_approach",
					Stale:            true,
					ConsecutiveWeeks: 4,
					OccurrenceCount:  8,
				},
			},
		},
	}

	additions := ruleKnownFrictionPatterns(ctx)
	if len(additions) == 0 {
		t.Fatal("expected additions for stale friction patterns")
	}
	if additions[0].Section != "## Known Patterns" {
		t.Errorf("expected section '## Known Patterns', got %q", additions[0].Section)
	}
	if !strings.Contains(additions[0].Content, "Wrong approach") {
		t.Errorf("expected humanized friction type in content, got %q", additions[0].Content)
	}
	if additions[0].Source != "known_friction_patterns" {
		t.Errorf("expected source 'known_friction_patterns', got %q", additions[0].Source)
	}
}

func TestRuleKnownFrictionPatterns_NoTriggerNoStale(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
		FrictionPatterns: &analyzer.PersistenceAnalysis{
			StaleCount: 0,
			Patterns: []analyzer.FrictionPersistence{
				{FrictionType: "wrong_approach", Stale: false},
			},
		},
	}

	additions := ruleKnownFrictionPatterns(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with no stale patterns, got %d", len(additions))
	}
}

func TestRuleKnownFrictionPatterns_SkipsWhenSectionExists(t *testing.T) {
	ctx := &FixContext{
		Project:          scanner.Project{Path: "/tmp/test", Name: "test"},
		ExistingClaudeMD: "## Known Patterns\n\nSome existing content.",
		FrictionPatterns: &analyzer.PersistenceAnalysis{
			StaleCount: 1,
			Patterns: []analyzer.FrictionPersistence{
				{FrictionType: "wrong_approach", Stale: true, ConsecutiveWeeks: 4, OccurrenceCount: 8},
			},
		},
	}

	additions := ruleKnownFrictionPatterns(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions when known patterns section exists, got %d", len(additions))
	}
}

// --- ruleScopeConstraints ---

func TestRuleScopeConstraints_Triggers(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
		Sessions: []claude.SessionMeta{
			{SessionID: "s1"},
		},
		ConversationData: &analyzer.ConversationAnalysis{
			AvgCorrectionRate: 0.45,
		},
	}

	additions := ruleScopeConstraints(ctx)
	if len(additions) == 0 {
		t.Fatal("expected additions when correction rate > 30%")
	}
	if additions[0].Section != "## Conventions" {
		t.Errorf("expected section '## Conventions', got %q", additions[0].Section)
	}
	if !strings.Contains(additions[0].Content, "explicitly requested") {
		t.Error("expected scope constraint content")
	}
	if additions[0].Source != "scope_constraints" {
		t.Errorf("expected source 'scope_constraints', got %q", additions[0].Source)
	}
}

func TestRuleScopeConstraints_NoTriggerLowRate(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
		ConversationData: &analyzer.ConversationAnalysis{
			AvgCorrectionRate: 0.20,
		},
	}

	additions := ruleScopeConstraints(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with low correction rate, got %d", len(additions))
	}
}

func TestRuleScopeConstraints_NoTriggerNilConversation(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
	}

	additions := ruleScopeConstraints(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with nil conversation data, got %d", len(additions))
	}
}

// --- ruleMissingTestingSection ---

func TestRuleMissingTestingSection_Triggers(t *testing.T) {
	ctx := ctxWithGoSessions(2)

	additions := ruleMissingTestingSection(ctx)
	if len(additions) == 0 {
		t.Fatal("expected additions when testing section missing and Go sessions present")
	}
	if additions[0].Section != "## Testing" {
		t.Errorf("expected section '## Testing', got %q", additions[0].Section)
	}
	if !strings.Contains(additions[0].Content, "go test") {
		t.Error("expected go test command in content")
	}
	if additions[0].Source != "missing_testing_section" {
		t.Errorf("expected source 'missing_testing_section', got %q", additions[0].Source)
	}
}

func TestRuleMissingTestingSection_SkipsWhenTestingSectionExists(t *testing.T) {
	ctx := ctxWithGoSessions(2)
	ctx.ExistingClaudeMD = "## Testing\n\ngo test ./...\n"

	additions := ruleMissingTestingSection(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions when testing section exists, got %d", len(additions))
	}
}

func TestRuleMissingTestingSection_SkipsNoLanguage(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
		Sessions: []claude.SessionMeta{
			{SessionID: "s1", Languages: map[string]int{}},
		},
	}

	additions := ruleMissingTestingSection(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with no language detected, got %d", len(additions))
	}
}

// --- ruleMissingArchitectureSection ---

func TestRuleMissingArchitectureSection_Triggers(t *testing.T) {
	ctx := ctxWithGoSessions(12) // >10 sessions
	ctx.Project.PrimaryLanguage = "Go"

	additions := ruleMissingArchitectureSection(ctx)
	if len(additions) == 0 {
		t.Fatal("expected additions when architecture section missing and >10 sessions")
	}
	if additions[0].Section != "## Architecture" {
		t.Errorf("expected section '## Architecture', got %q", additions[0].Section)
	}
	if !strings.Contains(additions[0].Content, "Go project") {
		t.Error("expected Go-specific architecture stub")
	}
	if additions[0].Source != "missing_architecture_section" {
		t.Errorf("expected source 'missing_architecture_section', got %q", additions[0].Source)
	}
}

func TestRuleMissingArchitectureSection_NoTriggerFewSessions(t *testing.T) {
	ctx := ctxWithGoSessions(5) // < 10 sessions

	additions := ruleMissingArchitectureSection(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with < 10 sessions, got %d", len(additions))
	}
}

func TestRuleMissingArchitectureSection_SkipsWhenSectionExists(t *testing.T) {
	ctx := ctxWithGoSessions(12)
	ctx.ExistingClaudeMD = "## Architecture\n\nExisting architecture docs.\n"

	additions := ruleMissingArchitectureSection(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions when architecture section exists, got %d", len(additions))
	}
}

// --- ruleActionBias ---

func TestRuleActionBias_Triggers(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
		Sessions: []claude.SessionMeta{
			{SessionID: "s1"},
		},
		CommitAnalysis: &analyzer.CommitAnalysis{
			ZeroCommitRate: 0.65,
		},
	}

	additions := ruleActionBias(ctx)
	if len(additions) == 0 {
		t.Fatal("expected additions when zero-commit rate > 50%")
	}
	if additions[0].Section != "## Conventions" {
		t.Errorf("expected section '## Conventions', got %q", additions[0].Section)
	}
	if !strings.Contains(additions[0].Content, "implementation") {
		t.Error("expected action bias content")
	}
	if additions[0].Source != "action_bias" {
		t.Errorf("expected source 'action_bias', got %q", additions[0].Source)
	}
}

func TestRuleActionBias_NoTriggerLowRate(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
		CommitAnalysis: &analyzer.CommitAnalysis{
			ZeroCommitRate: 0.40,
		},
	}

	additions := ruleActionBias(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with low zero-commit rate, got %d", len(additions))
	}
}

func TestRuleActionBias_NoTriggerNilCommitAnalysis(t *testing.T) {
	ctx := &FixContext{
		Project: scanner.Project{Path: "/tmp/test", Name: "test"},
	}

	additions := ruleActionBias(ctx)
	if len(additions) != 0 {
		t.Errorf("expected no additions with nil commit analysis, got %d", len(additions))
	}
}

// --- humanizeFrictionType ---

func TestHumanizeFrictionType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"wrong_approach", "Wrong approach"},
		{"misunderstood_request", "Misunderstood request"},
		{"scope_creep", "Scope creep"},
		{"", ""},
		{"single", "Single"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := humanizeFrictionType(tc.input)
			if result != tc.expected {
				t.Errorf("humanizeFrictionType(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// --- generateArchitectureStub ---

func TestGenerateArchitectureStub(t *testing.T) {
	tests := []struct {
		language string
		contains string
	}{
		{"Go", "Go project"},
		{"Python", "Python project"},
		{"JavaScript", "JavaScript/TypeScript"},
		{"TypeScript", "JavaScript/TypeScript"},
		{"Rust", "Rust project"},
		{"Unknown", "TODO"},
	}

	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			project := scanner.Project{PrimaryLanguage: tc.language}
			result := generateArchitectureStub(project)
			if !strings.Contains(result, tc.contains) {
				t.Errorf("generateArchitectureStub(%q) = %q, expected to contain %q", tc.language, result, tc.contains)
			}
		})
	}
}
