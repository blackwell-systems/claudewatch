package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helper to write a JSONL file in a temp dir and return its path.
func writeJSONL(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestParseSingleTranscript_AgentLaunchAndCompletion(t *testing.T) {
	dir := t.TempDir()
	jsonl := strings.Join([]string{
		// Assistant launches a Task agent.
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_001","name":"Task","input":{"subagent_type":"code-writer","description":"Write tests","prompt":"Write unit tests for foo.go","run_in_background":false}}]}}`,
		// User provides the tool_result (agent completed).
		`{"type":"user","timestamp":"2026-01-15T10:05:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_001","content":"Tests written successfully.","is_error":false}]}}`,
	}, "\n")

	path := writeJSONL(t, dir, "session-abc.jsonl", jsonl)
	spans, err := ParseSingleTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.SessionID != "session-abc" {
		t.Errorf("SessionID = %q, want %q", s.SessionID, "session-abc")
	}
	if s.AgentType != "code-writer" {
		t.Errorf("AgentType = %q, want %q", s.AgentType, "code-writer")
	}
	if s.Description != "Write tests" {
		t.Errorf("Description = %q, want %q", s.Description, "Write tests")
	}
	if s.Prompt != "Write unit tests for foo.go" {
		t.Errorf("Prompt = %q, want %q", s.Prompt, "Write unit tests for foo.go")
	}
	if s.Background {
		t.Error("expected Background = false")
	}
	if !s.Success {
		t.Error("expected Success = true")
	}
	if s.Killed {
		t.Error("expected Killed = false")
	}
	if s.ToolUseID != "tu_001" {
		t.Errorf("ToolUseID = %q, want %q", s.ToolUseID, "tu_001")
	}
	if s.Duration != 5*time.Minute {
		t.Errorf("Duration = %v, want %v", s.Duration, 5*time.Minute)
	}
	if s.ResultLength == 0 {
		t.Error("expected non-zero ResultLength")
	}
}

func TestParseSingleTranscript_AgentKilledViaTaskStop(t *testing.T) {
	dir := t.TempDir()
	jsonl := strings.Join([]string{
		// Launch agent.
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_002","name":"Task","input":{"subagent_type":"researcher","description":"Research topic","prompt":"Find info","run_in_background":true}}]}}`,
		// Progress entry maps agentId to parentToolUseID.
		`{"type":"progress","parentToolUseID":"tu_002","data":{"agentId":"agent-xyz","type":"agent_progress"}}`,
		// TaskStop kills the agent by agentId.
		`{"type":"assistant","timestamp":"2026-01-15T10:02:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_stop","name":"TaskStop","input":{"task_id":"agent-xyz"}}]}}`,
		// Tool result arrives after the kill (agent completed before stop processed).
		`{"type":"user","timestamp":"2026-01-15T10:03:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_002","content":"partial result","is_error":false}]}}`,
	}, "\n")

	path := writeJSONL(t, dir, "session-kill.jsonl", jsonl)
	spans, err := ParseSingleTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if !s.Killed {
		t.Error("expected Killed = true")
	}
	if s.Success {
		t.Error("expected Success = false after kill")
	}
	if !s.Background {
		t.Error("expected Background = true")
	}
	if s.AgentType != "researcher" {
		t.Errorf("AgentType = %q, want %q", s.AgentType, "researcher")
	}
}

func TestParseSingleTranscript_IncompleteAgent(t *testing.T) {
	dir := t.TempDir()
	// Agent launched but no tool_result ever arrives.
	jsonl := `{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_003","name":"Task","input":{"subagent_type":"","description":"Incomplete task","prompt":"Do something","run_in_background":false}}]}}`

	path := writeJSONL(t, dir, "session-incomplete.jsonl", jsonl)
	spans, err := ParseSingleTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(spans) != 1 {
		t.Fatalf("expected 1 span (pending), got %d", len(spans))
	}

	s := spans[0]
	if s.Success {
		t.Error("expected Success = false for incomplete agent")
	}
	// Default agent type when empty.
	if s.AgentType != "general-purpose" {
		t.Errorf("AgentType = %q, want %q", s.AgentType, "general-purpose")
	}
	if !s.CompletedAt.IsZero() {
		t.Errorf("expected zero CompletedAt, got %v", s.CompletedAt)
	}
}

func TestParseSingleTranscript_BackgroundAgent(t *testing.T) {
	dir := t.TempDir()
	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_bg","name":"Task","input":{"subagent_type":"linter","description":"Lint code","prompt":"Run lint","run_in_background":true}}]}}`,
		`{"type":"user","timestamp":"2026-01-15T10:01:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_bg","content":"No issues found","is_error":false}]}}`,
	}, "\n")

	path := writeJSONL(t, dir, "session-bg.jsonl", jsonl)
	spans, err := ParseSingleTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if !spans[0].Background {
		t.Error("expected Background = true")
	}
}

