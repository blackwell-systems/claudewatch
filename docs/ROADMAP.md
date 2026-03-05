# claudewatch Roadmap

A progression from monitoring tool to AI Ops platform. Items are ordered within each tier by value-to-effort ratio. Tiers represent dependency depth, not strict sequencing — Tier 2 items can begin once the relevant Tier 1 data is available.

---

## Shipped

- Friction detection and session monitoring
- Cost tracking (session, daily, weekly, per-project)
- Agent performance metrics
- CLAUDE.md effectiveness (before/after comparison)
- Suggestion engine (7 rules, impact-ranked)
- FTS transcript search with auto-indexing
- SAW vs sequential workflow comparison
- Per-project anomaly baselines (z-score detection)
- Doctor command with baseline coverage check
- PostToolUse and SessionStart hooks
- MCP server (28 tools across 7 categories)
- Per-turn cost attribution (`claudewatch attribute`, `get_cost_attribution` MCP)
- Session replay timeline (`claudewatch replay`)
- CLAUDE.md A/B testing (`claudewatch experiment start|stop|tag|report`)
- Self-optimizing anomaly baselines (EMA decay weighting, auto-refresh on every `anomalies` run)
- Factor analysis (`claudewatch correlate`, `get_causal_insights` MCP) — shipped v0.7.9
- Automated regression detection (`get_regression_status` MCP) — shipped v0.7.9
- Multi-project attribution (weighted repo tracking, `get_session_projects` MCP) — shipped v0.8.0
- Drift detection (`get_drift_signal` MCP) — shipped v0.8.0
- Cross-session memory (`memory show|clear` CLI, `get_task_history`/`get_blockers` MCP, SessionStart lazy extraction) — shipped v0.9.0
- Feature documentation (7 files: HOOKS.md, MCP_TOOLS.md, CLI.md, MEMORY.md, CONTEXT_SEARCH.md, METRICS.md, AGENTS.md) — shipped (unreleased)

---

## Documentation Hub Completion

**Status:** 7/21 files complete (feature docs done, 14 remaining)

**Remaining work:**
- Guides (6 files): INSTALLATION.md, QUICKSTART.md, CONFIGURATION.md, USE_CASE_FRICTION.md, USE_CASE_AGENTS.md, USE_CASE_CLAUDEMD.md
- Technical (4 files): ARCHITECTURE.md, HOOKS_IMPL.md, MCP_INTEGRATION.md, DATA_MODEL.md
- Comparison (3 files): VS_MEMORY_TOOLS.md, VS_OBSERVABILITY.md, VS_BUILTIN.md
- Community (1 file): CONTRIBUTING.md

**Content plan:** `docs/CONTENT_PLAN.md` has source mapping for all files

**Estimated time:** ~90 minutes for remaining 14 files (sequential batched creation)

---

## Tier 1 — Close gaps in existing dimensions ✓ Complete

All three Tier 1 items shipped in v0.7.9.

~~### Per-turn attribution~~

~~**New CLI:** `claudewatch attribute [--session <id>] [--project <name>]`~~
~~**New MCP:** `get_cost_attribution`~~

~~### Session replay~~

~~**New CLI:** `claudewatch replay <session-id> [--from <n>] [--to <n>] [--json]`~~

~~### CLAUDE.md A/B testing~~

~~**New CLI:** `claudewatch experiment start|stop|tag|report [--project <name>]`~~

---

## Tier 2 — New analytical dimensions ✓ Complete

All three Tier 2 items shipped in v0.7.9 and v0.8.0.

~~### Factor analysis~~

~~**Shipped in v0.7.9.** `claudewatch correlate` and `get_causal_insights` MCP tool.~~

~~### Automated regression detection~~

~~**Shipped in v0.7.9.** `get_regression_status` MCP tool checks if friction rate or cost has regressed beyond baseline threshold.~~

~~### Multi-project session attribution~~

~~**Shipped in v0.8.0.** Weighted repo attribution for multi-repo sessions. `get_session_projects` MCP tool shows cost/activity distribution across projects.~~

---

---

## Tier 2.5 — Intelligence enhancement

New analytical capabilities building on existing observability layer.

### Predictive Intelligence

**What:** Move from reactive alerts to predictive warnings. Detect exploration patterns that typically spiral, predict zero-commit sessions before they complete, forecast context pressure before it becomes critical.

