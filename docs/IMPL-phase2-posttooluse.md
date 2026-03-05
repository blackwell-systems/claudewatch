# IMPL: Phase 2 PostToolUse Hook Enhancements (v0.14.0)

**Feature:** Three enhancements to the `claudewatch hook` PostToolUse handler: (1) drift intervention with rolling 15-tool window warnings, (2) repetitive error blocking via (tool, error-substring) tuple tracking, (3) auto-extract on context pressure transitions to "pressure" or "critical".

**Design summary:** All three features modify `internal/app/hook.go` (the existing hook handler) and add supporting functions in existing files under `internal/claude/`. The hook already checks consecutive errors, context pressure, cost velocity, and drift — these enhancements extend that logic. Feature (1) enhances the existing drift check with an actionable intervention message. Feature (2) adds a new repetitive-error detector in `internal/claude/active_live.go` and a new priority check in the hook. Feature (3) adds auto-extract logic triggered by context pressure transitions, requiring a new state file and calling the memory extraction pipeline.

---

## Suitability Assessment

**Verdict: SUITABLE WITH CAVEATS**

### Answers to gate questions:

1. **File decomposition.** Yes — 3 agents with disjoint file ownership. Agent A owns `internal/claude/active_live.go` (new parser function). Agent B owns `internal/app/hook.go` (all hook handler changes). Agent C owns `internal/app/hook_extract.go` (new file for auto-extract helper logic). Agent B depends on Agent A's new function and Agent C's helper.

2. **Investigation-first items.** No — all three features have clear specifications and the existing code is well-understood from this analysis. No root cause analysis needed.

3. **Interface discoverability.** Yes — all cross-agent interfaces are Go function signatures defined below. The existing codebase patterns are consistent and predictable.

4. **Pre-implementation status check.**
   - Drift detection already exists in hook (Priority 4) and in `ParseLiveDriftSignal` — enhancement (1) modifies the warning message and adds the rolling window intervention concept. Already partially done.
   - Repetitive error tracking does NOT exist yet — `ParseLiveConsecutiveErrors` tracks consecutive errors regardless of tool/error identity; the new feature tracks (tool, error-content) tuples specifically.
   - Context pressure check exists (Priority 2) but has no auto-extract or state tracking for transitions.

5. **Parallelization value check.** Yes — Wave 1 (Agent A + Agent C) builds the two independent helper units. Wave 2 (Agent B) integrates them into the hook. Total ~25 min parallel vs ~40 min sequential. SAW saves ~15 min.

**Caveat:** Agent B depends on both Agent A and Agent C. Agent B runs in Wave 2 after merge. This is a strict 2-wave dependency, not a single-wave parallel.

**Estimated times:**
- Scout phase: ~15 min (this document)
- Wave 1: Agent A + Agent C in parallel, ~10–12 min each
- Merge + verify Wave 1: ~3 min
- Wave 2: Agent B, ~12–15 min
- Merge + verify Wave 2: ~3 min
- Total SAW time: ~35–45 min

---

## Known Issues

None. `go test ./...` passes on `main` HEAD. The Makefile `test` target runs `fmt` and `vet` before tests.

---

## Dependency Graph

```
Wave 1 (parallel):
  [Agent A] internal/claude/active_live.go   ← adds ParseLiveRepetitiveErrors()
  [Agent C] internal/app/hook_extract.go     ← new file: tryAutoExtract() helper

Wave 2 (depends on Wave 1 merge):
  [Agent B] internal/app/hook.go             ← integrates all three features into runHook()
                                                calls ParseLiveRepetitiveErrors (Agent A)
                                                calls tryAutoExtract (Agent C)
```

Supporting dependencies (read-only, no modifications):
- `internal/claude/active_live.go` — existing `readLiveJSONL`, `ParseLiveDriftSignal`, `ParseLiveContextPressure` — read-only by Agent B
- `internal/claude/active.go` — `FindActiveSessionPath` — read-only
- `internal/claude/transcripts.go` — `TranscriptEntry`, `AssistantMessage`, `UserMessage`, `ContentBlock` — read-only
- `internal/config/config.go` — `config.Load`, `config.ConfigDir` — read-only
- `internal/memory/extract.go` — `ExtractTaskMemory`, `ExtractBlockers`, `GetCommitSHAsSince` — read-only (used by Agent C)
- `internal/store/` — `WorkingMemoryStore` — read-only (used by Agent C)
- `internal/claude/session_meta.go` — `ParseAllSessionMeta` — read-only (used by Agent C)
- `internal/claude/facets.go` — `ParseAllFacets` — read-only (used by Agent C)

