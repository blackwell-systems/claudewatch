package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/spf13/cobra"
)

var startupCmd = &cobra.Command{
	Use:   "startup",
	Short: "Print session start briefing (for use as a SessionStart shell hook)",
	Long: `Prints a compact session briefing to stderr: project health snapshot,
available MCP tools, and hook status. Designed for use as a Claude Code
SessionStart shell hook to orient Claude at the start of every session.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run:           runStartup,
}

func init() {
	rootCmd.AddCommand(startupCmd)
}

func runStartup(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return
	}

	// Determine project from cwd.
	cwd, _ := os.Getwd()
	projectName := filepath.Base(cwd)

	// Load session metadata and filter to this project.
	sessions, _ := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	var projectSessions []claude.SessionMeta
	sessionIDs := make(map[string]struct{})
	for _, sess := range sessions {
		if claude.NormalizePath(sess.ProjectPath) == claude.NormalizePath(cwd) {
			projectSessions = append(projectSessions, sess)
			sessionIDs[sess.SessionID] = struct{}{}
		}
	}
	sessionCount := len(projectSessions)

	// Friction data from facets.
	facets, _ := claude.ParseAllFacets(cfg.ClaudeHome)
	frictionTypeCounts := make(map[string]int)
	frictionSessionCount := 0
	for _, f := range facets {
		if _, ok := sessionIDs[f.SessionID]; !ok {
			continue
		}
		if len(f.FrictionCounts) > 0 {
			frictionSessionCount++
		}
		for ft, count := range f.FrictionCounts {
			frictionTypeCounts[ft] += count
		}
	}

	frictionRate := 0.0
	if sessionCount > 0 {
		frictionRate = float64(frictionSessionCount) / float64(sessionCount)
	}
	frictionLabel := startupFrictionLabel(frictionRate)
	topFriction := startupTopFriction(frictionTypeCounts)

	// CLAUDE.md presence.
	claudeMD := "✗"
	if cwd != "" {
		if _, statErr := os.Stat(filepath.Join(cwd, "CLAUDE.md")); statErr == nil {
			claudeMD = "✓"
		}
	}

	// Agent success rate for this project.
	agentTasks, _ := claude.ParseAgentTasks(cfg.ClaudeHome)
	agentSuccessStr := "n/a"
	var projectTaskCount, projectTaskCompleted int
	for _, task := range agentTasks {
		if _, ok := sessionIDs[task.SessionID]; !ok {
			continue
		}
		projectTaskCount++
		if task.Status == "completed" {
			projectTaskCompleted++
		}
	}
	if projectTaskCount > 0 {
		pct := int(float64(projectTaskCompleted) / float64(projectTaskCount) * 100)
		agentSuccessStr = fmt.Sprintf("%d%%", pct)
	}

	// Build line 1: identity + friction signal.
	sessionStr := fmt.Sprintf("%d session", sessionCount)
	if sessionCount != 1 {
		sessionStr += "s"
	}
	frictionStr := frictionLabel
	if topFriction != "" {
		frictionStr = fmt.Sprintf("%s (%s dominant)", frictionLabel, topFriction)
	}

	// Build line 2: readiness + tip.
	tip := startupTip(topFriction)

	// Print to stdout so Claude Code injects this into Claude's context at session start.
	// SessionStart hooks: stdout → Claude's context. stderr + exit 2 → user terminal only.
	fmt.Printf("╔ claudewatch | %s | %s | friction: %s\n", projectName, sessionStr, frictionStr)
	fmt.Printf("║ CLAUDE.md: %s | agent success: %s | %s\n", claudeMD, agentSuccessStr, tip)
	fmt.Printf("║ tools: get_session_dashboard · get_project_health · get_live_friction · get_context_pressure · get_cost_velocity · get_suggestions\n")
	fmt.Printf("╚ PostToolUse hook active → fires on errors/context/cost → call get_session_dashboard\n")
}

func startupFrictionLabel(rate float64) string {
	switch {
	case rate >= 0.6:
		return "HIGH"
	case rate >= 0.3:
		return "moderate"
	default:
		return "low"
	}
}

func startupTopFriction(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	type kv struct {
		name  string
		count int
	}
	var entries []kv
	for name, count := range counts {
		entries = append(entries, kv{name, count})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})
	return entries[0].name
}

func startupTip(topFriction string) string {
	switch {
	case strings.HasPrefix(topFriction, "retry:Bash"):
		return "tip: verify Bash commands before running"
	case strings.HasPrefix(topFriction, "retry:Edit"):
		return "tip: read files before editing"
	case strings.HasPrefix(topFriction, "retry:Read"):
		return "tip: confirm paths exist before reading"
	case strings.HasPrefix(topFriction, "tool_error"):
		return "tip: check tool inputs carefully"
	case topFriction != "":
		return fmt.Sprintf("tip: top friction is %s", topFriction)
	default:
		return "tip: call get_project_health for project baseline"
	}
}
