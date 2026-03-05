# IMPL: Per-Model Cost Accuracy for Historical Session Calculations

## Suitability Assessment

Verdict: **SUITABLE WITH CAVEATS**
test_command: `go build ./... && go vet ./... && go test ./...`

This feature is parallelizable across 3 agents in 2 waves. The core insight is that
`EstimateSessionCost` (in `outcomes.go`) is the single choke point: all 20+ call
sites in app/, mcp/, export/, and watcher/ pass `analyzer.DefaultPricing["sonnet"]`
through it. The function already receives `claude.SessionMeta` which has `ModelUsage
map[string]ModelStats`. The fix is to make `EstimateSessionCost` use `ModelUsage`
when populated, falling back to the existing single-tier pricing when empty.

Once the core function is fixed, the `Pricing ModelPricing` fields on input structs
(`RegressionInput`, `CorrelateInput`, `BaselineInput`) and the `pricing` local vars
at 20+ call sites become fallback-only. They do NOT need to be removed in this
change — they serve as the fallback path for older sessions without `ModelUsage`.

**Caveats:**
- Wave 1 Agent A (core function + analyzer callers) must complete before Wave 2
  agents can run, because the function signature may change (adding a return or
  changing behavior).
- The `EstimateCosts` function in `cost.go` (used by `claudewatch cost` CLI from
  `StatsCache`) is a separate code path that already aggregates across models in
  `StatsCache.ModelUsage`. It does NOT call `EstimateSessionCost` and is out of
  scope for this change. However, it also applies single-tier pricing to the
  aggregate — a separate future fix.

**Pre-implementation scan results:**
- Total items: 1 core function + ~20 call sites + 1 CLI cost path
- Already implemented: `ComputeSessionDashboard` (dashboard.go) and `ParseLiveCostVelocity` (active_live_cost.go) — live path already uses per-model pricing
- Partially implemented: 0 items
- To-do: `EstimateSessionCost` + all historical call sites

Agent adjustments:
- No agents changed to "verify + add tests" — the live path is a different code path
- All agents proceed as planned (to-do)

Estimated time saved: ~0 minutes (no duplicate implementations)

**Estimated times:**
- Scout phase: ~10 min (dependency mapping, interface contracts, IMPL doc)
- Agent execution: ~15 min (3 agents, ~5 min avg, 2 waves with dependency)
- Merge & verification: ~5 min
Total SAW time: ~30 min

Sequential baseline: ~25 min (3 tasks x ~8 min avg sequential time)
Time savings: ~-5 min (marginal — SAW overhead roughly matches parallelization gains)

Recommendation: **Coordination value independent of speed.** The IMPL doc value is
in ensuring the 20+ call sites are handled consistently and the interface contract
for `EstimateSessionCost` is clear. Proceed with SAW for coordination, not speed.

## Scaffolds

No scaffolds needed — agents have independent type ownership. The new helper function
`EstimateSessionCostPerModel` (or the modified `EstimateSessionCost`) lives in
`outcomes.go` which is owned by Agent A alone.

## Known Issues

None identified. All tests pass on current `main`.

## Dependency Graph

```
outcomes.go (EstimateSessionCost) ← ROOT
  ├── regression.go (calls EstimateSessionCost via input.Pricing)
  ├── anomaly.go (calls EstimateSessionCost via input.Pricing / direct args)
  ├── compare.go (calls EstimateSessionCost with local pricing)
  ├── correlate.go (calls EstimateSessionCost via outcomeValue)
  ├── effectiveness.go (calls EstimateSessionCost with pricing arg)
  ├── experiment.go (calls EstimateSessionCost with pricing arg)
  ├── mcp/tools.go (4 call sites)
  ├── mcp/cost_tools.go (1 call site)
  ├── mcp/regression_tools.go (passes Pricing in RegressionInput)
  ├── mcp/analytics_tools.go (passes Pricing in BaselineInput)
  ├── mcp/anomaly_tools.go (passes Pricing in BaselineInput)
  ├── mcp/correlate_tools.go (passes Pricing in CorrelateInput)
  ├── app/sessions.go (2 call sites)
  ├── app/root.go (1 call site)
  ├── app/doctor.go (1 call site)
  ├── app/compare.go (1 call site)
  ├── app/startup.go (2 call sites — RegressionInput + CorrelateInput)
  ├── app/correlate.go (1 call site)
  ├── app/metrics.go (1 call site)
  ├── app/anomalies.go (1 call site)
  ├── app/experiment.go (1 call site)
  ├── app/replay.go (1 call site)
  ├── app/attribute.go (1 call site)
  ├── watcher/watcher.go (1 call site)
  └── export/metrics.go (4 call sites)
```

