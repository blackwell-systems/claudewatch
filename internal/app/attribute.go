package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/analyzer"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/blackwell-systems/claudewatch/internal/ui"
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

	sessionID := attrFlagSession

	// If no session specified, check for multiple active sessions
	if sessionID == "" {
		activeSessions, err := store.FindActiveSessions(cfg.ClaudeHome, 15*time.Minute)
		if err != nil {
			return fmt.Errorf("finding active sessions: %w", err)
		}

		if len(activeSessions) > 1 {
			// Multiple active sessions - prompt user to select
			if !ui.IsTTY() {
				return fmt.Errorf("multiple active sessions found (use --session to specify):\n%s",
					formatSessionList(activeSessions))
			}

			selectedID, err := ui.SelectSession(activeSessions)
			if err != nil {
				if errors.Is(err, ui.ErrCancelled) {
					return fmt.Errorf("selection cancelled")
				}
				return fmt.Errorf("session selection: %w", err)
			}
			sessionID = selectedID
		}
		// If 0 or 1 active sessions, let ComputeAttribution use existing logic
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

// formatSessionList formats active sessions for non-TTY error message
func formatSessionList(sessions []store.ActiveSession) string {
	var sb strings.Builder
	for _, s := range sessions {
		sb.WriteString(fmt.Sprintf("  - %s (%s)\n", s.SessionID[:12], s.ProjectName))
	}
	return sb.String()
}
