<!-- scout v0.3.5 | generated 2026-03-02 -->
# IMPL: Automated Regression Detection

## Feature Summary

Add automated regression detection for claudewatch: a new `doctor` check that
warns when a project's friction rate or cost-per-commit has regressed beyond a
configurable multiplier relative to the stored baseline, and a new
`get_regression_status` MCP tool that returns the current regression status on
demand. Rolling baseline comparison happens without requiring manual
`track --compare` snapshots.

---

### Suitability Assessment

Verdict: SUITABLE

The work cleanly splits into two disjoint agents: one owns the core regression
logic in a new `internal/analyzer/regression.go` file plus its test file, and
the other owns the new `internal/mcp/regression_tools.go` file plus its test
file and the single-line `addTools` registration in `tools.go`. The `doctor`
check addition is a minor append to `internal/app/doctor.go`, which is
orchestrator-owned and applied at post-merge. All cross-agent interfaces are
fully definable before implementation starts: Agent A produces a pure function
`ComputeRegressionStatus` that Agent B calls. No investigation-first items
exist — the behavior is fully specified. Both sessions and baselines are
readable via well-understood existing infrastructure.

Estimated times:
- Scout phase: ~15 min
- Agent execution: ~20 min (2 agents × ~20 min avg, fully parallel)
- Merge & verification: ~10 min
Total SAW time: ~45 min

Sequential baseline: ~60 min
Time savings: ~15 min (25% faster)

Recommendation: Clear speedup. Two well-isolated units with a defined interface.

---

### Known Issues

None identified. `go build ./...` and `go vet ./...` pass cleanly on the current
codebase (verified via existing CI). No pre-existing test failures relevant to
the files being changed.

---

### Dependency Graph

```
[existing] store.ProjectBaseline        (store/baselines.go — read-only)
[existing] store.DB.GetProjectBaseline  (store/baselines.go — read-only)
[existing] analyzer.ComputeProjectBaseline (analyzer/anomaly.go — read-only)
[existing] analyzer.EstimateSessionCost    (analyzer/cost.go — read-only)

Agent A (Wave 1):
  creates: internal/analyzer/regression.go
  creates: internal/analyzer/regression_test.go
  exports: RegressionStatus (type)
           RegressionInput  (type)
           ComputeRegressionStatus(RegressionInput) RegressionStatus

Agent B (Wave 1):
  creates: internal/mcp/regression_tools.go
  creates: internal/mcp/regression_tools_test.go
  calls:   analyzer.ComputeRegressionStatus  <-- from Agent A
  calls:   store.DB.GetProjectBaseline       <-- existing
  adds:    addRegressionTools(s) registration in tools.go

Orchestrator (post-merge):
  modifies: internal/app/doctor.go  — append checkRegressionStatus() call
  modifies: internal/mcp/tools.go   — append addRegressionTools(s) call
            (if Agent B did not already add it; see note in Agent B prompt)
```

The DAG is a single two-layer structure: Agent A's output is consumed by Agent
B. Both agents can be written against the pre-defined interface contract; Agent
B's file will not compile until merge, but it can reference the contract symbol
and the build blocker is noted in its completion report.

---

### Interface Contracts

These signatures are binding. Agents must implement them exactly as written.

#### Types produced by Agent A