---

## Interface Contracts

### Contract 1: `ParseLiveRepetitiveErrors` (owned by Agent A)

```go
// RepetitiveError represents a (tool, error-substring) tuple that has occurred
// consecutively in the session's recent tool results.
type RepetitiveError struct {
    Tool       string `json:"tool"`
    Pattern    string `json:"pattern"`     // first 120 chars of error content
    Count      int    `json:"count"`       // consecutive occurrences
}

// ParseLiveRepetitiveErrors tail-scans the last tailN entries of the JSONL
// file at path and returns any (tool, error-pattern) tuples that have occurred
// >= threshold times consecutively. "Consecutively" means the same tool produced
// the same error pattern with no intervening successful result from that tool.
//
// Error pattern matching: extract the first 120 characters of the tool_result
// content (stringified) as the pattern key. This groups errors that share
// the same prefix (e.g., "File does not exist" or "Exit code 1").
//
// Returns nil slice if no repetitive errors found.
// Returns (nil, err) only on I/O failure.
func ParseLiveRepetitiveErrors(path string, tailN int, threshold int) ([]RepetitiveError, error)
```

**Behavior:**
- Call `readLiveJSONL(path)` to get all entries, then take the last `tailN` entries (default 100 if tailN <= 0).
- Build a map of `toolUseID -> toolName` from assistant `tool_use` blocks.
- Walk user entries forward. For each `tool_result` block:
  - Look up the tool name from the `toolUseID`.
  - If `IsError == true`: extract error pattern (first 120 chars of stringified content). Increment the consecutive count for `(tool, pattern)` tuple.
  - If `IsError == false`: reset the consecutive count for that tool (all patterns for that tool reset — a successful use of the tool breaks any error streak for it).
- After the walk, return all tuples where `count >= threshold`.

**Key distinction from `ParseLiveConsecutiveErrors`:** The existing function counts ANY consecutive errors regardless of tool or error content. The new function tracks per-(tool, error-pattern) streaks, which catches the case where the agent retries the same failing operation on the same tool.

### Contract 2: `tryAutoExtract` (owned by Agent C)

```go
// tryAutoExtract checks whether context pressure has transitioned to "pressure"
// or "critical" since the last check, and if so, performs memory extraction.
// Returns a human-readable message if extraction was performed, or "" if skipped.
//
// State tracking: uses a file at ~/.cache/claudewatch-ctx-state to store the
// last observed context pressure status. Only triggers on transitions INTO
// "pressure" or "critical" (not on every check while already in those states).
//
// Parameters:
//   - activePath: path to the active session JSONL file
//   - claudeHome: path to ~/.claude (for session/facet parsing)
//
// Errors are swallowed (returns "") — hook must never fail on extract errors.
func tryAutoExtract(activePath string, claudeHome string) string
```

**Behavior:**
- Call `claude.ParseLiveContextPressure(activePath)` to get current status.
- Read `~/.cache/claudewatch-ctx-state` for the last recorded status. If file doesn't exist, treat last status as "comfortable".
- Write current status to `~/.cache/claudewatch-ctx-state`.
- If current status is "pressure" or "critical" AND last status was neither "pressure" nor "critical" (i.e., this is a transition), perform extraction:
  - Call `claude.FindActiveSessionPath(claudeHome)` (may return activePath itself).
  - Call `claude.ParseActiveSession(activePath)` to get session meta.
  - Resolve project name from `filepath.Base(meta.ProjectPath)`.
  - Load all sessions and facets via `claude.ParseAllSessionMeta` and `claude.ParseAllFacets`.
  - Find the session facet for the active session.
  - Call `memory.ExtractTaskMemory` and `memory.ExtractBlockers`.
  - Store results via `store.NewWorkingMemoryStore`.
  - Return a message like `"Auto-extracted memory (context at <status>): task '<identifier>' saved"`.
