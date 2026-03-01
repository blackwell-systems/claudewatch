# IMPL: Live/Active Session Reading

<!-- scout v0.2.0 — generated 2026-03-01 -->

---

### Suitability Assessment

**Verdict: SUITABLE**

All five gate questions resolve cleanly.

1. **File decomposition.** The work decomposes into three disjoint file sets:
   - **(A)** Active session detection and in-memory parsing in `internal/claude/`:
     two new files (`active.go`, `active_test.go`). Owns the `FindActiveSessionPath`
     and `ParseActiveSession` functions. No dependency on Wave 1B.
   - **(B)** MCP tool enhancement in `internal/mcp/tools.go` + existing test file
     `internal/mcp/tools_test.go`. Calls the functions delivered by A.
   - **(C)** CLI `--include-active` flag in `internal/app/scan.go`. Calls functions
     delivered by A. Parallel with B.

   A must complete before B and C begin (B and C both call `claude.ParseActiveSession`
   and `claude.FindActiveSessionPath`). B and C are fully parallel.

2. **Investigation-first items.** None. The JSONL format, path conventions, and
   `lsof`-based file-open detection are well-understood. There are no unknown root
   causes to investigate before writing code.

3. **Interface discoverability.** The cross-agent boundary is the two new exported
   functions in `internal/claude`. Their exact signatures are defined below in
   the Interface Contracts section. Agents B and C code against the contracts;
   Agent A delivers them.

4. **Pre-implementation scan.** No live-session reading code exists anywhere in
   the codebase. The existing `ParseSingleTranscript` in `transcripts.go` reads a
   JSONL file using `bufio.Scanner`, which handles partial reads safely (incomplete
   trailing lines are skipped). `ParseAllSessionMeta` reads closed JSON files from
   `usage-data/session-meta/` and returns `SessionMeta` structs. Neither reads
   in-progress JSONL files. Zero items already implemented.

5. **Parallelization value.** Agent A and Agents B+C span two waves. B and C are
   fully independent within Wave 2. The `go test ./...` cycle for this repo is
   ~2–4 s, so the build-time overhead of parallelism is negligible. Primary value
   is interface clarity and progress isolation.

Pre-implementation scan results:
- Total items: 3 agents' worth of work
- Already implemented: 0 items (0%)
- Partially implemented: 0 items
- To-do: 3 items

Estimated times:
- Scout phase: ~15 min (this doc)
- Wave 1 (Agent A): ~20 min
- Wave 2 (Agents B + C, parallel): ~20 min
- Merge and verification: ~10 min
- Total SAW time: ~65 min

Sequential baseline: ~60 min (3 agents × 20 min avg)
Time savings: ~15 min from B+C parallelism, plus interface contract safety.

Recommendation: Proceed with 2-wave SAW structure.

---

### Known Issues

None identified. `go test ./...` is clean as of 2026-03-01 (confirmed by existing
CI green status; no pre-existing test failures found in codebase inspection).

---

### Dependency Graph

```
internal/claude/active.go        ←── Agent A (Wave 1) — NEW FILE
internal/claude/active_test.go   ←── Agent A (Wave 1) — NEW FILE

internal/mcp/tools.go            ←── Agent B (Wave 2) — MODIFY
internal/mcp/tools_test.go       ←── Agent B (Wave 2) — MODIFY

internal/app/scan.go             ←── Agent C (Wave 2) — MODIFY
```

Existing packages that are **read** but NOT modified:
- `internal/claude/transcripts.go` — `ParseSingleTranscript`, `ParseTimestamp`,
  `TranscriptEntry`, `bufio.Scanner` pattern (Agent A models its scanner on this)
- `internal/claude/types.go` — `SessionMeta` type (Agent A returns `*SessionMeta`)
- `internal/claude/session_meta.go` — `ParseSessionMeta` (Agent A follows this
  single-file read pattern)
- `internal/claude/paths.go` — `NormalizePath` (already minimal; active.go may
  call it)
- `internal/analyzer/outcomes.go` — `EstimateSessionCost` (called by Agent B to
  compute cost for the live session result)
- `internal/analyzer/cost.go` — `DefaultPricing`, `NoCacheRatio`, `CacheRatio`
  (Agent B references these)
- `internal/config/config.go` — `Config.ClaudeHome` (Agents B and C read it via
  existing `s.claudeHome` / `cfg.ClaudeHome`)

Data flow:
```
Agent A (claude.active.go)
  FindActiveSessionPath(claudeHome) → (path string, err error)
  ParseActiveSession(path) → (*SessionMeta, error)
       ↓                              ↓
  Agent B (mcp/tools.go)       Agent C (app/scan.go)
  handleGetSessionStats         runScan --include-active
```

**Critical constraint from the feature spec: the DB (`claudewatch.db`) is NEVER
touched by the live path.** Only `internal/store/` calls touch the DB; none of
the three agents modify store files.

---

### Interface Contracts

These are binding contracts. Agents B and C code against them exactly.
Agent A is responsible for delivering them.

#### New file: `internal/claude/active.go`