Leaf nodes: All callers above are leaves — they consume `EstimateSessionCost` but
nothing depends on their output within this change.

Root node: `outcomes.go:EstimateSessionCost` — the function whose behavior changes.

**Cascade candidates** (files that reference cost-related interfaces whose semantics
change but are NOT directly modified):
- `internal/analyzer/cost.go` — `EstimateCosts()` uses `StatsCache` not `SessionMeta`,
  separate code path. Not modified but worth verifying post-merge that the two paths
  produce consistent results for the same data.
- `internal/analyzer/dashboard.go` — `ComputeSessionDashboard()` already uses per-model
  pricing (live path). No changes needed but confirms the pattern to follow.
- `internal/store/baselines.go` — stores `ProjectBaseline.AvgCostUSD`. Values will
  change after this fix (Opus sessions will be costed higher). Not a code change but
  a data migration consideration — existing baselines computed with Sonnet pricing
  will be stale. The `ComputeProjectBaseline` function will recompute correctly on
  next run.

## Interface Contracts

### Modified function (Agent A owns)

```go
// EstimateSessionCost computes the dollar cost of a single session.
// When s.ModelUsage is populated, each model's tokens are priced at the
// correct tier rate (opus/sonnet/haiku) using analyzer.DefaultPricing.
// When s.ModelUsage is empty (older sessions), falls back to single-tier
// pricing using the provided pricing and ratio parameters.
func EstimateSessionCost(s claude.SessionMeta, pricing ModelPricing, ratio CacheRatio) float64
```

**Signature is unchanged.** The `pricing` and `ratio` parameters are retained as the
fallback path. This means zero changes required at call sites for compilation — the
behavioral change is internal to the function.

### New helper function (Agent A owns)

```go
// estimateFromModelUsage computes cost by summing per-model costs from
// SessionMeta.ModelUsage. Each model is classified via ClassifyModelTier
// and priced via getPricingForTier (both already exist in models.go).
// Returns the total cost in USD.
func estimateFromModelUsage(usage map[string]claude.ModelStats) float64
```

This is unexported — used only by `EstimateSessionCost`. No cross-agent dependency.

## File Ownership

| File | Agent | Wave | Action | Depends On |
|------|-------|------|--------|------------|
| `internal/analyzer/outcomes.go` | A | 1 | modify | — |
| `internal/analyzer/outcomes_test.go` | A | 1 | modify | — |
| `internal/analyzer/regression_test.go` | A | 1 | modify (add per-model test) | — |
| `internal/analyzer/anomaly_test.go` | A | 1 | modify (add per-model test) | — |
| `internal/mcp/tools.go` | B | 2 | modify (update tests) | Agent A |
| `internal/mcp/tools_test.go` | B | 2 | modify | Agent A |
| `internal/mcp/cost_tools.go` | B | 2 | modify (update tests) | Agent A |
| `internal/mcp/cost_tools_test.go` | B | 2 | modify | Agent A |
| `internal/mcp/analytics_tools.go` | B | 2 | verify only | Agent A |
| `internal/mcp/analytics_tools_test.go` | B | 2 | modify | Agent A |
| `internal/mcp/anomaly_tools.go` | B | 2 | verify only | Agent A |
| `internal/mcp/anomaly_tools_test.go` | B | 2 | modify | Agent A |
| `internal/mcp/correlate_tools.go` | B | 2 | verify only | Agent A |
| `internal/mcp/correlate_tools_test.go` | B | 2 | modify | Agent A |
| `internal/mcp/regression_tools.go` | B | 2 | verify only | Agent A |
| `internal/mcp/regression_tools_test.go` | B | 2 | modify | Agent A |
| `internal/export/metrics.go` | C | 2 | verify only | Agent A |
| `internal/export/metrics_test.go` | C | 2 | modify | Agent A |
| `internal/app/sessions.go` | C | 2 | verify only | Agent A |
| `internal/app/root.go` | C | 2 | verify only | Agent A |
| `internal/app/doctor.go` | C | 2 | verify only | Agent A |
| `internal/app/compare.go` | C | 2 | verify only | Agent A |
| `internal/app/startup.go` | C | 2 | verify only | Agent A |
| `internal/app/correlate.go` | C | 2 | verify only | Agent A |
| `internal/app/metrics.go` | C | 2 | verify only | Agent A |
| `internal/app/anomalies.go` | C | 2 | verify only | Agent A |
| `internal/app/experiment.go` | C | 2 | verify only | Agent A |
| `internal/app/replay.go` | C | 2 | verify only | Agent A |
| `internal/app/attribute.go` | C | 2 | verify only | Agent A |
| `internal/watcher/watcher.go` | C | 2 | verify only | Agent A |