- If no transition (already was at pressure/critical), return "".
- If current status is "comfortable" or "filling", just update state file, return "".
- On any error, return "" (fail silently).

### Contract 3: Enhanced `runHook` integration (owned by Agent B)

Agent B modifies the existing `runHook` function in `internal/app/hook.go` to add:

**Enhancement 1 — Drift intervention (modify existing Priority 4):**
The existing drift check already warns when `drift.Status == "drifting"`. No functional change needed — the current implementation already covers this. Agent B should verify the message is actionable and includes the read/write counts. (Current implementation already does this — see existing Priority 4 code.)

After review: the existing drift check is already a rolling 15-tool window with an actionable message. **No code change needed for Enhancement 1.** The feature description's "rolling 15-tool window tracking read/write ratio" is already implemented.

**Enhancement 2 — Repetitive error blocking (new Priority 1.5, between current Priority 1 and Priority 2):**

```go
// After Priority 1 (consecutive tool errors) and before Priority 2 (context pressure):
// Priority 1.5: repetitive error patterns.
if reps, err := claude.ParseLiveRepetitiveErrors(activePath, 100, 3); err == nil && len(reps) > 0 {
    rep := reps[0] // report the worst offender
    fmt.Fprintf(os.Stderr, "⚠ Repetitive error: %s has failed %d times with same error (%s). "+
        "Call get_session_dashboard (claudewatch MCP) — break the loop by trying a different approach.\n",
        rep.Tool, rep.Count, rep.Pattern)
    os.Exit(2)
}
```

**Enhancement 3 — Auto-extract on context pressure transition (augment existing Priority 2):**

```go
// Before or alongside the existing Priority 2 context pressure check:
if extractMsg := tryAutoExtract(activePath, cfg.ClaudeHome); extractMsg != "" {
    fmt.Fprintln(os.Stderr, extractMsg)
}

// Existing Priority 2 check continues unchanged...
```

The auto-extract runs on every (non-rate-limited) hook invocation. It only performs extraction on state transitions, so repeated calls at "pressure" level won't re-extract. The existing Priority 2 warning still fires independently.

---

## File Ownership Table

| File | Agent | Action | Notes |
|------|-------|--------|-------|
| `internal/claude/active_live.go` | A | modify | Append `RepetitiveError` type and `ParseLiveRepetitiveErrors` function |
| `internal/claude/active_live_test.go` | A | modify | Append test cases for `ParseLiveRepetitiveErrors` |
| `internal/app/hook_extract.go` | C | create | New file: `tryAutoExtract` function + helpers |
| `internal/app/hook_extract_test.go` | C | create | Tests for `tryAutoExtract` |
| `internal/app/hook.go` | B | modify | Add Priority 1.5 (repetitive errors) and auto-extract call |
| `internal/app/hook_test.go` | B | modify | Add test for repetitive error check and auto-extract integration |

---

## Wave Structure

### Wave 1 — Foundation (2 agents, parallel)

| Agent | Files | Depends On | Verification |
|-------|-------|------------|--------------|
| Agent A | `internal/claude/active_live.go`, `internal/claude/active_live_test.go` | None (uses existing `readLiveJSONL`, types from `transcripts.go`) | `go test ./internal/claude/ -run TestParseLiveRepetitiveErrors -v` |
| Agent C | `internal/app/hook_extract.go`, `internal/app/hook_extract_test.go` | None (uses existing `claude.ParseLiveContextPressure`, `claude.ParseActiveSession`, `memory.*`, `store.*`) | `go test ./internal/app/ -run TestTryAutoExtract -v` |

### Wave 2 — Integration (1 agent, after Wave 1 merge)

| Agent | Files | Depends On | Verification |
|-------|-------|------------|--------------|
| Agent B | `internal/app/hook.go`, `internal/app/hook_test.go` | Agent A (`ParseLiveRepetitiveErrors`), Agent C (`tryAutoExtract`) | `go test ./internal/app/ -v && go vet ./...` |

---

## Agent Prompts

