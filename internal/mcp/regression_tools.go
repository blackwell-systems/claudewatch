package mcp

import (
	"encoding/json"
	"path/filepath"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// addRegressionTools registers the get_regression_status MCP tool on s.
func addRegressionTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_regression_status",
		Description: "Regression status for a project: whether friction rate or cost-per-session has exceeded a threshold multiplier relative to the stored baseline. Returns current vs baseline metrics.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project":{"type":"string","description":"Project name (e.g. 'commitmux'). Omit to use the current session's project."},"threshold":{"type":"number","description":"Regression multiplier over baseline (default 1.5). Must be > 1."}},"additionalProperties":false}`),
		Handler:     s.handleGetRegressionStatus,
	})
}

// handleGetRegressionStatus returns the regression status for a project.
// Project resolution follows the same pattern as handleGetProjectAnomalies:
// active session first, then most recent closed session.
// If no baseline exists in the DB, the result will have HasBaseline: false.
func (s *Server) handleGetRegressionStatus(args json.RawMessage) (any, error) {
	var params struct {
		Project   *string  `json:"project"`
		Threshold *float64 `json:"threshold"`
	}
	if len(args) > 0 && string(args) != "null" {
		_ = json.Unmarshal(args, &params)
	}

	// Threshold is passed through as-is (0 if not provided — Agent A defaults 0 → 1.5).
	threshold := 0.0
	if params.Threshold != nil {
		threshold = *params.Threshold
	}

	// Load all session metadata.
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		return nil, err
	}

	tags := s.loadTags()
	weightsPath := filepath.Join(filepath.Dir(s.tagStorePath), "session-project-weights.json")
	allWeights := loadAllWeights(weightsPath)

	// Determine the target project name (same pattern as handleGetProjectAnomalies).
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
				return analyzer.RegressionStatus{
					HasBaseline: false,
					Message:     "no sessions found",
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

	// Load facets (non-fatal if unavailable).
	facets, _ := claude.ParseAllFacets(s.claudeHome)

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

	// Select recent sessions: sort descending, take min(10, len).
	recentSessions := projectSessions
	if len(recentSessions) > 0 {
		sorted := make([]claude.SessionMeta, len(recentSessions))
		copy(sorted, recentSessions)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].StartTime > sorted[j].StartTime
		})
		limit := len(sorted)
		if limit > 10 {
			limit = 10
		}
		recentSessions = sorted[:limit]
	}

	// Compute and return regression status.
	return analyzer.ComputeRegressionStatus(analyzer.RegressionInput{
		Project:        project,
		Baseline:       baseline,
		RecentSessions: recentSessions,
		Facets:         facets,
		Pricing:        analyzer.DefaultPricing["sonnet"],
		CacheRatio:     s.loadCacheRatio(),
		Threshold:      threshold,
	}), nil
}
