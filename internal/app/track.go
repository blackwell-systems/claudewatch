package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/blackwell-systems/claudewatch/internal/suggest"
	"github.com/spf13/cobra"
)

var (
	trackCompare int
	trackJSON    bool
)

var trackCmd = &cobra.Command{
	Use:   "track",
	Short: "Snapshot and compare metrics over time",
	Long: `Run analysis, store a new snapshot, and compare against the most recent
previous snapshot to show deltas with trend arrows. Auto-resolves suggestions
whose trigger conditions are no longer true.`,
	RunE: runTrack,
}

func init() {
	trackCmd.Flags().IntVar(&trackCompare, "compare", 1, "Compare against Nth previous snapshot (1 = most recent)")
	trackCmd.Flags().BoolVar(&trackJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(trackCmd)
}

func runTrack(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	// Open the database.
	db, err := store.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Run all analysis.
	projects, err := scanner.DiscoverProjects(cfg.ScanPaths)
	if err != nil {
		return fmt.Errorf("discovering projects: %w", err)
	}

	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing session meta: %w", err)
	}

	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing facets: %w", err)
	}

	settings, err := claude.ParseSettings(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing settings: %w", err)
	}

	agentTasks, err := claude.ParseAgentTasks(cfg.ClaudeHome)
	if err != nil {
		agentTasks = nil
	}

	// Compute metrics.
	friction := analyzer.AnalyzeFriction(facets, cfg.Friction.RecurringThreshold)
	velocity := analyzer.AnalyzeVelocity(sessions, 0)
	satisfaction := analyzer.AnalyzeSatisfaction(facets)
	efficiency := analyzer.AnalyzeEfficiency(sessions)
	agentPerf := analyzer.AnalyzeAgents(agentTasks)

	// Score projects.
	for i := range projects {
		projects[i].Score = scanner.ComputeReadiness(&projects[i], sessions, facets, settings)
		// Count sessions for this project.
		count := 0
		for _, s := range sessions {
			if normalizePath(s.ProjectPath) == normalizePath(projects[i].Path) {
				count++
			}
		}
		projects[i].SessionCount = count
	}

	// Create new snapshot.
	snapshotID, err := db.CreateSnapshot("track", appVersion)
	if err != nil {
		return fmt.Errorf("creating snapshot: %w", err)
	}

	// Insert project scores.
	for _, p := range projects {
		ps := &store.ProjectScore{
			SnapshotID:       snapshotID,
			Project:          p.Path,
			Score:            p.Score,
			HasClaudeMD:      p.HasClaudeMD,
			HasDotClaude:     p.HasDotClaude,
			HasLocalSettings: p.HasLocalSettings,
			SessionCount:     p.SessionCount,
			LastSessionDate:  p.LastSessionDate,
			PrimaryLanguage:  p.PrimaryLanguage,
			GitCommit30D:     p.CommitsLast30Days,
		}
		if err := db.InsertProjectScore(ps); err != nil {
			return fmt.Errorf("inserting project score: %w", err)
		}
	}

	// Insert aggregate metrics.
	metrics := buildAggregateMetrics(friction, velocity, satisfaction, efficiency, agentPerf)
	for name, value := range metrics {
		if err := db.InsertAggregateMetric(snapshotID, name, value, ""); err != nil {
			return fmt.Errorf("inserting metric %s: %w", name, err)
		}
	}

	// Insert friction events.
	for _, f := range facets {
		for frictionType, count := range f.FrictionCounts {
			fe := &store.FrictionEvent{
				SnapshotID:   snapshotID,
				SessionID:    f.SessionID,
				FrictionType: frictionType,
				Count:        count,
			}
			if err := db.InsertFrictionEvent(fe); err != nil {
				return fmt.Errorf("inserting friction event: %w", err)
			}
		}
	}

	// Insert agent tasks.
	for _, task := range agentTasks {
		at := &store.AgentTaskRow{
			SnapshotID:  snapshotID,
			SessionID:   task.SessionID,
			AgentID:     task.AgentID,
			AgentType:   task.AgentType,
			Description: task.Description,
			Status:      task.Status,
			DurationMs:  task.DurationMs,
			TotalTokens: task.TotalTokens,
			ToolUses:    task.ToolUses,
			Background:  task.Background,
			CreatedAt:   task.CreatedAt,
		}
		if err := db.InsertAgentTask(at); err != nil {
			return fmt.Errorf("inserting agent task: %w", err)
		}
	}

	// Run suggest engine and store suggestions.
	suggestCtx, err := buildAnalysisContext(cfg)
	if err != nil {
		return fmt.Errorf("building suggest context: %w", err)
	}
	engine := suggest.NewEngine()
	suggestions := engine.Run(suggestCtx)
	for _, s := range suggestions {
		ss := &store.Suggestion{
			SnapshotID:  snapshotID,
			Category:    s.Category,
			Priority:    s.Priority,
			Title:       s.Title,
			Description: s.Description,
			ImpactScore: s.ImpactScore,
			Status:      "open",
		}
		if err := db.InsertSuggestion(ss); err != nil {
			return fmt.Errorf("inserting suggestion: %w", err)
		}
	}

	// Load previous snapshot for comparison.
	// GetSnapshotN(2) gets the second most recent (the one before the one we just created).
	prevSnapshot, err := db.GetSnapshotN(2)
	if err != nil {
		return fmt.Errorf("loading previous snapshot: %w", err)
	}

	currentSnapshot, err := db.GetSnapshot(snapshotID)
	if err != nil {
		return fmt.Errorf("loading current snapshot: %w", err)
	}

	// Compute deltas.
	var diff *store.SnapshotDiff
	if prevSnapshot != nil {
		prevMetrics, err := db.GetAggregateMetrics(prevSnapshot.ID)
		if err != nil {
			return fmt.Errorf("loading previous metrics: %w", err)
		}

		currMetrics, err := db.GetAggregateMetrics(snapshotID)
		if err != nil {
			return fmt.Errorf("loading current metrics: %w", err)
		}

		deltas := computeDeltas(prevMetrics, currMetrics)
		diff = &store.SnapshotDiff{
			Previous: prevSnapshot,
			Current:  currentSnapshot,
			Deltas:   deltas,
		}

		// Auto-resolve suggestions whose conditions have cleared.
		if err := autoResolveSuggestions(db, suggestCtx); err != nil {
			return fmt.Errorf("auto-resolving suggestions: %w", err)
		}
	}

	if trackJSON || flagJSON {
		return outputTrackJSON(currentSnapshot, diff)
	}

	renderTrackOutput(currentSnapshot, diff)
	return nil
}

