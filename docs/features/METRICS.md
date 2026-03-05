# Metrics: Friction Analysis and Cost-Per-Outcome

Metrics are how you measure what's actually happening in your AI workflow. Not guesses, not feelings—quantified patterns derived from session data. claudewatch computes friction, velocity, efficiency, cost-per-outcome, and effectiveness scoring to answer: "Is this working, and how do I make it better?"

## What Metrics Provide

Unlike hooks that alert in real-time or memory that stores task history, metrics give you **aggregate analysis over time**. They answer questions like:

- "What percentage of sessions hit friction?"
- "How much does a commit cost me on average?"
- "Did that CLAUDE.md change actually reduce errors?"
- "Which tool types burn the most tokens?"

This is the **measurement** half of the fix-measure loop. You make a change, work for a week, then run `claudewatch metrics` to see if it worked.

## Friction Scoring

Friction is any session event that represents wasted effort, rework, or developer intervention.

### What Counts as Friction

claudewatch defines friction as one of these event types:

| Friction Type | Definition | Example |
|---|---|---|
| `retry:Read` | Read tool fails, retried by Claude | File not found, wrong path, permission denied |
| `retry:Edit` | Edit tool fails, retried by Claude | Old string not found, indentation mismatch |
| `retry:Bash` | Bash command fails, retried by Claude | Command not found, non-zero exit code |
| `retry:Write` | Write tool fails, retried by Claude | Permission denied, path does not exist |
| `tool_error` | Any tool returns error status | Grep pattern invalid, glob failed |
| `correction` | User corrects Claude mid-session | "No, do X instead", "That's wrong, try Y" |
| `wrong_approach` | Claude pivots after user feedback | User says "wrong direction", Claude backtracks |
| `buggy_code` | Code written, then fixed in same session | Syntax error, logic bug, missing import |
| `unclear_requirements` | Claude asks clarifying questions | "Do you mean X or Y?", "Which file?" |

### Friction Rate

**Definition:** Fraction of sessions that had **any** friction.

```
friction_rate = sessions_with_friction / total_sessions
```

- 0.0 = No sessions hit friction (perfect)
- 0.2 = 20% of sessions hit friction (good)
- 0.5 = 50% of sessions hit friction (moderate, room for improvement)
- 0.8 = 80% of sessions hit friction (high, urgent improvements needed)

**Why it matters:** Friction rate is the single best health metric. It captures "how often does work go sideways?"

**Target:** <30% for mature projects, <50% for new projects.

### Friction by Type

Not all friction is equally costly. `retry:Bash` (command fails, retry takes 5 seconds) is cheaper than `wrong_approach` (entire feature pivoted, 20 minutes wasted).

**Top friction types** tell you where to focus:

```
Top Friction Types:
  retry:Bash          32% of friction events
  correction          18%
  retry:Read          15%
  tool_error          12%
  wrong_approach      10%
  (others)            13%
```

**If `retry:Bash` dominates:** Add pre-edit hooks or testing guidance to CLAUDE.md.

**If `correction` dominates:** Requirements are unclear or Claude is guessing. Add task breakdown or verification steps.

**If `wrong_approach` dominates:** Architectural guidance is missing. Add "How to approach X" sections to CLAUDE.md.

### Friction Intensity

**Definition:** Average friction events per session (for sessions that had friction).

```
friction_intensity = total_friction_events / sessions_with_friction
```

- 1.0 = Sessions hit one friction event and move on (low intensity)
- 3.0 = Sessions hit three friction events (moderate intensity)
- 10.0 = Sessions hit ten friction events (high intensity, stuck loops)

**Why it matters:** Low friction rate + high intensity = occasional disasters. High friction rate + low intensity = frequent but minor issues.

**Target:** <2.0 intensity (friction happens, but resolved quickly).

## Session Trends

### Cost Per Session

**Definition:** Average estimated spend per session.

```
avg_cost_per_session = total_cost_usd / total_sessions
```

Cost is computed from token usage (input + output + cache hits) using model-specific pricing:

- Sonnet 4: $3/MTok input, $15/MTok output, cache hits at 10% of input rate
- Opus 4: Higher rates (varies by model)
- Haiku: Lower rates (varies by model)

**Why it matters:** High cost per session means either:
1. Sessions are long (not necessarily bad)
2. Token usage is inefficient (lots of retries, reads, drift)

