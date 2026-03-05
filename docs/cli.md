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
- **Model Usage** — per-model cost and token breakdown (sonnet/opus/haiku), spend percentages, and potential savings if Opus usage moved to Sonnet
- **Project Confidence** — read vs. write ratio per project, low-confidence warnings

**JSON sections** (with `--json`): `velocity`, `efficiency`, `satisfaction`, `agents`, `tokens`, `models`, `commits`, `conversation`, `confidence`, `friction_trends`, `cost_per_outcome`, `effectiveness`, `planning`.

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

### memory

Manage cross-session working memory for projects. Working memory stores task history, blockers, and context hints that Claude can query via MCP tools (`get_task_history`, `get_blockers`). Memory is automatically extracted from completed sessions by the SessionStart hook, or can be manually checkpointed with `memory extract`.

**Subcommands:**

#### memory status

Show cross-project memory summary: total tasks and blockers across all projects, last extraction timestamp, most recent task with status, and per-project breakdown sorted by task count.

```bash
claudewatch memory status
```

**Output:**
```
Cross-session Memory Status
─────────────────────────────────────────
Tasks stored:           8 (across 3 projects)
Blockers recorded:      4
Last extraction:        2 minutes ago
Most recent task:       "implement drift detection" (completed, claudewatch)

Projects with memory:
  claudewatch          5 tasks, 2 blockers
  commitmux           2 tasks, 1 blocker
  scout-and-wave      1 task, 1 blocker

Run 'claudewatch memory show --project <name>' for details
```

#### memory show

Display detailed working memory for a project: task history with sessions, status, commits, solutions, and blockers hit; blocker list with file, issue, solution, and last-seen timestamp.

```bash
claudewatch memory show                  # current project (from cwd)
claudewatch memory show --project commitmux
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--project <name>` | No | Project name (defaults to basename of current directory) |

#### memory extract

Extract task and blocker memory from a session immediately. Useful for checkpointing long sessions, before risky operations, or after completing major work. If `--session-id` is not specified, uses active session selection (requires active session, no fallback to historical).

```bash
claudewatch memory extract                        # extract from active session
claudewatch memory extract --session-id abc123   # extract from specific session
claudewatch memory extract --project myproject   # override project name
```

**Session Selection:**

When `--session-id` is not provided:
- **Multiple active sessions** (modified within 15 min):
  - **TTY environment:** Shows interactive numbered menu. User selects by number or Ctrl+C to cancel.
  - **Non-TTY/piped:** Returns error with session list and suggests using `--session-id`.
- **Single active session:** Uses that session automatically.
- **No active sessions:** Returns error (no fallback to historical sessions - extraction requires active work).

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--session-id <id>` | No | Session ID to extract from. When omitted, uses active session selection. |
| `--project <name>` | No | Project name (defaults to basename of current directory) |

**Output:**
- Task identifier, status, commit count
- Blocker count
- Confirmation that memory is queryable

#### memory clear

Delete working memory for a project. Prompts for confirmation before deletion.

```bash
claudewatch memory clear                  # current project
claudewatch memory clear --project myproject
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--project <name>` | No | Project name (defaults to basename of current directory) |

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

PostToolUse shell hook subcommand. Checks the active session for four warning conditions in priority order: (1) ≥3 consecutive tool errors, (2) context window at "pressure" or "critical", (3) cost velocity "burning", (4) drift (read-heavy loop: ≥60% reads, 0 writes in last 15 tools). Exits 0 silently if all clear; exits 2 with a self-contained stderr message naming the relevant MCP tool to call when a threshold is crossed. Rate-limited to one alert per 30 seconds via a timestamp file at `~/.cache/claudewatch-hook.ts`.

When the consecutive error condition fires, the alert includes chronic pattern context if a friction type appears in more than 30% of the project's last 10 sessions and CLAUDE.md has not been updated in the past 14 days. In that case the alert reads:

```
⚠ 3 consecutive tool errors detected (chronic: wrong_approach in 33% of recent sessions). Stop and diagnose: call get_session_dashboard ...
```

Without a chronic pattern, the alert omits the parenthetical.

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
║ ⚠ regression: friction rate regressed (0.80 vs baseline 0.20, threshold 1.5x)   ← only present when regressed
║ tools: get_session_dashboard · get_project_health · get_live_friction · get_context_pressure · get_cost_velocity · get_suggestions
╚ PostToolUse hook active → fires on errors/context/cost → call get_session_dashboard
```

The optional regression line (line 3 above) appears only when a baseline exists for the project and friction rate or avg cost has exceeded 1.5× that baseline. It is omitted entirely when the project is within baseline.

The tip on line 2 is dynamically computed. By default it is derived from the top friction pattern for the project (e.g. "verify Bash commands before running" when `retry:Bash` dominates). When there are ≥10 SAW sessions and ≥10 non-SAW sessions for the project and SAW sessions show a meaningfully lower zero-commit rate (delta < -0.1), the tip is replaced with a data-driven SAW insight: `tip: SAW reduces zero-commit rate (X% vs Y% without)`. Falls back to the friction-based tip when session counts are insufficient for a confident comparison.

