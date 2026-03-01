package app

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

// TODO: These tests provide minimal smoke-test coverage for the --include-active
// flag added to renderScanTable. Full integration tests for runScan would require
// a more elaborate setup (config, temp directories, mock Claude data) and are
// candidates for future expansion.

// TestRenderScanTable_WithActiveMeta verifies that renderScanTable does not panic
// when an activeMeta is provided and a live row is prepended to the table.
func TestRenderScanTable_WithActiveMeta(t *testing.T) {
	results := []scanResult{
		{
			Project: scanner.Project{
				Name: "my-project",
				Path: "/Users/dayna/code/my-project",
			},
			Score: 75,
		},
	}
	activeMeta := &claude.SessionMeta{
		SessionID:   "test-session-id",
		ProjectPath: "test-project-hash",
	}

	// Should not panic.
	renderScanTable(results, activeMeta)
}

// TestRenderScanTable_NilActiveMeta verifies that renderScanTable behaves correctly
// (no panic) when activeMeta is nil, preserving the default scan output behavior.
func TestRenderScanTable_NilActiveMeta(t *testing.T) {
	results := []scanResult{
		{
			Project: scanner.Project{
				Name: "another-project",
				Path: "/Users/dayna/code/another-project",
			},
			Score: 50,
		},
	}

	// Should not panic.
	renderScanTable(results, nil)
}

// TestFlagRegistration_IncludeActive verifies that the --include-active flag is
// registered on scanCmd so it is accessible to users.
func TestFlagRegistration_IncludeActive(t *testing.T) {
	flag := scanCmd.Flags().Lookup("include-active")
	if flag == nil {
		t.Fatal("expected --include-active flag to be registered on scanCmd, but it was not found")
	}
	if flag.DefValue != "false" {
		t.Errorf("expected default value of --include-active to be %q, got %q", "false", flag.DefValue)
	}
}
