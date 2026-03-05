package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// MCPClient calls external MCP tools via stdio JSON-RPC.
type MCPClient interface {
	// CallTool execs the MCP server binary and calls a tool.
	// Returns the tool result as JSON bytes, or error.
	CallTool(ctx context.Context, serverBinary string, toolName string, args map[string]any) ([]byte, error)
}

// stdioMCPClient implements MCPClient by executing MCP server binaries
// and communicating via stdio JSON-RPC 2.0.
type stdioMCPClient struct{}

// NewMCPClient constructs a default stdio-based MCP client.
func NewMCPClient() MCPClient {
	return &stdioMCPClient{}
}

// jsonRPCRequest represents a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      int            `json:"id"`
	Result  *jsonRPCResult `json:"result,omitempty"`
	Error   *jsonRPCError  `json:"error,omitempty"`
}

// jsonRPCResult represents the result field of a JSON-RPC response.
type jsonRPCResult struct {
	Content []jsonRPCContent `json:"content"`
}

// jsonRPCContent represents a content item in the result.
type jsonRPCContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// jsonRPCError represents the error field of a JSON-RPC response.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// CallTool executes the MCP server binary and calls the specified tool.
func (c *stdioMCPClient) CallTool(ctx context.Context, serverBinary string, toolName string, args map[string]any) ([]byte, error) {
	// Construct JSON-RPC 2.0 request
	request := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON-RPC request: %w", err)
	}

	// Create command with context for timeout/cancellation
	cmd := exec.CommandContext(ctx, serverBinary)
	cmd.Stdin = bytes.NewReader(requestBytes)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("command cancelled: %w", ctx.Err())
		}
		// Check if binary doesn't exist
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == exec.ErrNotFound {
			return nil, fmt.Errorf("MCP server binary not found: %s", serverBinary)
		}
		return nil, fmt.Errorf("command failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	// Parse JSON-RPC response
	var response jsonRPCResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON-RPC response: %w (output: %s)", err, stdout.String())
	}

	// Check for JSON-RPC error
	if response.Error != nil {
		return nil, fmt.Errorf("JSON-RPC error %d: %s", response.Error.Code, response.Error.Message)
	}

	// Extract result.content[0].text
	if response.Result == nil || len(response.Result.Content) == 0 {
		return nil, errors.New("JSON-RPC response missing result.content")
	}

	if response.Result.Content[0].Type != "text" {
		return nil, fmt.Errorf("unexpected content type: %s", response.Result.Content[0].Type)
	}

	// Return the text field as raw JSON bytes
	return []byte(response.Result.Content[0].Text), nil
}

// SourceResult holds the result from a single source query.
type SourceResult struct {
	Source string
	Data   []byte
	Error  error
}

// FetchAllSources queries 4 sources in parallel and returns results + errors.
// Returns map[source]jsonBytes for successful queries and a slice of errors for failures.
// Partial failure handling: accumulates errors but returns results from successful sources.
// Graceful degradation: if commitmux is not available, skips memory and commit sources.
func FetchAllSources(ctx context.Context, client MCPClient, query string, project string, limit int) (map[string][]byte, []error) {
	results := make(map[string][]byte)
	var errs []error

	// Detect commitmux availability
	commitmuxBinary := detectCommitmux()
	commitmuxAvailable := commitmuxBinary != ""

	// Calculate per-source limit (distribute total limit across active sources)
	activeSources := 2 // task_history + transcript always available
	if commitmuxAvailable {
		activeSources = 4 // add memory + commit
	}
	perSourceLimit := limit / activeSources
	if perSourceLimit < 1 {
		perSourceLimit = 1
	}

	// Use errgroup for parallel execution with context cancellation
	g, gctx := errgroup.WithContext(ctx)
	resultChan := make(chan SourceResult, 4)

	// Define all sources
	sources := []struct {
		name       string
		binary     string
		toolName   string
		isExternal bool
		skip       bool
	}{
		{"memory", commitmuxBinary, "commitmux_search_memory", true, !commitmuxAvailable},
		{"commit", commitmuxBinary, "commitmux_search_semantic", true, !commitmuxAvailable},
		{"task_history", "", "get_task_history", false, false},
		{"transcript", "", "search_transcripts", false, false},
	}

	for _, src := range sources {
		src := src // capture for closure

		// Skip unavailable sources
		if src.skip {
			continue
		}

		g.Go(func() error {
			// Build arguments
			args := map[string]any{
				"query": query,
				"limit": perSourceLimit,
			}
			if project != "" && src.name != "transcript" {
				args["project"] = project
			}

			var data []byte
			var err error

			if src.isExternal {
				// Call external MCP binary
				data, err = client.CallTool(gctx, src.binary, src.toolName, args)
			} else {
				// For local tools, we skip them in this implementation
				// Agent D will wire these up properly in Wave 2
				// For now, just return empty result
				data = []byte("{}")
				err = nil
			}

			resultChan <- SourceResult{
				Source: src.name,
				Data:   data,
				Error:  err,
			}
			return nil // Don't propagate errors via errgroup (we want partial results)
		})
	}

	// Wait for all goroutines to complete
	_ = g.Wait()
	close(resultChan)

	// Collect results
	for result := range resultChan {
		if result.Error != nil {
			errs = append(errs, fmt.Errorf("%s: %w", result.Source, result.Error))
		} else {
			results[result.Source] = result.Data
		}
	}

	// Add informational message if commitmux unavailable
	if !commitmuxAvailable {
		errs = append(errs, fmt.Errorf("commitmux not found: memory and commit sources unavailable. Install commitmux for full context search: brew install blackwell-systems/tap/commitmux"))
	}

	return results, errs
}

// detectCommitmux checks for commitmux binary availability.
// Returns the full path to commitmux if found, empty string otherwise.
func detectCommitmux() string {
	// Try hardcoded path first (common install location)
	hardcodedPath := "/Users/dayna.blackwell/.cargo/bin/commitmux"
	if fileExists(hardcodedPath) {
		return hardcodedPath
	}

	// TODO: Add config option for custom path
	// TODO: Try PATH lookup as fallback

	return ""
}

// fileExists checks if a file exists and is executable.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// Check if it's a regular file and executable
	return !info.IsDir() && (info.Mode()&0111 != 0)
}

// CallToolWithTimeout is a convenience wrapper that adds a default 30s timeout.
func CallToolWithTimeout(client MCPClient, serverBinary string, toolName string, args map[string]any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return client.CallTool(ctx, serverBinary, toolName, args)
}
