// internal/mcp/jsonrpc.go (TEMPORARY STUB — Agent A provides real impl)
package mcp

import (
	"context"
	"encoding/json"
	"io"

	"github.com/blackwell-systems/claudewatch/internal/config"
)

type toolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     toolHandler
}

type toolHandler func(args json.RawMessage) (any, error)

type Server struct {
	tools      []toolDef
	claudeHome string
	budgetUSD  float64
}

func NewServer(cfg *config.Config, budgetUSD float64) *Server {
	s := &Server{claudeHome: cfg.ClaudeHome, budgetUSD: budgetUSD}
	addTools(s)
	return s
}

func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error { return nil }

func (s *Server) registerTool(def toolDef) { s.tools = append(s.tools, def) }
