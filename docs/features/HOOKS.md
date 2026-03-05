# Hooks: Push Observability

Hooks bring real-time awareness to Claude during execution. They are the **push layer** of claudewatch's three-layer AgentOps model—automatic intervention that fires when thresholds are crossed, without Claude needing to remember to check.

## What Hooks Provide

Unlike memory tools that archive conversations or observability platforms that log API metrics, hooks give Claude **behavioral intervention during the session where it can act**. They detect problems as they happen and tell Claude what to do about them.

Three hook types:

1. **SessionStart** - Briefing injected into context before the first message
2. **PostToolUse** - Alert fired after tool calls when thresholds are crossed
3. **Stop** - Prompt for memory extraction when sessions close

All hooks read local data from `~/.claude/`, compute signals, and emit structured output. No network calls. No telemetry. All local.

## SessionStart Hook

Runs once at the start of every session. Prints a compact 4-line briefing that Claude Code injects into Claude's context before the first user message.

### What It Reports

- **Project health snapshot** - Session count, friction level, dominant friction type
- **CLAUDE.md presence** - Whether project-specific guidance exists
- **Agent success rate** - How well agents perform on this project
- **Regression status** - Whether friction or cost has regressed beyond baseline (optional line, only shown when regressed)
- **Contextual tip** - Actionable guidance derived from friction patterns or SAW effectiveness data
- **Available tools** - Quick reference to the most useful MCP tools

### Output Format

```
╔ claudewatch | myproject | 42 sessions | friction: moderate (retry:Bash dominant)
║ CLAUDE.md: ✓ | agent success: 73% | tip: verify Bash commands before running
║ ⚠ regression: friction rate regressed (0.80 vs baseline 0.20, threshold 1.5x)
║ tools: get_session_dashboard · get_project_health · get_live_friction · get_context_pressure · get_cost_velocity · get_suggestions
╚ PostToolUse hook active → fires on errors/context/cost → call get_session_dashboard
```

The regression line (line 3 above) appears only when a stored baseline exists for the project and the current friction rate or average cost has exceeded 1.5× that baseline. Projects within baseline omit this line entirely.

The contextual tip (line 2) is dynamically computed:
- **Default**: Derived from the top friction pattern (e.g., "verify Bash commands before running" when `retry:Bash` dominates)
- **SAW insight**: When ≥10 SAW sessions and ≥10 non-SAW sessions exist and SAW shows a meaningfully lower zero-commit rate (delta < -0.1), the tip becomes data-driven: `tip: SAW reduces zero-commit rate (X% vs Y% without)`
- Falls back to friction-based tip when session counts are insufficient

### Configuration

Add to `~/.claude/settings.json`:

```json
{
  "SessionStart": [
    {
      "hooks": [
        {
          "type": "command",
          "command": "claudewatch startup"
        }
      ]
    }
  ]
}
```

### Data Source

Pulls from:
- `~/.claude/usage-data/session-meta/*.json` - Session patterns, friction trends
- `~/.claude/projects/*/CLAUDE.md` - Context file presence detection
- `~/.config/claudewatch/claudewatch.db` - Stored baselines for regression detection
- Agent span data from full session transcripts

Filtered to the current working directory to ensure project-specific signals.

### Why It Matters

Without SessionStart, Claude starts every session blind to the project's history. It doesn't know:
- Whether this project has chronic friction patterns
- Whether agents work well here or fail 40% of the time
- Whether a CLAUDE.md exists with project-specific guidance
- Whether recent sessions show regression from historical baseline

SessionStart surfaces this context immediately, before approach decisions are made.

## PostToolUse Hook

Runs after every tool call. Checks the active session for four warning conditions in priority order and alerts Claude when a threshold is crossed.

### What It Detects

