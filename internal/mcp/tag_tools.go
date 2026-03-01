package mcp

import (
	"encoding/json"
	"errors"

	"github.com/blackwell-systems/claudewatch/internal/store"
)

// SetSessionProjectResult is the response for set_session_project.
type SetSessionProjectResult struct {
	SessionID   string `json:"session_id"`
	ProjectName string `json:"project_name"`
	OK          bool   `json:"ok"`
}

// handleSetSessionProject sets a project name override for a session.
func (s *Server) handleSetSessionProject(args json.RawMessage) (any, error) {
	var params struct {
		SessionID   string `json:"session_id"`
		ProjectName string `json:"project_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if params.ProjectName == "" {
		return nil, errors.New("project_name is required")
	}

	ts := store.NewSessionTagStore(s.tagStorePath)
	if err := ts.Set(params.SessionID, params.ProjectName); err != nil {
		return nil, err
	}

	return SetSessionProjectResult{
		SessionID:   params.SessionID,
		ProjectName: params.ProjectName,
		OK:          true,
	}, nil
}