**Note on "verify only" files:** Since `EstimateSessionCost`'s signature is unchanged,
these files need no source modification. Wave 2 agents verify the build passes and
add/update tests that exercise the per-model path through these callers.

## Wave Structure

```
Wave 1: [A]              <- Core function change + analyzer-internal tests
           | (A completes)
Wave 2:   [B] [C]        <- MCP tool tests (B) + app/export/watcher tests (C)
```

## Agent Prompts

### Wave 1 Agent A: Core EstimateSessionCost per-model pricing

You are Wave 1 Agent A. You modify `EstimateSessionCost` to use per-model pricing
from `SessionMeta.ModelUsage` when available, and add tests covering the new behavior.

#### 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a 2>/dev/null || true
```

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a"
if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"; echo "Expected: $EXPECTED_DIR"; echo "Actual: $ACTUAL_DIR"; exit 1
fi
ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave1-agent-a"
if [ "$ACTUAL_BRANCH" != "$EXPECTED_BRANCH" ]; then
  echo "ISOLATION FAILURE: Wrong branch"; echo "Expected: $EXPECTED_BRANCH"; echo "Actual: $ACTUAL_BRANCH"; exit 1
fi
git worktree list | grep -q "$EXPECTED_BRANCH" || { echo "ISOLATION FAILURE: Worktree not in git worktree list"; exit 1; }
echo "Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

#### 1. File Ownership

You own these files. Do not touch any other files.
- `internal/analyzer/outcomes.go` - modify
- `internal/analyzer/outcomes_test.go` - modify
- `internal/analyzer/regression_test.go` - modify
- `internal/analyzer/anomaly_test.go` - modify

#### 2. Interfaces You Must Implement

```go
// Modified behavior (same signature):
func EstimateSessionCost(s claude.SessionMeta, pricing ModelPricing, ratio CacheRatio) float64

// New unexported helper:
func estimateFromModelUsage(usage map[string]claude.ModelStats) float64
```

#### 3. Interfaces You May Call

```go
// Already in models.go:
func ClassifyModelTier(modelName string) ModelTier
func getPricingForTier(tier ModelTier) ModelPricing

// Already in cost.go:
func tokensToCost(tokens int64, perMillion float64) float64
var DefaultPricing map[string]ModelPricing

