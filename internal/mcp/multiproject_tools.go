package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// SessionProjectsResult holds multi-project attribution for a session.
type SessionProjectsResult struct {
	SessionID      string                 `json:"session_id"`
	PrimaryProject string                 `json:"primary_project"`
	Projects       []claude.ProjectWeight `json:"projects"`
	Live           bool                   `json:"live"`
}

// addMultiProjectTools registers the get_session_projects tool on s.
func addMultiProjectTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_session_projects",
		Description: "Multi-project attribution for a session. Returns weighted project breakdown showing which repos were touched and how much activity each received.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string","description":"Session ID to analyze (optional, defaults to active/most recent)"}},"additionalProperties":false}`),
		Handler:     s.handleGetSessionProjects,
	})
}

// handleGetSessionProjects returns multi-project attribution for the active
// or most recent session. Computes weights on-the-fly from the transcript
// if not cached in the weights store.
func (s *Server) handleGetSessionProjects(args json.RawMessage) (any, error) {
	// Parse optional session_id argument.
	var params struct {
		SessionID string `json:"session_id"`
	}
	if len(args) > 0 && string(args) != "null" {
		_ = json.Unmarshal(args, &params)
	}

	sessionID := params.SessionID
	projectPath := ""
	live := false

	// If session_id is empty, find active or most-recent session.
	if sessionID == "" {
		activePath, err := claude.FindActiveSessionPathForMCP(s.claudeHome)
		if err == nil && activePath != "" {
			meta, parseErr := claude.ParseActiveSession(activePath)
			if parseErr == nil && meta != nil && meta.SessionID != "" {
				sessionID = meta.SessionID
				projectPath = meta.ProjectPath
				live = true
			}
		}

		// If still no session, fall back to most recent closed session.
		if sessionID == "" {
			sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
			if err != nil || len(sessions) == 0 {
				return SessionProjectsResult{
					Projects: []claude.ProjectWeight{},
				}, nil
			}
			sort.Slice(sessions, func(i, j int) bool {
				return sessions[i].StartTime > sessions[j].StartTime
			})
			sessionID = sessions[0].SessionID
			projectPath = sessions[0].ProjectPath
		}
	} else {
		// session_id explicitly provided — look up project path from meta.
		sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
		if err == nil {
			for _, s := range sessions {
				if s.SessionID == sessionID {
					projectPath = s.ProjectPath
					break
				}
			}
		}
	}

	// Derive the weights store path from the tag store path directory.
	weightsStorePath := filepath.Join(filepath.Dir(s.tagStorePath), "session-project-weights.json")
	ws := store.NewSessionProjectWeightsStore(weightsStorePath)

	// Check cache first.
	cached, err := ws.GetWeights(sessionID)
	if err == nil && cached != nil {
		// Convert store.ProjectWeight -> claude.ProjectWeight.
		claudeWeights := make([]claude.ProjectWeight, len(cached))
		for i, sw := range cached {
			claudeWeights[i] = claude.ProjectWeight{
				Project:   sw.Project,
				RepoRoot:  sw.RepoRoot,
				Weight:    sw.Weight,
				ToolCalls: sw.ToolCalls,
			}
		}
		primary := ""
		if len(claudeWeights) > 0 {
			primary = claudeWeights[0].Project
		}
		return SessionProjectsResult{
			SessionID:      sessionID,
			PrimaryProject: primary,
			Projects:       claudeWeights,
			Live:           live,
		}, nil
	}

	// Not cached — find and read the transcript JSONL file.
	entries, transcriptErr := s.findAndReadTranscript(sessionID)
	if transcriptErr != nil {
		// Non-fatal: return empty result.
		return SessionProjectsResult{
			SessionID: sessionID,
			Projects:  []claude.ProjectWeight{},
			Live:      live,
		}, nil
	}

	// Compute weights.
	weights := claude.ComputeProjectWeights(entries, projectPath)
	if weights == nil {
		weights = []claude.ProjectWeight{}
	}

	// Cache computed weights (convert claude.ProjectWeight -> store.ProjectWeight).
	storeWeights := make([]store.ProjectWeight, len(weights))
	for i, cw := range weights {
		storeWeights[i] = store.ProjectWeight{
			Project:   cw.Project,
			RepoRoot:  cw.RepoRoot,
			Weight:    cw.Weight,
			ToolCalls: cw.ToolCalls,
		}
	}
	// Cache failure is non-fatal.
	_ = ws.Set(sessionID, storeWeights)

	primary := ""
	if len(weights) > 0 {
		primary = weights[0].Project
	}

	return SessionProjectsResult{
		SessionID:      sessionID,
		PrimaryProject: primary,
		Projects:       weights,
		Live:           live,
	}, nil
}

// findAndReadTranscript walks the projects directory to find the JSONL file
// for the given sessionID and reads its transcript entries.
func (s *Server) findAndReadTranscript(sessionID string) ([]claude.TranscriptEntry, error) {
	projectsDir := filepath.Join(s.claudeHome, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, err
	}

	target := sessionID + ".jsonl"
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(projectsDir, entry.Name(), target)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return readTranscriptEntries(candidate)
		}
	}

	return nil, os.ErrNotExist
}

// readTranscriptEntries reads a JSONL transcript file and returns all
// successfully parsed entries. Malformed lines are skipped silently.
func readTranscriptEntries(path string) ([]claude.TranscriptEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var transcriptEntries []claude.TranscriptEntry

	scanner := bufio.NewScanner(bytes.NewReader(data))
	// 10MB buffer for long JSONL lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry claude.TranscriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Skip malformed lines.
			continue
		}
		transcriptEntries = append(transcriptEntries, entry)
	}

	return transcriptEntries, nil
}
