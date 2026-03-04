package mcp

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// ProjectAnomaliesResult holds anomaly detection results for a project.
type ProjectAnomaliesResult struct {
	Project   string                 `json:"project"`
	Baseline  *store.ProjectBaseline `json:"baseline,omitempty"`
	Anomalies []store.AnomalyResult  `json:"anomalies"`
}

// addAnomalyTools registers the get_project_anomalies tool on s.
func addAnomalyTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_project_anomalies",
		Description: "Detect anomalous sessions for a project using z-score analysis against a historical baseline. Returns sessions with cost or friction deviating beyond the threshold.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project":{"type":"string","description":"Project name (e.g. 'commitmux'). Omit to use the current session's project."},"threshold":{"type":"number","description":"Z-score threshold for anomaly detection (default 2.0)"}},"additionalProperties":false}`),
		Handler:     s.handleGetProjectAnomalies,
	})
}

// handleGetProjectAnomalies detects anomalous sessions for a project.
// Project resolution follows the same pattern as handleGetProjectHealth:
// active session first, then most recent closed session.
// If no baseline exists in the DB, it computes one on the fly and persists it.
func (s *Server) handleGetProjectAnomalies(args json.RawMessage) (any, error) {
	var params struct {
		Project   *string  `json:"project"`
		Threshold *float64 `json:"threshold"`
	}
	if len(args) > 0 && string(args) != "null" {
		_ = json.Unmarshal(args, &params)
	}

	threshold := 2.0
	if params.Threshold != nil && *params.Threshold > 0 {
		threshold = *params.Threshold
	}

	// Load all session metadata.
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		return nil, err
	}

	tags := s.loadTags()
	weightsPath := s.weightsStorePath
	allWeights := loadAllWeights(weightsPath)

	// Determine the target project name (same pattern as handleGetProjectHealth).
	project := ""
	if params.Project != nil && *params.Project != "" {
		project = *params.Project
	} else {
		// Prefer the active session's project.
		activePath, activeErr := claude.FindActiveSessionPath(s.claudeHome)
		if activeErr == nil && activePath != "" {
			meta, parseErr := claude.ParseActiveSession(activePath)
			if parseErr == nil && meta != nil && meta.ProjectPath != "" {
				project = sessionPrimaryProject(meta.SessionID, meta.ProjectPath, tags, allWeights[meta.SessionID])
			}
		}

		// Fall back to most-recently-closed session.
		if project == "" {
			if len(sessions) == 0 {
				return ProjectAnomaliesResult{
					Anomalies: []store.AnomalyResult{},
				}, nil
			}
			sorted := make([]claude.SessionMeta, len(sessions))
			copy(sorted, sessions)
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].StartTime > sorted[j].StartTime
			})
			project = sessionPrimaryProject(sorted[0].SessionID, sorted[0].ProjectPath, tags, allWeights[sorted[0].SessionID])
		}
	}

	// Filter sessions for the target project.
	var projectSessions []claude.SessionMeta
	for _, sess := range sessions {
		if sessionMatchesProject(sess.SessionID, sess.ProjectPath, tags, allWeights[sess.SessionID], project) {
			projectSessions = append(projectSessions, sess)
		}
	}

	if len(projectSessions) == 0 {
		return ProjectAnomaliesResult{
			Project:   project,
			Anomalies: []store.AnomalyResult{},
		}, nil
	}

	// Load facets (non-fatal if unavailable).
	facets, _ := claude.ParseAllFacets(s.claudeHome)

	pricing := analyzer.DefaultPricing["sonnet"]
	ratio := s.loadCacheRatio()

	// Open the DB and look up the stored baseline.
	db, err := store.Open(config.DBPath())
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	baseline, err := db.GetProjectBaseline(project)
	if err != nil {
		return nil, err
	}

	// If no baseline exists, compute it on the fly.
	if baseline == nil {
		// Build SAWIDs set for the project sessions.
		sawIDs, sawErr := buildSAWIDSet(s.claudeHome, projectSessions)
		if sawErr != nil {
			// Non-fatal: proceed with empty SAW set.
			sawIDs = map[string]bool{}
		}

		computed, computeErr := analyzer.ComputeProjectBaseline(analyzer.BaselineInput{
			Project:    project,
			Sessions:   projectSessions,
			Facets:     facets,
			SAWIDs:     sawIDs,
			Pricing:    pricing,
			CacheRatio: ratio,
		})
		if computeErr != nil {
			// Fewer than 3 sessions — return user-friendly error.
			return nil, fmt.Errorf("insufficient session history for project %s (need ≥3 sessions)", project)
		}

		// Persist the newly computed baseline.
		_ = db.UpsertProjectBaseline(computed) // non-fatal: proceed without persisting

		baseline = &computed
	}

	// Detect anomalies.
	anomalies := analyzer.DetectAnomalies(projectSessions, facets, *baseline, pricing, ratio, threshold)
	if anomalies == nil {
		anomalies = []store.AnomalyResult{}
	}

	return ProjectAnomaliesResult{
		Project:   project,
		Baseline:  baseline,
		Anomalies: anomalies,
	}, nil
}

// buildSAWIDSet parses session transcripts and returns a set of session IDs
// that were detected as SAW (Scout-and-Wave) sessions.
func buildSAWIDSet(claudeHome string, sessions []claude.SessionMeta) (map[string]bool, error) {
	spans, err := claude.ParseSessionTranscripts(claudeHome)
	if err != nil {
		return nil, err
	}

	sawSessions := claude.ComputeSAWWaves(spans)
	sawIDs := make(map[string]bool, len(sawSessions))
	for _, saw := range sawSessions {
		sawIDs[saw.SessionID] = true
	}

	// Only include SAW IDs that are in the project's session set.
	projectIDs := make(map[string]bool, len(sessions))
	for _, sess := range sessions {
		projectIDs[sess.SessionID] = true
	}

	filtered := make(map[string]bool)
	for id := range sawIDs {
		if projectIDs[id] {
			filtered[id] = true
		}
	}

	return filtered, nil
}
