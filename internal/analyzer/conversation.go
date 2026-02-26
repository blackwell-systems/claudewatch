package analyzer

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ConversationMetrics captures conversation-level signals for a single session.
type ConversationMetrics struct {
	SessionID         string `json:"session_id"`
	ProjectHash       string `json:"project_hash"`
	TotalMessages     int    `json:"total_messages"`
	UserMessages      int    `json:"user_messages"`
	AssistantMessages int    `json:"assistant_messages"`

	// Correction detection
	CorrectionCount    int      `json:"correction_count"`
	CorrectionRate     float64  `json:"correction_rate"`
	CorrectionExamples []string `json:"correction_examples,omitempty"`

	// Message length patterns
	AvgUserMsgLength int     `json:"avg_user_msg_length"`
	LongMessageCount int     `json:"long_message_count"`
	LongMessageRate  float64 `json:"long_message_rate"`

	// Timing (if timestamps available)
	AvgResponseGapSecs float64 `json:"avg_response_gap_secs"`
	LongPauseCount     int     `json:"long_pause_count"`

	// Zero-output detection
	HasCommits bool `json:"has_commits"`
}

// ConversationAnalysis aggregates conversation metrics across all sessions.
type ConversationAnalysis struct {
	Sessions               []ConversationMetrics `json:"sessions"`
	AvgCorrectionRate      float64               `json:"avg_correction_rate"`
	AvgLongMsgRate         float64               `json:"avg_long_msg_rate"`
	ZeroCommitSessions     int                   `json:"zero_commit_sessions"`
	ZeroCommitRate         float64               `json:"zero_commit_rate"`
	HighCorrectionSessions int                   `json:"high_correction_sessions"`
}

// correctionPatterns defines phrases that signal user corrections. Each pattern
// is checked case-insensitively against user message text. We intentionally
// keep this simple — false positives are acceptable because we track trends,
// not exact counts.
var correctionPatterns = []string{
	"no,",
	"no ",
	"revert",
	"undo",
	"wrong",
	"that's not",
	"thats not",
	"i meant",
	"i said",
	"don't",
	"dont",
	"stop",
	"not what i",
	"go back",
	"try again",
}

// longMessageThreshold is the character count above which a user message is
// considered "long" (likely re-explaining context).
const longMessageThreshold = 500

// longPauseThresholdSecs is the gap (in seconds) between an assistant message
// and the next user message that counts as a "long pause" (user may be confused
// or reviewing output).
const longPauseThresholdSecs = 120

// AnalyzeConversations scans all JSONL transcript files under claudeDir/projects/
// and returns conversation-level metrics for each session plus aggregate analysis.
func AnalyzeConversations(claudeDir string) (ConversationAnalysis, error) {
	projectsDir := filepath.Join(claudeDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return ConversationAnalysis{}, nil
		}
		return ConversationAnalysis{}, err
	}

	var allMetrics []ConversationMetrics

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
			metrics, err := analyzeTranscript(filePath)
			if err != nil {
				continue
			}

			metrics.ProjectHash = projectHash
			allMetrics = append(allMetrics, metrics)
		}
	}

	return aggregateConversations(allMetrics), nil
}

// aggregateConversations computes summary statistics from per-session metrics.
func aggregateConversations(sessions []ConversationMetrics) ConversationAnalysis {
	analysis := ConversationAnalysis{
		Sessions: sessions,
	}

	if len(sessions) == 0 {
		return analysis
	}

	var totalCorrectionRate, totalLongMsgRate float64

	for _, s := range sessions {
		totalCorrectionRate += s.CorrectionRate
		totalLongMsgRate += s.LongMessageRate

		if !s.HasCommits {
			analysis.ZeroCommitSessions++
		}

		if s.CorrectionRate > 0.3 {
			analysis.HighCorrectionSessions++
		}
	}

	n := float64(len(sessions))
	analysis.AvgCorrectionRate = totalCorrectionRate / n
	analysis.AvgLongMsgRate = totalLongMsgRate / n
	analysis.ZeroCommitRate = float64(analysis.ZeroCommitSessions) / n

	return analysis
}

// conversationEntry mirrors the transcript JSONL structure. We define our own
// copy here to avoid coupling the analyzer package to the claude package's
// unexported types.
type conversationEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

// conversationMessage represents a parsed message with role and text content.
type conversationMessage struct {
	Role    string            `json:"role"`
	Content []conversationCB  `json:"content"`
}