```go
// package analyzer

// RegressionInput holds everything needed to compute the regression status
// for one project. All fields are required unless marked optional.
type RegressionInput struct {
    // Project is the short project name (filepath.Base of ProjectPath).
    Project string

    // Baseline is the stored baseline for the project. If nil,
    // ComputeRegressionStatus returns RegressionStatus with HasBaseline=false.
    Baseline *store.ProjectBaseline

    // RecentSessions are the most recent N sessions for the project,
    // used to compute the rolling current metrics. Callers should pass
    // the last min(10, len(projectSessions)) sessions. If fewer than 3
    // sessions are available, Regressed will always be false and
    // InsufficientData will be true.
    RecentSessions []claude.SessionMeta

    // Facets are the session facets for all project sessions (used to
    // compute friction counts). May be nil; treated as empty.
    Facets []claude.SessionFacet

    // Pricing and CacheRatio are used to compute session costs.
    Pricing    ModelPricing
    CacheRatio CacheRatio

    // Threshold is the regression multiplier: current > Threshold * baseline
    // triggers a regression. Must be > 1; defaults to 1.5 if <= 1.
    Threshold float64
}

// RegressionStatus is the result of a rolling baseline regression check.
type RegressionStatus struct {
    // Project is the project name.
    Project string `json:"project"`

    // HasBaseline is false when no stored baseline exists for this project.
    // When false, all other fields except Project are zero values.
    HasBaseline bool `json:"has_baseline"`

    // InsufficientData is true when fewer than 3 recent sessions exist,
    // making a reliable current estimate impossible.
    InsufficientData bool `json:"insufficient_data"`

    // Regressed is true if any monitored metric has exceeded the threshold.
    Regressed bool `json:"regressed"`

    // FrictionRegressed is true if current friction rate > Threshold * baseline friction rate.
    FrictionRegressed bool `json:"friction_regressed"`

    // CostRegressed is true if current avg cost-per-session > Threshold * baseline avg cost.
    CostRegressed bool `json:"cost_regressed"`

    // CurrentFrictionRate is the rolling average friction rate across RecentSessions.
    // Friction rate is defined as: sessions_with_any_friction / total_sessions.
    CurrentFrictionRate float64 `json:"current_friction_rate"`

    // BaselineFrictionRate is baseline.AvgFriction (sessions-with-friction rate stored at baseline time).
    BaselineFrictionRate float64 `json:"baseline_friction_rate"`

    // CurrentAvgCostUSD is the rolling average cost per session across RecentSessions.
    CurrentAvgCostUSD float64 `json:"current_avg_cost_usd"`

    // BaselineAvgCostUSD is baseline.AvgCostUSD.
    BaselineAvgCostUSD float64 `json:"baseline_avg_cost_usd"`

    // Threshold is the multiplier used for comparison (as provided in RegressionInput).
    Threshold float64 `json:"threshold"`

    // Message is a human-readable summary: either "no regression detected",
    // "no baseline available", "insufficient data", or a description of which
    // metrics regressed and by how much.
    Message string `json:"message"`
}
```

#### Function produced by Agent A

```go
// package analyzer

// ComputeRegressionStatus computes the rolling regression status for a project
// by comparing recent session metrics against the stored baseline.
// It is a pure function: it does not open the database or read files.
func ComputeRegressionStatus(input RegressionInput) RegressionStatus
```

#### Function consumed by Agent B (calls Agent A's output)

```go
// Agent B calls:
analyzer.ComputeRegressionStatus(analyzer.RegressionInput{...}) analyzer.RegressionStatus
```

#### MCP result type produced by Agent B

```go
// package mcp

// RegressionStatusResult is the JSON-serializable result of get_regression_status.
// It wraps analyzer.RegressionStatus with no additional fields.
type RegressionStatusResult = analyzer.RegressionStatus
```

Note: Agent B may use a type alias or embed the struct — the JSON output shape
must match `analyzer.RegressionStatus` field-for-field.

#### Doctor check function produced by Orchestrator (post-merge)

The orchestrator adds this call to `internal/app/doctor.go` after Agent A and B
merge:

```go
// In runDoctor, after the anomaly baselines check:
if db != nil {
    checks = append(checks, checkRegressionStatus(db, sessions, cfg))
}
```

The helper function signature (implemented by orchestrator at post-merge, NOT by
either agent):

```go
func checkRegressionStatus(db *store.DB, sessions []claude.SessionMeta, cfg *config.Config) doctorCheck
```

This function calls `db.ListProjectBaselines()`, groups sessions by project,
calls `analyzer.ComputeRegressionStatus` for each project with a baseline, and
returns a single `doctorCheck` that passes when no regressions are detected.

---

### File Ownership

| File | Agent | Wave | Depends On |
|------|-------|------|------------|
| `internal/analyzer/regression.go` | A | 1 | existing: store.ProjectBaseline, analyzer.EstimateSessionCost, sessionFriction (package-private) |
| `internal/analyzer/regression_test.go` | A | 1 | regression.go (own file) |
| `internal/mcp/regression_tools.go` | B | 1 | analyzer.ComputeRegressionStatus (Agent A), store.DB.GetProjectBaseline (existing) |
| `internal/mcp/regression_tools_test.go` | B | 1 | regression_tools.go (own file) |
| `internal/mcp/tools.go` | B | 1 | adds `addRegressionTools(s)` call (append-only) |
| `internal/app/doctor.go` | Orchestrator | post-merge | checkRegressionStatus (new helper in doctor.go) |

Orchestrator-owned files are modified only after all agents complete and merge.

---

### Wave Structure

```
Wave 1: [A] [B]   <- parallel (Agent B references Agent A's types; will have
         |          build-blocker until merge — see Agent B constraints)
         | (A+B complete and committed)
         |
Orchestrator post-merge:
  - git merge wave1-agent-a, wave1-agent-b into main
  - go build ./... && go vet ./... && go test ./...
  - Fix any build blockers from Agent B's out-of-scope note
  - Add checkRegressionStatus doctor check to internal/app/doctor.go
  - go build ./... && go vet ./... && go test ./...
  - Commit
```

---

### Agent Prompts

---

