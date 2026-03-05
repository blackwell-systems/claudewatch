package app

import (
	"strings"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// Registration tests
func TestStopCmd_Registered(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "hook-stop" {
			return
		}
	}
	t.Fatal("hook-stop subcommand not registered")
}

func TestStopCmd_Use(t *testing.T) {
	if stopCmd.Use != "hook-stop" {
		t.Errorf("expected Use=hook-stop, got %s", stopCmd.Use)
	}
}

func TestStopCmd_SilenceErrors(t *testing.T) {
	if stopCmd.SilenceErrors != true {
		t.Error("expected SilenceErrors=true")
	}
}

func TestStopCmd_SilenceUsage(t *testing.T) {
	if stopCmd.SilenceUsage != true {
		t.Error("expected SilenceUsage=true")
	}
}

// Skip condition tests
func TestShouldSkipSession_Trivial(t *testing.T) {
	meta := &claude.SessionMeta{
		DurationMinutes: 5,
		ToolCounts:      map[string]int{"Read": 10},
	}
	if !shouldSkipSession(meta, "") {
		t.Error("expected trivial session to be skipped")
	}
}

func TestShouldSkipSession_AlreadyCheckpointed(t *testing.T) {
	meta := &claude.SessionMeta{
		DurationMinutes: 40,
		ToolCounts: map[string]int{
			"Edit":                          20,
			"extract_current_session_memory": 1,
		},
	}
	if !shouldSkipSession(meta, "") {
		t.Error("expected checkpointed session to be skipped")
	}
}

func TestShouldSkipSession_PureResearch(t *testing.T) {
	meta := &claude.SessionMeta{
		DurationMinutes: 45,
		ToolCounts: map[string]int{
			"Read": 30,
			"Grep": 20,
			"Glob": 10,
		},
	}
	if !shouldSkipSession(meta, "") {
		t.Error("expected pure research session to be skipped")
	}
}

func TestShouldSkipSession_SignificantNotSkipped(t *testing.T) {
	meta := &claude.SessionMeta{
		DurationMinutes: 35,
		ToolCounts: map[string]int{
			"Edit": 25,
			"Read": 15,
		},
	}
	if shouldSkipSession(meta, "") {
		t.Error("expected significant session with edits to not be skipped")
	}
}

// Significance tests
func TestIsSignificant_LongDuration(t *testing.T) {
	meta := &claude.SessionMeta{DurationMinutes: 35}
	if !isSignificant(meta) {
		t.Error("expected >30 min session to be significant")
	}
}

func TestIsSignificant_ShortDuration(t *testing.T) {
	meta := &claude.SessionMeta{DurationMinutes: 15}
	if isSignificant(meta) {
		t.Error("expected <30 min session with no other signals to not be significant")
	}
}

func TestIsSignificant_ManyToolCalls(t *testing.T) {
	meta := &claude.SessionMeta{
		ToolCounts: map[string]int{"Edit": 60},
	}
	if !isSignificant(meta) {
		t.Error("expected >50 tool calls to be significant")
	}
}

func TestIsSignificant_FewToolCalls(t *testing.T) {
	meta := &claude.SessionMeta{
		ToolCounts: map[string]int{"Edit": 30},
	}
	if isSignificant(meta) {
		t.Error("expected <50 tool calls with no other signals to not be significant")
	}
}

func TestIsSignificant_HasCommits(t *testing.T) {
	meta := &claude.SessionMeta{
		GitCommits:      2,
		DurationMinutes: 10,
	}
	if !isSignificant(meta) {
		t.Error("expected session with commits to be significant")
	}
}

func TestIsSignificant_ManyErrors(t *testing.T) {
	meta := &claude.SessionMeta{
		ToolErrors:      8,
		DurationMinutes: 20,
	}
	if !isSignificant(meta) {
		t.Error("expected session with >5 errors to be significant")
	}
}

// Prompt generation tests
func TestDeterminePrompt_Completed(t *testing.T) {
	meta := &claude.SessionMeta{
		GitCommits:      3,
		DurationMinutes: 25,
		ToolCounts:      map[string]int{"Edit": 30},
	}
	prompt := determinePrompt(meta, "")
	if !strings.Contains(prompt, "✓") {
		t.Error("expected completed prompt to contain ✓")
	}
	if !strings.Contains(prompt, "3 commit") {
		t.Error("expected prompt to mention commit count")
	}
	if !strings.Contains(prompt, "extract_current_session_memory") {
		t.Error("expected prompt to mention MCP tool")
	}
}

func TestDeterminePrompt_CompletedNoDuration(t *testing.T) {
	meta := &claude.SessionMeta{
		GitCommits:      2,
		DurationMinutes: 0, // Live session, duration not computed yet
		ToolCounts:      map[string]int{"Edit": 60}, // Significant via tool count
	}
	prompt := determinePrompt(meta, "")
	if !strings.Contains(prompt, "✓") {
		t.Error("expected completed prompt to contain ✓")
	}
	if !strings.Contains(prompt, "2 commit") {
		t.Error("expected prompt to mention commit count")
	}
	// Should not mention duration when it's 0
	if strings.Contains(prompt, "0 minutes") {
		t.Error("expected prompt to not mention 0 minutes")
	}
}

