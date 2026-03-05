package client

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCallTool_Success tests successful JSON-RPC communication with a mock binary.
func TestCallTool_Success(t *testing.T) {
	// Create a mock MCP server binary that echoes a valid JSON-RPC response
	mockBinary := createMockBinary(t, `#!/bin/sh
cat <<'EOF'
{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"status\":\"ok\"}"}]}}
EOF
`)
	defer func() { _ = os.Remove(mockBinary) }()

	client := NewMCPClient()
	ctx := context.Background()

	result, err := client.CallTool(ctx, mockBinary, "test_tool", map[string]any{"query": "test"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"ok"}`, string(result))
}

// TestCallTool_MissingBinary tests error handling when binary doesn't exist.
func TestCallTool_MissingBinary(t *testing.T) {
	client := NewMCPClient()
	ctx := context.Background()

	_, err := client.CallTool(ctx, "/nonexistent/binary", "test_tool", map[string]any{})
	require.Error(t, err)
	// Error message varies by OS, but should indicate missing binary or command failure
	assert.True(t,
		strings.Contains(err.Error(), "not found") ||
		strings.Contains(err.Error(), "no such file") ||
		strings.Contains(err.Error(), "command failed"),
		"error should indicate missing binary or command failure, got: %s", err.Error())
}

// TestCallTool_JSONRPCError tests handling of JSON-RPC error responses.
func TestCallTool_JSONRPCError(t *testing.T) {
	// Create a mock binary that returns a JSON-RPC error
	mockBinary := createMockBinary(t, `#!/bin/sh
cat <<'EOF'
{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Invalid Request"}}
EOF
`)
	defer func() { _ = os.Remove(mockBinary) }()

	client := NewMCPClient()
	ctx := context.Background()

	_, err := client.CallTool(ctx, mockBinary, "test_tool", map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JSON-RPC error")
	assert.Contains(t, err.Error(), "Invalid Request")
}

// TestCallTool_MalformedJSON tests error handling for malformed JSON responses.
func TestCallTool_MalformedJSON(t *testing.T) {
	// Create a mock binary that returns invalid JSON
	mockBinary := createMockBinary(t, `#!/bin/sh
echo "not valid json"
`)
	defer func() { _ = os.Remove(mockBinary) }()

	client := NewMCPClient()
	ctx := context.Background()

	_, err := client.CallTool(ctx, mockBinary, "test_tool", map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse JSON-RPC response")
}

// TestCallTool_MissingResultContent tests error handling when result.content is missing.
func TestCallTool_MissingResultContent(t *testing.T) {
	// Create a mock binary that returns response without content
	mockBinary := createMockBinary(t, `#!/bin/sh
cat <<'EOF'
{"jsonrpc":"2.0","id":1,"result":{}}
EOF
`)
	defer func() { _ = os.Remove(mockBinary) }()

	client := NewMCPClient()
	ctx := context.Background()

	_, err := client.CallTool(ctx, mockBinary, "test_tool", map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing result.content")
}

// TestCallTool_ContextCancellation tests that context cancellation is respected.
func TestCallTool_ContextCancellation(t *testing.T) {
	// Create a mock binary that sleeps (simulates slow response)
	mockBinary := createMockBinary(t, `#!/bin/sh
sleep 5
echo '{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{}"}]}}'
`)
	defer os.Remove(mockBinary)

	client := NewMCPClient()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.CallTool(ctx, mockBinary, "test_tool", map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
}

// TestFetchAllSources_PartialFailure tests that partial failures are handled gracefully.
func TestFetchAllSources_PartialFailure(t *testing.T) {
	// Create a mock binary for external sources
	mockBinary := createMockBinary(t, `#!/bin/sh
cat <<'EOF'
{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"[{\"item\":\"test\"}]"}]}}
EOF
`)
	defer os.Remove(mockBinary)

	// Create a mock client that fails for specific tools
	client := &mockMCPClient{
		responses: map[string]mockResponse{
			"commitmux_search_memory": {
				data: []byte(`[{"memory":"data"}]`),
				err:  nil,
			},
			"commitmux_search_semantic": {
				data: nil,
				err:  assert.AnError,
			},
		},
	}

	ctx := context.Background()
	results, errs := FetchAllSources(ctx, client, "test query", "test-project", 20)

	// Should have some results despite partial failure
	assert.NotEmpty(t, results)
	assert.NotEmpty(t, errs)

	// Verify successful source returned data
	assert.Contains(t, results, "memory")

	// Verify failed source recorded error
	assert.NotZero(t, len(errs))
}

// TestFetchAllSources_AllSuccess tests successful parallel execution.
func TestFetchAllSources_AllSuccess(t *testing.T) {
	client := &mockMCPClient{
		responses: map[string]mockResponse{
			"commitmux_search_memory": {
				data: []byte(`[{"memory":"data"}]`),
				err:  nil,
			},
			"commitmux_search_semantic": {
				data: []byte(`[{"commit":"data"}]`),
				err:  nil,
			},
		},
	}

	ctx := context.Background()
	results, errs := FetchAllSources(ctx, client, "test query", "test-project", 20)

	// All external sources should succeed (local sources return empty for now)
	assert.NotEmpty(t, results)
	assert.Empty(t, errs)

	// Should have memory and commit results
	assert.Contains(t, results, "memory")
	assert.Contains(t, results, "commit")
}

// createMockBinary creates a temporary executable script for testing.
func createMockBinary(t *testing.T, script string) string {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "mock-mcp-server")

	err := os.WriteFile(binPath, []byte(script), 0755)
	require.NoError(t, err)

	return binPath
}

// mockMCPClient is a mock implementation for testing.
type mockMCPClient struct {
	responses map[string]mockResponse
}

type mockResponse struct {
	data []byte
	err  error
}

func (m *mockMCPClient) CallTool(ctx context.Context, serverBinary string, toolName string, args map[string]any) ([]byte, error) {
	if resp, ok := m.responses[toolName]; ok {
		return resp.data, resp.err
	}
	return []byte("{}"), nil
}

// TestCallToolWithTimeout tests the convenience wrapper.
func TestCallToolWithTimeout(t *testing.T) {
	mockBinary := createMockBinary(t, `#!/bin/sh
cat <<'EOF'
{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{}"}]}}
EOF
`)
	defer os.Remove(mockBinary)

	client := NewMCPClient()

	result, err := CallToolWithTimeout(client, mockBinary, "test_tool", map[string]any{})
	require.NoError(t, err)
	assert.JSONEq(t, "{}", string(result))
}

// TestJSONRPCRequestMarshaling tests that the JSON-RPC request is properly structured.
func TestJSONRPCRequestMarshaling(t *testing.T) {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name": "test_tool",
			"arguments": map[string]any{
				"query": "test",
				"limit": 10,
			},
		},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Verify structure
	var decoded map[string]any
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "2.0", decoded["jsonrpc"])
	assert.Equal(t, float64(1), decoded["id"])
	assert.Equal(t, "tools/call", decoded["method"])

	params := decoded["params"].(map[string]any)
	assert.Equal(t, "test_tool", params["name"])

	args := params["arguments"].(map[string]any)
	assert.Equal(t, "test", args["query"])
	assert.Equal(t, float64(10), args["limit"])
}