**Hook routing note:** SessionStart hooks must write to stdout and exit 0 to inject into Claude's context. stderr output or exit 2 routes only to the user's terminal.

---

### install

Writes claudewatch behavioral rules to `~/.claude/rules/claudewatch-*.md` as modular rule files. Each file covers a single concern (session start protocol, during-session protocol, tool reference). Idempotent: re-running overwrites files in place.

On first run after upgrading from the legacy CLAUDE.md approach, automatically removes the old `<!-- claudewatch:start -->` / `<!-- claudewatch:end -->` block from `~/.claude/CLAUDE.md`.

The installed rules instruct Claude to call `get_project_health` at session start, stop and call `get_session_dashboard` when the PostToolUse hook fires, and include the full MCP tool manifest.

```bash
claudewatch install
```

**Flags:**
- `--skip-mcp` — Skip MCP server configuration (only update rules)
- `--mcp-only` — Only configure MCP server (skip rules)

**Output:**
- `rules: installed to ~/.claude/rules/claudewatch-*.md`
- `CLAUDE.md: removed legacy claudewatch block` (migration, if applicable)
- `MCP server: configured in ~/.claude.json`

**Installed files:**
- `~/.claude/rules/claudewatch-session-start.md` — session start protocol
- `~/.claude/rules/claudewatch-session-protocol.md` — during-session behaviors
- `~/.claude/rules/claudewatch-tools.md` — available MCP tools

---

### search

Full-text search over all indexed session transcripts using SQLite FTS5. Auto-indexes on first use if the index is empty, printing "Indexing transcripts…" while it runs. Use this to find sessions where a particular topic, error message, or tool was discussed.

```bash
claudewatch search "anomaly detection"
claudewatch search "go build" --limit 5
claudewatch search "friction spike" --json
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--limit <n>` | 20 | Maximum results to return |

**Output:** Table of results with session ID, entry type, timestamp, and a highlighted snippet showing where the query matched. With `--json`, returns an array of result objects. On first use with an empty index, indexing runs automatically before the query; suppressed with `--json`.

---

### compare

Side-by-side comparison of SAW parallel sessions vs sequential sessions for a project. Detects SAW sessions by parsing transcripts and identifying Scout-and-Wave patterns via `ComputeSAWWaves`. Useful for measuring whether parallelization is saving time and cost.

```bash
claudewatch compare
claudewatch compare --project claudewatch
claudewatch compare --json
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--project <name>` | most recent session's project | Project to compare |

**Output:** Table with two rows (SAW and Sequential) and columns: `Type | Sessions | Avg Cost | Avg Commits | Cost/Commit | Avg Friction`. SAW row appears first. A totals footer summarizes across both groups. With `--json`, returns the full comparison report with per-session breakdowns.

---

### anomalies

Per-project anomaly detection using z-score statistics over historical baselines. Requires ≥3 sessions. The baseline is recomputed and stored on every run using exponential decay weighting (decay=0.9), so recent sessions have more influence than older ones — baseline drift after workflow changes resolves automatically within ~10–15 sessions.

```bash
claudewatch anomalies
claudewatch anomalies --project claudewatch
claudewatch anomalies --threshold 3.0
claudewatch anomalies --json
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--project <name>` | most recent session's project | Project to analyze |
| `--threshold <float>` | 2.0 | Z-score threshold for anomaly detection |

**Output:** Table of anomalous sessions with columns: `Session | Start | Cost | Friction | Cost Z | Friction Z | Severity`. Severity is `warning` when the z-score exceeds the threshold and `critical` when it exceeds 3× the threshold. Returns an empty table (no error) when no anomalies are detected. With `--json`, returns the baseline stats alongside the anomaly list.

---

### correlate

Correlates session attributes against outcomes to identify what predicts success or failure. CLI version of factor analysis. Answers questions like "Does having CLAUDE.md reduce friction?" or "Do SAW sessions commit more?"

```bash
claudewatch correlate friction
claudewatch correlate friction --factor has_claude_md
claudewatch correlate commits --project myproject
claudewatch correlate cost --json
```

**Arguments:**

| Argument | Required | Description |
|---|---|---|
| `<outcome>` | yes | Outcome metric to analyze |

**Outcome values:** `friction`, `commits`, `zero_commit`, `cost`, `duration`, `tool_errors`

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--factor <field>` | (all factors) | Analyze specific factor instead of all factors |
| `--project <name>` | (all projects) | Filter to specific project |
| `--json` | false | Output as JSON |

**Factor values:** `has_claude_md`, `uses_task_agent`, `uses_mcp`, `uses_web_search`, `is_saw`, `tool_call_count`, `duration`, `input_tokens`

**Output:** Table with columns: `Factor | Type | Correlation/Delta | P-value | N | Confidence`

- For **numeric factors** (tool_call_count, duration, input_tokens): shows Pearson correlation coefficient and p-value
- For **boolean factors** (has_claude_md, uses_task_agent, etc.): shows delta between true and false groups
- **Confidence** is `low` when n < 10 sessions for boolean factor groups

**Example output:**

```
Factor Analysis: friction
Project: claudewatch (42 sessions)

