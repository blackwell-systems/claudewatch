package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/spf13/cobra"
)

var (
	logSession string
	logProject string
	logTags    []string
	logNote    string
	logList    bool
	logMetric  string
	logDays    int
	logJSON    bool
)

var logCmd = &cobra.Command{
	Use:   "log [metric_name] [value]",
	Short: "Inject custom user-defined metrics",
	Long: `Log custom metrics to the claudewatch database for tracking over time.
Metric types (scale, boolean, counter, duration) are defined in config.yaml
under custom_metrics.

Examples:
  claudewatch log session_quality 4
  claudewatch log session_quality 4 --session latest
  claudewatch log resume_callback true --project rezmakr
  claudewatch log time_to_first_commit 12m --session latest
  claudewatch log scope_creep false --session latest
  claudewatch log --list
  claudewatch log --list --metric session_quality --days 30`,
	Args: cobra.ArbitraryArgs,
	RunE: runLog,
}

func init() {
	logCmd.Flags().StringVar(&logSession, "session", "", "Session ID to associate (use 'latest' for most recent)")
	logCmd.Flags().StringVar(&logProject, "project", "", "Project name to associate")
	logCmd.Flags().StringSliceVar(&logTags, "tag", nil, "Tags for filtering (can specify multiple)")
	logCmd.Flags().StringVar(&logNote, "note", "", "Optional note")
	logCmd.Flags().BoolVar(&logList, "list", false, "List logged metrics")
	logCmd.Flags().StringVar(&logMetric, "metric", "", "Filter --list by metric name")
	logCmd.Flags().IntVar(&logDays, "days", 0, "Filter --list to last N days")
	logCmd.Flags().BoolVar(&logJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	db, err := store.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if logList {
		return runLogList(db, cfg)
	}

	// Logging mode: requires metric_name and value.
	if len(args) < 2 {
		return fmt.Errorf("usage: claudewatch log <metric_name> <value> [flags]\nUse --list to view logged metrics")
	}

	metricName := args[0]
	rawValue := args[1]

	// Validate metric is defined in config.
	metricDef, ok := cfg.CustomMetrics[metricName]
	if !ok {
		available := make([]string, 0, len(cfg.CustomMetrics))
		for name := range cfg.CustomMetrics {
			available = append(available, name)
		}
		return fmt.Errorf("unknown metric %q; defined metrics: %s", metricName, strings.Join(available, ", "))
	}

	// Parse value based on metric type.
	value, err := parseMetricValue(rawValue, metricDef)
	if err != nil {
		return fmt.Errorf("parsing value for %s (%s): %w", metricName, metricDef.Type, err)
	}

	// Resolve session ID.
	sessionID := ""
	if logSession != "" {
		sessionID, err = resolveSessionID(logSession, cfg.ClaudeHome)
		if err != nil {
			return fmt.Errorf("resolving session: %w", err)
		}
	}

	// Encode tags as JSON array.
	tagsJSON := ""
	if len(logTags) > 0 {
		tagBytes, err := json.Marshal(logTags)
		if err != nil {
			return fmt.Errorf("encoding tags: %w", err)
		}
		tagsJSON = string(tagBytes)
	}

	// Insert the custom metric.
	cm := &store.CustomMetricRow{
		LoggedAt:    time.Now().UTC().Format(time.RFC3339),
		SessionID:   sessionID,
		Project:     logProject,
		MetricName:  metricName,
		MetricValue: value,
		Tags:        tagsJSON,
		Note:        logNote,
	}

	if err := db.InsertCustomMetric(cm); err != nil {
		return fmt.Errorf("inserting custom metric: %w", err)
	}

	fmt.Printf("Logged %s = %s", metricName, formatMetricValue(value, metricDef))
	if sessionID != "" {
		fmt.Printf(" (session: %s)", truncateID(sessionID))
	}
	if logProject != "" {
		fmt.Printf(" (project: %s)", logProject)
	}
	fmt.Println()

	return nil
}

// parseMetricValue converts a raw string value to a float64 based on the metric type.
func parseMetricValue(raw string, def config.MetricDefinition) (float64, error) {
	switch def.Type {
	case "scale":
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, fmt.Errorf("expected a number: %w", err)
		}
		if v < def.Range[0] || v > def.Range[1] {
			return 0, fmt.Errorf("value %.1f is outside range [%.0f, %.0f]", v, def.Range[0], def.Range[1])
		}
		return v, nil

	case "boolean":
		return parseBooleanValue(raw)

	case "counter":
		// Counter accepts +N or -N for incremental changes, or raw numbers.
		return strconv.ParseFloat(raw, 64)

	case "duration":
		return parseDurationValue(raw)

	default:
		// Fall back to plain numeric parsing.
		return strconv.ParseFloat(raw, 64)
	}
}

// parseBooleanValue converts boolean-like strings to 1.0 or 0.0.
func parseBooleanValue(raw string) (float64, error) {
	switch strings.ToLower(raw) {
	case "true", "yes", "1":
		return 1.0, nil
	case "false", "no", "0":
		return 0.0, nil
	default:
		return 0, fmt.Errorf("expected true/false/yes/no/1/0, got %q", raw)
	}
}