**Target:** <$1 for typical sessions, <$5 for complex multi-agent sessions.

### Cost Per Commit

**Definition:** Average cost to produce one git commit.

```
cost_per_commit = total_cost_usd / total_commits
```

This is **cost-per-outcome**, not just cost-per-session. A session that costs $2 and produces 2 commits is more efficient than a session that costs $1 and produces 0 commits.

**Why it matters:** This is the metric that matters for productivity. You're not buying sessions, you're buying commits.

**Target:** <$0.50 per commit for routine changes, <$2 per commit for complex features.

### Zero-Commit Rate

**Definition:** Fraction of sessions that produced no git commits.

```
zero_commit_rate = sessions_with_zero_commits / total_sessions
```

- 0.0 = Every session produced commits (perfect)
- 0.2 = 20% of sessions produced no commits (good, some exploration is expected)
- 0.5 = 50% of sessions produced no commits (moderate, lots of research or stuck work)
- 0.8 = 80% of sessions produced no commits (high, something is wrong)

**Why it matters:** Zero-commit sessions are not always waste (research, planning, debugging), but a high rate indicates friction, stuck loops, or unclear goals.

**Exceptions:** Research sessions, documentation reviews, planning sessions. Use `claudewatch experiment` to tag these explicitly.

**Target:** <30% for implementation-focused projects, <50% for research-heavy projects.

### Commits Per Session

**Definition:** Average number of git commits per session.

```
commits_per_session = total_commits / total_sessions
```

**Why it matters:** Low commits/session + high cost/session = expensive sessions with little output. High commits/session = productive, incremental work.

**Target:** >1.5 commits/session for mature projects.

## Tool Error Patterns

### Error Rate

**Definition:** Tool calls that returned errors.

```
tool_error_rate = errored_tool_calls / total_tool_calls
```

**Why it matters:** High error rates indicate guessing (wrong paths, commands, patterns) or missing context.

**Target:** <5% for mature projects, <10% for new projects.

### Errors by Tool Type

Not all tools have equal error rates. `Bash` and `Grep` are inherently more error-prone than `Read` or `Edit`.

**Example breakdown:**

```
Tool Errors by Type:
  Bash      45 errors (32% of all tool errors)
  Read      28 errors (20%)
  Grep      18 errors (13%)
  Edit      15 errors (11%)
  (others)  34 errors (24%)
```

**If `Bash` dominates:** Add pre-edit testing hooks, or CLAUDE.md guidance on verifying commands.

**If `Read` dominates:** Path guessing is common. Add file tree context or "Use absolute paths" rule.

**If `Edit` dominates:** String matching issues. Add "Read before Edit" rule.

### Consecutive Error Streaks

**Definition:** Longest run of consecutive tool errors in a session.

```
max_consecutive_errors = max(streak_length) across all sessions
```

**Why it matters:** Long streaks indicate stuck loops. Claude retries the same approach without adjusting.

**Target:** <3 consecutive errors (PostToolUse hook fires at 3 to break the loop).

## Cost-Per-Outcome Metrics

### Cache-Adjusted Cost

claudewatch computes cost using **prompt caching rates** when cache hits are detected:

- Cache hits: charged at 10% of input token rate ($0.30/MTok vs $3/MTok for Sonnet 4)
- Cache writes: not separately charged (included in initial input rate)

**Why it matters:** Without cache adjustment, cost metrics overestimate spend. Real spend is lower when Claude reuses large contexts (CLAUDE.md, file reads).

**Example:**

- Session has 500k input tokens
- 400k are cache hits (80% hit rate)
- Cost without caching: 500k * $3/MTok = $1.50
- Cost with caching: (100k * $3/MTok) + (400k * $0.30/MTok) = $0.42

Cache-adjusted cost: **$0.42**, not $1.50.

### Model-Specific Cost Breakdown

Sessions may use multiple models (Sonnet for most work, Opus for complex planning, Haiku for quick checks).

**Example breakdown:**

```
Model Usage:
  sonnet-4.5     $4.20 (70% of spend, 60% of tokens)
  opus-4         $1.80 (30% of spend, 10% of tokens)
  haiku-3.5      $0.05 (<1% of spend, 30% of tokens)
```

**Why it matters:** If Opus usage is high, you may be using expensive models for routine work. Shift to Sonnet where possible.

**Savings potential:**

