package mcp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/memory"
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
	s.registerTool(toolDef{
		Name:        "extract_current_session_memory",
		Description: "Extract and store memory from the current active session immediately. Use this to checkpoint long sessions or capture work-in-progress before a potential crash. Returns confirmation of what was extracted.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Handler:     s.handleExtractCurrentSession,
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
	activePath, err := claude.FindActiveSessionPathForMCP(s.claudeHome)
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

// ExtractResult holds the result of a memory extraction operation.
type ExtractResult struct {
	Success        bool   `json:"success"`
	SessionID      string `json:"session_id"`
	TaskExtracted  bool   `json:"task_extracted"`
	TaskIdentifier string `json:"task_identifier,omitempty"`
	TaskStatus     string `json:"task_status,omitempty"`
	CommitCount    int    `json:"commit_count"`
	BlockerCount   int    `json:"blocker_count"`
	Message        string `json:"message"`
}

// handleExtractCurrentSession extracts memory from the current active session.
func (s *Server) handleExtractCurrentSession(args json.RawMessage) (any, error) {
	// Find the active session.
	activePath, err := claude.FindActiveSessionPathForMCP(s.claudeHome)
	if err != nil {
		return ExtractResult{
			Success: false,
			Message: fmt.Sprintf("Error finding active session: %v", err),
		}, nil
	}
	if activePath == "" {
		return ExtractResult{
			Success: false,
			Message: "No active session found",
		}, nil
	}

	// Extract session ID from path.
	targetSessionID := strings.TrimSuffix(filepath.Base(activePath), ".jsonl")

	// Parse the active session to get project info.
	meta, err := claude.ParseActiveSession(activePath)
	if err != nil {
		return ExtractResult{
			Success: false,
			Message: fmt.Sprintf("Error parsing active session: %v", err),
		}, nil
	}

	// Get project name.
	projectName := filepath.Base(meta.ProjectPath)

	// Load all sessions for this project.
	allSessions, err := claude.ParseAllSessionMeta(s.claudeHome)
	if err != nil {
		return ExtractResult{
			Success: false,
			Message: fmt.Sprintf("Error reading sessions: %v", err),
		}, nil
	}

	// Filter to project sessions.
	var projectSessions []claude.SessionMeta
	for _, sess := range allSessions {
		if filepath.Base(sess.ProjectPath) == projectName {
			projectSessions = append(projectSessions, sess)
		}
	}

	// Find the target session in the list.
	var targetSession *claude.SessionMeta
	for i := range projectSessions {
		if projectSessions[i].SessionID == targetSessionID {
			targetSession = &projectSessions[i]
			break
		}
	}

	if targetSession == nil {
		return ExtractResult{
			Success:   false,
			SessionID: targetSessionID,
			Message:   "Session metadata not found",
		}, nil
	}

	// Load all facets and find the one for this session.
	allFacets, err := claude.ParseAllFacets(s.claudeHome)
	if err != nil {
		return ExtractResult{
			Success:   false,
			SessionID: targetSessionID,
			Message:   "Error reading facets",
		}, nil
	}

	var sessionFacet *claude.SessionFacet
	for i := range allFacets {
		if allFacets[i].SessionID == targetSessionID {
			sessionFacet = &allFacets[i]
			break
		}
	}

	if sessionFacet == nil {
		return ExtractResult{
			Success:   false,
			SessionID: targetSessionID,
			Message:   "No AI analysis (facet) found for session - session may be too new",
		}, nil
	}

	// Extract commits.

	// Build transcript path for semantic extraction
	transcriptPath := ""
	if targetSession.ProjectPath != "" {
		projectHash := filepath.Base(targetSession.ProjectPath)
		transcriptPath = filepath.Join(s.claudeHome, "projects", projectHash, targetSessionID+".jsonl")
	}
	commits := memory.GetCommitSHAsSince(targetSession.ProjectPath, targetSession.StartTime)

	// Open working memory store.
	storePath := filepath.Join(config.ConfigDir(), "projects", projectName, "working-memory.json")
	memStore := store.NewWorkingMemoryStore(storePath)

	result := ExtractResult{
		Success:      true,
		SessionID:    targetSessionID,
		CommitCount:  len(commits),
		BlockerCount: 0,
	}

	// Extract task memory.
	task, err := memory.ExtractTaskMemory(*targetSession, sessionFacet, commits, transcriptPath)
	if err != nil {
		return ExtractResult{
			Success:   false,
			SessionID: targetSessionID,
			Message:   fmt.Sprintf("Error extracting task memory: %v", err),
		}, nil
	}

	if task != nil {
		if err := memStore.AddOrUpdateTask(task); err != nil {
			return ExtractResult{
				Success:   false,
				SessionID: targetSessionID,
				Message:   fmt.Sprintf("Error storing task memory: %v", err),
			}, nil
		}
		result.TaskExtracted = true
		result.TaskIdentifier = task.TaskIdentifier
		result.TaskStatus = task.Status
	}

	// Extract blockers (take last 10 sessions for chronic pattern detection).
	recentSessions := projectSessions
	if len(recentSessions) > 10 {
		recentSessions = recentSessions[:10]
	}

	// Load all facets for blocker context (reuse allFacets loaded above).
	if len(allFacets) > 0 {
		blockers, blockerErr := memory.ExtractBlockers(*targetSession, sessionFacet, projectName, recentSessions, allFacets, transcriptPath)
		if blockerErr == nil && len(blockers) > 0 {
			for _, blocker := range blockers {
				_ = memStore.AddBlocker(blocker)
			}
			result.BlockerCount = len(blockers)
		}
	}

	// Build message.
	if result.TaskExtracted {
		result.Message = fmt.Sprintf("Extracted task '%s' (status: %s) with %d commit(s) and %d blocker(s)",
			result.TaskIdentifier, result.TaskStatus, result.CommitCount, result.BlockerCount)
	} else {
		result.Message = fmt.Sprintf("Extracted %d commit(s) and %d blocker(s) (no task goal identified)",
			result.CommitCount, result.BlockerCount)
	}

	return result, nil
}
