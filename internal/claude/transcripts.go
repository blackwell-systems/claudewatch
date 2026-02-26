package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AgentSpan represents a single agent task extracted from a session transcript.
type AgentSpan struct {
	SessionID    string        `json:"session_id"`
	ProjectHash  string        `json:"project_hash"`
	AgentType    string        `json:"agent_type"`
	Description  string        `json:"description"`
	Prompt       string        `json:"prompt"`
	Background   bool          `json:"background"`
	LaunchedAt   time.Time     `json:"launched_at"`
	CompletedAt  time.Time     `json:"completed_at"`
	Duration     time.Duration `json:"duration"`
	Killed       bool          `json:"killed"`
	Success      bool          `json:"success"`
	ResultLength int           `json:"result_length"`
	ToolUseID    string        `json:"tool_use_id"`
}

// ParseSessionTranscripts scans all JSONL files under claudeDir/projects/
// and extracts AgentSpan data from Task tool_use / tool_result pairs.
func ParseSessionTranscripts(claudeDir string) ([]AgentSpan, error) {
	projectsDir := filepath.Join(claudeDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var allSpans []AgentSpan

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectHash := entry.Name()
		dirPath := filepath.Join(projectsDir, projectHash)

		files, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}

			filePath := filepath.Join(dirPath, f.Name())
			spans, err := ParseSingleTranscript(filePath)
			if err != nil {
				continue
			}

			// Fill in project hash for all spans.
			for i := range spans {
				spans[i].ProjectHash = projectHash
			}

			allSpans = append(allSpans, spans...)
		}
	}

	return allSpans, nil
}

// ParseSingleTranscript parses one JSONL file and returns agent spans.
func ParseSingleTranscript(path string) ([]AgentSpan, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Derive session ID from the filename (strip .jsonl extension).
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	// Track pending Task launches by tool_use_id.
	pending := make(map[string]*pendingTask)

	// Track killed task IDs (from TaskStop). TaskStop's task_id field
	// corresponds to the agentId in progress messages. We map agentId
	// back to tool_use_id via the progress entries.
	killedAgentIDs := make(map[string]bool)

	// Map agentId -> tool_use_id from progress entries.
	agentToToolUse := make(map[string]string)

	var spans []AgentSpan

	scanner := bufio.NewScanner(f)
	// Increase buffer for long JSONL lines (up to 10MB).
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry transcriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "assistant":
			processAssistantEntry(&entry, sessionID, pending, killedAgentIDs)
		case "user":
			processUserEntry(&entry, pending, &spans)
		case "progress":
			processProgressEntry(&entry, agentToToolUse)
		}
	}

	// Mark killed tasks using the agentId -> toolUseId mapping.
	for agentID := range killedAgentIDs {
		if toolUseID, ok := agentToToolUse[agentID]; ok {
			// Find the span and mark it killed.
			for i := range spans {
				if spans[i].ToolUseID == toolUseID {
					spans[i].Killed = true
					spans[i].Success = false
					break
				}
			}
			// Also check pending (may not have completed).
			if p, ok := pending[toolUseID]; ok {
				p.span.Killed = true
				p.span.Success = false
			}
		}
	}

	// Any remaining pending tasks never got a result — mark them incomplete.
	for _, p := range pending {
		p.span.Success = false
		spans = append(spans, p.span)
	}

	return spans, nil
}

// transcriptEntry is the top-level structure of a JSONL line.
type transcriptEntry struct {
	Type             string          `json:"type"`
	Timestamp        string          `json:"timestamp"`
	SessionID        string          `json:"sessionId"`
	Message          json.RawMessage `json:"message"`
	Data             json.RawMessage `json:"data"`
	ParentToolUseID  string          `json:"parentToolUseID"`
}

// assistantMessage represents an assistant-role message.
type assistantMessage struct {
	Role    string           `json:"role"`
	Content []contentBlock   `json:"content"`
}

// userMessage represents a user-role message.
type userMessage struct {
	Role    string           `json:"role"`
	Content []contentBlock   `json:"content"`
}

// contentBlock represents a single content block (tool_use, tool_result, text).
type contentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
	Text      string          `json:"text"`
}

