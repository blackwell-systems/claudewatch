# Changelog

All notable changes to claudewatch are documented here.

## [0.7.2] - 2026-03-02

### Added

- **`claudewatch install`** — writes the claudewatch behavioral contract into `~/.claude/CLAUDE.md`, delimited by HTML comment markers (`<!-- claudewatch:start -->` / `<!-- claudewatch:end -->`). Idempotent: re-running updates the section in place rather than appending. Ensures the behavioral instructions (when to call which MCP tool, how to respond to hook alerts) persist across the full session depth rather than eroding with context. Always uses `$HOME/.claude/CLAUDE.md` regardless of `claude_home` config overrides. The installed section includes a pointer to the full documentation at the bottom.

- **`claudewatch startup`** — `SessionStart` shell hook subcommand that orients Claude at the start of every session. Prints a compact 4-line briefing to stdout, which Claude Code injects directly into Claude's context before the first user message:
  - **Line 1:** Project name, session count, friction level and dominant friction type
  - **Line 2:** CLAUDE.md presence, agent success rate, and a context-specific tip derived from the top friction pattern (e.g. "verify Bash commands before running" when `retry:Bash` dominates)
  - **Line 3:** Full MCP tool manifest — every available claudewatch tool on one scannable line
  - **Line 4:** Reminder that the PostToolUse hook is active and what triggers it

  Data is pulled live from local Claude session files (`ParseAllSessionMeta`, `ParseAllFacets`, `ParseAgentTasks`) filtered to the current working directory. Requires no network calls.

  **Hook routing note:** `SessionStart` hooks use stdout + exit 0 to inject context into Claude. stderr output or exit 2 routes to the user's terminal only and is invisible to Claude. This is the inverse of `PostToolUse`, where stderr + exit 2 is what surfaces feedback to Claude.

  **settings.json configuration:**
  ```json
  {"SessionStart": [{"hooks": [{"type": "command", "command": "claudewatch startup"}]}]}
  ```

- **`claudewatch hook`** — `PostToolUse` shell hook subcommand for Claude Code. Checks the active session for three warning conditions in priority order: (1) ≥3 consecutive tool errors, (2) context window at "pressure" or "critical", (3) cost velocity "burning". Exits 0 silently if all clear; exits 2 with a self-contained stderr message naming `get_session_dashboard` and what it returns. Rate-limited to one check per 30 seconds via a timestamp file at `~/.cache/claudewatch-hook.ts`.

- **`get_session_dashboard`** — composite MCP tool that returns all live session metrics in a single call: token velocity, commit ratio, context pressure, cost velocity, tool errors, and friction patterns. Replaces 6 individual tool calls with one round-trip.

- **Active time tracking** — `get_session_dashboard` now includes an `active_time` section that distinguishes wall-clock elapsed time from actual active time. Gaps > 5 minutes between consecutive messages are classified as idle. Reports `active_minutes`, `idle_minutes`, `wall_clock_minutes`, and `resumptions` (number of idle gaps). Token velocity in the dashboard uses active minutes for lifetime averages on resumed sessions.

## [0.7.1] - 2026-03-02

### Added

- **`get_context_pressure`** — context window usage tracker for the current live session. Sums input/output tokens, counts compaction events, estimates usage ratio against 200k window. Status levels: "comfortable" (<50%), "filling" (50-75%), "pressure" (75-90%), "critical" (>=90%).

- **`get_cost_velocity`** — cost burn rate for the current live session over a 10-minute sliding window. Computes per-minute USD spend from token counts and Sonnet pricing. Status levels: "efficient" (<$0.05/min), "normal" ($0.05-0.20/min), "burning" (>=$0.20/min).

- **Friction pattern classification** — `get_live_friction` now includes a `patterns` field that collapses raw friction events into typed groups with counts, consecutive run detection, and first/last turn references. Sorted by frequency for quick triage.

## [0.7.0] - 2026-03-02

### Added

- **`get_token_velocity`** — token throughput rate for the current live session with 10-minute windowed velocity for accurate real-time status on long-running or resumed sessions. Classifies as "flowing" (>=5k tok/min), "slow" (>=1k), or "idle".

- **`get_commit_attempt_ratio`** — ratio of git commits to Edit/Write tool uses in the current live session. Classifies as "efficient" (>=0.3), "normal" (>=0.1), or "low". Signals guessing-vs-understanding.

- **`get_live_tool_errors`** — real-time tool error statistics: error rate, errors-by-tool breakdown, consecutive error streak, and severity classification ("clean", "mild", "degraded").

