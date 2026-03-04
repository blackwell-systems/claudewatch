package memory

import (
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

func TestDeriveTaskIdentifier_WithUnderlyingGoal(t *testing.T) {
	session := claude.SessionMeta{
		SessionID:   "test-session-1",
		ProjectPath: "/home/user/project",
		FirstPrompt: "Add user authentication",
	}
	facet := &claude.SessionFacet{
		UnderlyingGoal: "Implement user authentication system",
		SessionID:      "test-session-1",
	}

	got := DeriveTaskIdentifier(session, facet)
	expected := "implement user authentication system"
	if got != expected {
		t.Errorf("DeriveTaskIdentifier() = %q, want %q", got, expected)
	}
}

func TestDeriveTaskIdentifier_FallbackToHash(t *testing.T) {
	session := claude.SessionMeta{
		SessionID:   "test-session-2",
		ProjectPath: "/home/user/project",
		FirstPrompt: "Add user authentication",
	}
	facet := &claude.SessionFacet{
		UnderlyingGoal: "", // Empty goal
		SessionID:      "test-session-2",
	}

	got := DeriveTaskIdentifier(session, facet)
	if got == "" {
		t.Error("DeriveTaskIdentifier() returned empty string")
	}
	if got[:5] != "task-" {
		t.Errorf("DeriveTaskIdentifier() = %q, expected to start with 'task-'", got)
	}
}

func TestDeriveTaskIdentifier_FallbackToSessionID(t *testing.T) {
	session := claude.SessionMeta{
		SessionID:   "test-session-3",
		ProjectPath: "/home/user/project",
		FirstPrompt: "", // Empty prompt
	}
	facet := &claude.SessionFacet{
		UnderlyingGoal: "",
		SessionID:      "test-session-3",
	}

	got := DeriveTaskIdentifier(session, facet)
	if got != "test-session-3" {
		t.Errorf("DeriveTaskIdentifier() = %q, want %q", got, "test-session-3")
	}
}

func TestExtractTaskMemory_CompletedSession(t *testing.T) {
	session := claude.SessionMeta{
		SessionID:   "completed-session",
		ProjectPath: "/home/user/project",
		FirstPrompt: "Fix bug in auth",
		GitCommits:  2,
	}
	facet := &claude.SessionFacet{
		UnderlyingGoal: "Fix authentication bug",
		Outcome:        "fully_achieved",
		BriefSummary:   "Fixed OAuth token validation",
		SessionID:      "completed-session",
	}
	commits := []string{"abc123", "def456"}

	task, err := ExtractTaskMemory(session, facet, commits)
	if err != nil {
		t.Fatalf("ExtractTaskMemory() error = %v", err)
	}
	if task == nil {
		t.Fatal("ExtractTaskMemory() returned nil task")
	}

	if task.Status != "completed" {
		t.Errorf("Status = %q, want 'completed'", task.Status)
	}
	if task.Solution == "" {
		t.Error("Solution is empty for completed session with commits")
	}
	if len(task.Commits) != 2 {
		t.Errorf("len(Commits) = %d, want 2", len(task.Commits))
	}
	if len(task.Sessions) != 1 || task.Sessions[0] != "completed-session" {
		t.Errorf("Sessions = %v, want ['completed-session']", task.Sessions)
	}
}

func TestExtractTaskMemory_AbandonedSession(t *testing.T) {
	session := claude.SessionMeta{
		SessionID:   "abandoned-session",
		ProjectPath: "/home/user/project",
		FirstPrompt: "Refactor database layer",
		GitCommits:  0,
	}
	facet := &claude.SessionFacet{
		UnderlyingGoal: "Refactor database layer",
		Outcome:        "not_achieved",
		FrictionDetail: "Multiple schema migration failures",
		SessionID:      "abandoned-session",
	}
	commits := []string{}

	task, err := ExtractTaskMemory(session, facet, commits)
	if err != nil {
		t.Fatalf("ExtractTaskMemory() error = %v", err)
	}
	if task == nil {
		t.Fatal("ExtractTaskMemory() returned nil task")
	}

	if task.Status != "abandoned" {
		t.Errorf("Status = %q, want 'abandoned'", task.Status)
	}
	if task.Solution != "" {
		t.Errorf("Solution = %q, want empty for abandoned session", task.Solution)
	}
	if len(task.BlockersHit) == 0 {
		t.Error("BlockersHit is empty, expected friction detail")
	}
}

func TestExtractTaskMemory_NilFacet(t *testing.T) {
	session := claude.SessionMeta{
		SessionID:   "no-facet-session",
		ProjectPath: "/home/user/project",
		FirstPrompt: "Test task",
	}

	task, err := ExtractTaskMemory(session, nil, []string{})
	if err != nil {
		t.Fatalf("ExtractTaskMemory() error = %v", err)
	}
	if task != nil {
		t.Errorf("ExtractTaskMemory() with nil facet = %v, want nil", task)
	}
}

func TestExtractBlockers_HighToolErrors(t *testing.T) {
	session := claude.SessionMeta{
		SessionID:  "high-errors",
		ToolErrors: 8,
	}
	facet := &claude.SessionFacet{
		SessionID: "high-errors",
		Outcome:   "partially_achieved",
	}

	blockers, err := ExtractBlockers(session, facet, "testproject", []claude.SessionMeta{}, []claude.SessionFacet{})
	if err != nil {
		t.Fatalf("ExtractBlockers() error = %v", err)
	}
	if len(blockers) == 0 {
		t.Error("ExtractBlockers() with high tool errors returned no blockers")
	}

	found := false
	for _, b := range blockers {
		if b.Issue != "" && b.Issue[:4] == "High" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ExtractBlockers() did not create high tool error blocker")
	}
}

func TestExtractBlockers_NotAchievedOutcome(t *testing.T) {
	session := claude.SessionMeta{
		SessionID: "not-achieved",
	}
	facet := &claude.SessionFacet{
		SessionID:      "not-achieved",
		Outcome:        "not_achieved",
		FrictionCounts: map[string]int{"retry:Bash": 3},
		FrictionDetail: "Command failed: permission denied",
	}

	blockers, err := ExtractBlockers(session, facet, "testproject", []claude.SessionMeta{}, []claude.SessionFacet{})
	if err != nil {
		t.Fatalf("ExtractBlockers() error = %v", err)
	}
	if len(blockers) == 0 {
		t.Error("ExtractBlockers() with not_achieved outcome returned no blockers")
		return
	}

	found := false
	for _, b := range blockers {
		if b.Issue != "" && strings.Contains(b.Issue, "Session abandoned") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ExtractBlockers() did not create abandoned session blocker. Got issues: %v", func() []string {
			issues := make([]string, len(blockers))
			for i, b := range blockers {
				issues[i] = b.Issue
			}
			return issues
		}())
	}
}

