# Changelog

All notable changes to claudewatch are documented here.

## Unreleased

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

## v0.1.0 — Initial Foundation

- `scan` — inventory projects and score AI readiness (0-100).
- `metrics` — session trends: friction, satisfaction, velocity, efficiency, agent performance, token economics.
- `gaps` — surface missing CLAUDE.md, recurring friction, unconfigured hooks, stale patterns, tool anomalies.
- `suggest` — ranked improvement suggestions with impact scoring. 13 rules covering configuration, friction, agents, cost, and custom metrics.
- `track` — snapshot metrics to SQLite, diff against previous snapshot.
- `log` — inject custom metrics (scale, boolean, counter, duration).
- Pure Go with no CGO. SQLite via modernc.org/sqlite. Cross-compiles to linux/darwin/windows on amd64 and arm64.
- CLI built with Cobra. Styled terminal output with lipgloss.