**Why:** Reactive alerts say "you have a problem now." Predictive alerts say "you're heading toward a problem, adjust course."

**Examples:**
- "You've read auth.go 5 times without writing. Last 3 sessions with this pattern abandoned the task."
- "Commit ratio below 0.1. In 89% of cases, this trajectory leads to zero commits."
- "Context at 75%. At current velocity, you'll hit critical within 8 turns."

**Approach:** Time-series analysis on live metrics, pattern matching against historical outcomes, Markov chains for state transitions.

**New MCP:** `get_trajectory_prediction` (called mid-session, returns risk assessment + historical precedent)

---

### Cost forecasting

**What:** Given early-session signals (project, first tool calls, agent count, task type), predict final session cost before it's incurred. Enables proactive budget warnings rather than reactive ones.

**Why:** The PostToolUse hook warns when cost velocity is burning. Forecasting warns before that threshold is reached — giving Claude and the developer a chance to scope down.

**Approach:** Train a simple regression model on historical session data (first N tool calls → final cost). Update incrementally as sessions complete.

**New MCP:** `get_cost_forecast` (called early in session, returns predicted range)

---

## Tier 3 — Platform infrastructure

These require architectural additions beyond analysis of local session data.

~~### Cross-Session Memory~~

~~**Shipped in v0.9.0.** SessionStart lazy extraction, `memory show|clear` CLI, `get_task_history`/`get_blockers` MCP tools.~~

---

### Memory-Commit Integration (commitmux enrichment)

**What:** Enrich cross-session memory with commit details from commitmux. Connect the "why" (task goals, blockers) with the "what" (actual code changes).

**Why:** Memory currently stores commit SHAs as strings. Claude has to manually query commitmux to see what changed. Automatic enrichment creates a complete narrative: "We attempted X, hit blocker Y in file Z, solved it with commit ABC: [diff summary]."

**Approach (phased):**

**Phase 1 (passive):** Already shipped. Memory stores commit SHAs, Claude queries both tools manually.

**Phase 2 (auto-enrichment):** Enhance `ExtractTaskMemory` to populate Solution field automatically:
```go
if status == "completed" && len(commits) > 0 {
    // Query: commitmux get-patch <repo> <sha>
    solution = extractSolutionFromCommit(commits[0])
    // Returns: "Modified auth.go: added token refresh logic"
}
```
Guard: only runs if commitmux binary exists and repo is indexed.

**Phase 3 (MCP enrichment):** Add `enrich_commits` parameter to `get_task_history`:
```json
{
  "query": "auth",
  "enrich_commits": true  // Fetch full commit details from commitmux
}
```
Returns task history WITH commit messages and diff summaries on-demand.

**Phase 4 (solution mining):** Extract patterns from diffs ("added retry logic", "fixed race condition"), store pattern taxonomy, enable pattern-based search: "How did we solve retry logic before?"

**Decision criteria:** Measure memory query usage for 10-20 sessions. If Claude uses `get_task_history` ≥3x per session, Phase 2 ROI is clear.

**Depends on:** Cross-session memory (shipped), commitmux indexing

---

### Metrics export

**What:** Make claudewatch data accessible to external observability tools. Two surfaces:
1. **Prometheus `/metrics` endpoint** — expose session, friction, cost, and anomaly metrics in the Prometheus text format. Enables Grafana dashboards.
2. **Structured time-series export** — `claudewatch export --format json|otlp --since <date>` for integration with time-series databases.

**Why:** Teams with existing observability infrastructure (Grafana, Datadog) should not need a separate dashboard. claudewatch data should flow into the stack they already have.

---

### Closed-Loop Adaptation

**What:** claudewatch injects behavioral adjustments directly into Claude's context mid-session based on observed patterns. Moves from "alert Claude to a problem" to "adjust Claude's behavior automatically."

**Why:** Current flow: hook alerts → Claude sees alert → Claude decides what to do. Closed-loop: hook alerts → hook injects constraint → Claude's behavior changes automatically.

**Examples:**
- **Auto-scoping:** "High drift detected (15 min reading, zero writes). Injecting constraint: 'Next action must be Edit or Write.'"
- **Dynamic mode switching:** "Context pressure critical. Switching to summary-only responses until compaction completes."
- **Agent policy updates:** "Plan agents killed in 3/3 recent sessions. Updating behavioral contract: skip plan mode for this project."

