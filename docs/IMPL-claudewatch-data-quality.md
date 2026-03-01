# IMPL: claudewatch Data Quality Fixes

## Suitability Assessment

**Verdict: SUITABLE**

Three independent bug fixes with fully disjoint file ownership. All root causes are confirmed in source:
(1) `handleGetCostSummary` reads indexed sessions only — the $212 live session is invisible;
(2) `handleGetProjectHealth`'s no-arg default picks the most-recently-closed session by StartTime sort, not the active session;
(3) `handleGetProjectComparison` has no session-count filter, so a single high-volume low-commit project (rezmakr, 30 sessions, ZeroCommitRate: 1.0) dominates aggregate health signals.

All three fixes touch separate files with no cross-agent interface contracts beyond pre-existing `FindActiveSessionPath`/`ParseActiveSession` in `internal/claude/active.go`.

```
Estimated times:
- Scout phase: ~10 min
- Agent execution: ~10 min (3 agents × ~10 min, fully parallel → ~10 min total)
- Merge & verification: ~5 min
Total SAW time: ~25 min

Sequential baseline: ~30 min (3 × ~10 min sequential)
Time savings: ~5 min (marginal speed gain)

Recommendation: Proceed. Coordination value (interface spec, audit trail) justifies SAW
beyond the marginal speed gain. Each agent owns 2 files with non-trivial logic and tests.
```

---

## Known Issues

None identified.

---

## Dependency Graph

All three agents are leaves — no inter-agent dependencies.

```
internal/claude/active.go  (pre-existing, no agent modifies this)
        |
   +----|----+
   |    |    |
[A] [B] [C]  (Wave 1, parallel)
```

Post-merge orchestrator change: `internal/mcp/tools.go` — update `get_project_comparison`
InputSchema from `noArgsSchema` to include optional `min_sessions` int (1-line change).
This file is **orchestrator-owned** and excluded from all agent ownership.

---

## Interface Contracts

All agents call these pre-existing functions — no agent implements or modifies them:

```go
// internal/claude/active.go
func FindActiveSessionPath(claudeHome string) (string, error)
// Returns the path to the active (open) JSONL session file, or "" if none active.
// Errors are non-fatal — callers must fall back to closed-session logic.

func ParseActiveSession(path string) (*claude.SessionMeta, error)
// Reads a partial JSONL from an active session file (line-atomic truncation at last \n).
// Returns SessionMeta with InputTokens, OutputTokens, ProjectPath, SessionID, StartTime.
// Errors are non-fatal — callers must fall back to closed-session logic.
```

No cross-agent contracts (all 3 agents are independent of each other).

---

## File Ownership

| File | Agent | Wave | Depends On |
|------|-------|------|------------|
| `internal/mcp/cost_tools.go` | A | 1 | `internal/claude/active.go` (pre-existing) |
| `internal/mcp/cost_tools_test.go` | A | 1 | — |
| `internal/mcp/health_tools.go` | B | 1 | `internal/claude/active.go` (pre-existing) |
| `internal/mcp/health_tools_test.go` | B | 1 | — |
| `internal/mcp/project_tools.go` | C | 1 | — |
| `internal/mcp/project_tools_test.go` | C | 1 | — |
| `internal/mcp/tools.go` | Orchestrator | post-merge | Agent C completes |

---

## Wave Structure

```
Wave 1: [A] [B] [C]   ← 3 parallel agents, all independent
              |
    (all 3 complete + post-merge verification)
              |
    Orchestrator patches tools.go InputSchema for get_project_comparison
```

---

## Agent Prompts

---

### Wave 1 Agent A: include live session in get_cost_summary

You are Wave 1 Agent A. Add live (in-progress) session cost to `handleGetCostSummary` so the current session's $212+ spend is included in today/week/all-time totals and by-project aggregates.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-A 2>/dev/null || true
```

**Step 2: Verify isolation (strict fail-fast after self-correction attempt)**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-A"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave1-agent-A"

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

echo "✓ Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

**If verification fails:** Write error to completion report and exit immediately (do NOT modify files).

## 1. File Ownership

You own these files. Do not touch any other files.
- `internal/mcp/cost_tools.go` — modify
- `internal/mcp/cost_tools_test.go` — modify

## 2. Interfaces You Must Implement

No new exported functions. You modify `handleGetCostSummary` in-place.

## 3. Interfaces You May Call

```go
// internal/claude/active.go (pre-existing, do not modify)
func FindActiveSessionPath(claudeHome string) (string, error)
func ParseActiveSession(path string) (*claude.SessionMeta, error)

