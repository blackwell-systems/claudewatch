# Agent Analytics: Multi-Agent Workflow Observability

Agent analytics is claudewatch's core **AgentOps differentiation**. No other tool monitors agent-to-agent coordination, success rates by agent type, or parallelization efficiency. This is not API monitoring—it's **agent behavior analysis** extracted from session transcripts.

## What Agent Analytics Provides

Traditional observability platforms track API calls and costs. Agent analytics tracks **agent lifecycles**:

- Which agent types complete successfully vs get killed
- How long agents run and how many tokens they consume
- Whether agents run in parallel or sequentially
- Which projects have high agent kill rates
- Whether SAW (Scout-and-Wave) parallelization is effective

This is the operational layer for multi-agent workflows. You can't optimize what you can't measure.

## Data Source

Agent spans are extracted from session JSONL transcripts:

```
~/.claude/projects/<project-hash>/*.jsonl
```

Each agent task is represented by a Task tool_use/tool_result pair:

```json
{
  "type": "tool_use",
  "name": "Task",
  "input": {
    "id": "agent-abc123",
    "type": "explore",
    "background": false,
    "instructions": "..."
  }
}

// ... later in the transcript ...

{
  "type": "tool_result",
  "tool_use_id": "agent-abc123",
  "content": "..."
}
```

claudewatch parses these pairs to extract:
- Agent type (explore, plan, general-purpose, etc.)
- Duration (time between tool_use and tool_result)
- Status (completed, killed, failed)
- Token usage (from transcript token counts in that span)
- Parallelization (overlapping agent spans)

## Agent Lifecycle States

### Completed

Agent finished successfully. The tool_result contains output that was incorporated into the session.

**Example:**

```
Agent explore-abc123
  Type: explore
  Duration: 45 seconds
  Tokens: 12,500
  Status: completed
```

**What it means:** Agent did its job and returned useful results.

### Killed

Agent was stopped mid-execution via TaskStop tool call. Work was discarded.

**Example:**

```
Agent plan-def456
  Type: plan
  Duration: 120 seconds
  Tokens: 38,000
  Status: killed
```

**What it means:** Agent went off-track, took too long, or the user intervened. Work was wasted.

**Cost impact:** Tokens consumed but no output used. Pure cost with no value.

### Failed

Agent encountered an error and could not complete.

**Example:**

```
Agent general-ghi789
  Type: general
  Duration: 5 seconds
  Tokens: 800
  Status: failed
```

**What it means:** Technical failure (timeout, crash, missing dependencies). Less common than killed.

## Agent Types

claudewatch tracks agent types from the Task tool `type` field:

- **explore** - Read-heavy exploration agents
- **plan** - Planning and design agents
- **general** - General-purpose implementation agents
- **research** - Research and documentation agents
- **<custom>** - User-defined agent types

Each type has independent success/kill rates and performance metrics.

### Type-Specific Patterns

**Explore agents:**
- High read/write ratio (expected)
- Lower token usage per agent
- Short duration (typically <1 min)
- High success rate (low risk)

**Plan agents:**
- Long duration (design takes time)
- High token usage (reasoning-heavy)
- Moderate success rate (plans get killed when approach changes)

**General agents:**
- Balanced read/write ratio
- Moderate token usage
- Variable duration (depends on task complexity)
- Success rate depends on task clarity

## Metrics Computed

### Success Rate

**Definition:** Fraction of agent tasks with `completed` status.

```
success_rate = completed_agents / total_agents
```

- 1.0 = All agents completed (perfect)
- 0.8 = 80% completed (good)
- 0.5 = 50% completed (moderate, investigate kills)
- 0.3 = 30% completed (high kill rate, agents are not working)

**Why it matters:** Low success rates mean wasted tokens and time. Agents get killed because:
1. Instructions were unclear
2. Agent went off-track
3. Task was too complex for the agent type
4. User intervened due to wrong approach

**Target:** >70% for mature projects, >50% for experimental workflows.

### Kill Rate

**Definition:** Fraction of agent tasks stopped via TaskStop.

```
kill_rate = killed_agents / total_agents
```

