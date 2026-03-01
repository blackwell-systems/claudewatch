# Changelog

All notable changes to claudewatch are documented here.

## [v0.4.2] - 2026-03-01

### Fixed

- **Background agent timing** — `AgentSpan.CompletedAt` and `Duration` are now accurate for
  background Task agents. Previously, the tool_result for a background task fires at launch
  time (~1.5s), not completion, causing SAW wave timings to be severely understated. The fix
  parses `queue-operation` / `enqueue` entries in JSONL transcripts, which carry a
  `<task-notification>` payload with the real completion timestamp, `<tool-use-id>`, and
  `<total_tokens>`. These values are backfilled onto matching spans after the scan. For the
  SAW observability session: Agent A now shows 46s (was 1.5s), Agent B 108s (was 1.5s).
  `TotalTokens` is now propagated from `AgentSpan` through `ParseAgentTasks` into `AgentTask`.

## [v0.4.1] - 2026-03-01

### Added

- **SAW observability** — two new MCP tools surface Scout-and-Wave parallel agent sessions
  from session transcripts. `get_saw_sessions` lists recent sessions that used SAW-tagged
  agents (wave count, agent count, project name). `get_saw_wave_breakdown` returns per-wave
  timing and per-agent status for a given session ID. Both tools consume the structured
  `[SAW:wave{N}:agent-{X}]` prefix that `saw-skill.md` v0.3.1 now writes to Task
  `description` parameters during wave execution. Zero overhead: tags are parsed from
  existing JSONL transcripts with no additional instrumentation required.

- **`internal/claude/saw.go`** — `ParseSAWTag(description string) (wave int, agent string, ok bool)`
  parses the structured SAW tag prefix. `ComputeSAWWaves(spans []AgentSpan) []SAWSession`
  groups tagged spans into `SAWSession` / `SAWWave` / `SAWAgentRun` hierarchies with
  wall-clock timing per wave.

## [v0.4.0] - 2026-02-28

### Added

