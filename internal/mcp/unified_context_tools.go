package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/client"
	ctxpkg "github.com/blackwell-systems/claudewatch/internal/context"
)

// addUnifiedContextTools registers the get_context MCP tool on s.
func addUnifiedContextTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_context",
		Description: "Unified context search across commits, memory, tasks, and transcripts. Returns ranked, deduplicated results with source attribution.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query"},"project":{"type":"string","description":"Project name filter (optional)"},"limit":{"type":"integer","description":"Maximum results (default 20)"}},"required":["query"],"additionalProperties":false}`),
		Handler:     s.handleGetContext,
	})
}

// handleGetContext is the MCP handler for get_context.
func (s *Server) handleGetContext(args json.RawMessage) (any, error) {
	var params struct {
		Query   string  `json:"query"`
		Project *string `json:"project,omitempty"`
		Limit   *int    `json:"limit,omitempty"`
	}

	if err := json.Unmarshal(args, &params); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if params.Query == "" {
		return nil, errors.New("query is required")
	}

	// Set defaults
	limit := 20
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
	}

	project := ""
	if params.Project != nil {
		project = *params.Project
	}

	// Create MCP client and fetch from all sources in parallel
	mcpClient := client.NewMCPClient()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rawResults, errs := client.FetchAllSources(ctx, mcpClient, params.Query, project, limit)

	// Check if all sources failed
	if len(rawResults) == 0 && len(errs) > 0 {
		errMsgs := make([]string, len(errs))
		for i, err := range errs {
			errMsgs[i] = err.Error()
		}
		return nil, fmt.Errorf("all sources failed: %v", errMsgs)
	}

	// Parse results from each source into ContextItems
	var allItems []ctxpkg.ContextItem
	var errorMessages []string

	// Parse memory results
	if memData, ok := rawResults["memory"]; ok {
		items, err := parseMemoryResults(memData)
		if err != nil {
			errorMessages = append(errorMessages, fmt.Sprintf("memory parse error: %v", err))
		} else {
			allItems = append(allItems, items...)
		}
	}

	// Parse commit results
	if commitData, ok := rawResults["commit"]; ok {
		items, err := parseCommitResults(commitData)
		if err != nil {
			errorMessages = append(errorMessages, fmt.Sprintf("commit parse error: %v", err))
		} else {
			allItems = append(allItems, items...)
		}
	}

	// Parse task_history results
	if taskData, ok := rawResults["task_history"]; ok {
		items, err := parseTaskHistoryResults(taskData)
		if err != nil {
			errorMessages = append(errorMessages, fmt.Sprintf("task_history parse error: %v", err))
		} else {
			allItems = append(allItems, items...)
		}
	}

	// Parse transcript results
	if transcriptData, ok := rawResults["transcript"]; ok {
		items, err := parseTranscriptResults(transcriptData)
		if err != nil {
			errorMessages = append(errorMessages, fmt.Sprintf("transcript parse error: %v", err))
		} else {
			allItems = append(allItems, items...)
		}
	}

	// Add source-level errors to error messages
	for _, err := range errs {
		errorMessages = append(errorMessages, err.Error())
	}

	// Deduplicate items
	dedupedItems := ctxpkg.DeduplicateItems(allItems)

	// Rank and sort by relevance
	ctxpkg.RankAndSort(dedupedItems)

	// Truncate to limit
	if len(dedupedItems) > limit {
		dedupedItems = dedupedItems[:limit]
	}

	// Build list of sources that were queried
	sources := make([]string, 0, len(rawResults))
	for source := range rawResults {
		sources = append(sources, source)
	}

	// Build result
	result := ctxpkg.UnifiedContextResult{
		Query:   params.Query,
		Items:   dedupedItems,
		Count:   len(dedupedItems),
		Sources: sources,
		Errors:  errorMessages,
	}

	return result, nil
}

// parseMemoryResults parses commitmux_search_memory results.
func parseMemoryResults(data []byte) ([]ctxpkg.ContextItem, error) {
	var memResult struct {
		Memories []struct {
			Path     string  `json:"path"`
			Content  string  `json:"content"`
			Distance float64 `json:"distance"`
		} `json:"memories"`
	}

	if err := json.Unmarshal(data, &memResult); err != nil {
		return nil, err
	}

	items := make([]ctxpkg.ContextItem, 0, len(memResult.Memories))
	for _, mem := range memResult.Memories {
		items = append(items, ctxpkg.ContextItem{
			Source:    ctxpkg.SourceMemory,
			Title:     fmt.Sprintf("memory: %s", mem.Path),
			Snippet:   mem.Content,
			Timestamp: time.Now(), // Memory files don't have timestamps in search results
			Metadata: map[string]string{
				"path": mem.Path,
			},
			Score: 1.0 - mem.Distance, // Convert distance to score
		})
	}

	return items, nil
}

// parseCommitResults parses commitmux_search_semantic results.
func parseCommitResults(data []byte) ([]ctxpkg.ContextItem, error) {
	var commitResult struct {
		Commits []struct {
			SHA       string  `json:"sha"`
			Message   string  `json:"message"`
			Author    string  `json:"author"`
			Timestamp string  `json:"timestamp"`
			Repo      string  `json:"repo"`
			Distance  float64 `json:"distance"`
		} `json:"commits"`
	}

	if err := json.Unmarshal(data, &commitResult); err != nil {
		return nil, err
	}

	items := make([]ctxpkg.ContextItem, 0, len(commitResult.Commits))
	for _, commit := range commitResult.Commits {
		timestamp, _ := time.Parse(time.RFC3339, commit.Timestamp)
		items = append(items, ctxpkg.ContextItem{
			Source:    ctxpkg.SourceCommit,
			Title:     fmt.Sprintf("commit: %s", commit.SHA[:7]),
			Snippet:   commit.Message,
			Timestamp: timestamp,
			Metadata: map[string]string{
				"sha":    commit.SHA,
				"author": commit.Author,
				"repo":   commit.Repo,
			},
			Score: 1.0 - commit.Distance, // Convert distance to score
		})
	}

	return items, nil
}

// parseTaskHistoryResults parses get_task_history results.
func parseTaskHistoryResults(data []byte) ([]ctxpkg.ContextItem, error) {
	var taskResult struct {
		Tasks []struct {
			TaskIdentifier string   `json:"task_identifier"`
			Sessions       []string `json:"sessions"`
			Status         string   `json:"status"`
			BlockersHit    []string `json:"blockers_hit"`
			Solution       string   `json:"solution"`
			Commits        []string `json:"commits"`
		} `json:"tasks"`
	}

	if err := json.Unmarshal(data, &taskResult); err != nil {
		return nil, err
	}

	items := make([]ctxpkg.ContextItem, 0, len(taskResult.Tasks))
	for _, task := range taskResult.Tasks {
		snippet := fmt.Sprintf("Status: %s", task.Status)
		if task.Solution != "" {
			snippet += fmt.Sprintf("\nSolution: %s", task.Solution)
		}

		items = append(items, ctxpkg.ContextItem{
			Source:    ctxpkg.SourceTaskHistory,
			Title:     fmt.Sprintf("task: %s", task.TaskIdentifier),
			Snippet:   snippet,
			Timestamp: time.Now(), // Task history doesn't include timestamps in current schema
			Metadata: map[string]string{
				"status":       task.Status,
				"session_id":   "", // Join sessions if needed
				"commit_count": fmt.Sprintf("%d", len(task.Commits)),
			},
			Score: 0.5, // Default score for keyword-based matches
		})
	}

	return items, nil
}

// parseTranscriptResults parses search_transcripts results.
func parseTranscriptResults(data []byte) ([]ctxpkg.ContextItem, error) {
	var transcriptResult struct {
		Count   int `json:"count"`
		Results []struct {
			SessionID   string  `json:"session_id"`
			ProjectHash string  `json:"project_hash"`
			Snippet     string  `json:"snippet"`
			EntryType   string  `json:"entry_type"`
			Timestamp   string  `json:"timestamp"`
			Rank        float64 `json:"rank"`
		} `json:"results"`
	}

	if err := json.Unmarshal(data, &transcriptResult); err != nil {
		return nil, err
	}

	items := make([]ctxpkg.ContextItem, 0, len(transcriptResult.Results))
	for _, result := range transcriptResult.Results {
		timestamp, _ := time.Parse(time.RFC3339, result.Timestamp)
		items = append(items, ctxpkg.ContextItem{
			Source:    ctxpkg.SourceTranscript,
			Title:     fmt.Sprintf("transcript: %s", result.SessionID[:8]),
			Snippet:   result.Snippet,
			Timestamp: timestamp,
			Metadata: map[string]string{
				"session_id":   result.SessionID,
				"project_hash": result.ProjectHash,
				"entry_type":   result.EntryType,
			},
			Score: result.Rank, // FTS rank is already 0-1
		})
	}

	return items, nil
}
