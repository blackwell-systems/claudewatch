# MCP Tools: Pull Observability

The claudewatch MCP server gives you queryable access to your own session patterns, project health, and behavioral metrics. This is the **pull layer** of the three-layer AgentOps model—self-reflection tools you call explicitly to inform decisions.

## Overview

You have no native access to your session history, token spend, or behavioral patterns. You operate inside a context window without knowledge of what happened in prior sessions, which friction types you've generated repeatedly, or how much of today's budget has already been consumed.

claudewatch closes that gap. The raw data—JSONL transcripts, session metadata, usage facets—lives in `~/.claude/` and requires domain knowledge to parse correctly. The MCP server reads that data and surfaces it as structured, queryable tools you can call from inside the session where decisions are being made.

This is different from CLI commands. `claudewatch health` or `claudewatch friction` are retrospective: a developer reviews output after the fact, outside the session. The MCP server creates a feedback loop inside the session itself. You can check the project's friction history before choosing an approach, verify you have budget headroom before spinning up parallel agents, or confirm that a CLAUDE.md change actually shifted outcomes—all without leaving the conversation.

The result is dual observability: developers get the CLI for review and configuration; you get the MCP for real-time self-awareness. Both layers read the same underlying data, written by Claude Code as it works.

## Setup

Add claudewatch to `~/.claude.json` under `mcpServers`:

```json
{
  "mcpServers": {
    "claudewatch": {
      "command": "/opt/homebrew/bin/claudewatch",
      "args": ["mcp", "--budget", "20"]
    }
  }
}
```

The `--budget` flag sets a daily spend limit in USD used by `get_cost_budget` to compute remaining headroom and alert when the limit is approached. Omit the flag to disable budget tracking—`get_cost_budget` will still return today's spend without a limit comparison.

Restart Claude Code after installing a new claudewatch binary to pick up changes. The MCP server process runs for the lifetime of the Claude Code session; configuration changes take effect at the next session start.

## Tool Categories

29 tools organized into 7 functional groups:

1. **Session awareness** - Current session metrics and live data
2. **Project calibration** - Project-level health and friction patterns
3. **Improvement guidance** - Ranked suggestions and stale pattern detection
4. **Performance analytics** - Cost, effectiveness, and agent metrics
5. **SAW observability** - Scout-and-Wave parallel workflow timing
6. **Multi-project analysis** - Cross-repo attribution and weighted breakdowns
7. **AI Ops** - Transcript search, anomalies, attribution, regression detection

## Recommended Usage Pattern

### Session Start

Call these before making approach decisions:

1. **`get_project_health`** - Understand this project's friction history and agent success patterns before deciding how to work
2. **`get_project_comparison`** - If choosing which project needs attention, see all projects ranked by health score at once
3. **`get_cost_budget`** - Check available budget before committing to a token-intensive approach or parallel agent strategy

### Mid-Session

Call these when making decisions about what to do next:

4. **`get_session_stats`** - Current session cost and token usage
5. **`get_session_friction`** - What friction has already occurred this session, so you can adjust
6. **`get_suggestions`** - Ranked improvement opportunities if you're deciding what to tackle

### Review and Reflection

Call these to understand patterns over time or validate prior changes:

7. **`get_agent_performance`** - Which agent types work best for this project's sessions
8. **`get_effectiveness`** - Whether CLAUDE.md changes produced measurable before/after improvement
9. **`get_cost_summary`** - Where budget is going across all projects
10. **`get_stale_patterns`** - Friction types that have recurred without any CLAUDE.md response
11. **`get_saw_sessions`** + **`get_saw_wave_breakdown`** - Scout-and-Wave parallel agent workflow timing and status
12. **`search_transcripts`** - Find sessions where a specific topic, error, or tool was discussed
13. **`get_project_anomalies`** - Identify sessions that deviated significantly from the project's cost or friction baseline
14. **`get_cost_attribution`** - Break down token cost by tool type for a session
15. **`get_regression_status`** - Check whether friction rate or cost-per-session has regressed beyond baseline

