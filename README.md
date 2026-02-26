# claudewatch

Get measurably better at AI-assisted development.

claudewatch analyzes your Claude Code sessions and tells you — with data — what's working, what's not, and what to change. It reads local files under `~/.claude/`, computes metrics, and generates concrete improvements. No network calls, no telemetry, everything stays on your machine.

## The problem

Every developer using AI tools hits the same friction: sessions that spiral, plans that get killed, prompts that trigger the wrong behavior. Most people respond by guessing — tweaking their CLAUDE.md, trying different prompting styles, hoping things improve. There's no feedback loop. You can't improve what you can't measure.

## What claudewatch does

Claude Code already records rich session data locally — tool usage, friction events, satisfaction signals, agent lifecycles, commit patterns. claudewatch reads that data and turns it into actionable insights.

**Measure where you are.** `scan` scores every project's AI readiness. `metrics` shows session trends over time — friction rate, correction rate, cost per session, cache efficiency, agent success rates.

**Find what's hurting you.** `gaps` surfaces missing context (no CLAUDE.md, no hooks, no testing section), recurring friction patterns, and stale problems that have persisted for weeks. `suggest` ranks improvements by impact so you know what to fix first.

**Fix it automatically.** `fix` generates CLAUDE.md patches from your actual session data — not templates, not guesses. Seven data-driven rules inspect your friction patterns, tool usage, agent kill rates, and zero-commit streaks to produce targeted additions. The `--ai` flag calls the Claude API for project-specific content grounded in your real usage.

**Track whether it worked.** `track` snapshots your metrics to SQLite. Run it before and after changes to see measurable improvement. `watch` runs in the background and alerts you when friction spikes or quality degrades.

The differentiating capability is **multi-agent workflow analytics**. claudewatch is the only tool that extracts agent lifecycle data from session transcripts — which agent types you use, success and kill rates, parallelization patterns, and correction rates. A plan agent with a 40% kill rate is a signal. An explore agent that succeeds 95% of the time is a signal. claudewatch surfaces these so you can adjust your workflow based on evidence.

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

# Stay ahead of problems
claudewatch watch --daemon
```

## Commands

| Command | What it does |
|---------|-------------|
| `scan` | Score every project's AI readiness (0-100) |
| `metrics` | Session trends: friction, satisfaction, cost, agents, commits |
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

Background monitoring with desktop notifications (macOS and Linux).

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