# Wave 1 Agent A: Regression logic in analyzer package

You are Wave 1 Agent A. Your task is to implement the `ComputeRegressionStatus`
pure function and its supporting types in a new file `internal/analyzer/regression.go`.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a 2>/dev/null || true
```

**Step 2: Verify isolation (strict fail-fast after self-correction attempt)**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory (even after cd attempt)"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave1-agent-a"

if [ "$ACTUAL_BRANCH" != "$EXPECTED_BRANCH" ]; then
  echo "ISOLATION FAILURE: Wrong branch"
  echo "Expected: $EXPECTED_BRANCH"
  echo "Actual: $ACTUAL_BRANCH"
  exit 1
fi

git worktree list | grep -q "$EXPECTED_BRANCH" || {
  echo "ISOLATION FAILURE: Worktree not in git worktree list"
  exit 1
}

echo "Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

**If verification fails:** Write error to completion report and exit. Do NOT modify files.

**If verification passes:** Document briefly in completion report, then proceed.

## 1. File Ownership

You own these files. Do not touch any other files.

- `internal/analyzer/regression.go` — create
- `internal/analyzer/regression_test.go` — create

## 2. Interfaces You Must Implement

```go
// package analyzer

type RegressionInput struct {
    Project        string
    Baseline       *store.ProjectBaseline
    RecentSessions []claude.SessionMeta
    Facets         []claude.SessionFacet
    Pricing        ModelPricing
    CacheRatio     CacheRatio
    Threshold      float64
}

type RegressionStatus struct {
    Project              string  `json:"project"`
    HasBaseline          bool    `json:"has_baseline"`
    InsufficientData     bool    `json:"insufficient_data"`
    Regressed            bool    `json:"regressed"`
    FrictionRegressed    bool    `json:"friction_regressed"`
    CostRegressed        bool    `json:"cost_regressed"`
    CurrentFrictionRate  float64 `json:"current_friction_rate"`
    BaselineFrictionRate float64 `json:"baseline_friction_rate"`
    CurrentAvgCostUSD    float64 `json:"current_avg_cost_usd"`
    BaselineAvgCostUSD   float64 `json:"baseline_avg_cost_usd"`
    Threshold            float64 `json:"threshold"`
    Message              string  `json:"message"`
}

func ComputeRegressionStatus(input RegressionInput) RegressionStatus
```

## 3. Interfaces You May Call

These already exist in the `internal/analyzer` package (same package as your
new file — call them directly, no import needed):

```go
// From anomaly.go — package-private helpers, accessible within package:
func sessionFriction(sess claude.SessionMeta, facetsByID map[string]*claude.SessionFacet) int
func buildFacetIndex(facets []claude.SessionFacet) map[string]*claude.SessionFacet