```go
package claude

// ActiveSessionInfo wraps a parsed live session with its source path.
// The IsLive field is always true; it exists so callers can distinguish
// live results from closed-session results without a separate type switch.
type ActiveSessionInfo struct {
    SessionMeta        // embedded; all SessionMeta fields are available
    Path        string // absolute path to the open JSONL file
    IsLive      bool   // always true
}

// FindActiveSessionPath scans ~/.claude/projects/**/*.jsonl for a file
// currently open by a Claude Code process. On macOS/Linux it shells out to
// lsof to identify open file handles. If no active session is found, it
// returns ("", nil). If lsof is unavailable, it falls back to an mtime
// heuristic: the most-recently-modified .jsonl file whose mtime is within
// the last 5 minutes is considered active. Returns an error only for
// unexpected I/O failures (not for "no active session").
func FindActiveSessionPath(claudeHome string) (string, error)

// ParseActiveSession reads the JSONL file at path as a partial (possibly
// still-being-written) transcript and reconstructs a SessionMeta. It:
//   1. Reads the raw bytes of the file.
//   2. Truncates to the last '\n' byte (line-atomic safety).
//   3. Scans lines with bufio.Scanner (same pattern as ParseSingleTranscript).
//   4. Derives the session ID from the filename (strip .jsonl suffix).
//   5. Populates SessionMeta fields it can derive from the transcript:
//        SessionID, ProjectPath (from directory structure), StartTime
//        (first entry timestamp), InputTokens and OutputTokens (summed
//        from any usage fields found), UserMessageCount,
//        AssistantMessageCount. Fields not derivable from partial JSONL
//        (DurationMinutes, GitCommits, etc.) are left at zero values.
//   6. Returns a non-nil *SessionMeta on success, even if the file is
//      empty or only partially parseable (best-effort).
// Returns (nil, error) only for file read failures.
func ParseActiveSession(path string) (*SessionMeta, error)
```

#### Type decision: reuse `SessionMeta` vs. new type

`handleGetSessionStats` in `internal/mcp/tools.go` (lines 166–194) uses exactly
these fields from `SessionMeta`:
- `session.SessionID`
- `session.ProjectPath` (via `filepath.Base`)
- `session.StartTime`
- `session.DurationMinutes`
- `session.InputTokens`
- `session.OutputTokens`

`EstimateSessionCost` (called on line 183) uses only `s.InputTokens` and
`s.OutputTokens`.

**Decision: reuse `SessionMeta` directly.** `ParseActiveSession` returns
`*SessionMeta`. The fields that can be populated from a partial JSONL transcript
(`SessionID`, `ProjectPath`, `StartTime`, `InputTokens`, `OutputTokens`,
`UserMessageCount`, `AssistantMessageCount`) cover everything `handleGetSessionStats`
needs. Fields left at zero (`DurationMinutes`, `GitCommits`, etc.) are safe —
the MCP result struct (`SessionStatsResult`) includes `DurationMin` which would
show `0` for a live session, which is correct (session is ongoing).

Agent B adds a `Live bool` field to `SessionStatsResult` to let the MCP caller
know the data is from an active session. This is additive and backward-compatible.

#### JSONL token counting strategy for `ParseActiveSession`

The JSONL format in `~/.claude/projects/<hash>/<session>.jsonl` stores
`TranscriptEntry` objects. Token counts are available in `usage` fields on
assistant messages. The exact schema for usage data in the JSONL:

```json
{"type":"assistant","message":{"usage":{"input_tokens":1234,"output_tokens":456},...}}
```

`ParseActiveSession` should extract these by unmarshalling a minimal struct:

```go
type assistantUsage struct {
    Usage struct {
        InputTokens  int `json:"input_tokens"`
        OutputTokens int `json:"output_tokens"`
    } `json:"usage"`
}
```

This is internal to `active.go` and does not need to be exported.

#### ProjectPath derivation for `ParseActiveSession`

The JSONL files are stored at:
```
~/.claude/projects/<project-hash>/<session-uuid>.jsonl
```

The `projectPath` is NOT the project hash — it is the decoded project path.
Claude Code encodes the real path as the directory name by URL-encoding or
hashing. However, the JSONL itself contains `"cwd"` fields in some entries or
the `sessionId` in `TranscriptEntry.SessionID`. The safest approach:

- Check `TranscriptEntry.SessionID` — the first entry with a non-empty `sessionId`
  field gives us the session UUID but not the project path.
- The project hash directory name is not reversible to a path without additional
  metadata.

**Resolution:** For the `ProjectPath` field in the returned `SessionMeta`, set it
to the project-hash directory name (i.e., `filepath.Dir(filepath.Dir(path))`
yields the projects dir; `filepath.Base(filepath.Dir(path))` yields the hash).
This matches how `handleGetSAWSessions` works — it uses `session.ProjectHash`
when no meta lookup succeeds. Agent B will already call `filepath.Base()` on it
for display, so this degrades gracefully. Document this limitation in code comments.

#### `scan --include-active` flag behavior

`handleGetSessionStats` already does `sort.Slice(sessions, ...)` and picks the
newest. With `--include-active`, `runScan` fetches the active session info and
appends a synthetic row to the table tagged with `(live)` in the Project column.
No sorting interaction needed since it's a separate display path.