1. **Error loops** - 3+ consecutive tool errors
2. **Context pressure** - Context window at "pressure" or "critical" (75%+ utilization)
3. **Cost velocity spikes** - Burn rate ≥ $0.20/min over last 10 minutes
4. **Drift** - Read-heavy loop: ≥60% reads with 0 writes in last 15 tool calls (only fires when edits exist elsewhere in session)

### Alert Format

When a threshold is crossed, hook exits 2 with a message to stderr:

```
⚠ 3 consecutive tool errors detected (chronic: wrong_approach in 33% of recent sessions). Stop and diagnose: call get_session_dashboard to see all metrics, then get_blockers() to check for known solutions.
```

The chronic pattern parenthetical appears only when:
- A friction type appears in >30% of the project's last 10 sessions
- CLAUDE.md has not been updated in the past 14 days

Without a chronic pattern, the alert omits the parenthetical:

```
⚠ 3 consecutive tool errors detected. Stop and diagnose: call get_session_dashboard to see all metrics.
```

### Rate Limiting

Alerts are throttled to one per 30 seconds via a timestamp file at `~/.cache/claudewatch-hook.ts`. This prevents alert spam during rapid tool sequences. The cooldown is per-hook-type, not global—SessionStart is not rate-limited.

### Configuration

Add to `~/.claude/settings.json`:

```json
{
  "PostToolUse": [
    {
      "hooks": [
        {
          "type": "command",
          "command": "claudewatch hook"
        }
      ]
    }
  ]
}
```

### Data Source

Pulls from:
- Active session JSONL file (detected via `lsof` or recent mtime)
- Last 15-20 tool calls for threshold detection
- Session-level facets for chronic pattern detection

All reads are from `~/.claude/` on disk. No in-memory state required.

### Why It Matters

Without PostToolUse, Claude only learns about problems through:
1. User feedback ("this isn't working")
2. Explicit MCP tool calls (requires remembering to check)

PostToolUse fires automatically when thresholds are crossed. It doesn't wait for Claude to remember to call `get_session_dashboard`—it tells Claude to call it.

**Example intervention flow:**

1. Claude hits 3 consecutive Read errors (file not found, wrong path, etc.)
2. PostToolUse hook fires: `⚠ 3 consecutive tool errors detected. Call get_session_dashboard.`
3. Claude calls `get_session_dashboard`, sees error breakdown
4. Claude calls `get_blockers()` based on hook guidance
5. Blocker entry found: "Use absolute paths for Read tool, not relative"
6. Claude adjusts approach, applies known solution instead of rediscovering it

Without hooks, steps 2-5 don't happen automatically. Claude continues guessing until the user intervenes.

## Stop Hook

Runs when the Claude Code session closes. Detects significant sessions and prompts Claude to extract memory before context is lost.

### What It Detects

**Significant session criteria** (any of):
- Duration > 30 minutes
- Tool calls > 50
- Commits made > 0
- Errors encountered and resolved (>5 errors)

**Skip conditions**:
- Trivial session (< 10 min AND < 20 tool calls)
- Already checkpointed (`extract_current_session_memory` called)
- Pure research (zero Edit/Write calls)

### Prompt Format

The hook generates context-aware prompts based on session outcome:

**Completed session** (commits made):
```
✓ Session completed with 3 commit(s) in 45 minutes.
  Extract memory for future sessions? Call extract_current_session_memory
```

**Abandoned session** (zero commits, high errors):
```
⚠ Session ended with zero commits and 8 tool errors.
  Worth extracting blockers? Call extract_current_session_memory
```

**In-progress session** (significant work, no clear resolution):
```
📋 Session has significant work in progress (65 tool calls, 40 min).
  Extract checkpoint before closing? Call extract_current_session_memory
```