This is the inverse of success rate when failures are rare.

**Why it matters:** Kills are expensive. The work is discarded, but tokens are already consumed.

**Cost of kills:**

```
wasted_tokens = sum(killed_agents[].tokens)
wasted_cost = wasted_tokens * token_price
```

For a project with 10 killed agents averaging 30k tokens each:
```
wasted_tokens = 10 * 30,000 = 300,000
wasted_cost = 300,000 * $3/MTok = $0.90
```

That's $0.90 burned with zero output.

**Target:** <20% kill rate.

### Background vs Foreground Ratio

**Definition:** Fraction of agents that ran in background mode.

```
background_ratio = background_agents / total_agents
```

Background agents run asynchronously while the orchestrator continues. Foreground agents block until completion.

**Why it matters:** Background agents enable parallelization. High background ratio + low parallel sessions = underutilized parallelism (agents launched sequentially, not batched).

**Target:** >50% background ratio for SAW workflows.

### Average Duration

**Definition:** Mean time from agent launch to completion/kill.

```
avg_duration_ms = sum(agent_durations) / total_agents
```

**Why it matters:** Long durations indicate:
1. Complex tasks (expected)
2. Agents are stuck (bad)
3. Agents are exploring without direction (bad)

Correlate with success rate:
- Long duration + high success rate = complex work
- Long duration + low success rate = agents getting stuck, then killed

**Target:** <60 seconds for explore agents, <120 seconds for plan agents.

### Average Tokens Per Agent

**Definition:** Mean token consumption per agent task.

```
avg_tokens = sum(agent_tokens) / total_agents
```

**Why it matters:** High token usage per agent means either:
1. Large context windows (CLAUDE.md, file reads)
2. Verbose agent output
3. Agents running longer than needed

Correlate with duration and success rate:
- High tokens + low success rate = expensive failures
- High tokens + high success rate = justified complexity

**Target:** <20k tokens for explore agents, <50k tokens for plan agents.

### Parallel Sessions

**Definition:** Sessions that ran 2+ concurrent agents.

```
parallel_sessions = count(sessions with overlapping agent spans)
```

**Why it matters:** Parallelization is the point of multi-agent workflows. Low parallel session count means:
1. Tasks are sequential (no parallelization opportunity)
2. Parallelization is supported but not used (underutilized)

**Target:** >30% of sessions for SAW-enabled projects.

## SAW Observability

SAW (Scout-and-Wave) is a parallel agent workflow pattern where a scout agent identifies work items and wave agents execute them concurrently.

claudewatch provides SAW-specific observability:

### `get_saw_sessions`

Returns recent sessions that used SAW parallel agents.

**MCP tool signature:**

```json
{
  "name": "get_saw_sessions",
  "arguments": {
    "n": 5  // optional, default: 5
  }
}
```

**Returns:**

```json
{
  "sessions": [
    {
      "session_id": "abc123",
      "project_name": "claudewatch",
      "wave_count": 2,
      "agent_count": 7,
      "start_time": "2026-03-01T10:00:00Z"
    }
  ]
}
```

**Use case:** "How many SAW sessions did we run recently?"

### `get_saw_wave_breakdown`

Returns per-wave timing and per-agent status for a single SAW session.

**MCP tool signature:**

```json
{
  "name": "get_saw_wave_breakdown",
  "arguments": {
    "session_id": "abc123"
  }
}
```

**Returns:**

```json
{
  "waves": [
    {
      "wave_number": 1,
      "duration_ms": 125000,
      "agents": [
        {
          "agent_id": "agent-a",
          "type": "explore",
          "status": "completed",
          "duration_ms": 45000,
          "tokens": 12500
        },
        {
          "agent_id": "agent-b",
          "type": "explore",
          "status": "completed",
          "duration_ms": 38000,
          "tokens": 10200
        }
      ]
    },
    {
      "wave_number": 2,
      "duration_ms": 85000,
      "agents": [
        {
          "agent_id": "agent-c",
          "type": "general",
          "status": "completed",
          "duration_ms": 85000,
          "tokens": 32000
        }
      ]
    }
  ]
}
```

