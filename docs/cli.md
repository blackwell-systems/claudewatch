The claudewatch CLI gives developers structured visibility into their Claude Code sessions — the human-facing side of the dual observability layer. It reads local data under `~/.claude/`, computes metrics from session patterns, and stores snapshots in a local SQLite database. No network calls, no telemetry, everything stays on your machine.

These commands are run outside sessions for analysis, improvement, and tracking. For in-session real-time data — friction patterns, cost, and agent performance surfaced to Claude while decisions are being made — see [`docs/mcp.md`](mcp.md).

## Global flags

These flags are accepted by all commands.

| Flag | Default | Description |
|------|---------|-------------|
| `--config <path>` | `~/.config/claudewatch/config.yaml` | Use a custom config file |
| `--no-color` | — | Disable color output |
| `--json` | — | Emit machine-readable JSON to stdout (supported by most commands) |
| `--verbose` | — | Verbose output |

## Commands

### scan

Scores every project's AI readiness on a scale from 0 to 100. Walks `~/.claude/projects/`, computes a confidence score per project from session patterns: read/write ratio, friction rate, and context coverage. Use this as a baseline before making CLAUDE.md changes, then run it again after applying fixes to see whether scores improved.

```bash
claudewatch scan
claudewatch scan --json
claudewatch scan --include-active
```

**Flags:**

| Flag | Description |
|---|---|
| `--json` | Output as JSON instead of a table |
| `--include-active` | Include the currently running session as a live row in the output |

**Output:** Table of projects with readiness score, session count, last active date, friction rate, and confidence tier (low / medium / high). With `--include-active`, the live session appears as an additional row tagged `(live)`.

---

### metrics

Session trends over a configurable time window. The most comprehensive command — covers friction, cost-per-outcome, model usage, token breakdown, agent performance, effectiveness scoring, project confidence, and planning patterns.

```bash
claudewatch metrics
claudewatch metrics --days 7
claudewatch metrics --days 30 --json
claudewatch metrics --json > week.json
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--days <n>` | 30 | Lookback window in days |
| `--json` | — | Full JSON export |

**Key output sections:**

- **Session Trends** — friction rate, cost/session, commits/session
- **Tool Usage** — breakdown by tool type and frequency
- **Agent Performance** — by type: success rate, average duration, kill rate
- **Token Usage** — cache hit rate, input/output ratio, per-session averages
- **Model Usage** — spend and token share by model, overspend flag
- **Project Confidence** — read vs. write ratio per project, low-confidence warnings

**JSON sections** (with `--json`): `velocity`, `efficiency`, `satisfaction`, `agents`, `tokens`, `commits`, `conversation`, `confidence`, `friction_trends`, `cost_per_outcome`, `effectiveness`, `planning`.

---

### gaps

Surfaces what is structurally missing: projects without CLAUDE.md, hooks not configured, stale friction patterns that recur without a fix attempt, and high-friction commands without guidance. Faster than `metrics` — reads only metadata and facets, not full transcripts.

```bash
claudewatch gaps
claudewatch gaps --json
```

**Output:** Grouped list of gaps by category (context, hooks, patterns, friction), with project name and severity.

---

### suggest

Ranked improvement suggestions with impact scores, derived from session data. Seven rules cover: missing CLAUDE.md, recurring friction, low agent success rates, parallelization opportunities, hook configuration, stale patterns, and scope constraint issues. `suggest` shows what to fix; `fix` applies the fix.

```bash
claudewatch suggest
claudewatch suggest --limit 10
claudewatch suggest --project myproject
claudewatch suggest --json
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--limit <n>` | 5 | Maximum number of suggestions to return |
| `--project <name>` | — | Filter to a specific project |

**Output:** Ranked list with category, priority, title, description, and impact score. Higher impact score means more value to address.

---

### fix

Generates and applies CLAUDE.md patches from session data. Two modes:

- **Rule-based** (default, no API key required): Seven targeted fixes grounded in your friction patterns, tool usage, agent kill rates, and zero-commit streaks.
- **AI-powered** (`--ai`): Generates project-specific content via the Claude API. Requires `ANTHROPIC_API_KEY`.

