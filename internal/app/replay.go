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

var (
	replayFlagFrom    int
	replayFlagTo      int
	replayFlagSession string
)

var replayCmd = &cobra.Command{
	Use:   "replay [session-id]",
	Short: "Walk through a session as a structured timeline",
	Long: `Show every turn in a session with role, tool, token usage, cost, and
friction markers. Useful for post-mortems on expensive or high-friction sessions.

Examples:
  claudewatch replay                           # select from active sessions
  claudewatch replay abc123def456              # specific session
  claudewatch replay abc123 --from 10 --to 20  # range of turns
  claudewatch replay --json                    # JSON output`,
	Args: cobra.MaximumNArgs(1),
	RunE: runReplay,
}

func init() {
	replayCmd.Flags().IntVar(&replayFlagFrom, "from", 0, "First turn to show (1-indexed, default: all)")
	replayCmd.Flags().IntVar(&replayFlagTo, "to", 0, "Last turn to show (1-indexed, default: all)")
	replayCmd.Flags().StringVar(&replayFlagSession, "session", "", "Session ID to replay (default: select from active)")
	rootCmd.AddCommand(replayCmd)
}

func runReplay(cmd *cobra.Command, args []string) error {
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

	// Session ID can come from positional arg or --session flag
	flagOrArg := replayFlagSession
	if len(args) > 0 {
		flagOrArg = args[0]
	}

	// Select session: use flag/arg if provided, otherwise prompt for active sessions
	sessionID, err := SelectSession(cfg, flagOrArg, WithMostRecentFallback())
	if err != nil {
		return err
	}

	replay, err := store.BuildReplay(sessionID, cfg.ClaudeHome, replayFlagFrom, replayFlagTo, pricing)
	if err != nil {
		return fmt.Errorf("building replay: %w", err)
	}

	if flagJSON {
		return json.NewEncoder(os.Stdout).Encode(replay)
	}

	fmt.Println(output.Section(fmt.Sprintf("Session Replay — %s", sessionID[:min(12, len(sessionID))])))
	fmt.Println()
	fmt.Printf(" %d turns | $%.4f total | %d friction events\n\n",
		replay.TotalTurns, replay.TotalCostUSD, replay.FrictionCount)

	tbl := output.NewTable("Turn", "Role", "Tool", "In Tok", "Out Tok", "Cost", "F")

	for _, t := range replay.Turns {
		toolName := t.ToolName
		if len(toolName) > 20 {
			toolName = toolName[:20]
		}

		frictionMark := ""
		if t.Friction {
			frictionMark = "!"
		}

		tbl.AddRow(
			fmt.Sprintf("%d", t.Turn),
			t.Role,
			toolName,
			fmt.Sprintf("%d", t.InputTokens),
			fmt.Sprintf("%d", t.OutputTokens),
			fmt.Sprintf("$%.4f", t.EstCostUSD),
			frictionMark,
		)
	}

	tbl.Print()
	fmt.Println()

	return nil
}