## Session Awareness Tools

### `get_session_stats`

Returns metrics for the current active session: cost, duration, token usage, and friction score. When the session JSONL file is actively open (detected via `lsof` or recent mtime), the response includes live token and cost data read directly from the in-progress transcript.

**Input:** None

**Output:**

| Field | Type | Description |
|---|---|---|
| `session_id` | string | Unique session identifier |
| `project_name` | string | Name of the project directory |
| `start_time` | string | Session start time (RFC3339) |
| `duration_minutes` | float | Elapsed session duration |
| `estimated_cost_usd` | float | Estimated spend for this session |
| `friction_score` | float | Friction intensity (higher is worse) |
| `model_breakdown` | object | Token counts keyed by model name |
| `total_tokens` | int | Sum of input and output tokens |
| `live` | bool | `true` when data read from active in-progress file |

### `get_session_dashboard`

Composite live-session tool. Returns all six live-session metrics in a single call: token velocity, commit-to-attempt ratio, context pressure, cost velocity, tool errors, and friction patterns. Designed to replace six individual tool calls with one round-trip. **Use this when the PostToolUse hook fires.**

**Input:** None

**Output:**

| Field | Type | Description |
|---|---|---|
| `token_velocity` | object | Tokens/min rate with `status` ("flowing", "slow", "idle") |
| `commit_ratio` | object | Git commits vs Edit/Write attempts with `status` ("efficient", "normal", "low") |
| `context_pressure` | object | Context window utilization with `status` ("comfortable", "filling", "pressure", "critical") |
| `cost_velocity` | object | Cost burn rate (last 10 min) in USD/min with `status` ("efficient", "normal", "burning") |
| `tool_errors` | object | Error rate, errors-by-tool breakdown, consecutive streak, severity |
| `friction` | object | Recent friction event stream with pattern summary |
| `active_time` | object | `active_minutes`, `idle_minutes`, `wall_clock_minutes`, `resumptions` |
| `drift_signal` | object | Drift detection with `status` ("exploring", "implementing", "drifting"). Omitted for pure research sessions. |

### `get_context_pressure`

Context window utilization for the current live session. Sums input and output tokens across all messages, counts compaction events, and estimates the usage ratio against the 200k context window.

**Input:** None

**Output:**

| Field | Type | Description |
|---|---|---|
| `used_tokens` | int | Estimated total tokens consumed in this context window |
| `window_tokens` | int | Context window size (200,000) |
| `utilization_pct` | float | Percentage of context window consumed |
| `compaction_count` | int | Number of compaction events detected |
| `status` | string | "comfortable" (<50%), "filling" (50–75%), "pressure" (75–90%), "critical" (≥90%) |

### `get_cost_velocity`

Cost burn rate for the current live session over a 10-minute sliding window. Computes per-minute USD spend from token counts and Sonnet 4 pricing.

**Input:** None

**Output:**

| Field | Type | Description |
|---|---|---|
| `cost_per_minute` | float | USD/min over the last 10 minutes |
| `window_minutes` | int | Window size used for the calculation |
| `status` | string | "efficient" (<$0.05/min), "normal" ($0.05–$0.20/min), "burning" (≥$0.20/min) |

### `get_drift_signal`

Drift signal detection for the current live session. Classifies the last 20 tool calls to determine whether you're actively implementing (writes present), exploring (no edits yet in session), or drifting (edits exist session-wide but recent window is ≥60% reads with zero writes). Use this to detect when a session has shifted from writing to reading-only, which may indicate being stuck or lost.

**Input:** None

**Output:**