// From cost.go:
func EstimateSessionCost(sess claude.SessionMeta, pricing ModelPricing, cacheRatio CacheRatio) float64
```

Types from `store` package:
```go
store.ProjectBaseline  // fields: AvgCostUSD, StddevCostUSD, AvgFriction, StddevFriction, etc.
```

Types from `claude` package:
```go
claude.SessionMeta
claude.SessionFacet
```

## 4. What to Implement

Read these files first to understand existing patterns before writing:
- `internal/analyzer/anomaly.go` — understand `sessionFriction`, `buildFacetIndex`, `EstimateSessionCost` usage
- `internal/store/types.go` — understand `ProjectBaseline` fields

**`ComputeRegressionStatus` behavior:**

1. **No baseline guard:** If `input.Baseline == nil`, return
   `RegressionStatus{Project: input.Project, HasBaseline: false, Message: "no baseline available"}`.

2. **Insufficient data guard:** If `len(input.RecentSessions) < 3`, return
   `RegressionStatus{Project: input.Project, HasBaseline: true, InsufficientData: true, Message: "insufficient data: fewer than 3 recent sessions"}`.

3. **Threshold default:** If `input.Threshold <= 1`, use `1.5`.

4. **Compute current metrics** from `input.RecentSessions`:
   - Build facet index from `input.Facets` using `buildFacetIndex`.
   - Compute `currentFrictionRate`: count sessions where `sessionFriction(sess, facetsByID) > 0`,
     divide by `len(input.RecentSessions)`. This is a rate (0.0–1.0).
   - Compute `currentAvgCost`: mean of `EstimateSessionCost(sess, input.Pricing, input.CacheRatio)`
     across all `input.RecentSessions`.

5. **Compare against baseline:**
   - `frictionRegressed = currentFrictionRate > threshold * baseline.AvgFriction`
     (but only if `baseline.AvgFriction > 0`; skip comparison if baseline is zero).
   - `costRegressed = currentAvgCost > threshold * baseline.AvgCostUSD`
     (but only if `baseline.AvgCostUSD > 0`; skip comparison if baseline is zero).
   - `regressed = frictionRegressed || costRegressed`

6. **Build message:**
   - No regression: `"no regression detected"`
   - Both regressed: `"friction rate regressed (%.2f vs baseline %.2f) and cost regressed ($%.4f vs baseline $%.4f)"` (using fmt.Sprintf)
   - Friction only: `"friction rate regressed (%.2f vs baseline %.2f, threshold %.1fx)"`
   - Cost only: `"cost regressed ($%.4f vs baseline $%.4f, threshold %.1fx)"`

7. **Return** fully populated `RegressionStatus`.

**Edge cases:**
- `input.Facets` may be nil — pass it directly to `buildFacetIndex` which handles nil.
- Baseline fields `AvgFriction` and `AvgCostUSD` may be 0 — skip that comparison rather than dividing by zero or producing spurious regressions.
- `input.RecentSessions` contains all recent sessions for the project. Callers cap
  at 10; the function does not need to cap internally.

## 5. Tests to Write

Write tests in `internal/analyzer/regression_test.go`. Use `package analyzer`
(white-box, same package — you can call package-private helpers if needed).

1. `TestComputeRegressionStatus_NoBaseline` — nil Baseline returns HasBaseline=false, Regressed=false
2. `TestComputeRegressionStatus_InsufficientData` — 2 sessions returns InsufficientData=true
3. `TestComputeRegressionStatus_NoRegression` — sessions matching baseline exactly, Regressed=false
4. `TestComputeRegressionStatus_FrictionRegression` — sessions with high friction trigger FrictionRegressed=true
5. `TestComputeRegressionStatus_CostRegression` — sessions with high cost trigger CostRegressed=true
6. `TestComputeRegressionStatus_BothRegressed` — both friction and cost above threshold
7. `TestComputeRegressionStatus_ThresholdDefault` — Threshold=0 defaults to 1.5
8. `TestComputeRegressionStatus_ZeroBaselineSkipped` — baseline AvgFriction=0 does not trigger regression even with non-zero current rate
9. `TestComputeRegressionStatus_MessageContent` — verifies message strings for regression and no-regression cases
10. `TestComputeRegressionStatus_ExactlyAtThreshold` — current = threshold * baseline does NOT trigger (must be strictly greater than)

To create synthetic sessions for tests, construct `claude.SessionMeta` structs
directly with `InputTokens` / `OutputTokens` for cost tests, and write
`claude.SessionFacet` structs with `FrictionCounts` for friction tests. Look at
`internal/analyzer/anomaly_test.go` for the existing test pattern for building
synthetic `SessionMeta` and `SessionFacet` values.

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
go build ./...
go vet ./...
go test ./internal/analyzer/...
```

All must pass before reporting completion.

## 7. Constraints

- `ComputeRegressionStatus` must be a **pure function**: no file I/O, no database
  access, no global state. All data arrives via `RegressionInput`.
- Do not modify any existing files — only create new ones.
- If you discover that `sessionFriction` or `buildFacetIndex` are not exported and
  you cannot call them from the same package, confirm you are using `package analyzer`
  (not `package analyzer_test`) — they are package-private and accessible within
  the package.
- Do not add new dependencies to `go.mod`.

## 8. Report

**Before reporting:** Commit your changes to your worktree branch:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
git add internal/analyzer/regression.go internal/analyzer/regression_test.go
git commit -m "wave1-agent-a: add ComputeRegressionStatus to analyzer package"
```

Append your completion report to the IMPL doc at
`/Users/dayna.blackwell/code/claudewatch/docs/IMPL-regression-detection.md`
under `### Agent A - Completion Report`. Do not edit any earlier section.

```yaml
### Agent A - Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-a
commit: {sha}
files_changed: []
files_created:
  - internal/analyzer/regression.go
  - internal/analyzer/regression_test.go
interface_deviations:
  - "[] or exact description"
out_of_scope_deps:
  - "[] or file: ..., change: ..., reason: ..."
tests_added:
  - TestComputeRegressionStatus_NoBaseline
  - TestComputeRegressionStatus_InsufficientData
  - TestComputeRegressionStatus_NoRegression
  - TestComputeRegressionStatus_FrictionRegression
  - TestComputeRegressionStatus_CostRegression
  - TestComputeRegressionStatus_BothRegressed
  - TestComputeRegressionStatus_ThresholdDefault
  - TestComputeRegressionStatus_ZeroBaselineSkipped
  - TestComputeRegressionStatus_MessageContent
  - TestComputeRegressionStatus_ExactlyAtThreshold
verification: PASS | FAIL ({command} - N/N tests)
```

---

