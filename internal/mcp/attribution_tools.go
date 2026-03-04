package mcp

import (
	"encoding/json"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// CostAttributionResult wraps the attribution rows for MCP response.
type CostAttributionResult struct {
	SessionID string                  `json:"session_id"`
	Rows      []store.TurnAttribution `json:"rows"`
	TotalCost float64                 `json:"total_cost_usd"`
}

// addAttributionTools registers get_cost_attribution on s.
func addAttributionTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_cost_attribution",
		Description: "Break down token cost by tool type for a session. Answer 'which tool calls consumed most of my budget?' Defaults to most recent session.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string","description":"Session ID to analyze (optional, defaults to most recent)"},"project":{"type":"string","description":"Project name filter (optional, reserved for future use)"}},"additionalProperties":false}`),
		Handler:     s.handleGetCostAttribution,
	})
}

// handleGetCostAttribution computes per-tool-type attribution for a session.
func (s *Server) handleGetCostAttribution(args json.RawMessage) (any, error) {
	var params struct {
		SessionID *string `json:"session_id"`
		Project   *string `json:"project"`
	}
	if len(args) > 0 && string(args) != "null" {
		_ = json.Unmarshal(args, &params)
	}

	sessionID := ""
	if params.SessionID != nil {
		sessionID = *params.SessionID
	}

	ap := analyzer.DefaultPricing["sonnet"]
	pricing := store.ModelPricing{InputPerMillion: ap.InputPerMillion,
		OutputPerMillion: ap.OutputPerMillion, CacheReadPerMillion: ap.CacheReadPerMillion,
		CacheWritePerMillion: ap.CacheWritePerMillion}
	// NOTE: store.ComputeAttribution takes store.ModelPricing (not analyzer.ModelPricing)
	// due to an import cycle — the struct fields are identical.
	rows, err, _ := store.ComputeAttribution(sessionID, s.claudeHome, pricing)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []store.TurnAttribution{}
	}

	var total float64
	for _, r := range rows {
		total += r.EstCostUSD
	}

	resolvedID := sessionID
	if resolvedID == "" {
		resolvedID = "most-recent"
	}

	return CostAttributionResult{
		SessionID: resolvedID,
		Rows:      rows,
		TotalCost: total,
	}, nil
}