**Approach:** PostToolUse hook can inject structured instructions into stderr that Claude Code displays to Claude. The hook becomes write-enabled to `.claude/CLAUDE.md` for persistent policy updates.

**Depends on:** Cross-Session Memory (for tracking what adjustments were tried and their outcomes).

---

### Policy/rule engine

**What:** A declarative rule format that enforces guardrails at session start or during a session, not just suggests them. Example rules:

```yaml
rules:
  - when: project_health.friction_rate > 0.6
    action: inject "High friction project — run claudewatch doctor first"
  - when: session.consecutive_tool_errors >= 3
    action: alert "Stop and call get_session_dashboard"
  - when: cost_forecast.p90_usd > budget_remaining
    action: alert "Session likely to exceed daily budget"
  - when: drift_signal.status == "drifting" && drift_signal.read_calls > 10
    action: inject_constraint "Next action must be Edit or Write"
```

**Why:** Suggestions tell you what to fix; policies enforce guardrails automatically. The hook is the enforcement point — it needs a declarative rule format rather than hard-coded thresholds.

**Depends on:** Cost forecasting (for budget enforcement rules), Predictive Intelligence (for trajectory-based rules).

---

### Multi-developer aggregation

**What:** A shared data layer enabling teams using Claude Code on the same codebase to see aggregate patterns: cross-developer friction comparison, team-level baselines, shared CLAUDE.md effectiveness tracking.

**Why:** claudewatch is currently single-machine. A team of five developers on the same repo has no shared view of what's working.

**Approach:** A sync protocol or shared central DB (self-hosted or lightweight cloud). Requires authentication and data model changes for multi-tenant isolation.

---

## Tier 4 — Intelligence layer

These require inference or generative capabilities beyond pattern aggregation.

### Complexity estimation

**What:** Given a task description and a codebase's session history, estimate whether this is a 1-session task or a 5-session task, and whether SAW parallelization would help.

**Why:** The forecasting problem applied to planning rather than cost. Prevents underscoping tasks that consistently require multiple sessions.

---

### Automated CLAUDE.md generation from patterns

**What:** Move `fix --ai` from a one-shot generation to a continuously-updated recommendation. As sessions accumulate, the generator re-evaluates which sections are stale, which friction patterns are newly chronic, and proposes targeted patches — without requiring a manual invocation.

**Why:** `fix --ai` requires remembering to run it. Automated generation closes the loop on the fix-measure cycle.

---

## Summary table

| Feature | Tier | Status | New CLI | New MCP |
|---|---|---|---|---|
| ~~Per-turn attribution~~ | ~~1~~ | ~~✓ v0.7.9~~ | ~~`attribute`~~ | ~~`get_cost_attribution`~~ |
| ~~Session replay~~ | ~~1~~ | ~~✓ v0.7.9~~ | ~~`replay`~~ | ~~—~~ |
| ~~A/B testing~~ | ~~1~~ | ~~✓ v0.7.9~~ | ~~`experiment`~~ | ~~—~~ |
| ~~Factor analysis~~ | ~~2~~ | ~~✓ v0.7.9~~ | ~~`correlate`~~ | ~~`get_causal_insights`~~ |
| ~~Regression detection~~ | ~~2~~ | ~~✓ v0.7.9~~ | ~~doctor check~~ | ~~`get_regression_status`~~ |
| ~~Multi-project attribution~~ | ~~2~~ | ~~✓ v0.8.0~~ | ~~—~~ | ~~`get_session_projects`~~ |
| ~~Drift detection~~ | ~~2~~ | ~~✓ v0.8.0~~ | ~~—~~ | ~~`get_drift_signal`~~ |
| Predictive Intelligence | 2.5 | Proposed | — | `get_trajectory_prediction` |
| Cost forecasting | 2.5 | Proposed | — | `get_cost_forecast` |
| ~~Cross-Session Memory~~ | ~~3~~ | ~~✓ v0.9.0~~ | ~~`memory`~~ | ~~`get_task_history`, `get_blockers`~~ |
| Memory-Commit Integration | 3 | Proposed | — | `get_task_history` (enriched) |
| Closed-Loop Adaptation | 3 | Proposed | — | — |
| Metrics export | 3 | Proposed | `export` | — |
| Policy/rule engine | 3 | Proposed | — | — |
| Multi-developer aggregation | 3 | Proposed | — | — |
| Complexity estimation | 4 | Proposed | — | — |
| Auto CLAUDE.md generation | 4 | Proposed | — | — |
