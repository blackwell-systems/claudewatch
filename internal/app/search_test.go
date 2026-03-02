package app

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

// TestRenderSearchResults_Empty verifies that renderSearchResults does not panic
// when given an empty result set.
func TestRenderSearchResults_Empty(t *testing.T) {
	// Should not panic.
	renderSearchResults(nil, "test-query")
}

// TestRenderSearchResults_WithResults verifies that renderSearchResults does not panic
// when given a populated result set.
func TestRenderSearchResults_WithResults(t *testing.T) {
	results := []store.TranscriptSearchResult{
		{
			SessionID:   "abc123def456",
			ProjectHash: "hash1",
			LineNumber:  1,
			EntryType:   "say",
			Snippet:     "some matching text with [query] highlighted here",
			Timestamp:   "2024-01-15T10:30:00Z",
			Rank:        -1.5,
		},
		{
			SessionID:   "xyz789uvw012",
			ProjectHash: "hash2",
			LineNumber:  42,
			EntryType:   "tool_use",
			Snippet:     "another [query] result in tool output",
			Timestamp:   "2024-01-16T11:00:00Z",
			Rank:        -0.8,
		},
	}

	// Should not panic.
	renderSearchResults(results, "query")
}

// TestSearchCmd_Registered verifies that searchCmd is registered on rootCmd.
func TestSearchCmd_Registered(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "search <query>" {
			return
		}
	}
	t.Fatal("search subcommand not registered on rootCmd")
}

// TestSearchFlagLimit_DefaultValue verifies the default limit flag value.
func TestSearchFlagLimit_DefaultValue(t *testing.T) {
	flag := searchCmd.Flags().Lookup("limit")
	if flag == nil {
		t.Fatal("expected --limit flag to be registered on searchCmd")
	}
	if flag.DefValue != "20" {
		t.Errorf("expected default value of --limit to be %q, got %q", "20", flag.DefValue)
	}
}
