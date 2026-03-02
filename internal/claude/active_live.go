package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"time"
)

// LiveToolErrorStats holds tool error statistics parsed from a live JSONL session.
type LiveToolErrorStats struct {
	TotalToolUses   int            `json:"total_tool_uses"`
	TotalErrors     int            `json:"total_errors"`
	ErrorRate       float64        `json:"error_rate"`
	ErrorsByTool    map[string]int `json:"errors_by_tool"`
	ConsecutiveErrs int            `json:"consecutive_errors"`
}

// LiveFrictionEvent represents a single friction event detected in a live session.
type LiveFrictionEvent struct {
	Type      string `json:"type"`
	Tool      string `json:"tool,omitempty"`
	Count     int    `json:"count"`
	Timestamp string `json:"timestamp"`
}

// FrictionPattern represents a grouped friction pattern collapsed from individual events.
type FrictionPattern struct {
	Type        string `json:"type"`        // e.g. "tool_error:Edit", "retry:Bash"
	Count       int    `json:"count"`       // total occurrences
	Consecutive bool   `json:"consecutive"` // true if all occurrences were consecutive
	FirstTurn   int    `json:"first_turn"`  // position of first occurrence in events slice
	LastTurn    int    `json:"last_turn"`   // position of last occurrence
}

// LiveFrictionStats holds friction statistics parsed from a live JSONL session.
type LiveFrictionStats struct {
	Events        []LiveFrictionEvent `json:"events"`
	TotalFriction int                 `json:"total_friction"`
	Patterns      []FrictionPattern   `json:"patterns,omitempty"`
}

// LiveCommitAttemptStats holds commit-to-attempt ratio data parsed from a live session.
type LiveCommitAttemptStats struct {
	EditWriteAttempts int     `json:"edit_write_attempts"`
	GitCommits        int     `json:"git_commits"`
	Ratio             float64 `json:"ratio"`
}

// readLiveJSONL reads a JSONL file using the same line-atomic truncation pattern
// as ParseActiveSession: read entire file, truncate at last newline, scan with
// a 10MB buffer, skip malformed lines.
func readLiveJSONL(path string) ([]TranscriptEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if lastNL := bytes.LastIndexByte(data, '\n'); lastNL >= 0 {
		data = data[:lastNL+1]
	} else {
		return nil, nil
	}

	var entries []TranscriptEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		var entry TranscriptEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// ParseLiveToolErrors reads the JSONL file at path and computes tool error
// statistics from tool_use and tool_result content blocks.
func ParseLiveToolErrors(path string) (*LiveToolErrorStats, error) {
	entries, err := readLiveJSONL(path)
	if err != nil {
		return nil, err
	}

	stats := &LiveToolErrorStats{
		ErrorsByTool: make(map[string]int),
	}

	// Map tool_use ID -> tool name for correlating errors back to tools.
	toolUseNames := make(map[string]string)

	var consecutiveErrs int

	for _, entry := range entries {
		switch entry.Type {
		case "assistant":
			if entry.Message == nil {
				continue
			}
			var msg AssistantMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}
			for _, block := range msg.Content {
				if block.Type == "tool_use" {
					stats.TotalToolUses++
					toolUseNames[block.ID] = block.Name
				}
			}

		case "user":
			if entry.Message == nil {
				continue
			}
			var msg UserMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}
			for _, block := range msg.Content {
				if block.Type != "tool_result" {
					continue
				}
				if block.IsError {
					stats.TotalErrors++
					consecutiveErrs++
					if consecutiveErrs > stats.ConsecutiveErrs {
						stats.ConsecutiveErrs = consecutiveErrs
					}
					if name, ok := toolUseNames[block.ToolUseID]; ok {
						stats.ErrorsByTool[name]++
					}
				} else {
					consecutiveErrs = 0
				}
			}
		}
	}

	if stats.TotalToolUses > 0 {
		stats.ErrorRate = float64(stats.TotalErrors) / float64(stats.TotalToolUses)
	}

	return stats, nil
}

