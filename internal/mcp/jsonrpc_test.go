package mcp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/config"
)

// newTestServer creates a Server with an empty config for use in tests.
func newEmptyServer() *Server {
	cfg := &config.Config{ClaudeHome: "/tmp/test-claude-home"}
	return NewServer(cfg, 0)
}

// runServer starts s.Run in a goroutine piped through pw/pr and returns
// a function that writes a request line and reads the response line.
// Close pw to trigger EOF. The returned cleanup func cancels the context.
func runServer(t *testing.T, s *Server) (
	sendLine func(line string) string,
	closePipe func(),
	cleanup func(),
) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	// Pipe: test writes to pw, server reads from pr.
	pr, pw := io.Pipe()
	// Pipe: server writes to sw, test reads from sr.
	sr, sw := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx, pr, sw)
	}()

	sendLine = func(line string) string {
		_, err := io.WriteString(pw, line+"\n")
		if err != nil {
			t.Fatalf("sendLine write: %v", err)
		}

		// Read one response line.
		buf := make([]byte, 1<<16)
		var out strings.Builder
		for {
			n, err := sr.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
				s := out.String()
				if idx := strings.IndexByte(s, '\n'); idx >= 0 {
					return s[:idx]
				}
			}
			if err != nil {
				t.Fatalf("sendLine read: %v", err)
			}
		}
	}

	closePipe = func() {
		_ = pw.Close()
	}

	cleanup = func() {
		cancel()
		_ = pw.Close()
		// Drain done channel.
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Run did not return after cancel+close")
		}
	}

	return sendLine, closePipe, cleanup
}

// TestRun_Initialize verifies the server responds to "initialize" with the
// correct protocolVersion and serverInfo.name.
func TestRun_Initialize(t *testing.T) {
	s := newEmptyServer()
	sendLine, _, cleanup := runServer(t, s)
	defer cleanup()

	req := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	resp := sendLine(req)

	var parsed struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("unmarshal response: %v\nresponse: %s", err, resp)
	}
	if parsed.Result.ProtocolVersion == "" {
		t.Errorf("expected non-empty protocolVersion, got empty string; response: %s", resp)
	}
	if parsed.Result.ServerInfo.Name != "claudewatch" {
		t.Errorf("expected serverInfo.name == 'claudewatch', got %q; response: %s",
			parsed.Result.ServerInfo.Name, resp)
	}
}

// TestRun_ToolsList verifies the server responds to "tools/list" with a list
// of tools. Because the stub addTools is empty, we register a test tool
// directly and assert >= 1 tool. In the real build (after Agent B merges),
// addTools registers >= 3 tools; that assertion belongs to Agent B's tests.
func TestRun_ToolsList(t *testing.T) {
	s := newEmptyServer()
	// Register a test tool directly so we can assert a non-empty list.
	s.registerTool(toolDef{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Handler: func(args json.RawMessage) (any, error) {
			return map[string]string{"ok": "true"}, nil
		},
	})

	sendLine, _, cleanup := runServer(t, s)
	defer cleanup()

	req := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	resp := sendLine(req)

	var parsed struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("unmarshal response: %v\nresponse: %s", err, resp)
	}
	// NOTE: With the stub tools.go, addTools is a no-op. We manually registered
	// one test tool above so we can assert >= 1. When Agent B replaces tools.go
	// with real tool registrations, this test will see >= 4 tools (3 + test_tool),
	// which still satisfies the assertion.
	if len(parsed.Result.Tools) < 1 {
		t.Errorf("expected >= 1 tools in list, got %d; response: %s",
			len(parsed.Result.Tools), resp)
	}
	for _, tool := range parsed.Result.Tools {
		if tool.Name == "" {
			t.Errorf("tool has empty name; response: %s", resp)
		}
	}
}

// TestRun_UnknownMethod verifies that an unknown method returns JSON-RPC
// error code -32601.
func TestRun_UnknownMethod(t *testing.T) {
	s := newEmptyServer()
	sendLine, _, cleanup := runServer(t, s)
	defer cleanup()

	req := `{"jsonrpc":"2.0","id":3,"method":"nonexistent/method"}`
	resp := sendLine(req)

	var parsed struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
		t.Fatalf("unmarshal response: %v\nresponse: %s", err, resp)
	}
	if parsed.Error == nil {
		t.Fatalf("expected error in response, got none; response: %s", resp)
	}
	if parsed.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d; response: %s", parsed.Error.Code, resp)
	}
}

// TestRun_Notification verifies that a message without an "id" field
// (a JSON-RPC notification) produces no response.
func TestRun_Notification(t *testing.T) {
	s := newEmptyServer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	sr, sw := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx, pr, sw)
	}()

	// Send a notification (no "id" field).
	notification := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	if _, err := io.WriteString(pw, notification); err != nil {
		t.Fatalf("write notification: %v", err)
	}

	// After writing the notification, attempt to read a response with a short
	// deadline. We expect nothing to be written.
	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 1024)
		n, _ := sr.Read(buf)
		readDone <- buf[:n]
	}()

	select {
	case data := <-readDone:
		t.Errorf("expected no response for notification, but got: %s", data)
	case <-time.After(100 * time.Millisecond):
		// Correct: no response was written within the deadline.
	}

	// Clean up.
	cancel()
	_ = pw.Close()
	_ = sr.Close()
}

// TestRun_ContextCancel verifies that cancelling the context causes Run to
// return nil.
func TestRun_ContextCancel(t *testing.T) {
	s := newEmptyServer()
	ctx, cancel := context.WithCancel(context.Background())

	pr, pw := io.Pipe()
	_, sw := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx, pr, sw)
	}()

	// Cancel the context and expect Run to return nil.
	cancel()
	_ = pw.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected Run to return nil on context cancel, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Run did not return after context cancel")
	}
}

// TestRun_EOFClean verifies that closing the writer side of the input pipe
// causes Run to return nil (clean EOF).
func TestRun_EOFClean(t *testing.T) {
	s := newEmptyServer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	_, sw := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx, pr, sw)
	}()

	// Close the write side to signal EOF.
	_ = pw.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected Run to return nil on EOF, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Run did not return after EOF")
	}
}
