package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
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
	Use:   "sessions [session-id]",
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
  claudewatch sessions --days 7 --limit 5       # last 7 days, top 5
  claudewatch sessions abc12345                 # inspect a single session by ID prefix`,
	Args: cobra.MaximumNArgs(1),
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
	Meta          claude.SessionMeta   `json:"meta"`
	Facet         *claude.SessionFacet `json:"facet,omitempty"`
	EstimatedCost float64              `json:"estimated_cost"`
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

func runSessions(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	// Load stats-cache once for accurate cost estimation (non-fatal).
	pricing := analyzer.DefaultPricing["sonnet"]
	cacheRatio := analyzer.NoCacheRatio()
	if sc, scErr := claude.ParseStatsCache(cfg.ClaudeHome); scErr == nil && sc != nil {
		cacheRatio = analyzer.ComputeCacheRatio(*sc)
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

	// --inspect mode: a positional session-id argument was provided.
	if len(args) == 1 {
		return runInspect(args[0], sessions, facetMap, pricing, cacheRatio)
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

		row := sessionRow{
			Meta:          s,
			Facet:         facetMap[s.SessionID],
			EstimatedCost: analyzer.EstimateSessionCost(s, pricing, cacheRatio),
		}

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
			return rows[i].EstimatedCost > rows[j].EstimatedCost
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

// runInspect finds a session by full ID or prefix and renders a detailed view.
func runInspect(prefix string, sessions []claude.SessionMeta, facetMap map[string]*claude.SessionFacet, pricing analyzer.ModelPricing, cacheRatio analyzer.CacheRatio) error {
	var matched *claude.SessionMeta
	for i := range sessions {
		s := &sessions[i]
		if s.SessionID == prefix || strings.HasPrefix(s.SessionID, prefix) {
			if matched != nil {
				return fmt.Errorf("ambiguous session prefix %q — matches multiple sessions; use more characters", prefix)
			}
			matched = s
		}
	}
	if matched == nil {
		return fmt.Errorf("no session found matching %q", prefix)
	}

	row := sessionRow{
		Meta:          *matched,
		Facet:         facetMap[matched.SessionID],
		EstimatedCost: analyzer.EstimateSessionCost(*matched, pricing, cacheRatio),
	}

	if flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(row)
	}

	renderInspect(row)
	return nil
}

// renderInspect prints a detailed single-session view.
func renderInspect(r sessionRow) {
	fmt.Println(output.Section("Session Inspect"))
	fmt.Println()

	label := func(l, v string) {
		fmt.Printf(" %s  %s\n", output.StyleLabel.Render(l), output.StyleBold.Render(v))
	}
	muted := func(l, v string) {
		fmt.Printf(" %s  %s\n", output.StyleLabel.Render(l), output.StyleMuted.Render(v))
	}

	// Identity
	label("Session ID", r.Meta.SessionID)
	label("Project", r.projectName())
	label("Project Path", r.Meta.ProjectPath)

	date := r.Meta.StartTime
	if t, err := time.Parse(time.RFC3339, r.Meta.StartTime); err == nil {
		date = t.Format("2006-01-02 15:04:05")
	}
	label("Date", date)
	label("Duration", fmt.Sprintf("%d min", r.Meta.DurationMinutes))

	fmt.Println()

	// Messages
	fmt.Println(output.Section("Messages"))
	fmt.Println()
	muted("User messages", fmt.Sprintf("%d", r.Meta.UserMessageCount))
	muted("Assistant messages", fmt.Sprintf("%d", r.Meta.AssistantMessageCount))

	fmt.Println()

	// Tokens & Cost
	fmt.Println(output.Section("Tokens & Cost"))
	fmt.Println()
	muted("Input tokens", fmt.Sprintf("%d", r.Meta.InputTokens))
	muted("Output tokens", fmt.Sprintf("%d", r.Meta.OutputTokens))
	label("Estimated cost", fmt.Sprintf("$%.4f", r.EstimatedCost))

	fmt.Println()

	// Git
	fmt.Println(output.Section("Git"))
	fmt.Println()
	muted("Commits", fmt.Sprintf("%d", r.Meta.GitCommits))
	muted("Pushes", fmt.Sprintf("%d", r.Meta.GitPushes))
	muted("Lines added", fmt.Sprintf("%d", r.Meta.LinesAdded))
	muted("Lines removed", fmt.Sprintf("%d", r.Meta.LinesRemoved))
	muted("Files modified", fmt.Sprintf("%d", r.Meta.FilesModified))

	fmt.Println()

	// Tools — top 5 by usage count
	fmt.Println(output.Section("Tools (top 5)"))
	fmt.Println()
	if len(r.Meta.ToolCounts) == 0 {
		fmt.Printf(" %s\n", output.StyleMuted.Render("No tool usage recorded"))
	} else {
		type toolEntry struct {
			name  string
			count int
		}
		var tools []toolEntry
		for name, count := range r.Meta.ToolCounts {
			tools = append(tools, toolEntry{name, count})
		}
		sort.Slice(tools, func(i, j int) bool {
			if tools[i].count != tools[j].count {
				return tools[i].count > tools[j].count
			}
			return tools[i].name < tools[j].name
		})
		limit := 5
		if len(tools) < limit {
			limit = len(tools)
		}
		for _, t := range tools[:limit] {
			muted(t.name, fmt.Sprintf("%d", t.count))
		}
	}

	fmt.Println()

	// Friction (from facet)
	fmt.Println(output.Section("Friction"))
	fmt.Println()
	if r.Facet == nil || len(r.Facet.FrictionCounts) == 0 {
		fmt.Printf(" %s\n", output.StyleMuted.Render("No friction data recorded"))
	} else {
		for frictionType, count := range r.Facet.FrictionCounts {
			line := fmt.Sprintf("%d", count)
			if count > 2 {
				line = output.StyleWarning.Render(line)
			}
			fmt.Printf(" %s  %s\n", output.StyleLabel.Render(frictionType), line)
		}
	}

	fmt.Println()

	// Outcome & satisfaction (from facet)
	fmt.Println(output.Section("Outcome & Satisfaction"))
	fmt.Println()
	if r.Facet == nil {
		fmt.Printf(" %s\n", output.StyleMuted.Render("No facet data recorded"))
	} else {
		outcomeStyled := r.Facet.Outcome
		switch r.Facet.Outcome {
		case "achieved":
			outcomeStyled = output.StyleSuccess.Render(r.Facet.Outcome)
		case "not_achieved":
			outcomeStyled = output.StyleWarning.Render(r.Facet.Outcome)
		}

		fmt.Printf(" %s  %s\n", output.StyleLabel.Render("Outcome"), outcomeStyled)
		muted("Claude helpfulness", r.Facet.ClaudeHelpfulness)
		muted("Goal", r.Facet.UnderlyingGoal)
		muted("Summary", r.Facet.BriefSummary)
	}

	fmt.Println()

	// First prompt (truncated to 200 chars)
	fmt.Println(output.Section("First Prompt"))
	fmt.Println()
	prompt := r.Meta.FirstPrompt
	if len(prompt) == 0 {
		fmt.Printf(" %s\n", output.StyleMuted.Render("(none recorded)"))
	} else {
		if len(prompt) > 200 {
			prompt = prompt[:200] + "…"
		}
		fmt.Printf(" %s\n", output.StyleMuted.Render(prompt))
	}

	fmt.Println()
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

		cost := fmt.Sprintf("$%.2f", r.EstimatedCost)
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

	// Summary stats footer.
	var totalCost float64
	var totalFriction int
	var totalCommits int
	var totalDuration int

	for _, r := range rows {
		totalCost += r.EstimatedCost
		totalFriction += r.frictionTotal()
		totalCommits += r.Meta.GitCommits
		totalDuration += r.Meta.DurationMinutes
	}

	n := len(rows)
	avgFriction := 0.0
	avgDuration := 0.0
	if n > 0 {
		avgFriction = float64(totalFriction) / float64(n)
		avgDuration = float64(totalDuration) / float64(n)
	}

	fmt.Println()
	fmt.Printf(" %s\n", output.StyleBold.Render(fmt.Sprintf(
		"Totals: $%.2f cost · %d commits · %.1f avg friction · %.0fm avg duration",
		totalCost, totalCommits, avgFriction, avgDuration,
	)))
	fmt.Println()
	fmt.Printf(" %s\n", output.StyleMuted.Render("Use --sort friction|cost|duration|commits to reorder"))
	fmt.Printf(" %s\n", output.StyleMuted.Render("Use --project <name> to filter, --json for machine output"))
	fmt.Printf(" %s\n", output.StyleMuted.Render("Use claudewatch sessions <session-id> to inspect a session"))
}
