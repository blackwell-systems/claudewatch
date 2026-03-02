package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mkAssistantWithUsage builds a JSONL entry map for an assistant message with
// the given input/output token usage.
func mkAssistantWithUsage(ts string, inputTokens, outputTokens int) map[string]any {
	msg, _ := json.Marshal(map[string]any{
		"role":    "assistant",
		"content": []map[string]any{{"type": "text", "text": "hello"}},
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	})
	return map[string]any{
		"type":      "assistant",
		"timestamp": ts,
		"message":   json.RawMessage(msg),
	}
}

// mkSummaryEntry builds a JSONL entry for a compaction/summary event.
func mkSummaryEntry(ts string) map[string]any {
	return map[string]any{
		"type":      "summary",
		"timestamp": ts,
	}
}

func TestParseLiveContextPressure_NoEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	stats, err := ParseLiveContextPressure(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalInputTokens != 0 {
		t.Fatalf("expected TotalInputTokens=0, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 0 {
		t.Fatalf("expected TotalOutputTokens=0, got %d", stats.TotalOutputTokens)
	}
	if stats.TotalTokens != 0 {
		t.Fatalf("expected TotalTokens=0, got %d", stats.TotalTokens)
	}
	if stats.Compactions != 0 {
		t.Fatalf("expected Compactions=0, got %d", stats.Compactions)
	}
	if stats.EstimatedUsage != 0 {
		t.Fatalf("expected EstimatedUsage=0, got %f", stats.EstimatedUsage)
	}
	if stats.Status != "comfortable" {
		t.Fatalf("expected Status=comfortable, got %q", stats.Status)
	}
}

func TestParseLiveContextPressure_LowUsage(t *testing.T) {
	// Two assistant turns with low token counts -> comfortable.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantWithUsage("2026-03-01T10:00:00Z", 5000, 1000),
		mkAssistantWithUsage("2026-03-01T10:01:00Z", 10000, 2000),
	})

	stats, err := ParseLiveContextPressure(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalInputTokens != 15000 {
		t.Fatalf("expected TotalInputTokens=15000, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 3000 {
		t.Fatalf("expected TotalOutputTokens=3000, got %d", stats.TotalOutputTokens)
	}
	if stats.TotalTokens != 18000 {
		t.Fatalf("expected TotalTokens=18000, got %d", stats.TotalTokens)
	}
	// EstimatedUsage = last input_tokens (10000) / 200000 = 0.05
	expectedUsage := 10000.0 / 200000.0
	if stats.EstimatedUsage != expectedUsage {
		t.Fatalf("expected EstimatedUsage=%f, got %f", expectedUsage, stats.EstimatedUsage)
	}
	if stats.Status != "comfortable" {
		t.Fatalf("expected Status=comfortable, got %q", stats.Status)
	}
}

func TestParseLiveContextPressure_HighUsage(t *testing.T) {
	// Last assistant message has high input tokens -> pressure or critical.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantWithUsage("2026-03-01T10:00:00Z", 50000, 5000),
		mkAssistantWithUsage("2026-03-01T10:01:00Z", 100000, 8000),
		mkAssistantWithUsage("2026-03-01T10:02:00Z", 160000, 10000),
	})

	stats, err := ParseLiveContextPressure(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// EstimatedUsage = 160000 / 200000 = 0.8 -> "pressure"
	expectedUsage := 160000.0 / 200000.0
	if stats.EstimatedUsage != expectedUsage {
		t.Fatalf("expected EstimatedUsage=%f, got %f", expectedUsage, stats.EstimatedUsage)
	}
	if stats.Status != "pressure" {
		t.Fatalf("expected Status=pressure, got %q", stats.Status)
	}

	// Test critical threshold: 190000 / 200000 = 0.95
	path2 := writeLiveJSONL(t, []map[string]any{
		mkAssistantWithUsage("2026-03-01T10:00:00Z", 190000, 5000),
	})
	stats2, err := ParseLiveContextPressure(path2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats2.Status != "critical" {
		t.Fatalf("expected Status=critical, got %q", stats2.Status)
	}

	// Test filling threshold: 120000 / 200000 = 0.6
	path3 := writeLiveJSONL(t, []map[string]any{
		mkAssistantWithUsage("2026-03-01T10:00:00Z", 120000, 5000),
	})
	stats3, err := ParseLiveContextPressure(path3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats3.Status != "filling" {
		t.Fatalf("expected Status=filling, got %q", stats3.Status)
	}
}

func TestParseLiveContextPressure_WithCompactions(t *testing.T) {
	// Mix of assistant messages and summary (compaction) entries.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantWithUsage("2026-03-01T10:00:00Z", 50000, 5000),
		mkSummaryEntry("2026-03-01T10:01:00Z"),
		mkAssistantWithUsage("2026-03-01T10:02:00Z", 30000, 3000),
		mkSummaryEntry("2026-03-01T10:03:00Z"),
		mkAssistantWithUsage("2026-03-01T10:04:00Z", 40000, 4000),
	})

	stats, err := ParseLiveContextPressure(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Compactions != 2 {
		t.Fatalf("expected Compactions=2, got %d", stats.Compactions)
	}
	if stats.TotalInputTokens != 120000 {
		t.Fatalf("expected TotalInputTokens=120000, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 12000 {
		t.Fatalf("expected TotalOutputTokens=12000, got %d", stats.TotalOutputTokens)
	}
	// Last input_tokens = 40000 -> 40000/200000 = 0.2 -> comfortable
	expectedUsage := 40000.0 / 200000.0
	if stats.EstimatedUsage != expectedUsage {
		t.Fatalf("expected EstimatedUsage=%f, got %f", expectedUsage, stats.EstimatedUsage)
	}
	if stats.Status != "comfortable" {
		t.Fatalf("expected Status=comfortable, got %q", stats.Status)
	}
}

func TestParseLiveContextPressure_FileNotFound(t *testing.T) {
	_, err := ParseLiveContextPressure("/nonexistent/path.jsonl")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestParseLiveContextPressure_StatusThresholds(t *testing.T) {
	tests := []struct {
		inputTokens    int
		expectedStatus string
	}{
		{0, "comfortable"},
		{99999, "comfortable"},
		{100000, "filling"},
		{149999, "filling"},
		{150000, "pressure"},
		{179999, "pressure"},
		{180000, "critical"},
		{200000, "critical"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("input_%d", tc.inputTokens), func(t *testing.T) {
			path := writeLiveJSONL(t, []map[string]any{
				mkAssistantWithUsage("2026-03-01T10:00:00Z", tc.inputTokens, 1000),
			})
			stats, err := ParseLiveContextPressure(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stats.Status != tc.expectedStatus {
				t.Fatalf("for input_tokens=%d: expected Status=%q, got %q (usage=%f)",
					tc.inputTokens, tc.expectedStatus, stats.Status, stats.EstimatedUsage)
			}
		})
	}
}