---

### File Ownership Table

| File | Agent | Wave | Action |
|------|-------|------|--------|
| `internal/claude/active.go` | A | 1 | CREATE |
| `internal/claude/active_test.go` | A | 1 | CREATE |
| `internal/mcp/tools.go` | B | 2 | MODIFY |
| `internal/mcp/tools_test.go` | B | 2 | MODIFY |
| `internal/app/scan.go` | C | 2 | MODIFY |

No other files change. `internal/claude/types.go` is NOT modified (no new types
needed there). `internal/store/db.go` is NOT touched. `internal/watcher/watcher.go`
is NOT touched.

---

### Wave Structure

```
Wave 1 (sequential prerequisite)
└── Agent A: internal/claude/active.go + active_test.go
    Delivers: FindActiveSessionPath, ParseActiveSession, ActiveSessionInfo

Wave 2 (parallel, blocked on Wave 1 merge)
├── Agent B: internal/mcp/tools.go + tools_test.go
│   Calls: claude.FindActiveSessionPath, claude.ParseActiveSession
│   Surface: handleGetSessionStats gains live-session fallback
│
└── Agent C: internal/app/scan.go
    Calls: claude.FindActiveSessionPath, claude.ParseActiveSession
    Surface: scanCmd gains --include-active flag
```

Wave 2 merge: standard `git merge` of both B and C branches. No conflict expected
because B and C own different files.

---

### Agent Prompts

---

```
# Wave 1 Agent A: Active session detection and parsing

You are Wave 1 Agent A. Your task is to implement active (live) session detection
and in-memory parsing in the internal/claude package.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

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

echo "✓ Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

**If verification fails:** Write error to completion report and exit immediately.

## 1. File Ownership

You own these files. Do not touch any other files.
- `internal/claude/active.go` — CREATE
- `internal/claude/active_test.go` — CREATE

## 2. Interfaces You Must Implement

```go
// ActiveSessionInfo wraps a parsed live session with its source path.
type ActiveSessionInfo struct {
    SessionMeta
    Path   string
    IsLive bool
}

// FindActiveSessionPath scans ~/.claude/projects/**/*.jsonl for a file
// currently open by a Claude Code process.
// Returns ("", nil) if no active session is found.
// Returns ("", error) only on unexpected I/O failure.
func FindActiveSessionPath(claudeHome string) (string, error)

// ParseActiveSession reads the JSONL file at path as a partial transcript
// and reconstructs a *SessionMeta. Returns a non-nil *SessionMeta on
// success (even if partially populated). Returns (nil, error) only on
// file read failure.
func ParseActiveSession(path string) (*SessionMeta, error)
```

## 3. Interfaces You May Call

```go
// From internal/claude/transcripts.go — same package, call directly:
func ParseTimestamp(s string) time.Time

// From internal/claude/types.go — same package:
type SessionMeta struct { ... }   // reuse directly
type TranscriptEntry struct { ... } // reuse for JSONL parsing