// internal/analyzer/cost.go (pre-existing)
func EstimateSessionCost(session claude.SessionMeta, pricing Pricing, ratio CacheRatio) float64
var DefaultPricing map[string]Pricing  // use DefaultPricing["sonnet"]

// internal/claude (pre-existing)
func ParseTimestamp(s string) time.Time
```

## 4. What to Implement

In `handleGetCostSummary` in `cost_tools.go`:

After loading indexed sessions via `claude.ParseAllSessionMeta(s.claudeHome)`, also attempt to load the live session:

1. Call `claude.FindActiveSessionPath(s.claudeHome)`. If it returns an error or empty string, skip (non-fatal).
2. Call `claude.ParseActiveSession(activePath)`. If it returns an error or nil, skip (non-fatal).
3. **Deduplication:** Build a set of SessionIDs from the indexed sessions. If the live session's SessionID is already in the set, skip it (prevents double-counting if the session was indexed before closing).
4. If the live session is new, add its cost to TodayUSD, WeekUSD, and AllTimeUSD using the same time-bucket logic as the indexed loop:
   - Use `claude.ParseTimestamp(meta.StartTime)` to get the start time.
   - TodayUSD: if UTC date == today string.
   - WeekUSD: if ISO year+week matches now.
   - AllTimeUSD: always.
5. Add to the ByProject accumulator using `filepath.Base(meta.ProjectPath)` as the project name.

Read `internal/claude/active.go` to understand FindActiveSessionPath and ParseActiveSession before implementing.

Read `internal/claude/active_test.go` to understand the JSONL fixture format for your tests.

**Test fixture pattern:** `FindActiveSessionPath` uses lsof first, then falls back to mtime (files modified within 5 minutes). In tests, write the JSONL to `{claudeHome}/projects/{hash}/sessions/{sessionID}.jsonl` with fresh mtime. It will be picked up by the mtime fallback. The JSONL must contain at least one summary line with `"type":"summary"` and `"costUSD"` (or token fields) — check `active_test.go` for the exact format ParseActiveSession expects.

## 5. Tests to Write

1. `TestGetCostSummary_LiveSessionIncluded` — write a fake active JSONL (fresh mtime, NOT in indexed sessions), verify TodayUSD/AllTimeUSD includes the live session's cost on top of zero indexed sessions.
2. `TestGetCostSummary_LiveSessionNoDuplicate` — same session appears in both indexed session_meta AND as the active JSONL; verify cost is counted only once (not doubled).
3. `TestGetCostSummary_LiveSessionByProject` — live session for "liveproject", no indexed sessions; verify ByProject contains "liveproject" with correct cost.

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/mcp -run TestGetCostSummary -v
```

All must pass before reporting completion.

## 7. Constraints

- FindActiveSessionPath and ParseActiveSession errors are **non-fatal** — must never propagate; fall through to indexed-only path.
- No changes to exported types (`CostSummaryResult`, `ProjectSpend`).
- Use `filepath.Base(meta.ProjectPath)` consistent with existing code.
- Do not modify `tools.go` or any file outside your ownership list.

## 8. Report

Before reporting, commit your changes:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-A
git add internal/mcp/cost_tools.go internal/mcp/cost_tools_test.go
git commit -m "wave1-agent-A: include live session in get_cost_summary"
```

Append your completion report to this IMPL doc under `### Agent A — Completion Report`.

```yaml
### Agent A — Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-A
commit: {sha}
files_changed:
  - internal/mcp/cost_tools.go
files_created: []
interface_deviations: []
out_of_scope_deps: []
tests_added:
  - TestGetCostSummary_LiveSessionIncluded
  - TestGetCostSummary_LiveSessionNoDuplicate
  - TestGetCostSummary_LiveSessionByProject
verification: PASS | FAIL
```

---

### Wave 1 Agent B: fix get_project_health active-session default