// From claude package:
type SessionMeta struct { ModelUsage map[string]ModelStats; ... }
type ModelStats struct { InputTokens, OutputTokens, CacheReadInputTokens, CacheCreationInputTokens int }
```

#### 4. What to Implement

Read `internal/analyzer/outcomes.go` first. The current `EstimateSessionCost` function
uses only `s.InputTokens` and `s.OutputTokens` with the provided single-tier `pricing`.

**Modify `EstimateSessionCost` to:**

1. Check if `len(s.ModelUsage) > 0`.
2. If yes: call `estimateFromModelUsage(s.ModelUsage)` and return its result.
3. If no: fall through to the existing single-tier calculation (unchanged).

**Implement `estimateFromModelUsage`:**

1. Iterate over `s.ModelUsage` (map of model name to `ModelStats`).
2. For each model: call `ClassifyModelTier(modelName)` then `getPricingForTier(tier)`.
3. Compute cost as:
   ```
   tokensToCost(int64(stats.InputTokens), pricing.InputPerMillion) +
   tokensToCost(int64(stats.OutputTokens), pricing.OutputPerMillion) +
   tokensToCost(int64(stats.CacheReadInputTokens), pricing.CacheReadPerMillion) +
   tokensToCost(int64(stats.CacheCreationInputTokens), pricing.CacheWritePerMillion)
   ```
4. Sum across all models and return.

This matches the pattern already used in `ComputeSessionDashboard` (dashboard.go
lines 46-53) and `AnalyzeModelsFromSessions` (models.go lines 272-280). Follow
those as reference implementations.

**Key edge cases:**
- `ModelUsage` is non-nil but empty map: treat as "not populated", fall through to
  single-tier path.
- Unknown model name (TierOther): `getPricingForTier` already defaults to Sonnet.
- `CacheRatio` parameter is unused in the per-model path because `ModelStats` has
  actual `CacheReadInputTokens` and `CacheCreationInputTokens` fields (real data,
  not estimated). This is an accuracy improvement over the fallback path.

#### 5. Tests to Write

1. `TestEstimateSessionCost_PerModel` — Session with `ModelUsage` containing an Opus
   model: verify cost uses Opus pricing ($15/M input, $75/M output) not Sonnet.
2. `TestEstimateSessionCost_PerModelMulti` — Session with both Sonnet and Opus in
   `ModelUsage`: verify cost is the sum of per-model costs.
3. `TestEstimateSessionCost_FallbackWhenNoModelUsage` — Session with nil `ModelUsage`:
   verify the existing single-tier behavior is preserved exactly.
4. `TestEstimateSessionCost_EmptyModelUsage` — Session with `ModelUsage` set to an
   empty map: verify fallback to single-tier pricing.
5. `TestEstimateSessionCost_WithCacheTokensPerModel` — Session with `ModelUsage`
   containing cache read/write tokens: verify cache tokens are priced at the correct
   tier rates.
6. `TestEstimateFromModelUsage` — Direct unit test of the helper.
7. In `regression_test.go`: `TestComputeRegressionStatus_PerModelCost` — Verify that
   `ComputeRegressionStatus` with sessions having `ModelUsage` produces correct cost
   comparisons.
8. In `anomaly_test.go`: `TestDetectAnomalies_PerModelCost` — Verify anomaly detection
   uses per-model pricing when `ModelUsage` is populated.

#### 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/analyzer -run "TestEstimateSessionCost|TestEstimateFromModelUsage|TestComputeRegressionStatus_PerModel|TestDetectAnomalies_PerModel|TestAnalyzeOutcomes"
```

#### 7. Constraints

- Do NOT change the function signature of `EstimateSessionCost`. The `pricing` and
  `ratio` params remain as fallback for sessions without `ModelUsage`.
- Do NOT modify `models.go`, `cost.go`, or `dashboard.go`. Those are out of scope.
- Do NOT touch any file outside the analyzer package.
- The `CacheRatio` parameter is intentionally unused in the per-model path. When
  `ModelUsage` is populated, it contains real cache token counts. Do not apply the
  ratio multiplier to them.

#### 8. Report

Commit your changes, then append your completion report to this IMPL doc.

---

### Wave 2 Agent B: MCP tool integration tests

You are Wave 2 Agent B. You verify that MCP tool handlers work correctly with the
updated `EstimateSessionCost` and add tests exercising the per-model pricing path
through MCP tools.