Factor              Type     Correlation   P-value   N    Confidence
has_claude_md       boolean  -0.35        0.001     42   high
is_saw              boolean  -0.22        0.04      42   high
uses_task_agent     boolean  +0.18        0.08      42   medium
tool_call_count     numeric  +0.45        <0.001    42   high
```

---

### attribute

Break down token cost by tool type for a session. Answers "which tool calls consumed most of my budget?"

```bash
claudewatch attribute
claudewatch attribute --session abc123def456
claudewatch attribute --json
```

**Session Selection:**

When `--session` is not specified:
- **Multiple active sessions** (modified within 15 min):
  - **TTY environment:** Shows interactive numbered menu with session ID, project name, and time since last activity. User selects by number or Ctrl+C to cancel.
  - **Non-TTY/piped:** Returns error with session list and suggests using `--session` flag.
- **Single or zero active sessions:** Uses most recent session automatically.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--session <id>` | auto-detected or most recent | Session ID to analyze. When omitted, detects active sessions and prompts if multiple exist. |
| `--json` | false | Output as JSON for programmatic consumption |

**Output:** Table with columns: `Tool Type | Calls | Input Tokens | Output Tokens | Est. Cost`. A summary total line appears below the table. Header line shows which session was analyzed.

---

### replay

Walk through a session as a structured turn-by-turn timeline. Useful for post-mortems on expensive or high-friction sessions.

```bash
claudewatch replay                           # select from active sessions
claudewatch replay abc123def456              # specific session
claudewatch replay abc123 --from 10 --to 20  # range of turns
claudewatch replay --json                    # JSON output
```

**Args:** `[session-id]` (optional)

**Session Selection:**

When `session-id` argument is not provided:
- **Multiple active sessions** (modified within 15 min):
  - **TTY environment:** Shows interactive numbered menu. User selects by number or Ctrl+C to cancel.
  - **Non-TTY/piped:** Returns error with session list and suggests providing session ID.
- **Single or zero active sessions:** Uses most recent session automatically.

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--session <id>` | auto-detected or most recent | Session ID to replay (same as positional arg) |
| `--from <n>` | 1 | First turn to include (1-indexed) |
| `--to <n>` | last turn | Last turn to include |
| `--json` | false | Output as JSON for programmatic consumption |

**Output:** Section header with session summary (total turns, total cost, friction count), then a table with columns: `Turn | Role | Tool | In Tok | Out Tok | Cost | F`. The `F` column is a friction marker.

---

### experiment

Manage CLAUDE.md A/B experiments. Tag sessions to variants (a/b) and compare outcome metrics.

```bash
claudewatch experiment start --project myproject --note "testing new CLAUDE.md instructions"
claudewatch experiment tag --project myproject --variant a
claudewatch experiment report --project myproject
```

**Subcommands:**

| Subcommand | Flags | Description |
|---|---|---|
| `start` | `--project <name>`, `--note <text>` | Start a new experiment for a project |
| `stop` | `--project <name>` | Stop the active experiment |
| `tag` | `--project <name>`, `--variant <a\|b>`, `--session <id>` | Assign a session to a variant. When `--session` is omitted, uses interactive selection from active sessions (TTY) or most recent (non-TTY/fallback). |
| `report` | `--project <name>`, `--json` | Compare variant outcomes and declare a winner or "inconclusive" |

**Output of `report`:** Comparison table showing avg cost, avg friction, and avg commits per variant, followed by a winner declaration or "inconclusive" if the difference is not statistically meaningful. With `--json`, returns the full per-variant metric breakdown.

---

### doctor

Run a series of health checks against your claudewatch configuration and Claude Code data directory. Prints a pass/fail line for each check and a summary.

```bash
claudewatch doctor
claudewatch doctor --json
```

**Checks performed:**

1. Claude home directory — exists and is readable
2. Session data — at least one session-meta file found
3. Stats cache — `stats-cache.json` parses correctly
4. Scan paths — each configured path exists
5. SQLite database — `claudewatch.db` exists
6. Watch daemon — PID file exists and process is running
7. CLAUDE.md coverage — fraction of projects with a `CLAUDE.md` file (warns below 50%)
8. API key — `ANTHROPIC_API_KEY` is set (needed for `fix --ai`)
9. Anomaly baselines — all projects with ≥5 sessions have a stored baseline (run `claudewatch anomalies` to fix)
10. Regression detection — no project's friction rate or avg cost has regressed beyond 1.5× its stored baseline

**Output:** Pass (`✓`) or fail (`✗`) per check, summary line showing `N/10 checks passed`. With `--json`, a structured object with a `checks` array, `passed` count, and `total` count.

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

`--json` is supported by `scan`, `metrics`, `gaps`, `suggest`, `track`, `search`, `compare`, `anomalies`, `attribute`, `replay`, and `experiment report`. All JSON output goes to stdout; errors go to stderr. Pipe into `jq` or redirect to a file for integration with dashboards, time-series tools, or custom queries.

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
