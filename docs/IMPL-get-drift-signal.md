# IMPL: get_drift_signal standalone MCP tool

**Feature:** Add `get_drift_signal` as a standalone MCP tool mirroring the
`drift_signal` field already present in `get_session_dashboard`.

**Date:** 2026-03-02
**Status:** READY

---

## Suitability Assessment

| Question | Answer |
|---|---|
| 1. Can work be assigned to ≥2 agents with disjoint file ownership? | YES — one agent owns the new tool file + registration; one agent owns the test file. Files do not overlap. |
| 2. Does any part require investigation-first analysis? | NO — `ParseLiveDriftSignal` already exists and is fully implemented in `internal/claude/active_live.go`. The handler pattern is 1:1 with existing tools. |
| 3. Can cross-agent interfaces be defined before implementation? | YES — `DriftSignalResult` struct and `handleGetDriftSignal` signature are fully specifiable now. |
| 4. Pre-implementation status | `ParseLiveDriftSignal` and `LiveDriftStats` exist in `internal/claude/active_live.go`. `dashboard_tools.go` already calls it (window=20). No standalone handler, no standalone test file, no registration in `addTools`. |
| 5. Does parallelization gain exceed overhead? | YES — tool file and test file are fully independent; both agents can work simultaneously with zero merge risk. |

**Verdict: SUITABLE for 2-agent parallel wave.**

**Test command:** `go test ./internal/mcp/... -run TestGetDriftSignal`
**Full suite:** `go test ./...`
**Build check:** `go build ./...`
**Vet:** `go vet ./...`

---

## Known Issues

None. All dependencies pre-exist. This is purely additive.

---

## Dependency Graph

```
internal/claude/active_live.go          (EXISTS — read-only)
  └── ParseLiveDriftSignal(path, windowN) (*LiveDriftStats, error)
  └── LiveDriftStats struct

internal/claude/active.go               (EXISTS — read-only)
  └── FindActiveSessionPath(claudeHome) (string, error)
  └── ParseActiveSession(path) (*SessionMeta, error)

internal/mcp/drift_tools.go             [NEW — Agent A owns]
  └── DriftSignalResult struct
  └── addDriftTools(s *Server)
  └── (s *Server) handleGetDriftSignal(args json.RawMessage) (any, error)

internal/mcp/tools.go                   [MODIFY — Agent A owns]
  └── addTools() — add addDriftTools(s) call

internal/mcp/drift_tools_test.go        [NEW — Agent B owns]
  └── TestGetDriftSignal_NoActiveSession
  └── TestGetDriftSignal_Exploring
  └── TestGetDriftSignal_Implementing
  └── TestGetDriftSignal_Drifting
  └── TestGetDriftSignal_MCP
```

**Dependency between agents:** Agent B depends on Agent A's `DriftSignalResult`
type and `addDriftTools` function being defined. The interface contract below is
the binding agreement — Agent B writes tests against the contract; Agent A
implements it to match.

---

## Interface Contracts

### `DriftSignalResult` (defined in `internal/mcp/drift_tools.go`)

```go
// DriftSignalResult holds drift signal data for the current live session.
type DriftSignalResult struct {
    SessionID  string `json:"session_id"`
    Live       bool   `json:"live"`
    WindowN    int    `json:"window_n"`
    ReadCalls  int    `json:"read_calls"`
    WriteCalls int    `json:"write_calls"`
    HasAnyEdit bool   `json:"has_any_edit"`
    Status     string `json:"status"` // "exploring", "implementing", "drifting"
}
```

### `addDriftTools(s *Server)` signature

Registers exactly one tool: `"get_drift_signal"` with `noArgsSchema` and
handler `s.handleGetDriftSignal`.

### `handleGetDriftSignal(args json.RawMessage) (any, error)` contract

- Calls `claude.FindActiveSessionPath(s.claudeHome)`.
- If path is empty or err non-nil: returns `nil, errors.New("no active session found")`.
- Calls `claude.ParseActiveSession(activePath)`.
- If meta is nil or err non-nil: returns `nil, errors.New("no active session found")`.
- Calls `claude.ParseLiveDriftSignal(activePath, 20)` (windowN=20, matching dashboard).
- If err non-nil: returns `nil, err`.
- Returns `DriftSignalResult` with all fields populated from `meta` and `drift`.

