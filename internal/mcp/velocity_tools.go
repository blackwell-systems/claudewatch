package mcp

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// TokenVelocityResult holds token throughput rate for the current live session.
type TokenVelocityResult struct {
	SessionID       string                     `json:"session_id"`
	Live            bool                       `json:"live"`
	ElapsedMinutes  float64                    `json:"elapsed_minutes"`
	TotalTokens     int                        `json:"total_tokens"`
	TokensPerMinute float64                    `json:"tokens_per_minute"`
	OutputPerMinute float64                    `json:"output_tokens_per_minute"`
	Status          string                     `json:"status"` // "flowing", "slow", "idle"
	Window          *claude.WindowedTokenStats `json:"window,omitempty"`
}

// CommitAttemptResult holds the ratio of git commits to Edit/Write attempts.
type CommitAttemptResult struct {
	SessionID         string  `json:"session_id"`
	Live              bool    `json:"live"`
	EditWriteAttempts int     `json:"edit_write_attempts"`
	GitCommits        int     `json:"git_commits"`
	Ratio             float64 `json:"ratio"`
	Assessment        string  `json:"assessment"` // "efficient", "normal", "low", "no_changes"
}

// addVelocityTools registers the token velocity and commit-attempt ratio tools.
func addVelocityTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_token_velocity",
		Description: "Token throughput rate for the current live session: tokens per minute, elapsed minutes, and whether velocity indicates productive flow or stuck/idle state.",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetTokenVelocity,
	})
	s.registerTool(toolDef{
		Name:        "get_commit_attempt_ratio",
		Description: "Ratio of successful git commits to code change attempts (Edit/Write tool uses) in the current live session. Low ratio signals guessing rather than understanding.",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetCommitAttemptRatio,
	})
}

// handleGetTokenVelocity returns token throughput rate for the active session.
func (s *Server) handleGetTokenVelocity(args json.RawMessage) (any, error) {
	activePath, err := claude.FindActiveSessionPathForMCP(s.claudeHome)
	if err != nil || activePath == "" {
		return nil, errors.New("no active session found")
	}

	meta, err := claude.ParseActiveSession(activePath)
	if err != nil || meta == nil {
		return nil, errors.New("no active session found")
	}

	startTime := claude.ParseTimestamp(meta.StartTime)
	if startTime.IsZero() {
		return nil, errors.New("no active session found")
	}

	elapsed := time.Since(startTime).Minutes()
	totalTokens := meta.InputTokens + meta.OutputTokens

	var tokensPerMinute, outputPerMinute float64
	if elapsed > 0 {
		tokensPerMinute = float64(totalTokens) / elapsed
		outputPerMinute = float64(meta.OutputTokens) / elapsed
	}

	// Compute 10-minute windowed velocity for real-time status.
	window, _ := claude.ParseLiveTokenWindow(activePath, 10)

	// Status is based on windowed velocity (last 10 min) when available,
	// falling back to lifetime average.
	velocityForStatus := tokensPerMinute
	if window != nil && window.TokensPerMinute > 0 {
		velocityForStatus = window.TokensPerMinute
	}

	status := "idle"
	if velocityForStatus >= 5000 {
		status = "flowing"
	} else if velocityForStatus >= 1000 {
		status = "slow"
	}

	return TokenVelocityResult{
		SessionID:       meta.SessionID,
		Live:            true,
		ElapsedMinutes:  elapsed,
		TotalTokens:     totalTokens,
		TokensPerMinute: tokensPerMinute,
		OutputPerMinute: outputPerMinute,
		Status:          status,
		Window:          window,
	}, nil
}

// handleGetCommitAttemptRatio returns the commit-to-attempt ratio for the active session.
func (s *Server) handleGetCommitAttemptRatio(args json.RawMessage) (any, error) {
	activePath, err := claude.FindActiveSessionPathForMCP(s.claudeHome)
	if err != nil || activePath == "" {
		return nil, errors.New("no active session found")
	}

	meta, err := claude.ParseActiveSession(activePath)
	if err != nil || meta == nil {
		return nil, errors.New("no active session found")
	}

	commitStats, err := claude.ParseLiveCommitAttempts(activePath)
	if err != nil {
		return nil, err
	}

	assessment := "low"
	if commitStats.EditWriteAttempts == 0 {
		assessment = "no_changes"
	} else if commitStats.Ratio >= 0.3 {
		assessment = "efficient"
	} else if commitStats.Ratio >= 0.1 {
		assessment = "normal"
	}

	return CommitAttemptResult{
		SessionID:         meta.SessionID,
		Live:              true,
		EditWriteAttempts: commitStats.EditWriteAttempts,
		GitCommits:        commitStats.GitCommits,
		Ratio:             commitStats.Ratio,
		Assessment:        assessment,
	}, nil
}
