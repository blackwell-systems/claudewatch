package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// ProjectHealthResult holds aggregate health metrics for a single project.
type ProjectHealthResult struct {
	Project          string                      `json:"project"`
	SessionCount     int                         `json:"session_count"`
	FrictionRate     float64                     `json:"friction_rate"`
	TopFriction      []string                    `json:"top_friction_types"`
	AvgToolErrors    float64                     `json:"avg_tool_errors_per_session"`
	ZeroCommitRate   float64                     `json:"zero_commit_rate"`
	AgentSuccessRate float64                     `json:"agent_success_rate"`
	HasClaudeMD      bool                        `json:"has_claude_md"`
	ByAgentType      map[string]AgentTypeSummary `json:"agent_performance_by_type"`
}

// AgentTypeSummary holds count and success rate for a single agent type.
type AgentTypeSummary struct {
	Count       int     `json:"count"`
	SuccessRate float64 `json:"success_rate"`
}

// handleGetProjectHealth returns health metrics for a project.
// The optional "project" argument selects the project by base name; if omitted,
// the most recent session's project is used.
func (s *Server) handleGetProjectHealth(args json.RawMessage) (any, error) {
	// Parse optional project argument.
	var params struct {
		Project *string `json:"project"`
	}
	if len(args) > 0 && string(args) != "null" {
		_ = json.Unmarshal(args, &params)
	}

	// Load all session metadata; errors are fatal here since we need at least this data.
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		return nil, err
	}

	// Determine the target project name.
	project := ""
	if params.Project != nil && *params.Project != "" {
		project = *params.Project
	} else {
		// Default: most recent session's project.
		if len(sessions) == 0 {
			return ProjectHealthResult{
				TopFriction: []string{},
				ByAgentType: map[string]AgentTypeSummary{},
			}, nil
		}
		sorted := make([]claude.SessionMeta, len(sessions))
		copy(sorted, sessions)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].StartTime > sorted[j].StartTime
		})
		project = filepath.Base(sorted[0].ProjectPath)
	}

	// Filter sessions for the target project.
	var projectSessions []claude.SessionMeta
	var projectPath string
	sessionIDs := make(map[string]struct{})
	for _, s := range sessions {
		if filepath.Base(s.ProjectPath) == project {
			projectSessions = append(projectSessions, s)
			sessionIDs[s.SessionID] = struct{}{}
			if projectPath == "" && s.ProjectPath != "" {
				projectPath = s.ProjectPath
			}
		}
	}

	// Return zero-value result if no sessions exist for the project.
	if len(projectSessions) == 0 {
		return ProjectHealthResult{
			Project:      project,
			SessionCount: 0,
			TopFriction:  []string{},
			ByAgentType:  map[string]AgentTypeSummary{},
		}, nil
	}

	// Load facets (non-fatal if unavailable).
	facets, _ := claude.ParseAllFacets(s.claudeHome)

	// Compute FrictionRate: fraction of project sessions with any friction.
	frictionSessionCount := 0
	frictionTypeCounts := make(map[string]int)
	for _, facet := range facets {
		if _, ok := sessionIDs[facet.SessionID]; !ok {
			continue
		}
		if len(facet.FrictionCounts) > 0 {
			frictionSessionCount++
		}
		for frictionType, count := range facet.FrictionCounts {
			frictionTypeCounts[frictionType] += count
		}
	}
	frictionRate := 0.0
	if len(projectSessions) > 0 {
		frictionRate = float64(frictionSessionCount) / float64(len(projectSessions))
	}

	// Compute TopFriction: top 3 friction types by frequency.
	topFriction := topFrictionTypes(frictionTypeCounts, 3)

	// Compute AvgToolErrors.
	var totalToolErrors int
	for _, sess := range projectSessions {
		totalToolErrors += sess.ToolErrors
	}
	avgToolErrors := float64(totalToolErrors) / float64(len(projectSessions))

	// Compute ZeroCommitRate via analyzer.
	commitAnalysis := analyzer.AnalyzeCommits(projectSessions)
	zeroCommitRate := commitAnalysis.ZeroCommitRate

	// Load agent tasks (non-fatal if unavailable).
	agentTasks, _ := claude.ParseAgentTasks(s.claudeHome)

	// Filter agent tasks by project session IDs.
	var projectAgentTasks []claude.AgentTask
	for _, task := range agentTasks {
		if _, ok := sessionIDs[task.SessionID]; ok {
			projectAgentTasks = append(projectAgentTasks, task)
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

	// Compute ByAgentType.
	byAgentType := computeByAgentType(projectAgentTasks)

	// Compute HasClaudeMD.
	hasClaudeMD := false
	if projectPath != "" {
		claudeMDPath := filepath.Join(projectPath, "CLAUDE.md")
		if _, statErr := os.Stat(claudeMDPath); statErr == nil {
			hasClaudeMD = true
		}
	}

	return ProjectHealthResult{
		Project:          project,
		SessionCount:     len(projectSessions),
		FrictionRate:     frictionRate,
		TopFriction:      topFriction,
		AvgToolErrors:    avgToolErrors,
		ZeroCommitRate:   zeroCommitRate,
		AgentSuccessRate: agentSuccessRate,
		HasClaudeMD:      hasClaudeMD,
		ByAgentType:      byAgentType,
	}, nil
}

// topFrictionTypes returns up to n friction type names sorted by descending count.
// Returns []string{} (not nil) when there are no friction types.
func topFrictionTypes(counts map[string]int, n int) []string {
	type entry struct {
		name  string
		count int
	}

	entries := make([]entry, 0, len(counts))
	for name, count := range counts {
		entries = append(entries, entry{name: name, count: count})
	}

	// Sort descending by count, then ascending by name for determinism.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].name < entries[j].name
	})

	limit := n
	if limit > len(entries) {
		limit = len(entries)
	}

	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = entries[i].name
	}
	return result
}

// computeByAgentType groups agent tasks by type and computes count and success rate.
func computeByAgentType(tasks []claude.AgentTask) map[string]AgentTypeSummary {
	type bucket struct {
		count     int
		completed int
	}

	buckets := make(map[string]*bucket)
	for _, task := range tasks {
		b, ok := buckets[task.AgentType]
		if !ok {
			b = &bucket{}
			buckets[task.AgentType] = b
		}
		b.count++
		if task.Status == "completed" {
			b.completed++
		}
	}

	result := make(map[string]AgentTypeSummary, len(buckets))
	for agentType, b := range buckets {
		successRate := 0.0
		if b.count > 0 {
			successRate = float64(b.completed) / float64(b.count)
		}
		result[agentType] = AgentTypeSummary{
			Count:       b.count,
			SuccessRate: successRate,
		}
	}
	return result
}