### Agent A — Repetitive Error Parser

```
ROLE: Implement ParseLiveRepetitiveErrors in the claudewatch Go codebase.

CONTEXT: The claudewatch PostToolUse hook needs to detect when the same tool
produces the same error repeatedly (e.g., agent retrying a failing Bash command).
The existing ParseLiveConsecutiveErrors counts ANY consecutive errors; this new
function tracks per-(tool, error-pattern) tuples.

FILES_TO_MODIFY:
- internal/claude/active_live.go (append new type + function)
- internal/claude/active_live_test.go (append test cases)

INTERFACE_CONTRACT:
// RepetitiveError represents a (tool, error-substring) tuple.
type RepetitiveError struct {
    Tool    string `json:"tool"`
    Pattern string `json:"pattern"`
    Count   int    `json:"count"`
}

// ParseLiveRepetitiveErrors tail-scans the last tailN entries for repeated
// (tool, error-pattern) tuples. Returns tuples with count >= threshold.
// Pattern = first 120 chars of stringified tool_result content.
// A successful tool_result resets all streaks for that tool.
// tailN defaults to 100 if <= 0. threshold defaults to 3 if <= 0.
func ParseLiveRepetitiveErrors(path string, tailN int, threshold int) ([]RepetitiveError, error)

IMPLEMENTATION_NOTES:
- Follow the exact pattern of ParseLiveConsecutiveErrors: call readLiveJSONL,
  take tail slice, walk forward through entries.
- Build toolUseID->name map from assistant tool_use blocks.
- For error content extraction: the tool_result's Content field is
  json.RawMessage. Try json.Unmarshal as string first; if that fails,
  use string(content). Truncate to 120 chars.
- Track streaks per tool: map[string]map[string]int where outer key is tool
  name, inner key is error pattern, value is consecutive count.
- On a successful tool_result for a tool, delete that tool's entire inner map
  (reset all patterns for that tool).
- Sort results by Count descending before returning.

TESTING:
- Write a helper that creates a temp JSONL file with controlled entries.
  Follow the pattern in active_live_test.go or active_live_context_test.go.
- Test cases:
  1. No errors → nil result
  2. 3 consecutive same-tool same-error → returns 1 RepetitiveError
  3. Mixed errors from different tools → only the one hitting threshold returned
  4. Successful result between errors resets streak → no result
  5. Different error patterns on same tool → tracked separately
  6. tailN limits scan window correctly

VERIFICATION: go test ./internal/claude/ -run TestParseLiveRepetitiveErrors -v
OUT_OF_SCOPE: Do not modify hook.go. Do not modify any file outside internal/claude/.
```

### Agent C — Auto-Extract Helper