| Field | Type | Description |
|---|---|---|
| `session_id` | string | Session identifier |
| `live` | bool | Always `true` (this tool only works on active sessions) |
| `window_n` | int | Number of tool calls analyzed (last 20) |
| `read_calls` | int | Count of Read/Grep/Glob calls in the window |
| `write_calls` | int | Count of Edit/Write calls in the window |
| `has_any_edit` | bool | Whether any Edit/Write exists anywhere in the session |
| `status` | string | "exploring" (no edits yet), "implementing" (writes in window), "drifting" (edits exist but window is read-heavy) |

**Use case:** Detect stuck sessions where you're reading repeatedly without writing. When `status` is `"drifting"`, consider whether you're gathering information that will lead to a write, or whether you're spinning without clear next steps.

### `get_cost_budget`

Returns today's spend against the configured daily budget.

**Input:** None

**Output:**

| Field | Type | Description |
|---|---|---|
| `today_usd` | float | Total spend across all sessions today |
| `budget_usd` | float | Configured daily limit (null if no budget set) |
| `remaining_usd` | float | Budget remaining today (null if no budget set) |
| `percent_used` | float | Fraction of daily budget consumed (null if no budget set) |

### `get_session_friction`

Returns friction counts for a specific session, broken down by type.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `session_id` | string | yes | Session ID from `get_session_stats` |

**Output:**

| Field | Type | Description |
|---|---|---|
| `session_id` | string | The requested session ID |
| `friction_counts` | object | Map of friction type to count |
| `total_friction` | int | Sum of all friction events in this session |
| `top_friction_type` | string | The friction type with the highest count |

A session with no recorded friction returns `total_friction: 0`. This is not an error—it means no friction facets were written for that session.

### `get_recent_sessions`

Returns a list of recent sessions with summary metrics.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `n` | int | no | Number of sessions to return. Default: 5. Max: 50. |

**Output:** Array of session objects:

| Field | Type | Description |
|---|---|---|
| `session_id` | string | Unique session identifier |
| `project_name` | string | Project directory name |
| `start_time` | string | Session start time (RFC3339) |
| `duration_minutes` | float | Session duration |
| `estimated_cost_usd` | float | Estimated session cost |
| `friction_score` | float | Friction intensity for the session |

## Project Calibration Tools

### `get_project_health`

Returns friction patterns, tool error rates, and agent performance for a single project.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `project` | string | no | Project name to query. Defaults to the most recent session's project. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `project` | string | Project name |
| `session_count` | int | Total sessions analyzed for this project |
| `friction_rate` | float | Fraction of sessions that had any friction |
| `top_friction_types` | string[] | Up to three most frequent friction types |
| `avg_tool_errors_per_session` | float | Mean tool errors per session |
| `zero_commit_rate` | float | Fraction of sessions that produced no git commits |
| `agent_success_rate` | float | Fraction of agent tasks that completed successfully |
| `has_claude_md` | bool | Whether a CLAUDE.md exists for this project |
| `agent_performance_by_type` | object | Per-type agent stats (count, success_rate, avg_duration_ms, avg_tokens) |

### `get_project_comparison`

Returns all known projects ranked by health score, enabling cross-project triage.

**Input:** None

**Output:** Array of project entries:

| Field | Type | Description |
|---|---|---|
| `project` | string | Project name |
| `session_count` | int | Total sessions analyzed |
| `health_score` | float | Composite score from 0 to 100 (higher is healthier) |
| `friction_rate` | float | Fraction of sessions with friction |
| `has_claude_md` | bool | Whether a CLAUDE.md exists |
| `agent_success_rate` | float | Agent task success rate |
| `zero_commit_rate` | float | Fraction of sessions with no commits |
| `top_friction_types` | string[] | Most frequent friction types |

Health score formula: `100 − friction penalty − zero-commit penalty + agent success bonus + CLAUDE.md bonus`, clamped to [0, 100]. Projects are returned in descending health score order.

## Improvement Guidance Tools

### `get_suggestions`