func TestExtractBlockers_SuccessfulSession(t *testing.T) {
	session := claude.SessionMeta{
		SessionID:  "success",
		ToolErrors: 1,
	}
	facet := &claude.SessionFacet{
		SessionID:      "success",
		Outcome:        "fully_achieved",
		FrictionCounts: map[string]int{},
	}

	blockers, err := ExtractBlockers(session, facet, "testproject", []claude.SessionMeta{}, []claude.SessionFacet{})
	if err != nil {
		t.Fatalf("ExtractBlockers() error = %v", err)
	}
	if len(blockers) != 0 {
		t.Errorf("ExtractBlockers() for successful session = %d blockers, want 0", len(blockers))
	}
}

func TestExtractBlockers_ChronicFriction(t *testing.T) {
	// Create 10 recent sessions where 4 have "retry:Bash" friction (40% rate).
	recentSessions := make([]claude.SessionMeta, 10)
	recentFacets := make([]claude.SessionFacet, 10)
	for i := 0; i < 10; i++ {
		sid := time.Now().Add(-time.Duration(i) * time.Hour).Format("session-%d")
		recentSessions[i] = claude.SessionMeta{
			SessionID:   sid,
			ProjectPath: "/home/user/testproject",
		}
		recentFacets[i] = claude.SessionFacet{
			SessionID:      sid,
			FrictionCounts: map[string]int{},
		}
		// Add friction to sessions 0, 2, 4, 6 (40% of 10 sessions).
		if i%2 == 0 && i < 8 {
			recentFacets[i].FrictionCounts["retry:Bash"] = 2
		}
	}

	// Current session also has retry:Bash friction.
	session := claude.SessionMeta{
		SessionID:   "current",
		ProjectPath: "/home/user/testproject",
	}
	facet := &claude.SessionFacet{
		SessionID:      "current",
		Outcome:        "partially_achieved",
		FrictionCounts: map[string]int{"retry:Bash": 1},
	}

	blockers, err := ExtractBlockers(session, facet, "testproject", recentSessions, recentFacets)
	if err != nil {
		t.Fatalf("ExtractBlockers() error = %v", err)
	}

	// Should detect chronic friction (40% > 30% threshold).
	found := false
	for _, b := range blockers {
		if len(b.Issue) > 7 && b.Issue[:7] == "Chronic" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ExtractBlockers() did not detect chronic friction pattern")
	}
}

