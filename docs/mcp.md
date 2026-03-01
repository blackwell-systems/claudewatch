Claude has no native access to its own session history, token spend, or behavioral patterns. It operates inside a context window without knowledge of what happened in prior sessions, which friction types it has generated repeatedly, or how much of today's budget has already been consumed. claudewatch closes that gap. The raw data — JSONL transcripts, session metadata, usage facets — lives in `~/.claude/` and requires domain knowledge to parse correctly. The MCP server reads that data and surfaces it as structured, queryable tools that Claude can call from inside the session where decisions are being made.

This is different from the CLI commands. `claudewatch health` or `claudewatch friction` are retrospective: a developer reviews output after the fact, outside the session. The MCP server creates a feedback loop inside the session itself. Claude can check the project's friction history before choosing an approach, verify it has budget headroom before spinning up parallel agents, or confirm that a CLAUDE.md change actually shifted outcomes — all without leaving the conversation.

The result is dual observability: developers get the CLI for review and configuration; Claude gets the MCP for real-time self-awareness. Both layers read the same underlying data, written by Claude Code as it works.

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

`--budget` sets a daily spend limit in USD used by `get_cost_budget` to compute remaining headroom and alert when the limit is approached. Omit the flag to disable budget tracking — `get_cost_budget` will still return today's spend without a limit comparison.

Restart Claude Code after installing a new claudewatch binary to pick up changes. The MCP server process runs for the lifetime of the Claude Code session; configuration changes take effect at the next session start.

## Recommended usage pattern

### Session start

Call these before making approach decisions:

1. **`get_project_health`** — understand this project's friction history and agent success patterns before deciding how to work
2. **`get_project_comparison`** — if you're choosing which project needs attention, see all projects ranked by health score at once
3. **`get_cost_budget`** — check available budget before committing to a token-intensive approach or parallel agent strategy

### Mid-session

Call these when making decisions about what to do next:

4. **`get_session_stats`** — current session cost and token usage
5. **`get_session_friction`** — what friction has already occurred this session, so you can adjust
6. **`get_suggestions`** — ranked improvement opportunities if you're deciding what to tackle

### Review and reflection

Call these to understand patterns over time or validate prior changes:

7. **`get_agent_performance`** — which agent types work best for this project's sessions
8. **`get_effectiveness`** — whether CLAUDE.md changes produced measurable before/after improvement
9. **`get_cost_summary`** — where budget is going across all projects
10. **`get_stale_patterns`** — friction types that have recurred without any CLAUDE.md response
11. **`get_saw_sessions`** + **`get_saw_wave_breakdown`** — Scout-and-Wave parallel agent workflow timing and status

## Tool reference

### Session awareness

#### `get_session_stats`

Returns metrics for the current active session: cost, duration, token usage, and friction score. When the session JSONL file is actively open (detected via `lsof` or recent mtime), the response includes live token and cost data read directly from the in-progress transcript.

No input parameters.

| Output field | Type | Description |
|---|---|---|
| `session_id` | string | Unique session identifier |
| `project_name` | string | Name of the project directory |
| `start_time` | string | Session start time (RFC3339) |
| `duration_minutes` | float | Elapsed session duration |
| `estimated_cost_usd` | float | Estimated spend for this session |
| `friction_score` | float | Friction intensity for this session (higher is worse) |
| `model_breakdown` | object | Token counts keyed by model name |
| `total_tokens` | int | Sum of input and output tokens |
| `live` | bool | `true` when data was read from the active in-progress session file |

---

#### `get_cost_budget`

Returns today's spend against the configured daily budget.

No input parameters.

| Output field | Type | Description |
|---|---|---|
| `today_usd` | float | Total spend across all sessions today |
| `budget_usd` | float | Configured daily limit (null if no budget set) |
| `remaining_usd` | float | Budget remaining today (null if no budget set) |
| `percent_used` | float | Fraction of daily budget consumed (null if no budget set) |

---

#### `get_session_friction`

Returns friction counts for a specific session, broken down by type.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `session_id` | string | yes | Session ID from `get_session_stats` |

| Output field | Type | Description |
|---|---|---|
| `session_id` | string | The requested session ID |
| `friction_counts` | object | Map of friction type to count |
| `total_friction` | int | Sum of all friction events in this session |
| `top_friction_type` | string | The friction type with the highest count |