# Wave 1 Agent B: MCP tool and doctor registration

You are Wave 1 Agent B. Your task is to implement the `get_regression_status`
MCP tool handler and its registration in `internal/mcp/regression_tools.go`,
plus tests in `internal/mcp/regression_tools_test.go`, and append the
`addRegressionTools(s)` call in `internal/mcp/tools.go`.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b 2>/dev/null || true
```

**Step 2: Verify isolation (strict fail-fast after self-correction attempt)**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory (even after cd attempt)"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave1-agent-b"

if [ "$ACTUAL_BRANCH" != "$EXPECTED_BRANCH" ]; then
  echo "ISOLATION FAILURE: Wrong branch"
  echo "Expected: $EXPECTED_BRANCH"
  echo "Actual: $ACTUAL_BRANCH"
  exit 1
fi

git worktree list | grep -q "$EXPECTED_BRANCH" || {
  echo "ISOLATION FAILURE: Worktree not in git worktree list"
  exit 1
}

echo "Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

**If verification fails:** Write error to completion report and exit. Do NOT modify files.

**If verification passes:** Document briefly in completion report, then proceed.

## 1. File Ownership

You own these files. Do not touch any other files except as noted below.

- `internal/mcp/regression_tools.go` — create
- `internal/mcp/regression_tools_test.go` — create
- `internal/mcp/tools.go` — modify (append `addRegressionTools(s)` call only)

## 2. Interfaces You Must Implement

```go
// package mcp

// addRegressionTools registers the get_regression_status MCP tool.
func addRegressionTools(s *Server)

// handleGetRegressionStatus is the handler for the get_regression_status tool.
// Attached to *Server as a method.
func (s *Server) handleGetRegressionStatus(args json.RawMessage) (any, error)
```

The JSON result shape is `analyzer.RegressionStatus` — either define a type
alias `type RegressionStatusResult = analyzer.RegressionStatus` or return the
struct directly. The MCP framework accepts `any`.

## 3. Interfaces You May Call

From Agent A (will exist post-merge; reference the contract now):

```go
// package analyzer — types and function Agent A creates
type RegressionInput struct { ... }   // full definition in Interface Contracts section
type RegressionStatus struct { ... }  // full definition in Interface Contracts section
func ComputeRegressionStatus(input analyzer.RegressionInput) analyzer.RegressionStatus
```

From existing infrastructure (already implemented):

```go
// store package
func (db *DB) GetProjectBaseline(project string) (*ProjectBaseline, error)
func (db *DB) ListProjectBaselines() ([]ProjectBaseline, error)
store.Open(path string) (*DB, error)

// config package
func DBPath() string

// claude package
func ParseAllSessionMeta(claudeHome string) ([]SessionMeta, error)
func ParseAllFacets(claudeHome string) ([]SessionFacet, error)
func FindActiveSessionPath(claudeHome string) (string, error)
func ParseActiveSession(path string) (*SessionMeta, error)

