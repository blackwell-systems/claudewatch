package claude

import (
	"encoding/json"
	"fmt"
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

// --- collapseFrictionPatterns tests ---

func TestCollapseFrictionPatterns_NoEvents(t *testing.T) {
	patterns := collapseFrictionPatterns(nil)
	if patterns == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(patterns) != 0 {
		t.Fatalf("expected 0 patterns, got %d", len(patterns))
	}

	patterns = collapseFrictionPatterns([]LiveFrictionEvent{})
	if len(patterns) != 0 {
		t.Fatalf("expected 0 patterns for empty slice, got %d", len(patterns))
	}
}

func TestCollapseFrictionPatterns_SingleType(t *testing.T) {
	events := []LiveFrictionEvent{
		{Type: "tool_error", Tool: "Edit", Count: 1},
		{Type: "tool_error", Tool: "Edit", Count: 1},
		{Type: "tool_error", Tool: "Edit", Count: 1},
	}
	patterns := collapseFrictionPatterns(events)
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	p := patterns[0]
	if p.Type != "tool_error:Edit" {
		t.Errorf("Type = %q, want %q", p.Type, "tool_error:Edit")
	}
	if p.Count != 3 {
		t.Errorf("Count = %d, want 3", p.Count)
	}
	if !p.Consecutive {
		t.Error("Consecutive = false, want true")
	}
	if p.FirstTurn != 0 {
		t.Errorf("FirstTurn = %d, want 0", p.FirstTurn)
	}
	if p.LastTurn != 2 {
		t.Errorf("LastTurn = %d, want 2", p.LastTurn)
	}
}

func TestCollapseFrictionPatterns_MixedTypes(t *testing.T) {
	events := []LiveFrictionEvent{
		{Type: "tool_error", Tool: "Edit", Count: 1},
		{Type: "tool_error", Tool: "Edit", Count: 1},
		{Type: "retry", Tool: "Bash", Count: 2},
		{Type: "error_burst", Count: 3},
	}
	patterns := collapseFrictionPatterns(events)
	if len(patterns) != 3 {
		t.Fatalf("expected 3 patterns, got %d", len(patterns))
	}

	// Sorted by count descending: error_burst(3), tool_error:Edit(2), retry:Bash(2)
	// tie between tool_error:Edit and retry:Bash -> alphabetical: retry:Bash < tool_error:Edit
	if patterns[0].Type != "error_burst" {
		t.Errorf("patterns[0].Type = %q, want %q", patterns[0].Type, "error_burst")
	}
	if patterns[0].Count != 3 {
		t.Errorf("patterns[0].Count = %d, want 3", patterns[0].Count)
	}
	if patterns[1].Type != "retry:Bash" {
		t.Errorf("patterns[1].Type = %q, want %q", patterns[1].Type, "retry:Bash")
	}
	if patterns[1].Count != 2 {
		t.Errorf("patterns[1].Count = %d, want 2", patterns[1].Count)
	}
	if patterns[2].Type != "tool_error:Edit" {
		t.Errorf("patterns[2].Type = %q, want %q", patterns[2].Type, "tool_error:Edit")
	}
	if patterns[2].Count != 2 {
		t.Errorf("patterns[2].Count = %d, want 2", patterns[2].Count)
	}
}

func TestCollapseFrictionPatterns_ConsecutiveDetection(t *testing.T) {
	events := []LiveFrictionEvent{
		{Type: "tool_error", Tool: "Edit", Count: 1}, // idx 0
		{Type: "retry", Tool: "Bash", Count: 2},      // idx 1
		{Type: "tool_error", Tool: "Edit", Count: 1}, // idx 2 — gap (retry:Bash at idx 1)
	}
	patterns := collapseFrictionPatterns(events)

	// Find tool_error:Edit pattern.
	var editPattern *FrictionPattern
	for i := range patterns {
		if patterns[i].Type == "tool_error:Edit" {
			editPattern = &patterns[i]
			break
		}
	}
	if editPattern == nil {
		t.Fatal("tool_error:Edit pattern not found")
	}
	if editPattern.Consecutive {
		t.Error("Consecutive = true, want false (gap at idx 1)")
	}
	if editPattern.FirstTurn != 0 {
		t.Errorf("FirstTurn = %d, want 0", editPattern.FirstTurn)
	}
	if editPattern.LastTurn != 2 {
		t.Errorf("LastTurn = %d, want 2", editPattern.LastTurn)
	}

	// retry:Bash should be consecutive (only one occurrence).
	var bashPattern *FrictionPattern
	for i := range patterns {
		if patterns[i].Type == "retry:Bash" {
			bashPattern = &patterns[i]
			break
		}
	}
	if bashPattern == nil {
		t.Fatal("retry:Bash pattern not found")
	}
	if !bashPattern.Consecutive {
		t.Error("retry:Bash Consecutive = false, want true (single occurrence)")
	}
}

func TestParseLiveFriction_PopulatesPatterns(t *testing.T) {
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1", "is_error": true},
		),
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu2", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu2", "is_error": true},
		),
	})

	stats, err := ParseLiveFriction(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Patterns == nil {
		t.Fatal("Patterns is nil, expected non-nil")
	}
	if len(stats.Patterns) == 0 {
		t.Fatal("Patterns is empty, expected at least one pattern")
	}

	// Should have tool_error:Edit and retry:Edit patterns.
	found := make(map[string]bool)
	for _, p := range stats.Patterns {
		found[p.Type] = true
	}
	if !found["tool_error:Edit"] {
		t.Errorf("expected tool_error:Edit pattern, got patterns: %+v", stats.Patterns)
	}
}