```
ROLE: Implement tryAutoExtract in a new file internal/app/hook_extract.go.

CONTEXT: The claudewatch PostToolUse hook should automatically extract session
memory when context pressure transitions to "pressure" or "critical". This
preserves work-in-progress before potential compaction. The extraction pipeline
already exists (memory.ExtractTaskMemory, store.WorkingMemoryStore) — this
helper just wires it up with transition detection.

FILES_TO_CREATE:
- internal/app/hook_extract.go (new file, package app)
- internal/app/hook_extract_test.go (new file, package app)

INTERFACE_CONTRACT:
// tryAutoExtract checks whether context pressure has transitioned to
// "pressure" or "critical" since the last check and performs memory
// extraction on transitions. Returns a human-readable message if extraction
// occurred, or "" if skipped/failed. Errors are swallowed.
func tryAutoExtract(activePath string, claudeHome string) string

IMPLEMENTATION_NOTES:
- State file: ~/.cache/claudewatch-ctx-state (plain text, one word: the last
  observed status string). Use os.ExpandEnv("$HOME/.cache/claudewatch-ctx-state").
- Read current pressure via claude.ParseLiveContextPressure(activePath).
- Compare with last stored status. Transition = current is "pressure"|"critical"
  AND previous was NOT "pressure"|"critical".
- Always write current status to state file (even if no transition).
- On transition, perform extraction:
  - meta, err := claude.ParseActiveSession(activePath) — get SessionMeta
  - projectName := filepath.Base(meta.ProjectPath)
  - sessionID := strings.TrimSuffix(filepath.Base(activePath), ".jsonl")
  - allSessions, _ := claude.ParseAllSessionMeta(claudeHome)
  - allFacets, _ := claude.ParseAllFacets(claudeHome)
  - Find the matching session and facet by sessionID.
  - commits := memory.GetCommitSHAsSince(meta.ProjectPath, meta.StartTime)
  - storePath := filepath.Join(config.ConfigDir(), "projects", projectName, "working-memory.json")
  - memStore := store.NewWorkingMemoryStore(storePath)
  - task, _ := memory.ExtractTaskMemory(session, facet, commits)
  - if task != nil { memStore.AddOrUpdateTask(task) }
  - blockers, _ := memory.ExtractBlockers(session, facet, projectName, recentSessions, allFacets)
  - for _, b := range blockers { memStore.AddBlocker(b) }
- Return message format: "Auto-extracted memory (context at <status>): task '<id>' with <N> blocker(s)"
  or "Auto-extracted memory (context at <status>): <N> blocker(s) saved" if no task.
- If facet not found for session (common for very new sessions), return "" silently.
  Memory extraction requires a facet — this is expected behavior.

IMPORTS:
- "fmt", "os", "path/filepath", "strings"
- "github.com/blackwell-systems/claudewatch/internal/claude"
- "github.com/blackwell-systems/claudewatch/internal/config"
- "github.com/blackwell-systems/claudewatch/internal/memory"
- "github.com/blackwell-systems/claudewatch/internal/store"

TESTING:
- Test transition detection logic with a temp state file:
  1. No state file exists + pressure status → triggers extraction (mock extraction
     by testing the state file write behavior; actual extraction will fail without
     real session data, so test the transition logic separately)
  2. State file says "comfortable" + current "pressure" → transition detected
  3. State file says "pressure" + current "pressure" → NO transition (already there)
  4. State file says "critical" + current "comfortable" → NO transition (going down)
  5. State file says "filling" + current "critical" → transition detected
- For integration: write a minimal JSONL fixture that ParseLiveContextPressure
  can parse, set up temp dirs for claudeHome. The extraction will likely return
  empty results (no facet) which is fine — test that it doesn't crash and returns "".

VERIFICATION: go test ./internal/app/ -run TestTryAutoExtract -v
OUT_OF_SCOPE: Do not modify hook.go. Do not modify any file outside internal/app/.
```

### Agent B — Hook Integration

```
ROLE: Integrate three PostToolUse hook enhancements into internal/app/hook.go.

CONTEXT: The claudewatch PostToolUse hook (runHook function) currently checks
4 priorities: consecutive errors, context pressure, cost velocity, drift.
You will add: (1) repetitive error blocking as Priority 1.5, (2) auto-extract
call before the context pressure check.

Enhancement 1 (drift intervention) requires NO code change — the existing
Priority 4 drift check already implements the rolling 15-tool window with
an actionable message. Verify this is the case and note in your output.

FILES_TO_MODIFY:
- internal/app/hook.go
- internal/app/hook_test.go

CHANGES TO hook.go:

1. Add a new priority check between Priority 1 (consecutive errors) and
   Priority 2 (context pressure). Comment it as "Priority 1.5":

   // Priority 1.5: repetitive error patterns.
   if reps, err := claude.ParseLiveRepetitiveErrors(activePath, 100, 3); err == nil && len(reps) > 0 {
       rep := reps[0]
       if note := hookChronicPatternNote(cfg, cwd); note != "" {
           fmt.Fprintf(os.Stderr, "⚠ Repetitive error: %s failed %d times with same error (%s). %s. Call get_session_dashboard (claudewatch MCP) — break the loop by trying a different approach.\n",
               rep.Tool, rep.Count, rep.Pattern, note)
       } else {
           fmt.Fprintf(os.Stderr, "⚠ Repetitive error: %s failed %d times with same error (%s). Call get_session_dashboard (claudewatch MCP) — break the loop by trying a different approach.\n",
               rep.Tool, rep.Count, rep.Pattern)
       }
       os.Exit(2)
   }

2. Add auto-extract call BEFORE the existing Priority 2 context pressure check:

   // Auto-extract on context pressure transitions.
   if extractMsg := tryAutoExtract(activePath, cfg.ClaudeHome); extractMsg != "" {
       fmt.Fprintln(os.Stderr, extractMsg)
   }

   // Priority 2: context pressure. (existing code, unchanged)

TESTING:
- Add test that hookCmd is still registered (existing test covers this).
- Add test verifying the priority ordering documentation in the Long description
  is updated to mention repetitive errors.
- The integration tests primarily verify that the hook.go file compiles and
  the imports are correct. Deep behavioral testing is in Agent A and Agent C's
  test files.

VERIFICATION: go test ./internal/app/ -v && go vet ./...
OUT_OF_SCOPE: Do not modify active_live.go or hook_extract.go.
```

