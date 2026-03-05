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

## Tier 2.5 — Polish & Integration UX

### MCP Tool Naming Consistency (Pre-1.0.0 blocker)

**Status:** Shipped `get_context` in v0.15.0, but naming is inconsistent with existing patterns.

**Problem:** Semantic inconsistency in MCP tool verb prefixes creates cognitive friction:
- `search_transcripts` — takes `query` parameter, returns filtered results
- `get_context` — takes `query` parameter, returns filtered results
- `get_task_history` — takes `query` parameter, returns filtered results

Using different verbs (`get` vs `search`) for the same semantic operation (query-based search) violates the principle of least surprise. Users build mental models: `get_*` should retrieve current state without filtering, `search_*` should accept queries and return ranked results.

**Solution:** Establish clear semantic patterns before 1.0.0:

**Semantic verb rules:**
```
search_* = takes query parameter, returns filtered/ranked results
get_*    = computes/returns current state, no query filtering
extract_* = write/checkpoint operation
```

**Required renames (v0.16.0):**
1. `get_context` → `search_context` (shipped yesterday, minimal adoption)
2. `get_task_history` → `search_task_history` (older, more usage, but still pre-1.0.0)

**Impact on existing tools:**
- ✓ `search_transcripts` — already correct
- ✓ `get_session_dashboard` — returns state, no query
- ✓ `get_project_health` — returns state, no query
- ✓ `get_drift_signal` — returns state, no query
- ✓ `get_cost_velocity` — returns state, no query
- ✓ `get_context_pressure` — returns state, no query
- ✓ `get_blockers` — returns all blockers, no filtering (correct as-is)
- ✓ `get_suggestions` — returns all suggestions, no filtering
- ✓ `extract_current_session_memory` — write operation

**Files to change:**
- `internal/mcp/unified_context_tools.go` — rename handler function
- `internal/mcp/server.go` — update tool registration
- `docs/features/MCP_TOOLS.md` — update documentation
- `docs/features/CONTEXT_SEARCH.md` — update examples
- `internal/mcp/task_memory_tools.go` — rename `get_task_history` handler
- All test files referencing these tools

**Migration path:**
- Alias old names to new names for 1-2 releases with deprecation warning
- Remove aliases in v1.1.0 or v1.2.0 after sufficient time for users to migrate

**Why before 1.0.0:**
- API naming is a contract — post-1.0.0 renames are breaking changes
- v0.15.0 shipped yesterday — adoption of `get_context` is minimal
- Establishing clear semantic patterns now prevents future naming debates

**Estimated effort:** ~2 hours (rename + alias + docs + tests)

---

### Stop Hook for Memory Extraction (Pre-1.0.0 enhancement)

**Status:** ✅ Implemented in v0.16.0 — See `internal/app/hook_stop.go` (27 passing tests)

**Problem:** Ops Memory layer depends on consistent memory extraction, but users have to remember to call `extract_current_session_memory` before sessions close. Long sessions with significant progress can close without checkpointing, losing valuable context about task state, blockers encountered, and solutions attempted.

**Why it matters for AgentOps:**
- Session close is a context loss event — equivalent to a system restart without saving state
- Cross-session learning breaks if memory extraction is manual and forgotten
- SessionStart can only surface prior context if Stop ensured it was saved
- The memory lifecycle is incomplete: SessionStart (load) → PostToolUse (monitor) → **Stop (checkpoint)** ← missing

**Solution:** Add Stop hook that detects significant sessions and prompts for memory extraction.

**Detection logic:**
```go
// Significant session criteria (any of):
- Duration > 30 minutes
- Tool calls > 50
- Commits made > 0
- Errors encountered and resolved (error count > 5 but friction not critical)
```

**Smart prompting based on session status:**
1. **Task completed** (commits > 0, low final drift):
   - "Session completed with N commits. Extract memory? Call extract_current_session_memory"

2. **Task abandoned** (zero commits, high tool errors):
   - "Session ended with zero commits and N errors. Worth extracting blockers? Call extract_current_session_memory"

3. **Task in-progress** (active work, no clear resolution):
   - "Session has significant work in progress. Extract checkpoint before closing? Call extract_current_session_memory"

**Skip conditions:**
- Session < 10 minutes and < 20 tool calls (trivial session)
- Last `extract_current_session_memory` call within 20 minutes (already checkpointed)
- Session is pure research (zero Edit/Write calls, only Read/Grep/Glob)

**Implementation:**
- New file: `internal/app/hook_stop.go`
- New hook configuration: `~/.claude/settings.json` Stop hook registration
- Leverage existing `GetActiveSession()` and session metadata
- Check `extract_current_session_memory` invocation timestamp from MCP call log
- Non-blocking: prompts Claude, doesn't force extraction

