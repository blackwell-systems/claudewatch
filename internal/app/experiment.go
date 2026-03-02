package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/spf13/cobra"
)

var experimentCmd = &cobra.Command{
	Use:   "experiment",
	Short: "Manage CLAUDE.md A/B experiments",
	Long: `Create and report on A/B experiments that compare two CLAUDE.md variants.

Subcommands: start, stop, tag, report`,
}

func init() {
	rootCmd.AddCommand(experimentCmd)
}

// experiment start

var (
	experimentStartFlagProject string
	experimentStartFlagNote    string
)

var experimentStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a new A/B experiment for a project",
	RunE:  runExperimentStart,
}

func init() {
	experimentStartCmd.Flags().StringVar(&experimentStartFlagProject, "project", "", "Project name (required)")
	_ = experimentStartCmd.MarkFlagRequired("project")
	experimentStartCmd.Flags().StringVar(&experimentStartFlagNote, "note", "", "Optional note for the experiment")
	experimentCmd.AddCommand(experimentStartCmd)
}

func runExperimentStart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	_ = cfg

	db, err := store.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	id, err := db.CreateExperiment(experimentStartFlagProject, experimentStartFlagNote)
	if err != nil {
		return err
	}

	fmt.Printf("Experiment #%d started for project %q.\n", id, experimentStartFlagProject)
	return nil
}

// experiment stop

var experimentStopFlagProject string

var experimentStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the active experiment for a project",
	RunE:  runExperimentStop,
}

func init() {
	experimentStopCmd.Flags().StringVar(&experimentStopFlagProject, "project", "", "Project name (required)")
	_ = experimentStopCmd.MarkFlagRequired("project")
	experimentCmd.AddCommand(experimentStopCmd)
}

func runExperimentStop(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	_ = cfg

	db, err := store.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	exp, err := db.GetActiveExperiment(experimentStopFlagProject)
	if err != nil {
		return fmt.Errorf("getting active experiment: %w", err)
	}
	if exp == nil {
		return fmt.Errorf("no active experiment for project %q", experimentStopFlagProject)
	}

	if err := db.StopExperiment(exp.ID); err != nil {
		return fmt.Errorf("stopping experiment: %w", err)
	}

	fmt.Printf("Experiment #%d stopped.\n", exp.ID)
	return nil
}

// experiment tag

var (
	experimentTagFlagProject string
	experimentTagFlagSession string
	experimentTagFlagVariant string
)

var experimentTagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Tag a session as variant a or b in the active experiment",
	RunE:  runExperimentTag,
}

func init() {
	experimentTagCmd.Flags().StringVar(&experimentTagFlagProject, "project", "", "Project name (required)")
	_ = experimentTagCmd.MarkFlagRequired("project")
	experimentTagCmd.Flags().StringVar(&experimentTagFlagSession, "session", "", "Session ID (default: most recent)")
	experimentTagCmd.Flags().StringVar(&experimentTagFlagVariant, "variant", "", "Variant to assign: \"a\" or \"b\" (required)")
	_ = experimentTagCmd.MarkFlagRequired("variant")
	experimentCmd.AddCommand(experimentTagCmd)
}

func runExperimentTag(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	db, err := store.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	exp, err := db.GetActiveExperiment(experimentTagFlagProject)
	if err != nil {
		return fmt.Errorf("getting active experiment: %w", err)
	}
	if exp == nil {
		return fmt.Errorf("no active experiment for project %q", experimentTagFlagProject)
	}

	sessionID := experimentTagFlagSession
	if sessionID == "" {
		sessions, parseErr := claude.ParseAllSessionMeta(cfg.ClaudeHome)
		if parseErr != nil {
			return fmt.Errorf("parsing session meta: %w", parseErr)
		}
		if len(sessions) == 0 {
			return fmt.Errorf("no sessions found")
		}
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].StartTime > sessions[j].StartTime
		})
		sessionID = sessions[0].SessionID
	}

	if experimentTagFlagVariant != "a" && experimentTagFlagVariant != "b" {
		return fmt.Errorf("variant must be \"a\" or \"b\", got %q", experimentTagFlagVariant)
	}

	if err := db.RecordSessionVariant(exp.ID, sessionID, experimentTagFlagVariant); err != nil {
		return fmt.Errorf("recording session variant: %w", err)
	}

	fmt.Printf("Tagged session %s as variant %s in experiment #%d.\n", sessionID, experimentTagFlagVariant, exp.ID)
	return nil
}

// experiment report

var (
	experimentReportFlagProject string
)

var experimentReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Show the results of an experiment",
	RunE:  runExperimentReport,
}

func init() {
	experimentReportCmd.Flags().StringVar(&experimentReportFlagProject, "project", "", "Project name (required)")
	_ = experimentReportCmd.MarkFlagRequired("project")
	experimentCmd.AddCommand(experimentReportCmd)
}

func runExperimentReport(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	db, err := store.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	exp, err := db.GetActiveExperiment(experimentReportFlagProject)
	if err != nil {
		return fmt.Errorf("getting active experiment: %w", err)
	}
	if exp == nil {
		// Fall back to most recent stopped experiment.
		all, listErr := db.ListExperiments(experimentReportFlagProject)
		if listErr != nil {
			return fmt.Errorf("listing experiments: %w", listErr)
		}
		if len(all) == 0 {
			return fmt.Errorf("no experiments found for project %q", experimentReportFlagProject)
		}
		exp = &all[0]
	}

	expSessions, err := db.GetExperimentSessions(exp.ID)
	if err != nil {
		return fmt.Errorf("getting experiment sessions: %w", err)
	}

	assignments := make(map[string]string, len(expSessions))
	for _, es := range expSessions {
		assignments[es.SessionID] = es.Variant
	}

	allSessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing session meta: %w", err)
	}

	var filteredSessions []claude.SessionMeta
	for _, s := range allSessions {
		if _, ok := assignments[s.SessionID]; ok {
			filteredSessions = append(filteredSessions, s)
		}
	}

	allFacets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing facets: %w", err)
	}

	var filteredFacets []claude.SessionFacet
	for _, f := range allFacets {
		if _, ok := assignments[f.SessionID]; ok {
			filteredFacets = append(filteredFacets, f)
		}
	}

	pricing := analyzer.DefaultPricing["sonnet"]
	ratio := analyzer.NoCacheRatio()
	if sc, scErr := claude.ParseStatsCache(cfg.ClaudeHome); scErr == nil && sc != nil {
		ratio = analyzer.ComputeCacheRatio(*sc)
	}

	report := analyzer.AnalyzeExperiment(*exp, filteredSessions, filteredFacets, assignments, pricing, ratio)

	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}

	renderExperimentReport(report)
	return nil
}

func renderExperimentReport(report analyzer.ExperimentReport) {
	fmt.Println(output.Section(fmt.Sprintf("Experiment Report — %s #%d", report.Project, report.ExperimentID)))
	fmt.Println()

	tbl := output.NewTable("Metric", "Variant A", "Variant B")
	tbl.AddRow("Sessions",
		fmt.Sprintf("%d", report.A.SessionCount),
		fmt.Sprintf("%d", report.B.SessionCount),
	)
	tbl.AddRow("Avg Cost",
		fmt.Sprintf("$%.3f", report.A.AvgCostUSD),
		fmt.Sprintf("$%.3f", report.B.AvgCostUSD),
	)
	tbl.AddRow("Avg Friction",
		fmt.Sprintf("%.1f", report.A.AvgFriction),
		fmt.Sprintf("%.1f", report.B.AvgFriction),
	)
	tbl.AddRow("Avg Commits",
		fmt.Sprintf("%.1f", report.A.AvgCommits),
		fmt.Sprintf("%.1f", report.B.AvgCommits),
	)
	tbl.Print()
	fmt.Println()

	winnerLine := formatWinnerLine(report)
	fmt.Printf(" %s\n", winnerLine)
	fmt.Println()
}

func formatWinnerLine(report analyzer.ExperimentReport) string {
	if report.Winner == "inconclusive" {
		return output.StyleMuted.Render("Winner: inconclusive")
	}

	var winnerLabel string
	var loserStats, winnerStats analyzer.VariantStats
	switch report.Winner {
	case "a":
		winnerLabel = "A"
		winnerStats = report.A
		loserStats = report.B
	case "b":
		winnerLabel = "B"
		winnerStats = report.B
		loserStats = report.A
	default:
		return output.StyleMuted.Render("Winner: inconclusive")
	}

	// Compute cost difference percentage.
	if loserStats.AvgCostUSD > 0 {
		diff := (loserStats.AvgCostUSD - winnerStats.AvgCostUSD) / loserStats.AvgCostUSD * 100
		return output.StyleSuccess.Render(fmt.Sprintf("Winner: %s — cost %.0f%% lower", winnerLabel, diff))
	}

	return output.StyleSuccess.Render(fmt.Sprintf("Winner: %s", winnerLabel))
}
