package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/spf13/cobra"
)

var (
	anomaliesFlagProject   string
	anomaliesFlagThreshold float64
)

var anomaliesCmd = &cobra.Command{
	Use:   "anomalies",
	Short: "Detect anomalous sessions for a project",
	Long: `Detect sessions that deviate significantly from the project's historical baseline.

Anomalies are identified by cost and friction z-scores exceeding the threshold.
On first use, the baseline is computed from all available sessions and stored in
the database for future comparisons.

If --project is not specified, the project from the most recent session is used.

Examples:
  claudewatch anomalies
  claudewatch anomalies --project claudewatch
  claudewatch anomalies --threshold 3.0
  claudewatch anomalies --json`,
	RunE: runAnomalies,
}

func init() {
	anomaliesCmd.Flags().StringVar(&anomaliesFlagProject, "project", "", "Project name (default: most recent session's project)")
	anomaliesCmd.Flags().Float64Var(&anomaliesFlagThreshold, "threshold", 2.0, "Z-score threshold for anomaly detection")
	rootCmd.AddCommand(anomaliesCmd)
}

func runAnomalies(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing session meta: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Printf(" %s\n", output.StyleMuted.Render("No sessions found."))
		return nil
	}

	// Determine project: use flag or derive from most recent session.
	project := anomaliesFlagProject
	if project == "" {
		sorted := make([]claude.SessionMeta, len(sessions))
		copy(sorted, sessions)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].StartTime > sorted[j].StartTime
		})
		project = filepath.Base(sorted[0].ProjectPath)
	}

	// Filter sessions to the requested project.
	var projectSessions []claude.SessionMeta
	for _, s := range sessions {
		name := filepath.Base(s.ProjectPath)
		if strings.EqualFold(name, project) ||
			strings.Contains(strings.ToLower(s.ProjectPath), strings.ToLower(project)) {
			projectSessions = append(projectSessions, s)
		}
	}

	if len(projectSessions) == 0 {
		return fmt.Errorf("no sessions found for project %q", project)
	}

	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing facets: %w", err)
	}

	pricing := analyzer.DefaultPricing["sonnet"]
	cacheRatio := analyzer.NoCacheRatio()
	if sc, scErr := claude.ParseStatsCache(cfg.ClaudeHome); scErr == nil && sc != nil {
		cacheRatio = analyzer.ComputeCacheRatio(*sc)
	}

	// Open DB for baseline retrieval / storage.
	db, dbErr := store.Open(config.DBPath())
	if dbErr != nil {
		return fmt.Errorf("opening database: %w", dbErr)
	}
	defer func() { _ = db.Close() }()

	// Fetch or compute the baseline.
	baseline, err := db.GetProjectBaseline(project)
	if err != nil {
		return fmt.Errorf("fetching project baseline: %w", err)
	}

	if baseline == nil {
		// Compute baseline on the fly.
		spans, _ := claude.ParseSessionTranscripts(cfg.ClaudeHome)
		sawSessions := claude.ComputeSAWWaves(spans)
		sawIDs := make(map[string]bool, len(sawSessions))
		for _, ss := range sawSessions {
			sawIDs[ss.SessionID] = true
		}

		computed, computeErr := analyzer.ComputeProjectBaseline(analyzer.BaselineInput{
			Project:    project,
			Sessions:   projectSessions,
			Facets:     facets,
			SAWIDs:     sawIDs,
			Pricing:    pricing,
			CacheRatio: cacheRatio,
		})
		if computeErr != nil {
			return fmt.Errorf("computing baseline for %q: %w", project, computeErr)
		}

		if upsertErr := db.UpsertProjectBaseline(computed); upsertErr != nil {
			// Non-fatal: warn but continue with the computed baseline.
			fmt.Fprintf(os.Stderr, "warning: could not store baseline: %v\n", upsertErr)
		}

		baseline = &computed
	}

	anomalies := analyzer.DetectAnomalies(projectSessions, facets, *baseline, pricing, cacheRatio, anomaliesFlagThreshold)

	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"project":   project,
			"baseline":  baseline,
			"threshold": anomaliesFlagThreshold,
			"anomalies": anomalies,
		})
	}

	renderAnomalies(anomalies, project, *baseline, anomaliesFlagThreshold)
	return nil
}

func renderAnomalies(anomalies []store.AnomalyResult, project string, baseline store.ProjectBaseline, threshold float64) {
	fmt.Println(output.Section(fmt.Sprintf("Anomalies — %s", project)))
	fmt.Println()

	fmt.Printf(" %s  threshold: %.1f σ  baseline: %d sessions (avg cost $%.3f, avg friction %.1f)\n\n",
		output.StyleMuted.Render(fmt.Sprintf("Project: %s", project)),
		threshold,
		baseline.SessionCount,
		baseline.AvgCostUSD,
		baseline.AvgFriction,
	)

	if len(anomalies) == 0 {
		fmt.Printf(" %s\n\n", output.StyleSuccess.Render("No anomalies detected."))
		return
	}

	tbl := output.NewTable("Session", "Start", "Cost", "Friction", "Cost Z", "Friction Z", "Severity")

	for _, a := range anomalies {
		sessionShort := a.SessionID
		if len(sessionShort) > 12 {
			sessionShort = sessionShort[:12]
		}

		start := a.StartTime
		if len(start) > 16 {
			start = start[:16]
		}

		severityStyled := a.Severity
		switch a.Severity {
		case "critical":
			severityStyled = output.StyleWarning.Render(a.Severity)
		case "warning":
			severityStyled = output.StyleMuted.Render(a.Severity)
		}

		tbl.AddRow(
			sessionShort,
			start,
			fmt.Sprintf("$%.3f", a.CostUSD),
			fmt.Sprintf("%d", a.Friction),
			fmt.Sprintf("%.2f", a.CostZScore),
			fmt.Sprintf("%.2f", a.FrictionZScore),
			severityStyled,
		)
	}

	tbl.Print()
	fmt.Println()
	fmt.Printf(" %s\n", output.StyleBold.Render(fmt.Sprintf("%d anomalous session(s) detected", len(anomalies))))
	fmt.Println()
	fmt.Printf(" %s\n", output.StyleMuted.Render("Use --threshold to adjust sensitivity, --json for machine output"))
	fmt.Println()
}
