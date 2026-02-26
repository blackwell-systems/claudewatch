# Claude Code Instructions for claudewatch

CLI observability and improvement tracking for AI-assisted development workflows.

## Project Overview

**claudewatch** analyzes Claude Code sessions, scores project AI readiness, surfaces friction patterns, and tracks improvement over time. It reads local Claude data files (no network calls), computes metrics, and stores snapshots in a pure-Go SQLite database.

**Commands**: `scan` (inventory + score projects), `metrics` (trends), `gaps` (friction), `suggest` (ranked improvements), `track` (snapshot diffing), `log` (custom metrics).

## Architecture

- **CLI framework**: Cobra with global flags (`--config`, `--no-color`, `--json`, `--verbose`)
- **Database**: modernc.org/sqlite (pure Go, no CGO, enables CGO_ENABLED=0 cross-compilation)
- **Output**: lipgloss for styled tables and progress bars
- **Data flow**: parsers (claude/) → analyzers (analyzer/) → suggest engine → store → output

**Key design principle**: Use the lightest data source that answers the question. Pre-computed metadata (session-meta, facets) for session-level metrics. Full JSONL transcripts for agent lifecycle extraction — the only source of multi-agent workflow data.

## Internal Packages

| Package | Purpose |
|---------|---------|
| `app/` | Cobra command handlers (scan, metrics, gaps, suggest, track, log) |
| `claude/` | Data parsers: history, stats-cache, session-meta, facets, settings, projects, agents, **session transcripts** |
| `scanner/` | Project discovery, readiness scoring algorithm |
| `analyzer/` | Friction, velocity, satisfaction, efficiency, agent metrics computation |
| `suggest/` | Rule engine for ranked improvement suggestions |
| `store/` | SQLite schema, migrations, snapshots, queries |
| `config/` | Viper config loading with YAML defaults |
| `output/` | Table rendering, progress bars, color management |

## Build & Test

```bash
# Build
go build ./...

# Test (in-memory SQLite for all DB tests)
go test ./...

# Lint
go vet ./...

# All at once
make test
```

**Important**: Use `sqlite.NewMemoryConn()` for all database tests. Embed test fixtures (session-meta, facets) in test files via `//go:embed`.

## Key Conventions

1. **No CGO** - All dependencies must work with `CGO_ENABLED=0`. Verify with `go build -tags sqlite_omit_load_extension`.
2. **No network calls** - Data comes from local files only. Tests must not make HTTP requests.
3. **Metric types** - Custom metrics can be scale (float), boolean (0/1), counter (cumulative), or duration (seconds). Define in config.yaml under `custom_metrics`.
4. **Suggest rules** - Implement as `func(ctx *AnalysisContext) []Suggestion`. Register in `suggest.NewEngine()`.

## Multi-Agent Analytics (Active Development)

The differentiating capability of claudewatch: extracting and analyzing multi-agent workflow data from session transcripts.

**Data source**: `~/.claude/projects/<hash>/*.jsonl` — full session transcripts containing Task tool_use/tool_result pairs.

**What we extract**: Agent spans (launch → completion), agent type, duration, kill/success status, background vs foreground, parallelization patterns.

**Metrics computed**:
- Agent success/kill rate by type (Explore, Plan, general-purpose, etc.)
- Average agent duration and turns
- Parallel agent sessions (2+ concurrent agents)
- Correction rate (user redirects mid-agent)
- Agent adoption trend over time

**Suggest rules powered by agent data**:
- `ParallelizationOpportunity` — sequential agents that could run in parallel
- `AgentTypeEffectiveness` — types with high kill rates
- `AgentAdoption` — tracking agent usage growth

**Key files**: `claude/transcripts.go` (parser), `claude/agents.go` (integration), `analyzer/agents.go` (metrics).

## Extending the System

**Add a new suggest rule**:
1. Define rule function in `suggest/rules.go`: `func MyRule(ctx *AnalysisContext) []Suggestion`
2. Register in `suggest/engine.go`: `NewEngine()` rules slice
3. Implement impact scoring: `(affected_sessions * frequency * time_saved) / effort_minutes`

**Add a new custom metric type**:
1. Define in `config/config.yaml` under `custom_metrics`
2. Parse in `app/log.go` based on metric `type` field
3. Validate direction (higher_is_better, lower_is_better, etc.)
4. Store in `store.custom_metrics` table (metric_name, metric_value, tags, note)

**Add a new Claude data source**:
1. Create parser in `claude/{source}.go`
2. Add type definitions in `claude/types.go`
3. Call from appropriate command (scan → `claude.Discover()`, metrics → `claude.ParseStats()`)
4. Add unit tests with embedded test fixtures

## Testing Patterns

- Unit tests: Mock data in package-local `testdata/` directories
- Integration tests: Use temp directories with synthetic `~/.claude/` structure
- Database tests: `sqlite.NewMemoryConn()` (ephemeral, fast)
- Fixtures: Embed small JSON files with `//go:embed`, avoid version control of large files

## Important Files

- `/Users/dayna.blackwell/code/claudewatch/PLAN.md` - Full specification (data sources, scoring algorithm, agent analytics)
- `~/.claude/usage-data/session-meta/` - Session metadata (time, tools, commits, languages)
- `~/.claude/usage-data/facets/` - Session analysis (friction, satisfaction, goals)
- `~/.config/claudewatch/claudewatch.db` - SQLite snapshots, metrics, suggestions

## Distribution

- **Cross-compilation**: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/claudewatch`
- **goreleaser**: Configured in `.goreleaser.yml` for linux/darwin/windows, amd64/arm64
- **Installation**: `go install github.com/blackwell-systems/claudewatch/cmd/claudewatch@latest`

## Quick Debugging

- **Config not loading?** Check `~/.config/claudewatch/config.yaml` exists. Use `--config` flag to override.
- **Database locked?** Only one instance can write. Check for hung processes: `lsof ~/.config/claudewatch/claudewatch.db`
- **Parser failing?** Test with actual data: `go run ./cmd/claudewatch scan --verbose --json | jq .`
- **Suggest rule missing?** Verify registered in `suggest/engine.go` and implements `Rule` function type.

## Related Projects

- **interview-kit** (CLAUDE.md reference): primitives-first approach, narrative prose style
- **crosschain-verifier** (CLAUDE.md reference): two-repo architecture, testing patterns
