// Package app contains the Cobra command tree for claudewatch.
package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var appVersion = "dev"

// SetVersion sets the application version (called from main with ldflags value).
func SetVersion(v string) {
	appVersion = v
	rootCmd.Version = v
}

var (
	flagNoColor bool
	flagJSON    bool
	flagVerbose bool
	flagConfig  string
)

var rootCmd = &cobra.Command{
	Use:   "claudewatch",
	Short: "Observability for AI-assisted development workflows",
	Long: `claudewatch provides observability and continuous improvement for
Claude Code workflows. It reads local Claude data, analyzes session patterns,
scores project readiness, surfaces friction, and tracks improvement over time.

Run 'claudewatch' with no arguments to see a quick dashboard summary.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("claudewatch", appVersion)
		fmt.Println()
		fmt.Println("Dashboard coming soon. Use a subcommand:")
		fmt.Println("  scan      Inventory projects and score AI readiness")
		fmt.Println("  metrics   Parse session data and display trends")
		fmt.Println("  gaps      Surface friction patterns and missing config")
		fmt.Println("  suggest   Generate ranked improvement recommendations")
		fmt.Println("  fix       Generate CLAUDE.md improvements from session data")
		fmt.Println("  track     Snapshot and compare metrics over time")
		fmt.Println("  log       Inject custom user-defined metrics")
		fmt.Println("  watch     Monitor session data and alert on friction spikes")
		return nil
	},
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "Config file path (default: ~/.config/claudewatch/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output as JSON")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "Enable verbose output")
}