```bash
claudewatch fix myproject              # rule-based, interactive
claudewatch fix myproject --dry-run    # preview without applying
claudewatch fix myproject --ai         # AI-powered generation
claudewatch fix --all                  # fix all projects scoring < 50
claudewatch fix --all --dry-run
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview changes without writing to disk |
| `--ai` | Use the Claude API for generation (requires `ANTHROPIC_API_KEY`) |
| `--all` | Apply to all projects with a readiness score below 50 |

Interactive mode shows a diff and prompts before each change. Run with `--dry-run` first to review what will be applied.

---

### track

Snapshots current metrics to a local SQLite database, then diffs against previous snapshots to show whether things are improving. This is the measurement half of the fix-then-measure loop.

```bash
claudewatch track              # snapshot current state
claudewatch track --compare    # diff against previous snapshot
claudewatch track --days 7     # snapshot for last 7 days only
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--compare` | — | Show delta against the most recent previous snapshot |
| `--days <n>` | 30 | Time window for the snapshot |

**Output with `--compare`:** Delta table showing friction rate change, cost/session change, agent success rate change, and commit rate change. Improvements are shown in green; regressions in red.

---

### log

Injects custom metrics into the tracking store. Supports four metric types: scale (float, for values on a continuous range), boolean (0 or 1), counter (cumulative integer), and duration (seconds).

```bash
claudewatch log --metric task_complexity --value 7.5 --type scale
claudewatch log --metric used_pair_programming --value 1 --type boolean
claudewatch log --metric bugs_found --value 3 --type counter
claudewatch log --metric review_time --value 1800 --type duration --note "auth PR"
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--metric <name>` | Yes | Metric name |
| `--value <n>` | Yes | Metric value |
| `--type <type>` | Yes | One of: `scale`, `boolean`, `counter`, `duration` |
| `--note <text>` | No | Optional annotation stored alongside the value |

---

### watch

Background daemon that monitors session data and sends desktop notifications on friction spikes, new stale patterns, agent kill rate increases, and zero-commit streaks. Uses Notification Center on macOS and libnotify on Linux.

```bash
claudewatch watch                     # foreground, ctrl-c to stop
claudewatch watch --daemon            # background with PID file
claudewatch watch --interval 5m       # custom check interval (default: 2m)
claudewatch watch --stop              # stop background daemon
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--daemon` | — | Run in background; write PID to `~/.config/claudewatch/watch.pid` |
| `--interval <duration>` | `2m` | Check interval (e.g. `30s`, `5m`, `1h`) |
| `--stop` | — | Send stop signal to the background daemon |

**Notifies on:**

- Friction rate crossing a configured threshold
- A new friction type appearing
- Agent kill rate increasing by more than 10%
- Zero-commit streak exceeding 3 sessions

---

### hook

PostToolUse shell hook subcommand. Checks the active session for three warning conditions in priority order: (1) ≥3 consecutive tool errors, (2) context window at "pressure" or "critical", (3) cost velocity "burning". Exits 0 silently if all clear; exits 2 with a self-contained stderr message naming `get_session_dashboard` and what it returns when a threshold is crossed. Rate-limited to one alert per 30 seconds via a timestamp file at `~/.cache/claudewatch-hook.ts`.

```bash
claudewatch hook   # run from a PostToolUse shell hook
```

Configuration in `~/.claude/settings.json`:
```json
{"PostToolUse": [{"hooks": [{"type": "command", "command": "claudewatch hook"}]}]}
```

No flags.

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | All clear — no thresholds exceeded |
| 2 | Threshold exceeded — message on stderr describes which condition fired and what to call |

---

### startup

SessionStart shell hook subcommand. Prints a compact 4-line briefing to stdout, which Claude Code injects into Claude's context before the first user message. Pulls live data from local session files filtered to the current working directory. Requires no network calls.

```bash
claudewatch startup   # run from a SessionStart shell hook
```

Configuration in `~/.claude/settings.json`:
```json
{"SessionStart": [{"hooks": [{"type": "command", "command": "claudewatch startup"}]}]}
```

No flags.

**Output format:**
```
╔ claudewatch | <project> | <N> sessions | friction: <level> (<top-type> dominant)
║ CLAUDE.md: ✓/✗ | agent success: <pct>% | tip: <contextual tip>
║ tools: get_session_dashboard · get_project_health · get_live_friction · get_context_pressure · get_cost_velocity · get_suggestions
╚ PostToolUse hook active → fires on errors/context/cost → call get_session_dashboard
```

**Hook routing note:** SessionStart hooks must write to stdout and exit 0 to inject into Claude's context. stderr output or exit 2 routes only to the user's terminal.

---

### install

Writes the claudewatch behavioral contract into `~/.claude/CLAUDE.md`, delimited by `<!-- claudewatch:start -->` / `<!-- claudewatch:end -->` markers. Idempotent: re-running updates the section in place rather than appending. Always writes to `$HOME/.claude/CLAUDE.md` regardless of the `claude_home` config setting.

The installed section instructs Claude to call `get_project_health` at session start, stop and call `get_session_dashboard` when the PostToolUse hook fires, and includes the full MCP tool manifest.

```bash
claudewatch install
```

No flags.

**Output:**
- `claudewatch installed: <path>` — first run
- `claudewatch updated: <path>` — subsequent runs (section replaced in place)

---

## The fix-measure loop

These commands are designed to work together in a repeated cycle:

```bash
claudewatch scan           # baseline: where are you now?
claudewatch gaps           # what's structurally missing?
claudewatch suggest        # what should you fix first?
claudewatch fix myproject  # apply the fix
# ... work for several sessions ...
claudewatch track          # snapshot after
claudewatch track --compare # did it help?
claudewatch metrics        # full picture
```

This cycle produces measurable improvement rather than guessed improvement. Skipping `track` before and after a change means you have no before/after reference point — the delta table will be empty or misleading. Snapshot before making changes, make the changes, work for several sessions, then compare.

The `get_effectiveness` MCP tool surfaces the same before/after data inside sessions, so Claude can reason about its own improvement trajectory while working.

## JSON output

`--json` is supported by `scan`, `metrics`, `gaps`, `suggest`, and `track`. All JSON output goes to stdout; errors go to stderr. Pipe into `jq` or redirect to a file for integration with dashboards, time-series tools, or custom queries.

```bash
claudewatch metrics --days 30 --json | jq '.agents.by_type'
claudewatch suggest --json | jq '[.[] | select(.impact_score > 10)]'
```

Redirect to a file to create a baseline, make CLAUDE.md changes, then diff the two exports:

```bash
claudewatch metrics --days 30 --json > baseline.json
# ... apply fixes, work for a week ...
claudewatch metrics --days 30 --json > after.json
```