**Use case:** "Which wave took longest? Which agents got killed?"

### SAW vs Sequential Comparison

**CLI command:** `claudewatch compare`

Detects SAW sessions by parsing transcripts and identifying Scout-and-Wave patterns. Compares SAW sessions vs sequential sessions for a project.

```bash
claudewatch compare
claudewatch compare --project claudewatch
```

**Output:**

```
SAW vs Sequential Comparison: claudewatch

Type         Sessions   Avg Cost   Avg Commits   Cost/Commit   Avg Friction
SAW          12         $1.45      2.3           $0.63         0.35
Sequential   28         $1.20      1.8           $0.67         0.42

Totals       40         $1.28      2.0           $0.64         0.40
```

**Insights:**

- SAW sessions cost more per session (+21%) but produce more commits (+28%)
- Cost per commit is slightly better with SAW ($0.63 vs $0.67)
- Friction is lower with SAW (0.35 vs 0.42)

**Use case:** "Is SAW worth the complexity? Does it reduce cost per commit?"

## Agent Performance by Type

**MCP tool:** `get_agent_performance`

Returns aggregate performance metrics for all agent tasks across all sessions, with per-type breakdown.

**MCP tool signature:**

```json
{
  "name": "get_agent_performance",
  "arguments": {}
}
```

**Returns:**

```json
{
  "total_agents": 145,
  "success_rate": 0.73,
  "kill_rate": 0.22,
  "background_ratio": 0.58,
  "avg_duration_ms": 68000,
  "avg_tokens_per_agent": 24500,
  "parallel_sessions": 18,
  "by_type": {
    "explore": {
      "count": 62,
      "success_rate": 0.85,
      "avg_duration_ms": 42000,
      "avg_tokens": 12000
    },
    "plan": {
      "count": 28,
      "success_rate": 0.50,
      "avg_duration_ms": 135000,
      "avg_tokens": 48000
    },
    "general": {
      "count": 55,
      "success_rate": 0.75,
      "avg_duration_ms": 72000,
      "avg_tokens": 28000
    }
  }
}
```

**Insights:**

- Explore agents have high success rate (85%) and low tokens (12k avg)
- Plan agents have low success rate (50%) and high tokens (48k avg) → investigate why plans get killed
- General agents are mid-range on all metrics

**Action:** If plan agents have low success rate, consider:
1. Breaking planning tasks into smaller chunks
2. Using synchronous (foreground) planning instead of background
3. Documenting planning approach in CLAUDE.md

## Agent Adoption Trends

**Metric:** Agent task count over time.

```
Agent Adoption:
  Week 1:  12 agents
  Week 2:  18 agents (+50%)
  Week 3:  28 agents (+56%)
  Week 4:  35 agents (+25%)
```

**Why it matters:** Increasing agent usage indicates growing confidence in multi-agent workflows. Decreasing usage may indicate:
1. High kill rates discouraged further use
2. Tasks became more sequential (less parallelization opportunity)
3. Friction made synchronous work preferable

**Target:** Increasing or stable adoption for projects that benefit from parallelization.

## Integration with Other Metrics

Agent analytics integrates with:

- **Project health** - Agent success rate appears in SessionStart briefing
- **Friction** - Agent kills often correlate with `wrong_approach` friction
- **Cost-per-outcome** - Agents consume tokens; compare cost/commit for SAW vs sequential
- **Stale patterns** - If agent kill rate is high and CLAUDE.md has no agent guidance, suggest adding it

## Suggest Rules Powered by Agent Data

claudewatch's suggest engine has three rules derived from agent analytics:

### 1. ParallelizationOpportunity

Detects sessions with sequential agents that could run in parallel.

**Trigger:** ≥3 agents launched sequentially (not overlapping) in a session

**Suggestion:**

```
Category: agents
Priority: 2
Title: Enable SAW parallelization for multi-file features
Description: 8 sessions launched ≥3 agents sequentially. Use Scout-and-Wave
to parallelize independent work items. Estimated time savings: 40%.
Impact score: 15.2
```

**Action:** Use `/saw` command to analyze and parallelize work.

