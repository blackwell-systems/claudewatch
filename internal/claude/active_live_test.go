package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// mkAssistantToolUse builds a JSONL entry map for an assistant message with tool_use blocks.
func mkAssistantToolUse(ts string, tools ...map[string]any) map[string]any {
	var content []map[string]any
	for _, t := range tools {
		block := map[string]any{
			"type": "tool_use",
			"id":   t["id"],
			"name": t["name"],
		}
		if inp, ok := t["input"]; ok {
			block["input"] = inp
		} else {
			block["input"] = map[string]any{}
		}
		content = append(content, block)
	}
	msg, _ := json.Marshal(map[string]any{
		"role":    "assistant",
		"content": content,
	})
	return map[string]any{
		"type":      "assistant",
		"timestamp": ts,
		"message":   json.RawMessage(msg),
	}
}

// mkUserToolResult builds a JSONL entry map for a user message with tool_result blocks.
func mkUserToolResult(ts string, results ...map[string]any) map[string]any {
	var content []map[string]any
	for _, r := range results {
		block := map[string]any{
			"type":        "tool_result",
			"tool_use_id": r["tool_use_id"],
		}
		if isErr, ok := r["is_error"]; ok {
			block["is_error"] = isErr
		}
		content = append(content, block)
	}
	msg, _ := json.Marshal(map[string]any{
		"role":    "user",
		"content": content,
	})
	return map[string]any{
		"type":      "user",
		"timestamp": ts,
		"message":   json.RawMessage(msg),
	}
}

func writeLiveJSONL(t *testing.T, entries []map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	writeJSONLFile(t, path, entries)
	return path
}

// --- ParseLiveToolErrors tests ---

func TestParseLiveToolErrors_NoErrors(t *testing.T) {
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Read"},
			map[string]any{"id": "tu2", "name": "Grep"},
			map[string]any{"id": "tu3", "name": "Bash"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1"},
			map[string]any{"tool_use_id": "tu2"},
			map[string]any{"tool_use_id": "tu3"},
		),
	})

	stats, err := ParseLiveToolErrors(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalToolUses != 3 {
		t.Fatalf("expected TotalToolUses=3, got %d", stats.TotalToolUses)
	}
	if stats.TotalErrors != 0 {
		t.Fatalf("expected TotalErrors=0, got %d", stats.TotalErrors)
	}
	if stats.ErrorRate != 0 {
		t.Fatalf("expected ErrorRate=0, got %f", stats.ErrorRate)
	}
	if stats.ConsecutiveErrs != 0 {
		t.Fatalf("expected ConsecutiveErrs=0, got %d", stats.ConsecutiveErrs)
	}
	if stats.ErrorsByTool == nil {
		t.Fatal("ErrorsByTool should not be nil")
	}
}

func TestParseLiveToolErrors_WithErrors(t *testing.T) {
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Edit"},
			map[string]any{"id": "tu2", "name": "Bash"},
			map[string]any{"id": "tu3", "name": "Edit"},
			map[string]any{"id": "tu4", "name": "Read"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1", "is_error": true},
			map[string]any{"tool_use_id": "tu2"},
			map[string]any{"tool_use_id": "tu3", "is_error": true},
			map[string]any{"tool_use_id": "tu4"},
		),
	})

	stats, err := ParseLiveToolErrors(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalToolUses != 4 {
		t.Fatalf("expected TotalToolUses=4, got %d", stats.TotalToolUses)
	}
	if stats.TotalErrors != 2 {
		t.Fatalf("expected TotalErrors=2, got %d", stats.TotalErrors)
	}
	expectedRate := 2.0 / 4.0
	if stats.ErrorRate != expectedRate {
		t.Fatalf("expected ErrorRate=%f, got %f", expectedRate, stats.ErrorRate)
	}
	if stats.ErrorsByTool["Edit"] != 2 {
		t.Fatalf("expected ErrorsByTool[Edit]=2, got %d", stats.ErrorsByTool["Edit"])
	}
}

func TestParseLiveToolErrors_ConsecutiveErrors(t *testing.T) {
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Edit"},
			map[string]any{"id": "tu2", "name": "Edit"},
			map[string]any{"id": "tu3", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1", "is_error": true},
			map[string]any{"tool_use_id": "tu2", "is_error": true},
			map[string]any{"tool_use_id": "tu3", "is_error": true},
		),
		// A successful result resets the streak.
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu4", "name": "Read"},
		),
		mkUserToolResult("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu4"},
		),
		// One more error: streak should be 1, but max should remain 3.
		mkAssistantToolUse("2026-03-01T10:00:04Z",
			map[string]any{"id": "tu5", "name": "Bash"},
		),
		mkUserToolResult("2026-03-01T10:00:05Z",
			map[string]any{"tool_use_id": "tu5", "is_error": true},
		),
	})

	stats, err := ParseLiveToolErrors(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.ConsecutiveErrs != 3 {
		t.Fatalf("expected ConsecutiveErrs=3 (max streak), got %d", stats.ConsecutiveErrs)
	}
	if stats.TotalErrors != 4 {
		t.Fatalf("expected TotalErrors=4, got %d", stats.TotalErrors)
	}
}

