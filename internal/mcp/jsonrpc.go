package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/blackwell-systems/claudewatch/internal/config"
)

// Server is an MCP stdio server. It reads JSON-RPC requests from r and
// writes JSON-RPC responses to w. Calls are dispatched to registered tools.
type Server struct {
	tools      []toolDef
	claudeHome string
	budgetUSD  float64
}

// toolDef describes a registered MCP tool.
type toolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     toolHandler
}

// toolHandler is the function signature for MCP tool handlers.
type toolHandler func(args json.RawMessage) (any, error)

// jsonrpcRequest is a JSON-RPC 2.0 request message.
type jsonrpcRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

// jsonrpcResponse is a JSON-RPC 2.0 response message.
type jsonrpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *jsonrpcError    `json:"error,omitempty"`
}

// jsonrpcError represents a JSON-RPC 2.0 error object.
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolsCallParams is the params structure for tools/call requests.
type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// toolsCallResult wraps a tool result as an MCP content response.
type toolsCallResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

// mcpContent is a single content item in an MCP tool response.
type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// toolListEntry is the shape of a single tool in a tools/list response.
type toolListEntry struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// NewServer constructs a Server. cfg provides ClaudeHome for data access.
// budgetUSD of 0.0 means no budget configured.
func NewServer(cfg *config.Config, budgetUSD float64) *Server {
	s := &Server{
		claudeHome: cfg.ClaudeHome,
		budgetUSD:  budgetUSD,
	}
	addTools(s)
	return s
}

// registerTool appends a toolDef to s.tools.
func (s *Server) registerTool(def toolDef) {
	s.tools = append(s.tools, def)
}

// Run blocks, reading JSON-RPC 2.0 messages from r and writing responses to w,
// until ctx is cancelled or r returns EOF. Returns nil on clean shutdown,
// or a non-nil error for unexpected I/O failures.
func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	bw := bufio.NewWriter(w)
	scanner := bufio.NewScanner(r)

	scanDone := make(chan struct{})
	lineCh := make(chan string)
	errCh := make(chan error, 1)

	go func() {
		defer close(scanDone)
		for scanner.Scan() {
			line := scanner.Text()
			select {
			case lineCh <- line:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
		}
		close(lineCh)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case line, ok := <-lineCh:
			if !ok {
				// EOF — clean shutdown
				return nil
			}
			if err := s.handleLine(ctx, line, bw); err != nil {
				return err
			}
		}
	}
}

// handleLine processes a single JSON-RPC line and writes the response.
func (s *Server) handleLine(_ context.Context, line string, bw *bufio.Writer) error {
	var req jsonrpcRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		// Malformed JSON — write parse error if we can't read an id.
		return s.writeError(bw, nil, -32700, "Parse error")
	}

	// Notifications (no id) — write no response.
	if req.ID == nil {
		return nil
	}

	var resp jsonrpcResponse
	resp.JSONRPC = "2.0"
	resp.ID = req.ID

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "claudewatch",
				"version": "0.1.0",
			},
		}

	case "tools/list":
		entries := make([]toolListEntry, 0, len(s.tools))
		for _, t := range s.tools {
			entries = append(entries, toolListEntry{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
		resp.Result = map[string]any{
			"tools": entries,
		}

	case "tools/call":
		var params toolsCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &jsonrpcError{Code: -32602, Message: "Invalid params"}
			break
		}

		// Find the tool.
		var found *toolDef
		for i := range s.tools {
			if s.tools[i].Name == params.Name {
				found = &s.tools[i]
				break
			}
		}

		if found == nil {
			resp.Result = toolsCallResult{
				Content: []mcpContent{{Type: "text", Text: fmt.Sprintf("unknown tool: %s", params.Name)}},
				IsError: true,
			}
			break
		}

		args := params.Arguments
		if args == nil {
			args = json.RawMessage(`{}`)
		}

		result, err := found.Handler(args)
		if err != nil {
			resp.Result = toolsCallResult{
				Content: []mcpContent{{Type: "text", Text: err.Error()}},
				IsError: true,
			}
			break
		}

		// Marshal the tool result to JSON text.
		resultJSON, merr := json.Marshal(result)
		if merr != nil {
			resp.Result = toolsCallResult{
				Content: []mcpContent{{Type: "text", Text: merr.Error()}},
				IsError: true,
			}
			break
		}

		resp.Result = toolsCallResult{
			Content: []mcpContent{{Type: "text", Text: string(resultJSON)}},
			IsError: false,
		}

	default:
		resp.Error = &jsonrpcError{Code: -32601, Message: "Method not found"}
	}

	return s.writeResponse(bw, resp)
}

// writeError writes a JSON-RPC error response with no result.
func (s *Server) writeError(bw *bufio.Writer, id *json.RawMessage, code int, message string) error {
	resp := jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonrpcError{Code: code, Message: message},
	}
	return s.writeResponse(bw, resp)
}

// writeResponse marshals resp as a single JSON line and flushes the writer.
func (s *Server) writeResponse(bw *bufio.Writer, resp jsonrpcResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := bw.Write(data); err != nil {
		return err
	}
	if err := bw.WriteByte('\n'); err != nil {
		return err
	}
	return bw.Flush()
}
