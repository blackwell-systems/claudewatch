# claudewatch

[![Blackwell Systems™](https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg)](https://github.com/blackwell-systems)
[![CI](https://github.com/blackwell-systems/claudewatch/actions/workflows/ci.yml/badge.svg)](https://github.com/blackwell-systems/claudewatch/actions)
[![Release](https://img.shields.io/github/v/release/blackwell-systems/claudewatch)](https://github.com/blackwell-systems/claudewatch/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/blackwell-systems/claudewatch)](https://goreportcard.com/report/github.com/blackwell-systems/claudewatch)

Observability and memory for Claude Code — Claude monitors its own friction, queries past attempts, and checkpoints progress across sessions.

## What you get

**Measure where you are:**
- Friction rate, cost-per-commit, agent success rates across all projects
- CLAUDE.md effectiveness scoring: before/after comparison shows what actually worked
- Model usage analysis with overspend detection and cost attribution by tool type
- Project confidence scoring: see where Claude acts vs where it's stuck reading

**Give Claude a mirror:**
- **Cross-session memory**: Claude queries "what did we try before?" and "what blockers did we hit?"
- **Live self-monitoring**: alerts on error loops, context pressure, cost spikes mid-session
- **Project health briefings**: injected at session start so Claude knows the friction history
- **Behavioral protocols**: explicit WHEN→DO triggers that tell Claude when to use memory tools

**Fix what's broken:**
- Data-driven CLAUDE.md patches generated from your actual friction patterns, not templates
- Ranked improvement suggestions by impact: see what to fix first
- Track changes over time: prove whether your CLAUDE.md edits reduced friction or increased cost
- Automatic memory extraction from completed sessions: no manual logging required

**All local.** Reads `~/.claude/` files on disk. No network calls. No telemetry.

## Why

Every developer using AI tools is guessing at how to get better. You tweak your CLAUDE.md, try different prompting styles, maybe add a hook — and hope things improve. There's no feedback loop. Did that scope constraint actually reduce unrequested edits? Did the testing section cut your debugging cycles? You have no idea. You can't improve what you can't measure.

Claude is guessing too. Every session starts fresh: no memory of which agent types failed on this project, what friction it generated last time, whether the approach it's about to take has a poor track record here. Claude makes decisions about parallelization, tool selection, and scope without any feedback from its own history. claudewatch closes that loop for both parties at once.

This is the layer nobody else occupies. LLM observability tools (LangSmith, Langfuse, Braintrust) give *humans* dashboards over API calls. claudewatch gives the *AI agent itself* queryable access to its own performance history and real-time session health — inside the session where decisions are being made. Post-hoc analytics for you, live self-reflection for Claude.

But queryable tools only help if Claude thinks to call them. claudewatch also runs a push layer: a SessionStart hook that injects a project health briefing before the first message, a PostToolUse hook that fires on error loops, context pressure, and cost spikes, and a behavioral contract in `~/.claude/CLAUDE.md` that tells Claude exactly what to do when those signals arrive. The result is a system that orients Claude at session start, alerts it mid-session when things go wrong, and gives it the vocabulary to respond — without requiring Claude to remember any of this from a previous conversation.

## How it works

claudewatch reads local session data from `~/.claude/` and turns it into actionable insights for both you and Claude.

**Give Claude a mirror.** `claudewatch install` writes a behavioral contract into `~/.claude/CLAUDE.md`. Two shell hooks — `claudewatch startup` (SessionStart) and `claudewatch hook` (PostToolUse) — orient Claude at session start and alert it mid-session when thresholds are crossed. The MCP server gives Claude queryable access to its own project health, agent history, and live session metrics. Together these form a self-monitoring layer that runs inside every Claude Code session: Claude knows what project it's on, what friction it generated last time, and when to stop and reassess — without requiring you to prompt it explicitly.

Cross-session memory tracks task history, blockers, and partial progress across sessions. When you return to a task, Claude can query "what did we try before?" via `get_task_history` and "what blockers did we hit?" via `get_blockers`. The SessionStart hook automatically extracts memory from completed sessions — no manual logging required. For long sessions or before risky operations, use `claudewatch memory extract` or the `extract_current_session_memory` MCP tool to checkpoint immediately. View stored memory with `claudewatch memory show`, or let Claude query it directly mid-session.

For multi-repo workflows, weighted attribution automatically routes sessions to their dominant project based on which files were actually touched, not just launch directory. Drift detection identifies when a session shifts from writing to reading-only (a signal you're stuck exploring). Factor analysis correlates session attributes against outcomes to answer "what predicts success on this project?" — all queryable by Claude mid-session.

**Measure where you are.** `scan` scores every project's AI readiness. `metrics` shows session trends over time -- friction rate, correction rate, cost per outcome, model usage, cache efficiency, agent success rates. Cost-per-outcome connects your token spend to what you actually shipped: cost per commit, cost per file modified, and whether successful sessions cost more or less than failed ones. Model usage analysis shows which models are consuming your budget and flags overspend. Project confidence scoring tells you where Claude knows enough to act vs where it's stuck reading -- a proxy for whether your CLAUDE.md gives the AI enough context to be productive.

```
$ claudewatch metrics --days 30

 Session Trends (30 days)
 ---------------------------------------------------------------
 Sessions            42          (1.4/day)
 Avg duration        38 min
 Friction rate       32%         down from 45%
 Satisfaction        3.8/5       up from 3.2
 Commits/session     4.2
 Cost/commit         $1.42       down from $2.10
 Cost/session        $4.28

 Tool Usage
 ---------------------------------------------------------------
 Edit                38%         most used
 Bash                24%
 Read                19%
 Grep/Glob           12%
 Task (agents)        7%

 Agent Performance
 ---------------------------------------------------------------
 Total spawned       47          (1.2/session)
 Success rate        83%         up 5%
 Background ratio    68%
 Avg duration        42s
 Avg tokens/agent    12,400

 By type:
  Explore            18  (92% success)  avg 15s
  general-purpose    14  (71% success)  avg 68s
  Plan                8  (88% success)  avg 45s
  documentation       7  (86% success)  avg 52s

 Token Usage
 ---------------------------------------------------------------
 Total tokens        18.4M
 Input               14.2M
 Output               4.2M
 Cache hit rate       62%
 Input/output ratio   3.4:1
 Avg tokens/session   438K

 Model Usage
 ---------------------------------------------------------------
 claude-sonnet-4     $48.20 (78% of spend)   16.1M tokens (87%)
 claude-opus-4       $12.80 (21% of spend)    1.8M tokens (10%)
 claude-haiku-4       $0.60  (1% of spend)    0.5M tokens  (3%)

 ⚠ Potential savings: $9.40 if Opus usage moved to Sonnet

 Project Confidence
 ---------------------------------------------------------------
 shelfctl              score: 74  read: 28%  write: 52%  explore: 15%
 bubbletea-components  score: 68  read: 35%  write: 42%  explore: 25%
 crosschain-verifier   score: 31  read: 72%  write: 12%  explore: 80%
   ⚠ low confidence — Claude spends most time reading, CLAUDE.md may need more context
```

**Find what's hurting you.** `gaps` surfaces missing context (no CLAUDE.md, no hooks, no testing section), recurring friction patterns, and stale problems that have persisted for weeks. `suggest` ranks improvements by impact so you know what to fix first.

```
$ claudewatch suggest --limit 3

 #1  Add scope constraints to shelfctl CLAUDE.md        impact: 8.4
     Unrequested edits in 55% of sessions. Adding "do not add
     features beyond what is asked" reduced this to 12% in similar
     projects.

 #2  Skip plan mode for TUI features                    impact: 7.1
     Plan agent killed in 40% of TUI sessions. Direct implementation
     with a task list achieves the same outcome faster.

 #3  Add post-edit lint hook                             impact: 6.3
     Tool errors from lint failures in 38% of Go sessions. A
     PreToolUse hook running go vet catches these before they
     cascade into multi-cycle debugging loops.
```

**Fix it automatically.** `fix` generates CLAUDE.md patches from your actual session data -- not templates, not guesses. Seven data-driven rules inspect your friction patterns, tool usage, agent kill rates, and zero-commit streaks to produce targeted additions. The `--ai` flag calls the Claude API for project-specific content grounded in your real usage.

**Track whether it worked.** `track` snapshots your metrics to SQLite and diffs against previous snapshots so you can see exactly what changed. `watch` runs in the background and alerts you when friction spikes or quality degrades.

```
$ claudewatch track --compare

 Metric                  Before     Now        Delta
 ---------------------------------------------------------------
 Friction rate           45%        28%        -17%  (improved)
 Agent success rate      71%        89%        +18%  (improved)
 Avg corrections/session 2.4        0.8        -1.6  (improved)
 Commits/session         3.1        4.6        +1.5  (improved)
 Zero-commit sessions    18%        5%         -13%  (improved)
```

**Prove it with effectiveness scoring.** `metrics` automatically scores your CLAUDE.md changes -- it splits sessions at the modification timestamp, compares before/after on friction, tool errors, goal achievement, and cost per commit, then produces a -100 to +100 effectiveness score. Did adding that scope constraint actually reduce unrequested edits? Now you know.

```
$ claudewatch metrics --effectiveness

 CLAUDE.md Effectiveness
 ---------------------------------------------------------------
 Project             Score   Verdict      Changed
 shelfctl              +72   effective    2026-01-15
 crosschain-verifier   +34   effective    2026-01-20
 bubbletea-components   -8   neutral      2026-01-22

 shelfctl (detailed):
   Friction rate       45% → 28%     -17%  (improved)
   Tool errors/session 4.2 → 1.1     -74%  (improved)
   Goal achievement    62% → 89%     +44%  (improved)
   Cost/commit         $2.10 → $1.42 -32%  (improved)
```

## Multi-agent workflow analytics

claudewatch parses session transcripts to extract agent lifecycle data that isn't available anywhere else -- not in Claude Code's UI, not in the API, not in any third-party tool.

It reconstructs agent spans from JSONL transcripts: launch to completion, success or kill, parallel or sequential, duration and token cost. From that raw data it computes success rates by agent type, parallelization ratios, correction rates, and cost per task.

A plan agent with a 40% kill rate is a signal that plan mode is costing you sessions for that project type. An explore agent that succeeds 95% of the time tells you to delegate more search tasks. claudewatch surfaces these patterns so you can adjust your workflow based on evidence, not intuition.

## Installation

**Homebrew (macOS/Linux):**
```bash
brew install blackwell-systems/tap/claudewatch
```

**Direct download:**
```bash
# Download latest release for your platform
# https://github.com/blackwell-systems/claudewatch/releases/latest

# macOS/Linux: extract and move to PATH
tar -xzf claudewatch_*_$(uname -s)_$(uname -m).tar.gz
sudo mv claudewatch /usr/local/bin/

# Windows: extract ZIP and add to PATH
```

**From source (requires Go 1.26+):**
```bash
go install github.com/blackwell-systems/claudewatch/cmd/claudewatch@latest
```

**Build from source:**
```bash
git clone https://github.com/blackwell-systems/claudewatch.git
cd claudewatch
make build
```

## Quick start

```bash
# Get a baseline on all your projects
claudewatch scan

# Find what's costing you time
claudewatch gaps

# See what to fix first
claudewatch suggest --limit 5

# Generate CLAUDE.md improvements from your session data
claudewatch fix myproject --dry-run   # preview first
claudewatch fix myproject             # apply interactively

# Measure whether it helped
claudewatch track
# ... work for a week ...
claudewatch track --compare
```

**Enable Claude's self-monitoring layer** (run once):

```bash
# Write the behavioral contract into ~/.claude/CLAUDE.md
claudewatch install

# Add hooks to ~/.claude/settings.json
# SessionStart: injects project health briefing before first message
# PostToolUse: fires on error loops, context pressure, cost spikes
```

```json
{
  "hooks": {
    "SessionStart": [{"hooks": [{"type": "command", "command": "claudewatch startup"}]}],
    "PostToolUse":  [{"hooks": [{"type": "command", "command": "claudewatch hook"}]}]
  }
}
```

Then add the MCP server to `~/.claude.json`:

```json
{
  "mcpServers": {
    "claudewatch": {
      "command": "claudewatch",
      "args": ["mcp"]
    }
  }
}
```

## Documentation

| | |
|---|---|
| 📗 [Quickstart](docs/quickstart.md) | Install, baseline, fix, measure — the full cycle in one guide |
| 📘 [CLI Reference](docs/cli.md) | All commands and flags: `scan`, `metrics`, `gaps`, `suggest`, `fix`, `track`, `log`, `watch`, `hook`, `startup`, `install` |
| 📙 [MCP Reference](docs/mcp.md) | All 29 MCP tools, setup, recommended usage pattern, and data freshness notes |
| 📕 [Effectiveness Scoring](docs/effectiveness.md) | How CLAUDE.md before/after scoring works, how to read verdicts, and what to do with regressions |

---

## Commands

| Command | What it does |
|---------|-------------|
| `scan` | Score every project's AI readiness (0-100) |
| `metrics` | Session trends: friction, cost per outcome, model usage, token breakdown, effectiveness scoring, agents, task planning |
| `gaps` | What's missing: context, hooks, stale friction patterns |
| `correlate` | Correlate session attributes against outcomes (friction, commits, cost, etc.) to find what predicts success |
| `suggest` | Ranked improvements with impact scores |
| `fix` | Generate and apply CLAUDE.md patches from session data |
| `memory` | Cross-session task history and blockers (`memory status`, `memory show`, `memory extract`, `memory clear`) |
| `track` | Snapshot metrics to SQLite, diff against previous |
| `log` | Inject custom metrics (scale, boolean, counter, duration) |
| `watch` | Background daemon with desktop alerts on friction spikes |
| `mcp` | Run an MCP stdio server — gives Claude real-time access to its own session metrics |
| `hook` | PostToolUse shell hook — checks for error loops, context pressure, and cost spikes; exits 2 with a self-contained alert if action is needed |
| `startup` | SessionStart shell hook — prints a compact briefing into Claude's context: project health, session count, friction level, MCP tool manifest |
| `install` | Write the claudewatch behavioral contract into `~/.claude/CLAUDE.md`, delimited by markers; idempotent |

### `claudewatch fix`

This is the command that closes the loop. Two modes:

- **Rule-based** (default): Seven rules inspect friction patterns, tool usage, agent kill rates, and zero-commit rates. No external dependencies.
- **AI-powered** (`--ai`): Calls the Claude API to generate project-specific content grounded in your session data and project structure. Requires `ANTHROPIC_API_KEY`.

```bash
claudewatch fix shelfctl              # rule-based, interactive
claudewatch fix shelfctl --dry-run    # preview without applying
claudewatch fix shelfctl --ai         # AI-powered generation
claudewatch fix --all                 # fix all projects scoring < 50
```

### `claudewatch watch`

Background monitoring with desktop notifications via Notification Center on macOS and libnotify on Linux.

```bash
claudewatch watch                     # foreground, ctrl-c to stop
claudewatch watch --daemon            # background with PID file
claudewatch watch --interval 5m       # custom check interval
claudewatch watch --stop              # stop background daemon
```

Notifies on: friction spikes, new stale patterns, agent kill rate increases, zero-commit streaks.

### `claudewatch mcp`

Claude doesn't understand itself. It has no native access to its own session history, cost, friction patterns, or agent timing — that data lives in JSONL transcript files that require significant domain knowledge to parse correctly. Claude could read those files directly, but doing so burns context budget on infrastructure, and some data is structurally misleading without correction (background agent completion timestamps, for example, require joining across two different JSONL entry types to get accurate durations).

claudewatch is the mirror that lets Claude see itself. The MCP server transforms raw transcript data into structured, queryable tools so that Claude can ask "how long did that parallel agent run take?" or "what has this session cost so far?" and get an answer it can immediately reason about — without leaving the session, without parsing JSONL, and without spending context on plumbing.

The 29 MCP tools operate at two time scales:

- **Historical** — project health, agent performance, friction patterns, effectiveness scores. Claude queries its own track record to make better decisions: "plan agents get killed 40% of the time on this project, skip plan mode."
- **Live** — token velocity, commit-to-attempt ratio, tool error rate, friction events. Claude monitors its own session in real time: "I'm generating errors at 30% rate, slow down and read more before editing."

No other tool gives an AI agent queryable access to its own performance data. This is what makes the MCP server qualitatively different from the CLI commands: it closes the feedback loop *inside* the session where decisions are being made.

Run claudewatch as an MCP ([Model Context Protocol](https://modelcontextprotocol.io)) stdio server.

```bash
claudewatch mcp                    # start MCP server on stdio
claudewatch mcp --budget 20        # enable daily budget tracking ($20 limit)
```

**Configure in Claude Code** by adding to `~/.claude.json`:

```json
{
  "mcpServers": {
    "claudewatch": {
      "command": "/usr/local/bin/claudewatch",
      "args": ["mcp", "--budget", "20"]
    }
  }
}
```

**Tools exposed (26 tools across 6 categories):**

*Session & cost:*

| Tool | Description |
|------|-------------|
| `get_session_stats` | Current session: cost, tokens, duration, project |
| `get_cost_budget` | Today's estimated spend vs your daily budget |
| `get_cost_summary` | Aggregated cost data: today, this week, all time, by project |
| `get_recent_sessions` | Last N sessions with friction scores and cost |

*Live self-reflection (real-time, current session):*

| Tool | Description |
|------|-------------|
| `get_session_dashboard` | All live metrics in one call: token velocity, commit ratio, context pressure, cost velocity, tool errors, friction patterns. Replaces 6 individual tool calls with one round-trip. |
| `get_token_velocity` | Tokens/minute with 10-min windowed rate — flowing, slow, or idle |
| `get_commit_attempt_ratio` | Git commits vs Edit/Write attempts — efficient, normal, or guessing |
| `get_live_tool_errors` | Error rate, errors by tool, consecutive errors, severity |
| `get_live_friction` | Friction events detected so far — retries, error bursts, tool failures |
| `get_context_pressure` | Context window utilization — comfortable, filling, pressure, or critical |
| `get_cost_velocity` | Cost burn rate over the last 10 minutes — efficient, normal, or burning |
| `get_drift_signal` | Drift detection — classifies last 20 tool calls as exploring, implementing, or drifting (stuck reading without writing) |

*Project & pattern analysis:*

| Tool | Description |
|------|-------------|
| `get_project_health` | Friction rate, agent success rate, zero-commit rate, top errors |
| `get_project_comparison` | All projects ranked side by side — health, friction, CLAUDE.md status |
| `get_suggestions` | Ranked improvement suggestions by impact score |
| `get_stale_patterns` | Chronic friction that recurs across sessions with no CLAUDE.md fix |
| `get_project_anomalies` | Detect sessions with abnormal cost or friction using z-score analysis — auto-refreshing baselines adapt to workflow changes |
| `get_regression_status` | Check if project friction rate or cost has regressed beyond baseline threshold |

*Agent & workflow analytics:*

| Tool | Description |
|------|-------------|
| `get_agent_performance` | Agent metrics: success rate, duration, tokens by type |
| `get_effectiveness` | CLAUDE.md before/after effectiveness scores per project |
| `get_session_friction` | Friction events for a specific session |
| `get_saw_sessions` | SAW parallel agent sessions with wave and agent counts |
| `get_saw_wave_breakdown` | Per-wave timing and agent status for a SAW session |
| `get_cost_attribution` | Break down token cost by tool type for a session — which tools consumed your budget |

*Multi-project analysis:*

| Tool | Description |
|------|-------------|
| `get_session_projects` | Weighted per-repo breakdown for sessions touching multiple repos — shows cost and activity distribution across projects |

*Factor analysis:*

| Tool | Description |
|------|-------------|
| `get_causal_insights` | Correlate session attributes (has_claude_md, is_saw, tool_call_count) against outcomes (friction, commits, cost) to find what predicts success |

*Cross-session memory:*

| Tool | Description |
|------|-------------|
| `get_task_history` | Query previous task attempts by description — returns task status, blockers encountered, solutions, and commits |
| `get_blockers` | List known blockers for a project with date filtering — file-specific issues, solutions, and frequency of occurrence |
| `extract_current_session_memory` | Extract and store memory from the current active session immediately — checkpoint for long sessions or before risky operations |

*Session management:*

| Tool | Description |
|------|-------------|
| `set_session_project` | Override project attribution for a session |

### Self-reflection architecture

claudewatch closes the feedback loop for Claude through three components that work at different layers of persistence. Understanding how they fit together matters for setup.

**The push/pull problem**

MCP tools are *pull* — Claude must think to call them. If Claude doesn't realize it's in trouble, it won't query `get_live_friction`. Hooks are *push* — they fire automatically after every tool use and inject signals whether Claude thinks to look or not. CLAUDE.md is *persistent* — behavioral rules that Claude Code loads at the start of every session and that remain in context regardless of how deep the conversation grows.

Each component covers a gap the others leave open:

**1. Startup briefing** (`claudewatch startup` as a SessionStart hook)

Fires at session start and prints a compact briefing directly into Claude's context: project name, session count, friction level, CLAUDE.md status, agent success rate, a context-specific tip, the full MCP tool manifest, and a PostToolUse hook reminder. This orients Claude to the project and the tools available before the first user message. Because it's injected context, it erodes as the conversation grows — useful for orientation at the start, not for behavioral rules that need to survive a 100-turn session.

Two elements of the briefing are dynamic. First, an optional regression warning line appears between the tip line and the tools line when the project's friction rate or avg cost has exceeded 1.5× its stored baseline — it is omitted entirely when the project is within baseline. Second, the tip is friction-based by default but is replaced with a SAW-correlation insight (`tip: SAW reduces zero-commit rate (X% vs Y% without)`) when there are ≥10 SAW sessions and ≥10 non-SAW sessions and the data shows a meaningful difference in zero-commit rate.

**2. Behavioral contract** (`claudewatch install` → `~/.claude/CLAUDE.md`)

`claudewatch install` writes a block of instructions into `~/.claude/CLAUDE.md`, delimited by `<!-- claudewatch:start -->` / `<!-- claudewatch:end -->` markers. The block tells Claude what to do when it sees the startup briefing (call `get_project_health` to calibrate) and what to do when the PostToolUse hook fires (stop, call `get_session_dashboard`). Without this, Claude sees the briefing but has no standing instruction to act on it. CLAUDE.md is loaded by Claude Code at session start and remains in context for the full session — it's where behavioral rules belong. Re-running `claudewatch install` updates the section in place; it's idempotent.

**3. Reactive alerts** (`claudewatch hook` as a PostToolUse hook)

Fires after every tool use, rate-limited to once per 30 seconds via `~/.cache/claudewatch-hook.ts`. Checks three conditions in priority order: (1) three or more consecutive tool errors, (2) context pressure at "pressure" or "critical", (3) cost velocity "burning". Exits 0 silently if all clear. If a condition is met, exits 2 with a self-contained stderr message that names the MCP server, the tool to call (`get_session_dashboard`), and what that tool returns — so Claude with zero prior context about claudewatch knows exactly what to do. When a consecutive error alert fires, the message also names the chronic friction pattern if one is detected (a friction type appearing in >30% of the project's last 10 sessions with no recent CLAUDE.md update), surfacing it as `(chronic: {type} in N% of recent sessions)` so Claude knows whether this is a systemic issue or an isolated event.

**Why CLAUDE.md persistence matters**

Injected context from the startup hook erodes as the conversation grows. By turn 50 it's buried under newer content. CLAUDE.md is loaded by Claude Code at the start of every session and remains in context regardless of depth. The behavioral rules — "when the hook fires, stop and call get_session_dashboard" — need to persist for the full session. The dynamic project data only needs to be fresh at session start.

**Setup**

```bash
# Install behavioral contract into ~/.claude/CLAUDE.md
claudewatch install
```

Add hooks to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [{"hooks": [{"type": "command", "command": "claudewatch startup"}]}],
    "PostToolUse": [{"hooks": [{"type": "command", "command": "claudewatch hook"}]}]
  }
}
```

### `claudewatch metrics --json`

Machine-readable JSON export for all metrics sections. Use for time-series analysis, cost dashboards, CI/CD integration, or custom queries.

```bash
claudewatch metrics --json                        # full export
claudewatch metrics --days 7 --json > week.json   # save to file
```

**Exported sections:**
- `velocity` - productivity metrics (commits/session, files modified, lines added)
- `efficiency` - tool usage, error rates, interruptions
- `satisfaction` - weighted scores, outcome distribution
- `agents` - agent performance by type, success/kill rates
- `tokens` - input/output tokens, ratios, per-session averages
- `commits` - commit patterns, zero-commit rate, detailed session list
- `conversation` - correction rate, long message frequency
- `confidence` - project confidence scores, read/write ratios
- `friction_trends` - stale/improving/worsening friction patterns
- `cost_per_outcome` - cost per commit/file/session, goal achievement (cache-adjusted)
- `effectiveness` - CLAUDE.md before/after effectiveness scoring
- `planning` - task completion rates and file churn intensity

**Example queries:**

```bash
# Track cost trends
claudewatch metrics --json | jq '.cost_per_outcome.avg_cost_per_commit'

# Find low-confidence projects
claudewatch metrics --json | jq '.confidence.projects[] | select(.confidence_score < 40)'

# Monitor effectiveness
claudewatch metrics --json | jq '.effectiveness[] | {project: .project_name, score, verdict}'

# Export for analysis
claudewatch metrics --days 30 --json > baseline.json
# ... make CLAUDE.md changes ...
claudewatch metrics --days 30 --json > after.json
# Compare in Python/R/Excel
```

## Data sources

All data is read from local files. claudewatch never writes to these paths, never modifies them, and never reads anything outside `~/.claude/`.

| Source | What it contains |
|--------|-----------------|
| `~/.claude/history.jsonl` | Conversation history |
| `~/.claude/usage-data/session-meta/` | Session metadata (tools, commits, languages) |
| `~/.claude/usage-data/facets/` | Session analysis (friction, satisfaction, goals) |
| `~/.claude/stats-cache.json` | Aggregate token usage and cache statistics |
| `~/.claude/todos/` | Task lists created during sessions (completion tracking) |
| `~/.claude/file-history/` | File edit snapshots per session (churn analysis) |
| `~/.claude/settings.json` | Global settings, hooks, permissions |
| `~/.claude/projects/` | Project-specific settings and session transcripts |
| `~/.claude/commands/` | Custom slash commands |

## Privacy

Zero network calls. Reads only local files under `~/.claude/`. Writes only to a local SQLite database for snapshot storage. No telemetry, no analytics, no crash reporting, no update checks. Nothing leaves your machine.

## Development

Pure Go, no CGO. Cross-compiles to linux/darwin/windows on amd64 and arm64.

```bash
make build      # compile to bin/claudewatch
make test       # run all tests
make vet        # go vet
make lint       # golangci-lint
make snapshot   # goreleaser snapshot build (all platforms)
```

## License

Dual-licensed under [MIT](LICENSE) and [Apache 2.0](LICENSE-APACHE).