func TestDeterminePrompt_Abandoned(t *testing.T) {
	meta := &claude.SessionMeta{
		GitCommits:      0,
		ToolErrors:      8,
		DurationMinutes: 35,
		ToolCounts:      map[string]int{"Edit": 20},
	}
	prompt := determinePrompt(meta, "")
	if !strings.Contains(prompt, "⚠") {
		t.Error("expected abandoned prompt to contain ⚠")
	}
	if !strings.Contains(prompt, "8 tool errors") {
		t.Error("expected prompt to mention error count")
	}
	if !strings.Contains(prompt, "blocker") {
		t.Error("expected prompt to mention blockers")
	}
}

func TestDeterminePrompt_InProgress(t *testing.T) {
	meta := &claude.SessionMeta{
		GitCommits:      0,
		ToolErrors:      2,
		DurationMinutes: 40,
		ToolCounts:      map[string]int{"Edit": 25, "Read": 30},
	}
	prompt := determinePrompt(meta, "")
	if !strings.Contains(prompt, "📋") {
		t.Error("expected in-progress prompt to contain 📋")
	}
	if !strings.Contains(prompt, "checkpoint") {
		t.Error("expected prompt to suggest checkpoint")
	}
	if !strings.Contains(prompt, "55 tool calls") {
		t.Error("expected prompt to mention total tool calls")
	}
}

func TestDeterminePrompt_InProgressNoDuration(t *testing.T) {
	meta := &claude.SessionMeta{
		GitCommits:      0,
		ToolErrors:      2,
		DurationMinutes: 0,
		ToolCounts:      map[string]int{"Edit": 60}, // Significant via tool count
	}
	prompt := determinePrompt(meta, "")
	if !strings.Contains(prompt, "📋") {
		t.Error("expected in-progress prompt to contain 📋")
	}
	if !strings.Contains(prompt, "60 tool calls") {
		t.Error("expected prompt to mention tool calls")
	}
	// Should not mention duration when it's 0
	if strings.Contains(prompt, "0 min") {
		t.Error("expected prompt to not mention 0 minutes")
	}
}

func TestDeterminePrompt_SkipsTrivial(t *testing.T) {
	meta := &claude.SessionMeta{
		DurationMinutes: 5,
		ToolCounts:      map[string]int{"Read": 8},
	}
	prompt := determinePrompt(meta, "")
	if prompt != "" {
		t.Errorf("expected no prompt for trivial session, got: %s", prompt)
	}
}

func TestDeterminePrompt_SkipsCheckpointed(t *testing.T) {
	meta := &claude.SessionMeta{
		GitCommits:      3,
		DurationMinutes: 45,
		ToolCounts: map[string]int{
			"Edit":                          30,
			"extract_current_session_memory": 1,
		},
	}
	prompt := determinePrompt(meta, "")
	if prompt != "" {
		t.Errorf("expected no prompt for checkpointed session, got: %s", prompt)
	}
}

func TestDeterminePrompt_SkipsPureResearch(t *testing.T) {
	meta := &claude.SessionMeta{
		DurationMinutes: 40,
		ToolCounts: map[string]int{
			"Read": 60,
			"Grep": 30,
		},
	}
	prompt := determinePrompt(meta, "")
	if prompt != "" {
		t.Errorf("expected no prompt for pure research session, got: %s", prompt)
	}
}

func TestDeterminePrompt_SkipsInsignificant(t *testing.T) {
	meta := &claude.SessionMeta{
		DurationMinutes: 15,
		ToolCounts: map[string]int{
			"Edit": 30,
			"Read": 10,
		},
	}
	prompt := determinePrompt(meta, "")
	if prompt != "" {
		t.Errorf("expected no prompt for insignificant session, got: %s", prompt)
	}
}

// Helper function tests
func TestComputeDuration(t *testing.T) {
	tests := []struct {
		name     string
		meta     *claude.SessionMeta
		expected float64
	}{
		{
			name:     "duration set",
			meta:     &claude.SessionMeta{DurationMinutes: 45},
			expected: 45.0,
		},
		{
			name:     "duration not set",
			meta:     &claude.SessionMeta{DurationMinutes: 0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeDuration(tt.meta)
			if result != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestSumToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		counts   map[string]int
		expected int
	}{
		{
			name:     "empty",
			counts:   map[string]int{},
			expected: 0,
		},
		{
			name:     "single tool",
			counts:   map[string]int{"Edit": 10},
			expected: 10,
		},
		{
			name: "multiple tools",
			counts: map[string]int{
				"Edit":  20,
				"Read":  30,
				"Write": 5,
			},
			expected: 55,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sumToolCalls(tt.counts)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestWasRecentlyCheckpointed(t *testing.T) {
	tests := []struct {
		name     string
		meta     *claude.SessionMeta
		expected bool
	}{
		{
			name: "not checkpointed",
			meta: &claude.SessionMeta{
				ToolCounts: map[string]int{"Edit": 20},
			},
			expected: false,
		},
		{
			name: "checkpointed once",
			meta: &claude.SessionMeta{
				ToolCounts: map[string]int{
					"Edit":                          20,
					"extract_current_session_memory": 1,
				},
			},
			expected: true,
		},
		{
			name: "checkpointed multiple times",
			meta: &claude.SessionMeta{
				ToolCounts: map[string]int{
					"Edit":                          20,
					"extract_current_session_memory": 3,
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wasRecentlyCheckpointed(tt.meta, "")
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