// --- ParseLiveConsecutiveErrors tests ---

func TestParseLiveConsecutiveErrors_NoErrors(t *testing.T) {
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

	streak, err := ParseLiveConsecutiveErrors(path, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if streak != 0 {
		t.Fatalf("expected streak=0, got %d", streak)
	}
}

func TestParseLiveConsecutiveErrors_TrailingStreak(t *testing.T) {
	// 2 successes then 3 errors at the tail — expect trailing streak of 3.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Read"},
			map[string]any{"id": "tu2", "name": "Grep"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1"},
			map[string]any{"tool_use_id": "tu2"},
		),
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu3", "name": "Edit"},
			map[string]any{"id": "tu4", "name": "Edit"},
			map[string]any{"id": "tu5", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu3", "is_error": true},
			map[string]any{"tool_use_id": "tu4", "is_error": true},
			map[string]any{"tool_use_id": "tu5", "is_error": true},
		),
	})

	streak, err := ParseLiveConsecutiveErrors(path, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if streak != 3 {
		t.Fatalf("expected streak=3, got %d", streak)
	}
}

func TestParseLiveConsecutiveErrors_StreakBrokenBySuccess(t *testing.T) {
	// 3 errors then 1 success then 2 errors at tail — expect trailing streak of 2.
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
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu4", "name": "Read"},
		),
		mkUserToolResult("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu4"},
		),
		mkAssistantToolUse("2026-03-01T10:00:04Z",
			map[string]any{"id": "tu5", "name": "Edit"},
			map[string]any{"id": "tu6", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:05Z",
			map[string]any{"tool_use_id": "tu5", "is_error": true},
			map[string]any{"tool_use_id": "tu6", "is_error": true},
		),
	})

	streak, err := ParseLiveConsecutiveErrors(path, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if streak != 2 {
		t.Fatalf("expected streak=2 (trailing only), got %d", streak)
	}
}