```
If all Opus usage moved to Sonnet:
  Current: $6.05 total
  Potential: $4.50 total
  Savings: $1.55 (26%)
```

This is not always actionable (some tasks require Opus), but it surfaces the tradeoff.

### Tool-Type Cost Attribution

Token spend is not uniform. Agent tool calls (launching subagents) consume more tokens per call than Read tool calls.

**Example breakdown:**

```
Tool Type Attribution:
  Agent       $2.40 (40% of spend, 3 calls)
  Read        $1.20 (20% of spend, 45 calls)
  Edit        $0.90 (15% of spend, 18 calls)
  Bash        $0.80 (13% of spend, 22 calls)
  (others)    $0.70 (12% of spend, 50 calls)
```

**Use case:** After a high-cost session, call `claudewatch attribute` to see which tool types drove spending. If Agent tool calls dominate, consider whether those agents were necessary or if synchronous work would suffice.

## Stale Patterns Detection

**Definition:** Friction types that recur across sessions without a corresponding CLAUDE.md update.

A pattern is **stale** when:
1. It appears in >30% of the lookback window (default: last 10 sessions)
2. CLAUDE.md has not been updated in the past 14 days

**Why it matters:** Stale patterns indicate **chronic problems you're ignoring**. They happen repeatedly, but you haven't addressed the root cause.

**Example:**

```
Stale Patterns:
  retry:Bash (appears in 55% of last 10 sessions)
    Last CLAUDE.md update: 21 days ago
    Issue: "go vet" errors recur without pre-edit hook

  wrong_approach (appears in 40% of last 10 sessions)
    Last CLAUDE.md update: 21 days ago
    Issue: Architectural guidance missing for agent workflows
```

**Action:** Update CLAUDE.md with guidance to prevent recurrence, or run `claudewatch fix` to auto-generate patches.

**MCP tool:** `get_stale_patterns` surfaces this inside sessions.

**CLI command:** `claudewatch gaps` surfaces stale patterns alongside other structural gaps.

## Effectiveness Scoring

**Definition:** Before/after comparison for projects where a CLAUDE.md change was detected.

Effectiveness scoring answers: "Did that CLAUDE.md change actually help?"

### How It Works

1. Detect CLAUDE.md file modification timestamp
2. Split sessions into "before" and "after" groups
3. Compute metrics for each group: friction rate, tool error rate, zero-commit rate
4. Compare deltas

**Verdict:**

- **Effective**: Friction rate dropped by >10% AND tool error rate dropped
- **Regression**: Friction rate increased by >10% OR cost per session increased by >50%
- **Neutral**: Changes are within noise (±10%)

### Example Output

```
Effectiveness: myproject
  Verdict: effective
  Score: 78/100

  Before (15 sessions):
    Friction rate:      0.60 (60%)
    Tool error rate:    0.08 (8%)
    Zero-commit rate:   0.40 (40%)

  After (12 sessions):
    Friction rate:      0.42 (42%)  ← -18% (improvement)
    Tool error rate:    0.05 (5%)   ← -38% (improvement)
    Zero-commit rate:   0.25 (25%)  ← -38% (improvement)

  Change detected: 2026-02-20 10:15:00
```

**Why it matters:** Without effectiveness scoring, you guess whether changes helped. With it, you **measure** improvement.

**MCP tool:** `get_effectiveness` surfaces this inside sessions so Claude can reason about its own improvement trajectory.

## Velocity and Efficiency

### Read/Write Ratio

**Definition:** Ratio of Read/Grep/Glob tool calls to Edit/Write tool calls.

```
read_write_ratio = read_calls / write_calls
```

- 1.0 = Equal reads and writes (balanced exploration and implementation)
- 3.0 = 3× more reads than writes (research-heavy or uncertain)
- 10.0 = 10× more reads than writes (stuck, drifting, or avoiding implementation)

**Why it matters:** High read/write ratios indicate drift. Claude is exploring without implementing.

**Target:** 2.0-5.0 for typical sessions (read to understand, then write).

**MCP tool:** `get_drift_signal` detects high read/write ratios in the last 20 tool calls and alerts when >60% reads with 0 writes.

### Session Duration

**Definition:** Wall-clock time from session start to session end.

**Why it matters:** Long sessions may indicate:
1. Complex work (good)
2. Stuck loops (bad)
3. Context thrashing (bad)

Correlate duration with commits and cost:
- Long session + many commits + low cost = productive
- Long session + zero commits + high cost = stuck

