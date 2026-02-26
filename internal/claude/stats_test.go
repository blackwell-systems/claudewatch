package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStatsCache_ValidFile(t *testing.T) {
	dir := t.TempDir()
	data := `{
		"version": 1,
		"lastComputedDate": "2026-01-15",
		"dailyActivity": [
			{"date":"2026-01-14","messageCount":50,"sessionCount":3,"toolCallCount":120},
			{"date":"2026-01-15","messageCount":30,"sessionCount":2,"toolCallCount":80}
		],
		"dailyModelTokens": [
			{"date":"2026-01-15","tokensByModel":{"claude-3.5-sonnet":5000,"claude-3-opus":2000}}
		],
		"modelUsage": {
			"claude-3.5-sonnet": {
				"inputTokens": 100000,
				"outputTokens": 50000,
				"cacheReadInputTokens": 20000,
				"cacheCreationInputTokens": 5000,
				"webSearchRequests": 3,
				"costUSD": 1.25,
				"contextWindow": 200000,
				"maxOutputTokens": 8192
			}
		},
		"totalSessions": 42,
		"totalMessages": 500,
		"longestSession": {
			"sessionId": "sess-long",
			"duration": 7200000,
			"messageCount": 85,
			"timestamp": "2026-01-10T14:00:00Z"
		},
		"firstSessionDate": "2025-12-01",
		"hourCounts": {"10": 15, "14": 20, "22": 5},
		"totalSpeculationTimeSavedMs": 45000
	}`
	if err := os.WriteFile(filepath.Join(dir, "stats-cache.json"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	stats, err := ParseStatsCache(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if stats.Version != 1 {
		t.Errorf("Version = %d, want 1", stats.Version)
	}
	if stats.TotalSessions != 42 {
		t.Errorf("TotalSessions = %d, want 42", stats.TotalSessions)
	}
	if stats.TotalMessages != 500 {
		t.Errorf("TotalMessages = %d, want 500", stats.TotalMessages)
	}
	if stats.FirstSessionDate != "2025-12-01" {
		t.Errorf("FirstSessionDate = %q, want %q", stats.FirstSessionDate, "2025-12-01")
	}
	if stats.TotalSpeculationTimeSavedMs != 45000 {
		t.Errorf("TotalSpeculationTimeSavedMs = %d, want 45000", stats.TotalSpeculationTimeSavedMs)
	}
	if len(stats.DailyActivity) != 2 {
		t.Errorf("DailyActivity length = %d, want 2", len(stats.DailyActivity))
	}
	if stats.DailyActivity[0].MessageCount != 50 {
		t.Errorf("DailyActivity[0].MessageCount = %d, want 50", stats.DailyActivity[0].MessageCount)
	}
	if len(stats.DailyModelTokens) != 1 {
		t.Errorf("DailyModelTokens length = %d, want 1", len(stats.DailyModelTokens))
	}
	if stats.DailyModelTokens[0].TokensByModel["claude-3.5-sonnet"] != 5000 {
		t.Errorf("TokensByModel[claude-3.5-sonnet] = %d, want 5000", stats.DailyModelTokens[0].TokensByModel["claude-3.5-sonnet"])
	}

	sonnet, ok := stats.ModelUsage["claude-3.5-sonnet"]
	if !ok {
		t.Fatal("missing ModelUsage for claude-3.5-sonnet")
	}
	if sonnet.InputTokens != 100000 {
		t.Errorf("InputTokens = %d, want 100000", sonnet.InputTokens)
	}
	if sonnet.CostUSD != 1.25 {
		t.Errorf("CostUSD = %f, want 1.25", sonnet.CostUSD)
	}
	if sonnet.ContextWindow != 200000 {
		t.Errorf("ContextWindow = %d, want 200000", sonnet.ContextWindow)
	}

	if stats.LongestSession.SessionID != "sess-long" {
		t.Errorf("LongestSession.SessionID = %q, want %q", stats.LongestSession.SessionID, "sess-long")
	}
	if stats.LongestSession.MessageCount != 85 {
		t.Errorf("LongestSession.MessageCount = %d, want 85", stats.LongestSession.MessageCount)
	}

	if stats.HourCounts["14"] != 20 {
		t.Errorf("HourCounts[14] = %d, want 20", stats.HourCounts["14"])
	}
}

func TestParseStatsCache_MissingFile(t *testing.T) {
	dir := t.TempDir()
	stats, err := ParseStatsCache(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if stats != nil {
		t.Errorf("expected nil stats, got %+v", stats)
	}
}

func TestParseStatsCache_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stats-cache.json"), []byte("broken"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	stats, err := ParseStatsCache(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if stats != nil {
		t.Errorf("expected nil stats on error, got %+v", stats)
	}
}

func TestParseStatsCache_EmptyJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stats-cache.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	stats, err := ParseStatsCache(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats == nil {
		t.Fatal("expected non-nil stats for empty JSON object")
	}
	if stats.TotalSessions != 0 {
		t.Errorf("expected default TotalSessions = 0, got %d", stats.TotalSessions)
	}
}
