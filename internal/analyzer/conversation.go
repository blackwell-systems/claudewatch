package analyzer

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
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
}

// ConversationAnalysis aggregates conversation metrics across all sessions.
type ConversationAnalysis struct {
	Sessions               []ConversationMetrics `json:"sessions"`
	AvgCorrectionRate      float64               `json:"avg_correction_rate"`
	AvgLongMsgRate         float64               `json:"avg_long_msg_rate"`
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

// sessionAccumulator collects per-entry data for a single session.
type sessionAccumulator struct {
	metrics         ConversationMetrics
	totalUserMsgLen int
	lastAssistantTS time.Time
	responseGaps    []float64
}

// AnalyzeConversations scans all JSONL transcript files under claudeDir/projects/
// and returns conversation-level metrics for each session plus aggregate analysis.
func AnalyzeConversations(claudeDir string) (ConversationAnalysis, error) {
	accumulators := make(map[string]*sessionAccumulator)

	err := claude.WalkTranscriptEntries(claudeDir, func(entry claude.TranscriptEntry, sessionID string, projectHash string) {
		acc, ok := accumulators[sessionID]
		if !ok {
			acc = &sessionAccumulator{
				metrics: ConversationMetrics{
					SessionID:   sessionID,
					ProjectHash: projectHash,
				},
			}
			accumulators[sessionID] = acc
		}

		if entry.Message == nil {
			return
		}

		var msg claude.AssistantMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			return
		}

		ts := claude.ParseTimestamp(entry.Timestamp)

		switch entry.Type {
		case "user":
			userText := extractTextContent(msg.Content)

			// Skip tool_result-only messages (automated responses, not human input).
			if userText == "" {
				return
			}

			// Only count messages that have actual human text, not just tool results.
			if !hasHumanText(msg.Content) {
				return
			}

			acc.metrics.UserMessages++
			acc.metrics.TotalMessages++
			acc.totalUserMsgLen += len(userText)

			if len(userText) > longMessageThreshold {
				acc.metrics.LongMessageCount++
			}

			if isCorrection(userText) {
				acc.metrics.CorrectionCount++
				if len(acc.metrics.CorrectionExamples) < 3 {
					example := userText
					if len(example) > 100 {
						example = example[:100]
					}
					acc.metrics.CorrectionExamples = append(acc.metrics.CorrectionExamples, example)
				}
			}

			// Compute response gap from last assistant message.
			if !acc.lastAssistantTS.IsZero() && !ts.IsZero() {
				gap := ts.Sub(acc.lastAssistantTS).Seconds()
				if gap >= 0 {
					acc.responseGaps = append(acc.responseGaps, gap)
					if gap > longPauseThresholdSecs {
						acc.metrics.LongPauseCount++
					}
				}
			}

		case "assistant":
			acc.metrics.AssistantMessages++
			acc.metrics.TotalMessages++

			if !ts.IsZero() {
				acc.lastAssistantTS = ts
			}
		}
	})
	if err != nil {
		return ConversationAnalysis{}, err
	}

	// Finalize per-session derived metrics.
	var allMetrics []ConversationMetrics
	for _, acc := range accumulators {
		m := &acc.metrics
		if m.UserMessages > 0 {
			m.AvgUserMsgLength = acc.totalUserMsgLen / m.UserMessages
			m.CorrectionRate = float64(m.CorrectionCount) / float64(m.UserMessages)
			m.LongMessageRate = float64(m.LongMessageCount) / float64(m.UserMessages)
		}

		if len(acc.responseGaps) > 0 {
			var totalGap float64
			for _, g := range acc.responseGaps {
				totalGap += g
			}
			m.AvgResponseGapSecs = math.Round(totalGap/float64(len(acc.responseGaps))*100) / 100
		}

		allMetrics = append(allMetrics, *m)
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

		if s.CorrectionRate > 0.3 {
			analysis.HighCorrectionSessions++
		}
	}

	n := float64(len(sessions))
	analysis.AvgCorrectionRate = totalCorrectionRate / n
	analysis.AvgLongMsgRate = totalLongMsgRate / n

	return analysis
}

// extractTextContent concatenates all text-type content blocks in a message.
func extractTextContent(blocks []claude.ContentBlock) string {
	var parts []string
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, " ")
}

// hasHumanText returns true if the message contains at least one text content block,
// distinguishing human-authored messages from automated tool_result messages.
func hasHumanText(blocks []claude.ContentBlock) bool {
	for _, block := range blocks {
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