### 2. AgentTypeEffectiveness

Identifies agent types with high kill rates.

**Trigger:** Agent type has <50% success rate and ≥10 instances

**Suggestion:**

```
Category: agents
Priority: 1
Title: Plan agents have 45% kill rate
Description: 28 plan agents launched, only 13 completed. Document planning
approach in CLAUDE.md or switch to synchronous planning.
Impact score: 22.8
```

**Action:** Add CLAUDE.md guidance on when to use plan agents, or stop using them.

### 3. AgentAdoption

Tracks agent usage growth.

**Trigger:** Agent usage increased by >50% over the last 4 weeks

**Suggestion:**

```
Category: agents
Priority: 3
Title: Agent usage growing (+120% in 4 weeks)
Description: Positive trend. Consider documenting agent best practices for
other developers.
Impact score: 8.5
```

**Action:** Document agent patterns in CLAUDE.md or project README.

## CLI Commands

### `claudewatch metrics`

Includes agent performance section in output:

```bash
claudewatch metrics --days 30
```

**Agent Performance section:**

```
Agent Performance:
  Total agents:        145
  Success rate:        73%
  Kill rate:           22%
  Background ratio:    58%
  Avg duration:        68s
  Avg tokens/agent:    24,500
  Parallel sessions:   18 (45% of sessions with agents)

By Type:
  explore    62 agents, 85% success, 42s avg, 12k tokens
  plan       28 agents, 50% success, 135s avg, 48k tokens
  general    55 agents, 75% success, 72s avg, 28k tokens
```

### `claudewatch compare`

SAW vs sequential session comparison (see SAW Observability above).

```bash
claudewatch compare --project myproject
```

### `claudewatch suggest`

Includes agent-related suggestions:

```bash
claudewatch suggest --limit 10
```

Surfaces parallelization opportunities, agent type effectiveness issues, and adoption trends.

## MCP Tools

Agent-specific MCP tools:

- **`get_agent_performance`** - Aggregate agent metrics with per-type breakdown
- **`get_saw_sessions`** - Recent SAW sessions
- **`get_saw_wave_breakdown`** - Per-wave timing and status for a SAW session
- **`get_project_health`** - Includes agent success rate for the project

See [MCP Tools Reference](/docs/features/MCP_TOOLS.md) for full signatures.

## Debugging Agent Issues

### High Kill Rate

**Symptom:** Agent success rate <50%

**Diagnosis:**

1. Call `get_agent_performance` to see per-type breakdown
2. Identify which types have low success rate
3. Check recent sessions: `claudewatch replay <session-id>` to see where agents were killed
4. Look for patterns: same task type killed repeatedly, or random?

**Fixes:**

- **Task complexity**: Break tasks into smaller chunks
- **Unclear instructions**: Add task template or checklist to CLAUDE.md
- **Agent type mismatch**: Use synchronous (foreground) agents for tasks that need coordination

### Long Agent Duration

**Symptom:** Agents take >2 minutes on average

**Diagnosis:**

1. Check `get_agent_performance` for `avg_duration_ms` per type
2. Identify which types are slow
3. Replay sessions to see what agents are doing during that time

**Fixes:**

- **Stuck loops**: Agent is retrying without progress. Add failure detection and early exit guidance.
- **Large context**: Agent is reading too many files. Add file scope guidance.
- **Unclear task**: Agent is exploring instead of implementing. Add clearer instructions.

### Underutilized Parallelization

**Symptom:** High background ratio (>50%) but low parallel sessions (<30%)

**Diagnosis:**

1. Call `get_agent_performance` to confirm background_ratio and parallel_sessions
2. Check if agents are launched sequentially (one after the other) instead of batched
3. Look for `/saw wave` usage—if not used, parallelization is not happening

**Fixes:**

- **Sequential workflow**: Tasks are inherently sequential. No fix needed.
- **SAW not used**: Use `/saw scout` to identify parallelization opportunities, then `/saw wave` to execute.
- **Batch size = 1**: Agents launched one at a time. Increase batch size in SAW waves.

### High Tokens Per Agent

**Symptom:** Agents consume >50k tokens on average

