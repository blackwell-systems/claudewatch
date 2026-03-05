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

	// All rule files are prefixed to make ownership clear and cleanup easy.
	ruleFilePrefix = "claudewatch-"
)

// ruleFile describes a single rule file to install.
type ruleFile struct {
	Name    string // e.g. "claudewatch-session-start.md"
	Content string
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install claudewatch: behavioral rules + MCP server config",
	Long: `Sets up claudewatch for use with Claude Code:

1. Writes behavioral rules to ~/.claude/rules/claudewatch-*.md
2. Configures MCP server in ~/.claude.json (enables real-time observability tools)
3. Migrates legacy CLAUDE.md block if present (removes it)

All operations are idempotent. Re-run after upgrading to pick up changes.

Flags:
  --skip-mcp     Skip MCP server configuration (only update rules)
  --mcp-only     Only configure MCP server (skip rules)

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

	// Install rules files
	if !mcpOnly {
		if err := installRules(); err != nil {
			failed = append(failed, fmt.Sprintf("rules: %v", err))
		} else {
			success = append(success, "rules: installed to ~/.claude/rules/claudewatch-*.md")
		}

		// Migrate legacy CLAUDE.md block (best-effort, don't fail install)
		if migrated, err := migrateCLAUDEMD(); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ CLAUDE.md migration: %v\n", err)
		} else if migrated {
			success = append(success, "CLAUDE.md: removed legacy claudewatch block")
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

// installRules writes behavioral protocol files to ~/.claude/rules/.
// Each file is overwritten on every install (idempotent by design).
func installRules() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home directory: %w", err)
	}

	rulesDir := filepath.Join(home, ".claude", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return fmt.Errorf("creating rules directory: %w", err)
	}

	for _, rf := range buildRuleFiles() {
		path := filepath.Join(rulesDir, rf.Name)
		if err := os.WriteFile(path, []byte(rf.Content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", rf.Name, err)
		}
	}

	return nil
}

// migrateCLAUDEMD removes the legacy <!-- claudewatch:start/end --> block
// from ~/.claude/CLAUDE.md if present. Returns true if migration occurred.
func migrateCLAUDEMD() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("finding home directory: %w", err)
	}

	claudeMDPath := filepath.Join(home, ".claude", "CLAUDE.md")

	data, err := os.ReadFile(claudeMDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // Nothing to migrate
		}
		return false, fmt.Errorf("reading CLAUDE.md: %w", err)
	}

	existing := string(data)
	startIdx := strings.Index(existing, claudeMDMarkerStart)
	endIdx := strings.Index(existing, claudeMDMarkerEnd)

	if startIdx < 0 || endIdx <= startIdx {
		return false, nil // No legacy block present
	}

	// Remove the block and any surrounding blank lines
	before := existing[:startIdx]
	after := existing[endIdx+len(claudeMDMarkerEnd):]

	// Trim trailing whitespace from before, leading whitespace from after,
	// then rejoin with a double newline if both sides have content.
	before = strings.TrimRight(before, " \t\n")
	after = strings.TrimLeft(after, " \t\n")

	var updated string
	switch {
	case before == "" && after == "":
		// CLAUDE.md was entirely the claudewatch block — remove the file
		if err := os.Remove(claudeMDPath); err != nil {
			return false, fmt.Errorf("removing empty CLAUDE.md: %w", err)
		}
		return true, nil
	case before == "":
		updated = after + "\n"
	case after == "":
		updated = before + "\n"
	default:
		updated = before + "\n\n" + after + "\n"
	}

	if err := os.WriteFile(claudeMDPath, []byte(updated), 0o644); err != nil {
		return false, fmt.Errorf("writing CLAUDE.md: %w", err)
	}

	return true, nil
}

// buildRuleFiles returns the set of rule files that claudewatch installs.
// Split by concern so each file receives focused attention from the model.
func buildRuleFiles() []ruleFile {
	return []ruleFile{
		{
			Name:    ruleFilePrefix + "session-start.md",
			Content: buildSessionStartRule(),
		},
		{
			Name:    ruleFilePrefix + "session-protocol.md",
			Content: buildSessionProtocolRule(),
		},
		{
			Name:    ruleFilePrefix + "tools.md",
			Content: buildToolsRule(),
		},
	}
}

func buildSessionStartRule() string {
	return `# claudewatch — Session Start Protocol

At the start of EVERY session (new or resumed), execute this protocol
BEFORE responding to the user's request:

1. Call ` + "`get_project_health`" + ` (claudewatch MCP) immediately.
   You cannot calibrate to friction patterns, historical blockers, or agent
   failure modes without this baseline. Proceeding without it means repeating
   known failures.

2. Read and internalize the friction metrics returned. If friction
   rate > 30%, you are operating in a high-error environment. If specific agent
   types show 0% success rate, DO NOT spawn them. If top friction type is
   ` + "`buggy_code`" + ` or ` + "`retry:Bash`" + `, verify every command before execution.

3. When resuming work: If the user says "continue", "resume", "keep
   working on", or references previous work, call ` + "`get_task_history(query: \"<topic>\")`" + `
   BEFORE attempting implementation. If you find a matching task with status
   "abandoned" or "in_progress", read its blockers/solution. Do not repeat
   failed approaches.
`
}

func buildSessionProtocolRule() string {
	return `# claudewatch — During-Session Protocol

## Hook Response

If the PostToolUse hook fires with a ` + "`⚠`" + ` warning, stop immediately and call
` + "`get_session_dashboard`" + ` (claudewatch MCP). Do NOT continue without assessing
the situation. This warning indicates error loops, drift, context pressure,
or cost spikes that require immediate attention.

## Context Pressure & Memory

- If context pressure reaches "pressure" or "critical", consider compacting or
  scoping down the current task. At "pressure" level, call
  ` + "`extract_current_session_memory`" + ` before compaction to preserve work-in-progress.
- If cost velocity is "burning", identify the source before continuing.
- Every 30-45 minutes in long sessions, ask: "Have I learned something worth
  preserving?" If yes, call ` + "`extract_current_session_memory`" + ` now.
- Before destructive operations (git reset --hard, git push --force, rm -rf,
  large refactors), ALWAYS call ` + "`extract_current_session_memory`" + ` first.

## Error & Blocker Handling

- When hitting errors or blockers: Before retrying or investigating extensively,
  call ` + "`get_blockers()`" + ` to check if this is a known issue with a documented solution.
  If found, apply the solution instead of rediscovering it.
- On repetitive tool errors: If the same tool/operation fails 2-3 times in a row,
  STOP and call ` + "`get_blockers()`" + `. This pattern indicates a known issue, not a
  one-off mistake.

## Task History Awareness

- Before implementing large features: Query ` + "`get_task_history(query: \"<feature>\")`" + `
  to check if this was attempted before. If status is "abandoned", read why it
  failed and avoid the same approach.
- When the user expresses frustration ("this isn't working", "why is this broken",
  "we tried this before"), immediately call both ` + "`get_task_history`" + ` and
  ` + "`get_blockers`" + ` to surface prior context.
- When the user asks "how did we...?", call ` + "`search_transcripts(query: \"X\")`" + `
  to find relevant sessions before ` + "`get_task_history`" + `.

## Progress Tracking

- After completing major work (feature, complex bug fix, multi-step task),
  call ` + "`extract_current_session_memory`" + ` to save context while it's fresh.
- Before major commits, check ` + "`get_session_dashboard`" + ` for the
  commit-to-attempt ratio. Low ratio (<0.3) suggests guessing — investigate
  before committing potentially broken code.
- If stuck exploring (reading files repeatedly without making progress),
  call ` + "`get_drift_signal`" + `. If status is "drifting", scope down or ask
  the user for clarification.
`
}

func buildToolsRule() string {
	return `# claudewatch — Available MCP Tools

- ` + "`get_session_dashboard`" + ` — all live metrics in one call (drift, commit ratio, cost, errors)
- ` + "`get_project_health`" + ` — session count, friction rate, CLAUDE.md status, agent success rate
- ` + "`get_task_history`" + ` — query previous task attempts by description
- ` + "`get_blockers`" + ` — list known blockers with solutions
- ` + "`extract_current_session_memory`" + ` — checkpoint task state (before risky ops, after milestones)
- ` + "`search_transcripts`" + ` — full-text search across all session transcripts
- ` + "`get_drift_signal`" + ` — detect when you're stuck reading without implementing
- ` + "`get_live_friction`" + ` — real-time friction event stream
- ` + "`get_context_pressure`" + ` — context window utilization
- ` + "`get_cost_velocity`" + ` — cost burn rate (last 10 min)
- ` + "`get_suggestions`" + ` — improvement suggestions ranked by impact

Full documentation: https://github.com/blackwell-systems/claudewatch#readme
`
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
