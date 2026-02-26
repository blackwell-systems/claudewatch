# claudewatch

Observability for AI-assisted development workflows. claudewatch reads local Claude Code data files and surfaces actionable insights about how you work with AI — no network calls, no telemetry.

## What it does

When you work with Claude Code regularly, patterns emerge: which projects have good AI context, where you hit friction repeatedly, how your agent usage evolves over time. That data already exists in local files under `~/.claude/`. claudewatch reads it and makes it useful.

The core use case is improvement over time. Run `claudewatch scan` to see where your projects stand today. Run `claudewatch gaps` to find what's missing. Apply improvements. Run `claudewatch track` to measure whether things got better.

claudewatch detects the anti-patterns that erode productivity — sessions consumed by planning without implementation, recurring wrong approaches that trigger multi-cycle debugging loops, scope creep where edits add unrequested content. These show up in your session data. claudewatch surfaces them with concrete numbers so you can act on them.

The differentiating capability is **multi-agent workflow analytics**. claudewatch parses session transcripts to extract agent lifecycle data — which agent types you use, success and kill rates, parallelization patterns, and correction rates. It can also detect agent kill patterns: a plan agent with a 40% kill rate is a signal that plan mode is costing you sessions for that project type. No other tool surfaces this data.

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

The binary lands at `bin/claudewatch` when built locally. To install it to your PATH:

```bash
make install
```

## Commands

### `claudewatch scan`

Inventory all projects and score AI readiness on a 0-100 scale. The score is based on CLAUDE.md quality, session history, configured hooks, and other signals. Use this to get a baseline before making improvements.

```bash
claudewatch scan
```

### `claudewatch metrics`

Session trends over a given time window: satisfaction scores, friction rate, tool usage breakdown, cache efficiency, and agent performance.

```bash
claudewatch metrics
claudewatch metrics --days 30
```

### `claudewatch gaps`

Surface what is missing or underused — projects without CLAUDE.md files, recurring friction patterns across sessions, unconfigured hooks, unused slash commands. Recurring patterns are reported with frequency data, so you can see things like "wrong approach taken in 55% of sessions" rather than vague friction signals.

```bash
claudewatch gaps
```

### `claudewatch suggest`

Ranked improvement suggestions with impact scores. Tells you what to fix first and why. Suggestions are grounded in your actual session data — for example: add scope constraints to your CLAUDE.md to reduce unrequested edits, skip plan mode for TUI features based on your kill rate history, or auto-run the linter after Go file edits via hooks to break a recurring debugging loop.

```bash
claudewatch suggest
claudewatch suggest --limit 5
```

### `claudewatch track`

Snapshot current metrics to SQLite and, optionally, diff against the previous snapshot to measure progress.

```bash
claudewatch track                # take a snapshot
claudewatch track --compare      # diff against last snapshot
```

### `claudewatch log`

Inject custom metrics for personal tracking. Supports scale (1-10), boolean, counter, and duration types.

```bash
claudewatch log
```

## Quick start

```bash
# Get a baseline on all your projects
claudewatch scan

# Find what is missing
claudewatch gaps

# See where to focus first
claudewatch suggest --limit 5

# Check session metrics for the past month
claudewatch metrics --days 30

# Snapshot current state, then compare after making improvements
claudewatch track
# ... make improvements ...
claudewatch track --compare
```

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

Snapshot data written by `claudewatch track` is stored in a local SQLite database. That file stays on your machine.

## Privacy

claudewatch makes zero network calls. It reads only local files under `~/.claude/` and writes only to a local SQLite database for snapshot storage. No data leaves your machine. There is no telemetry, no analytics, no crash reporting, and no update checks.

## Architecture

- Pure Go, no CGO — cross-compiles to linux/darwin/windows on amd64 and arm64
- SQLite via `modernc.org/sqlite` (pure Go, no CGO dependency)
- Terminal output styled with `charmbracelet/lipgloss`
- CLI built with `cobra` and `viper`

## Development

Requirements: Go 1.24+, `golangci-lint` (for lint), `goreleaser` (for snapshot builds).

```bash
make build      # compile to bin/claudewatch
make test       # run all tests
make vet        # go vet
make lint       # golangci-lint
make clean      # remove bin/ and dist/
make snapshot   # goreleaser snapshot build (all platforms)
make install    # build and copy to GOPATH/bin or /usr/local/bin
```

The build sets version, commit, and build date via ldflags from git metadata. A dirty working tree appends `-dirty` to the version string.

## License

MIT. See [LICENSE](LICENSE).