**Diagnosis:**

1. Check `get_agent_performance` for `avg_tokens_per_agent`
2. Call `get_cost_attribution` for high-cost sessions to see which agent tool calls consumed tokens
3. Replay sessions to see agent output length

**Fixes:**

- **Large context**: Agent reads too many files. Add file scope guidance or use Grep instead of Read.
- **Verbose output**: Agent writes long explanations. Add "be concise" instruction to agent template.
- **Reasoning overhead**: Agent type is reasoning-heavy (e.g., plan agents). Consider synchronous planning instead.

## Agent Performance Best Practices

### 1. Use the Right Agent Type

- **Explore**: Read-heavy tasks, file discovery, research
- **Plan**: Design and architecture (expect long duration, high tokens)
- **General**: Implementation (balanced read/write)

Don't use plan agents for routine implementation—they're optimized for reasoning, not execution.

### 2. Launch Agents in Batches

Sequential agent launches (one after the other) don't benefit from parallelization. Use SAW to batch independent agents:

```
Scout phase: Identify 5 independent file edits
Wave 1: Launch all 5 agents in parallel
Wave 2: Merge results
```

This reduces total time from `5 × agent_duration` to `max(agent_durations)`.

### 3. Monitor Kill Patterns

If the same task type gets killed repeatedly, it's not the agent—it's the task definition or approach. Document the pattern in CLAUDE.md:

```markdown
## Agent Usage

- ❌ Don't use plan agents for <50 line changes (overkill, high kill rate)
- ✅ Use general agents for routine implementation
- ✅ Use explore agents for initial file discovery
```

### 4. Measure Before and After

When adopting SAW or changing agent usage patterns:

```bash
# Baseline
claudewatch metrics --days 30 > before.json

# Make changes, work for 2 weeks

# Compare
claudewatch metrics --days 30 > after.json
claudewatch compare --project myproject
```

Look at:
- Agent success rate (should increase)
- Cost per commit (should decrease or stay flat)
- Parallel sessions (should increase if SAW adopted)

### 5. Document Agent Patterns

Add a section to CLAUDE.md:

```markdown
## Multi-Agent Workflows

This project uses SAW for features that touch ≥3 files.

**When to use agents:**
- File discovery: explore agents
- Multi-file refactors: SAW waves with general agents
- Research: explore or research agents

**When NOT to use agents:**
- Single-file changes (<50 lines): synchronous work is faster
- Complex coordination: agents can't see each other's work, synchronous is safer
```

## Known Limitations

### 1. Agent type detection is heuristic

Agent types are extracted from the Task tool `type` field. If the field is missing or non-standard, the agent is classified as `<unknown>`.

**Mitigation:** Use standard agent types (explore, plan, general, research) for consistent analytics.

### 2. Agent spans require transcript parsing

Extracting agent spans from JSONL transcripts is slow for large sessions (>100MB). This is why agent metrics are cached in session-meta files after first parse.

**Mitigation:** Agent analytics may lag for very recent sessions until the cache updates.

### 3. Parallelization detection is timing-based

Two agents are considered parallel if their time spans overlap. This may misclassify:
- Agents launched sequentially with <1s gap (counted as parallel)
- Agents launched in parallel but one finishes before the other starts (counted as sequential)

**Mitigation:** Timing detection is approximate. Use SAW wave structure for explicit parallelization tracking.

### 4. Cost attribution is estimate-based

Agent token usage is estimated from transcript token counts in the agent's time span. This includes orchestrator overhead (messages between agent launch and completion).

**Mitigation:** Use agent cost attribution as a directional metric, not precise accounting.

## Related Documentation

- [Metrics](/docs/features/METRICS.md) - Agent performance is one section of comprehensive metrics
- [MCP Tools Reference](/docs/features/MCP_TOOLS.md) - `get_agent_performance`, `get_saw_sessions`, `get_saw_wave_breakdown`
- [CLI Commands](/docs/features/CLI.md) - `metrics`, `compare`, `suggest` command details
- [Technical: Data Model](/docs/technical/DATA_MODEL.md) - Agent span extraction from transcripts
