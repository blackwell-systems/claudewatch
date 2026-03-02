package mcp

import (
	"encoding/json"
	"errors"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// Standard Claude Sonnet pricing (USD per million tokens).
const (
	sonnetInputPerMillion  = 3.0
	sonnetOutputPerMillion = 15.0
)

// CostVelocityResult holds cost velocity data for the active session.
type CostVelocityResult struct {
	SessionID     string  `json:"session_id"`
	Live          bool    `json:"live"`
	WindowMinutes float64 `json:"window_minutes"`
	WindowCostUSD float64 `json:"window_cost_usd"`
	CostPerMinute float64 `json:"cost_per_minute"`
	Status        string  `json:"status"`
}

// addCostVelocityTools registers the cost velocity MCP tool.
func addCostVelocityTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_cost_velocity",
		Description: "Cost per minute in a rolling 10-minute window for the active session. Returns window cost, cost/minute rate, and status (efficient/normal/burning).",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetCostVelocity,
	})
}

// handleGetCostVelocity returns cost velocity for the active session.
func (s *Server) handleGetCostVelocity(args json.RawMessage) (any, error) {
	activePath, err := claude.FindActiveSessionPath(s.claudeHome)
	if err != nil || activePath == "" {
		return nil, errors.New("no active session found")
	}

	meta, err := claude.ParseActiveSession(activePath)
	if err != nil || meta == nil {
		return nil, errors.New("no active session found")
	}

	pricing := claude.CostPricing{
		InputPerMillion:  sonnetInputPerMillion,
		OutputPerMillion: sonnetOutputPerMillion,
	}

	costStats, err := claude.ParseLiveCostVelocity(activePath, 10, pricing)
	if err != nil {
		return nil, err
	}

	return CostVelocityResult{
		SessionID:     meta.SessionID,
		Live:          true,
		WindowMinutes: costStats.WindowMinutes,
		WindowCostUSD: costStats.WindowCostUSD,
		CostPerMinute: costStats.CostPerMinute,
		Status:        costStats.Status,
	}, nil
}
