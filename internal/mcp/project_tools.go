package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// loadAllWeights loads per-session project weights from disk.
// Returns an empty map on any error (non-fatal: missing weights file is normal).
func loadAllWeightsPT(weightsPath string) map[string][]store.ProjectWeight {
	ws := store.NewSessionProjectWeightsStore(weightsPath)
	m, err := ws.Load()
	if err != nil || m == nil {
		return map[string][]store.ProjectWeight{}
	}
	return m
}

// ProjectComparisonResult holds comparison metrics across all projects.
type ProjectComparisonResult struct {
	Projects []ProjectSummary `json:"projects"`
}

// ProjectSummary holds health and performance metrics for a single project.
type ProjectSummary struct {
	Project          string   `json:"project"`
	SessionCount     int      `json:"session_count"`
	HealthScore      int      `json:"health_score"`
	FrictionRate     float64  `json:"friction_rate"`
	HasClaudeMD      bool     `json:"has_claude_md"`
	AgentSuccessRate float64  `json:"agent_success_rate"`
	ZeroCommitRate   float64  `json:"zero_commit_rate"`
	TopFriction      []string `json:"top_friction_types"`
}

// handleGetProjectComparison returns health metrics for all projects, sorted by health score descending.
// Optional arg: min_sessions (int) — if > 0, projects with fewer sessions are excluded.
func (s *Server) handleGetProjectComparison(args json.RawMessage) (any, error) {
	var params struct {
		MinSessions *int `json:"min_sessions"`
	}
	if len(args) > 0 && string(args) != "null" {
		_ = json.Unmarshal(args, &params)
	}
	minSessions := 0
	if params.MinSessions != nil && *params.MinSessions > 0 {
		minSessions = *params.MinSessions
	}

	// Load all session metadata; non-fatal on error.
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil || len(sessions) == 0 {
		return ProjectComparisonResult{Projects: []ProjectSummary{}}, nil
	}

	tags := s.loadTags()

	weightsPath := s.weightsStorePath
	allWeights := loadAllWeightsPT(weightsPath)

	// Group sessions by project base name (with tag/weights override).
	type projectGroup struct {
		sessions    []claude.SessionMeta
		projectPath string
	}
	groups := make(map[string]*projectGroup)
	for _, sess := range sessions {
		var name string
		if w := allWeights[sess.SessionID]; len(w) > 0 {
			name = sessionPrimaryProject(sess.SessionID, sess.ProjectPath, tags, w)
		} else {
			name = resolveProjectName(sess.SessionID, sess.ProjectPath, tags)
		}
		g, ok := groups[name]
		if !ok {
			g = &projectGroup{}
			groups[name] = g
		}
		g.sessions = append(g.sessions, sess)
		if g.projectPath == "" && sess.ProjectPath != "" {
			g.projectPath = sess.ProjectPath
		}
	}

	// Load facets (non-fatal if unavailable).
	facets, _ := claude.ParseAllFacets(s.claudeHome)

	// Build a facet index by session ID.
	facetMap := make(map[string]*claude.SessionFacet, len(facets))
	for i := range facets {
		facetMap[facets[i].SessionID] = &facets[i]
	}

	// Load agent tasks (non-fatal if unavailable).
	agentTasks, _ := claude.ParseAgentTasks(s.claudeHome)

	// Build an agent task index by session ID.
	type taskList []claude.AgentTask
	tasksBySession := make(map[string]taskList, len(agentTasks))
	for _, task := range agentTasks {
		tasksBySession[task.SessionID] = append(tasksBySession[task.SessionID], task)
	}

	// Compute metrics per project group.
	summaries := make([]ProjectSummary, 0, len(groups))
	for projectName, g := range groups {
		projectSessions := g.sessions
		sessionCount := len(projectSessions)

		// Build set of session IDs for this project.
		sessionIDs := make(map[string]struct{}, sessionCount)
		for _, sess := range projectSessions {
			sessionIDs[sess.SessionID] = struct{}{}
		}

		// Compute FrictionRate and friction type counts.
		frictionSessionCount := 0
		frictionTypeCounts := make(map[string]int)
		for _, sess := range projectSessions {
			if facet, ok := facetMap[sess.SessionID]; ok {
				if len(facet.FrictionCounts) > 0 {
					frictionSessionCount++
				}
				for ft, count := range facet.FrictionCounts {
					frictionTypeCounts[ft] += count
				}
			}
		}
		frictionRate := float64(frictionSessionCount) / float64(sessionCount)

		// Compute TopFriction.
		topFriction := topFrictionTypes(frictionTypeCounts, 3)

		// Compute ZeroCommitRate via analyzer.
		commitAnalysis := analyzer.AnalyzeCommits(projectSessions)
		zeroCommitRate := commitAnalysis.ZeroCommitRate

		// Collect agent tasks for this project.
		var projectAgentTasks []claude.AgentTask
		for _, sess := range projectSessions {
			if tasks, ok := tasksBySession[sess.SessionID]; ok {
				projectAgentTasks = append(projectAgentTasks, tasks...)
			}
		}

		// Compute AgentSuccessRate.
		agentSuccessRate := 0.0
		if len(projectAgentTasks) > 0 {
			completedCount := 0
			for _, task := range projectAgentTasks {
				if task.Status == "completed" {
					completedCount++
				}
			}
			agentSuccessRate = float64(completedCount) / float64(len(projectAgentTasks))
		}

		// Compute HasClaudeMD.
		hasClaudeMD := false
		if g.projectPath != "" {
			if _, statErr := os.Stat(filepath.Join(g.projectPath, "CLAUDE.md")); statErr == nil {
				hasClaudeMD = true
			}
		}

		// Compute HealthScore.
		healthScore := 100
		healthScore -= int(frictionRate * 40)
		healthScore -= int(zeroCommitRate * 30)
		healthScore += int(agentSuccessRate * 20)
		if hasClaudeMD {
			healthScore += 10
		}
		if healthScore < 0 {
			healthScore = 0
		}
		if healthScore > 100 {
			healthScore = 100
		}

		summaries = append(summaries, ProjectSummary{
			Project:          projectName,
			SessionCount:     sessionCount,
			HealthScore:      healthScore,
			FrictionRate:     frictionRate,
			HasClaudeMD:      hasClaudeMD,
			AgentSuccessRate: agentSuccessRate,
			ZeroCommitRate:   zeroCommitRate,
			TopFriction:      topFriction,
		})
	}

	// Filter out low-confidence projects when min_sessions is specified.
	if minSessions > 0 {
		filtered := summaries[:0]
		for _, s := range summaries {
			if s.SessionCount >= minSessions {
				filtered = append(filtered, s)
			}
		}
		summaries = filtered
	}

	// Sort descending by HealthScore, then ascending by project name for determinism.
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].HealthScore != summaries[j].HealthScore {
			return summaries[i].HealthScore > summaries[j].HealthScore
		}
		return summaries[i].Project < summaries[j].Project
	})

	return ProjectComparisonResult{Projects: summaries}, nil
}
