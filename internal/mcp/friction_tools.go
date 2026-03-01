package mcp

import (
	"encoding/json"
	"errors"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// SessionFrictionResult holds friction data for a specific session.
type SessionFrictionResult struct {
	SessionID       string         `json:"session_id"`
	FrictionCounts  map[string]int `json:"friction_counts"`
	TotalFriction   int            `json:"total_friction"`
	TopFrictionType string         `json:"top_friction_type,omitempty"`
}

// handleGetSessionFriction returns friction data for a specific session by session_id.
func (s *Server) handleGetSessionFriction(args json.RawMessage) (any, error) {
	var params struct {
		SessionID string `json:"session_id"`
	}
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, errors.New("session_id is required")
		}
	}
	if params.SessionID == "" {
		return nil, errors.New("session_id is required")
	}

	facets, err := claude.ParseAllFacets(s.claudeHome)
	if err != nil {
		return nil, err
	}

	// Find the matching facet.
	var found *claude.SessionFacet
	for i := range facets {
		if facets[i].SessionID == params.SessionID {
			found = &facets[i]
			break
		}
	}

	// No facet found — return empty result (not an error).
	if found == nil {
		return SessionFrictionResult{
			SessionID:      params.SessionID,
			FrictionCounts: map[string]int{},
			TotalFriction:  0,
		}, nil
	}

	// Build result from found facet.
	frictionCounts := map[string]int{}
	totalFriction := 0
	for k, v := range found.FrictionCounts {
		frictionCounts[k] = v
		totalFriction += v
	}

	// Determine top friction type: highest count, ties broken alphabetically.
	topFrictionType := ""
	if len(frictionCounts) > 0 {
		keys := make([]string, 0, len(frictionCounts))
		for k := range frictionCounts {
			keys = append(keys, k)
		}
		// Sort alphabetically first for deterministic tie-breaking.
		sort.Strings(keys)
		// Find the key with the highest count (stable due to sorted keys).
		topKey := keys[0]
		for _, k := range keys[1:] {
			if frictionCounts[k] > frictionCounts[topKey] {
				topKey = k
			}
		}
		topFrictionType = topKey
	}

	return SessionFrictionResult{
		SessionID:       params.SessionID,
		FrictionCounts:  frictionCounts,
		TotalFriction:   totalFriction,
		TopFrictionType: topFrictionType,
	}, nil
}
