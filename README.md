# claudewatch

Get measurably better at AI-assisted development.

claudewatch generates CLAUDE.md improvements from your actual Claude Code session data, then proves whether they worked with before/after effectiveness scoring. It reads local files under `~/.claude/`, finds what's costing you time and money, fixes it, and measures the result. No network calls, no telemetry, everything stays on your machine.

## The problem

Every developer using AI tools is guessing at how to get better. You tweak your CLAUDE.md, try different prompting styles, maybe add a hook -- and hope things improve. There's no feedback loop. Did that scope constraint actually reduce unrequested edits? Did the testing section cut your debugging cycles? You have no idea. You can't improve what you can't measure.

## What claudewatch does

Claude Code already records rich session data locally -- tool usage, friction events, satisfaction signals, agent lifecycles, commit patterns. claudewatch reads that data and turns it into actionable insights.

**Measure where you are.** `scan` scores every project's AI readiness. `metrics` shows session trends over time -- friction rate, correction rate, cost per outcome, model usage, cache efficiency, agent success rates. Cost-per-outcome connects your token spend to what you actually shipped: cost per commit, cost per file modified, and whether successful sessions cost more or less than failed ones. Model usage analysis shows which models are consuming your budget and flags overspend -- like burning Opus tokens on tasks Sonnet handles fine.

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

```bash
# From source (requires Go 1.24+)
go install github.com/blackwell-systems/claudewatch/cmd/claudewatch@latest

# Homebrew
brew install blackwell-systems/tap/claudewatch

# Build locally
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

## Commands

| Command | What it does |
|---------|-------------|
| `scan` | Score every project's AI readiness (0-100) |
| `metrics` | Session trends: friction, cost per outcome, model usage, token breakdown, effectiveness scoring, agents |
| `gaps` | What's missing: context, hooks, stale friction patterns |
| `suggest` | Ranked improvements with impact scores |
| `fix` | Generate and apply CLAUDE.md patches from session data |
| `track` | Snapshot metrics to SQLite, diff against previous |
| `log` | Inject custom metrics (scale, boolean, counter, duration) |
| `watch` | Background daemon with desktop alerts on friction spikes |

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

## Data sources

All data is read from local files. claudewatch never writes to these paths, never modifies them, and never reads anything outside `~/.claude/`.

| Source | What it contains |
|--------|-----------------|
| `~/.claude/history.jsonl` | Conversation history |
| `~/.claude/usage-data/session-meta/` | Session metadata (tools, commits, languages) |
| `~/.claude/usage-data/facets/` | Session analysis (friction, satisfaction, goals) |
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
