package mcp

import (
	"encoding/json"
	"errors"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// LiveToolErrorResult holds tool error statistics for the current live session.
type LiveToolErrorResult struct {
	SessionID       string         `json:"session_id"`
	Live            bool           `json:"live"`
	TotalToolUses   int            `json:"total_tool_uses"`
	TotalErrors     int            `json:"total_errors"`
	ErrorRate       float64        `json:"error_rate"`
	ErrorsByTool    map[string]int `json:"errors_by_tool"`
	ConsecutiveErrs int            `json:"consecutive_errors"`
	Severity        string         `json:"severity"`
}

// LiveFrictionResult holds friction statistics for the current live session.
type LiveFrictionResult struct {
	SessionID     string                     `json:"session_id"`
	Live          bool                       `json:"live"`
	Events        []claude.LiveFrictionEvent `json:"events"`
	TotalFriction int                        `json:"total_friction"`
	Truncated     bool                       `json:"truncated"`
	TopType       string                     `json:"top_type,omitempty"`
}

const maxFrictionEvents = 50

// addLiveTools registers the live tool error and friction MCP tools on s.
func addLiveTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_live_tool_errors",
		Description: "Tool error statistics (error rate, errors by tool, consecutive errors, severity) for the currently active Claude Code session.",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetLiveToolErrors,
	})
	s.registerTool(toolDef{
		Name:        "get_live_friction",
		Description: "Friction events (tool errors, retries, error bursts) detected in the currently active Claude Code session.",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetLiveFriction,
	})
}

// handleGetLiveToolErrors returns tool error statistics for the active session.
func (s *Server) handleGetLiveToolErrors(args json.RawMessage) (any, error) {
	activePath, err := claude.FindActiveSessionPath(s.claudeHome)
	if err != nil || activePath == "" {
		return nil, errors.New("no active session found")
	}

	meta, err := claude.ParseActiveSession(activePath)
	if err != nil || meta == nil {
		return nil, errors.New("no active session found")
	}

	stats, err := claude.ParseLiveToolErrors(activePath)
	if err != nil {
		return nil, err
	}

	// Compute severity.
	severity := "clean"
	if stats.ConsecutiveErrs >= 4 || stats.ErrorRate > 0.3 {
		severity = "degraded"
	} else if stats.ErrorRate > 0.1 {
		severity = "mild"
	}

	// Ensure ErrorsByTool is never nil.
	errorsByTool := stats.ErrorsByTool
	if errorsByTool == nil {
		errorsByTool = make(map[string]int)
	}

	return LiveToolErrorResult{
		SessionID:       meta.SessionID,
		Live:            true,
		TotalToolUses:   stats.TotalToolUses,
		TotalErrors:     stats.TotalErrors,
		ErrorRate:       stats.ErrorRate,
		ErrorsByTool:    errorsByTool,
		ConsecutiveErrs: stats.ConsecutiveErrs,
		Severity:        severity,
	}, nil
}

// handleGetLiveFriction returns friction events for the active session.
func (s *Server) handleGetLiveFriction(args json.RawMessage) (any, error) {
	activePath, err := claude.FindActiveSessionPath(s.claudeHome)
	if err != nil || activePath == "" {
		return nil, errors.New("no active session found")
	}

	meta, err := claude.ParseActiveSession(activePath)
	if err != nil || meta == nil {
		return nil, errors.New("no active session found")
	}

	stats, err := claude.ParseLiveFriction(activePath)
	if err != nil {
		return nil, err
	}

	events := stats.Events
	if events == nil {
		events = []claude.LiveFrictionEvent{}
	}

	// Compute TopType from ALL events (before truncation).
	topType := ""
	if len(events) > 0 {
		typeCounts := make(map[string]int)
		for _, ev := range events {
			typeCounts[ev.Type] += ev.Count
		}

		keys := make([]string, 0, len(typeCounts))
		for k := range typeCounts {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		topKey := keys[0]
		for _, k := range keys[1:] {
			if typeCounts[k] > typeCounts[topKey] {
				topKey = k
			}
		}
		topType = topKey
	}

	// Tail to most recent events to bound response size.
	truncated := false
	if len(events) > maxFrictionEvents {
		events = events[len(events)-maxFrictionEvents:]
		truncated = true
	}

	return LiveFrictionResult{
		SessionID:     meta.SessionID,
		Live:          true,
		Events:        events,
		TotalFriction: stats.TotalFriction,
		Truncated:     truncated,
		TopType:       topType,
	}, nil
}