// Standard library only. No new dependencies.
```

## 4. What to Implement

Read these files first to understand the patterns you are extending:
- `/Users/dayna.blackwell/code/claudewatch/internal/claude/transcripts.go`
  (lines 81–175): `ParseSingleTranscript` — this is the primary model for how
  JSONL is scanned. Your `ParseActiveSession` follows the same `bufio.Scanner`
  approach with the 10MB buffer.
- `/Users/dayna.blackwell/code/claudewatch/internal/claude/types.go`: the full
  `SessionMeta` struct — understand which fields you can populate.
- `/Users/dayna.blackwell/code/claudewatch/internal/claude/session_meta.go`:
  `ParseSessionMeta` — the closed-session equivalent for reference.

### `FindActiveSessionPath(claudeHome string) (string, error)`

1. Enumerate all `.jsonl` files under `filepath.Join(claudeHome, "projects")/**`.
   Walk one level deep: `projects/<hash>/<session>.jsonl`.
2. **Primary detection method (macOS/Linux):** run `lsof -F n` to get a list of
   open file paths, then check which (if any) of the enumerated JSONL files
   appear in the lsof output. Use `exec.Command("lsof", "-F", "n")` with a
   timeout context of 3 seconds. Parse stdout lines: lines starting with `n`
   carry a filename (e.g., `n/Users/dayna/.claude/projects/abc/session.jsonl`).
   Return the first match.
3. **Fallback (lsof unavailable or no match):** find the `.jsonl` file with the
   most recent mtime. If its mtime is within the last 5 minutes (`time.Since(fi.ModTime()) < 5*time.Minute`),
   treat it as active and return its path.
4. If no file qualifies, return `("", nil)`.
5. **Error handling:** `lsof` failure (non-zero exit, timeout, not found in PATH)
   is non-fatal — silently fall through to the mtime heuristic. Return a non-nil
   error only for `os.ReadDir` failures on the projects directory.

### `ParseActiveSession(path string) (*SessionMeta, error)`

1. Read the file with `os.ReadFile(path)`. Return `(nil, err)` on read failure.
2. **Line-atomic truncation:** find the last `'\n'` byte in the data using
   `bytes.LastIndexByte(data, '\n')`. If found, truncate `data = data[:lastNL+1]`.
   If not found (no complete lines yet), return `&SessionMeta{}` immediately
   with an empty but non-nil struct (not an error).
3. Scan lines with `bufio.NewScanner(bytes.NewReader(data))`, buffer up to 10MB
   (same as `ParseSingleTranscript`).
4. For each line, unmarshal into `TranscriptEntry`. Skip malformed lines silently.
5. Populate `SessionMeta` fields:
   - `SessionID`: from `entry.SessionID` on the first non-empty value seen, OR
     from `strings.TrimSuffix(filepath.Base(path), ".jsonl")` if no entry has it.
   - `ProjectPath`: set to the project hash directory name:
     `filepath.Base(filepath.Dir(path))`. (This is the hash, not the real path,
     but it's the best we have from the JSONL alone. Document this in comments.)
   - `StartTime`: `ParseTimestamp(entry.Timestamp)` on the first entry with a
     non-zero timestamp, formatted as RFC3339.
   - `UserMessageCount`: count entries with `entry.Type == "user"`.
   - `AssistantMessageCount`: count entries with `entry.Type == "assistant"`.
   - `InputTokens` and `OutputTokens`: for each assistant-type entry, attempt to
     unmarshal `entry.Message` into a minimal struct:
     ```go
     var msg struct {
         Usage struct {
             InputTokens  int `json:"input_tokens"`
             OutputTokens int `json:"output_tokens"`
         } `json:"usage"`
     }
     ```
     Accumulate into running totals.
6. Return `&meta, nil`. Do not return an error for partially-parseable files;
   return what you have.

### Internal-only type

Define this private struct inside `active.go` for usage extraction:

```go
type assistantMsgUsage struct {
    Usage struct {
        InputTokens  int `json:"input_tokens"`
        OutputTokens int `json:"output_tokens"`
    } `json:"usage"`
}
```

### `ActiveSessionInfo`

Define the exported type in `active.go`:

```go
type ActiveSessionInfo struct {
    SessionMeta
    Path   string
    IsLive bool
}
```

This type is not strictly required by the callers in this feature (they use
`*SessionMeta` directly), but it provides a useful wrapper for future callers.
Export it but do not force callers to use it — `FindActiveSessionPath` and
`ParseActiveSession` remain the primary API.

## 5. Tests to Write

All tests go in `internal/claude/active_test.go`. Use table-driven tests where
appropriate. Use `t.TempDir()` for all filesystem fixtures.

1. `TestFindActiveSessionPath_NoProjectsDir` — returns `("", nil)` when projects
   dir does not exist.
2. `TestFindActiveSessionPath_EmptyProjectsDir` — returns `("", nil)` when
   projects dir has no JSONL files.
3. `TestFindActiveSessionPath_MtimeFallback_RecentFile` — creates a JSONL file
   whose mtime is within 5 minutes (touch it right after creation); verifies the
   function returns its path.
4. `TestFindActiveSessionPath_MtimeFallback_OldFile` — creates a JSONL file with
   an mtime >5 minutes ago (use `os.Chtimes`); verifies `("", nil)` is returned.
5. `TestParseActiveSession_Empty` — empty file returns non-nil `*SessionMeta`
   with zero values, no error.
6. `TestParseActiveSession_NoTrailingNewline` — file with one complete JSON line
   but no trailing `\n`; verifies the line is still parsed.
7. `TestParseActiveSession_PartialLastLine` — file with one valid complete line
   followed by a partial (unterminated) JSON fragment; verifies only the complete
   line is parsed.
8. `TestParseActiveSession_SessionIDFromEntry` — JSONL with a `"sessionId"` field
   in the entry; verifies `meta.SessionID` is set from the entry.
9. `TestParseActiveSession_SessionIDFromFilename` — JSONL where entries have no
   `sessionId` field; verifies `meta.SessionID` comes from the filename.
10. `TestParseActiveSession_TokenAccumulation` — two assistant entries each with
    `usage.input_tokens` and `usage.output_tokens`; verifies totals are summed.
11. `TestParseActiveSession_MessageCounts` — mixed user/assistant entries; verifies
    `UserMessageCount` and `AssistantMessageCount`.
12. `TestParseActiveSession_ProjectPathIsHash` — verifies `meta.ProjectPath` equals
    the directory name of the parent dir of the JSONL file.
13. `TestParseActiveSession_ReadError` — passing a path that does not exist returns
    `(nil, err)`.
14. `TestActiveSessionInfo_EmbedsMeta` — construct an `ActiveSessionInfo` and
    verify `IsLive == true` and embedded `SessionMeta` fields are accessible.

## 6. Verification Gate

Run these commands from your worktree root. All must pass.

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
go build ./...
go vet ./...
go test ./internal/claude/... -count=1 -timeout 30s
```

## 7. Constraints

- Do NOT import anything outside the Go standard library. No new `go.mod` entries.
- `lsof` invocation must use a 3-second timeout context (`context.WithTimeout`).
  If `lsof` is not in PATH or times out, fall through silently to mtime heuristic.
- `ParseActiveSession` MUST NOT write to the DB. It is a pure read-and-parse
  function.
- `FindActiveSessionPath` MUST NOT write to the DB.
- The mtime fallback threshold is exactly 5 minutes (`5 * time.Minute`). No config.
- All errors from the projects-dir walk that are due to the directory not existing
  must return `(nil, nil)` — not an error.
- Do not modify `transcripts.go`, `types.go`, `session_meta.go`, or any existing
  file in `internal/claude/`.

## 8. Report

Commit your changes:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-a
git add internal/claude/active.go internal/claude/active_test.go
git commit -m "wave1-agent-a: add active session detection and parsing"
```

Append your completion report to
`/Users/dayna.blackwell/code/claudewatch/docs/IMPL-live-session-read.md`
under `### Agent A — Completion Report`. Use the structured format:

```yaml
### Agent A — Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-a
commit: {sha}
files_changed: []
files_created:
  - internal/claude/active.go
  - internal/claude/active_test.go
interface_deviations:
  - "exact description or []"
out_of_scope_deps:
  - "file: path, change: what, reason: why" or []
tests_added:
  - TestFindActiveSessionPath_NoProjectsDir
  - (etc.)
verification: PASS | FAIL ({command} — N/N tests)
```
```

---

```
# Wave 2 Agent B: MCP get_session_stats live-session enhancement

You are Wave 2 Agent B. Your task is to enhance handleGetSessionStats in
internal/mcp/tools.go to check for an active (live) session first, and return
live session data when found.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-b 2>/dev/null || true
```

**Step 2: Verify isolation (strict fail-fast after self-correction attempt)**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-b"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory (even after cd attempt)"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave2-agent-b"

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
- `internal/mcp/tools.go` — MODIFY
- `internal/mcp/tools_test.go` — MODIFY

## 2. Interfaces You Must Implement

Modify `SessionStatsResult` to add a `Live` field:

```go
type SessionStatsResult struct {
    SessionID     string  `json:"session_id"`
    ProjectName   string  `json:"project_name"`
    StartTime     string  `json:"start_time"`
    DurationMin   int     `json:"duration_minutes"`
    InputTokens   int     `json:"input_tokens"`
    OutputTokens  int     `json:"output_tokens"`
    EstimatedCost float64 `json:"estimated_cost_usd"`
    Live          bool    `json:"live"`   // true when data is from an active session
}
```

Modify `handleGetSessionStats` to check for an active session first:

```go
func (s *Server) handleGetSessionStats(args json.RawMessage) (any, error)
```

The new behavior: try `claude.FindActiveSessionPath(s.claudeHome)` first; if a
path is returned, parse it with `claude.ParseActiveSession(path)` and return that
as the result with `Live: true`. Only if no active session is found should it
fall through to the existing logic (parse all closed sessions, pick most recent,
return with `Live: false`).

## 3. Interfaces You May Call

These are delivered by Wave 1 Agent A. They will be present in your worktree
after the Wave 1 merge.

```go
// From internal/claude/active.go:
func claude.FindActiveSessionPath(claudeHome string) (string, error)
func claude.ParseActiveSession(path string) (*claude.SessionMeta, error)
```

These are existing functions you already call or reference:

```go
// From internal/claude/session_meta.go:
func claude.ParseAllSessionMeta(claudeHome string) ([]claude.SessionMeta, error)

// From internal/analyzer/outcomes.go:
func analyzer.EstimateSessionCost(s claude.SessionMeta, pricing analyzer.ModelPricing, ratio analyzer.CacheRatio) float64

// From internal/analyzer/cost.go:
var analyzer.DefaultPricing map[string]analyzer.ModelPricing
func analyzer.NoCacheRatio() analyzer.CacheRatio

// Existing method on *Server:
func (s *Server) loadCacheRatio() analyzer.CacheRatio
```

## 4. What to Implement

Read these files first:
- `/Users/dayna.blackwell/code/claudewatch/internal/mcp/tools.go` lines 165–194:
  the existing `handleGetSessionStats`. Understand exactly what it does today.
- `/Users/dayna.blackwell/code/claudewatch/internal/mcp/tools_test.go`: existing
  tests. Your new tests must not break existing ones.

### Modified `handleGetSessionStats` logic

```
1. Call claude.FindActiveSessionPath(s.claudeHome).
   - If error: log/ignore it, fall through to closed-session path.
   - If path != "": proceed to step 2.
   - If path == "": skip to step 4 (closed-session path).

2. Call claude.ParseActiveSession(path).
   - If error: ignore, fall through to closed-session path.
   - If meta != nil: proceed to step 3.

3. Build and return SessionStatsResult from the live session meta:
   - SessionID: meta.SessionID
   - ProjectName: filepath.Base(meta.ProjectPath)
   - StartTime: meta.StartTime
   - DurationMin: meta.DurationMinutes (will be 0 for live sessions — expected)
   - InputTokens: meta.InputTokens
   - OutputTokens: meta.OutputTokens
   - EstimatedCost: analyzer.EstimateSessionCost(*meta, pricing, ratio)
   - Live: true
   Return immediately. Do NOT write to the DB.

4. (Closed-session fallback — existing logic, unchanged):
   Parse all session meta from disk, sort by StartTime descending,
   take the most recent. Return with Live: false.
```

The existing `errors.New("no sessions found")` return still applies when step 4
finds zero closed sessions AND step 1 found no active session.

### No DB interaction

The live path MUST NOT call any function in `internal/store/`. This is a hard
constraint. The existing closed-session path also does not touch the DB — there
is nothing to change there.

## 5. Tests to Write

Add these tests to `internal/mcp/tools_test.go`. The existing test helpers
(`writeSessionMeta`, `newTestServer`, `callTool`) are already present; use them.

You will need a new helper to write an active JSONL file:

```go
// writeActiveJSONL writes a minimal JSONL transcript to
// <claudeHome>/projects/<hash>/<sessionID>.jsonl
// with a recent mtime, simulating an in-progress session.
func writeActiveJSONL(t *testing.T, claudeHome, hash, sessionID string, inputTokens, outputTokens int, projectPath string) string
```

New tests:

1. `TestGetSessionStats_LiveSession_TakesPrecedence` — when both an active JSONL
   file and a closed session meta file exist, the result should be the live
   session (`Live: true`).
2. `TestGetSessionStats_LiveSession_Fields` — verify that `SessionID`,
   `ProjectName`, `InputTokens`, `OutputTokens`, `EstimatedCost`, and
   `Live: true` are correctly populated from the active JSONL.
3. `TestGetSessionStats_NoActiveFallsBackToClosed` — when no active session exists
   (empty projects dir), returns the closed session with `Live: false`.
4. `TestGetSessionStats_LiveField_False_WhenClosed` — verifies the `Live` field
   is `false` for the normal closed-session path (existing test
   `TestGetSessionStats_SingleSession` implicitly covers this, but add an explicit
   assertion).

Note: Because `FindActiveSessionPath` uses the mtime heuristic in the absence of
lsof, your test JSONL files must have a recent mtime. Creating a file sets its
mtime to now, so this is automatic in tests (no `os.Chtimes` needed).

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-b
go build ./...
go vet ./...
go test ./internal/mcp/... -count=1 -timeout 30s
```

Also run the full suite to catch any regressions:

```bash
go test ./... -count=1 -timeout 60s
```

## 7. Constraints

- The `Live bool` field on `SessionStatsResult` is additive. Existing callers
  that deserialize the JSON will simply see a new field; this is backward-
  compatible.
- Do NOT modify `handleGetRecentSessions`, `handleGetCostBudget`, or any other
  handler. Only `handleGetSessionStats` changes.
- Do NOT modify `SessionStatsResult`'s JSON tags for existing fields.
- The live path MUST NOT import or call anything from `internal/store/`.
- If `claude.FindActiveSessionPath` or `claude.ParseActiveSession` return errors,
  treat them as non-fatal and fall through to the closed-session path silently.
  Do not surface these errors to the MCP caller.
- Do not import `os/exec` or call `lsof` directly in this file — that is owned
  by `internal/claude/active.go`.

## 8. Report

Commit your changes:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-b
git add internal/mcp/tools.go internal/mcp/tools_test.go
git commit -m "wave2-agent-b: enhance get_session_stats with live session support"
```

Append your completion report to
`/Users/dayna.blackwell/code/claudewatch/docs/IMPL-live-session-read.md`
under `### Agent B — Completion Report`. Use the structured format:

```yaml
### Agent B — Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave2-agent-b
commit: {sha}
files_changed:
  - internal/mcp/tools.go
  - internal/mcp/tools_test.go
files_created: []
interface_deviations:
  - "exact description or []"
out_of_scope_deps:
  - "file: path, change: what, reason: why" or []
tests_added:
  - TestGetSessionStats_LiveSession_TakesPrecedence
  - (etc.)
verification: PASS | FAIL ({command} — N/N tests)
```
```

---

```
# Wave 2 Agent C: claudewatch scan --include-active flag

You are Wave 2 Agent C. Your task is to add an --include-active flag to the
claudewatch scan command that shows any active (live) Claude Code session in
the scan output, tagged as (live), without writing anything to the database.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c 2>/dev/null || true
```

**Step 2: Verify isolation (strict fail-fast after self-correction attempt)**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory (even after cd attempt)"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave2-agent-c"

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
- `internal/app/scan.go` — MODIFY

## 2. Interfaces You Must Implement

Add an `--include-active` boolean flag to `scanCmd`. When set, the scan output
includes an additional row at the top of the table for the active session (if any),
with `(live)` appended to the project name. The flag is off by default.

New package-level flag variable:

```go
var scanFlagIncludeActive bool
```

New row in `renderScanTable` output when `--include-active` is set and an active
session exists:
- Score: `"  ---"` (not applicable for live sessions)
- Project: `"<project-name-or-hash> (live)"` using `output.StyleBold` rendering
- CLAUDE.md: `"---"` (unknown for live sessions)
- Sessions: `"--"` (not applicable)
- Last Active: `"now"`

## 3. Interfaces You May Call

These are delivered by Wave 1 Agent A. They will be present in your worktree
after the Wave 1 merge.

```go
// From internal/claude/active.go:
func claude.FindActiveSessionPath(claudeHome string) (string, error)
func claude.ParseActiveSession(path string) (*claude.SessionMeta, error)
```

Existing functions in scope:

```go
// From internal/claude/paths.go (same package used via import):
func claude.NormalizePath(path string) string

// From internal/output/ — already imported:
output.StyleBold     // lipgloss style
output.StyleMuted    // lipgloss style
output.StyleSuccess  // lipgloss style
output.StyleError    // lipgloss style
```

## 4. What to Implement

Read these files first:
- `/Users/dayna.blackwell/code/claudewatch/internal/app/scan.go`: the complete
  existing implementation. Understand the flag pattern in `init()`, the
  `runScan` function structure, `renderScanTable`, and `renderScanSummary`.

### Flag registration in `init()`

Add to the existing `init()` func in `scan.go`:

```go
scanCmd.Flags().BoolVar(&scanFlagIncludeActive, "include-active", false,
    "Include any currently active (live) Claude Code session in scan output")
```

### `runScan` changes

At the start of `runScan`, after loading cfg and before discovering projects,
optionally query for an active session:

```go
var activeMeta *claude.SessionMeta
if scanFlagIncludeActive {
    activePath, err := claude.FindActiveSessionPath(cfg.ClaudeHome)
    if err == nil && activePath != "" {
        activeMeta, _ = claude.ParseActiveSession(activePath)
        // Ignore error — display nothing for the live row if parsing fails.
    }
}
```

Pass `activeMeta` through to `renderScanTable`. If `activeMeta` is nil (no active
session or flag not set), the table is rendered exactly as before.

### `renderScanTable` signature change

Change the signature from:

```go
func renderScanTable(results []scanResult)
```

to:

```go
func renderScanTable(results []scanResult, activeMeta *claude.SessionMeta)
```

Inside `renderScanTable`, if `activeMeta != nil`, prepend a special row before
the normal results loop:

```go
if activeMeta != nil {
    projectDisplay := output.StyleBold.Render(filepath.Base(activeMeta.ProjectPath) + " (live)")
    tbl.AddRow("  ---", projectDisplay, output.StyleMuted.Render("---"), "--", output.StyleBold.Render("now"))
}
```

Update the call in `runScan` to pass `activeMeta`.

### `renderScanSummary` — no changes needed

The summary (mean score, project count, etc.) is computed from `results` only.
The live session is intentionally excluded from summary statistics.

### JSON output

When `--json` or `--include-active` flags are combined: the live session is NOT
included in JSON output. JSON output renders only closed-session results (the
existing `renderScanJSON` is unchanged). Document this as a current limitation
in a code comment. This keeps the JSON schema stable.

## 5. Tests to Write

There are currently no tests for `runScan` or `renderScanTable` in `scan.go`
(the file has no corresponding `_test.go`). Create `internal/app/scan_test.go`
to add minimal coverage for the new flag behavior.

1. `TestRenderScanTable_WithActiveMeta` — construct a small `[]scanResult`,
   call `renderScanTable` with a non-nil `activeMeta`, and verify no panic occurs
   and that the function returns. (Output capture via `os.Pipe` or a mock is
   optional; the primary goal is no panic/crash coverage.)
2. `TestRenderScanTable_NilActiveMeta` — call with `activeMeta == nil`; verify
   the function still works correctly (no panic).
3. `TestFlagRegistration_IncludeActive` — verify that `scanCmd.Flags().Lookup("include-active")`
   is non-nil, confirming the flag is registered.

Note: Full integration tests for `runScan` would require a more elaborate setup.
The three lightweight tests above are sufficient for this PR. Mark them as
candidates for future expansion in a comment.

## 6. Verification Gate

Before running verification, check whether existing tests reference `renderScanTable`
with the old single-argument signature:

```bash
grep -r "renderScanTable" /Users/dayna.blackwell/code/claudewatch
```

If any test file references `renderScanTable`, update those calls to pass a
second `nil` argument.

Run:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c
go build ./...
go vet ./...
go test ./internal/app/... -count=1 -timeout 30s
go test ./... -count=1 -timeout 60s
```

## 7. Constraints

- The `--include-active` flag is opt-in. Default behavior of `claudewatch scan`
  is unchanged.
- The live row MUST NOT appear in `--json` output. JSON schema stability is
  more important than completeness here.
- `FindActiveSessionPath` and `ParseActiveSession` errors are non-fatal and
  silently ignored — the scan runs normally without the live row.
- Do NOT import anything from `internal/store/`. The live path is memory-only.
- Do NOT modify `renderScanSummary`. Live sessions are excluded from statistics.
- `renderScanTable` signature change from 1 arg to 2 args: if the grep above
  finds existing callers in test files, update those calls. If only called from
  `runScan`, only `runScan` needs updating.
- Do not modify any file outside `internal/app/scan.go` and the new test file.

## 8. Report

Commit your changes:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c
git add internal/app/scan.go internal/app/scan_test.go
git commit -m "wave2-agent-c: add --include-active flag to scan command"
```

Append your completion report to
`/Users/dayna.blackwell/code/claudewatch/docs/IMPL-live-session-read.md`
under `### Agent C — Completion Report`. Use the structured format:

```yaml
### Agent C — Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave2-agent-c
commit: {sha}
files_changed:
  - internal/app/scan.go
files_created:
  - internal/app/scan_test.go
interface_deviations:
  - "exact description or []"
out_of_scope_deps:
  - "file: path, change: what, reason: why" or []
tests_added:
  - TestRenderScanTable_WithActiveMeta
  - TestRenderScanTable_NilActiveMeta
  - TestFlagRegistration_IncludeActive
verification: PASS | FAIL ({command} — N/N tests)
```
```

---

### Wave Execution Loop

#### Pre-wave setup (orchestrator)

```bash
cd /Users/dayna.blackwell/code/claudewatch

# Create Wave 1 worktree
git worktree add .claude/worktrees/wave1-agent-a -b wave1-agent-a

# Create Wave 2 worktrees (pre-create so agents can self-verify isolation)
git worktree add .claude/worktrees/wave2-agent-b -b wave2-agent-b
git worktree add .claude/worktrees/wave2-agent-c -b wave2-agent-c
```

#### Wave 1 execution

Launch Agent A. Wait for completion report.

#### Wave 1 merge

```bash
cd /Users/dayna.blackwell/code/claudewatch
git merge wave1-agent-a --no-ff -m "merge wave1-agent-a: active session detection"

# Rebase Wave 2 branches onto the merged main so they have active.go available
git checkout wave2-agent-b && git rebase main
git checkout wave2-agent-c && git rebase main
git checkout main
```

#### Wave 2 execution

Launch Agents B and C in parallel. Wait for both completion reports.

#### Wave 2 merge

```bash
cd /Users/dayna.blackwell/code/claudewatch
git merge wave2-agent-b --no-ff -m "merge wave2-agent-b: live session in get_session_stats"
git merge wave2-agent-c --no-ff -m "merge wave2-agent-c: scan --include-active flag"
```

Resolve any conflicts (unlikely — B owns `mcp/tools.go`, C owns `app/scan.go`).

#### Post-merge verification

```bash
cd /Users/dayna.blackwell/code/claudewatch

# Full build
go build ./...

# Vet
go vet ./...

# Full test suite
go test ./... -count=1 -timeout 120s

# Spot-check the MCP tool description is unchanged
grep -A3 "get_session_stats" internal/mcp/tools.go

# Verify --include-active flag is registered
./bin/claudewatch scan --help | grep include-active

# Verify no store imports in active.go
grep "internal/store" internal/claude/active.go && echo "FAIL: store import found" || echo "PASS: no store import"

# Verify no store imports in the live paths of mcp/tools.go
# (manual review: confirm handleGetSessionStats new branch does not call store)

# Verify no DB writes from the live path (no import of store package in affected files)
for f in internal/claude/active.go internal/mcp/tools.go internal/app/scan.go; do
  grep "store\." "$f" && echo "WARN: $f may touch store" || true
done
```

#### Cleanup

```bash
git worktree remove .claude/worktrees/wave1-agent-a
git worktree remove .claude/worktrees/wave2-agent-b
git worktree remove .claude/worktrees/wave2-agent-c
git branch -d wave1-agent-a wave2-agent-b wave2-agent-c
```

---

### Status

- [ ] Agent A — Wave 1: active session detection and parsing
- [ ] Agent B — Wave 2: MCP get_session_stats live enhancement
- [ ] Agent C — Wave 2: scan --include-active flag
- [ ] Post-merge verification: go test ./... passes
- [ ] Post-merge verification: no store imports in live path
- [ ] Post-merge verification: --include-active flag registered in scan --help

---

<!-- Completion reports appended below by agents -->

### Agent A — Completion Report
```yaml
status: complete
worktree: main (solo wave)
commit: 8b1e5e7fae8bdea659b6266ade5c7063380ef218
files_changed: []
files_created:
  - internal/claude/active.go
  - internal/claude/active_test.go
interface_deviations: []
out_of_scope_deps: []
tests_added:
  - TestFindActiveSessionPath_NoProjectsDir
  - TestFindActiveSessionPath_EmptyProjectsDir
  - TestFindActiveSessionPath_MtimeFallback_RecentFile
  - TestFindActiveSessionPath_MtimeFallback_OldFile
  - TestParseActiveSession_Empty
  - TestParseActiveSession_NoTrailingNewline
  - TestParseActiveSession_PartialLastLine
  - TestParseActiveSession_SessionIDFromEntry
  - TestParseActiveSession_SessionIDFromFilename
  - TestParseActiveSession_TokenAccumulation
  - TestParseActiveSession_MessageCounts
  - TestParseActiveSession_ProjectPathIsHash
  - TestParseActiveSession_ReadError
  - TestActiveSessionInfo_EmbedsMeta
verification: PASS (go test ./internal/claude/... -count=1 -timeout 30s — 14/14 new tests, 111/111 total)
```