// mcp server helpers (same package)
func (s *Server) loadTags() map[string]string
func (s *Server) loadCacheRatio() analyzer.CacheRatio
func resolveProjectName(sessionID, projectPath string, tags map[string]string) string
var analyzer.DefaultPricing map[string]analyzer.ModelPricing  // use ["sonnet"]
```

## 4. What to Implement

Read these files first to understand the full existing pattern:

- `internal/mcp/anomaly_tools.go` — closest existing tool (project resolution,
  DB usage, baseline loading, session filtering). Model your handler after this file.
- `internal/mcp/tools.go` — understand where to append `addRegressionTools(s)`.
- `internal/mcp/tools_test.go` — understand `newTestServer`, `callTool`, helper
  functions available in tests.

**`addRegressionTools` behavior:**

Register one tool:
- Name: `"get_regression_status"`
- Description: `"Regression status for a project: whether friction rate or cost-per-session has exceeded a threshold multiplier relative to the stored baseline. Returns current vs baseline metrics."`
- Input schema:
  ```json
  {
    "type": "object",
    "properties": {
      "project": {
        "type": "string",
        "description": "Project name (e.g. 'commitmux'). Omit to use the current session's project."
      },
      "threshold": {
        "type": "number",
        "description": "Regression multiplier over baseline (default 1.5). Must be > 1."
      }
    },
    "additionalProperties": false
  }
  ```
- Handler: `s.handleGetRegressionStatus`

**`handleGetRegressionStatus` behavior:**

1. Parse optional `project` string and `threshold` float64 from args.

2. **Project resolution** — identical pattern to `handleGetProjectAnomalies`:
   - If `project` arg is non-empty, use it.
   - Else: try active session → fall back to most-recently-closed session.
   - If no sessions exist at all, return an empty `RegressionStatus{HasBaseline: false, Message: "no sessions found"}`.

3. **Load sessions and filter** by project (same pattern as anomaly tool).

4. **Open DB** and call `db.GetProjectBaseline(project)`.
   - If error, return `nil, err`.
   - If baseline is nil (no stored baseline), call
     `analyzer.ComputeRegressionStatus` with `Baseline: nil` and return the result
     (which will have `HasBaseline: false`).

5. **Select recent sessions:** take the last `min(10, len(projectSessions))`
   sessions sorted by `StartTime` descending, then pass them to
   `ComputeRegressionStatus` (the function sorts internally if needed, but
   pass them in descending order so the caller convention is clear).
   Actually: pass all project sessions; document that Agent A's function
   receives all sessions in the `RecentSessions` field — callers should
   pass only recent sessions, so **cap at 10** before passing.

6. **Call** `analyzer.ComputeRegressionStatus(analyzer.RegressionInput{...})`
   with threshold from args (or 0 if not provided — Agent A defaults 0 → 1.5).

7. Return the `analyzer.RegressionStatus` directly (no wrapper struct needed).

8. **Append `addRegressionTools(s)`** in `internal/mcp/tools.go` after the
   existing `addAnomalyTools(s)` line (inside `addTools` function). This is an
   append-only change to one line — do not modify any other part of `tools.go`.

**Expected build state:** Your worktree will NOT have Agent A's
`internal/analyzer/regression.go`. Therefore `go build ./...` will fail with
"undefined: analyzer.ComputeRegressionStatus". This is expected. Note it in
your completion report as an out-of-scope build blocker. Run
`go test ./internal/mcp/...` with a build tag or skip — see constraint below.

## 5. Tests to Write

Write tests in `internal/mcp/regression_tools_test.go`.

**Important:** Because `analyzer.ComputeRegressionStatus` does not exist in your
worktree, tests that call the handler will fail at compile time. Handle this by
writing the tests and adding a build constraint to the test file:

```go
//go:build integration
```

This allows the orchestrator to run the full test suite post-merge when both
agents' code is present. Document this in your completion report.

Tests to write (behind the `//go:build integration` tag):

1. `TestGetRegressionStatus_EmptyDir` — no sessions returns HasBaseline=false, no error
2. `TestGetRegressionStatus_NoBaseline` — 5 sessions, no stored baseline → HasBaseline=false
3. `TestGetRegressionStatus_NoRegression` — 5 uniform sessions with stored baseline within threshold
4. `TestGetRegressionStatus_FrictionRegression` — stored baseline with low friction, recent sessions all have friction → FrictionRegressed=true
5. `TestGetRegressionStatus_ExplicitProject` — project name in args filters correctly
6. `TestGetRegressionStatus_CustomThreshold` — threshold=2.0 is not exceeded when threshold=1.5 would be

For tests 2–6, use `writeSessionMetaFull` (defined in `health_tools_test.go`),
`openTestDB` (defined in `transcript_tools_test.go`), and `store.UpsertProjectBaseline`
to pre-populate baselines. Follow the exact pattern from `anomaly_tools_test.go`.

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claire/worktrees/wave1-agent-b
go vet ./internal/mcp/...
go test -tags integration ./internal/mcp/... 2>&1 || true  # expected compile fail on analyzer symbols
```

Due to the build blocker, report verification as:
`FAIL (build blocked on analyzer.ComputeRegressionStatus — Agent A out-of-scope symbol)`

The orchestrator resolves this at post-merge. After merge, the full gate is:

```bash
go build ./...
go vet ./...
go test ./...
```

## 7. Constraints

- Do not modify any file not in your ownership list except the single-line
  append to `internal/mcp/tools.go`.
- Use `//go:build integration` on your test file to avoid compile failure in the
  isolated worktree. Remove this tag after the orchestrator confirms the merged
  build is clean (or the orchestrator removes it — note it as an out-of-scope dep).
- Match the error handling convention of `handleGetProjectAnomalies` exactly:
  non-fatal failures (facet parse errors, SAW ID build failures) are silently
  swallowed; fatal failures (session parse errors, DB open errors) are returned.
- The `threshold` parameter from args is passed through to `RegressionInput.Threshold`
  as-is. Agent A defaults it to 1.5 if <= 1.
- Do not add new dependencies to `go.mod`.

## 8. Report

**Before reporting:** Commit your changes to your worktree branch:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b
git add internal/mcp/regression_tools.go internal/mcp/regression_tools_test.go internal/mcp/tools.go
git commit -m "wave1-agent-b: add get_regression_status MCP tool"
```

Append your completion report to the IMPL doc at
`/Users/dayna.blackwell/code/claudewatch/docs/IMPL-regression-detection.md`
under `### Agent B - Completion Report`. Do not edit any earlier section.

