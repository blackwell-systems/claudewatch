This guide walks through a complete claudewatch cycle — from first install to measuring whether your changes helped. Setup takes about 10 minutes; meaningful effectiveness data takes a week or two of sessions to accumulate.

## Install

```bash
# Homebrew (macOS/Linux)
brew install blackwell-systems/tap/claudewatch

# Verify
claudewatch --version
```

## Baseline: where are you now?

Run a scan to see all your projects scored:

```bash
claudewatch scan
```

Example output:

```
 Project              Score  Sessions  Friction  Last Active
 ─────────────────────────────────────────────────────────────
 shelfctl               31      47       44%     2 days ago   ⚠ low
 rezmakr                68      23       18%     5 days ago
 interview-kit          82      11        9%     1 week ago
```

A score below 50 means the project would benefit from CLAUDE.md work. The `⚠ low` flag is where to start.

## Find what's missing

```bash
claudewatch gaps --project shelfctl
claudewatch suggest --project shelfctl --limit 5
```

Example `suggest` output:

```
 # Suggestions for shelfctl (5 of 12)

 1. [CLAUDE.md missing] impact: 38.9
    Add CLAUDE.md to give Claude persistent context about this project.

 2. [Parallelization opportunity] impact: 21.5
    47 agents ran sequentially that could have been parallel, costing ~24 extra minutes.

 3. [Recurring friction: wrong_approach] impact: 8.4
    "wrong_approach" appears in 41% of sessions. Add project-specific guidance to prevent it.

 4. [High plan-agent kill rate] impact: 6.1
    Plan agent killed in 38% of sessions. Consider direct implementation with a task list instead.

 5. [No post-edit hook] impact: 4.8
    Tool errors from lint failures cascade into multi-cycle debugging loops in 29% of sessions.
```

Impact scores are additive estimates of friction minutes saved per session. Higher is more urgent. See [docs/effectiveness.md](effectiveness.md) for how impact is calculated.

## Apply a fix

```bash
# Preview first (always a good idea)
claudewatch fix shelfctl --dry-run

# Apply interactively
claudewatch fix shelfctl
```

The command generates a CLAUDE.md patch from your session data and shows a diff. You approve or skip each section interactively — nothing is written until you confirm.

For AI-powered generation (more project-specific output, requires an API key):

```bash
ANTHROPIC_API_KEY=sk-... claudewatch fix shelfctl --ai
```

The `--ai` flag calls the Claude API to produce content grounded in your actual session data and project structure, rather than the default rule-based templates. Either mode writes the same format; `--ai` just produces more targeted prose.

## Enable real-time self-monitoring (optional but recommended)

Add claudewatch as an MCP server in `~/.claude.json`:

```json
{
  "mcpServers": {
    "claudewatch": {
      "command": "/opt/homebrew/bin/claudewatch",
      "args": ["mcp", "--budget", "20"]
    }
  }
}
```

Restart Claude Code. Claude can now query its own session data in real time — cost so far, friction patterns, agent performance — without leaving the session where decisions are being made. See [docs/mcp.md](mcp.md) for the full tool reference.

If you installed via direct download rather than Homebrew, replace `/opt/homebrew/bin/claudewatch` with the path from `which claudewatch`.

## Work normally for a week

Let sessions accumulate on both sides of the CLAUDE.md change. The effectiveness system splits sessions at the modification timestamp and compares before/after metrics — roughly 5 or more sessions before and 5 or more after gives meaningful signal. Fewer than that and the verdict will show as inconclusive.

Enable background monitoring if you want to be notified of friction spikes while you wait:

```bash
claudewatch watch --daemon
```

## Measure: did it help?

```bash
# Snapshot current state
claudewatch track

# After a week, compare
claudewatch track --compare
```

Example `track --compare` output:

```
 Metric              Before    After    Delta
 ─────────────────────────────────────────────
 Friction rate        44%       28%    ↓ -16%   ✓
 Cost/session        $4.20     $3.10   ↓ -26%   ✓
 Agent success rate   71%       89%    ↑ +18%   ✓
 Zero-commit rate     38%       22%    ↓ -16%   ✓
```

For the CLAUDE.md effectiveness verdict specifically:

```bash
claudewatch metrics | grep -A 10 effectiveness
```

Or in-session via MCP: call `get_effectiveness`.

The effectiveness score runs from -100 to +100. A score above +30 is a clear win. Neutral (-10 to +10) usually means not enough sessions yet, or the change addressed a pattern that wasn't actually driving friction. See [docs/effectiveness.md](effectiveness.md) for how to interpret scores.

## Repeat

```bash
claudewatch suggest --project shelfctl   # next highest-impact fix
claudewatch fix shelfctl                  # apply it
# work, track, compare, repeat
```

The loop compounds. Each measured improvement gives you confidence that the next change is worth making, and raises the floor so future sessions start from a better baseline.

## Next steps

- [docs/cli.md](cli.md) — full CLI reference
- [docs/mcp.md](mcp.md) — in-session real-time tools for Claude
- [docs/effectiveness.md](effectiveness.md) — how to interpret effectiveness scores