// taskInput represents the input fields of a Task tool_use.
type taskInput struct {
	SubagentType    string `json:"subagent_type"`
	Description     string `json:"description"`
	Prompt          string `json:"prompt"`
	RunInBackground bool   `json:"run_in_background"`
}

// taskStopInput represents the input fields of a TaskStop tool_use.
type taskStopInput struct {
	TaskID string `json:"task_id"`
}

// progressData represents the data field of a progress entry.
type progressData struct {
	AgentID string `json:"agentId"`
	Type    string `json:"type"`
}

type pendingTask struct {
	span AgentSpan
}

// processAssistantEntry handles assistant-type entries, extracting Task
// launches and TaskStop calls.
func processAssistantEntry(entry *transcriptEntry, sessionID string, pending map[string]*pendingTask, killedAgentIDs map[string]bool) {
	if entry.Message == nil {
		return
	}

	var msg assistantMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return
	}

	ts := parseTimestamp(entry.Timestamp)

	for _, block := range msg.Content {
		switch {
		case block.Type == "tool_use" && block.Name == "Task":
			var input taskInput
			if err := json.Unmarshal(block.Input, &input); err != nil {
				continue
			}

			prompt := input.Prompt
			if len(prompt) > 200 {
				prompt = prompt[:200]
			}

			agentType := input.SubagentType
			if agentType == "" {
				agentType = "general-purpose"
			}

			pending[block.ID] = &pendingTask{
				span: AgentSpan{
					SessionID:   sessionID,
					AgentType:   agentType,
					Description: input.Description,
					Prompt:      prompt,
					Background:  input.RunInBackground,
					LaunchedAt:  ts,
					ToolUseID:   block.ID,
					Success:     true, // assume success until proven otherwise
				},
			}

		case block.Type == "tool_use" && block.Name == "TaskStop":
			var input taskStopInput
			if err := json.Unmarshal(block.Input, &input); err != nil {
				continue
			}
			if input.TaskID != "" {
				killedAgentIDs[input.TaskID] = true
			}
		}
	}
}

// processUserEntry handles user-type entries, looking for tool_result blocks
// that complete a pending Task.
func processUserEntry(entry *transcriptEntry, pending map[string]*pendingTask, spans *[]AgentSpan) {
	if entry.Message == nil {
		return
	}

	var msg userMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return
	}

	ts := parseTimestamp(entry.Timestamp)

	for _, block := range msg.Content {
		if block.Type != "tool_result" {
			continue
		}

		p, ok := pending[block.ToolUseID]
		if !ok {
			continue
		}

		p.span.CompletedAt = ts
		if !p.span.LaunchedAt.IsZero() && !ts.IsZero() {
			p.span.Duration = ts.Sub(p.span.LaunchedAt)
		}

		// Determine result length from the content.
		p.span.ResultLength = resultContentLength(block.Content, block.Text)

		// Check for error.
		if block.IsError {
			p.span.Success = false
		}

		*spans = append(*spans, p.span)
		delete(pending, block.ToolUseID)
	}
}

// processProgressEntry handles progress-type entries, mapping agentId to
// the parentToolUseID so we can correlate TaskStop calls.
func processProgressEntry(entry *transcriptEntry, agentToToolUse map[string]string) {
	if entry.Data == nil || entry.ParentToolUseID == "" {
		return
	}

	var data progressData
	if err := json.Unmarshal(entry.Data, &data); err != nil {
		return
	}

	if data.AgentID != "" && data.Type == "agent_progress" {
		agentToToolUse[data.AgentID] = entry.ParentToolUseID
	}
}

// resultContentLength computes the total text length of a tool_result's content.
func resultContentLength(raw json.RawMessage, text string) int {
	if text != "" {
		return len(text)
	}
	if raw == nil {
		return 0
	}

	// Content can be a string or an array of content blocks.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return len(s)
	}

	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		total := 0
		for _, b := range blocks {
			total += len(b.Text)
		}
		return total
	}

	return len(raw)
}

// parseTimestamp parses an ISO 8601 timestamp string.
func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}