Returns ranked improvement suggestions based on session data. The engine evaluates seven rules covering CLAUDE.md gaps, recurring friction patterns, agent parallelization opportunities, and hook configuration.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `project` | string | no | Filter suggestions to a specific project. Returns suggestions for all projects if omitted. |
| `limit` | int | no | Maximum suggestions to return. Default: 5. Max: 20. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `suggestions` | array | Ranked list of suggestions |
| `total_count` | int | Total suggestions before the limit was applied |

Each suggestion:

| Field | Type | Description |
|---|---|---|
| `category` | string | Suggestion category (e.g., `claude_md`, `friction`, `agents`, `hooks`) |
| `priority` | int | Numeric priority (lower is higher priority) |
| `title` | string | Short description of the suggestion |
| `description` | string | Detailed explanation with recommended action |
| `impact_score` | float | Computed impact: `(affected_sessions × frequency × time_saved) / effort` |

Suggestions are sorted by `impact_score` descending. High-impact suggestions appear first regardless of category.

### `get_stale_patterns`

Returns friction types that have recurred across sessions without a corresponding CLAUDE.md update—indicating chronic problems that have not been addressed.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `threshold` | float | no | Minimum recurrence rate to include a pattern. Default: 0.3 (30% of sessions). |
| `lookback` | int | no | Number of recent sessions to analyze. Default: 10. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `patterns` | array | Stale friction patterns |
| `total_sessions` | int | Total sessions available for analysis |
| `window_sessions` | int | Sessions analyzed within the lookback window |

Each pattern:

| Field | Type | Description |
|---|---|---|
| `friction_type` | string | The friction category |
| `recurrence_rate` | float | Fraction of window sessions where this friction appeared |
| `session_count` | int | Number of sessions in the window with this friction |
| `last_claude_md_age` | int | Days since the last CLAUDE.md change for this project |
| `is_stale` | bool | True when recurrence_rate > threshold and no CLAUDE.md change was made in the window |

A pattern is stale if it appears in more than `threshold` of the lookback sessions and no CLAUDE.md update has been detected during that window. Stale patterns appear first in the response, sorted by recurrence rate descending.

## Performance Analytics Tools

### `get_agent_performance`

Returns aggregate performance metrics for all agent tasks across all sessions.

**Input:** None

**Output:**

| Field | Type | Description |
|---|---|---|
| `total_agents` | int | Total agent task count |
| `success_rate` | float | Fraction of agents with `completed` status |
| `kill_rate` | float | Fraction of agents stopped via TaskStop |
| `background_ratio` | float | Fraction of agents that ran in background mode |
| `avg_duration_ms` | float | Mean agent task duration in milliseconds |
| `avg_tokens_per_agent` | float | Mean token consumption per agent task |
| `parallel_sessions` | int | Sessions that ran two or more concurrent agents |
| `by_type` | object | Per-agent-type breakdown |

Each entry in `by_type` is keyed by agent type string and contains:

| Field | Type | Description |
|---|---|---|
| `count` | int | Number of agents of this type |
| `success_rate` | float | Completion rate for this type |
| `avg_duration_ms` | float | Mean duration for this type |
| `avg_tokens` | float | Mean token usage for this type |

### `get_effectiveness`

Returns before/after comparisons for projects where a CLAUDE.md change was detected, showing whether the change produced measurable improvement.

**Input:** None

**Output:** Array of project effectiveness entries. Only projects with a CLAUDE.md and sufficient session data on both sides of the change are included.

| Field | Type | Description |
|---|---|---|
| `project` | string | Project name |
| `verdict` | string | `effective`, `neutral`, or `regression` |
| `score` | float | Effectiveness score from 0 to 100 |
| `friction_delta` | float | Change in friction rate (negative means improvement) |
| `tool_error_delta` | float | Change in tool error rate (negative means improvement) |
| `before_sessions` | int | Sessions analyzed before the CLAUDE.md change |
| `after_sessions` | int | Sessions analyzed after the CLAUDE.md change |
| `change_detected_at` | string | Timestamp when the CLAUDE.md modification was detected |

