package mcp

import (
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// DashboardResult is the composite response for get_session_dashboard.
// One call replaces six individual tool calls.
type DashboardResult struct {
	SessionID   string `json:"session_id"`
	ProjectName string `json:"project_name"`
	Live        bool   `json:"live"`

	ActiveTime      *claude.ActiveTimeStats `json:"active_time,omitempty"`
	TokenVelocity   *DashboardVelocity      `json:"token_velocity"`
	CommitRatio     *DashboardCommit        `json:"commit_ratio"`
	ContextPressure *DashboardContext       `json:"context_pressure"`
	CostVelocity    *DashboardCost          `json:"cost_velocity"`
	ToolErrors      *DashboardErrors        `json:"tool_errors"`
	Friction        *DashboardFriction      `json:"friction"`
	DriftSignal     *claude.LiveDriftStats  `json:"drift_signal,omitempty"`
}

// DashboardVelocity is a trimmed TokenVelocityResult (no session_id/live duplication).
type DashboardVelocity struct {
	ElapsedMinutes  float64 `json:"elapsed_minutes"`
	TokensPerMinute float64 `json:"tokens_per_minute"`
	Status          string  `json:"status"`
}

// DashboardCommit is a trimmed CommitAttemptResult.
type DashboardCommit struct {
	EditWriteAttempts int     `json:"edit_write_attempts"`
	GitCommits        int     `json:"git_commits"`
	Ratio             float64 `json:"ratio"`
	Assessment        string  `json:"assessment"`
}

// DashboardContext is a trimmed ContextPressureResult.
type DashboardContext struct {
	TotalTokens    int     `json:"total_tokens"`
	Compactions    int     `json:"compactions"`
	EstimatedUsage float64 `json:"estimated_usage"`
	Status         string  `json:"status"`
}

// DashboardCost is a trimmed CostVelocityResult.
type DashboardCost struct {
	WindowCostUSD float64 `json:"window_cost_usd"`
	CostPerMinute float64 `json:"cost_per_minute"`
	Status        string  `json:"status"`
}

// DashboardErrors is a trimmed LiveToolErrorResult.
type DashboardErrors struct {
	TotalToolUses   int            `json:"total_tool_uses"`
	TotalErrors     int            `json:"total_errors"`
	ErrorRate       float64        `json:"error_rate"`
	ErrorsByTool    map[string]int `json:"errors_by_tool"`
	ConsecutiveErrs int            `json:"consecutive_errors"`
	Severity        string         `json:"severity"`
}

// DashboardFriction is a trimmed LiveFrictionResult.
type DashboardFriction struct {
	TotalFriction int                      `json:"total_friction"`
	TopType       string                   `json:"top_type,omitempty"`
	Patterns      []claude.FrictionPattern `json:"patterns,omitempty"`
}

// loadAllWeightsDashboard loads the full session-project-weights map from disk.
// Returns an empty map on any error (non-fatal: missing file is normal).
func (s *Server) loadAllWeightsDashboard() map[string][]store.ProjectWeight {
	ws := store.NewSessionProjectWeightsStore(s.weightsStorePath)
	m, err := ws.Load()
	if err != nil || m == nil {
		return map[string][]store.ProjectWeight{}
	}
	return m
}

// addDashboardTools registers the composite dashboard tool.
func addDashboardTools(s *Server) {
	s.registerTool(toolDef{
		Name:        "get_session_dashboard",
		Description: "All live session metrics in one call: token velocity, commit ratio, context pressure, cost velocity, tool errors, and friction patterns. Use this instead of calling individual metric tools separately.",
		InputSchema: noArgsSchema,
		Handler:     s.handleGetSessionDashboard,
	})
}

// handleGetSessionDashboard returns all live metrics in a single response.
func (s *Server) handleGetSessionDashboard(args json.RawMessage) (any, error) {
	// Single session discovery — replaces 6 redundant FindActiveSessionPath calls.
	activePath, err := claude.FindActiveSessionPath(s.claudeHome)
	if err != nil || activePath == "" {
		return nil, errors.New("no active session found")
	}

	meta, err := claude.ParseActiveSession(activePath)
	if err != nil || meta == nil {
		return nil, errors.New("no active session found")
	}

	tags := s.loadTags()
	allWeights := s.loadAllWeightsDashboard()
	result := DashboardResult{
		SessionID:   meta.SessionID,
		ProjectName: sessionPrimaryProject(meta.SessionID, meta.ProjectPath, tags, allWeights[meta.SessionID]),
		Live:        true,
	}

	// Active time — compute wall-clock vs active minutes.
	activeTime, _ := claude.ParseLiveActiveTime(activePath)
	if activeTime != nil && activeTime.ActiveMinutes > 0 {
		result.ActiveTime = activeTime
	}

	// Token velocity — use active minutes when available for lifetime average.
	startTime := claude.ParseTimestamp(meta.StartTime)
	if !startTime.IsZero() {
		elapsed := time.Since(startTime).Minutes()
		totalTokens := meta.InputTokens + meta.OutputTokens

		// Use active time for lifetime average when the session has idle gaps.
		lifetimeMinutes := elapsed
		if activeTime != nil && activeTime.ActiveMinutes > 0 && activeTime.Resumptions > 0 {
			lifetimeMinutes = activeTime.ActiveMinutes
		}

		var tokPerMin float64
		if lifetimeMinutes > 0 {
			tokPerMin = float64(totalTokens) / lifetimeMinutes
		}

		window, _ := claude.ParseLiveTokenWindow(activePath, 10)
		velocityForStatus := tokPerMin
		if window != nil && window.TokensPerMinute > 0 {
			velocityForStatus = window.TokensPerMinute
		}

		status := "idle"
		if velocityForStatus >= 5000 {
			status = "flowing"
		} else if velocityForStatus >= 1000 {
			status = "slow"
		}

		result.TokenVelocity = &DashboardVelocity{
			ElapsedMinutes:  elapsed,
			TokensPerMinute: tokPerMin,
			Status:          status,
		}
	}

	// Commit ratio.
	if commitStats, err := claude.ParseLiveCommitAttempts(activePath); err == nil {
		assessment := "low"
		if commitStats.EditWriteAttempts == 0 {
			assessment = "no_changes"
		} else if commitStats.Ratio >= 0.3 {
			assessment = "efficient"
		} else if commitStats.Ratio >= 0.1 {
			assessment = "normal"
		}
		result.CommitRatio = &DashboardCommit{
			EditWriteAttempts: commitStats.EditWriteAttempts,
			GitCommits:        commitStats.GitCommits,
			Ratio:             commitStats.Ratio,
			Assessment:        assessment,
		}
	}

	// Context pressure.
	if ctxStats, err := claude.ParseLiveContextPressure(activePath); err == nil {
		result.ContextPressure = &DashboardContext{
			TotalTokens:    ctxStats.TotalTokens,
			Compactions:    ctxStats.Compactions,
			EstimatedUsage: ctxStats.EstimatedUsage,
			Status:         ctxStats.Status,
		}
	}

	// Cost velocity (per-model pricing used internally by ParseLiveCostVelocity).
	if costStats, err := claude.ParseLiveCostVelocity(activePath, 10, fallbackPricing); err == nil {
		result.CostVelocity = &DashboardCost{
			WindowCostUSD: costStats.WindowCostUSD,
			CostPerMinute: costStats.CostPerMinute,
			Status:        costStats.Status,
		}
	}

	// Tool errors.
	if errStats, err := claude.ParseLiveToolErrors(activePath); err == nil {
		severity := "clean"
		if errStats.ConsecutiveErrs >= 4 || errStats.ErrorRate > 0.3 {
			severity = "degraded"
		} else if errStats.ErrorRate > 0.1 {
			severity = "mild"
		}

		errorsByTool := errStats.ErrorsByTool
		if errorsByTool == nil {
			errorsByTool = make(map[string]int)
		}

		result.ToolErrors = &DashboardErrors{
			TotalToolUses:   errStats.TotalToolUses,
			TotalErrors:     errStats.TotalErrors,
			ErrorRate:       errStats.ErrorRate,
			ErrorsByTool:    errorsByTool,
			ConsecutiveErrs: errStats.ConsecutiveErrs,
			Severity:        severity,
		}
	}

	// Friction.
	if frictionStats, err := claude.ParseLiveFriction(activePath); err == nil {
		events := frictionStats.Events
		if events == nil {
			events = []claude.LiveFrictionEvent{}
		}

		topType := ""
		if len(events) > 0 {
			typeCounts := make(map[string]int)
			for _, ev := range events {
				typeCounts[ev.Type] += ev.Count
			}
			keys := make([]string, 0, len(typeCounts))
			for k := range typeCounts {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			topKey := keys[0]
			for _, k := range keys[1:] {
				if typeCounts[k] > typeCounts[topKey] {
					topKey = k
				}
			}
			topType = topKey
		}

		result.Friction = &DashboardFriction{
			TotalFriction: frictionStats.TotalFriction,
			TopType:       topType,
			Patterns:      frictionStats.Patterns,
		}
	}

	// Drift signal.
	if drift, err := claude.ParseLiveDriftSignal(activePath, 20); err == nil {
		result.DriftSignal = drift
	}

	return result, nil
}