- **MCP server** — new `mcp` subcommand runs a JSON-RPC 2.0 stdio server compatible with the [Model Context Protocol](https://modelcontextprotocol.io). Exposes three tools to Claude Code and other MCP clients: `get_session_stats` (most recent completed session with cost and token breakdown), `get_cost_budget` (today's estimated spend vs a configurable daily budget), and `get_recent_sessions` (last N sessions with friction scores and cost, default 5, max 50). Start with `claudewatch mcp` or add `--budget <USD>` to enable budget tracking. Configure in `~/.claude.json` under `mcpServers` to make the tools available inside Claude Code sessions.

## [v0.3.0] - 2026-02-28

### Added

- **Session drill-down** — new `sessions` command lists individual sessions with sorting (`--sort friction|cost|duration|commits|recent`), project filtering (`--project`), configurable lookback (`--days`), and result limit (`--limit`). `--worst` is a shortcut for `--sort friction`. Supports `--json` output.
- **Session inspect** — `sessions <session-id>` shows a detailed single-session view: messages, tokens, cost, git stats, tool usage breakdown, friction events, outcome, satisfaction, and first prompt. Matches by full ID or prefix.
- **Session summary stats** — sessions table now shows a totals footer: total cost, total commits, average friction, and average duration across displayed sessions.
- **Doctor command** — new `doctor` command runs 8 health checks: Claude home directory, session data, stats cache, scan paths, SQLite database, watch daemon status, CLAUDE.md coverage, and API key. Reports pass/fail per check with a summary. Supports `--json` output.
- **Cost budget alerts** — `watch --budget <USD>` alerts when estimated daily spend exceeds the given threshold. Integrated with existing alert deduplication.
- **Track history timeline** — `track --history N` shows metric trends across N most recent snapshots in a multi-column table with trend arrows. Supports `--json` for machine-readable output.
- **Track compare wired** — `track --compare N` now actually compares against the Nth previous snapshot. Previously the flag was defined but ignored.

### Fixed

- **Accurate cost estimation** — session cost calculations now use `EstimateSessionCost` with per-model pricing (Sonnet default) and cache-adjusted ratios from `stats-cache.json`, replacing hardcoded $3/$15 per-MTok estimates. Applied to `sessions`, `watch --budget`, and cost-per-outcome metrics.

## [v0.2.1] - 2026-02-28

### Added

- **Default dashboard** — running `claudewatch` with no subcommand now shows a compact summary of key metrics from the last 30 days (sessions, duration, commits, satisfaction, tool errors, cost, zero-commit rate) instead of a static help message.

### Fixed

- **False zero-commit alerts** — watch daemon now filters trivial sessions (<5 messages and <10 minutes) from zero-commit rate detection. Short Q&A sessions no longer trigger false "High zero-commit rate" alerts.
- **Repeated alert suppression** — watch daemon deduplicates identical alerts between check cycles, only re-alerting when the underlying data changes.
- **CI workflow** — removed auto-format-and-push step that violated branch protection rules. CI now fails on unformatted code instead of attempting to push fixes directly to main. Permissions downgraded from write to read.
- **Lint compliance** — resolved all 27 golangci-lint v2 violations (errcheck, staticcheck) across 14 files. Upgraded Go from 1.24 to 1.26 and golangci-lint action from v6 to v7.

## [v0.2.0] - 2026-02-27

### Added

- **Cache-adjusted cost estimation** — cost-per-outcome and effectiveness scoring now include estimated cache-read and cache-write token costs. Derives a global cache ratio from `stats-cache.json` (cache-read/uncached multiplier, cache-write/uncached multiplier) and scales each session's uncached input tokens accordingly. Previously only priced uncached input and output tokens, significantly underestimating total spend. Falls back to uncached-only pricing when stats-cache is unavailable.
- **Task planning metrics** — new "Task Planning & File Churn" section in `metrics` parses `~/.claude/todos/` to report task completion rate, pending task count, sessions using task lists, and average tasks per session. Surfaces abandoned tasks as a friction indicator.
- **File churn analysis** — parses `~/.claude/file-history/` to measure per-session editing intensity: unique files touched, total edits (version count), average edits per file, and peak session churn. High edit counts on the same file correlate with iterative debugging cycles.

## [v0.1.1] - 2026-02-27

### Added

- **Expanded JSON output** — `metrics --json` now exports all 13 metric sections including tokens, commits, conversation quality, project confidence, friction trends, cost per outcome, and CLAUDE.md effectiveness. Previously only exported 6 top-level metrics. Enables machine-readable output for time-series analysis, cost dashboards, CI/CD integration, and custom metric queries.

### Fixed

- **Metrics data consistency** — eliminated stats-cache data mixing where metrics sections showed contradictory numbers by combining all-time historical data with time-filtered session data. All metrics now computed from the same filtered session dataset. Resolved token count discrepancies (31M vs 6B cache reads), cost contradictions ($0.00 vs $5,480.20 vs $5.29), and message count mismatches. Cost-per-outcome formatting improved to prevent line wrapping on narrow terminals.
- **Removed unused function** — removed unused `renderCostEstimation()` function that was replaced by `renderCostPerOutcome()` during stats-cache refactor.

## [v0.1.0] - 2026-02-26

### Added

- **Project confidence scoring** — classifies sessions as exploration (>60% read tools) vs implementation (>60% write tools) and computes a 0-100 confidence score per project. High read ratio with low commits signals Claude lacks project context. Surfaced in `metrics` with per-project breakdown and low-confidence warnings.
- **Model usage analysis** — per-model cost and token breakdown, tier classification (opus/sonnet/haiku), overspend detection with potential savings estimate if Opus usage moved to Sonnet, and daily model mix trends. Rendered as a new section in `metrics`.
- **Token usage breakdown** — raw token counts (input/output/cache reads/writes), cache hit rate, input/output ratio, and per-session averages. Replaces the old Token Economics section with richer detail.
- **Cost-per-outcome tracking** — connects token spend to commits, files modified, and goal achievement. Shows cost/commit (avg + median), cost/file, achieved vs not-achieved cost comparison, trend direction, and per-project breakdown. Rendered as a new section in `metrics`.
- **CLAUDE.md effectiveness scoring** — splits sessions at the CLAUDE.md modification timestamp, compares before/after on friction rate, tool errors, interruptions, goal achievement, and cost per commit. Produces a -100 to +100 score with verdict (effective/neutral/regression). Rendered as a new section in `metrics`.
- **AI-powered fix generation** — `fix --ai` calls the Claude API to generate project-specific CLAUDE.md content grounded in session data and project structure. Requires `ANTHROPIC_API_KEY`. Rule-based and AI additions are merged with AI taking precedence.
- **Watch daemon** — `watch` monitors session data in foreground or background (`--daemon`) and sends desktop notifications on friction spikes, stale patterns, agent kill rate increases, and zero-commit streaks. Supports macOS Notification Center and Linux libnotify.
- **Session transcript parser** — extracts agent lifecycle data from `~/.claude/projects/*/*.jsonl`. Reconstructs agent spans (launch to completion), success/kill status, parallel vs sequential, duration, and token cost.
- **Six new analyzers** — tool usage profiling, conversation flow (correction rate), CLAUDE.md quality correlation, friction persistence with weekly trend detection, cost estimation from token data, and commit pattern analysis (zero-commit rate).
- **claudewatch fix** — rule-based CLAUDE.md patch generation from session data. Seven rules inspect friction patterns, tool usage, agent kill rates, and zero-commit rates. Interactive apply with dry-run preview.
- **Expanded test coverage** — 375+ tests across 10 packages. suggest at 100%, scanner at 94%, claude at 89%, analyzer at 84%.
- Dual MIT / Apache-2.0 license.

### Fixed

- Plugin parser no longer crashes on unexpected JSON formats (handles structured object, plain map, and array).
- Cache hit ratio formula capped at 100% (was showing 282426%).
- Sessions sorted chronologically in project views.
- ANSI escape codes stripped before measuring table column widths.
- `SetNoColor` now actually disables styled output.
- Removed duplicate `normalizePath`, `filterFacetsByProject`, and timestamp parsers from parallel agent work.
- Windows build support with platform-specific process management (watch daemon).

### Core Features

- `scan` — inventory projects and score AI readiness (0-100).
- `metrics` — session trends: friction, satisfaction, velocity, efficiency, agent performance, token economics, model usage, project confidence.
- `gaps` — surface missing CLAUDE.md, recurring friction, unconfigured hooks, stale patterns, tool anomalies.
- `suggest` — ranked improvement suggestions with impact scoring. 13 rules covering configuration, friction, agents, cost, and custom metrics.
- `track` — snapshot metrics to SQLite, diff against previous snapshot.
- `log` — inject custom metrics (scale, boolean, counter, duration).
- Pure Go with no CGO. SQLite via modernc.org/sqlite. Cross-compiles to linux/darwin/windows on amd64 and arm64.
- CLI built with Cobra. Styled terminal output with lipgloss.
- CI/CD with format checks, lint, tests with race detection, and goreleaser on tags.