A session with no recorded friction returns `total_friction: 0`. This is not an error — it means no friction facets were written for that session.

---

#### `get_recent_sessions`

Returns a list of recent sessions with summary metrics.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `n` | int | no | Number of sessions to return. Default: 5. Max: 50. |

Returns an array of session objects:

| Field | Type | Description |
|---|---|---|
| `session_id` | string | Unique session identifier |
| `project_name` | string | Project directory name |
| `start_time` | string | Session start time (RFC3339) |
| `duration_minutes` | float | Session duration |
| `estimated_cost_usd` | float | Estimated session cost |
| `friction_score` | float | Friction intensity for the session |

---

### Project calibration

#### `get_project_health`

Returns friction patterns, tool error rates, and agent performance for a single project.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `project` | string | no | Project name to query. Defaults to the most recent session's project. |

| Output field | Type | Description |
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

---

#### `get_project_comparison`

Returns all known projects ranked by health score, enabling cross-project triage.

No input parameters.

Returns an array of project entries:

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

---

### Improvement guidance

#### `get_suggestions`

Returns ranked improvement suggestions based on session data. The engine evaluates seven rules covering CLAUDE.md gaps, recurring friction patterns, agent parallelization opportunities, and hook configuration.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `project` | string | no | Filter suggestions to a specific project. Returns suggestions for all projects if omitted. |
| `limit` | int | no | Maximum suggestions to return. Default: 5. Max: 20. |

| Output field | Type | Description |
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

---

#### `get_stale_patterns`

Returns friction types that have recurred across sessions without a corresponding CLAUDE.md update — indicating chronic problems that have not been addressed.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `threshold` | float | no | Minimum recurrence rate to include a pattern. Default: 0.3 (30% of sessions). |
| `lookback` | int | no | Number of recent sessions to analyze. Default: 10. |

| Output field | Type | Description |
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

---

### Performance analytics

#### `get_agent_performance`

Returns aggregate performance metrics for all agent tasks across all sessions.

No input parameters.

| Output field | Type | Description |
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

---

#### `get_effectiveness`

Returns before/after comparisons for projects where a CLAUDE.md change was detected, showing whether the change produced measurable improvement.

No input parameters.

Returns an array of project effectiveness entries. Only projects with a CLAUDE.md and sufficient session data on both sides of the change are included.

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

---

#### `get_cost_summary`

Returns spend totals across time horizons and a per-project cost breakdown.

No input parameters.

| Output field | Type | Description |
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

---

### SAW observability

SAW (Scout-and-Wave) is a parallel agent workflow pattern where a scout agent identifies work items and wave agents execute them concurrently. These tools expose timing and status data for SAW sessions.

#### `get_saw_sessions`

Returns recent sessions that used Scout-and-Wave parallel agents.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `n` | int | no | Number of sessions to return. Default: 5. |

Returns an array of SAW session summaries:

| Field | Type | Description |
|---|---|---|
| `session_id` | string | Session identifier |
| `project_name` | string | Project directory name |
| `wave_count` | int | Number of waves executed in this session |
| `agent_count` | int | Total agents across all waves |
| `start_time` | string | Session start time (RFC3339) |

---

#### `get_saw_wave_breakdown`

Returns per-wave timing and per-agent status for a single SAW session.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `session_id` | string | yes | Session ID from `get_saw_sessions` |

| Output field | Type | Description |
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

## Data freshness

All tools read from `~/.claude/` at call time. There is no cache layer — each tool call reflects the current state of the data files on disk. This means a `get_session_stats` call made at the start of the session and another made twenty minutes later will return different values as the session accumulates cost and tokens.

The MCP server process runs for the lifetime of the Claude Code session. Restart Claude Code after installing a new claudewatch binary to pick up changes to the server implementation.

## What the MCP cannot do

The MCP server is strictly read-only. It reads data from `~/.claude/` at call time but has no write paths — it cannot modify sessions, update CLAUDE.md files, inject instructions, or communicate with other Claude instances. It makes no network calls; all data is local. The server has no mechanism to push information to other sessions or persist state between calls beyond what the data files already contain.