#### 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-b 2>/dev/null || true
```

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-b"
if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"; echo "Expected: $EXPECTED_DIR"; echo "Actual: $ACTUAL_DIR"; exit 1
fi
ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave2-agent-b"
if [ "$ACTUAL_BRANCH" != "$EXPECTED_BRANCH" ]; then
  echo "ISOLATION FAILURE: Wrong branch"; echo "Expected: $EXPECTED_BRANCH"; echo "Actual: $ACTUAL_BRANCH"; exit 1
fi
git worktree list | grep -q "$EXPECTED_BRANCH" || { echo "ISOLATION FAILURE: Worktree not in git worktree list"; exit 1; }
echo "Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

#### 1. File Ownership

You own these files. Do not touch any other files.
- `internal/mcp/tools_test.go` - modify
- `internal/mcp/cost_tools_test.go` - modify
- `internal/mcp/analytics_tools_test.go` - modify
- `internal/mcp/anomaly_tools_test.go` - modify
- `internal/mcp/correlate_tools_test.go` - modify
- `internal/mcp/regression_tools_test.go` - modify

#### 2. Interfaces You Must Implement

No new interfaces. Test-only changes.

#### 3. Interfaces You May Call

```go
// From analyzer package (modified by Agent A):
func EstimateSessionCost(s claude.SessionMeta, pricing ModelPricing, ratio CacheRatio) float64

// Existing:
var DefaultPricing map[string]ModelPricing
func NoCacheRatio() CacheRatio
```

#### 4. What to Implement

For each MCP tool test file, add test cases that create `claude.SessionMeta` objects
with `ModelUsage` populated (containing Opus-tier models) and verify the cost
calculations in the tool handler's output reflect Opus pricing, not Sonnet.

Read the existing test files first to understand the test patterns. Most MCP tool tests
use a `testServer` helper with mock session data. Add parallel test cases that:
1. Create sessions with `ModelUsage` containing `"claude-3-opus-20240229"` entries.
2. Verify the returned cost is higher than Sonnet-tier pricing would produce.
3. Create sessions with mixed models (Opus + Haiku) and verify the cost is the weighted sum.

Focus on `tools_test.go` (session stats, recent sessions, cost summary, SAW sessions)
and `cost_tools_test.go` (cost summary) as the highest-impact tests. The others
(analytics, anomaly, correlate, regression) pass `Pricing` through input structs
that still work as fallback — add one test per file confirming the per-model path
takes precedence.

#### 5. Tests to Write

1. `TestHandleGetSessionStats_PerModelCost` — Verify session cost uses per-model pricing.
2. `TestHandleGetRecentSessions_PerModelCost` — Verify recent sessions list shows correct per-model costs.
3. `TestHandleGetCostSummary_PerModelCost` — Verify cost summary aggregates use per-model pricing.
4. `TestHandleGetCausalInsights_PerModelCost` — Verify factor analysis uses per-model costs.
5. `TestHandleGetProjectAnomalies_PerModelCost` — Verify anomaly detection uses per-model costs.
6. `TestHandleGetRegressionStatus_PerModelCost` — Verify regression detection uses per-model costs.

#### 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/mcp -run "PerModelCost"
```

#### 7. Constraints

- Do NOT modify any non-test files. The MCP tool handler source files (`tools.go`,
  `cost_tools.go`, etc.) should NOT need changes — the `DefaultPricing["sonnet"]`
  they pass is now the fallback, and the per-model path is triggered by `ModelUsage`
  being populated in the session data.
- If a test fails because a handler does something unexpected with per-model data,
  report it as an `out_of_scope_deps` item — do not fix the handler source.

#### 8. Report

Commit your changes, then append your completion report to this IMPL doc.

---

### Wave 2 Agent C: App, export, and watcher integration tests

You are Wave 2 Agent C. You verify that app commands, export formatters, and the
watcher work correctly with the updated `EstimateSessionCost` and add tests exercising
the per-model pricing path.

