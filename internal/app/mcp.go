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
query during a session. The server exposes 26 MCP tools across 6 categories:

  • Session & cost tracking (get_session_stats, get_cost_budget, get_cost_summary, etc.)
  • Live self-reflection (get_session_dashboard, get_drift_signal, get_token_velocity, etc.)
  • Project & pattern analysis (get_project_health, get_suggestions, get_stale_patterns, etc.)
  • Agent & workflow analytics (get_agent_performance, get_saw_sessions, get_cost_attribution, etc.)
  • Multi-project analysis (get_session_projects for weighted repo attribution)
  • Factor analysis (get_causal_insights to correlate attributes with outcomes)

See docs/mcp.md for complete tool reference.

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