// ParseLiveFriction reads the JSONL file at path and detects friction patterns:
//   - "tool_error": any tool_result with IsError == true
//   - "retry": same tool name used 2+ times within the last 3 tool_use entries
//   - "error_burst": 3+ tool errors within any 5 consecutive tool_results
func ParseLiveFriction(path string) (*LiveFrictionStats, error) {
	entries, err := readLiveJSONL(path)
	if err != nil {
		return nil, err
	}

	stats := &LiveFrictionStats{
		Events: []LiveFrictionEvent{},
	}

	// Map tool_use ID -> tool name for correlating errors.
	toolUseNames := make(map[string]string)

	// Sliding window of last 3 tool_use names for retry detection.
	var recentToolUses []string

	// Sliding window of last 5 tool_results for error burst detection.
	// Each element is true if that tool_result was an error.
	var recentResults []bool

	for _, entry := range entries {
		switch entry.Type {
		case "assistant":
			if entry.Message == nil {
				continue
			}
			var msg AssistantMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}
			for _, block := range msg.Content {
				if block.Type != "tool_use" {
					continue
				}
				toolUseNames[block.ID] = block.Name

				// Retry detection: check if this tool name appears in the
				// last 3 tool_use entries (including this one).
				recentToolUses = append(recentToolUses, block.Name)
				if len(recentToolUses) > 3 {
					recentToolUses = recentToolUses[len(recentToolUses)-3:]
				}
				count := 0
				for _, name := range recentToolUses {
					if name == block.Name {
						count++
					}
				}
				if count >= 2 {
					stats.Events = append(stats.Events, LiveFrictionEvent{
						Type:      "retry",
						Tool:      block.Name,
						Count:     count,
						Timestamp: entry.Timestamp,
					})
					stats.TotalFriction++
				}
			}

		case "user":
			if entry.Message == nil {
				continue
			}
			var msg UserMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}
			for _, block := range msg.Content {
				if block.Type != "tool_result" {
					continue
				}

				// Tool error detection.
				if block.IsError {
					toolName := toolUseNames[block.ToolUseID]
					stats.Events = append(stats.Events, LiveFrictionEvent{
						Type:      "tool_error",
						Tool:      toolName,
						Count:     1,
						Timestamp: entry.Timestamp,
					})
					stats.TotalFriction++
				}

				// Error burst detection: track last 5 tool_results.
				recentResults = append(recentResults, block.IsError)
				if len(recentResults) > 5 {
					recentResults = recentResults[len(recentResults)-5:]
				}
				if len(recentResults) >= 5 {
					errCount := 0
					for _, isErr := range recentResults {
						if isErr {
							errCount++
						}
					}
					if errCount >= 3 {
						stats.Events = append(stats.Events, LiveFrictionEvent{
							Type:      "error_burst",
							Count:     errCount,
							Timestamp: entry.Timestamp,
						})
						stats.TotalFriction++
					}
				}
			}
		}
	}

	stats.Patterns = collapseFrictionPatterns(stats.Events)

	return stats, nil
}

