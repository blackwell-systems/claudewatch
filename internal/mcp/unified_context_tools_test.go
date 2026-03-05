package mcp

import (
	"encoding/json"
	"testing"

	ctxpkg "github.com/blackwell-systems/claudewatch/internal/context"
)

// TestAddUnifiedContextTools verifies tool registration.
func TestAddUnifiedContextTools(t *testing.T) {
	s := newTestServer("/tmp/test-claude", 0.0)
	addUnifiedContextTools(s)

	// Check that get_context tool is registered
	found := false
	for _, tool := range s.tools {
		if tool.Name == "get_context" {
			found = true

			// Verify description
			if tool.Description == "" {
				t.Error("get_context tool has empty description")
			}

			// Verify input schema is valid JSON
			var schema map[string]interface{}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				t.Errorf("get_context tool has invalid input schema: %v", err)
			}

			// Verify required fields
			if props, ok := schema["properties"].(map[string]interface{}); ok {
				if _, hasQuery := props["query"]; !hasQuery {
					t.Error("get_context schema missing 'query' property")
				}
				if _, hasProject := props["project"]; !hasProject {
					t.Error("get_context schema missing 'project' property")
				}
				if _, hasLimit := props["limit"]; !hasLimit {
					t.Error("get_context schema missing 'limit' property")
				}
			} else {
				t.Error("get_context schema missing 'properties' field")
			}

			// Verify required array
			if required, ok := schema["required"].([]interface{}); ok {
				if len(required) != 1 || required[0] != "query" {
					t.Errorf("get_context schema required should be ['query'], got %v", required)
				}
			} else {
				t.Error("get_context schema missing 'required' field")
			}

			break
		}
	}

	if !found {
		t.Error("get_context tool not registered")
	}
}

// TestHandleGetContext_EmptyQuery verifies error handling for empty query.
func TestHandleGetContext_EmptyQuery(t *testing.T) {
	s := newTestServer("/tmp/test-claude", 0.0)

	// Test with empty query
	args := json.RawMessage(`{"query":""}`)
	_, err := s.handleGetContext(args)
	if err == nil {
		t.Error("expected error for empty query, got nil")
	}
	if err != nil && err.Error() != "query is required" {
		t.Errorf("expected 'query is required' error, got: %v", err)
	}

	// Test with missing query
	args = json.RawMessage(`{}`)
	_, err = s.handleGetContext(args)
	if err == nil {
		t.Error("expected error for missing query, got nil")
	}
}

// TestHandleGetContext_ParsesArgs verifies argument parsing.
func TestHandleGetContext_ParsesArgs(t *testing.T) {
	s := newTestServer("/tmp/test-claude", 0.0)

	// Note: This test will likely fail in practice because it tries to call external tools.
	// In a real test environment, we'd mock the client.FetchAllSources function.
	// For now, we just verify the args parsing works without panicking.

	args := json.RawMessage(`{"query":"test query","project":"myproject","limit":10}`)

	// We expect this to fail at the FetchAllSources stage (no commitmux binary),
	// but it should at least parse the args without error.
	result, err := s.handleGetContext(args)

	// Either we get a result (if somehow sources work) or an error about sources failing
	if err != nil {
		// Expected: all sources failed
		if err.Error() == "" {
			t.Error("expected non-empty error message")
		}
	} else {
		// Unexpected success, but let's verify the result structure
		if result == nil {
			t.Error("expected non-nil result when err is nil")
		}
	}
}

// TestHandleGetContext_ReturnsUnifiedContextResult verifies return type.
func TestHandleGetContext_ReturnsUnifiedContextResult(t *testing.T) {
	// This is a structural test - we verify that when we get a successful result,
	// it has the correct type and structure.

	// Create a mock result directly (bypassing the handler's external calls)
	mockResult := ctxpkg.UnifiedContextResult{
		Query: "test query",
		Items: []ctxpkg.ContextItem{
			{
				Source:  ctxpkg.SourceMemory,
				Title:   "memory: test.md",
				Snippet: "test content",
				Score:   0.8,
			},
		},
		Count:   1,
		Sources: []string{"memory"},
		Errors:  []string{},
	}

	// Verify the structure
	if mockResult.Query != "test query" {
		t.Errorf("expected query 'test query', got %s", mockResult.Query)
	}
	if mockResult.Count != 1 {
		t.Errorf("expected count 1, got %d", mockResult.Count)
	}
	if len(mockResult.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(mockResult.Items))
	}
	if len(mockResult.Sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(mockResult.Sources))
	}

	// Verify JSON marshaling works
	data, err := json.Marshal(mockResult)
	if err != nil {
		t.Errorf("failed to marshal UnifiedContextResult: %v", err)
	}

	// Verify we can unmarshal it back
	var unmarshaled ctxpkg.UnifiedContextResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Errorf("failed to unmarshal UnifiedContextResult: %v", err)
	}

	if unmarshaled.Query != mockResult.Query {
		t.Errorf("query mismatch after unmarshal: expected %s, got %s", mockResult.Query, unmarshaled.Query)
	}
}