func TestParseLiveToolErrors_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	stats, err := ParseLiveToolErrors(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalToolUses != 0 {
		t.Fatalf("expected TotalToolUses=0, got %d", stats.TotalToolUses)
	}
	if stats.TotalErrors != 0 {
		t.Fatalf("expected TotalErrors=0, got %d", stats.TotalErrors)
	}
	if stats.ErrorRate != 0 {
		t.Fatalf("expected ErrorRate=0, got %f", stats.ErrorRate)
	}
	if stats.ErrorsByTool == nil {
		t.Fatal("ErrorsByTool should not be nil")
	}
}

// --- ParseLiveFriction tests ---

func TestParseLiveFriction_NoFriction(t *testing.T) {
	// Three different tools, no errors -> no friction.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Read"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1"},
		),
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu2", "name": "Grep"},
		),
		mkUserToolResult("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu2"},
		),
		mkAssistantToolUse("2026-03-01T10:00:04Z",
			map[string]any{"id": "tu3", "name": "Bash"},
		),
		mkUserToolResult("2026-03-01T10:00:05Z",
			map[string]any{"tool_use_id": "tu3"},
		),
	})

	stats, err := ParseLiveFriction(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats.Events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(stats.Events))
	}
	if stats.TotalFriction != 0 {
		t.Fatalf("expected TotalFriction=0, got %d", stats.TotalFriction)
	}
}

func TestParseLiveFriction_ToolError(t *testing.T) {
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1", "is_error": true},
		),
	})

	stats, err := ParseLiveFriction(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have at least one tool_error event.
	found := false
	for _, ev := range stats.Events {
		if ev.Type == "tool_error" && ev.Tool == "Edit" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tool_error event for Edit, got events: %+v", stats.Events)
	}
	if stats.TotalFriction < 1 {
		t.Fatalf("expected TotalFriction >= 1, got %d", stats.TotalFriction)
	}
}

func TestParseLiveFriction_Retry(t *testing.T) {
	// Same tool (Edit) used 3 times in a row -> retry detected on 2nd and 3rd.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1"},
		),
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu2", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu2"},
		),
		mkAssistantToolUse("2026-03-01T10:00:04Z",
			map[string]any{"id": "tu3", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:05Z",
			map[string]any{"tool_use_id": "tu3"},
		),
	})

	stats, err := ParseLiveFriction(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	retryCount := 0
	for _, ev := range stats.Events {
		if ev.Type == "retry" && ev.Tool == "Edit" {
			retryCount++
		}
	}
	if retryCount < 1 {
		t.Fatalf("expected at least 1 retry event for Edit, got %d; events: %+v", retryCount, stats.Events)
	}
}

// --- ParseLiveCommitAttempts tests ---

func TestParseLiveCommitAttempts_NoEdits(t *testing.T) {
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Read"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1"},
		),
	})

	stats, err := ParseLiveCommitAttempts(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.EditWriteAttempts != 0 {
		t.Fatalf("expected EditWriteAttempts=0, got %d", stats.EditWriteAttempts)
	}
	if stats.GitCommits != 0 {
		t.Fatalf("expected GitCommits=0, got %d", stats.GitCommits)
	}
	if stats.Ratio != 0 {
		t.Fatalf("expected Ratio=0, got %f", stats.Ratio)
	}
}

func TestParseLiveCommitAttempts_WithCommits(t *testing.T) {
	bashInput, _ := json.Marshal(map[string]any{
		"command": "git commit -m 'fix bug'",
	})
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Edit"},
			map[string]any{"id": "tu2", "name": "Write"},
			map[string]any{"id": "tu3", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1"},
			map[string]any{"tool_use_id": "tu2"},
			map[string]any{"tool_use_id": "tu3"},
		),
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu4", "name": "Bash", "input": json.RawMessage(bashInput)},
		),
		mkUserToolResult("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu4"},
		),
	})

	stats, err := ParseLiveCommitAttempts(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.EditWriteAttempts != 3 {
		t.Fatalf("expected EditWriteAttempts=3, got %d", stats.EditWriteAttempts)
	}
	if stats.GitCommits != 1 {
		t.Fatalf("expected GitCommits=1, got %d", stats.GitCommits)
	}
	expectedRatio := 1.0 / 3.0
	if stats.Ratio < expectedRatio-0.001 || stats.Ratio > expectedRatio+0.001 {
		t.Fatalf("expected Ratio~%f, got %f", expectedRatio, stats.Ratio)
	}
}