- **`get_live_friction`** — live friction event stream parsed from the active JSONL transcript. Detects tool errors, retries, and error bursts. Capped at 50 most recent events to prevent response overflow; summary aggregates (TotalFriction, TopType) computed from the full stream.

- **`ParseLiveToolErrors`**, **`ParseLiveFriction`**, **`ParseLiveCommitAttempts`**, **`ParseLiveTokenWindow`** — live JSONL parsing helpers in `internal/claude/active_live.go` for the self-reflection MCP tools.

## [0.6.1] - 2026-03-02

### Fixed

- **`get_cost_summary` live session gap** — the current in-progress session was invisible to cost aggregates, causing a ~$212 hole in today/week/all-time totals and by-project breakdowns. `handleGetCostSummary` now calls `FindActiveSessionPath` + `ParseActiveSession` after loading indexed sessions, deduplicates by SessionID to prevent double-counting if the session closes between calls, and applies the same time-bucket and by-project logic as indexed sessions. Non-fatal: any active session error falls through to indexed-only path.

- **`get_project_health` wrong default** — with no `project` arg the tool sorted indexed sessions by `StartTime` and picked the most recent closed session, which was wrong when a session was actively running. The default now checks for an active session first via `FindActiveSessionPath` + `ParseActiveSession`, resolves the project name via `resolveProjectName` (not `filepath.Base`, which returned the raw hash directory), and falls back to the existing sort-by-StartTime logic only when no active session is available. Priority: explicit arg > active session > most-recent indexed session.

- **`get_project_health` active-session project name** — `filepath.Base(meta.ProjectPath)` returned the hashed directory name (e.g. `-Users-dayna-blackwell-code-commitmux`) instead of the friendly project name. Root cause: `ParseActiveSession` set `meta.ProjectPath` to the hash dir name; indexed sessions carry the real filesystem path. Fixed by extracting `cwd` from the JSONL `SessionStart` progress entry (present on line 1 of every session), which contains the real project path. `resolveProjectName`'s `filepath.Base` fallback then correctly returns `commitmux` rather than the hash string. Fallback to hash-dir name is preserved for sessions without a `cwd` entry.

- **`get_cost_summary` today/week undercounting for resumed sessions** — time-bucket logic used `session.StartTime` to decide whether a session counted toward `today_usd` or `week_usd`. Long-running sessions resumed across day or week boundaries had a start time in the past, causing their cost to appear in neither bucket. Fixed by anchoring on the last entry in `UserMessageTimestamps` (most recent user activity), falling back to `StartTime` only when the timestamps list is empty.

- **`get_cost_summary` stale indexed data masking live session** — when a session was both indexed (session-meta written days ago) and live (currently running), the deduplication logic skipped the live session entirely. The indexed version had stale token counts ($1 vs $217 live) and old timestamps, leaving `today_usd` at 0. Fixed by replacing the indexed version with live data when both exist.

- **`FindActiveSessionPath` symlink resolution** — `~/.claude` symlinked to `~/workspace/.claude` caused a path mismatch: `os.ReadDir` built paths through the symlink while `lsof` reported resolved paths, so the pathSet lookup always failed. Now resolves symlinks on `claudeHome` before scanning.

- **`FindActiveSessionPath` Spotlight false positives** — `lsof -F n` (all processes) matched macOS Spotlight/mds holding stale JSONL files open for indexing, returning the wrong session. Scoped to `-c claude` to match only Claude processes.

- **`FindActiveSessionPath` stale FD selection** — when multiple JSONL files were open (stale FDs from previous sessions), the first lsof match won regardless of recency. Now collects all matches and selects by newest mtime.

- **`ParseActiveSession` missing `UserMessageTimestamps`** — active sessions had `UserMessageCount` but not `UserMessageTimestamps`, so `lastActiveTime` always fell back to `StartTime`. Now collects timestamps from user-type entries.

- **`get_session_stats` active-session project name** — same `filepath.Base(meta.ProjectPath)` bug as `get_project_health`; returned hash directory name instead of friendly project name. Fixed by using `resolveProjectName`.

### Added

- **`get_project_comparison` `min_sessions` filter** — optional integer parameter to exclude low-confidence projects from the ranked comparison. Projects with fewer sessions than `min_sessions` are filtered before sorting. Default 0 (no filter). Fixes the rezmakr skew where a single high-volume zero-commit project (81% of 43 sessions, ZeroCommitRate: 1.0) dominated aggregate health signals.

## [0.6.0] - 2026-03-01

### Added