// buildAggregateMetrics produces a flat map of metric name to value from
// the various analyzer results.
func buildAggregateMetrics(
	friction analyzer.FrictionSummary,
	velocity analyzer.VelocityMetrics,
	satisfaction analyzer.SatisfactionScore,
	efficiency analyzer.EfficiencyMetrics,
	agentPerf analyzer.AgentPerformance,
) map[string]float64 {
	m := map[string]float64{
		"total_sessions":              float64(velocity.TotalSessions),
		"avg_lines_added_per_session": velocity.AvgLinesAddedPerSession,
		"avg_commits_per_session":     velocity.AvgCommitsPerSession,
		"avg_files_modified":          velocity.AvgFilesModifiedPerSession,
		"avg_duration_minutes":        velocity.AvgDurationMinutes,
		"avg_messages_per_session":    velocity.AvgMessagesPerSession,
		"total_friction_events":       float64(friction.TotalFrictionEvents),
		"sessions_with_friction":      float64(friction.SessionsWithFriction),
		"satisfaction_score":          satisfaction.WeightedScore,
		"avg_tool_errors":             efficiency.AvgToolErrorsPerSession,
		"avg_interruptions":           efficiency.AvgInterruptionsPerSession,
		"avg_tokens_per_session":      efficiency.AvgTokensPerSession,
		"agent_total":                 float64(agentPerf.TotalAgents),
		"agent_success_rate":          agentPerf.SuccessRate * 100,
		"agent_background_ratio":      agentPerf.BackgroundRatio * 100,
	}
	return m
}