func TestParseLiveConsecutiveErrors_TailNLimit(t *testing.T) {
	// 10 errors at the start then 2 successes at the tail, tailN=5.
	// With tailN=5 the window covers only the last 5 entries; the last 2 user
	// entries contain only successes so the streak should be 0.
	var entries []map[string]any

	// 10 error tool uses/results (20 entries total: 10 assistant + 10 user).
	for i := 0; i < 10; i++ {
		id := "err" + string(rune('0'+i))
		entries = append(entries,
			mkAssistantToolUse("2026-03-01T10:00:00Z",
				map[string]any{"id": id, "name": "Edit"},
			),
			mkUserToolResult("2026-03-01T10:00:01Z",
				map[string]any{"tool_use_id": id, "is_error": true},
			),
		)
	}

	// 2 success tool uses/results (4 entries).
	for i := 0; i < 2; i++ {
		id := "ok" + string(rune('0'+i))
		entries = append(entries,
			mkAssistantToolUse("2026-03-01T10:01:00Z",
				map[string]any{"id": id, "name": "Read"},
			),
			mkUserToolResult("2026-03-01T10:01:01Z",
				map[string]any{"tool_use_id": id},
			),
		)
	}

	path := writeLiveJSONL(t, entries)

	// tailN=5 — window is the last 5 entries: 1 error pair + 2 success pairs (5 total)
	// but since we take entries[max(0, len-5):], the window contains only clean entries.
	streak, err := ParseLiveConsecutiveErrors(path, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if streak != 0 {
		t.Fatalf("expected streak=0 (tail window only contains successes), got %d", streak)
	}
}

func TestParseLiveConsecutiveErrors_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/empty.jsonl"
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	streak, err := ParseLiveConsecutiveErrors(path, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if streak != 0 {
		t.Fatalf("expected streak=0, got %d", streak)
	}
}

func TestParseLiveConsecutiveErrors_DefaultTailN(t *testing.T) {
	// 60 entries: first 15 are errors, last 45 are clean.
	// With default tailN=50, the window covers entries[10:60].
	// Entries 10-14 are errors (5 errors in window), entries 15-59 are clean.
	// Since the tail ends with clean results, the trailing streak should be 0.
	var entries []map[string]any

	// First 15 pairs (30 entries) — all errors.
	for i := 0; i < 15; i++ {
		id := "e" + string(rune('A'+i))
		entries = append(entries,
			mkAssistantToolUse("2026-03-01T10:00:00Z",
				map[string]any{"id": id, "name": "Edit"},
			),
			mkUserToolResult("2026-03-01T10:00:01Z",
				map[string]any{"tool_use_id": id, "is_error": true},
			),
		)
	}

	// Last 45 pairs (90 entries) — all clean.
	for i := 0; i < 45; i++ {
		id := "c" + string(rune('A'+i%26)) + string(rune('0'+i/26))
		entries = append(entries,
			mkAssistantToolUse("2026-03-01T10:01:00Z",
				map[string]any{"id": id, "name": "Read"},
			),
			mkUserToolResult("2026-03-01T10:01:01Z",
				map[string]any{"tool_use_id": id},
			),
		)
	}

	path := writeLiveJSONL(t, entries)

	// tailN=0 triggers default of 50. Total entries=120 (15+45 pairs * 2).
	// Last 50 entries are all from the clean section, so trailing streak = 0.
	streak, err := ParseLiveConsecutiveErrors(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if streak != 0 {
		t.Fatalf("expected streak=0 (tail-50 window is all clean), got %d", streak)
	}
}

func TestParseLiveConsecutiveErrors_AllErrors(t *testing.T) {
	// 5 error results — expect trailing streak of 5.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Edit"},
			map[string]any{"id": "tu2", "name": "Edit"},
			map[string]any{"id": "tu3", "name": "Edit"},
			map[string]any{"id": "tu4", "name": "Edit"},
			map[string]any{"id": "tu5", "name": "Edit"},
		),
		mkUserToolResult("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1", "is_error": true},
			map[string]any{"tool_use_id": "tu2", "is_error": true},
			map[string]any{"tool_use_id": "tu3", "is_error": true},
			map[string]any{"tool_use_id": "tu4", "is_error": true},
			map[string]any{"tool_use_id": "tu5", "is_error": true},
		),
	})

	streak, err := ParseLiveConsecutiveErrors(path, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if streak != 5 {
		t.Fatalf("expected streak=5, got %d", streak)
	}
}

// ---------- ParseLiveDriftSignal tests ----------

func TestParseLiveDriftSignal_Exploring(t *testing.T) {
	// All reads, no writes anywhere in session → "exploring".
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Read"},
			map[string]any{"id": "tu2", "name": "Grep"},
			map[string]any{"id": "tu3", "name": "Glob"},
		),
	})

	got, err := ParseLiveDriftSignal(path, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != "exploring" {
		t.Fatalf("expected status=exploring, got %q", got.Status)
	}
	if got.HasAnyEdit {
		t.Fatalf("expected HasAnyEdit=false")
	}
	if got.ReadCalls != 3 {
		t.Fatalf("expected ReadCalls=3, got %d", got.ReadCalls)
	}
}

func TestParseLiveDriftSignal_Implementing(t *testing.T) {
	// Mix of reads and writes in window → "implementing".
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Read"},
			map[string]any{"id": "tu2", "name": "Edit"},
			map[string]any{"id": "tu3", "name": "Grep"},
		),
	})

	got, err := ParseLiveDriftSignal(path, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != "implementing" {
		t.Fatalf("expected status=implementing, got %q", got.Status)
	}
	if !got.HasAnyEdit {
		t.Fatalf("expected HasAnyEdit=true")
	}
	if got.WriteCalls != 1 {
		t.Fatalf("expected WriteCalls=1, got %d", got.WriteCalls)
	}
}

