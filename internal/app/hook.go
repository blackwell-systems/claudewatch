package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/spf13/cobra"
)

const (
	hookThreshold       = 3
	hookCooldownSeconds = 30
	// Sonnet pricing ($/million tokens) — matches mcp/cost_velocity_tools.go
	hookInputPerMillion  = 3.0
	hookOutputPerMillion = 15.0
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Check session health (for use as a PostToolUse shell hook)",
	Long: `Checks the active Claude Code session for warning conditions in priority order:
  1. Consecutive tool errors >= 3
  2. Context pressure at "pressure" or "critical"
  3. Cost velocity "burning"

Exit 0 if all clear (silent). Exit 2 if a threshold is exceeded, with one
actionable line printed to stderr naming the get_session_dashboard MCP tool.

Rate-limited to one check per 30 seconds to minimize overhead.

Intended for use as a Claude Code PostToolUse shell hook.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run:           runHook,
}

func init() {
	rootCmd.AddCommand(hookCmd)
}

func runHook(cmd *cobra.Command, args []string) {
	// Rate limiter: skip if within cooldown window.
	stampFile := os.ExpandEnv("$HOME/.cache/claudewatch-hook.ts")
	now := time.Now().Unix()
	if data, err := os.ReadFile(stampFile); err == nil {
		if last, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
			if now-last < hookCooldownSeconds {
				return
			}
		}
	}
	_ = os.MkdirAll(os.ExpandEnv("$HOME/.cache"), 0o755)
	_ = os.WriteFile(stampFile, []byte(strconv.FormatInt(now, 10)), 0o644)

	cfg, err := config.Load(flagConfig)
	if err != nil {
		return
	}

	activePath, err := claude.FindActiveSessionPath(cfg.ClaudeHome)
	if err != nil || activePath == "" {
		return
	}

	// Priority 1: consecutive tool errors.
	if n, err := claude.ParseLiveConsecutiveErrors(activePath, 50); err == nil && n >= hookThreshold {
		fmt.Fprintf(os.Stderr, "⚠ %d consecutive tool errors detected. Stop and diagnose: call get_session_dashboard (claudewatch MCP) to check token velocity, friction patterns, and context pressure before continuing.\n", n)
		os.Exit(2)
	}

	// Priority 2: context pressure.
	if ctx, err := claude.ParseLiveContextPressure(activePath); err == nil {
		if ctx.Status == "critical" || ctx.Status == "pressure" {
			pct := int(ctx.EstimatedUsage * 100)
			fmt.Fprintf(os.Stderr, "⚠ Context window at %d%% (%s). Call get_session_dashboard (claudewatch MCP) — consider compacting or wrapping up the current task before continuing.\n", pct, ctx.Status)
			os.Exit(2)
		}
	}

	// Priority 3: cost velocity.
	pricing := claude.CostPricing{
		InputPerMillion:  hookInputPerMillion,
		OutputPerMillion: hookOutputPerMillion,
	}
	if cost, err := claude.ParseLiveCostVelocity(activePath, 10, pricing); err == nil && cost.Status == "burning" {
		fmt.Fprintf(os.Stderr, "⚠ Cost velocity burning ($%.3f/min over last 10 min). Call get_session_dashboard (claudewatch MCP) to identify the source before continuing.\n", cost.CostPerMinute)
		os.Exit(2)
	}
}