```yaml
### Agent B - Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-b
commit: {sha}
files_changed:
  - internal/mcp/tools.go
files_created:
  - internal/mcp/regression_tools.go
  - internal/mcp/regression_tools_test.go
interface_deviations:
  - "[] or exact description"
out_of_scope_deps:
  - "file: internal/analyzer/regression.go, change: must exist with ComputeRegressionStatus, reason: build dependency"
  - "file: internal/mcp/regression_tools_test.go, change: remove //go:build integration tag after merge, reason: workaround for missing analyzer symbol"
tests_added:
  - TestGetRegressionStatus_EmptyDir
  - TestGetRegressionStatus_NoBaseline
  - TestGetRegressionStatus_NoRegression
  - TestGetRegressionStatus_FrictionRegression
  - TestGetRegressionStatus_ExplicitProject
  - TestGetRegressionStatus_CustomThreshold
verification: FAIL (build blocked on analyzer.ComputeRegressionStatus — Agent A out-of-scope symbol)
```

---

### Wave Execution Loop

After Wave 1 completes:

1. Read completion reports from Agent A and Agent B sections appended to this doc.
2. Verify both agents report `commit:` with a SHA (not "uncommitted").
3. Merge both worktree branches:
   ```bash
   cd /Users/dayna.blackwell/code/claudewatch
   git merge wave1-agent-a --no-ff -m "merge wave1-agent-a: ComputeRegressionStatus"
   git merge wave1-agent-b --no-ff -m "merge wave1-agent-b: get_regression_status MCP tool"
   ```
4. Remove the `//go:build integration` tag from `internal/mcp/regression_tools_test.go`
   if Agent B added it (edit the file to remove line 1 `//go:build integration` and
   the blank line following it).
5. Run:
   ```bash
   go build ./...
   go vet ./...
   go test ./...
   ```
6. Fix any issues (most likely: none, or minor import issues from the build tag removal).
7. Add the `checkRegressionStatus` doctor check to `internal/app/doctor.go`:
   - After the `checkAnomalyBaselines` call block (around line 92), add:
     ```go
     // 10. Regression detection — warn if any project's friction or cost has regressed.
     if db != nil {
         checks = append(checks, checkRegressionStatus(db, sessions, cfg))
     }
     ```
   - Implement `checkRegressionStatus` as a new function at the bottom of `doctor.go`:
     ```go
     func checkRegressionStatus(db *store.DB, sessions []claude.SessionMeta, cfg *config.Config) doctorCheck {
         baselines, err := db.ListProjectBaselines()
         if err != nil {
             return doctorCheck{
                 Name:    "Regression detection",
                 Passed:  false,
                 Message: fmt.Sprintf("could not list baselines: %v", err),
             }
         }
         if len(baselines) == 0 {
             return doctorCheck{
                 Name:    "Regression detection",
                 Passed:  true,
                 Message: "no baselines stored — nothing to compare",
             }
         }

         // Group sessions by project name.
         byProject := make(map[string][]claude.SessionMeta)
         for _, s := range sessions {
             proj := filepath.Base(s.ProjectPath)
             if proj == "" || proj == "." {
                 continue
             }
             byProject[proj] = append(byProject[proj], s)
         }

         pricing := analyzer.DefaultPricing["sonnet"]
         // CacheRatio: use NoCacheRatio for doctor simplicity (no stats-cache dependency).
         ratio := analyzer.NoCacheRatio()

         var regressed []string
         for _, b := range baselines {
             projectSessions := byProject[b.Project]
             // Cap at 10 most recent.
             if len(projectSessions) > 10 {
                 projectSessions = projectSessions[len(projectSessions)-10:]
             }
             status := analyzer.ComputeRegressionStatus(analyzer.RegressionInput{
                 Project:        b.Project,
                 Baseline:       &b,
                 RecentSessions: projectSessions,
                 Pricing:        pricing,
                 CacheRatio:     ratio,
                 Threshold:      0, // default 1.5
             })
             if status.Regressed {
                 regressed = append(regressed, fmt.Sprintf("%s (%s)", b.Project, status.Message))
             }
         }

         if len(regressed) == 0 {
             return doctorCheck{
                 Name:    "Regression detection",
                 Passed:  true,
                 Message: fmt.Sprintf("%d project(s) within baseline thresholds", len(baselines)),
             }
         }
         return doctorCheck{
             Name:    "Regression detection",
             Passed:  false,
             Message: fmt.Sprintf("%d project(s) regressed: %s", len(regressed), strings.Join(regressed, "; ")),
         }
     }
     ```
   - Note: `cfg` parameter is included for future use (configurable threshold); for now
     the function ignores it. If linting complains about the unused parameter, replace
     `cfg *config.Config` with `_ *config.Config` or remove the parameter and update
     the call site accordingly.
   - The `runDoctor` function needs access to `cfg` at the point where `db` is opened;
     it already loads `cfg` at the top of `runDoctor`. Pass `cfg` (or remove the param
     and hardcode threshold=0) depending on what compiles cleanly.