func TestParseSingleTranscript_MultipleAgents(t *testing.T) {
	dir := t.TempDir()
	jsonl := strings.Join([]string{
		// Launch two agents in one assistant message.
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_a","name":"Task","input":{"subagent_type":"writer","description":"Task A","prompt":"Do A","run_in_background":false}},{"type":"tool_use","id":"tu_b","name":"Task","input":{"subagent_type":"reviewer","description":"Task B","prompt":"Do B","run_in_background":true}}]}}`,
		// Both complete.
		`{"type":"user","timestamp":"2026-01-15T10:05:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_a","content":"Done A","is_error":false},{"type":"tool_result","tool_use_id":"tu_b","content":"Done B","is_error":false}]}}`,
	}, "\n")

	path := writeJSONL(t, dir, "session-multi.jsonl", jsonl)
	spans, err := ParseSingleTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	types := map[string]bool{}
	for _, s := range spans {
		types[s.AgentType] = true
	}
	if !types["writer"] || !types["reviewer"] {
		t.Errorf("expected writer and reviewer types, got %v", types)
	}
}

func TestParseSingleTranscript_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	jsonl := strings.Join([]string{
		`not valid json at all`,
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_ok","name":"Task","input":{"subagent_type":"fixer","description":"Fix it","prompt":"Fix","run_in_background":false}}]}}`,
		`{broken`,
		`{"type":"user","timestamp":"2026-01-15T10:01:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_ok","content":"Fixed","is_error":false}]}}`,
	}, "\n")

	path := writeJSONL(t, dir, "session-malformed.jsonl", jsonl)
	spans, err := ParseSingleTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still extract the valid agent span despite malformed lines.
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].AgentType != "fixer" {
		t.Errorf("AgentType = %q, want %q", spans[0].AgentType, "fixer")
	}
}

func TestParseSingleTranscript_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeJSONL(t, dir, "session-empty.jsonl", "")

	spans, err := ParseSingleTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spans) != 0 {
		t.Errorf("expected 0 spans, got %d", len(spans))
	}
}

func TestParseSingleTranscript_LargePromptTruncation(t *testing.T) {
	dir := t.TempDir()
	longPrompt := strings.Repeat("x", 300)
	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_long","name":"Task","input":{"subagent_type":"writer","description":"Long task","prompt":"` + longPrompt + `","run_in_background":false}}]}}`,
		`{"type":"user","timestamp":"2026-01-15T10:01:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_long","content":"done","is_error":false}]}}`,
	}, "\n")

	path := writeJSONL(t, dir, "session-long.jsonl", jsonl)
	spans, err := ParseSingleTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if len(spans[0].Prompt) != 200 {
		t.Errorf("Prompt length = %d, want 200 (truncated)", len(spans[0].Prompt))
	}
}

func TestParseSingleTranscript_ErrorResult(t *testing.T) {
	dir := t.TempDir()
	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_err","name":"Task","input":{"subagent_type":"builder","description":"Build","prompt":"Build it","run_in_background":false}}]}}`,
		`{"type":"user","timestamp":"2026-01-15T10:01:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_err","content":"build failed","is_error":true}]}}`,
	}, "\n")

	path := writeJSONL(t, dir, "session-err.jsonl", jsonl)
	spans, err := ParseSingleTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Success {
		t.Error("expected Success = false for error result")
	}
}

func TestParseSessionTranscripts_IntegrationWithProjectHash(t *testing.T) {
	// Set up a fake claude dir with projects/<hash>/<session>.jsonl
	claudeDir := t.TempDir()
	projectDir := filepath.Join(claudeDir, "projects", "abc123")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-01-15T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_int","name":"Task","input":{"subagent_type":"helper","description":"Help","prompt":"Help me","run_in_background":false}}]}}`,
		`{"type":"user","timestamp":"2026-01-15T10:01:00Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_int","content":"Helped","is_error":false}]}}`,
	}, "\n")
	writeJSONL(t, projectDir, "sess1.jsonl", jsonl)

	spans, err := ParseSessionTranscripts(claudeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].ProjectHash != "abc123" {
		t.Errorf("ProjectHash = %q, want %q", spans[0].ProjectHash, "abc123")
	}
}

func TestParseSessionTranscripts_MissingProjectsDir(t *testing.T) {
	claudeDir := t.TempDir()
	// No projects/ directory exists.
	spans, err := ParseSessionTranscripts(claudeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spans != nil {
		t.Errorf("expected nil spans, got %v", spans)
	}
}

func TestResultContentLength(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		text   string
		expect int
	}{
		{
			name:   "text field takes precedence",
			raw:    `"ignored"`,
			text:   "hello",
			expect: 5,
		},
		{
			name:   "string content",
			raw:    `"result text here"`,
			text:   "",
			expect: len("result text here"),
		},
		{
			name:   "array of blocks",
			raw:    `[{"type":"text","text":"abc"},{"type":"text","text":"de"}]`,
			text:   "",
			expect: 5,
		},
		{
			name:   "nil raw and empty text",
			text:   "",
			expect: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var raw []byte
			if tc.raw != "" {
				raw = []byte(tc.raw)
			}
			got := resultContentLength(raw, tc.text)
			if got != tc.expect {
				t.Errorf("resultContentLength = %d, want %d", got, tc.expect)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		isZero bool
	}{
		{"RFC3339", "2026-01-15T10:00:00Z", false},
		{"RFC3339Nano", "2026-01-15T10:00:00.123456789Z", false},
		{"empty", "", true},
		{"invalid", "not-a-date", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := parseTimestamp(tc.input)
			if tc.isZero && !ts.IsZero() {
				t.Errorf("expected zero time for %q", tc.input)
			}
			if !tc.isZero && ts.IsZero() {
				t.Errorf("expected non-zero time for %q", tc.input)
			}
		})
	}
}
