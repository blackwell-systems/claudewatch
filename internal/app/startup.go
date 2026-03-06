package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/memory"
	"github.com/blackwell-systems/claudewatch/internal/store"
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

	// Update working memory from most recent completed session.
	if err := updateWorkingMemoryIfNeeded(cfg, projectName, projectSessions, facets); err != nil {
		// Non-fatal: log to stderr, continue.
		_, _ = fmt.Fprintf(os.Stderr, "claudewatch: memory update failed: %v\n", err)
	}
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

	// Track agent performance by type
	type agentTypeSummary struct {
		count     int
		completed int
	}
	agentByType := make(map[string]*agentTypeSummary)

	for _, task := range agentTasks {
		if _, ok := sessionIDs[task.SessionID]; !ok {
			continue
		}
		projectTaskCount++
		if task.Status == "completed" {
			projectTaskCompleted++
		}

		// Track by agent type
		if agentByType[task.AgentType] == nil {
			agentByType[task.AgentType] = &agentTypeSummary{}
		}
		agentByType[task.AgentType].count++
		if task.Status == "completed" {
			agentByType[task.AgentType].completed++
		}
	}
	if projectTaskCount > 0 {
		pct := int(float64(projectTaskCompleted) / float64(projectTaskCount) * 100)
		agentSuccessStr = fmt.Sprintf("%d%%", pct)
	}

	// Identify failing agents (0% success rate)
	var failingAgents []string
	for agentType, summary := range agentByType {
		if summary.completed == 0 && summary.count > 0 {
			failingAgents = append(failingAgents, agentType)
		}
	}
	sort.Strings(failingAgents)

	// Regression check: warn if the project's friction or cost has regressed vs baseline.
	var regressionWarning string
	if db, dbErr := store.Open(config.DBPath()); dbErr == nil {
		defer func() { _ = db.Close() }()
		baseline, _ := db.GetProjectBaseline(projectName)
		regStatus := analyzer.ComputeRegressionStatus(analyzer.RegressionInput{
			Project:        projectName,
			Baseline:       baseline,
			RecentSessions: projectSessions,
			Facets:         facets,
			Pricing:        analyzer.DefaultPricing["sonnet"],
			CacheRatio:     analyzer.NoCacheRatio(),
			Threshold:      1.5,
		})
		if regStatus.Regressed {
			regressionWarning = fmt.Sprintf("║ ⚠ regression: %s\n", regStatus.Message)
		}
	}

	// Average tool errors per session
	var totalToolErrors int
	for _, sess := range projectSessions {
		totalToolErrors += sess.ToolErrors
	}
	avgToolErrors := 0.0
	if sessionCount > 0 {
		avgToolErrors = float64(totalToolErrors) / float64(sessionCount)
	}

	// Count new blockers since last session
	newBlockerCount := 0
	memoryPath := filepath.Join(config.ConfigDir(), "working-memory.json")
	if memStore := store.NewWorkingMemoryStore(memoryPath); memStore != nil {
		if wm, wmErr := memStore.Load(); wmErr == nil {
			// Find second-most-recent session end time (if exists)
			if len(projectSessions) > 1 {
				// Sessions are sorted newest-first in ParseAllSessionMeta
				sort.Slice(projectSessions, func(i, j int) bool {
					return projectSessions[i].StartTime > projectSessions[j].StartTime
				})
				secondMostRecent := projectSessions[1]
				lastSessionEnd := claude.ParseTimestamp(secondMostRecent.StartTime).Add(
					time.Duration(secondMostRecent.DurationMinutes) * time.Minute,
				)
				for _, blocker := range wm.Blockers {
					if blocker.LastSeen.After(lastSessionEnd) {
						newBlockerCount++
					}
				}
			}
		}
	}

	// SAW correlation: does SAW reduce zero-commit rate for this project?
	tip := startupTip(topFriction)
	spans, spanErr := claude.ParseSessionTranscripts(cfg.ClaudeHome)
	if spanErr == nil {
		sawSessionMap := make(map[string]bool)
		for _, saw := range claude.ComputeSAWWaves(spans) {
			sawSessionMap[saw.SessionID] = true
		}
		projectPathMap := make(map[string]string, len(sessions))
		for _, sess := range sessions {
			projectPathMap[sess.SessionID] = sess.ProjectPath
		}
		report, corrErr := analyzer.CorrelateFactors(analyzer.CorrelateInput{
			Sessions:    sessions,
			Facets:      facets,
			SAWSessions: sawSessionMap,
			ProjectPath: projectPathMap,
			Pricing:     analyzer.DefaultPricing["sonnet"],
			CacheRatio:  analyzer.NoCacheRatio(),
			Project:     projectName,
			Outcome:     analyzer.OutcomeZeroCommit,
			Factor:      analyzer.FactorIsSAW,
		})
		if corrErr == nil && report.SingleGroupComparison != nil {
			gc := report.SingleGroupComparison
			// SAW (true group) has meaningfully lower zero-commit rate than non-SAW sessions.
			if !gc.LowConfidence && gc.Delta < -0.1 {
				tip = fmt.Sprintf("tip: SAW reduces zero-commit rate (%.0f%% vs %.0f%% without)",
					gc.TrueGroup.AvgOutcome*100, gc.FalseGroup.AvgOutcome*100)
			}
		}
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

	// Print to stdout so Claude Code injects this into Claude's context at session start.
	// SessionStart hooks: stdout → Claude's context. stderr + exit 2 → user terminal only.
	fmt.Printf("╔ claudewatch | %s | %s | friction: %s\n", projectName, sessionStr, frictionStr)

	// High friction warning
	if frictionRate >= 0.6 {
		fmt.Printf("║ ⚠ HIGH FRICTION ENVIRONMENT - verify commands before execution\n")
	} else if frictionRate >= 0.3 {
		fmt.Printf("║ ⚠ Moderate friction detected - watch for error patterns\n")
	}

	// Line 2: CLAUDE.md status, agent success, avg tool errors
	avgToolErrorsStr := fmt.Sprintf("%.1f", avgToolErrors)
	fmt.Printf("║ CLAUDE.md: %s | agent success: %s | avg %s tool errors/session (project baseline)\n",
		claudeMD, agentSuccessStr, avgToolErrorsStr)

	// Agent failures by type
	if len(failingAgents) > 0 {
		fmt.Printf("║\n")
		fmt.Printf("║ Agent failures by type:\n")
		for _, agentType := range failingAgents {
			summary := agentByType[agentType]
			fmt.Printf("║   • %s: 0%% success (0/%d completed) - DO NOT SPAWN\n", agentType, summary.count)
		}
	}

	// New blockers count
	if newBlockerCount > 0 {
		fmt.Printf("║\n")
		fmt.Printf("║ %d new blocker(s) since last session → call get_blockers() for details\n", newBlockerCount)
	}

	// Regression warning
	if regressionWarning != "" {
		fmt.Printf("║\n")
		fmt.Print(regressionWarning)
	}

	// Contextual memory surfacing: auto-query task history based on first user message.
	// This only runs if there's an active session with a user message available.
	activePath, _ := claude.FindActiveSessionPath(cfg.ClaudeHome)
	if activePath != "" {
		activeSession, parseErr := claude.ParseActiveSession(activePath)
		if parseErr == nil && activeSession != nil && activeSession.FirstPrompt != "" {
			memoryPath := filepath.Join(config.ConfigDir(), "working-memory.json")
			memStore := store.NewWorkingMemoryStore(memoryPath)
			surfaceResult, surfaceErr := memory.SurfaceRelevantMemory(activeSession.FirstPrompt, projectName, memStore)

			if surfaceErr == nil && len(surfaceResult.MatchedTasks) > 0 {
				fmt.Printf("║\n")
				fmt.Printf("║ 📋 TASK HISTORY MATCH (keywords: %s)\n", strings.Join(surfaceResult.Keywords, ", "))
				fmt.Printf("║ Found %d prior attempt(s) on this project:\n", len(surfaceResult.MatchedTasks))

				maxDisplay := 3
				if len(surfaceResult.MatchedTasks) < maxDisplay {
					maxDisplay = len(surfaceResult.MatchedTasks)
				}

				for i := 0; i < maxDisplay; i++ {
					task := surfaceResult.MatchedTasks[i]
					statusIcon := "✓"
					if task.Status == "abandoned" || task.Status == "in_progress" {
						statusIcon = "✗"
					}

					timeAgo := formatTimeAgo(task.LastUpdated)
					fmt.Printf("║   %s %s (%s, %s)\n", statusIcon, task.TaskIdentifier, task.Status, timeAgo)

					if len(task.BlockersHit) > 0 {
						fmt.Printf("║      Blockers: %s\n", strings.Join(task.BlockersHit, ", "))
					}
					if task.Solution != "" {
						fmt.Printf("║      Solution: %s\n", truncateString(task.Solution, 60))
					}
				}

				if len(surfaceResult.Keywords) > 0 {
					fmt.Printf("║   → Call get_task_history(\"%s\") for full details\n", surfaceResult.Keywords[0])
				}
			}
		}
	}

	// Tip line
	fmt.Printf("║\n")
	fmt.Printf("║ %s\n", tip)

	// Tools listing and hook status
	fmt.Printf("║\n")
	fmt.Printf("║ tools: get_session_dashboard · get_project_health · get_task_history · get_blockers · extract_current_session_memory · get_live_friction · get_context_pressure · get_cost_velocity · get_suggestions\n")
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

// updateWorkingMemoryIfNeeded checks if the most recent completed session
// for this project is missing from working memory. If so, extracts and stores it.
func updateWorkingMemoryIfNeeded(cfg *config.Config, projectName string, sessions []claude.SessionMeta, facets []claude.SessionFacet) error {
	if len(sessions) == 0 {
		return nil
	}

	// Find the active session path to exclude it.
	activePath, _ := claude.FindActiveSessionPath(cfg.ClaudeHome)
	activeSessionID := ""
	if activePath != "" {
		activeSessionID = strings.TrimSuffix(filepath.Base(filepath.Dir(activePath)), ".jsonl")
	}

	// Find most recent completed session (not the active one).
	var mostRecent *claude.SessionMeta
	for i := range sessions {
		if sessions[i].SessionID == activeSessionID {
			continue
		}
		if mostRecent == nil || sessions[i].StartTime > mostRecent.StartTime {
			mostRecent = &sessions[i]
		}
	}

	if mostRecent == nil {
		return nil
	}

	// Find the facet for this session.
	var sessionFacet *claude.SessionFacet
	for i := range facets {
		if facets[i].SessionID == mostRecent.SessionID {
			sessionFacet = &facets[i]
			break
		}
	}

	if sessionFacet == nil {
		// No facet means no AI analysis available; skip.
		return nil
	}

	// Open working memory store.
	memoryPath := filepath.Join(config.ConfigDir(), "working-memory.json")
	memStore := store.NewWorkingMemoryStore(memoryPath)
	wm, err := memStore.Load()
	if err != nil {
		return fmt.Errorf("load working memory: %w", err)
	}

	// Check if this session is already in working memory.
	for _, task := range wm.Tasks {
		for _, sid := range task.Sessions {
			if sid == mostRecent.SessionID {
				// Already extracted.
				return nil
			}
		}
	}

	// Extract commits.

	// Build transcript path for semantic extraction
	transcriptPath := ""
	if mostRecent.ProjectPath != "" {
		projectHash := filepath.Base(mostRecent.ProjectPath)
		transcriptPath = filepath.Join(cfg.ClaudeHome, "projects", projectHash, mostRecent.SessionID+".jsonl")
	}
	commits := memory.GetCommitSHAsSince(mostRecent.ProjectPath, mostRecent.StartTime)

	// Extract task memory.
	task, err := memory.ExtractTaskMemory(*mostRecent, sessionFacet, commits, transcriptPath)
	if err != nil {
		return fmt.Errorf("extract task memory: %w", err)
	}
	if task != nil {
		if err := memStore.AddOrUpdateTask(task); err != nil {
			return fmt.Errorf("store task memory: %w", err)
		}
	}

	// Extract blockers (take last 10 sessions for chronic pattern detection).
	recentSessions := sessions
	if len(recentSessions) > 10 {
		recentSessions = recentSessions[:10]
	}
	blockers, err := memory.ExtractBlockers(*mostRecent, sessionFacet, projectName, recentSessions, facets, transcriptPath)
	if err != nil {
		return fmt.Errorf("extract blockers: %w", err)
	}
	for _, blocker := range blockers {
		if err := memStore.AddBlocker(blocker); err != nil {
			return fmt.Errorf("store blocker: %w", err)
		}
	}

	return nil
}

// truncateString truncates a string to maxLen with ellipsis.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
