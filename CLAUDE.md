# Claude Code Instructions for claudewatch

**AgentOps for Claude Code.** Real-time monitoring and behavioral intervention for AI agents + post-session analytics for developers.

## Project Overview

**claudewatch** is AgentOps infrastructure for AI agent development. It monitors Claude Code sessions during execution (via hooks and MCP tools) and provides post-session analytics (via CLI). Think DevOps for software delivery, MLOps for ML models—AgentOps is operations for AI agents.

**What makes this AgentOps:** Monitors agent behavior (error loops, drift, context pressure), intervenes automatically (PostToolUse hooks), provides agent self-awareness (29 MCP tools Claude queries mid-session), and offers developer analytics (friction trends, cost-per-commit, agent success rates).

**Data layer:** Reads local Claude data files at `~/.claude/` (no network calls except opt-in Claude API for `fix --ai`), computes metrics, stores snapshots in a pure-Go SQLite database.

**Commands**: `scan` (inventory + score projects), `metrics` (trends), `gaps` (friction), `suggest` (ranked improvements), `track` (snapshot diffing), `log` (custom metrics), `fix` (generate + apply CLAUDE.md patches), `watch` (background daemon, friction alerts).

## Architecture

**Three-layer AgentOps model:**

1. **Push (Hooks)** - SessionStart briefing + PostToolUse alerts (error loops, drift, context pressure) fire automatically
2. **Pull (MCP Tools)** - 29 tools Claude queries mid-session for self-reflection (`get_project_health`, `get_drift_signal`, `get_blockers`)
3. **Persistent (Task Memory)** - Cross-session task history and blocker tracking via `extract_current_session_memory`

**Technical stack:**

- **CLI framework**: Cobra with global flags (`--config`, `--no-color`, `--json`, `--verbose`)
- **Database**: modernc.org/sqlite (pure Go, no CGO, enables CGO_ENABLED=0 cross-compilation)
- **Output**: lipgloss for styled tables and progress bars
- **Data flow**: parsers (claude/) → analyzers (analyzer/) → suggest engine → store → output
- **Claude API integration**: `fixer/` calls the Anthropic API (opt-in via `--ai` flag, standard `net/http`, no external SDK). Requires `ANTHROPIC_API_KEY`. All other commands remain offline-only.

**Key design principle**: Use the lightest data source that answers the question. Pre-computed metadata (session-meta, facets) for session-level metrics. Full JSONL transcripts for agent lifecycle extraction — the only source of multi-agent workflow data.

## Internal Packages

| Package | Purpose |
|---------|---------|
| `app/` | Cobra command handlers (scan, metrics, gaps, suggest, track, log, fix, watch) |
| `mcp/` | MCP stdio server (`mcp` command): JSON-RPC 2.0 tool handlers, one file per tool group |
| `claude/` | Data parsers: history, stats-cache, session-meta, facets, settings, projects, agents, **session transcripts**, todos, file-history |
| `scanner/` | Project discovery, readiness scoring algorithm |
| `analyzer/` | Friction, velocity, satisfaction, efficiency, agent metrics, cost-per-outcome (cache-adjusted), CLAUDE.md effectiveness scoring, model usage analysis, token breakdown, project confidence scoring, task planning & file churn |
| `suggest/` | Rule engine for ranked improvement suggestions |
| `store/` | SQLite schema, migrations, snapshots, queries |
| `config/` | Viper config loading with YAML defaults |
| `output/` | Table rendering, progress bars, color management |
| `fixer/` | CLAUDE.md fix generation engine (rule-based + AI-powered via Claude API) |
| `watcher/` | Background daemon, alert detection, desktop notifications (macOS/Linux) |

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
5. **MCP import cycle** — `internal/mcp` must NOT import `internal/app`. The app package imports mcp indirectly via config. MCP tool handlers build their own context inline using `claude`, `analyzer`, and `suggest` directly.

## Adding a new MCP tool