8. Run verification again:
   ```bash
   go build ./...
   go vet ./...
   go test ./...
   ```
9. Tick status checkboxes below.
10. Commit the merged result:
    ```bash
    git commit -m "feat: automated regression detection — doctor check and MCP tool"
    ```

---

### Status

- [x] Wave 1 Agent A — `internal/analyzer/regression.go` + `regression_test.go` (ComputeRegressionStatus)
- [x] Wave 1 Agent B — `internal/mcp/regression_tools.go` + `regression_tools_test.go` + tools.go registration (get_regression_status MCP tool)
- [x] Post-merge: remove `//go:build integration` tag from Agent B's test file
- [x] Post-merge: add `checkRegressionStatus` doctor check to `internal/app/doctor.go`
- [x] Post-merge: `go build ./... && go vet ./... && go test ./...` passes
- [ ] Post-merge: commit

---

### Agent B - Completion Report
status: complete
worktree: .claude/worktrees/wave1-agent-b
commit: dc334d4
files_changed:
  - internal/mcp/tools.go
files_created:
  - internal/mcp/regression_tools.go
  - internal/mcp/regression_tools_test.go
interface_deviations:
  - []
out_of_scope_deps:
  - "file: internal/analyzer/regression.go, change: must exist with ComputeRegressionStatus, RegressionInput, and RegressionStatus, reason: build dependency"
  - "file: internal/mcp/regression_tools_test.go, change: remove //go:build integration tag after merge, reason: workaround for missing analyzer symbol"
tests_added:
  - TestGetRegressionStatus_EmptyDir
  - TestGetRegressionStatus_NoBaseline
  - TestGetRegressionStatus_NoRegression
  - TestGetRegressionStatus_FrictionRegression
  - TestGetRegressionStatus_ExplicitProject
  - TestGetRegressionStatus_CustomThreshold
verification: FAIL (build blocked on analyzer.ComputeRegressionStatus — Agent A out-of-scope symbol)
notes:
  - Isolation verified before any file modifications
  - Handler follows handleGetProjectAnomalies exactly for project resolution, DB usage, session filtering
  - Returns analyzer.RegressionStatus directly (no wrapper struct)
  - threshold=0 is passed through as-is to RegressionInput; Agent A defaults 0 to 1.5
  - go vet confirms only undefined symbols are analyzer.RegressionStatus, analyzer.RegressionInput, analyzer.ComputeRegressionStatus

### Agent A - Completion Report
status: complete
worktree: .claude/worktrees/wave1-agent-a
commit: f0f2a72
files_changed: []
files_created:
  - internal/analyzer/regression.go
  - internal/analyzer/regression_test.go
interface_deviations:
  - "The task spec listed sessionFriction and buildFacetIndex signatures with pointer map values (map[string]*claude.SessionFacet). The actual implementations in compare.go use value maps (map[string]claude.SessionFacet). Implementation matches the actual code, not the spec."
out_of_scope_deps:
  - []
tests_added:
  - TestComputeRegressionStatus_NoBaseline
  - TestComputeRegressionStatus_InsufficientData
  - TestComputeRegressionStatus_NoRegression
  - TestComputeRegressionStatus_FrictionRegression
  - TestComputeRegressionStatus_CostRegression
  - TestComputeRegressionStatus_BothRegressed
  - TestComputeRegressionStatus_ThresholdDefault
  - TestComputeRegressionStatus_ZeroBaselineSkipped
  - TestComputeRegressionStatus_MessageContent
  - TestComputeRegressionStatus_ExactlyAtThreshold
  - TestComputeRegressionStatus_FacetsUsed (bonus: verifies facets override ToolErrors)
verification: PASS (go build ./... && go vet ./... && go test ./internal/analyzer/... - all tests pass)
notes:
  - Isolation verified before any file modifications
  - sessionFriction and buildFacetIndex are in compare.go (not anomaly.go as spec suggested); both are package-private and accessible within package analyzer
  - ComputeRegressionStatus is a pure function: no I/O, no DB access, no global state
  - frictionRate is computed as fraction of sessions with any friction (0.0–1.0), not raw count
  - Threshold default applies when Threshold <= 1 (spec says <= 1, not just == 0)
  - Zero baseline guard skips comparison entirely to avoid spurious regressions
  - Both-regressed message omits threshold per spec; single-regression messages include threshold