You are Wave 1 Agent B. Fix `handleGetProjectHealth`'s no-arg default to use the active session's project instead of the most-recently-closed session's project.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-B 2>/dev/null || true
```

**Step 2: Verify isolation**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-B"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave1-agent-B"

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

echo "✓ Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

**If verification fails:** Write error to completion report and exit immediately.

## 1. File Ownership

You own these files. Do not touch any other files.
- `internal/mcp/health_tools.go` — modify
- `internal/mcp/health_tools_test.go` — modify

## 2. Interfaces You Must Implement

No new exported functions. You modify `handleGetProjectHealth` in-place.

## 3. Interfaces You May Call

```go
// internal/claude/active.go (pre-existing, do not modify)
func FindActiveSessionPath(claudeHome string) (string, error)
func ParseActiveSession(path string) (*claude.SessionMeta, error)
```

## 4. What to Implement

In `handleGetProjectHealth` in `health_tools.go`, the current no-arg default is:

```go
// Default: most recent session's project.
sorted := make([]claude.SessionMeta, len(sessions))
copy(sorted, sessions)
sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartTime > sorted[j].StartTime })
project = resolveProjectName(sorted[0].SessionID, sorted[0].ProjectPath, tags)
```

Replace this block with:

1. First attempt to use the active session:
   - Call `claude.FindActiveSessionPath(s.claudeHome)`
   - If no error and path is non-empty, call `claude.ParseActiveSession(activePath)`
   - If no error and meta is non-nil and `meta.ProjectPath != ""`: set `project = filepath.Base(meta.ProjectPath)` and proceed.
2. If the active session is not available (any error or empty path/project): fall back to the existing sort-by-StartTime logic (unchanged).

Priority order:
1. Explicit `project` arg (existing behavior — keep as-is)
2. Active session (new)
3. Most-recent indexed session (existing fallback — keep as-is)

Read `internal/claude/active.go` to understand FindActiveSessionPath and ParseActiveSession before implementing.

Read `internal/claude/active_test.go` to understand the JSONL fixture format for tests.

**Test fixture pattern:** Write the JSONL to `{claudeHome}/projects/{hash}/sessions/{sessionID}.jsonl` with fresh mtime. See `active_test.go` for exact format. The mtime fallback picks up files modified within 5 minutes.

Also write indexed session_meta files for the "old" project via `writeSessionMeta`/`writeSessionMetaFull` helpers visible in the existing test file.

## 5. Tests to Write

1. `TestGetProjectHealth_DefaultsToActiveSession` — write active JSONL for "activeproject" + indexed sessions for "oldproject"; call with no args; verify result.Project == "activeproject".
2. `TestGetProjectHealth_FallsBackToRecentWhenNoActive` — no active JSONL (or stale mtime); write indexed sessions for "oldproject"; call with no args; verify falls back to "oldproject" (existing behavior).
3. `TestGetProjectHealth_ExplicitProjectOverridesActive` — active JSONL for "activeproject"; call with explicit arg `"project": "oldproject"`; verify result.Project == "oldproject".

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/mcp -run TestGetProjectHealth -v
```

All must pass before reporting completion.

## 7. Constraints

- Active session detection is **non-fatal** — both FindActiveSessionPath and ParseActiveSession errors must never propagate; fall through to existing logic.
- Explicit `project` arg always takes priority (no behavior change there).
- Do not modify `tools.go` or any file outside your ownership list.

## 8. Report

Before reporting, commit your changes:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-B
git add internal/mcp/health_tools.go internal/mcp/health_tools_test.go
git commit -m "wave1-agent-B: fix get_project_health active-session default"
```

Append your completion report to this IMPL doc under `### Agent B — Completion Report`.

```yaml
### Agent B — Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-B
commit: {sha}
files_changed:
  - internal/mcp/health_tools.go
files_created: []
interface_deviations: []
out_of_scope_deps: []
tests_added:
  - TestGetProjectHealth_DefaultsToActiveSession
  - TestGetProjectHealth_FallsBackToRecentWhenNoActive
  - TestGetProjectHealth_ExplicitProjectOverridesActive
verification: PASS | FAIL
```

---

### Wave 1 Agent C: add min_sessions filter to get_project_comparison

