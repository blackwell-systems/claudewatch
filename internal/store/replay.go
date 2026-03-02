package store

import (
	"bufio"
	"encoding/json"
	"os"
)

// ReplayTurn represents one turn in a session timeline.
type ReplayTurn struct {
	Turn         int     `json:"turn"`
	Role         string  `json:"role"`
	ToolName     string  `json:"tool_name"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	EstCostUSD   float64 `json:"est_cost_usd"`
	Friction     bool    `json:"friction"`
	Timestamp    string  `json:"timestamp"`
}

// SessionReplay is the ordered timeline for one session.
type SessionReplay struct {
	SessionID     string       `json:"session_id"`
	TotalTurns    int          `json:"total_turns"`
	TotalCostUSD  float64      `json:"total_cost_usd"`
	FrictionCount int          `json:"friction_count"`
	Turns         []ReplayTurn `json:"turns"`
}

// replayEntry is a minimal struct for parsing JSONL lines in replay.
type replayEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

// replayAssistantMsg parses the assistant message for usage and content.
type replayAssistantMsg struct {
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Content []struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"content"`
}

// replayUserMsg parses user message content for tool_result error detection.
type replayUserMsg struct {
	Content []struct {
		Type    string `json:"type"`
		IsError bool   `json:"is_error"`
	} `json:"content"`
}

// BuildReplay reads the JSONL transcript for sessionID under claudeHome/projects/
// and returns a structured timeline of turns.
// Turn numbers start at 1 and increment on each "user" or "assistant" entry.
// from and to are 1-indexed inclusive bounds for Turns slice (0 = no bound).
// Session-level totals (TotalTurns, TotalCostUSD, FrictionCount) reflect the
// full session before bounds are applied.
func BuildReplay(sessionID, claudeHome string, from, to int, pricing ModelPricing) (SessionReplay, error) {
	filePath, err := findTranscriptFile(sessionID, claudeHome)
	if err != nil {
		return SessionReplay{}, err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return SessionReplay{}, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 10*1024*1024)
	scanner.Buffer(buf, len(buf))

	var allTurns []ReplayTurn
	turnNum := 0

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry replayEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "assistant":
			if entry.Message == nil {
				continue
			}

			var msg replayAssistantMsg
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}

			turnNum++

			// Find the first tool_use block name.
			var toolName string
			for _, block := range msg.Content {
				if block.Type == "tool_use" {
					toolName = block.Name
					break
				}
			}

			inputTokens := msg.Usage.InputTokens
			outputTokens := msg.Usage.OutputTokens
			cost := (float64(inputTokens)/1_000_000.0)*pricing.InputPerMillion +
				(float64(outputTokens)/1_000_000.0)*pricing.OutputPerMillion

			allTurns = append(allTurns, ReplayTurn{
				Turn:         turnNum,
				Role:         "assistant",
				ToolName:     toolName,
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				EstCostUSD:   cost,
				Friction:     false,
				Timestamp:    entry.Timestamp,
			})

		case "user":
			if entry.Message == nil {
				continue
			}

			var msg replayUserMsg
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				continue
			}

			turnNum++

			// Detect friction: any tool_result content block with IsError == true.
			friction := false
			for _, block := range msg.Content {
				if block.Type == "tool_result" && block.IsError {
					friction = true
					break
				}
			}

			allTurns = append(allTurns, ReplayTurn{
				Turn:         turnNum,
				Role:         "user",
				ToolName:     "",
				InputTokens:  0,
				OutputTokens: 0,
				EstCostUSD:   0,
				Friction:     friction,
				Timestamp:    entry.Timestamp,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return SessionReplay{}, err
	}

	// Compute session-level totals over ALL turns before applying bounds.
	var totalCost float64
	frictionCount := 0
	for _, t := range allTurns {
		totalCost += t.EstCostUSD
		if t.Friction {
			frictionCount++
		}
	}

	// Apply from/to bounds (1-indexed inclusive, 0 = no bound).
	filtered := allTurns
	if from > 0 || to > 0 {
		var bounded []ReplayTurn
		for _, t := range allTurns {
			if from > 0 && t.Turn < from {
				continue
			}
			if to > 0 && t.Turn > to {
				continue
			}
			bounded = append(bounded, t)
		}
		filtered = bounded
	}

	if filtered == nil {
		filtered = []ReplayTurn{}
	}

	return SessionReplay{
		SessionID:     sessionID,
		TotalTurns:    len(allTurns),
		TotalCostUSD:  totalCost,
		FrictionCount: frictionCount,
		Turns:         filtered,
	}, nil
}