#### 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c 2>/dev/null || true
```

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c"
if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"; echo "Expected: $EXPECTED_DIR"; echo "Actual: $ACTUAL_DIR"; exit 1
fi
ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave2-agent-c"
if [ "$ACTUAL_BRANCH" != "$EXPECTED_BRANCH" ]; then
  echo "ISOLATION FAILURE: Wrong branch"; echo "Expected: $EXPECTED_BRANCH"; echo "Actual: $ACTUAL_BRANCH"; exit 1
fi
git worktree list | grep -q "$EXPECTED_BRANCH" || { echo "ISOLATION FAILURE: Worktree not in git worktree list"; exit 1; }
echo "Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

#### 1. File Ownership

You own these files. Do not touch any other files.
- `internal/export/metrics_test.go` - modify
- `internal/app/scan_test.go` - modify (if per-model cost is testable here)
- `internal/app/compare_test.go` - modify
- `internal/app/anomalies_test.go` - modify
- `internal/app/hook_test.go` - modify (if applicable)

#### 2. Interfaces You Must Implement

No new interfaces. Test-only changes.

#### 3. Interfaces You May Call

```go
// From analyzer package (modified by Agent A):
func EstimateSessionCost(s claude.SessionMeta, pricing ModelPricing, ratio CacheRatio) float64

// Existing:
var DefaultPricing map[string]ModelPricing
func NoCacheRatio() CacheRatio
func CompareSAWVsSequential(...) ComparisonReport
func DetectAnomalies(...) []store.AnomalyResult
```

#### 4. What to Implement

Add test cases to the existing test files that exercise the per-model pricing path
through the app commands and export system. The pattern is the same as Agent B:
create `claude.SessionMeta` objects with `ModelUsage` populated and verify downstream
cost calculations are correct.

Read the existing test files first. Key areas:
1. `export/metrics_test.go` — The Prometheus and JSON exporters call
   `EstimateSessionCost`. Add tests with Opus sessions and verify exported cost
   metrics reflect Opus pricing.
2. `app/compare_test.go` — `CompareSAWVsSequential` calls `EstimateSessionCost`.
   Add a test with mixed-model sessions.
3. `app/anomalies_test.go` — `DetectAnomalies` calls `EstimateSessionCost`. Add a
   test with Opus sessions.

For files marked "verify only" in the file ownership table, the build passing is
sufficient verification — no test changes needed unless you identify a gap.

#### 5. Tests to Write

1. `TestExportPrometheus_PerModelCost` — Verify Prometheus metrics export uses per-model costs.
2. `TestExportJSON_PerModelCost` — Verify JSON export uses per-model costs.
3. `TestCompareSAWVsSequential_PerModelCost` — Verify SAW comparison uses per-model costs.
4. `TestDetectAnomalies_PerModelCost_AppLevel` — Verify anomaly CLI uses per-model costs.

#### 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/export -run "PerModelCost"
go test ./internal/app -run "PerModelCost"
```

#### 7. Constraints

- Do NOT modify any non-test files. The app command files, export files, and
  `watcher.go` should NOT need source changes — the per-model pricing is handled
  internally by `EstimateSessionCost`.
- If a test reveals that a handler constructs `SessionMeta` without propagating
  `ModelUsage` from the parsed data, report it as an `out_of_scope_deps` item.

#### 8. Report

Commit your changes, then append your completion report to this IMPL doc.

## Wave Execution Loop

After each wave completes, work through the Orchestrator Post-Merge Checklist
below in order. The merge procedure detail is in `saw-merge.md`. Key principles:
- Read completion reports first — a `status: partial` or `status: blocked` blocks
  the merge entirely. No partial merges.
- Interface deviations with `downstream_action_required: true` must be propagated
  to downstream agent prompts before that wave launches.
- Post-merge verification is the real gate. Agents pass in isolation; the merged
  codebase surfaces cross-package failures none of them saw individually.
- Fix before proceeding. Do not launch the next wave with a broken build.

## Orchestrator Post-Merge Checklist

After wave 1 completes:

- [ ] Read all agent completion reports — confirm all `status: complete`; if any
      `partial` or `blocked`, stop and resolve before merging
- [ ] Conflict prediction — cross-reference `files_changed` lists; flag any file
      appearing in >1 agent's list before touching the working tree
- [ ] Review `interface_deviations` — update downstream agent prompts for any
      item with `downstream_action_required: true`
