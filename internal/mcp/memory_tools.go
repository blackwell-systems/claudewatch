package mcp

import (
	"encoding/json"
	"path/filepath"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// TaskHistoryResult holds a list of matching task memories.
type TaskHistoryResult struct {
	Tasks []TaskMemoryResult `json:"tasks"`
}

// TaskMemoryResult is the MCP-facing representation of a task memory.
type TaskMemoryResult struct {
	TaskIdentifier string   `json:"task_identifier"`
	Sessions       []string `json:"sessions"`
	Status         string   `json:"status"`
	BlockersHit    []string `json:"blockers_hit"`
	Solution       string   `json:"solution"`
	Commits        []string `json:"commits"`
}

// BlockersResult holds a list of blocker memories.
type BlockersResult struct {
	Blockers []BlockerMemoryResult `json:"blockers"`
}

// BlockerMemoryResult is the MCP-facing representation of a blocker memory.
type BlockerMemoryResult struct {
	File        string   `json:"file"`
	Issue       string   `json:"issue"`
	Solution    string   `json:"solution"`
	Encountered []string `json:"encountered"`
}

// addMemoryTools registers the memory MCP tools on s.
func addMemoryTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_task_history",
		Description: "Query previous task attempts across sessions for the current project. Returns task status, blockers encountered, solutions, and commits.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search term to match against task descriptions (substring match, case-insensitive)"},"project":{"type":"string","description":"Project name (optional, defaults to current session's project)"}},"required":["query"],"additionalProperties":false}`),
		Handler:     s.handleGetTaskHistory,
	})
	s.registerTool(toolDef{
		Name:        "get_blockers",
		Description: "List known blockers for a project. Returns file-specific issues, solutions, and frequency of occurrence.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"project":{"type":"string","description":"Project name (optional, defaults to current session's project)"},"days":{"type":"integer","description":"Show blockers seen within last N days (default 30)"}},"additionalProperties":false}`),
		Handler:     s.handleGetBlockers,
	})
}

// handleGetTaskHistory returns task memories matching the query string.
func (s *Server) handleGetTaskHistory(args json.RawMessage) (any, error) {
	var params struct {
		Query   string  `json:"query"`
		Project *string `json:"project"`
	}
	if err := json.Unmarshal(args, &params); err != nil || params.Query == "" {
		return TaskHistoryResult{Tasks: []TaskMemoryResult{}}, nil
	}

	// Resolve project name.
	project := s.resolveProject(params.Project)
	if project == "" {
		return TaskHistoryResult{Tasks: []TaskMemoryResult{}}, nil
	}

	// Build store path: ~/.config/claudewatch/projects/<project>/working-memory.json
	storePath := filepath.Join(config.ConfigDir(), "projects", project, "working-memory.json")
	memStore := store.NewWorkingMemoryStore(storePath)

	// Query task history.
	tasks, err := memStore.GetTaskHistory(params.Query)
	if err != nil {
		return TaskHistoryResult{Tasks: []TaskMemoryResult{}}, nil
	}

	// Sort by LastUpdated descending.
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].LastUpdated.After(tasks[j].LastUpdated)
	})

	// Convert to MCP result type.
	result := make([]TaskMemoryResult, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, TaskMemoryResult{
			TaskIdentifier: task.TaskIdentifier,
			Sessions:       task.Sessions,
			Status:         task.Status,
			BlockersHit:    task.BlockersHit,
			Solution:       task.Solution,
			Commits:        task.Commits,
		})
	}

	return TaskHistoryResult{Tasks: result}, nil
}

// handleGetBlockers returns blocker memories for the given project.
func (s *Server) handleGetBlockers(args json.RawMessage) (any, error) {
	var params struct {
		Project *string `json:"project"`
		Days    *int    `json:"days"`
	}
	if len(args) > 0 && string(args) != "null" {
		_ = json.Unmarshal(args, &params)
	}

	// Default days to 30.
	days := 30
	if params.Days != nil {
		days = *params.Days
	}

	// Resolve project name.
	project := s.resolveProject(params.Project)
	if project == "" {
		return BlockersResult{Blockers: []BlockerMemoryResult{}}, nil
	}

	// Build store path: ~/.config/claudewatch/projects/<project>/working-memory.json
	storePath := filepath.Join(config.ConfigDir(), "projects", project, "working-memory.json")
	memStore := store.NewWorkingMemoryStore(storePath)

	// Query blockers.
	blockers, err := memStore.GetRecentBlockers(days)
	if err != nil {
		return BlockersResult{Blockers: []BlockerMemoryResult{}}, nil
	}

	// Sort by LastSeen descending.
	sort.Slice(blockers, func(i, j int) bool {
		return blockers[i].LastSeen.After(blockers[j].LastSeen)
	})

	// Convert to MCP result type.
	result := make([]BlockerMemoryResult, 0, len(blockers))
	for _, blocker := range blockers {
		result = append(result, BlockerMemoryResult{
			File:        blocker.File,
			Issue:       blocker.Issue,
			Solution:    blocker.Solution,
			Encountered: blocker.Encountered,
		})
	}

	return BlockersResult{Blockers: result}, nil
}

// resolveProject returns the project name to use for the query.
// If projectParam is non-nil and non-empty, use it directly.
// Otherwise, fall back to the active session's project or most recent session's project.
func (s *Server) resolveProject(projectParam *string) string {
	if projectParam != nil && *projectParam != "" {
		return *projectParam
	}

	// Try to get the active session's project.
	activePath, err := claude.FindActiveSessionPath(s.claudeHome)
	if err == nil && activePath != "" {
		meta, parseErr := claude.ParseActiveSession(activePath)
		if parseErr == nil && meta != nil && meta.ProjectPath != "" {
			tags := s.loadTags()
			allWeights := s.loadAllWeightsTools()
			return sessionPrimaryProject(meta.SessionID, meta.ProjectPath, tags, allWeights[meta.SessionID])
		}
	}

	// Fall back to most recent session.
	sessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil || len(sessions) == 0 {
		return ""
	}

	// Sort descending by start time.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime > sessions[j].StartTime
	})

	tags := s.loadTags()
	allWeights := s.loadAllWeightsTools()
	session := sessions[0]
	return sessionPrimaryProject(session.SessionID, session.ProjectPath, tags, allWeights[session.SessionID])
}