// conversationCB is a content block within a message.
type conversationCB struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// analyzeTranscript processes a single JSONL file and returns conversation metrics.
func analyzeTranscript(path string) (ConversationMetrics, error) {
	f, err := os.Open(path)
	if err != nil {
		return ConversationMetrics{}, err
	}
	defer f.Close()

	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	metrics := ConversationMetrics{
		SessionID: sessionID,
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var (
		totalUserMsgLen  int
		lastAssistantTS  time.Time
		responseGaps     []float64
		hasCommitTool    bool
	)

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry conversationEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Message == nil {
			continue
		}

		var msg conversationMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}

		ts := parseConversationTimestamp(entry.Timestamp)

		switch entry.Type {
		case "user":
			userText := extractTextContent(&msg)

			// Skip tool_result-only messages (automated responses, not human input).
			if userText == "" {
				continue
			}

			// Only count messages that have actual human text, not just tool results.
			if !hasHumanText(&msg) {
				continue
			}

			metrics.UserMessages++
			metrics.TotalMessages++
			totalUserMsgLen += len(userText)

			if len(userText) > longMessageThreshold {
				metrics.LongMessageCount++
			}

			if isCorrection(userText) {
				metrics.CorrectionCount++
				if len(metrics.CorrectionExamples) < 3 {
					example := userText
					if len(example) > 100 {
						example = example[:100]
					}
					metrics.CorrectionExamples = append(metrics.CorrectionExamples, example)
				}
			}

			// Compute response gap from last assistant message.
			if !lastAssistantTS.IsZero() && !ts.IsZero() {
				gap := ts.Sub(lastAssistantTS).Seconds()
				if gap >= 0 {
					responseGaps = append(responseGaps, gap)
					if gap > longPauseThresholdSecs {
						metrics.LongPauseCount++
					}
				}
			}

		case "assistant":
			metrics.AssistantMessages++
			metrics.TotalMessages++

			if !ts.IsZero() {
				lastAssistantTS = ts
			}

			// Check for commit-related tool usage (Bash with git commit).
			if detectCommitUsage(&msg) {
				hasCommitTool = true
			}
		}
	}

	// Compute derived metrics.
	if metrics.UserMessages > 0 {
		metrics.AvgUserMsgLength = totalUserMsgLen / metrics.UserMessages
		metrics.CorrectionRate = float64(metrics.CorrectionCount) / float64(metrics.UserMessages)
		metrics.LongMessageRate = float64(metrics.LongMessageCount) / float64(metrics.UserMessages)
	}

	if len(responseGaps) > 0 {
		var totalGap float64
		for _, g := range responseGaps {
			totalGap += g
		}
		metrics.AvgResponseGapSecs = math.Round(totalGap/float64(len(responseGaps))*100) / 100
	}

	metrics.HasCommits = hasCommitTool

	return metrics, nil
}

// extractTextContent concatenates all text-type content blocks in a message.
func extractTextContent(msg *conversationMessage) string {
	var parts []string
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, " ")
}

// hasHumanText returns true if the message contains at least one text content block,
// distinguishing human-authored messages from automated tool_result messages.
func hasHumanText(msg *conversationMessage) bool {
	for _, block := range msg.Content {
		if block.Type == "text" && block.Text != "" {
			return true
		}
	}
	return false
}

// isCorrection checks whether a user message contains correction signals.
func isCorrection(text string) bool {
	lower := strings.ToLower(text)
	for _, pattern := range correctionPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// bashInput represents the input payload of a Bash tool_use block.
type bashInput struct {
	Command string `json:"command"`
}

// detectCommitUsage checks whether an assistant message contains a Bash tool_use
// that includes a git commit command, or a Skill tool invocation for commit.
func detectCommitUsage(msg *conversationMessage) bool {
	for _, block := range msg.Content {
		if block.Type != "tool_use" {
			continue
		}

		// Skill tool invoked with "commit" typically means a commit action.
		if block.Name == "Skill" {
			var input struct {
				Skill string `json:"skill"`
			}
			if json.Unmarshal(block.Input, &input) == nil {
				if strings.Contains(strings.ToLower(input.Skill), "commit") {
					return true
				}
			}
		}

		// Check Bash tool_use for git commit commands.
		if block.Name == "Bash" && block.Input != nil {
			var input bashInput
			if json.Unmarshal(block.Input, &input) == nil {
				if strings.Contains(input.Command, "git commit") {
					return true
				}
			}
		}
	}
	return false
}

// parseConversationTimestamp parses an ISO 8601 timestamp string.
func parseConversationTimestamp(s string) time.Time {
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
