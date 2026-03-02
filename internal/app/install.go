package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	claudeMDMarkerStart = "<!-- claudewatch:start -->"
	claudeMDMarkerEnd   = "<!-- claudewatch:end -->"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install claudewatch behavioral instructions into ~/.claude/CLAUDE.md",
	Long: `Writes the claudewatch behavioral contract into the global CLAUDE.md file
(~/.claude/CLAUDE.md). The section is delimited with HTML comment markers so
subsequent runs update the section in place rather than appending duplicates.

Run this once after setting up claudewatch. Re-run after upgrading to pick up
any changes to the behavioral instructions.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run:           runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error finding home directory: %v\n", err)
		return
	}

	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")

	// Read existing content; treat missing file as empty.
	existing := ""
	if data, err := os.ReadFile(claudeMDPath); err == nil {
		existing = string(data)
	}

	section := buildClaudeMDSection()
	var updated string
	var action string

	startIdx := strings.Index(existing, claudeMDMarkerStart)
	endIdx := strings.Index(existing, claudeMDMarkerEnd)

	if startIdx >= 0 && endIdx > startIdx {
		// Markers present — replace the existing section.
		updated = existing[:startIdx] + section + existing[endIdx+len(claudeMDMarkerEnd):]
		action = "updated"
	} else {
		// No markers — append. Ensure a blank line separator if file has content.
		if existing != "" {
			if !strings.HasSuffix(existing, "\n") {
				existing += "\n"
			}
			existing += "\n"
		}
		updated = existing + section + "\n"
		action = "installed"
	}

	if err := os.WriteFile(claudeMDPath, []byte(updated), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", claudeMDPath, err)
		return
	}

	fmt.Printf("claudewatch %s: %s\n", action, claudeMDPath)
}

// buildClaudeMDSection returns the full delimited claudewatch CLAUDE.md section.
// The content instructs Claude to use claudewatch MCP tools at session start
// and in response to PostToolUse hook alerts.
func buildClaudeMDSection() string {
	b := "`"
	return claudeMDMarkerStart + `
## claudewatch

At the start of every session you will receive a claudewatch briefing injected
into your context. When you see it:
1. Call ` + b + `get_project_health` + b + ` (claudewatch MCP) immediately to calibrate to
   this project's friction patterns, session history, and readiness before starting work.
2. Note the friction level and dominant friction type — adjust your approach
   accordingly (e.g. if ` + b + `retry:Bash` + b + ` dominates, verify commands before running them).

During the session:
- If the PostToolUse hook fires with a ` + b + `⚠` + b + ` warning, **stop what you are doing**
  and call ` + b + `get_session_dashboard` + b + ` (claudewatch MCP) before continuing.
  Do not proceed until you have assessed the situation.
- If context pressure reaches "pressure" or "critical", consider compacting or
  scoping down the current task.
- If cost velocity is "burning", identify the source before continuing.

Available claudewatch MCP tools:
- ` + b + `get_session_dashboard` + b + ` — all live metrics in one call (start here when the hook fires)
- ` + b + `get_project_health` + b + ` — session count, friction rate, CLAUDE.md status, agent success rate
- ` + b + `get_live_friction` + b + ` — real-time friction event stream
- ` + b + `get_context_pressure` + b + ` — context window utilization
- ` + b + `get_cost_velocity` + b + ` — cost burn rate (last 10 min)
- ` + b + `get_suggestions` + b + ` — improvement suggestions ranked by impact

Full documentation: https://github.com/blackwell-systems/claudewatch#readme
` + claudeMDMarkerEnd
}