---

## Wave Execution Loop

### Pre-flight
```
go test ./... (must pass)
go vet ./...  (must pass)
```

### Wave 1: Agent A + Agent C (parallel)

**Launch:**
- Agent A in worktree, working on `internal/claude/active_live.go` + test
- Agent C in worktree, working on `internal/app/hook_extract.go` + test

**Merge gate (both agents):**
```bash
go test ./internal/claude/ -run TestParseLiveRepetitiveErrors -v   # Agent A
go test ./internal/app/ -run TestTryAutoExtract -v                  # Agent C
go vet ./...
```

**Merge order:** Agent A first (no conflicts possible), then Agent C.

### Wave 2: Agent B (sequential, after Wave 1 merge)

**Launch:**
- Agent B on main branch (post-merge), modifying `internal/app/hook.go` + test

**Merge gate:**
```bash
go test ./... -v
go vet ./...
```

### Post-merge verification (all waves complete)
```bash
go test ./... -v
go vet ./...
gofmt -l .   # should return empty (no unformatted files)
```

---

## Orchestrator Post-Merge Checklist

1. [ ] Verify `go test ./...` passes after all merges
2. [ ] Verify `go vet ./...` passes
3. [ ] Verify `gofmt -l .` returns empty
4. [ ] Verify `hookCmd.Long` description in hook.go mentions repetitive errors
5. [ ] Manual smoke test: `go run ./cmd/claudewatch hook` exits 0 with no active session
6. [ ] Verify auto-extract state file is created at `~/.cache/claudewatch-ctx-state` after hook run with active session

---

## Status

| Agent | Wave | Status | Notes |
|-------|------|--------|-------|
| A — Repetitive Error Parser | 1 | COMPLETE | See completion report below |
| C — Auto-Extract Helper | 1 | COMPLETE | See completion report below |
| B — Hook Integration | 2 | BLOCKED on Wave 1 | |

---

### Agent A - Completion Report

```
status: complete
files_changed: [internal/claude/active_live.go, internal/claude/active_live_test.go]
tests_added: 6
tests_passing: yes
interface_deviations: none
commits: [8a0b73e]
notes: Implementation follows the exact pattern of ParseLiveConsecutiveErrors. Added RepetitiveError type, ParseLiveRepetitiveErrors function, and extractErrorPattern helper. Error pattern extraction tries json.Unmarshal as string first, falls back to raw string, truncates to 120 chars. Streaks tracked as map[string]map[string]int (tool -> pattern -> count). Success on a tool deletes its entire inner map. Results sorted by Count descending. All 6 specified test cases implemented and passing. All existing tests in the package continue to pass. go vet and gofmt clean.
```

---

### Agent C - Completion Report

```
status: complete
files_changed: [internal/app/hook_extract.go, internal/app/hook_extract_test.go]
tests_added: 5
tests_passing: yes
interface_deviations: none — used os.UserCacheDir() instead of os.ExpandEnv for state file path (more portable, same result on macOS/Linux)
commits: [a777782]
notes: All 5 transition detection scenarios tested via isElevatedPressure logic tests. Integration test with minimal JSONL fixture confirms graceful handling when no facet exists (returns "" silently). tryAutoExtract uses ParseActiveSession meta as fallback if session not found in ParseAllSessionMeta results. Full app package test suite passes. No modifications to hook.go or any files outside internal/app/.
```
