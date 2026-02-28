package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/spf13/cobra"
)

var (
	sessionsFlagSort    string
	sessionsFlagProject string
	sessionsFlagDays    int
	sessionsFlagLimit   int
	sessionsFlagWorst   bool
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List, filter, and inspect individual sessions",
	Long: `Browse individual Claude Code sessions sorted by various criteria.
Useful for finding your worst sessions, drilling into friction, or
understanding where time and tokens went.

Examples:
  claudewatch sessions                          # recent sessions
  claudewatch sessions --sort friction          # most friction first
  claudewatch sessions --sort cost              # most expensive first
  claudewatch sessions --worst                  # shortcut for --sort friction
  claudewatch sessions --project claudewatch    # filter by project name
  claudewatch sessions --days 7 --limit 5       # last 7 days, top 5`,
	RunE: runSessions,
}

func init() {
	sessionsCmd.Flags().StringVar(&sessionsFlagSort, "sort", "recent", "Sort by: recent, friction, cost, duration, commits")
	sessionsCmd.Flags().StringVar(&sessionsFlagProject, "project", "", "Filter to sessions matching project name or path")
	sessionsCmd.Flags().IntVar(&sessionsFlagDays, "days", 30, "Number of days to look back")
	sessionsCmd.Flags().IntVar(&sessionsFlagLimit, "limit", 15, "Maximum sessions to display")
	sessionsCmd.Flags().BoolVar(&sessionsFlagWorst, "worst", false, "Shortcut for --sort friction")
	rootCmd.AddCommand(sessionsCmd)
}

// sessionRow combines meta and facet data for a single session.
type sessionRow struct {
	Meta  claude.SessionMeta   `json:"meta"`
	Facet *claude.SessionFacet `json:"facet,omitempty"`
}

func (s sessionRow) projectName() string {
	return filepath.Base(s.Meta.ProjectPath)
}

func (s sessionRow) frictionTotal() int {
	if s.Facet == nil {
		return 0
	}
	total := 0
	for _, c := range s.Facet.FrictionCounts {
		total += c
	}
	return total
}

func (s sessionRow) estimatedCost() float64 {
	// Rough estimate using uncached token pricing.
	inputCost := float64(s.Meta.InputTokens) / 1_000_000 * 3.0    // $3/MTok input
	outputCost := float64(s.Meta.OutputTokens) / 1_000_000 * 15.0 // $15/MTok output
	return inputCost + outputCost
}

func runSessions(cmd *cobra.Command, args []string) error {
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

	facets, err := claude.ParseAllFacets(cfg.ClaudeHome)
	if err != nil {
		return fmt.Errorf("parsing facets: %w", err)
	}

	// Index facets by session ID.
	facetMap := make(map[string]*claude.SessionFacet, len(facets))
	for i := range facets {
		facetMap[facets[i].SessionID] = &facets[i]
	}

	// Build combined rows.
	var rows []sessionRow
	cutoff := time.Now().AddDate(0, 0, -sessionsFlagDays)

	for _, s := range sessions {
		t, err := time.Parse(time.RFC3339, s.StartTime)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			continue
		}

		row := sessionRow{Meta: s, Facet: facetMap[s.SessionID]}

		// Project filter.
		if sessionsFlagProject != "" {
			name := filepath.Base(s.ProjectPath)
			if !strings.Contains(strings.ToLower(name), strings.ToLower(sessionsFlagProject)) &&
				!strings.Contains(strings.ToLower(s.ProjectPath), strings.ToLower(sessionsFlagProject)) {
				continue
			}
		}

		rows = append(rows, row)
	}

	if len(rows) == 0 {
		fmt.Println(" No sessions found matching filters.")
		return nil
	}

	// Sort.
	sortKey := sessionsFlagSort
	if sessionsFlagWorst {
		sortKey = "friction"
	}

	switch sortKey {
	case "friction":
		sort.Slice(rows, func(i, j int) bool {
			fi, fj := rows[i].frictionTotal(), rows[j].frictionTotal()
			if fi != fj {
				return fi > fj
			}
			return rows[i].Meta.ToolErrors > rows[j].Meta.ToolErrors
		})
	case "cost":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].estimatedCost() > rows[j].estimatedCost()
		})
	case "duration":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Meta.DurationMinutes > rows[j].Meta.DurationMinutes
		})
	case "commits":
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Meta.GitCommits > rows[j].Meta.GitCommits
		})
	default: // "recent"
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Meta.StartTime > rows[j].Meta.StartTime
		})
	}

	// Limit.
	if sessionsFlagLimit > 0 && len(rows) > sessionsFlagLimit {
		rows = rows[:sessionsFlagLimit]
	}

	// JSON output.
	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	renderSessions(rows, sortKey)
	return nil
}

func renderSessions(rows []sessionRow, sortKey string) {
	fmt.Println(output.Section("Sessions"))
	fmt.Println()
	fmt.Printf(" %s  sorted by %s\n\n",
		output.StyleMuted.Render(fmt.Sprintf("%d sessions", len(rows))),
		output.StyleBold.Render(sortKey))

	tbl := output.NewTable("Date", "Project", "Duration", "Messages", "Commits", "Friction", "Errors", "Cost", "Outcome")

	for _, r := range rows {
		date := ""
		if t, err := time.Parse(time.RFC3339, r.Meta.StartTime); err == nil {
			date = t.Format("Jan 02 15:04")
		}

		outcome := ""
		if r.Facet != nil {
			outcome = r.Facet.Outcome
		}

		cost := fmt.Sprintf("$%.2f", r.estimatedCost())
		friction := fmt.Sprintf("%d", r.frictionTotal())
		errors := fmt.Sprintf("%d", r.Meta.ToolErrors)

		// Color high-friction/error cells.
		if r.frictionTotal() > 3 {
			friction = output.StyleWarning.Render(friction)
		}
		if r.Meta.ToolErrors > 5 {
			errors = output.StyleWarning.Render(errors)
		}

		outcomeStyled := outcome
		switch outcome {
		case "achieved":
			outcomeStyled = output.StyleSuccess.Render(outcome)
		case "not_achieved":
			outcomeStyled = output.StyleWarning.Render(outcome)
		}

		tbl.AddRow(
			date,
			r.projectName(),
			fmt.Sprintf("%dm", r.Meta.DurationMinutes),
			fmt.Sprintf("%d", r.Meta.UserMessageCount),
			fmt.Sprintf("%d", r.Meta.GitCommits),
			friction,
			errors,
			cost,
			outcomeStyled,
		)
	}

	tbl.Print()

	// Summary line.
	fmt.Println()
	fmt.Printf(" %s\n", output.StyleMuted.Render("Use --sort friction|cost|duration|commits to reorder"))
	fmt.Printf(" %s\n", output.StyleMuted.Render("Use --project <name> to filter, --json for machine output"))
}