func TestParseLiveDriftSignal_Drifting(t *testing.T) {
	// Early edit, then window is all reads (≥60%) with zero writes → "drifting".
	// Build 21 tool calls: 1 early Edit followed by 20 Reads.
	tools := []map[string]any{{"id": "tu0", "name": "Edit"}}
	for i := 1; i <= 20; i++ {
		tools = append(tools, map[string]any{"id": fmt.Sprintf("tu%d", i), "name": "Read"})
	}
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z", tools...),
	})

	got, err := ParseLiveDriftSignal(path, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != "drifting" {
		t.Fatalf("expected status=drifting, got %q (reads=%d writes=%d hasAnyEdit=%v)",
			got.Status, got.ReadCalls, got.WriteCalls, got.HasAnyEdit)
	}
	if !got.HasAnyEdit {
		t.Fatalf("expected HasAnyEdit=true")
	}
	if got.WriteCalls != 0 {
		t.Fatalf("expected WriteCalls=0 in window, got %d", got.WriteCalls)
	}
}

func TestParseLiveDriftSignal_DefaultWindow(t *testing.T) {
	// windowN <= 0 should default to 20.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Read"},
		),
	})

	got, err := ParseLiveDriftSignal(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.WindowN != 1 {
		// Only 1 tool call in file; window capped to actual call count.
		t.Fatalf("expected WindowN=1, got %d", got.WindowN)
	}
}

func TestParseLiveDriftSignal_EmptySession(t *testing.T) {
	path := writeLiveJSONL(t, []map[string]any{})

	got, err := ParseLiveDriftSignal(path, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != "exploring" {
		t.Fatalf("expected status=exploring for empty session, got %q", got.Status)
	}
	if got.WindowN != 0 {
		t.Fatalf("expected WindowN=0, got %d", got.WindowN)
	}
}

// ---------- ParseLiveRepetitiveErrors tests ----------

// mkUserToolResultWithContent builds a JSONL entry for a user message with tool_result
// blocks that include content fields (needed for error pattern extraction).
func mkUserToolResultWithContent(ts string, results ...map[string]any) map[string]any {
	var content []map[string]any
	for _, r := range results {
		block := map[string]any{
			"type":        "tool_result",
			"tool_use_id": r["tool_use_id"],
		}
		if isErr, ok := r["is_error"]; ok {
			block["is_error"] = isErr
		}
		if c, ok := r["content"]; ok {
			block["content"] = c
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

func TestParseLiveRepetitiveErrors_NoErrors(t *testing.T) {
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Read"},
			map[string]any{"id": "tu2", "name": "Bash"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1", "content": "file contents"},
			map[string]any{"tool_use_id": "tu2", "content": "ok"},
		),
	})

	results, err := ParseLiveRepetitiveErrors(path, 100, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected nil/empty results, got %+v", results)
	}
}

func TestParseLiveRepetitiveErrors_ThreeConsecutiveSameError(t *testing.T) {
	// Same tool (Edit) fails 3 times with the same error content.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1", "is_error": true, "content": "old_string not found in file"},
		),
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu2", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu2", "is_error": true, "content": "old_string not found in file"},
		),
		mkAssistantToolUse("2026-03-01T10:00:04Z",
			map[string]any{"id": "tu3", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:05Z",
			map[string]any{"tool_use_id": "tu3", "is_error": true, "content": "old_string not found in file"},
		),
	})

	results, err := ParseLiveRepetitiveErrors(path, 100, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %+v", len(results), results)
	}
	if results[0].Tool != "Edit" {
		t.Errorf("Tool = %q, want Edit", results[0].Tool)
	}
	if results[0].Count != 3 {
		t.Errorf("Count = %d, want 3", results[0].Count)
	}
	if results[0].Pattern != "old_string not found in file" {
		t.Errorf("Pattern = %q, want %q", results[0].Pattern, "old_string not found in file")
	}
}

func TestParseLiveRepetitiveErrors_MixedToolsOnlyOneHitsThreshold(t *testing.T) {
	// Edit fails 3 times (hits threshold), Bash fails 2 times (below threshold).
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1", "is_error": true, "content": "not unique"},
		),
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu2", "name": "Bash"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu2", "is_error": true, "content": "exit code 1"},
		),
		mkAssistantToolUse("2026-03-01T10:00:04Z",
			map[string]any{"id": "tu3", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:05Z",
			map[string]any{"tool_use_id": "tu3", "is_error": true, "content": "not unique"},
		),
		mkAssistantToolUse("2026-03-01T10:00:06Z",
			map[string]any{"id": "tu4", "name": "Bash"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:07Z",
			map[string]any{"tool_use_id": "tu4", "is_error": true, "content": "exit code 1"},
		),
		mkAssistantToolUse("2026-03-01T10:00:08Z",
			map[string]any{"id": "tu5", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:09Z",
			map[string]any{"tool_use_id": "tu5", "is_error": true, "content": "not unique"},
		),
	})

	results, err := ParseLiveRepetitiveErrors(path, 100, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (only Edit hits threshold), got %d: %+v", len(results), results)
	}
	if results[0].Tool != "Edit" {
		t.Errorf("Tool = %q, want Edit", results[0].Tool)
	}
	if results[0].Count != 3 {
		t.Errorf("Count = %d, want 3", results[0].Count)
	}
}