// collapseFrictionPatterns groups friction events by a key derived from Type and Tool,
// counts total occurrences, and detects whether all occurrences were consecutive.
func collapseFrictionPatterns(events []LiveFrictionEvent) []FrictionPattern {
	if len(events) == 0 {
		return []FrictionPattern{}
	}

	// Build key for each event.
	keyFor := func(ev LiveFrictionEvent) string {
		if ev.Tool == "" {
			return ev.Type
		}
		return ev.Type + ":" + ev.Tool
	}

	type patternInfo struct {
		count       int
		consecutive bool
		firstTurn   int
		lastTurn    int
	}

	info := make(map[string]*patternInfo)
	var order []string // track insertion order for stable iteration

	// Track consecutiveness: for each key, check if all its occurrences
	// are contiguous (no different key appears between them).
	// We do this by tracking the last index where each key appeared
	// and checking if a different key was seen in between.
	lastSeen := make(map[string]int) // key -> last index seen

	for i, ev := range events {
		k := keyFor(ev)

		pi, exists := info[k]
		if !exists {
			pi = &patternInfo{
				consecutive: true,
				firstTurn:   i,
			}
			info[k] = pi
			order = append(order, k)
		} else {
			// Check if any different key appeared between lastSeen[k] and i.
			if pi.consecutive && lastSeen[k] < i-1 {
				// There's at least one event between the last occurrence and this one.
				// Check if any of them have a different key.
				for j := lastSeen[k] + 1; j < i; j++ {
					if keyFor(events[j]) != k {
						pi.consecutive = false
						break
					}
				}
			}
		}
		pi.count += ev.Count
		pi.lastTurn = i
		lastSeen[k] = i
	}

	patterns := make([]FrictionPattern, 0, len(info))
	for _, k := range order {
		pi := info[k]
		patterns = append(patterns, FrictionPattern{
			Type:        k,
			Count:       pi.count,
			Consecutive: pi.consecutive,
			FirstTurn:   pi.firstTurn,
			LastTurn:    pi.lastTurn,
		})
	}

	// Sort by count descending, then alphabetically by type for stability.
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].Count != patterns[j].Count {
			return patterns[i].Count > patterns[j].Count
		}
		return patterns[i].Type < patterns[j].Type
	})

	return patterns
}

// ParseLiveCommitAttempts reads the JSONL file at path and counts Edit/Write
// tool uses and git commit attempts to compute a commit-to-attempt ratio.
func ParseLiveCommitAttempts(path string) (*LiveCommitAttemptStats, error) {
	entries, err := readLiveJSONL(path)
	if err != nil {
		return nil, err
	}

	stats := &LiveCommitAttemptStats{}

	for _, entry := range entries {
		if entry.Type != "assistant" || entry.Message == nil {
			continue
		}

		var msg AssistantMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}

		for _, block := range msg.Content {
			if block.Type != "tool_use" {
				continue
			}

			switch block.Name {
			case "Edit", "Write":
				stats.EditWriteAttempts++
			case "Bash":
				if block.Input != nil && strings.Contains(string(block.Input), "git commit") {
					stats.GitCommits++
				}
			}
		}
	}

	if stats.EditWriteAttempts > 0 {
		stats.Ratio = float64(stats.GitCommits) / float64(stats.EditWriteAttempts)
	}

	return stats, nil
}

// WindowedTokenStats holds token usage within a time window.
type WindowedTokenStats struct {
	WindowMinutes   float64 `json:"window_minutes"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	TotalTokens     int     `json:"total_tokens"`
	TokensPerMinute float64 `json:"tokens_per_minute"`
	OutputPerMinute float64 `json:"output_tokens_per_minute"`
	Turns           int     `json:"turns"`
}

// ParseLiveTokenWindow reads the JSONL file and computes token usage within the
// last windowMinutes. Uses assistant entry timestamps and per-turn usage fields.
func ParseLiveTokenWindow(path string, windowMinutes float64) (*WindowedTokenStats, error) {
	entries, err := readLiveJSONL(path)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-time.Duration(windowMinutes * float64(time.Minute)))
	stats := &WindowedTokenStats{WindowMinutes: windowMinutes}

	for _, entry := range entries {
		if entry.Type != "assistant" || entry.Message == nil {
			continue
		}
		ts := ParseTimestamp(entry.Timestamp)
		if ts.IsZero() || ts.Before(cutoff) {
			continue
		}
		var msg assistantMsgUsage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}
		stats.InputTokens += msg.Usage.InputTokens
		stats.OutputTokens += msg.Usage.OutputTokens
		stats.Turns++
	}

	stats.TotalTokens = stats.InputTokens + stats.OutputTokens
	if windowMinutes > 0 {
		stats.TokensPerMinute = float64(stats.TotalTokens) / windowMinutes
		stats.OutputPerMinute = float64(stats.OutputTokens) / windowMinutes
	}

	return stats, nil
}