### Tool registration in `addTools()` (`internal/mcp/tools.go`)

The call `addDriftTools(s)` is added after `addDashboardTools(s)` and before
the inline `s.registerTool` calls for `get_project_comparison` and
`get_stale_patterns`. Exact insertion point: after line containing
`addDashboardTools(s)`.

---

## File Ownership

| File | Agent | Action |
|---|---|---|
| `internal/mcp/drift_tools.go` | Agent A | CREATE |
| `internal/mcp/tools.go` | Agent A | MODIFY — add `addDriftTools(s)` call |
| `internal/mcp/drift_tools_test.go` | Agent B | CREATE |
| `internal/claude/active_live.go` | — | READ-ONLY (no changes) |
| `internal/claude/active.go` | — | READ-ONLY (no changes) |

---

## Wave Structure

**Single wave (Wave 1) — 2 parallel agents:**

| Agent | Files Owned | Depends On |
|---|---|---|
| Agent A: Tool Implementation | `drift_tools.go`, `tools.go` | Interface contracts above |
| Agent B: Tests | `drift_tools_test.go` | Interface contracts above |

No sequential waves required. Both agents can start simultaneously. Agent B
writes tests against the contract; they will not compile until Agent A's file
exists, but compilation is checked at merge time, not during individual agent
work.

---

## Agent Prompts

### Agent A: Tool Implementation

**agent:** A
**wave:** 1
**title:** Implement `get_drift_signal` MCP tool handler and registration
**owns:** `internal/mcp/drift_tools.go` (CREATE), `internal/mcp/tools.go` (MODIFY)
**reads:** `internal/claude/active_live.go`, `internal/mcp/context_tools.go`, `internal/mcp/cost_velocity_tools.go`, `internal/mcp/tools.go`
**interface_contract:** See Interface Contracts section above. `DriftSignalResult` struct must match exactly.
**success_criteria:** `go build ./...` passes. `go vet ./...` passes. The tool `get_drift_signal` is registered and callable via the MCP server. `DriftSignalResult` fields match the contract exactly.
**test_command:** `go build ./... && go vet ./...`

**Prompt:**

You are implementing a new standalone MCP tool `get_drift_signal` for the
claudewatch project at `/Users/dayna.blackwell/code/claudewatch`.

**Step 1: Read these files before writing anything.**
- `/Users/dayna.blackwell/code/claudewatch/internal/claude/active_live.go` — find `ParseLiveDriftSignal` and `LiveDriftStats`
- `/Users/dayna.blackwell/code/claudewatch/internal/mcp/context_tools.go` — use as exact structural template
- `/Users/dayna.blackwell/code/claudewatch/internal/mcp/cost_velocity_tools.go` — secondary template reference
- `/Users/dayna.blackwell/code/claudewatch/internal/mcp/tools.go` — find where to add the registration call

**Step 2: Create `/Users/dayna.blackwell/code/claudewatch/internal/mcp/drift_tools.go`**

The file must be `package mcp` and contain exactly:

```go
package mcp

import (
    "encoding/json"
    "errors"

    "github.com/blackwell-systems/claudewatch/internal/claude"
)

// DriftSignalResult holds drift signal data for the current live session.
type DriftSignalResult struct {
    SessionID  string `json:"session_id"`
    Live       bool   `json:"live"`
    WindowN    int    `json:"window_n"`
    ReadCalls  int    `json:"read_calls"`
    WriteCalls int    `json:"write_calls"`
    HasAnyEdit bool   `json:"has_any_edit"`
    Status     string `json:"status"` // "exploring", "implementing", "drifting"
}

// addDriftTools registers the get_drift_signal MCP tool on s.
func addDriftTools(s *Server) {
    s.registerTool(toolDef{
        Name:        "get_drift_signal",
        Description: "Detect exploration drift in the current live session: whether the agent is exploring (read-heavy), implementing (write-active), or drifting (edits exist session-wide but recent window is read-heavy with zero writes).",
        InputSchema: noArgsSchema,
        Handler:     s.handleGetDriftSignal,
    })
}

// handleGetDriftSignal returns the drift signal for the active session.
func (s *Server) handleGetDriftSignal(args json.RawMessage) (any, error) {
    activePath, err := claude.FindActiveSessionPath(s.claudeHome)
    if err != nil || activePath == "" {
        return nil, errors.New("no active session found")
    }

    meta, err := claude.ParseActiveSession(activePath)
    if err != nil || meta == nil {
        return nil, errors.New("no active session found")
    }

    drift, err := claude.ParseLiveDriftSignal(activePath, 20)
    if err != nil {
        return nil, err
    }

    return DriftSignalResult{
        SessionID:  meta.SessionID,
        Live:       true,
        WindowN:    drift.WindowN,
        ReadCalls:  drift.ReadCalls,
        WriteCalls: drift.WriteCalls,
        HasAnyEdit: drift.HasAnyEdit,
        Status:     drift.Status,
    }, nil
}
```

**Step 3: Modify `/Users/dayna.blackwell/code/claudewatch/internal/mcp/tools.go`**

In the `addTools` function, add the call `addDriftTools(s)` on a new line
immediately after the existing line `addDashboardTools(s)`. Do not move or
reorder any other lines.

**Step 4: Verify.**

Run:
```
cd /Users/dayna.blackwell/code/claudewatch && go build ./... && go vet ./...
```

Both must pass with no output. If they fail, fix any issues before reporting done.

---

### Agent B: Tests

**agent:** B
**wave:** 1
**title:** Write tests for `get_drift_signal` MCP tool
**owns:** `internal/mcp/drift_tools_test.go` (CREATE)
**reads:** `internal/mcp/live_tools_test.go`, `internal/mcp/context_tools_test.go`, `internal/mcp/velocity_tools_test.go`, `internal/mcp/tools_test.go`
**interface_contract:** `DriftSignalResult` struct from Interface Contracts section. Handler returns error `"no active session found"` when no active session exists.
**success_criteria:** All tests in `drift_tools_test.go` pass: `go test ./internal/mcp/... -run TestGetDriftSignal`. File compiles. Tests cover: no-active-session error, exploring status, implementing status, drifting status, tool registration check.
**test_command:** `go test ./internal/mcp/... -run TestGetDriftSignal -v`

**Prompt:**

You are writing tests for the new `get_drift_signal` MCP tool in claudewatch
at `/Users/dayna.blackwell/code/claudewatch`.

**Step 1: Read these files before writing anything.**
- `/Users/dayna.blackwell/code/claudewatch/internal/mcp/live_tools_test.go` — study `writeActiveToolJSONL` helper and test structure
- `/Users/dayna.blackwell/code/claudewatch/internal/mcp/context_tools_test.go` — study the no-active-session and status-check patterns
- `/Users/dayna.blackwell/code/claudewatch/internal/mcp/tools_test.go` — study `newTestServer`, `callTool`, `writeActiveJSONL`
- `/Users/dayna.blackwell/code/claudewatch/internal/claude/active_live.go` — understand `ParseLiveDriftSignal` logic (lines 554–664): read vs write tool name sets, window=20, status classification

**Step 2: Understand the drift classification rules.**

From `ParseLiveDriftSignal` (windowN=20):
- `readToolNames`: `Read`, `Grep`, `Glob`, `WebFetch`, `WebSearch`
- `writeToolNames`: `Edit`, `Write`, `NotebookEdit`
- `status = "exploring"` when no write-type call exists anywhere in the full session
- `status = "implementing"` when at least one write-type call exists in the window
- `status = "drifting"` when write-type call exists session-wide, but the window has 0 write calls AND reads*100/total >= 60

**Step 3: Create `/Users/dayna.blackwell/code/claudewatch/internal/mcp/drift_tools_test.go`**