**Target:** <30 minutes for routine changes, <2 hours for complex features.

### Active vs Idle Time

**Definition:** Time with tool activity vs time without.

claudewatch tracks:
- **Active time**: Spans where tool calls occur
- **Idle time**: Gaps >5 minutes between tool calls
- **Resumptions**: Sessions that were paused and resumed (idle gaps >15 minutes)

**Why it matters:** High idle time in a long session means the session was interrupted, not continuously stuck.

## Agent Performance Metrics

Agent performance is covered in detail in [Agent Analytics](/docs/features/AGENTS.md), but metrics include:

- **Success rate** - Fraction of agents with `completed` status
- **Kill rate** - Fraction of agents stopped via TaskStop
- **Average duration** - Mean agent task duration in milliseconds
- **Average tokens per agent** - Mean token consumption per agent task
- **Parallel sessions** - Sessions that ran 2+ concurrent agents
- **By-type breakdown** - Per-agent-type success rate, duration, tokens

These are computed from session transcripts by parsing agent spans (Task tool_use → tool_result pairs).

## CLI Usage

### `claudewatch metrics`

Comprehensive metrics report over a configurable time window.

```bash
claudewatch metrics
claudewatch metrics --days 7
claudewatch metrics --days 30 --json
```

**Flags:**

| Flag | Default | Description |
|---|---|---|
| `--days <n>` | 30 | Lookback window in days |
| `--json` | false | Full JSON export |

**Output sections:**

- **Session Trends** - Friction rate, cost/session, commits/session
- **Tool Usage** - Breakdown by tool type and frequency
- **Agent Performance** - By type: success rate, average duration, kill rate
- **Token Usage** - Cache hit rate, input/output ratio, per-session averages
- **Model Usage** - Per-model cost and token breakdown, spend percentages, savings potential
- **Project Confidence** - Read vs write ratio per project, low-confidence warnings
- **Friction Trends** - Friction rate over time, top types, intensity
- **Cost Per Outcome** - Cost per commit, zero-commit rate
- **Effectiveness** - Before/after CLAUDE.md change comparisons
- **Planning** - Task breakdown patterns, file churn analysis

**JSON sections** (with `--json`):

```json
{
  "velocity": {...},
  "efficiency": {...},
  "satisfaction": {...},
  "agents": {...},
  "tokens": {...},
  "models": {...},
  "commits": {...},
  "conversation": {...},
  "confidence": {...},
  "friction_trends": {...},
  "cost_per_outcome": {...},
  "effectiveness": {...},
  "planning": {...}
}
```

### `claudewatch gaps`

Surfaces what is structurally missing: projects without CLAUDE.md, stale friction patterns, high-friction commands without guidance.

```bash
claudewatch gaps
claudewatch gaps --json
```

Faster than `metrics` because it reads only metadata and facets, not full transcripts.

**Output categories:**

- **Context gaps** - Projects without CLAUDE.md
- **Hook gaps** - Sessions where hooks were not active
- **Stale patterns** - Friction types recurring without CLAUDE.md updates
- **High-friction commands** - Bash commands with >30% error rate

### `claudewatch suggest`

Ranked improvement suggestions with impact scores.

```bash
claudewatch suggest
claudewatch suggest --limit 10
claudewatch suggest --project myproject
```

Suggestions are derived from metrics: missing CLAUDE.md, stale patterns, low agent success rates, parallelization opportunities.

**Impact score formula:**

```
impact = (affected_sessions * frequency * time_saved) / effort_minutes
```

Higher impact = more value to address.

### `claudewatch track`

Snapshot current metrics to SQLite database, then diff against previous snapshots.

```bash
claudewatch track              # snapshot
claudewatch track --compare    # diff against previous
```

**Output with `--compare`:**

```
Metrics Comparison
────────────────────────────────────────
Metric                  Before    After     Delta
Friction rate           0.60      0.42      -18% ↓
Cost/session            $1.20     $0.95     -21% ↓
Agent success rate      0.73      0.82      +9% ↑
Zero-commit rate        0.40      0.25      -38% ↓
```

Green for improvements, red for regressions.

**Use case:** The measurement half of the fix-measure loop. Snapshot before making changes, work for a week, compare to see if it worked.

### `claudewatch correlate`

Factor analysis: correlate session attributes against outcomes.