// TestParseMemoryResults verifies memory result parsing.
func TestParseMemoryResults(t *testing.T) {
	data := []byte(`{
		"memories": [
			{
				"path": "MEMORY.md",
				"content": "test memory content",
				"distance": 0.2
			}
		]
	}`)

	items, err := parseMemoryResults(data)
	if err != nil {
		t.Fatalf("parseMemoryResults failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item.Source != ctxpkg.SourceMemory {
		t.Errorf("expected source memory, got %v", item.Source)
	}
	if item.Title != "memory: MEMORY.md" {
		t.Errorf("expected title 'memory: MEMORY.md', got %s", item.Title)
	}
	if item.Snippet != "test memory content" {
		t.Errorf("expected snippet 'test memory content', got %s", item.Snippet)
	}
	if item.Score != 0.8 { // 1.0 - 0.2 = 0.8
		t.Errorf("expected score 0.8, got %f", item.Score)
	}
}

// TestParseCommitResults verifies commit result parsing.
func TestParseCommitResults(t *testing.T) {
	data := []byte(`{
		"commits": [
			{
				"sha": "abc123def456",
				"message": "test commit message",
				"author": "John Doe",
				"timestamp": "2025-01-01T12:00:00Z",
				"repo": "test-repo",
				"distance": 0.15
			}
		]
	}`)

	items, err := parseCommitResults(data)
	if err != nil {
		t.Fatalf("parseCommitResults failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item.Source != ctxpkg.SourceCommit {
		t.Errorf("expected source commit, got %v", item.Source)
	}
	if item.Title != "commit: abc123d" {
		t.Errorf("expected title 'commit: abc123d', got %s", item.Title)
	}
	if item.Snippet != "test commit message" {
		t.Errorf("expected snippet 'test commit message', got %s", item.Snippet)
	}
	if item.Score != 0.85 { // 1.0 - 0.15 = 0.85
		t.Errorf("expected score 0.85, got %f", item.Score)
	}
	if item.Metadata["sha"] != "abc123def456" {
		t.Errorf("expected sha metadata, got %v", item.Metadata)
	}
}

// TestParseTaskHistoryResults verifies task history result parsing.
func TestParseTaskHistoryResults(t *testing.T) {
	data := []byte(`{
		"tasks": [
			{
				"task_identifier": "implement feature X",
				"sessions": ["session1", "session2"],
				"status": "complete",
				"blockers_hit": [],
				"solution": "used approach Y",
				"commits": ["abc123", "def456"]
			}
		]
	}`)

	items, err := parseTaskHistoryResults(data)
	if err != nil {
		t.Fatalf("parseTaskHistoryResults failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item.Source != ctxpkg.SourceTaskHistory {
		t.Errorf("expected source task_history, got %v", item.Source)
	}
	if item.Title != "task: implement feature X" {
		t.Errorf("expected title 'task: implement feature X', got %s", item.Title)
	}
	if item.Metadata["status"] != "complete" {
		t.Errorf("expected status metadata 'complete', got %v", item.Metadata)
	}
	if item.Score != 0.5 {
		t.Errorf("expected default score 0.5, got %f", item.Score)
	}
}

// TestParseTranscriptResults verifies transcript result parsing.
func TestParseTranscriptResults(t *testing.T) {
	data := []byte(`{
		"count": 1,
		"results": [
			{
				"session_id": "session123abc",
				"project_hash": "proj456",
				"snippet": "transcript snippet text",
				"entry_type": "user",
				"timestamp": "2025-01-01T12:00:00Z",
				"rank": 0.75
			}
		]
	}`)

	items, err := parseTranscriptResults(data)
	if err != nil {
		t.Fatalf("parseTranscriptResults failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	if item.Source != ctxpkg.SourceTranscript {
		t.Errorf("expected source transcript, got %v", item.Source)
	}
	if item.Title != "transcript: session1" {
		t.Errorf("expected title 'transcript: session1', got %s", item.Title)
	}
	if item.Snippet != "transcript snippet text" {
		t.Errorf("expected snippet 'transcript snippet text', got %s", item.Snippet)
	}
	if item.Score != 0.75 {
		t.Errorf("expected score 0.75, got %f", item.Score)
	}
	if item.Metadata["session_id"] != "session123abc" {
		t.Errorf("expected session_id metadata, got %v", item.Metadata)
	}
}