1. Create `internal/mcp/<name>_tools.go` (handler + result types) and `internal/mcp/<name>_tools_test.go`
2. Handler signature: `func (s *Server) handleGet<Name>(args json.RawMessage) (any, error)`
3. Register in `addTools` in `internal/mcp/tools.go` via `s.registerTool(toolDef{Name, Description, InputSchema, Handler})`
4. Use `noArgsSchema` for no-argument tools; write inline JSON schema for tools with args
5. Data loading is always non-fatal — on error, return zero-value result, not error
6. Result slices must be `[]T{}` (not nil); maps must be `map[K]V{}` (not nil) for clean JSON

Test helpers (all in `internal/mcp/tools_test.go`):
- `newTestServer(tmpDir)` — server with all tools registered
- `callTool(t, s, "tool_name", args)` — invoke and decode
- `writeSessionMeta`, `writeFacet`, `writeTranscriptJSONL` — synthetic data setup

Install updated binary after changes: `go build -o /opt/homebrew/bin/claudewatch ./cmd/claudewatch`

## SAW workflow for parallel features

Features with ≥2 independent file groups use Scout-and-Wave parallel agents:
- IMPL docs live in `docs/IMPL-<slug>.md` — single source of truth
- Worktrees go in `.claude/worktrees/` (gitignored)
- Agents create new files only; `tools.go` registration is always orchestrator-owned post-merge
- Run `/saw scout` to analyze, `/saw wave` to execute

## Multi-Agent Analytics

**Core AgentOps capability:** Extracting and analyzing multi-agent workflow data from session transcripts. No other tool monitors agent-to-agent coordination, success rates by agent type, or parallelization efficiency.

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

## Common Blockers & Solutions

### Session-Meta Cache Stale
**Symptom:** Metrics don't show recent sessions or model usage is missing
**Solution:** `rm -rf ~/.claude/usage-data/session-meta/*.json && claudewatch scan`
**Prevention:** Code auto-rebuilds stale cache, but manual clear sometimes needed

### Model Usage Shows `<synthetic>`
**Symptom:** Model breakdown includes `<synthetic>` with $0 cost
**Cause:** Some assistant messages lack `model` field in transcript
**Solution:** Expected behavior. Filter in display if needed

### MCP Tool Schema Mismatch
**Symptom:** Claude Code shows "tool failed" with no clear error
**Solution:** Verify parameter names in `tools.go` match handler exactly. Test with: `echo '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"tool_name"},"id":1}' | claudewatch mcp`

### SQLite Lock Error
**Symptom:** `database is locked` when running commands
**Solution:** `killall claudewatch` (check for multiple processes)
**Location:** `~/.config/claudewatch/claudewatch.db`

### Hook Fires Too Frequently
**Symptom:** PostToolUse alert appears every few tool calls
**Solution:** Hook rate-limits with 30s cooldown via `~/.cache/claudewatch-hook.ts`. Adjust `hookCooldownSeconds` in `internal/app/hook.go` if needed

### Transcript Parsing Hangs
**Symptom:** `claudewatch scan` hangs on large sessions (>100MB)
**Solution:** Already handled with 10MB buffer. If still fails, check for corrupted JSONL (missing newlines)

## Quick Debugging

- **Config not loading?** Check `~/.config/claudewatch/config.yaml` exists. Use `--config` flag to override.
- **Database locked?** Only one instance can write. Check for hung processes: `lsof ~/.config/claudewatch/claudewatch.db`
- **Parser failing?** Test with actual data: `go run ./cmd/claudewatch scan --verbose --json | jq .`
- **Suggest rule missing?** Verify registered in `suggest/engine.go` and implements `Rule` function type.
- **MCP tool not working?** Test directly: `claudewatch mcp < test-request.json`
- **Cost calculations wrong?** Verify using `analyzer.DefaultPricing` for consistency

## Documentation Standards

**CLI commands** (`docs/cli.md`): Brief description, usage examples with flags, output format description
**MCP tools** (`docs/mcp.md`): Purpose, input schema, output schema, use case (when Claude should call)
**CHANGELOG** (`CHANGELOG.md`): Follow Keep a Changelog spec - Added/Changed/Fixed/Removed sections, write for users not developers
**IMPL docs** (`docs/IMPL-*.md`): Design spec before implementation - suitability assessment, interfaces, file ownership, wave structure

