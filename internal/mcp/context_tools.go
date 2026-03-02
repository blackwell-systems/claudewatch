package mcp

import (
	"encoding/json"
	"errors"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// ContextPressureResult holds the MCP response for get_context_pressure.
type ContextPressureResult struct {
	SessionID         string  `json:"session_id"`
	Live              bool    `json:"live"`
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TotalTokens       int     `json:"total_tokens"`
	Compactions       int     `json:"compactions"`
	EstimatedUsage    float64 `json:"estimated_usage"`
	Status            string  `json:"status"`
}

// addContextTools registers the get_context_pressure handler on s.
func addContextTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_context_pressure",
		Description: "How much of the context window has been consumed. Reports token usage, compaction count, and utilization status (comfortable/filling/pressure/critical).",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetContextPressure,
	})
}

// handleGetContextPressure returns context window utilization for the active session.
func (s *Server) handleGetContextPressure(args json.RawMessage) (any, error) {
	activePath, err := claude.FindActiveSessionPath(s.claudeHome)
	if err != nil || activePath == "" {
		return nil, errors.New("no active session found")
	}

	meta, err := claude.ParseActiveSession(activePath)
	if err != nil || meta == nil {
		return nil, errors.New("no active session found")
	}

	stats, err := claude.ParseLiveContextPressure(activePath)
	if err != nil {
		return nil, err
	}

	return ContextPressureResult{
		SessionID:         meta.SessionID,
		Live:              true,
		TotalInputTokens:  stats.TotalInputTokens,
		TotalOutputTokens: stats.TotalOutputTokens,
		TotalTokens:       stats.TotalTokens,
		Compactions:       stats.Compactions,
		EstimatedUsage:    stats.EstimatedUsage,
		Status:            stats.Status,
	}, nil
}