```bash
claudewatch correlate friction
claudewatch correlate friction --factor has_claude_md
claudewatch correlate commits --project myproject
```

**Outcomes:** `friction`, `commits`, `zero_commit`, `cost`, `duration`, `tool_errors`

**Factors:** `has_claude_md`, `uses_task_agent`, `uses_mcp`, `uses_web_search`, `is_saw`, `tool_call_count`, `duration`, `input_tokens`

**Output:**

```
Factor Analysis: friction
Project: claudewatch (42 sessions)

Factor              Type     Correlation   P-value   N    Confidence
has_claude_md       boolean  -0.35        0.001     42   high
is_saw              boolean  -0.22        0.04      42   high
tool_call_count     numeric  +0.45        <0.001    42   high
```

**Use case:** "Does having CLAUDE.md reduce friction?" "Do SAW sessions commit more?"

## MCP Tool Usage

Metrics are available inside Claude sessions via MCP tools:

- **`get_project_health`** - Friction patterns, tool error rates, agent performance for one project
- **`get_session_stats`** - Metrics for the current session (live)
- **`get_session_dashboard`** - All live metrics in one call (PostToolUse hook context)
- **`get_effectiveness`** - Before/after CLAUDE.md change comparisons
- **`get_cost_summary`** - Spend totals across time horizons
- **`get_stale_patterns`** - Friction types recurring without CLAUDE.md updates
- **`get_agent_performance`** - Aggregate agent metrics
- **`get_cost_velocity`** - Burn rate over last 10 minutes
- **`get_cost_attribution`** - Token cost by tool type for a session

See [MCP Tools Reference](/docs/features/MCP_TOOLS.md) for full signatures.

## The Fix-Measure Loop

Metrics are designed for iteration:

```bash
# 1. Baseline - where are you now?
claudewatch scan
claudewatch metrics --days 30 > baseline.json

# 2. Diagnose - what's wrong?
claudewatch gaps
claudewatch suggest --limit 3

# 3. Fix - apply improvements
claudewatch fix myproject

# 4. Measure - did it work?
# ... work for a week ...
claudewatch metrics --days 30 > after.json
claudewatch track --compare

# 5. Iterate - repeat
```

Without `track` snapshots before and after, you have no reference point. The compare table will be empty or misleading.

## Exporting Metrics

All metric commands support `--json` for programmatic consumption:

```bash
claudewatch metrics --days 30 --json | jq '.friction_trends'
claudewatch suggest --json | jq '[.[] | select(.impact_score > 10)]'
```

Redirect to a file for baselines:

```bash
claudewatch metrics --days 30 --json > baseline.json
# ... apply fixes, work for a week ...
claudewatch metrics --days 30 --json > after.json
diff <(jq . baseline.json) <(jq . after.json)
```

## Known Limitations

### 1. Friction detection is heuristic

Friction is inferred from tool errors, user corrections, and retry patterns. Not all friction is captured:
- Silent failures (code that compiles but doesn't work)
- User corrections not visible in transcripts
- Context thrashing without explicit errors

**Mitigation:** Friction rate is a proxy, not ground truth. Combine with cost-per-commit and zero-commit rate for full picture.

### 2. Cache hit rate estimation

Cache hits are detected by comparing consecutive input token counts. If input tokens drop significantly, it's inferred as a cache hit. This is approximate.

**Mitigation:** Use cache-adjusted costs as estimates, not precise accounting.

### 3. Agent performance requires full transcripts

Agent success/kill rates are computed from session transcripts (Task tool_use/tool_result pairs). This is slow for large codebases.

**Mitigation:** Agent metrics are cached in session-meta files after first parse.

### 4. Effectiveness scoring requires sufficient data

Before/after comparisons need ≥5 sessions on each side of the CLAUDE.md change timestamp. Projects with sparse sessions may show "insufficient data".

**Mitigation:** Wait for more sessions to accumulate, or manually tag sessions with `claudewatch experiment`.

## Related Documentation

- [Hooks](/docs/features/HOOKS.md) - SessionStart briefing includes friction rate and top types
- [MCP Tools Reference](/docs/features/MCP_TOOLS.md) - Full list of metrics-related MCP tools
- [CLI Commands](/docs/features/CLI.md) - `metrics`, `gaps`, `suggest`, `track`, `correlate` command details
- [Agent Analytics](/docs/features/AGENTS.md) - Agent-specific performance metrics