- **Live/active session reading** — closes the gap where `get_session_stats` returned the previous completed session instead of the current one. Implemented via a 2-wave SAW run (3 agents total):

  - `internal/claude.FindActiveSessionPath(claudeHome)` — detects the currently-open JSONL session file using `lsof` (3s timeout) with mtime heuristic fallback (5-minute threshold). Returns `("", nil)` when no active session is found; never errors on missing directory.

  - `internal/claude.ParseActiveSession(path)` — reads a partial (still-being-written) JSONL transcript with line-atomic truncation at the last `\n` byte. Populates `SessionID`, `ProjectPath`, `StartTime`, `InputTokens`, `OutputTokens`, `UserMessageCount`, `AssistantMessageCount`. Best-effort: returns non-nil `*SessionMeta` even from partially parseable files.

  - `get_session_stats` MCP tool — now checks for an active session first; returns live token and cost data mid-session with `"live": true` in the response. Falls through to the previous completed-session logic when no active session is found. Enables real-time self-model: Claude can now see its own current-session token spend and cost while working.

  - `claudewatch scan --include-active` — surfaces the live session as a tagged row in the scan output. Useful for monitoring from the terminal while a session runs.

- **Explicit session project tagging** — fixes wrong project attribution when Claude Code is launched from a different directory than the project being worked on (e.g. SAW worktrees). Implemented via a 2-wave SAW run (3 agents total):

  - `set_session_project` MCP tool — override the project name for any session by ID. Call with the `session_id` from `get_session_stats` and the correct project name. The override is stored in `~/.config/claudewatch/session-tags.json` and takes precedence over the launch-directory-derived name everywhere: `get_session_stats`, `get_recent_sessions`, `get_saw_sessions`, `get_project_health`, `get_project_comparison`.

  - `claudewatch tag --project <name> [--session <id>]` CLI command — same override from the terminal. Defaults to the most recent session when `--session` is omitted. Useful for correcting attribution after the fact or from outside a Claude session.

  - `internal/store.SessionTagStore` — atomic JSON file store backing both surfaces. Write-to-temp-then-rename for POSIX atomicity; mutex-protected for concurrent access.

- **3 additional MCP self-model tools** — cross-session spend visibility, full project landscape, and chronic friction detection. Implemented via SAW Wave 1 (3 parallel agents, 3m 45s wall-clock vs ~41m sequential):

  - `get_cost_summary` — cross-session spend aggregated by period (today, this week, all time) and broken down by project, sorted by total spend. Answers "how much have I spent this week and which project is driving it?"

  - `get_project_comparison` — all known projects ranked side-by-side by health score in a single call. Health score formula: 100 − friction penalty − zero-commit penalty + agent success bonus + CLAUDE.md bonus. Enables project triage at session start without knowing project names upfront.

  - `get_stale_patterns` — friction types that recur in >N% of recent sessions AND have no corresponding CLAUDE.md change in the lookback window. Parameterized: `threshold` (default 0.3) and `lookback` (default 10 sessions). The "chronically ignored" view, distinct from `get_suggestions`.

- **5 new MCP self-model tools** — closes the gap between Claude's session data and Claude's
  in-session awareness. All five tools are thin wrappers over existing `analyzer` and `claude`
  packages, implemented via a SAW Wave 1 (4 parallel agents):

  - `get_project_health` — per-project health snapshot: friction rate, agent success rate,
    zero-commit rate, top friction types, avg tool errors, and whether a `CLAUDE.md` exists.
    Call at session start to calibrate behavior for the current project before making approach
    decisions.

  - `get_suggestions` — ranked improvement suggestions derived from session history: missing
    `CLAUDE.md`, recurring friction patterns, low agent success rates, parallelization
    opportunities. Returns top N by impact score, optionally filtered by project.

  - `get_agent_performance` — aggregate agent metrics across all session transcripts: overall
    success rate, kill rate, background ratio, avg duration and tokens. Broken down by agent
    type (Explore, Plan, general-purpose, etc.).

  - `get_effectiveness` — before/after `CLAUDE.md` effectiveness scoring per project. Compares
    friction rate, tool errors, and goal achievement across sessions before and after each
    `CLAUDE.md` change. Tells Claude whether its previous CLAUDE.md edits actually helped.

  - `get_session_friction` — live friction events for a specific session ID. Pass the current
    session ID (from `get_session_stats`) to see what friction patterns have been recorded so
    far this session, with per-type counts and the dominant friction type.

### Changed

- **README repositioned** — claudewatch is now described as a dual observability layer: for
  developers, and for Claude itself. The `## Why` section now names both blind parties — the
  developer who can't see whether their CLAUDE.md changes worked, and Claude who starts every
  session with no memory of its own failure patterns. GitHub repo description updated to match.

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