### Configuration

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "Stop": [
      {
        "type": "command",
        "command": "claudewatch hook-stop"
      }
    ]
  }
}
```

Or install via: `claudewatch install` (if Stop hook support added in installer)

### Non-Blocking Design

The Stop hook **always exits 0** and only suggests extraction—it never forces it. Claude can ignore the prompt if extraction isn't needed. This preserves user agency while providing a helpful reminder.

### Why It Matters

Completes the memory lifecycle: **SessionStart (load) → PostToolUse (monitor) → Stop (checkpoint)**

Without Stop hook:
- Long productive sessions close without checkpointing
- Task context, blockers, and solutions are lost
- SessionStart can't surface prior context because it was never saved
- Manual extraction depends on agents remembering to call the tool

With Stop hook:
- Significant sessions prompt for extraction automatically
- Cross-session learning becomes reliable
- Memory layer builds incrementally over time
- Agents develop consistent checkpoint habits

## Hook Lifecycle

1. **Installation**: `claudewatch install` writes hook configuration to `~/.claude/settings.json`
2. **Execution**: Claude Code shell hook system runs the hook command
3. **Output routing**:
   - **SessionStart**: stdout → injected into Claude's context
   - **PostToolUse**: stderr → displayed to user and Claude sees it as feedback
4. **Rate limiting**: Hook cooldown enforced via timestamp cache file

## Disabling Hooks

Remove the hook configuration from `~/.claude/settings.json`:

```bash
# Disable SessionStart briefing
jq 'del(.SessionStart)' ~/.claude/settings.json > tmp.json && mv tmp.json ~/.claude/settings.json

# Disable PostToolUse alerts
jq 'del(.PostToolUse)' ~/.claude/settings.json > tmp.json && mv tmp.json ~/.claude/settings.json
```

Or manually edit the file to remove the relevant sections.

Hooks can also be temporarily bypassed by renaming the hook cache file:

```bash
mv ~/.cache/claudewatch-hook.ts ~/.cache/claudewatch-hook.ts.disabled
```

This suppresses PostToolUse alerts without removing the configuration. Restore by renaming back.

## Hook vs MCP Tools

| | Hooks | MCP Tools |
|---|---|---|
| **Trigger** | Automatic | Explicit call |
| **Purpose** | Intervention | Query |
| **When** | Threshold crossed | Anytime |
| **Output** | Alert message | Structured data |
| **Use case** | "Stop, something is wrong" | "What is my current state?" |

Hooks push. MCP tools pull. Both read the same underlying data from `~/.claude/`.

You want both:
- **Hooks** prevent Claude from continuing down a failing path without awareness
- **MCP tools** let Claude query details and make informed decisions

## Debugging Hooks

**Hook not firing?**

1. Check `~/.claude/settings.json` for correct hook configuration
2. Verify `claudewatch startup` or `claudewatch hook` runs without error from terminal
3. Check Claude Code logs for hook execution output
4. Verify `~/.cache/claudewatch-hook.ts` timestamp—if it's recent, cooldown may be active

**Hook firing too often?**

Adjust the cooldown threshold in `internal/app/hook.go` (requires rebuilding claudewatch):

```go
const hookCooldownSeconds = 30  // increase to 60 or 120
```

Or check whether chronic patterns are misidentified—run `claudewatch gaps` to see stale friction patterns.

**Hook output not appearing in Claude's context?**

- **SessionStart**: stdout must be valid text. stderr goes only to terminal, not Claude.
- **PostToolUse**: stderr is correct. Claude sees it as feedback, not injected context.

## Installation via `claudewatch install`

The `claudewatch install` command writes hook configuration automatically:

```bash
claudewatch install
```

This updates `~/.claude/settings.json` with both SessionStart and PostToolUse hooks, and writes behavioral rules to `~/.claude/rules/claudewatch-*.md`.

Re-run after upgrading claudewatch to update hook configuration for new features.

## Related Documentation

- [MCP Tools Reference](/docs/features/MCP_TOOLS.md) - Pull-based self-reflection API
- [Hooks Implementation](/docs/technical/HOOKS_IMPL.md) - Rate limiting internals, chronic pattern detection
- [Configuration Guide](/docs/guides/CONFIGURATION.md) - Complete setup walkthrough
