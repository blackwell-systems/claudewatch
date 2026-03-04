package app

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/spf13/cobra"
)

var attrFlagSession string

var attributeCmd = &cobra.Command{
	Use:   "attribute",
	Short: "Break down token cost by tool type for a session",
	Long: `Show which tool types consumed most tokens and budget in a session.
Defaults to the most recent session.

Examples:
  claudewatch attribute
  claudewatch attribute --session abc123
  claudewatch attribute --json`,
	RunE: runAttribute,
}

func init() {
	attributeCmd.Flags().StringVar(&attrFlagSession, "session", "", "Session ID to analyze (default: most recent)")
	rootCmd.AddCommand(attributeCmd)
}

func runAttribute(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	ap := analyzer.DefaultPricing["sonnet"]
	pricing := store.ModelPricing{
		InputPerMillion:      ap.InputPerMillion,
		OutputPerMillion:     ap.OutputPerMillion,
		CacheReadPerMillion:  ap.CacheReadPerMillion,
		CacheWritePerMillion: ap.CacheWritePerMillion,
	}

	// Select session: use flag if provided, otherwise prompt for active sessions
	sessionID, err := SelectSession(cfg, attrFlagSession, WithMostRecentFallback())
	if err != nil {
		return err
	}

	rows, err, selectedSessionID := store.ComputeAttribution(sessionID, cfg.ClaudeHome, pricing)
	if err != nil {
		return fmt.Errorf("computing attribution: %w", err)
	}

	if flagJSON {
		return json.NewEncoder(os.Stdout).Encode(rows)
	}

	fmt.Println(output.Section("Cost Attribution"))
	if sessionID == "" {
		// Show which session was selected when using default (most recent)
		fmt.Printf(" Session: %s (most recent)\n", output.StyleMuted.Render(selectedSessionID[:12]+"..."))
	}
	fmt.Println()

	tbl := output.NewTable("Tool Type", "Calls", "Input Tokens", "Output Tokens", "Est. Cost")

	var total float64
	for _, row := range rows {
		tbl.AddRow(
			row.ToolType,
			fmt.Sprintf("%d", row.Calls),
			fmt.Sprintf("%d", row.InputTokens),
			fmt.Sprintf("%d", row.OutputTokens),
			fmt.Sprintf("$%.4f", row.EstCostUSD),
		)
		total += row.EstCostUSD
	}

	tbl.Print()
	fmt.Println()
	fmt.Printf("Total: $%.4f\n", total)
	fmt.Println()

	return nil
}
