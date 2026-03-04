package app

import (
	"encoding/json"
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
	Short: "Install claudewatch: behavioral instructions (CLAUDE.md) + MCP server config",
	Long: `Sets up claudewatch for use with Claude Code:

1. Writes behavioral protocols to ~/.claude/CLAUDE.md (enables Claude to self-reflect)
2. Configures MCP server in ~/.claude.json (enables real-time observability tools)

Both operations are idempotent. Re-run after upgrading to pick up changes.

Flags:
  --skip-mcp     Skip MCP server configuration (only update CLAUDE.md)
  --mcp-only     Only configure MCP server (skip CLAUDE.md)

Run this once after installing claudewatch.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run:           runInstall,
}

func init() {
	installCmd.Flags().Bool("skip-mcp", false, "Skip MCP server configuration")
	installCmd.Flags().Bool("mcp-only", false, "Only configure MCP server")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) {
	skipMCP, _ := cmd.Flags().GetBool("skip-mcp")
	mcpOnly, _ := cmd.Flags().GetBool("mcp-only")

	var success []string
	var failed []string

	// Install CLAUDE.md behavioral protocols
	if !mcpOnly {
		if err := installCLAUDEMD(); err != nil {
			failed = append(failed, fmt.Sprintf("CLAUDE.md: %v", err))
		} else {
			success = append(success, "CLAUDE.md: behavioral protocols installed")
		}
	}

	// Install MCP server configuration
	if !skipMCP {
		if err := installMCPServer(); err != nil {
			failed = append(failed, fmt.Sprintf("MCP server: %v", err))
		} else {
			success = append(success, "MCP server: configured in ~/.claude.json")
		}
	}

	// Report results
	if len(success) > 0 {
		fmt.Println("✓ claudewatch installed:")
		for _, msg := range success {
			fmt.Printf("  • %s\n", msg)
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "\n✗ Errors:\n")
		for _, msg := range failed {
			fmt.Fprintf(os.Stderr, "  • %s\n", msg)
		}
	}
}

// installCLAUDEMD installs behavioral protocols to ~/.claude/CLAUDE.md
func installCLAUDEMD() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home directory: %w", err)
	}

	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")

	// Read existing content; treat missing file as empty.
	existing := ""
	if data, err := os.ReadFile(claudeMDPath); err == nil {
		existing = string(data)
	}

	section := buildClaudeMDSection()
	var updated string

	startIdx := strings.Index(existing, claudeMDMarkerStart)
	endIdx := strings.Index(existing, claudeMDMarkerEnd)

	if startIdx >= 0 && endIdx > startIdx {
		// Markers present — replace the existing section.
		updated = existing[:startIdx] + section + existing[endIdx+len(claudeMDMarkerEnd):]
	} else {
		// No markers — append. Ensure a blank line separator if file has content.
		if existing != "" {
			if !strings.HasSuffix(existing, "\n") {
				existing += "\n"
			}
			existing += "\n"
		}
		updated = existing + section + "\n"
	}

	// Ensure .claude directory exists
	if err := os.MkdirAll(filepath.Dir(claudeMDPath), 0o755); err != nil {
		return fmt.Errorf("creating .claude directory: %w", err)
	}

	if err := os.WriteFile(claudeMDPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// installMCPServer configures the claudewatch MCP server in ~/.claude.json
func installMCPServer() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home directory: %w", err)
	}

	claudeJSONPath := filepath.Join(home, ".claude.json")

	// Read existing config or create empty structure
	var config map[string]interface{}
	data, err := os.ReadFile(claudeJSONPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("reading config: %w", err)
		}
		// File doesn't exist - create new config
		config = make(map[string]interface{})
	} else {
		// Parse existing config
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parsing JSON: %w", err)
		}
	}

	// Ensure mcpServers map exists
	if config["mcpServers"] == nil {
		config["mcpServers"] = make(map[string]interface{})
	}
	mcpServers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("mcpServers is not an object in config")
	}

	// Get claudewatch binary path
	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding claudewatch binary: %w", err)
	}

	// Configure claudewatch MCP server
	// JSON-RPC 2.0 protocol over stdio transport
	mcpServers["claudewatch"] = map[string]interface{}{
		"command": binPath,
		"args":    []string{"mcp", "--budget", "20"},
	}

	// Marshal back to JSON with indentation for readability
	output, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}

	// Ensure .claude directory exists
	if err := os.MkdirAll(filepath.Dir(claudeJSONPath), 0o755); err != nil {
		return fmt.Errorf("creating .claude directory: %w", err)
	}

	// Write config file
	if err := os.WriteFile(claudeJSONPath, output, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
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
3. **Check for prior context:** If the user says "continue", "resume", "keep working on",
   or references previous work, call ` + b + `get_task_history(query: "<topic>")` + b + ` to see what
   was attempted before. If you find a matching task with status "abandoned" or "in_progress",
   read its blockers/solution before proceeding.

During the session:
- If the PostToolUse hook fires with a ` + b + `⚠` + b + ` warning, **stop what you are doing**
  and call ` + b + `get_session_dashboard` + b + ` (claudewatch MCP) before continuing.
  Do not proceed until you have assessed the situation.
- If context pressure reaches "pressure" or "critical", consider compacting or
  scoping down the current task. **At "pressure" level, call ` + b + `extract_current_session_memory` + b + `
  before compaction to preserve work-in-progress.**
- If cost velocity is "burning", identify the source before continuing.
- **When hitting errors or blockers:** Before retrying or investigating extensively,
  call ` + b + `get_blockers()` + b + ` to check if this is a known issue with a documented solution.
  If found, apply the solution instead of rediscovering it.
- **On repetitive tool errors:** If the same tool/operation fails 2-3 times in a row,
  STOP and call ` + b + `get_blockers()` + b + `. This pattern indicates a known issue, not a one-off mistake.
- **Before implementing large features:** Query ` + b + `get_task_history(query: "<feature>")` + b + `
  to check if this was attempted before. If status is "abandoned", read why it failed
  and avoid the same approach.
- **When user expresses frustration:** If user says "this isn't working", "why is this broken",
  or "we tried this before", immediately call both ` + b + `get_task_history` + b + ` and ` + b + `get_blockers` + b + ` to
  surface prior context. Don't make them repeat history.
- **After completing major work:** When you finish implementing a feature, fixing a complex bug,
  or completing a multi-step task, call ` + b + `extract_current_session_memory` + b + ` to save context while
  it's fresh. Future sessions can query this via ` + b + `get_task_history` + b + ` and ` + b + `get_blockers` + b + `.
- **Before destructive operations:** ALWAYS call ` + b + `extract_current_session_memory` + b + ` before:
  git reset --hard, git push --force, rm -rf, large refactors, or any operation that could
  lose work if it fails.
- **When stuck exploring:** If you find yourself reading files repeatedly without making
  progress, call ` + b + `get_drift_signal` + b + `. If status is "drifting" (reads dominate recent
  window but session has edits), you may be avoiding implementation. Consider scoping down
  or asking the user for clarification.
- **When user asks "how did we...?":** If the user references prior work ("how did we solve X?",
  "what approach did we use for Y?"), call ` + b + `search_transcripts(query: "X")` + b + ` to find
  relevant sessions before ` + b + `get_task_history` + b + `. Transcripts include tool calls and
  conversation context that task history summaries might miss.
- **Before major commits:** After making many edits, check ` + b + `get_session_dashboard` + b + ` for
  the commit-to-attempt ratio. High ratio (commits/edits close to 1.0) means steady progress.
  Low ratio (<0.3) suggests guessing — investigate before committing potentially broken code.
- **Mid-session memory check:** Every 30-45 minutes in long sessions, ask yourself: "Have I
  learned something worth preserving?" If you've diagnosed a tricky bug, discovered an
  architectural pattern, or identified a blocker, call ` + b + `extract_current_session_memory` + b + `
  now. Don't wait until end-of-session when context might be lost to compaction.

Available claudewatch MCP tools:
- ` + b + `get_session_dashboard` + b + ` — all live metrics in one call (includes drift signal, commit ratio, and all other live data)
- ` + b + `get_project_health` + b + ` — session count, friction rate, CLAUDE.md status, agent success rate
- ` + b + `get_task_history` + b + ` — query previous task attempts by description
- ` + b + `get_blockers` + b + ` — list known blockers with solutions
- ` + b + `extract_current_session_memory` + b + ` — checkpoint task state immediately (before risky ops, after milestones, in long sessions)
- ` + b + `search_transcripts` + b + ` — full-text search across all session transcripts (use when user asks "how did we...?")
- ` + b + `get_drift_signal` + b + ` — detect when you're stuck reading without implementing
- ` + b + `get_live_friction` + b + ` — real-time friction event stream
- ` + b + `get_context_pressure` + b + ` — context window utilization
- ` + b + `get_cost_velocity` + b + ` — cost burn rate (last 10 min)
- ` + b + `get_suggestions` + b + ` — improvement suggestions ranked by impact

Full documentation: https://github.com/blackwell-systems/claudewatch#readme
` + claudeMDMarkerEnd
}
