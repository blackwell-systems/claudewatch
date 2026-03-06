package mcp

import (
	"encoding/json"
	"errors"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// DriftSignalResult holds drift signal data for the current live session.
type DriftSignalResult struct {
	SessionID  string `json:"session_id"`
	Live       bool   `json:"live"`
	WindowN    int    `json:"window_n"`
	ReadCalls  int    `json:"read_calls"`
	WriteCalls int    `json:"write_calls"`
	HasAnyEdit bool   `json:"has_any_edit"`
	Status     string `json:"status"` // "exploring", "implementing", "drifting"
}

// addDriftTools registers the get_drift_signal MCP tool on s.
func addDriftTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_drift_signal",
		Description: "Detect exploration drift in the current live session: whether the agent is exploring (read-heavy), implementing (write-active), or drifting (edits exist session-wide but recent window is read-heavy with zero writes).",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetDriftSignal,
	})
}

// handleGetDriftSignal returns the drift signal for the active session.
func (s *Server) handleGetDriftSignal(args json.RawMessage) (any, error) {
	activePath, err := claude.FindActiveSessionPathForMCP(s.claudeHome)
	if err != nil || activePath == "" {
		return nil, errors.New("no active session found")
	}

	meta, err := claude.ParseActiveSession(activePath)
	if err != nil || meta == nil {
		return nil, errors.New("no active session found")
	}

	drift, err := claude.ParseLiveDriftSignal(activePath, 20)
	if err != nil {
		return nil, err
	}

	return DriftSignalResult{
		SessionID:  meta.SessionID,
		Live:       true,
		WindowN:    drift.WindowN,
		ReadCalls:  drift.ReadCalls,
		WriteCalls: drift.WriteCalls,
		HasAnyEdit: drift.HasAnyEdit,
		Status:     drift.Status,
	}, nil
}