// parseDurationValue converts duration strings like "12m", "30s", "1h" to seconds.
func parseDurationValue(raw string) (float64, error) {
	if len(raw) < 2 {
		// Try plain number (seconds).
		return strconv.ParseFloat(raw, 64)
	}

	suffix := raw[len(raw)-1]
	numStr := raw[:len(raw)-1]

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		// Not a suffixed duration; try plain number.
		return strconv.ParseFloat(raw, 64)
	}

	switch suffix {
	case 's', 'S':
		return num, nil
	case 'm', 'M':
		return num * 60, nil
	case 'h', 'H':
		return num * 3600, nil
	default:
		// Suffix is a digit; try the whole string as a plain number.
		return strconv.ParseFloat(raw, 64)
	}
}

// resolveSessionID resolves the --session flag value.
// "latest" resolves to the most recent session from history.jsonl.
func resolveSessionID(session, claudeHome string) (string, error) {
	if strings.ToLower(session) == "latest" {
		id, err := claude.LatestSessionID(claudeHome)
		if err != nil {
			return "", fmt.Errorf("finding latest session: %w", err)
		}
		if id == "" {
			return "", fmt.Errorf("no sessions found in history")
		}
		return id, nil
	}
	return session, nil
}

// runLogList queries and displays logged custom metrics.
func runLogList(db *store.DB, cfg *config.Config) error {
	rows, err := queryCustomMetrics(db, logMetric, logDays)
	if err != nil {
		return fmt.Errorf("querying custom metrics: %w", err)
	}

	if logJSON || flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	if len(rows) == 0 {
		fmt.Println("No custom metrics logged yet. Use 'claudewatch log <metric> <value>' to start.")
		return nil
	}

	fmt.Println(output.Section("Custom Metrics Log"))
	fmt.Println()

	tbl := output.NewTable("Time", "Metric", "Value", "Session", "Project", "Note")
	for _, r := range rows {
		// Format the value using the metric definition if available.
		valueStr := fmt.Sprintf("%.2f", r.MetricValue)
		if def, ok := cfg.CustomMetrics[r.MetricName]; ok {
			valueStr = formatMetricValue(r.MetricValue, def)
		}

		sessionStr := ""
		if r.SessionID != "" {
			sessionStr = truncateID(r.SessionID)
		}

		timeStr := ""
		if t, err := time.Parse(time.RFC3339, r.LoggedAt); err == nil {
			timeStr = t.Local().Format("2006-01-02 15:04")
		} else {
			timeStr = r.LoggedAt
		}

		tbl.AddRow(timeStr, r.MetricName, valueStr, sessionStr, r.Project, r.Note)
	}
	tbl.Print()

	return nil
}

// queryCustomMetrics queries the custom_metrics table with optional filters.
func queryCustomMetrics(db *store.DB, metricFilter string, days int) ([]store.CustomMetricRow, error) {
	query := "SELECT id, logged_at, session_id, project, metric_name, metric_value, tags, note FROM custom_metrics WHERE 1=1"
	var queryArgs []any

	if metricFilter != "" {
		query += " AND metric_name = ?"
		queryArgs = append(queryArgs, metricFilter)
	}

	if days > 0 {
		cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)
		query += " AND logged_at >= ?"
		queryArgs = append(queryArgs, cutoff)
	}

	query += " ORDER BY logged_at DESC"

	rows, err := db.Conn().Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []store.CustomMetricRow
	for rows.Next() {
		var r store.CustomMetricRow
		var sessionID, project, tags, note nullString
		if err := rows.Scan(&r.ID, &r.LoggedAt, &sessionID, &project, &r.MetricName, &r.MetricValue, &tags, &note); err != nil {
			return nil, err
		}
		r.SessionID = string(sessionID)
		r.Project = string(project)
		r.Tags = string(tags)
		r.Note = string(note)
		results = append(results, r)
	}
	return results, rows.Err()
}

// nullString is a helper type that implements sql.Scanner for nullable TEXT columns.
type nullString string

func (ns *nullString) Scan(value any) error {
	if value == nil {
		*ns = ""
		return nil
	}
	switch v := value.(type) {
	case string:
		*ns = nullString(v)
	case []byte:
		*ns = nullString(v)
	default:
		*ns = nullString(fmt.Sprintf("%v", v))
	}
	return nil
}

// formatMetricValue formats a float64 value for display based on its metric type.
func formatMetricValue(value float64, def config.MetricDefinition) string {
	switch def.Type {
	case "boolean":
		if value >= 1.0 {
			return "true"
		}
		return "false"
	case "duration":
		return formatDuration(value)
	case "scale":
		if value == float64(int(value)) {
			return fmt.Sprintf("%.0f", value)
		}
		return fmt.Sprintf("%.1f", value)
	default:
		return fmt.Sprintf("%.1f", value)
	}
}

// formatDuration converts seconds to a human-readable duration string.
func formatDuration(seconds float64) string {
	if seconds >= 3600 {
		return fmt.Sprintf("%.1fh", seconds/3600)
	}
	if seconds >= 60 {
		return fmt.Sprintf("%.0fm", seconds/60)
	}
	return fmt.Sprintf("%.0fs", seconds)
}

// truncateID shortens a UUID for display.
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