**Files to change:**
- `internal/app/hook_stop.go` — new handler
- `internal/claude/active.go` — add `WorthCheckpointing()` method to Session
- `cmd/claudewatch/hook.go` — register Stop event
- `docs/features/HOOKS.md` — document Stop hook behavior
- Tests for detection logic and skip conditions

**Migration:**
- Hook is additive — existing users get prompts, can ignore if they prefer manual extraction
- No breaking changes to existing hook behavior

**Why before 1.0.0:**
- Completes the memory lifecycle story (load → monitor → checkpoint)
- Establishes hook coverage expectations for 1.0.0 (SessionStart + PostToolUse + Stop)
- Validates that Ops Memory works end-to-end without manual intervention

**Estimated effort:** ~3 hours (detection logic + prompting + tests + docs)

---

### Graceful commitmux integration

**Status:** Shipped in v0.15.0. ✓ Complete

**Problem:** `get_context` MCP tool and `claudewatch context` CLI fail silently or return partial results when commitmux not installed. Users don't know commitmux is required for full functionality (memory + commit search). Hard dependency on separate tool creates adoption friction.

**Solution:** Graceful degradation with clear upgrade path.

**Implementation:**

1. **Detect commitmux availability**
   - Check `~/.cargo/bin/commitmux` (hardcoded path)
   - Check PATH lookup as fallback
   - Add config option: `commitmux_binary_path`

2. **Degrade gracefully when missing**
   - Return 2/4 sources (task history + transcripts) instead of failing
   - Include clear message in response: "2 of 4 sources available. Install commitmux for memory and commit search: brew install blackwell-systems/tap/commitmux"
   - Log source availability: `[context_search] sources: task_history=ok, transcript=ok, memory=unavailable (commitmux not found), commit=unavailable (commitmux not found)`

3. **Doctor command integration**
   - Add feature gate check: `claudewatch doctor` shows "Context search: 2/4 sources (install commitmux for full functionality)"
   - Test commitmux version compatibility
   - Show indexed repo count if commitmux present

4. **Documentation updates**
   - `docs/features/CONTEXT_SEARCH.md` — clearly mark memory + commits as "requires commitmux"
   - Add "Dependencies" section explaining optional commitmux
   - `docs/guides/INSTALLATION.md` — recommend installing both, show standalone vs full
   - README installation section — mention optional commitmux

5. **Homebrew formula**
   - Make commitmux `depends_on :recommended` (optional but recommended)
   - Add caveats message explaining feature availability

**Files to change:**
- `internal/client/mcp_client.go` — detect binary, skip external sources if missing
- `internal/mcp/unified_context_tools.go` — return availability message in response
- `internal/app/doctor.go` — add context search feature gate check
- `docs/features/CONTEXT_SEARCH.md` — document dependency
- Homebrew formula — make commitmux recommended not required

**Impact:** 🔥🔥 High — Removes adoption friction. Users can install claudewatch standalone, discover value, upgrade to full context search later. "Just works" experience.

**Estimated effort:** ~4 hours (detection logic + error handling + docs + testing)

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

### Memory-Commit Integration (optional convenience wrapper)

**Status:** Phase 1 shipped. Phase 2 deferred (needs usage data to justify).

**Current state (Phase 1):** Memory stores commit SHAs as strings. Claude queries both systems manually:
1. `get_task_history(query="auth")` → returns task with `commits=["abc123"]`
2. `commitmux_get_patch(sha="abc123")` → returns full diff and message
3. Claude synthesizes answer from both sources

**This explicit composition is the right default.** It's verbose but traceable, works without commitmux (step 2 just skips), and keeps concerns separated.

**Future consideration (Phase 2 - convenience only):**

IF usage data shows Claude frequently queries both systems together (≥5x per session over 20+ sessions), consider adding `enrich_commits` parameter to `get_task_history` as a convenience wrapper:

```json
{
  "query": "auth",
  "enrich_commits": true  // Explicit opt-in
}
```

**Implementation requirements:**
- Must be explicit parameter (no auto-magic)
- Must degrade gracefully when commitmux unavailable (return SHAs only)
- Enrichment happens at query time (no storage of commit data in memory files)
- Returns commit SHAs with inline diff summaries

**Why defer Phase 2:**
- Saves Claude one MCP call, but adds complexity
- Hidden dependency on commitmux creates confusion
- Need data proving the convenience is worth the coupling

**Rejected approaches:**
- ❌ Auto-enrichment during memory extraction (hidden coupling, staleness, storage bloat)
- ❌ Solution pattern mining (premature optimization, no proven need)

**Decision gate:** Deploy current state, measure `get_task_history` usage for 20+ sessions. If frequency ≥5x per session, revisit. Otherwise, explicit composition wins.

**Depends on:** Cross-session memory (shipped), graceful commitmux integration

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
