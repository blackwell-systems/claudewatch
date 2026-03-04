package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/spf13/cobra"
)

const (
	hookThreshold       = 3
	hookCooldownSeconds = 30
	// Sonnet pricing ($/million tokens) — matches mcp/cost_velocity_tools.go
	hookInputPerMillion  = 3.0
	hookOutputPerMillion = 15.0
	// Dashboard display interval (tool calls).
	dashboardInterval = 50
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Check session health (for use as a PostToolUse shell hook)",
	Long: `Checks the active Claude Code session for warning conditions in priority order:
  1. Consecutive tool errors >= 3
  2. Context pressure at "pressure" or "critical"
  3. Cost velocity "burning"
  4. Drift: read-heavy loop (>=60% reads, 0 writes in last 15 tools)

Exit 0 if all clear (silent). Exit 2 if a threshold is exceeded, with one
actionable line printed to stderr naming the relevant MCP tool to call.

Rate-limited to one check per 30 seconds to minimize overhead.

Intended for use as a Claude Code PostToolUse shell hook.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run:           runHook,
}

func init() {
	rootCmd.AddCommand(hookCmd)
}

func runHook(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return
	}

	activePath, err := claude.FindActiveSessionPath(cfg.ClaudeHome)
	if err != nil || activePath == "" {
		return
	}

	// Periodic dashboard display (every 50 tool calls).
	// This runs independently of threshold checks and has its own state file.
	if shouldDisplayDashboard(activePath) {
		pricing := claude.CostPricing{
			InputPerMillion:  hookInputPerMillion,
			OutputPerMillion: hookOutputPerMillion,
		}
		if dashboard, err := analyzer.ComputeSessionDashboard(activePath, pricing); err == nil {
			fmt.Fprintln(os.Stderr, analyzer.FormatDashboard(dashboard))
			recordDashboardDisplay(activePath, dashboard.ToolCallCount)
		}
	}

	// Rate limiter for threshold checks: skip if within cooldown window.
	stampFile := os.ExpandEnv("$HOME/.cache/claudewatch-hook.ts")
	now := time.Now().Unix()
	if data, err := os.ReadFile(stampFile); err == nil {
		if last, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
			if now-last < hookCooldownSeconds {
				return
			}
		}
	}
	_ = os.MkdirAll(os.ExpandEnv("$HOME/.cache"), 0o755)
	_ = os.WriteFile(stampFile, []byte(strconv.FormatInt(now, 10)), 0o644)

	cwd, _ := os.Getwd()

	// Priority 1: consecutive tool errors.
	if n, err := claude.ParseLiveConsecutiveErrors(activePath, 50); err == nil && n >= hookThreshold {
		if note := hookChronicPatternNote(cfg, cwd); note != "" {
			fmt.Fprintf(os.Stderr, "⚠ %d consecutive tool errors detected (%s). Stop and diagnose: call get_session_dashboard (claudewatch MCP) to check token velocity, friction patterns, and context pressure before continuing.\n", n, note)
		} else {
			fmt.Fprintf(os.Stderr, "⚠ %d consecutive tool errors detected. Stop and diagnose: call get_session_dashboard (claudewatch MCP) to check token velocity, friction patterns, and context pressure before continuing.\n", n)
		}
		os.Exit(2)
	}

	// Priority 2: context pressure.
	if ctx, err := claude.ParseLiveContextPressure(activePath); err == nil {
		if ctx.Status == "critical" || ctx.Status == "pressure" {
			pct := int(ctx.EstimatedUsage * 100)
			fmt.Fprintf(os.Stderr, "⚠ Context window at %d%% (%s). Call get_session_dashboard (claudewatch MCP) — consider compacting or wrapping up the current task before continuing.\n", pct, ctx.Status)
			os.Exit(2)
		}
	}

	// Priority 3: cost velocity.
	pricing := claude.CostPricing{
		InputPerMillion:  hookInputPerMillion,
		OutputPerMillion: hookOutputPerMillion,
	}
	if cost, err := claude.ParseLiveCostVelocity(activePath, 10, pricing); err == nil && cost.Status == "burning" {
		fmt.Fprintf(os.Stderr, "⚠ Cost velocity burning ($%.3f/min over last 10 min). Call get_session_dashboard (claudewatch MCP) to identify the source before continuing.\n", cost.CostPerMinute)
		os.Exit(2)
	}

	// Priority 4: drift detection (stuck in read loop).
	if drift, err := claude.ParseLiveDriftSignal(activePath, 15); err == nil && drift.Status == "drifting" {
		fmt.Fprintf(os.Stderr, "⚠ Drift detected: %d consecutive reads, 0 writes in last %d tools (%.0f%% read-heavy). Call get_drift_signal (claudewatch MCP) for analysis or get_blockers() if investigating an error.\n",
			drift.ReadCalls, drift.WindowN, float64(drift.ReadCalls)*100/float64(drift.WindowN))
		os.Exit(2)
	}
}

// hookChronicPatternNote returns a short description of the top friction type
// for the current project if it appears in >30% of recent sessions and CLAUDE.md
// has not been updated recently. Returns "" when no chronic pattern is found.
func hookChronicPatternNote(cfg *config.Config, cwd string) string {
	if cwd == "" {
		return ""
	}
	projectName := filepath.Base(cwd)

	sessions, _ := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	facets, _ := claude.ParseAllFacets(cfg.ClaudeHome)

	// Filter sessions to current project and take last 10.
	var projectSessions []claude.SessionMeta
	for _, sess := range sessions {
		if filepath.Base(sess.ProjectPath) == projectName {
			projectSessions = append(projectSessions, sess)
		}
	}
	if len(projectSessions) < 3 {
		return ""
	}
	// sessions are returned newest-first by ParseAllSessionMeta; take up to 10.
	window := projectSessions
	if len(window) > 10 {
		window = window[:10]
	}

	// Build friction session counts.
	idSet := make(map[string]struct{}, len(window))
	for _, sess := range window {
		idSet[sess.SessionID] = struct{}{}
	}
	frictionSessionCount := make(map[string]int)
	for _, f := range facets {
		if _, ok := idSet[f.SessionID]; !ok {
			continue
		}
		seen := make(map[string]bool)
		for ft, count := range f.FrictionCounts {
			if count > 0 && !seen[ft] {
				seen[ft] = true
				frictionSessionCount[ft]++
			}
		}
	}
	if len(frictionSessionCount) == 0 {
		return ""
	}

	// Find top friction type.
	var topType string
	var topCount int
	for ft, count := range frictionSessionCount {
		if count > topCount {
			topCount = count
			topType = ft
		}
	}

	// Must appear in >30% of window sessions.
	rate := float64(topCount) / float64(len(window))
	if rate < 0.3 {
		return ""
	}

	// Only report as chronic if CLAUDE.md is absent or hasn't been updated recently.
	claudeMDPath := filepath.Join(cwd, "CLAUDE.md")
	if info, err := os.Stat(claudeMDPath); err == nil {
		if time.Since(info.ModTime()) < 14*24*time.Hour {
			return "" // Recently updated — pattern may already be addressed.
		}
	}

	return fmt.Sprintf("chronic: %s in %.0f%% of recent sessions", topType, rate*100)
}

// shouldDisplayDashboard checks if the dashboard should be displayed based on
// tool call count. Returns true if the tool call count has crossed a multiple
// of dashboardInterval since the last display.
func shouldDisplayDashboard(activePath string) bool {
	// Get current tool call count using the full parser for accurate ToolCounts.
	meta, err := claude.ParseJSONLToSessionMeta(activePath)
	if err != nil {
		return false
	}

	toolCallCount := 0
	for _, count := range meta.ToolCounts {
		toolCallCount += count
	}

	// Check if we've crossed a dashboard interval threshold.
	countFile := os.ExpandEnv("$HOME/.cache/claudewatch-toolcount")
	lastCount := 0
	if data, err := os.ReadFile(countFile); err == nil {
		if count, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			lastCount = count
		}
	}

	// Display if we've crossed a multiple of dashboardInterval.
	// For example: 0 → 50, 50 → 100, 100 → 150, etc.
	lastInterval := lastCount / dashboardInterval
	currentInterval := toolCallCount / dashboardInterval

	return currentInterval > lastInterval && toolCallCount >= dashboardInterval
}

// recordDashboardDisplay records the current tool call count to prevent
// duplicate displays at the same interval.
func recordDashboardDisplay(activePath string, toolCallCount int) {
	countFile := os.ExpandEnv("$HOME/.cache/claudewatch-toolcount")
	_ = os.MkdirAll(os.ExpandEnv("$HOME/.cache"), 0o755)
	_ = os.WriteFile(countFile, []byte(strconv.Itoa(toolCallCount)), 0o644)
}
