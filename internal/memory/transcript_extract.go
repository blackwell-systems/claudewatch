package memory

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// TranscriptContext holds extracted semantic information from a session transcript.
type TranscriptContext struct {
	TaskDescription string              // From first user message
	UserCorrections []string            // User redirects/feedback
	ErrorPatterns   []ErrorPattern      // What failed and why
	FilesAccessed   map[string]int      // Files touched (path -> access count)
	ToolSequence    []string            // Ordered list of tools used
}

// ErrorPattern represents a failure sequence.
type ErrorPattern struct {
	Tool         string // Tool that failed
	ErrorMessage string // First 200 chars of error
	Attempts     int    // How many times retried
	Resolved     bool   // Did it eventually succeed?
}

// ExtractFromTranscript parses a session transcript and extracts semantic information.
// Returns rich context that can be used for task descriptions and blocker detection.
func ExtractFromTranscript(transcriptPath string) (*TranscriptContext, error) {
	// Read transcript entries
	entries, err := readTranscriptEntries(transcriptPath)
	if err != nil {
		return nil, err
	}

	ctx := &TranscriptContext{
		FilesAccessed: make(map[string]int),
	}

	// Track error sequences
	type errorKey struct {
		tool    string
		errText string
	}
	errorSequences := make(map[errorKey]*ErrorPattern)

	// Track most recent tool use per tool name
	lastToolUse := make(map[string]bool) // tool -> was last call successful?

	for i, entry := range entries {
		switch entry.Type {
		case "user":
			msg := extractUserText(entry.Message)
			if i == 0 && msg != "" {
				// First user message is the task description
				ctx.TaskDescription = truncate(msg, 200)
			} else if isUserCorrection(msg) {
				// User is redirecting or correcting
				ctx.UserCorrections = append(ctx.UserCorrections, truncate(msg, 150))
			}

		case "tool_use":
			toolName := extractToolName(entry.Message)
			if toolName != "" {
				ctx.ToolSequence = append(ctx.ToolSequence, toolName)

				// Track file accesses
				if files := extractFilePathsFromToolUse(entry.Message, toolName); len(files) > 0 {
					for _, f := range files {
						ctx.FilesAccessed[f]++
					}
				}
			}

		case "tool_result":
			toolName := extractToolName(entry.Message)
			isError := extractIsError(entry.Message)

			if isError {
				errContent := extractToolResultContent(entry.Message)
				errMsg := truncate(errContent, 200)

				// Create key from tool + first 50 chars of error
				keyText := truncate(errMsg, 50)
				key := errorKey{tool: toolName, errText: keyText}

				if pattern, exists := errorSequences[key]; exists {
					pattern.Attempts++
				} else {
					errorSequences[key] = &ErrorPattern{
						Tool:         toolName,
						ErrorMessage: errMsg,
						Attempts:     1,
						Resolved:     false,
					}
				}
				lastToolUse[toolName] = false
			} else {
				// Success - mark any previous errors for this tool as resolved
				if !lastToolUse[toolName] {
					for key, pattern := range errorSequences {
						if key.tool == toolName && !pattern.Resolved {
							pattern.Resolved = true
						}
					}
				}
				lastToolUse[toolName] = true
			}
		}
	}

	// Convert error sequences to patterns (only keep patterns with 2+ attempts)
	for _, pattern := range errorSequences {
		if pattern.Attempts >= 2 {
			ctx.ErrorPatterns = append(ctx.ErrorPatterns, *pattern)
		}
	}

	return ctx, nil
}

// readTranscriptEntries reads and parses all entries from a JSONL transcript file.
func readTranscriptEntries(path string) ([]claude.TranscriptEntry, error) {
	// Use existing ParseSingleTranscript infrastructure
	// For now, directly read and parse JSONL
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entries []claude.TranscriptEntry
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry claude.TranscriptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip malformed lines
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// extractUserText extracts text content from a user message.
func extractUserText(raw json.RawMessage) string {
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}

	var texts []string
	for _, c := range msg.Content {
		if c.Type == "text" && c.Text != "" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, " ")
}

// extractToolName extracts the tool name from a tool_use or tool_result message.
func extractToolName(raw json.RawMessage) string {
	var msg struct {
		Name string `json:"name"`
	}
	json.Unmarshal(raw, &msg)
	return msg.Name
}

// extractIsError checks if a tool_result indicates an error.
func extractIsError(raw json.RawMessage) bool {
	var msg struct {
		IsError bool `json:"is_error"`
	}
	json.Unmarshal(raw, &msg)
	return msg.IsError
}

// extractToolResultContent extracts the content from a tool_result.
func extractToolResultContent(raw json.RawMessage) string {
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}

	var texts []string
	for _, c := range msg.Content {
		if c.Type == "text" && c.Text != "" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, " ")
}

// extractFilePathsFromToolUse extracts file paths from tool_use input based on tool type.
func extractFilePathsFromToolUse(raw json.RawMessage, toolName string) []string {
	var msg struct {
		Input map[string]interface{} `json:"input"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil
	}

	var paths []string

	// Common file path parameter names
	fileParams := []string{"file_path", "path", "file", "files"}
	for _, param := range fileParams {
		if val, ok := msg.Input[param]; ok {
			if strVal, ok := val.(string); ok && strVal != "" {
				paths = append(paths, strVal)
			}
		}
	}

	return paths
}

// isUserCorrection detects if a user message is correcting or redirecting.
func isUserCorrection(msg string) bool {
	lower := strings.ToLower(msg)
	corrections := []string{
		"no,", "no ", "nope", "incorrect", "wrong",
		"instead", "actually", "don't", "stop",
		"that's not", "that isn't", "not what i",
	}

	for _, phrase := range corrections {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// truncate limits string length and adds ellipsis if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

