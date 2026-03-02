package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLiveCostVelocity_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	pricing := CostPricing{InputPerMillion: 3.0, OutputPerMillion: 15.0}
	stats, err := ParseLiveCostVelocity(path, 10, pricing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.WindowCostUSD != 0 {
		t.Fatalf("expected WindowCostUSD=0, got %f", stats.WindowCostUSD)
	}
	if stats.CostPerMinute != 0 {
		t.Fatalf("expected CostPerMinute=0, got %f", stats.CostPerMinute)
	}
	if stats.Status != "efficient" {
		t.Fatalf("expected Status=efficient, got %q", stats.Status)
	}
}

func TestParseLiveCostVelocity_AllOutsideWindow(t *testing.T) {
	// Entries from 2 hours ago — outside a 10-minute window.
	oldTS := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantWithUsage(oldTS, 500_000, 100_000),
	})

	pricing := CostPricing{InputPerMillion: 3.0, OutputPerMillion: 15.0}
	stats, err := ParseLiveCostVelocity(path, 10, pricing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.WindowCostUSD != 0 {
		t.Fatalf("expected WindowCostUSD=0, got %f", stats.WindowCostUSD)
	}
	if stats.CostPerMinute != 0 {
		t.Fatalf("expected CostPerMinute=0, got %f", stats.CostPerMinute)
	}
	if stats.Status != "efficient" {
		t.Fatalf("expected Status=efficient, got %q", stats.Status)
	}
}

func TestParseLiveCostVelocity_WithinWindow(t *testing.T) {
	// Entry from 2 minutes ago — within a 10-minute window.
	recentTS := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339)
	// 1_000_000 input tokens, 200_000 output tokens
	// Cost = (1_000_000/1_000_000)*3.0 + (200_000/1_000_000)*15.0 = 3.0 + 3.0 = 6.0
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantWithUsage(recentTS, 1_000_000, 200_000),
	})

	pricing := CostPricing{InputPerMillion: 3.0, OutputPerMillion: 15.0}
	stats, err := ParseLiveCostVelocity(path, 10, pricing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedCost := 6.0
	if stats.WindowCostUSD < expectedCost-0.001 || stats.WindowCostUSD > expectedCost+0.001 {
		t.Fatalf("expected WindowCostUSD~%f, got %f", expectedCost, stats.WindowCostUSD)
	}

	expectedCPM := expectedCost / 10.0 // 0.6
	if stats.CostPerMinute < expectedCPM-0.001 || stats.CostPerMinute > expectedCPM+0.001 {
		t.Fatalf("expected CostPerMinute~%f, got %f", expectedCPM, stats.CostPerMinute)
	}

	if stats.Status != "burning" {
		t.Fatalf("expected Status=burning for CostPerMinute=%f, got %q", stats.CostPerMinute, stats.Status)
	}
}

func TestParseLiveCostVelocity_StatusThresholds(t *testing.T) {
	pricing := CostPricing{InputPerMillion: 3.0, OutputPerMillion: 15.0}
	recentTS := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	windowMinutes := 10.0

	tests := []struct {
		name           string
		inputTokens    int
		outputTokens   int
		expectedStatus string
	}{
		{
			// Cost = 0, CostPerMinute = 0 -> efficient
			name:           "zero_tokens_efficient",
			inputTokens:    0,
			outputTokens:   0,
			expectedStatus: "efficient",
		},
		{
			// Cost = (100_000/1M)*3 + (10_000/1M)*15 = 0.3 + 0.15 = 0.45
			// CostPerMinute = 0.45/10 = 0.045 -> efficient (< 0.05)
			name:           "low_cost_efficient",
			inputTokens:    100_000,
			outputTokens:   10_000,
			expectedStatus: "efficient",
		},
		{
			// Cost = (200_000/1M)*3 + (50_000/1M)*15 = 0.6 + 0.75 = 1.35
			// CostPerMinute = 1.35/10 = 0.135 -> normal (0.05 <= x < 0.20)
			name:           "moderate_cost_normal",
			inputTokens:    200_000,
			outputTokens:   50_000,
			expectedStatus: "normal",
		},
		{
			// Cost = (1_000_000/1M)*3 + (200_000/1M)*15 = 3.0 + 3.0 = 6.0
			// CostPerMinute = 6.0/10 = 0.6 -> burning (>= 0.20)
			name:           "high_cost_burning",
			inputTokens:    1_000_000,
			outputTokens:   200_000,
			expectedStatus: "burning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var entries []map[string]any
			if tt.inputTokens > 0 || tt.outputTokens > 0 {
				entries = append(entries, mkAssistantWithUsage(recentTS, tt.inputTokens, tt.outputTokens))
			}
			path := writeLiveJSONL(t, entries)

			stats, err := ParseLiveCostVelocity(path, windowMinutes, pricing)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stats.Status != tt.expectedStatus {
				t.Fatalf("expected Status=%q, got %q (CostPerMinute=%f)",
					tt.expectedStatus, stats.Status, stats.CostPerMinute)
			}
		})
	}
}

func TestParseLiveCostVelocity_MixedWindowEntries(t *testing.T) {
	// One entry inside window, one outside — only the inside one should count.
	oldTS := time.Now().Add(-20 * time.Minute).UTC().Format(time.RFC3339)
	recentTS := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339)

	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantWithUsage(oldTS, 5_000_000, 1_000_000), // outside window
		mkAssistantWithUsage(recentTS, 100_000, 10_000),   // inside window
	})

	pricing := CostPricing{InputPerMillion: 3.0, OutputPerMillion: 15.0}
	stats, err := ParseLiveCostVelocity(path, 10, pricing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the recent entry: cost = (100_000/1M)*3 + (10_000/1M)*15 = 0.3 + 0.15 = 0.45
	expectedCost := 0.45
	if stats.WindowCostUSD < expectedCost-0.01 || stats.WindowCostUSD > expectedCost+0.01 {
		t.Fatalf("expected WindowCostUSD~%f, got %f", expectedCost, stats.WindowCostUSD)
	}

}