- [ ] Merge each agent: `git merge --no-ff <branch> -m "Merge wave1-agent-a: per-model EstimateSessionCost"`
- [ ] Worktree cleanup: `git worktree remove <path>` + `git branch -d <branch>` for each
- [ ] Post-merge verification:
      - [ ] Linter auto-fix pass: `gofmt -w .`
      - [ ] `go build ./... && go vet ./... && go test ./...`
- [ ] Fix any cascade failures — pay attention to cascade candidates listed above
- [ ] Tick status checkboxes in this IMPL doc for completed agents
- [ ] Update interface contracts for any deviations logged by agents
- [ ] Apply `out_of_scope_deps` fixes flagged in completion reports
- [ ] Feature-specific steps:
      - [ ] Verify `go test ./internal/analyzer -run TestEstimateSessionCost` shows new per-model tests passing
      - [ ] Spot-check: run `claudewatch sessions` and compare cost for a known Opus session against manual calculation
- [ ] Commit: `git commit -m "feat: per-model historical cost in EstimateSessionCost"`
- [ ] Launch wave 2

After wave 2 completes:

- [ ] Read all agent completion reports — confirm all `status: complete`
- [ ] Conflict prediction — cross-reference `files_changed` lists
- [ ] Review `interface_deviations`
- [ ] Merge Agent B: `git merge --no-ff <branch> -m "Merge wave2-agent-b: MCP per-model cost tests"`
- [ ] Merge Agent C: `git merge --no-ff <branch> -m "Merge wave2-agent-c: app/export per-model cost tests"`
- [ ] Worktree cleanup for both agents
- [ ] Post-merge verification:
      - [ ] Linter auto-fix pass: `gofmt -w .`
      - [ ] `go build ./... && go vet ./... && go test ./...`
- [ ] Fix any cascade failures
- [ ] Tick status checkboxes
- [ ] Feature-specific steps:
      - [ ] Verify all `PerModelCost` tests pass: `go test ./... -run PerModelCost`
- [ ] Commit: `git commit -m "test: per-model cost coverage for MCP, app, and export"`
- [ ] Launch next wave (or pause for review if not `--auto`)

## Status

| Wave | Agent | Description | Status |
|------|-------|-------------|--------|
| 1 | A | Core `EstimateSessionCost` per-model pricing + analyzer tests | TO-DO |
| 2 | B | MCP tool integration tests for per-model pricing | COMPLETE |
| 2 | C | App/export/watcher integration tests for per-model pricing | TO-DO |
| — | Orch | Post-merge verification + binary install | TO-DO |

## Wave 2 Agent B — Completion Report

```yaml
agent: wave2-agent-b
status: complete
branch: wave2-agent-b

files_changed:
  - internal/mcp/tools_test.go
  - internal/mcp/cost_tools_test.go
  - internal/mcp/correlate_tools_test.go
  - internal/mcp/anomaly_tools_test.go
  - internal/mcp/regression_tools_test.go

tests_added:
  - TestHandleGetSessionStats_PerModelCost (tools_test.go)
  - TestHandleGetRecentSessions_PerModelCost (tools_test.go)
  - TestHandleGetSessionStats_MixedModels (tools_test.go)
  - TestGetCostSummary_PerModelCost (cost_tools_test.go)
  - TestHandleGetCausalInsights_PerModelCost (correlate_tools_test.go)
  - TestGetProjectAnomalies_PerModelCost (anomaly_tools_test.go)
  - TestGetRegressionStatus_PerModelCost (regression_tools_test.go)

tests_passing: 7/7

interface_deviations: []

out_of_scope_deps: []

notes: |
  Added writeSessionMetaWithModels helper to tools_test.go that creates
  session meta cache JSON with model_usage populated, triggering the
  per-model pricing path in EstimateSessionCost. All MCP tool handlers
  correctly propagate ModelUsage from parsed session data through to
  EstimateSessionCost. No handler source changes needed. The helper is
  accessible to all test files in the mcp package.

  Key verification pattern: each test creates sessions with specific model
  tiers (Opus, Sonnet, Haiku) and asserts that the resulting cost matches
  per-model pricing rather than the single-tier Sonnet fallback. The
  mixed-model test (Opus + Haiku) verifies weighted-sum cost calculation.
```
