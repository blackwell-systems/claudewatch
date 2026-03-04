package mcp

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

// TestSessionMatchesProject_TagOverride verifies that a tag override matching
// the filter string returns true, regardless of projectPath or weights.
func TestSessionMatchesProject_TagOverride(t *testing.T) {
	t.Skip("pending: sessionMatchesProject not yet implemented")

	tags := map[string]string{"sess-1": "myproject"}
	weights := []store.ProjectWeight{}
	got := sessionMatchesProject("sess-1", "/some/other/path", tags, weights, "myproject")
	if !got {
		t.Errorf("expected true when tag override matches filter, got false")
	}
}

// TestSessionMatchesProject_WeightMatch verifies that a session with a
// high-weight project entry matching the filter returns true.
func TestSessionMatchesProject_WeightMatch(t *testing.T) {
	t.Skip("pending: sessionMatchesProject not yet implemented")

	tags := map[string]string{}
	weights := []store.ProjectWeight{
		{Project: "alpha", RepoRoot: "/code/alpha", Weight: 0.8, ToolCalls: 40},
		{Project: "beta", RepoRoot: "/code/beta", Weight: 0.2, ToolCalls: 10},
	}
	got := sessionMatchesProject("sess-2", "/code/alpha", tags, weights, "alpha")
	if !got {
		t.Errorf("expected true when weight entry matches filter, got false")
	}
}

// TestSessionMatchesProject_PathFallback verifies that when no tags or weights
// are present, the filter is matched against filepath.Base(projectPath).
func TestSessionMatchesProject_PathFallback(t *testing.T) {
	t.Skip("pending: sessionMatchesProject not yet implemented")

	tags := map[string]string{}
	weights := []store.ProjectWeight{}
	got := sessionMatchesProject("sess-3", "/home/user/myrepo", tags, weights, "myrepo")
	if !got {
		t.Errorf("expected true when filter matches filepath.Base(projectPath), got false")
	}
}

// TestSessionMatchesProject_NoMatch verifies that a session not associated
// with the filter project returns false.
func TestSessionMatchesProject_NoMatch(t *testing.T) {
	t.Skip("pending: sessionMatchesProject not yet implemented")

	tags := map[string]string{"sess-4": "otherproject"}
	weights := []store.ProjectWeight{
		{Project: "otherproject", RepoRoot: "/code/other", Weight: 1.0, ToolCalls: 50},
	}
	got := sessionMatchesProject("sess-4", "/code/other", tags, weights, "myproject")
	if got {
		t.Errorf("expected false when session belongs to a different project, got true")
	}
}

// TestSessionPrimaryProject_TagOverride verifies that a tag override takes
// priority over weights and path when determining the primary project.
func TestSessionPrimaryProject_TagOverride(t *testing.T) {
	t.Skip("pending: sessionMatchesProject not yet implemented")

	tags := map[string]string{"sess-5": "tagged-project"}
	weights := []store.ProjectWeight{
		{Project: "weight-project", RepoRoot: "/code/weight", Weight: 1.0, ToolCalls: 100},
	}
	got := sessionPrimaryProject("sess-5", "/code/weight", tags, weights)
	if got != "tagged-project" {
		t.Errorf("expected %q, got %q", "tagged-project", got)
	}
}

// TestSessionPrimaryProject_WeightFirst verifies that the highest-weight
// project entry is returned as the primary project when no tag override exists.
func TestSessionPrimaryProject_WeightFirst(t *testing.T) {
	t.Skip("pending: sessionMatchesProject not yet implemented")

	tags := map[string]string{}
	weights := []store.ProjectWeight{
		{Project: "minor", RepoRoot: "/code/minor", Weight: 0.1, ToolCalls: 5},
		{Project: "dominant", RepoRoot: "/code/dominant", Weight: 0.9, ToolCalls: 45},
	}
	got := sessionPrimaryProject("sess-6", "/code/minor", tags, weights)
	if got != "dominant" {
		t.Errorf("expected %q (highest weight), got %q", "dominant", got)
	}
}

// TestSessionPrimaryProject_PathFallback verifies that when neither tags nor
// weights are available, filepath.Base(projectPath) is returned.
func TestSessionPrimaryProject_PathFallback(t *testing.T) {
	t.Skip("pending: sessionMatchesProject not yet implemented")

	tags := map[string]string{}
	weights := []store.ProjectWeight{}
	got := sessionPrimaryProject("sess-7", "/home/user/myrepo", tags, weights)
	if got != "myrepo" {
		t.Errorf("expected %q, got %q", "myrepo", got)
	}
}
