package app

import (
	"fmt"
	"os"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/spf13/cobra"
)

const hookThreshold = 3

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Check for consecutive tool errors (for use as a postToolCall shell hook)",
	Long: `Tail-scans the active Claude Code session for consecutive tool errors.
Exit 0 if below threshold (silent). Exit 2 if threshold exceeded, with one
actionable line printed to stderr.

Intended for use as a Claude Code postToolCall shell hook:
  {"postToolCall": {"command": "claudewatch hook"}}`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run:           runHook,
}

func init() {
	rootCmd.AddCommand(hookCmd)
}

func runHook(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return
	}

	activePath, err := claude.FindActiveSessionPath(cfg.ClaudeHome)
	if err != nil || activePath == "" {
		return
	}

	n, err := claude.ParseLiveConsecutiveErrors(activePath, 50)
	if err != nil || n < hookThreshold {
		return
	}

	fmt.Fprintf(os.Stderr, "⚠ %d consecutive tool errors detected. Stop and diagnose: call get_session_dashboard (claudewatch MCP) to check token velocity, friction patterns, and context pressure before continuing.\n", n)
	os.Exit(2)
}