## Security & Privacy

- **No network calls:** All data stays local at `~/.claude/` (except `fix --ai` which calls Anthropic API with explicit opt-in)
- **No telemetry:** Zero analytics, tracking, or remote logging
- **Sensitive data:** Transcripts may contain code, credentials, API keys
  - Never upload transcripts to external services
  - Never log transcript content without explicit flag
- **SQLite contents:** Session IDs, project names, timestamps only - no code or transcript content
- **Cross-session memory:** `~/.config/claudewatch/projects/<project>/working-memory.json` contains task descriptions, blockers - user-editable, can be deleted anytime

## Related Projects

- **interview-kit** (CLAUDE.md reference): primitives-first approach, narrative prose style
- **crosschain-verifier** (CLAUDE.md reference): two-repo architecture, testing patterns

---

## Self-Reflection Guidelines for Contributors

### Before Writing Any Code

**Ask yourself:**
- Have I read this CLAUDE.md? (If no, read it NOW)
- Does this feature already exist? (Run `claudewatch --help` to check)
- Am I about to re-implement something that's already shipped?
- Do I understand the architecture well enough to know where this code belongs?

### After Reading 3+ Files

**Pause and ask:**
- Am I exploring or implementing? (If stuck reading, call `get_drift_signal`)
- Have I found what I'm looking for, or am I drifting?
- Should I use a subagent for this exploration instead?

### After Writing Code, Before Moving On

**Check:**
- Did I write tests? (If no, STOP and write them now)
- Did I follow the naming conventions? (Analyze*/Parse*/Create-Get-Update-Delete)
- Will this break backwards compatibility? (SessionMeta, MCP API, CLI flags)
- Did I check for import cycles? (internal/mcp must NOT import internal/app)

### After Completing a Feature

**Verify:**
- Did I update docs? (cli.md, mcp.md, CHANGELOG.md, README counts)
- Did I run the full test suite? (`go test ./...`)
- Can I explain why this code is better than alternatives?
- Should I call `extract_current_session_memory` to preserve what I learned?

### Mid-Session (Every 30 Minutes)

**Reflect:**
- Am I stuck in an error loop? (Same failure 2-3 times = call `get_blockers()`)
- Is my approach working, or should I try something else?
- Have I ignored a PostToolUse hook warning? (If yes, call `get_session_dashboard` NOW)
- Am I making progress, or just generating activity?

---

## Dogfooding Protocol

**We eat our own dog food.** When developing claudewatch:
- Run with claudewatch installed and active
- Let hooks fire (don't disable them to "avoid noise")
- When PostToolUse hook alerts, call `get_session_dashboard` as instructed
- When hitting blockers, call `get_blockers()` before debugging
- After completing features, call `extract_current_session_memory`
- Track our own friction rate - target <10%

**Credibility matters:** We can't evangelize CLAUDE.md best practices without having an exemplary one ourselves. We can't recommend hooks without using them. We can't sell AgentOps without measuring our own agent workflows.

**Self-awareness questions to ask during development:**
- "Am I practicing what we preach?" (Using hooks, calling get_blockers, extracting memory)
- "Would I use this tool if I weren't building it?" (If no, why are we building it?)
- "Is this the simplest solution that could work?" (Complexity is a code smell)
- "Am I solving a real problem or building something cool?" (Former ships, latter doesn't)
- "If this were someone else's PR, would I approve it?" (Be your own tough reviewer)

---

## Quality Checklist

Before merging any change:

1. ✅ `go test ./...` passes
2. ✅ `go build ./cmd/claudewatch` succeeds
3. ✅ `claudewatch metrics --days 7` completes without panic
4. ✅ `claudewatch scan` doesn't regress on known projects
5. ✅ Documentation updated (cli.md, mcp.md, CHANGELOG.md)
6. ✅ No backwards compatibility breaks (SessionMeta, MCP API, CLI flags)
