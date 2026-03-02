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
- MCP server (13+ tools)

---

## Tier 1 — Close gaps in existing dimensions

These require no new data sources. The raw data is already in transcripts or the existing DB.

### Per-turn attribution

**What:** Break cost and token counts down to the tool-call level rather than session level. Answer "which tool calls consumed most of my budget this session?" and "which phase (Scout / Wave agents / merge) was most expensive?"

**Why:** Session-level cost is too coarse for optimization decisions. Knowing that 60% of a session's cost came from Agent tool calls, and 30% from MCP round-trips, gives actionable targets.

**Data source:** Each transcript entry already carries token counts and model. Needs aggregation at tool-call type and phase level.

**New CLI:** `claudewatch attribute [--session <id>] [--project <name>]`
**New MCP:** `get_cost_attribution`

---

### Session replay

**What:** Walk through a session's decision points as a structured timeline: turns, tool calls, cost per turn, friction events, agent launches and completions. Comparable to a flame graph for a session.

**Why:** `claudewatch search` finds sessions but can't explain why a session went sideways. Replay makes post-mortems actionable.

**New CLI:** `claudewatch replay <session-id> [--turn <n>] [--json]`

---

### CLAUDE.md A/B testing

**What:** Deliberately compare two CLAUDE.md variants over a controlled time window. Tag sessions to a variant, compute outcome metrics per variant, report the winner.

**Why:** `get_effectiveness` detects accidental before/after improvement. A/B testing enables designed experiments — change one thing, measure it, decide.

**Depends on:** Session project tagging (already shipped).

**New CLI:** `claudewatch experiment start|stop|report [--project <name>]`

---

## Tier 2 — New analytical dimensions

These derive new insight from existing data without new collection infrastructure.

### Causal correlation engine

**What:** Correlate session attributes against outcomes to answer "what factors predict good sessions?" Example questions:
- Do sessions starting with `doctor` warnings have higher friction?
- Does SAW reduce zero-commit rate for tasks over N tool calls?
- Which CLAUDE.md sections correlate with lower friction?

**Why:** The biggest structural gap between claudewatch and a real AI Ops platform. Monitoring tells you what happened; causality tells you why and what to change.

**Approach:** Logistic/linear regression over the existing `session_meta` + `facets` data. No new collection needed — needs a correlation engine and a query interface.

**New CLI:** `claudewatch correlate <outcome> [--factor <field>] [--project <name>]`
**New MCP:** `get_causal_insights`

---

### Cost forecasting

**What:** Given early-session signals (project, first tool calls, agent count, task type), predict final session cost before it's incurred. Enables proactive budget warnings rather than reactive ones.

**Why:** The PostToolUse hook warns when cost velocity is burning. Forecasting warns before that threshold is reached — giving Claude and the developer a chance to scope down.

**Approach:** Train a simple regression model on historical session data (first N tool calls → final cost). Update incrementally as sessions complete.

**New MCP:** `get_cost_forecast` (called early in session, returns predicted range)

---

### Automated regression detection

**What:** Automatically flag when a project's friction rate or cost-per-commit crosses a threshold relative to its rolling baseline, without requiring a manual `track --compare` snapshot.

**Why:** Regressions currently surface only if you remember to snapshot before and after a change. Automated detection catches silent regressions.

**Approach:** Rolling baseline update on each `track` run; alert threshold as configurable multiplier over baseline.

**New CLI:** surfaces as a new doctor check and `watch` notification
**New MCP:** `get_regression_status`

---

### Self-optimizing baselines

**What:** Anomaly baselines are currently computed once and stored statically. They should recompute automatically as sessions accumulate, with exponential weighting that gives more influence to recent sessions.

**Why:** Static baselines drift as workflow patterns change (e.g., after adopting SAW, the cost baseline should shift downward). A baseline that doesn't update becomes a false-positive generator.

**Approach:** On each new session completion, re-weight the stored baseline with a configurable decay factor. Expose last-updated and session-count metadata in the baseline output.

---

## Tier 3 — Platform infrastructure

These require architectural additions beyond analysis of local session data.

### Metrics export

**What:** Make claudewatch data accessible to external observability tools. Two surfaces:
1. **Prometheus `/metrics` endpoint** — expose session, friction, cost, and anomaly metrics in the Prometheus text format. Enables Grafana dashboards.
2. **Structured time-series export** — `claudewatch export --format json|otlp --since <date>` for integration with time-series databases.

**Why:** Teams with existing observability infrastructure (Grafana, Datadog) should not need a separate dashboard. claudewatch data should flow into the stack they already have.

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
```

**Why:** Suggestions tell you what to fix; policies enforce guardrails automatically. The hook is the enforcement point — it needs a declarative rule format rather than hard-coded thresholds.

**Depends on:** Cost forecasting (for budget enforcement rules).

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

| Feature | Tier | New data needed | New CLI | New MCP |
|---|---|---|---|---|
| Per-turn attribution | 1 | No | `attribute` | `get_cost_attribution` |
| Session replay | 1 | No | `replay` | — |
| A/B testing | 1 | No | `experiment` | — |
| Causal correlation | 2 | No | `correlate` | `get_causal_insights` |
| Cost forecasting | 2 | No | — | `get_cost_forecast` |
| Regression detection | 2 | No | doctor check | `get_regression_status` |
| Self-optimizing baselines | 2 | No | — | — |
| Metrics export | 3 | No | `export` | — |
| Policy/rule engine | 3 | No | — | — |
| Multi-developer aggregation | 3 | Yes (sync) | — | — |
| Complexity estimation | 4 | No | — | — |
| Auto CLAUDE.md generation | 4 | No | — | — |
