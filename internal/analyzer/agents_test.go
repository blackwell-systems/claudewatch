package analyzer

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

func TestAnalyzeAgents_Empty(t *testing.T) {
	perf := AnalyzeAgents(nil)
	if perf.TotalAgents != 0 {
		t.Errorf("TotalAgents = %d, want 0", perf.TotalAgents)
	}
	if perf.SuccessRate != 0 {
		t.Errorf("SuccessRate = %v, want 0", perf.SuccessRate)
	}
	if perf.ByType == nil {
		t.Error("ByType should be initialized, not nil")
	}
}

func TestAnalyzeAgents_SingleCompleted(t *testing.T) {
	tasks := []claude.AgentTask{
		{
			AgentID:     "a1",
			AgentType:   "code-writer",
			SessionID:   "s1",
			Status:      "completed",
			DurationMs:  5000,
			TotalTokens: 1000,
			Background:  false,
		},
	}

	perf := AnalyzeAgents(tasks)

	if perf.TotalAgents != 1 {
		t.Errorf("TotalAgents = %d, want 1", perf.TotalAgents)
	}
	if perf.SuccessRate != 1.0 {
		t.Errorf("SuccessRate = %v, want 1.0", perf.SuccessRate)
	}
	if perf.KillRate != 0 {
		t.Errorf("KillRate = %v, want 0", perf.KillRate)
	}
	if perf.BackgroundRatio != 0 {
		t.Errorf("BackgroundRatio = %v, want 0", perf.BackgroundRatio)
	}
	if perf.AvgDurationMs != 5000 {
		t.Errorf("AvgDurationMs = %v, want 5000", perf.AvgDurationMs)
	}
	if perf.AvgTokensPerAgent != 1000 {
		t.Errorf("AvgTokensPerAgent = %v, want 1000", perf.AvgTokensPerAgent)
	}
	if perf.ParallelSessions != 0 {
		t.Errorf("ParallelSessions = %d, want 0", perf.ParallelSessions)
	}
}

func TestAnalyzeAgents_MixedStatuses(t *testing.T) {
	tasks := []claude.AgentTask{
		{AgentID: "a1", AgentType: "writer", SessionID: "s1", Status: "completed", DurationMs: 2000, TotalTokens: 500},
		{AgentID: "a2", AgentType: "writer", SessionID: "s1", Status: "killed", DurationMs: 1000, TotalTokens: 200, Background: true},
		{AgentID: "a3", AgentType: "reviewer", SessionID: "s2", Status: "completed", DurationMs: 3000, TotalTokens: 800},
		{AgentID: "a4", AgentType: "reviewer", SessionID: "s2", Status: "failed", DurationMs: 500, TotalTokens: 100, Background: true},
	}

	perf := AnalyzeAgents(tasks)

	if perf.TotalAgents != 4 {
		t.Errorf("TotalAgents = %d, want 4", perf.TotalAgents)
	}

	// 2 completed out of 4.
	expectedSuccess := 0.5
	if perf.SuccessRate != expectedSuccess {
		t.Errorf("SuccessRate = %v, want %v", perf.SuccessRate, expectedSuccess)
	}

	// 1 killed out of 4.
	expectedKill := 0.25
	if perf.KillRate != expectedKill {
		t.Errorf("KillRate = %v, want %v", perf.KillRate, expectedKill)
	}

	// 2 background out of 4.
	expectedBG := 0.5
	if perf.BackgroundRatio != expectedBG {
		t.Errorf("BackgroundRatio = %v, want %v", perf.BackgroundRatio, expectedBG)
	}

	// Both sessions have 2 agents => 2 parallel sessions.
	if perf.ParallelSessions != 2 {
		t.Errorf("ParallelSessions = %d, want 2", perf.ParallelSessions)
	}

	// Per-type stats.
	if len(perf.ByType) != 2 {
		t.Fatalf("expected 2 agent types, got %d", len(perf.ByType))
	}

	writerStats := perf.ByType["writer"]
	if writerStats.Count != 2 {
		t.Errorf("writer count = %d, want 2", writerStats.Count)
	}
	if writerStats.SuccessRate != 0.5 {
		t.Errorf("writer success rate = %v, want 0.5", writerStats.SuccessRate)
	}

	reviewerStats := perf.ByType["reviewer"]
	if reviewerStats.Count != 2 {
		t.Errorf("reviewer count = %d, want 2", reviewerStats.Count)
	}
}

func TestAnalyzeAgents_SingleSession_NoParallel(t *testing.T) {
	tasks := []claude.AgentTask{
		{AgentID: "a1", AgentType: "writer", SessionID: "s1", Status: "completed", DurationMs: 1000, TotalTokens: 100},
	}

	perf := AnalyzeAgents(tasks)
	if perf.ParallelSessions != 0 {
		t.Errorf("ParallelSessions = %d, want 0 (only 1 agent in session)", perf.ParallelSessions)
	}
}

func TestAnalyzeAgents_AllBackground(t *testing.T) {
	tasks := []claude.AgentTask{
		{AgentID: "a1", SessionID: "s1", Status: "completed", Background: true, DurationMs: 100, TotalTokens: 50},
		{AgentID: "a2", SessionID: "s2", Status: "completed", Background: true, DurationMs: 200, TotalTokens: 150},
	}

	perf := AnalyzeAgents(tasks)
	if perf.BackgroundRatio != 1.0 {
		t.Errorf("BackgroundRatio = %v, want 1.0", perf.BackgroundRatio)
	}
}

func TestAnalyzeAgents_AverageDuration(t *testing.T) {
	tasks := []claude.AgentTask{
		{AgentID: "a1", SessionID: "s1", Status: "completed", DurationMs: 1000, TotalTokens: 100},
		{AgentID: "a2", SessionID: "s2", Status: "completed", DurationMs: 3000, TotalTokens: 300},
	}

	perf := AnalyzeAgents(tasks)
	expectedAvgDuration := 2000.0
	if perf.AvgDurationMs != expectedAvgDuration {
		t.Errorf("AvgDurationMs = %v, want %v", perf.AvgDurationMs, expectedAvgDuration)
	}
	expectedAvgTokens := 200.0
	if perf.AvgTokensPerAgent != expectedAvgTokens {
		t.Errorf("AvgTokensPerAgent = %v, want %v", perf.AvgTokensPerAgent, expectedAvgTokens)
	}
}