// metricDirection maps metric names to whether higher values are better.
var metricDirection = map[string]bool{
	"total_sessions":              true,
	"avg_lines_added_per_session": true,
	"avg_commits_per_session":     true,
	"avg_files_modified":          true,
	"avg_duration_minutes":        false, // shorter sessions can be more efficient
	"avg_messages_per_session":    false, // fewer messages = more efficient
	"total_friction_events":       false, // lower friction = better
	"sessions_with_friction":      false,
	"satisfaction_score":          true,
	"avg_tool_errors":             false,
	"avg_interruptions":           false,
	"avg_tokens_per_session":      false, // lower token usage = more efficient
	"agent_total":                 true,
	"agent_success_rate":          true,
	"agent_background_ratio":      true,
}

// computeDeltas compares two sets of aggregate metrics and returns MetricDelta entries.
func computeDeltas(prev, curr []store.AggregateMetric) []store.MetricDelta {
	prevMap := make(map[string]float64)
	for _, m := range prev {
		prevMap[m.MetricName] = m.MetricValue
	}

	var deltas []store.MetricDelta
	for _, m := range curr {
		prevVal := prevMap[m.MetricName]
		delta := m.MetricValue - prevVal

		direction := "unchanged"
		if delta != 0 {
			higherIsBetter, known := metricDirection[m.MetricName]
			if !known {
				higherIsBetter = true // default assumption
			}
			isPositive := delta > 0
			if (isPositive && higherIsBetter) || (!isPositive && !higherIsBetter) {
				direction = "improved"
			} else {
				direction = "regressed"
			}
		}

		deltas = append(deltas, store.MetricDelta{
			Name:      m.MetricName,
			Previous:  prevVal,
			Current:   m.MetricValue,
			Delta:     delta,
			Direction: direction,
		})
	}

	return deltas
}

// autoResolveSuggestions resolves open suggestions whose trigger conditions
// are no longer true.
func autoResolveSuggestions(db *store.DB, ctx *suggest.AnalysisContext) error {
	openSuggestions, err := db.GetOpenSuggestions()
	if err != nil {
		return err
	}

	// Build a set of current project names that still lack CLAUDE.md.
	missingCMD := make(map[string]bool)
	for _, p := range ctx.Projects {
		if p.SessionCount > 0 && !p.HasClaudeMD {
			missingCMD[p.Name] = true
		}
	}

	for _, s := range openSuggestions {
		shouldResolve := false

		switch s.Category {
		case "configuration":
			// Resolve "Add CLAUDE.md" suggestions if the project now has one.
			for _, p := range ctx.Projects {
				if p.HasClaudeMD && strings.Contains(s.Title, p.Name) {
					shouldResolve = true
					break
				}
			}
		case "friction":
			// Resolve recurring friction if no longer recurring.
			if len(ctx.RecurringFriction) == 0 {
				shouldResolve = true
			}
		}

		if shouldResolve {
			if err := db.ResolveSuggestion(s.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

func outputTrackJSON(current *store.Snapshot, diff *store.SnapshotDiff) error {
	result := map[string]any{
		"snapshot": current,
	}
	if diff != nil {
		result["diff"] = diff
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func renderTrackOutput(current *store.Snapshot, diff *store.SnapshotDiff) {
	fmt.Println(output.Section("Track: Snapshot Comparison"))
	fmt.Println()
	fmt.Printf(" Snapshot #%d taken at %s\n\n", current.ID, current.TakenAt.Format("2006-01-02 15:04:05"))

	if diff == nil {
		fmt.Println(" First snapshot recorded. Run 'claudewatch track' again later to see trends.")
		return
	}

	fmt.Printf(" Comparing against snapshot #%d (%s)\n\n",
		diff.Previous.ID, diff.Previous.TakenAt.Format("2006-01-02 15:04:05"))

	tbl := output.NewTable("Metric", "Previous", "Current", "Delta", "Trend")

	for _, d := range diff.Deltas {
		higherIsBetter, known := metricDirection[d.Name]
		if !known {
			higherIsBetter = true
		}

		trend := output.TrendArrow(d.Delta, higherIsBetter)

		tbl.AddRow(
			d.Name,
			fmt.Sprintf("%.1f", d.Previous),
			fmt.Sprintf("%.1f", d.Current),
			fmt.Sprintf("%+.1f", d.Delta),
			trend,
		)
	}

	tbl.Print()
}
