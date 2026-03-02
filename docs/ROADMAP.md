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
- MCP server (14 tools)
- Per-turn cost attribution (`claudewatch attribute`, `get_cost_attribution` MCP)
- Session replay timeline (`claudewatch replay`)
- CLAUDE.md A/B testing (`claudewatch experiment start|stop|tag|report`)
- Self-optimizing anomaly baselines (EMA decay weighting, auto-refresh on every `anomalies` run)

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

## Tier 2 — New analytical dimensions

These derive new insight from existing data without new collection infrastructure.

### Factor analysis

**What:** Correlate session attributes against outcomes to answer "what factors predict good sessions?" Example questions:
- Do sessions starting with `doctor` warnings have higher friction?
- Does SAW reduce zero-commit rate for tasks over N tool calls?
- Which CLAUDE.md sections correlate with lower friction?

**Why:** The biggest structural gap between claudewatch and a real AI Ops platform. Monitoring tells you what happened; correlation tells you what to investigate and what to experiment on.

**Approach:** SQL aggregations + Pearson correlation coefficients over the existing `session_meta` + `facets` data. No model training — grouped comparisons with sample sizes shown. Results include `n` per group; findings with n < 10 are flagged as low-confidence. True causal inference requires the `experiment` feature (designed A/B comparison); this is the exploratory precursor that identifies what to experiment on.

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

~~### Self-optimizing baselines~~

~~**Shipped in v0.7.9.** `claudewatch anomalies` now recomputes the baseline with EMA decay weighting (0.9) on every run.~~

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
| ~~Per-turn attribution~~ | ~~1~~ | ~~No~~ | ~~`attribute`~~ | ~~`get_cost_attribution`~~ |
| ~~Session replay~~ | ~~1~~ | ~~No~~ | ~~`replay`~~ | ~~—~~ |
| ~~A/B testing~~ | ~~1~~ | ~~No~~ | ~~`experiment`~~ | ~~—~~ |
| Factor analysis | 2 | No | `correlate` | `get_causal_insights` |
| Cost forecasting | 2 | No | — | `get_cost_forecast` |
| Regression detection | 2 | No | doctor check | `get_regression_status` |
| ~~Self-optimizing baselines~~ | ~~2~~ | ~~No~~ | ~~—~~ | ~~—~~ |
| Metrics export | 3 | No | `export` | — |
| Policy/rule engine | 3 | No | — | — |
| Multi-developer aggregation | 3 | Yes (sync) | — | — |
| Complexity estimation | 4 | No | — | — |
| Auto CLAUDE.md generation | 4 | No | — | — |