The file must be `package mcp`. Use `writeActiveToolJSONL` (defined in
`live_tools_test.go`) for building JSONL fixtures. Use `addDriftTools` to
register the tool on the test server. Use `callTool(s, "get_drift_signal", ...)`.
The result type to assert is `DriftSignalResult`.

Write exactly these 5 tests:

**Test 1: TestGetDriftSignal_NoActiveSession**
- Empty tmpDir, no JSONL files.
- Call `get_drift_signal`, expect error `"no active session found"`.

**Test 2: TestGetDriftSignal_Exploring**
- Session with only `Read` and `Grep` tool_use calls (no Edit/Write anywhere).
- Expected: `Status == "exploring"`, `HasAnyEdit == false`, `Live == true`.
- Use JSONL lines with `"type":"assistant"` entries containing `tool_use` blocks
  for Read and Grep, and corresponding `"type":"user"` `tool_result` blocks.

**Test 3: TestGetDriftSignal_Implementing**
- Session with at least one `Edit` tool_use call in the window.
- Expected: `Status == "implementing"`, `HasAnyEdit == true`, `WriteCalls >= 1`.

**Test 4: TestGetDriftSignal_Drifting**
- Session where Edit was called earlier (full session has edit) but the last 20
  tool calls are all reads (>= 60% read, 0 writes).
- Build this with enough Read/Grep calls after a single Edit so the window is
  read-only. Use at least 15 Read calls after the Edit.
- Expected: `Status == "drifting"`, `HasAnyEdit == true`, `WriteCalls == 0`,
  `ReadCalls >= 9` (>= 60% of window).

**Test 5: TestGetDriftSignal_MCP**
- Verify that calling `addDriftTools(s)` causes `get_drift_signal` to appear
  in `s.tools`.
- No JSONL needed; just check tool registration.

**JSONL line format for assistant with tool_use:**
```
{"type":"assistant","sessionId":"SESSID","timestamp":"2026-03-01T10:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-1","name":"Read","input":{}}],"usage":{"input_tokens":100,"output_tokens":50}}}
```

**JSONL line format for user tool_result (success):**
```
{"type":"user","sessionId":"SESSID","timestamp":"2026-03-01T10:00:01Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-1","content":"ok"}]}}
```

**Step 4: Verify.**

Run:
```
cd /Users/dayna.blackwell/code/claudewatch && go test ./internal/mcp/... -run TestGetDriftSignal -v
```

All 5 tests must pass. Fix any failures before reporting done.

---

## Wave Execution Loop

```
Orchestrator:
  1. Dispatch Agent A and Agent B simultaneously (Wave 1).
  2. Wait for both to complete.
  3. Merge: verify no file conflicts (A owns drift_tools.go + tools.go modification;
     B owns drift_tools_test.go — no overlap).
  4. Run full test suite: go test ./...
  5. If all pass, proceed to Post-Merge Checklist.
  6. If failures: diagnose, fix inline (Orchestrator handles if trivial),
     or re-dispatch single agent for targeted fix.
```

---

## Orchestrator Post-Merge Checklist

- [ ] `go build ./...` — clean
- [ ] `go vet ./...` — clean
- [ ] `go test ./...` — all pass
- [ ] `go test ./internal/mcp/... -run TestGetDriftSignal -v` — 5 tests pass
- [ ] `grep -r "get_drift_signal" internal/mcp/` — appears in both `drift_tools.go` and the compiled tool list (via `tools.go`)
- [ ] `grep "addDriftTools" internal/mcp/tools.go` — present exactly once, after `addDashboardTools(s)`
- [ ] Confirm `DriftSignalResult` JSON field names match: `session_id`, `live`, `window_n`, `read_calls`, `write_calls`, `has_any_edit`, `status`
- [ ] No changes to `internal/claude/active_live.go` (read-only)
- [ ] No changes to `internal/mcp/dashboard_tools.go` (read-only)

---

## Status

- [ ] Wave 1 dispatched
- [ ] Agent A complete: `drift_tools.go` created, `tools.go` modified
- [ ] Agent B complete: `drift_tools_test.go` created, all tests pass
- [ ] Merge complete, no conflicts
- [ ] Full test suite passing
- [ ] Post-merge checklist signed off