### `get_cost_summary`

Returns spend totals across time horizons and a per-project cost breakdown.

**Input:** None

**Output:**

| Field | Type | Description |
|---|---|---|
| `today_usd` | float | Spend across all sessions today |
| `week_usd` | float | Spend over the last 7 days |
| `all_time_usd` | float | Total spend across all recorded sessions |
| `by_project` | array | Per-project spend, sorted by total_usd descending |

Each entry in `by_project`:

| Field | Type | Description |
|---|---|---|
| `project` | string | Project name |
| `total_usd` | float | Total spend for this project |
| `sessions` | int | Number of sessions contributing to this spend |

## SAW Observability Tools

SAW (Scout-and-Wave) is a parallel agent workflow pattern where a scout agent identifies work items and wave agents execute them concurrently. These tools expose timing and status data for SAW sessions.

### `get_saw_sessions`

Returns recent sessions that used Scout-and-Wave parallel agents.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `n` | int | no | Number of sessions to return. Default: 5. |

**Output:** Array of SAW session summaries:

| Field | Type | Description |
|---|---|---|
| `session_id` | string | Session identifier |
| `project_name` | string | Project directory name |
| `wave_count` | int | Number of waves executed in this session |
| `agent_count` | int | Total agents across all waves |
| `start_time` | string | Session start time (RFC3339) |

### `get_saw_wave_breakdown`

Returns per-wave timing and per-agent status for a single SAW session.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `session_id` | string | yes | Session ID from `get_saw_sessions` |

**Output:**

| Field | Type | Description |
|---|---|---|
| `waves` | array | Ordered list of wave execution details |

Each wave:

| Field | Type | Description |
|---|---|---|
| `wave_number` | int | Wave index (1-based) |
| `duration_ms` | int | Total duration of this wave in milliseconds |
| `agents` | array | Agents that ran in this wave |

Each agent within a wave:

| Field | Type | Description |
|---|---|---|
| `agent_id` | string | Unique agent identifier |
| `type` | string | Agent type string |
| `status` | string | `completed`, `killed`, or `failed` |
| `duration_ms` | int | Agent task duration in milliseconds |
| `tokens` | int | Total tokens consumed by this agent |

## Multi-Project Analysis Tools

### `get_session_projects`

Returns weighted per-repo breakdown for sessions that touch multiple repositories. Shows how cost and activity are distributed across projects based on which files were actually edited or read. All project-scoped tools (like `get_project_health`, `get_cost_summary`) now use this weighted attribution automatically to route multi-repo sessions to their dominant project.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `session_id` | string | no | Session ID to analyze (defaults to active/most recent session) |

**Output:**

| Field | Type | Description |
|---|---|---|
| `session_id` | string | Session identifier |
| `primary_project` | string | Dominant project by weighted activity |
| `projects` | array | Weighted breakdown of all repos touched |
| `live` | bool | Whether this is an active session |

Each project in the `projects` array:

| Field | Type | Description |
|---|---|---|
| `project` | string | Project name (repo basename) |
| `repo_root` | string | Absolute path to repo root |
| `weight` | float | Proportional share of activity (0.0–1.0) |
| `tool_calls` | int | Number of tool calls that touched files in this repo |

**How weights are computed:** The tool extracts file paths from `Read`, `Edit`, `Write`, and `Bash` tool_use entries in the transcript, maps each path to its repo root using `git rev-parse --show-toplevel`, and computes proportional weights based on tool call counts per repo.

**Use case:** "Which repos dominated this session's cost? Was this primarily claudewatch work or commitmux work?"

### `get_causal_insights`

Correlates session attributes against outcomes to identify what predicts success or failure. Answers questions like: "Does having CLAUDE.md reduce friction?" or "Do SAW sessions commit more than non-SAW sessions?" Returns Pearson correlation coefficients for numeric factors and grouped comparison stats for boolean factors.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `outcome` | string | yes | Outcome metric to analyze |
| `factor` | string | no | Specific factor to analyze (omit to analyze all factors) |
| `project` | string | no | Filter to specific project by name |