You are Wave 1 Agent C. Add an optional `min_sessions` parameter to `handleGetProjectComparison` so low-confidence projects (e.g., rezmakr with 30 sessions and ZeroCommitRate: 1.0) can be filtered out, preventing them from dominating aggregate health signals.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-C 2>/dev/null || true
```

**Step 2: Verify isolation**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-C"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave1-agent-C"

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

echo "✓ Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

**If verification fails:** Write error to completion report and exit immediately.

## 1. File Ownership

You own these files. Do not touch any other files.
- `internal/mcp/project_tools.go` — modify
- `internal/mcp/project_tools_test.go` — modify

**Note:** The InputSchema registration for `get_project_comparison` is in `tools.go`, which is **orchestrator-owned**. You do NOT modify `tools.go`. Your handler must correctly parse `min_sessions` from args — the schema update happens post-merge.

## 2. Interfaces You Must Implement

No new exported functions. You modify `handleGetProjectComparison` in-place.

## 3. Interfaces You May Call

No new cross-agent calls. All dependencies are pre-existing and unchanged.

## 4. What to Implement

In `handleGetProjectComparison` in `project_tools.go`:

**Step 1:** At the top of the function, parse the optional `min_sessions` arg:

```go
var params struct {
    MinSessions *int `json:"min_sessions"`
}
if len(args) > 0 && string(args) != "null" {
    _ = json.Unmarshal(args, &params)
}
minSessions := 0
if params.MinSessions != nil && *params.MinSessions > 0 {
    minSessions = *params.MinSessions
}
```

**Step 2:** After computing all `summaries` (the existing logic is unchanged), apply the filter before sorting:

```go
if minSessions > 0 {
    filtered := summaries[:0]
    for _, s := range summaries {
        if s.SessionCount >= minSessions {
            filtered = append(filtered, s)
        }
    }
    summaries = filtered
}
```

**Step 3:** The existing sort and return logic is unchanged.

Also update `addProjectComparisonTool` in `project_tools_test.go` (used only in tests) to register with the new schema so tests can pass `min_sessions`. The production schema in `tools.go` is updated post-merge by the orchestrator.

## 5. Tests to Write

1. `TestGetProjectComparison_MinSessionsFilter` — 3 projects with 1, 3, 5 sessions; call with `min_sessions: 3`; verify only 2 projects returned (those with ≥3 sessions).
2. `TestGetProjectComparison_MinSessionsZeroNoFilter` — 3 projects; call with `min_sessions: 0`; verify all 3 projects returned.
3. `TestGetProjectComparison_MinSessionsDefaultNoFilter` — 3 projects; call with `{}`; verify all 3 projects returned (no filter when arg absent).

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/mcp -run TestGetProjectComparison -v
```

All must pass before reporting completion.

## 7. Constraints

- `min_sessions` default is 0 (no filter). Never filter when min_sessions ≤ 0.
- Invalid or missing args are silently ignored (default 0).
- Do not modify `tools.go` or any file outside your ownership list.
- The `addProjectComparisonTool` helper in `project_tools_test.go` may be updated to register with a local schema that includes `min_sessions` — this is test-only and does not affect production registration.

## 8. Report

Before reporting, commit your changes:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-C
git add internal/mcp/project_tools.go internal/mcp/project_tools_test.go
git commit -m "wave1-agent-C: add min_sessions filter to get_project_comparison"
```

Append your completion report to this IMPL doc under `### Agent C — Completion Report`.

```yaml
### Agent C — Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-C
commit: {sha}
files_changed:
  - internal/mcp/project_tools.go
  - internal/mcp/project_tools_test.go
files_created: []
interface_deviations: []
out_of_scope_deps:
  - "file: internal/mcp/tools.go, change: update get_project_comparison InputSchema to include min_sessions, reason: orchestrator-owned registration"
tests_added:
  - TestGetProjectComparison_MinSessionsFilter
  - TestGetProjectComparison_MinSessionsZeroNoFilter
  - TestGetProjectComparison_MinSessionsDefaultNoFilter
verification: PASS | FAIL
```

---

## Wave Execution Loop

After Wave 1 completes:
1. Read each agent's completion report (`### Agent {A/B/C} — Completion Report`).
2. Merge all 3 worktrees into main:
   ```bash
   cd /Users/dayna.blackwell/code/claudewatch
   git merge wave1-agent-A wave1-agent-B wave1-agent-C
   ```
3. Apply lint auto-fix (CI runs golangci-lint --fix):
   ```bash
   golangci-lint run --fix ./...
   ```
4. Apply orchestrator-owned change to `tools.go`: update `get_project_comparison` InputSchema from `noArgsSchema` to include optional `min_sessions`:
   ```go
   InputSchema: json.RawMessage(`{"type":"object","properties":{"min_sessions":{"type":"integer","description":"Minimum session count to include a project (default 0 = no filter)"}},"additionalProperties":false}`),
   ```
5. Run full verification:
   ```bash
   go build ./...
   go vet ./...
   go test ./... -race
   ```
6. Fix any integration issues. Pay attention to: any test helpers that collide across the 3 agent files (same function names in the same `package mcp` test package).
7. Commit the wave result.

---

## Status

- [ ] Wave 1 Agent A — include live session in get_cost_summary
- [ ] Wave 1 Agent B — fix get_project_health active-session default
- [ ] Wave 1 Agent C — add min_sessions filter to get_project_comparison