func TestParseLiveRepetitiveErrors_SuccessResetsStreak(t *testing.T) {
	// Edit fails 2 times, then succeeds, then fails 2 times -> never hits 3.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1", "is_error": true, "content": "error msg"},
		),
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu2", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu2", "is_error": true, "content": "error msg"},
		),
		// Success resets the streak.
		mkAssistantToolUse("2026-03-01T10:00:04Z",
			map[string]any{"id": "tu3", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:05Z",
			map[string]any{"tool_use_id": "tu3", "content": "success"},
		),
		// Two more errors after reset.
		mkAssistantToolUse("2026-03-01T10:00:06Z",
			map[string]any{"id": "tu4", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:07Z",
			map[string]any{"tool_use_id": "tu4", "is_error": true, "content": "error msg"},
		),
		mkAssistantToolUse("2026-03-01T10:00:08Z",
			map[string]any{"id": "tu5", "name": "Edit"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:09Z",
			map[string]any{"tool_use_id": "tu5", "is_error": true, "content": "error msg"},
		),
	})

	results, err := ParseLiveRepetitiveErrors(path, 100, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results (success resets streak), got %d: %+v", len(results), results)
	}
}

func TestParseLiveRepetitiveErrors_DifferentPatternsTrackedSeparately(t *testing.T) {
	// Same tool (Bash) but two different error patterns.
	// Pattern A appears 3 times, Pattern B appears 2 times.
	path := writeLiveJSONL(t, []map[string]any{
		mkAssistantToolUse("2026-03-01T10:00:00Z",
			map[string]any{"id": "tu1", "name": "Bash"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:01Z",
			map[string]any{"tool_use_id": "tu1", "is_error": true, "content": "command not found"},
		),
		mkAssistantToolUse("2026-03-01T10:00:02Z",
			map[string]any{"id": "tu2", "name": "Bash"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:03Z",
			map[string]any{"tool_use_id": "tu2", "is_error": true, "content": "permission denied"},
		),
		mkAssistantToolUse("2026-03-01T10:00:04Z",
			map[string]any{"id": "tu3", "name": "Bash"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:05Z",
			map[string]any{"tool_use_id": "tu3", "is_error": true, "content": "command not found"},
		),
		mkAssistantToolUse("2026-03-01T10:00:06Z",
			map[string]any{"id": "tu4", "name": "Bash"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:07Z",
			map[string]any{"tool_use_id": "tu4", "is_error": true, "content": "permission denied"},
		),
		mkAssistantToolUse("2026-03-01T10:00:08Z",
			map[string]any{"id": "tu5", "name": "Bash"},
		),
		mkUserToolResultWithContent("2026-03-01T10:00:09Z",
			map[string]any{"tool_use_id": "tu5", "is_error": true, "content": "command not found"},
		),
	})

	results, err := ParseLiveRepetitiveErrors(path, 100, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (only 'command not found' hits 3), got %d: %+v", len(results), results)
	}
	if results[0].Pattern != "command not found" {
		t.Errorf("Pattern = %q, want %q", results[0].Pattern, "command not found")
	}
	if results[0].Count != 3 {
		t.Errorf("Count = %d, want 3", results[0].Count)
	}
}

func TestParseLiveRepetitiveErrors_TailNLimitsWindow(t *testing.T) {
	// Build a session with 3 Edit errors, but set tailN so small that
	// only the last pair of entries is in the window.
	var allEntries []map[string]any
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("tu%d", i+1)
		allEntries = append(allEntries,
			mkAssistantToolUse("2026-03-01T10:00:00Z",
				map[string]any{"id": id, "name": "Edit"},
			),
			mkUserToolResultWithContent("2026-03-01T10:00:01Z",
				map[string]any{"tool_use_id": id, "is_error": true, "content": "same error"},
			),
		)
	}

	path := writeLiveJSONL(t, allEntries)

	// tailN=2 means only the last 2 entries (1 assistant + 1 user), so only 1 error.
	results, err := ParseLiveRepetitiveErrors(path, 2, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results (tailN limits window to 1 error), got %d: %+v", len(results), results)
	}

	// tailN=0 defaults to 100, which includes all entries.
	results, err = ParseLiveRepetitiveErrors(path, 0, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with default tailN, got %d: %+v", len(results), results)
	}
}