**Outcome values:** `friction`, `commits`, `zero_commit`, `cost`, `duration`, `tool_errors`

**Factor values:** `has_claude_md`, `uses_task_agent`, `uses_mcp`, `uses_web_search`, `is_saw`, `tool_call_count`, `duration`, `input_tokens`

**Output:** Returns correlation analysis with:
- For **numeric factors** (tool_call_count, duration, input_tokens): Pearson correlation coefficient, p-value, n
- For **boolean factors** (has_claude_md, uses_task_agent, etc.): Mean outcome for true vs false groups, delta, n for each group

Groups with n < 10 sessions are flagged as **low-confidence**.

**Use case:** "Find which session attributes correlate strongest with low friction. Does SAW reduce zero-commit rate? Does MCP usage increase cost?"

**Example:** To check if having CLAUDE.md reduces friction:
```json
{
  "outcome": "friction",
  "factor": "has_claude_md"
}
```

Returns grouped comparison showing mean friction for sessions with vs without CLAUDE.md.

## AI Ops Tools

### `search_transcripts`

Full-text search over all indexed session transcript entries. Searches the FTS5 index built from JSONL transcript files. Returns matching entries with session ID, type, timestamp, and highlighted snippet.

If the transcript index is empty, returns a user-friendly error directing you to run `claudewatch search <query>` first (which builds the index automatically). The MCP handler does not auto-index—indexing is a slow CLI operation that should be initiated explicitly.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | Full-text search query. FTS5 operators supported (e.g. `"error" AND "build"`). |
| `limit` | int | no | Maximum results to return. Default: 20. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `count` | int | Number of results returned |
| `indexed_count` | int | Total entries in the transcript index |
| `results` | array | Matching entries |

Each result:

| Field | Type | Description |
|---|---|---|
| `session_id` | string | Session the entry came from |
| `project_hash` | string | Project identifier for the session |
| `entry_type` | string | Entry type (e.g. `message`, `tool_use`) |
| `timestamp` | string | Entry timestamp (RFC3339) |
| `snippet` | string | Highlighted excerpt showing where the query matched |
| `rank` | float | FTS5 relevance rank (lower is more relevant) |

### `get_project_anomalies`

Detects anomalous sessions for a project using per-project z-score baselines. On first call for a project, computes and persists a baseline automatically from all available sessions. Subsequent calls load the stored baseline. **Baselines auto-refresh on every run using exponential decay weighting (decay=0.9), so recent sessions have more influence than older ones.** Baseline drift after workflow changes (e.g., adopting SAW) resolves automatically within ~10-15 sessions.

Project resolution follows the same pattern as `get_project_health`: active session's project is used if no `project` arg is provided, falling back to the most recent closed session.

Returns an error if fewer than 3 sessions exist for the project (insufficient data to compute a meaningful baseline).

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `project` | string | no | Project name. Defaults to the active session's project. |
| `threshold` | float | no | Z-score threshold for anomaly detection. Default: 2.0. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `project` | string | Project name used for the query |
| `baseline` | object | Per-project baseline stats (omitted if not yet computed) |
| `anomalies` | array | Sessions with z-scores exceeding the threshold |

`baseline` fields:

| Field | Type | Description |
|---|---|---|
| `avg_cost_usd` | float | Mean session cost across all sessions |
| `stddev_cost_usd` | float | Population standard deviation of session cost |
| `avg_friction` | float | Mean friction score |
| `stddev_friction` | float | Population standard deviation of friction score |
| `avg_commits` | float | Mean commits per session |
| `saw_session_frac` | float | Fraction of sessions that used SAW parallel agents |

Each anomaly:

| Field | Type | Description |
|---|---|---|
| `session_id` | string | Anomalous session identifier |
| `start_time` | string | Session start time (RFC3339) |
| `cost_usd` | float | Actual session cost |
| `friction_score` | float | Actual friction score |
| `cost_z` | float | Z-score for cost (positive = above average) |
| `friction_z` | float | Z-score for friction (positive = above average) |
| `severity` | string | `warning` (z ≥ threshold) or `critical` (z ≥ 3× threshold) |
| `reason` | string | Human-readable explanation of which signal triggered the anomaly |

### `get_cost_attribution`

Breaks down token cost by tool type for a session. Answers "which tool calls consumed most of my budget this session?"

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `session_id` | string | no | Session to analyze. Defaults to the most recent session if omitted. |
| `project` | string | no | Reserved for future filtering. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `session_id` | string | The session analyzed |
| `rows` | array | Per-tool-type cost breakdown |
| `total_cost_usd` | float | Total estimated cost for the session |

Each entry in `rows`:

| Field | Type | Description |
|---|---|---|
| `tool_type` | string | Tool type name (e.g. `Agent`, `Bash`, `Read`, `Edit`) |
| `calls` | int | Number of calls of this tool type in the session |
| `input_tokens` | int | Input tokens consumed by this tool type |
| `output_tokens` | int | Output tokens consumed by this tool type |
| `est_cost_usd` | float | Estimated cost for this tool type |

### `get_regression_status`

Checks whether a project's friction rate or average cost per session has regressed relative to its stored baseline. The comparison uses a configurable multiplier threshold (default 1.5×)—regression is flagged when the current value exceeds `threshold × baseline_value`.

Returns `has_baseline: false` if no baseline exists for the project (run `claudewatch anomalies` to compute one). Returns `insufficient_data: true` if fewer than 3 recent sessions are available.

**Input:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `project` | string | no | Project name to check. Defaults to the most recently active project. |
| `threshold` | float | no | Regression multiplier. Default: 1.5. Values ≤ 1 are treated as 1.5. |

**Output:**

| Field | Type | Description |
|---|---|---|
| `project` | string | Project name checked |
| `has_baseline` | bool | Whether a stored baseline exists for this project |
| `insufficient_data` | bool | True if fewer than 3 recent sessions are available |
| `regressed` | bool | True if either friction or cost has regressed |
| `friction_regressed` | bool | True if friction rate exceeds `threshold × baseline_friction_rate` |
| `cost_regressed` | bool | True if avg cost exceeds `threshold × baseline_avg_cost_usd` |
| `current_friction_rate` | float | Current friction rate (sessions with friction / total, 0.0–1.0) |
| `baseline_friction_rate` | float | Baseline friction rate |
| `current_avg_cost_usd` | float | Current average session cost |
| `baseline_avg_cost_usd` | float | Baseline average session cost |
| `threshold` | float | Threshold used for comparison |
| `message` | string | Human-readable summary of the regression status |

## Data Freshness

All tools read from `~/.claude/` at call time. There is no cache layer—each tool call reflects the current state of the data files on disk. This means a `get_session_stats` call made at the start of the session and another made twenty minutes later will return different values as the session accumulates cost and tokens.

The MCP server process runs for the lifetime of the Claude Code session. Restart Claude Code after installing a new claudewatch binary to pick up changes to the server implementation.

## What the MCP Cannot Do

The MCP server is strictly read-only. It reads data from `~/.claude/` at call time but has no write paths—it cannot modify sessions, update CLAUDE.md files, inject instructions, or communicate with other Claude instances. It makes no network calls; all data is local. The server has no mechanism to push information to other sessions or persist state between calls beyond what the data files already contain.

## Related Documentation

- [Hooks](/docs/features/HOOKS.md) - Push-based automatic intervention
- [CLI Commands](/docs/features/CLI.md) - Human-facing analytics and improvement tools
- [MCP Integration](/docs/technical/MCP_INTEGRATION.md) - Server lifecycle and tool design
