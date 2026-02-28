package app

import (
	"fmt"
	"os"

	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/mcp"
	"github.com/spf13/cobra"
)

var mcpBudget float64

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run an MCP stdio server for use with Claude Code",
	Long: `Start a Model Context Protocol stdio server that Claude Code can
query during a session. The server exposes three tools:

  get_session_stats   Token usage, cost, and duration for the current session
  get_cost_budget     Today's spend vs daily budget
  get_recent_sessions Last N sessions with cost, friction, and project name

Add to your Claude Code MCP configuration (~/.claude/settings.json):
  {"mcpServers":{"claudewatch":{"command":"claudewatch","args":["mcp"]}}}`,
	RunE: runMCP,
}

func init() {
	mcpCmd.Flags().Float64Var(&mcpBudget, "budget", 0, "Daily cost budget in USD for budget reporting (e.g. --budget 20)")
	rootCmd.AddCommand(mcpCmd)
}

func runMCP(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	srv := mcp.NewServer(cfg, mcpBudget)
	return srv.Run(cmd.Context(), os.Stdin, os.Stdout)
}