func TestSuggestSolutionForFriction(t *testing.T) {
	tests := []struct {
		friction string
		wantSome bool
	}{
		{"retry:Bash", true},
		{"retry:Edit", true},
		{"retry:Read", true},
		{"tool_error", true},
		{"unknown_type", false},
	}

	for _, tt := range tests {
		t.Run(tt.friction, func(t *testing.T) {
			got := suggestSolutionForFriction(tt.friction)
			if tt.wantSome && got == "" {
				t.Errorf("suggestSolutionForFriction(%q) = empty, want non-empty", tt.friction)
			}
			if !tt.wantSome && got != "" {
				t.Errorf("suggestSolutionForFriction(%q) = %q, want empty", tt.friction, got)
			}
		})
	}
}

// Benchmark to ensure extraction is fast enough for startup hook.
func BenchmarkExtractTaskMemory(b *testing.B) {
	session := claude.SessionMeta{
		SessionID:   "bench-session",
		ProjectPath: "/home/user/project",
		FirstPrompt: "Benchmark test",
		GitCommits:  2,
	}
	facet := &claude.SessionFacet{
		UnderlyingGoal: "Benchmark extraction",
		Outcome:        "fully_achieved",
		BriefSummary:   "Done",
		SessionID:      "bench-session",
	}
	commits := []string{"abc123", "def456"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExtractTaskMemory(session, facet, commits)
	}
}

func BenchmarkExtractBlockers(b *testing.B) {
	session := claude.SessionMeta{
		SessionID:  "bench-session",
		ToolErrors: 8,
	}
	facet := &claude.SessionFacet{
		SessionID:      "bench-session",
		Outcome:        "not_achieved",
		FrictionCounts: map[string]int{"retry:Bash": 3},
		FrictionDetail: "Permission denied",
	}
	recentSessions := make([]claude.SessionMeta, 10)
	recentFacets := make([]claude.SessionFacet, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ExtractBlockers(session, facet, "bench", recentSessions, recentFacets)
	}
}

// Table-driven test for extractTaskMemory edge cases.
func TestExtractTaskMemory_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		session claude.SessionMeta
		facet   *claude.SessionFacet
		commits []string
		wantNil bool
		checks  func(*testing.T, *store.TaskMemory)
	}{
		{
			name: "no commits but completed",
			session: claude.SessionMeta{
				SessionID: "no-commits",
			},
			facet: &claude.SessionFacet{
				UnderlyingGoal: "Documentation update",
				Outcome:        "fully_achieved",
			},
			commits: []string{},
			wantNil: false,
			checks: func(t *testing.T, task *store.TaskMemory) {
				if task.Status != "completed" {
					t.Errorf("Status = %q, want 'completed'", task.Status)
				}
				if task.Solution != "" {
					t.Errorf("Solution = %q, want empty (no commits)", task.Solution)
				}
			},
		},
		{
			name: "in progress with commits",
			session: claude.SessionMeta{
				SessionID: "in-progress",
			},
			facet: &claude.SessionFacet{
				UnderlyingGoal: "Ongoing refactor",
				Outcome:        "partially_achieved",
				BriefSummary:   "Refactored auth module",
			},
			commits: []string{"abc123"},
			wantNil: false,
			checks: func(t *testing.T, task *store.TaskMemory) {
				if task.Status != "in_progress" {
					t.Errorf("Status = %q, want 'in_progress'", task.Status)
				}
				if task.Solution != "" {
					t.Errorf("Solution = %q, want empty (not completed)", task.Solution)
				}
				if len(task.Commits) != 1 {
					t.Errorf("len(Commits) = %d, want 1", len(task.Commits))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := ExtractTaskMemory(tt.session, tt.facet, tt.commits)
			if err != nil {
				t.Fatalf("ExtractTaskMemory() error = %v", err)
			}
			if tt.wantNil {
				if task != nil {
					t.Errorf("ExtractTaskMemory() = %v, want nil", task)
				}
				return
			}
			if task == nil {
				t.Fatal("ExtractTaskMemory() = nil, want non-nil")
			}
			if tt.checks != nil {
				tt.checks(t, task)
			}
		})
	}
}
