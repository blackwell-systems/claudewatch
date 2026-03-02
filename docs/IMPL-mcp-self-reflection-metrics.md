# IMPL: MCP Self-Reflection Metrics

**Feature:** New MCP tools providing real-time behavioral feedback for AI
self-correction during active sessions: token velocity, live tool error rate,
context window utilization, commit-to-attempt ratio, and live friction event stream.

---

### Suitability Assessment

**1. File decomposition.** The five metrics decompose into 3 agents with disjoint
file ownership. Each agent creates a new `*_tools.go` file under `internal/mcp/`
and a corresponding `*_tools_test.go`. Two agents also create new analysis
functions under `internal/claude/` (live JSONL parsing). The shared file
`internal/mcp/tools.go` is orchestrator-owned (append-only `registerTool` calls).

**2. Investigation-first items.** Context window utilization (item 3) requires
investigation. The JSONL transcript data does not contain context window usage
information. The `stats-cache.json` has `ContextWindow` and `MaxOutputTokens`
per model, but no per-message fill level. This cannot be computed from available
data without the API response's `usage.cache_creation_input_tokens` /
`usage.cache_read_input_tokens` fields per-turn, which are not present in the
JSONL. **Decision: DROP item 3 (context window utilization) from this IMPL.**
It would require upstream changes to Claude Code's JSONL format.

**3. Interface discoverability.** All cross-agent interfaces are fully specifiable.
The live JSONL parsing functions extend the existing `ParseActiveSession` pattern
in `internal/claude/active.go`. MCP handler signatures follow the established
`func (s *Server) handle*(args json.RawMessage) (any, error)` pattern. No
upstream agent output is needed.

**4. Pre-implementation status check.**

| Item | Status | Notes |
|------|--------|-------|
| Token velocity | TO-DO | `SessionStatsResult` has tokens/cost but no velocity |
| Live tool error rate | TO-DO | `get_project_health` has `avg_tool_errors_per_session` for closed sessions only |
| Context window utilization | DROPPED | Data not available from JSONL |
| Commit-to-attempt ratio | TO-DO | `SessionMeta` has `GitCommits` and `ToolCounts` but no live version |
| Live friction event stream | TO-DO | `get_session_friction` reads facets (post-hoc); no live JSONL parsing |

**5. Parallelization value check.**

```
Estimated times:
- Scout phase: ~20 min (reading ~20 files across 4 packages)
- Wave 1: ~30 min (3 parallel agents x ~30 min avg; parallel time = max)
- Merge & verification: ~10 min
Total SAW time: ~60 min

Sequential baseline: ~100 min (3 agents x ~30 min + overhead)
Time savings: ~40 min (40% faster)

Recommendation: Clear speedup. 3 fully independent agents, each creating 2-3
new files. Proceed.
```

**Verdict: SUITABLE** (4 items, after dropping context window utilization)

---

### Known Issues

- `ParseActiveSession` reads the entire JSONL file into memory. For very long
  sessions this could be slow. The new live parsers should follow the same
  approach for consistency but could be optimized later with tail-based reads.
- The `FindActiveSessionPath` lsof-based detection has a 3-second timeout.
  All new live tools reuse this function, so they inherit that latency.

---

### Dependency Graph

```
internal/claude/active.go        -- existing, leaf (READ ONLY by all agents)
internal/claude/types.go         -- existing, leaf (READ ONLY)
internal/claude/transcripts.go   -- existing, leaf (READ ONLY)
internal/analyzer/cost.go        -- existing, leaf (READ ONLY)

internal/claude/active_live.go       (Agent A) -- new: live JSONL parsing helpers
internal/claude/active_live_test.go  (Agent A) -- new: tests

internal/mcp/velocity_tools.go      (Agent B) -- new: token velocity + commit-to-attempt
internal/mcp/velocity_tools_test.go  (Agent B) -- new: tests

internal/mcp/live_tools.go          (Agent C) -- new: live tool error rate + live friction
internal/mcp/live_tools_test.go     (Agent C) -- new: tests

internal/mcp/tools.go               (Orchestrator) -- append-only registrations
```

All agents import existing packages only. No new packages created.

**Cascade candidates:** None. All changes are additive new files. No existing
types are renamed or removed.

---

### Interface Contracts

#### Agent A -> Agent B, Agent C: Live parsing helpers

Agent A creates `internal/claude/active_live.go` exposing these functions that
Agents B and C call:

```go
package claude

// LiveToolErrors parses the active JSONL at path and returns tool error
// counts from tool_result entries where IsError is true.
type LiveToolErrorStats struct {
    TotalToolUses   int            `json:"total_tool_uses"`
    TotalErrors     int            `json:"total_errors"`
    ErrorRate       float64        `json:"error_rate"`
    ErrorsByTool    map[string]int `json:"errors_by_tool"`
    ConsecutiveErrs int            `json:"consecutive_errors"` // current streak
}

func ParseLiveToolErrors(path string) (*LiveToolErrorStats, error)

// LiveFrictionIndicators parses the active JSONL at path and detects
// friction patterns: user rejections (short user messages after tool errors),
// retries (same tool_use name repeated within 3 entries), error bursts.
type LiveFrictionEvent struct {
    Type      string `json:"type"`       // "tool_error", "retry", "rejection", "error_burst"
    Tool      string `json:"tool,omitempty"`
    Count     int    `json:"count"`
    Timestamp string `json:"timestamp"`
}

type LiveFrictionStats struct {
    Events        []LiveFrictionEvent `json:"events"`
    TotalFriction int                 `json:"total_friction"`
}

func ParseLiveFriction(path string) (*LiveFrictionStats, error)

// LiveCommitAttemptStats parses the active JSONL for Edit/Write tool uses
// and git commit indicators to compute a commit-to-attempt ratio.
type LiveCommitAttemptStats struct {
    EditWriteAttempts int     `json:"edit_write_attempts"`
    GitCommits        int     `json:"git_commits"`
    Ratio             float64 `json:"ratio"` // commits / attempts, 0 if no attempts
}

func ParseLiveCommitAttempts(path string) (*LiveCommitAttemptStats, error)
```

#### Orchestrator post-merge additions to `internal/mcp/tools.go`

Add to `addTools(s *Server)` after existing registrations:

```go
// Self-reflection metrics (post-merge registration)
addVelocityTools(s)
addLiveTools(s)
```

#### Agent B: Velocity tools registration

Agent B creates `addVelocityTools(s *Server)` in `velocity_tools.go`:

```go
func addVelocityTools(s *Server)
```

Registers two tools:
- `get_token_velocity` -- tokens per wall-clock minute for current live session
- `get_commit_attempt_ratio` -- live commit-to-attempt ratio

#### Agent C: Live tools registration

Agent C creates `addLiveTools(s *Server)` in `live_tools.go`:

```go
func addLiveTools(s *Server)
```

Registers two tools:
- `get_live_tool_errors` -- live tool error count and rate for current session
- `get_live_friction` -- live friction event stream for current session

---

### MCP Tool Specifications

#### `get_token_velocity` (Agent B)

```json
{
  "name": "get_token_velocity",
  "description": "Token throughput rate for the current live session: tokens per minute, elapsed minutes, and whether velocity indicates productive flow or stuck/idle state.",
  "inputSchema": {"type":"object","properties":{},"additionalProperties":false}
}
```

Response type:
```go
type TokenVelocityResult struct {
    SessionID        string  `json:"session_id"`
    Live             bool    `json:"live"`
    ElapsedMinutes   float64 `json:"elapsed_minutes"`
    TotalTokens      int     `json:"total_tokens"`
    TokensPerMinute  float64 `json:"tokens_per_minute"`
    OutputPerMinute  float64 `json:"output_tokens_per_minute"`
    Status           string  `json:"status"` // "flowing", "slow", "idle"
}
```

Status thresholds: `>= 5000 tokens/min` = "flowing", `>= 1000` = "slow", `< 1000` = "idle".

#### `get_commit_attempt_ratio` (Agent B)

```json
{
  "name": "get_commit_attempt_ratio",
  "description": "Ratio of successful git commits to code change attempts (Edit/Write tool uses) in the current live session. Low ratio signals guessing rather than understanding.",
  "inputSchema": {"type":"object","properties":{},"additionalProperties":false}
}
```

Response type:
```go
type CommitAttemptResult struct {
    SessionID         string  `json:"session_id"`
    Live              bool    `json:"live"`
    EditWriteAttempts int     `json:"edit_write_attempts"`
    GitCommits        int     `json:"git_commits"`
    Ratio             float64 `json:"ratio"`
    Assessment        string  `json:"assessment"` // "efficient", "normal", "low"
}
```

Assessment thresholds: ratio `>= 0.3` = "efficient", `>= 0.1` = "normal", `< 0.1` = "low".

#### `get_live_tool_errors` (Agent C)

```json
{
  "name": "get_live_tool_errors",
  "description": "Live tool error count and rate for the current session. Detects degradation patterns like consecutive failed edits.",
  "inputSchema": {"type":"object","properties":{},"additionalProperties":false}
}
```

Response type:
```go
type LiveToolErrorResult struct {
    SessionID       string         `json:"session_id"`
    Live            bool           `json:"live"`
    TotalToolUses   int            `json:"total_tool_uses"`
    TotalErrors     int            `json:"total_errors"`
    ErrorRate       float64        `json:"error_rate"`
    ErrorsByTool    map[string]int `json:"errors_by_tool"`
    ConsecutiveErrs int            `json:"consecutive_errors"`
    Severity        string         `json:"severity"` // "clean", "mild", "degraded"
}
```

Severity thresholds: `consecutive >= 4 || error_rate > 0.3` = "degraded",
`error_rate > 0.1` = "mild", else "clean".

#### `get_live_friction` (Agent C)

```json
{
  "name": "get_live_friction",
  "description": "Live friction indicators for the current session parsed from the active transcript. Detects tool errors, retries, error bursts, and rejection patterns in real time.",
  "inputSchema": {"type":"object","properties":{},"additionalProperties":false}
}
```

Response type:
```go
type LiveFrictionResult struct {
    SessionID     string               `json:"session_id"`
    Live          bool                 `json:"live"`
    Events        []LiveFrictionEvent  `json:"events"`
    TotalFriction int                  `json:"total_friction"`
    TopType       string               `json:"top_type,omitempty"`
}
```

Uses `claude.LiveFrictionEvent` from Agent A.

---

### File Ownership Table

| File | Owner | Action |
|------|-------|--------|
| `internal/claude/active_live.go` | Agent A | CREATE |
| `internal/claude/active_live_test.go` | Agent A | CREATE |
| `internal/mcp/velocity_tools.go` | Agent B | CREATE |
| `internal/mcp/velocity_tools_test.go` | Agent B | CREATE |
| `internal/mcp/live_tools.go` | Agent C | CREATE |
| `internal/mcp/live_tools_test.go` | Agent C | CREATE |
| `internal/mcp/tools.go` | Orchestrator | MODIFY (append-only) |

---

### Wave Structure

```
Wave 1:  Agent A (live parsing helpers)
Wave 2:  Agent B (velocity tools) + Agent C (live tools)   [parallel]
Post:    Orchestrator merges tools.go registrations
```

Agent A must complete first because Agents B and C import the functions it creates.
Agents B and C are fully independent of each other.

---

### Agent Prompts

#### Agent A: Live JSONL Parsing Helpers

```
FIELD 0 — ISOLATION VERIFICATION
Before writing any code, run:
  git worktree list
Confirm you are working in a worktree, NOT in /Users/dayna.blackwell/code/claudewatch directly.
If you ARE in the main repo, STOP and report.

FIELD 1 — FILE OWNERSHIP
You may ONLY create or modify these files:
  internal/claude/active_live.go       (CREATE)
  internal/claude/active_live_test.go  (CREATE)
Do NOT modify any other file. Do NOT modify active.go, types.go, or transcripts.go.

FIELD 2 — INTERFACES TO IMPLEMENT
Create internal/claude/active_live.go in package claude with these exported functions:

  func ParseLiveToolErrors(path string) (*LiveToolErrorStats, error)
  func ParseLiveFriction(path string) (*LiveFrictionStats, error)
  func ParseLiveCommitAttempts(path string) (*LiveCommitAttemptStats, error)

And these exported types:

  type LiveToolErrorStats struct {
      TotalToolUses   int            `json:"total_tool_uses"`
      TotalErrors     int            `json:"total_errors"`
      ErrorRate       float64        `json:"error_rate"`
      ErrorsByTool    map[string]int `json:"errors_by_tool"`
      ConsecutiveErrs int            `json:"consecutive_errors"`
  }

  type LiveFrictionEvent struct {
      Type      string `json:"type"`
      Tool      string `json:"tool,omitempty"`
      Count     int    `json:"count"`
      Timestamp string `json:"timestamp"`
  }

  type LiveFrictionStats struct {
      Events        []LiveFrictionEvent `json:"events"`
      TotalFriction int                 `json:"total_friction"`
  }

  type LiveCommitAttemptStats struct {
      EditWriteAttempts int     `json:"edit_write_attempts"`
      GitCommits        int     `json:"git_commits"`
      Ratio             float64 `json:"ratio"`
  }

FIELD 3 — INTERFACES TO CALL
Read existing code for patterns:
  - ParseActiveSession in internal/claude/active.go: follow its JSONL reading
    pattern (os.ReadFile, line-atomic truncation, bufio.Scanner with 10MB buffer,
    json.Unmarshal into TranscriptEntry).
  - TranscriptEntry, ContentBlock, AssistantMessage, UserMessage from
    internal/claude/transcripts.go: use these types for parsing.
  - ParseTimestamp from internal/claude/transcripts.go for timestamp parsing.

FIELD 4 — WHAT TO IMPLEMENT
ParseLiveToolErrors:
  - Read JSONL file at path using the same pattern as ParseActiveSession.
  - For each "assistant" entry, parse AssistantMessage and count tool_use blocks.
  - For each "user" entry, parse UserMessage and check tool_result blocks for
    IsError == true. Track which tool name the error belongs to by correlating
    tool_use_id with the preceding assistant tool_use blocks.
  - Track consecutive errors: reset counter on any non-error tool_result, increment
    on error.
  - Compute ErrorRate = TotalErrors / TotalToolUses (0 if no tool uses).

ParseLiveFriction:
  - Read JSONL the same way.
  - Detect friction patterns:
    * "tool_error": any tool_result with IsError == true
    * "retry": same tool name used 2+ times within the last 3 tool_use entries
    * "error_burst": 3+ tool errors within any 5 consecutive tool_results
  - Return events in chronological order.

ParseLiveCommitAttempts:
  - Read JSONL the same way.
  - Count tool_use blocks where Name is "Edit" or "Write" -> EditWriteAttempts.
  - Count tool_use blocks where Name is "Bash" and the input contains "git commit"
    (simple substring match on the serialized input) -> GitCommits.
  - Compute Ratio = GitCommits / EditWriteAttempts (0 if no attempts).

FIELD 5 — TESTS TO WRITE
Write internal/claude/active_live_test.go with:
  - TestParseLiveToolErrors_NoErrors: JSONL with 3 tool uses, 0 errors -> rate 0
  - TestParseLiveToolErrors_WithErrors: JSONL with errors -> correct counts, rate
  - TestParseLiveToolErrors_ConsecutiveErrors: verify streak tracking
  - TestParseLiveToolErrors_EmptyFile: empty file -> zero stats, no error
  - TestParseLiveFriction_NoFriction: clean session -> empty events
  - TestParseLiveFriction_ToolError: session with tool errors -> events detected
  - TestParseLiveFriction_Retry: same tool used 2+ times -> retry detected
  - TestParseLiveCommitAttempts_NoEdits: no Edit/Write -> 0 attempts, ratio 0
  - TestParseLiveCommitAttempts_WithCommits: Edit+Write+commit -> correct ratio

Use t.TempDir() and write JSONL test files inline (same pattern as
active_test.go and tools_test.go helpers).

FIELD 6 — VERIFICATION GATE
Run:
  go build ./internal/claude/...
  go vet ./internal/claude/...
  go test ./internal/claude/ -run "TestParseLive" -v
All must pass with zero errors.

FIELD 7 — CONSTRAINTS
  - Do NOT create new packages.
  - Do NOT modify existing files.
  - Initialize all map fields (never return nil maps).
  - Follow the existing JSONL scanning pattern: 10MB scanner buffer,
    skip malformed lines silently, line-atomic truncation.
  - Keep function signatures exactly as specified.

FIELD 8 — REPORT
When done, report:
  - Functions implemented and their line counts
  - Test count and pass/fail status
  - Output of go vet
  - Any deviations from spec
```

#### Agent B: Velocity Tools (Token Velocity + Commit-to-Attempt Ratio)

```
FIELD 0 — ISOLATION VERIFICATION
Before writing any code, run:
  git worktree list
Confirm you are working in a worktree, NOT in /Users/dayna.blackwell/code/claudewatch directly.
If you ARE in the main repo, STOP and report.

FIELD 1 — FILE OWNERSHIP
You may ONLY create or modify these files:
  internal/mcp/velocity_tools.go       (CREATE)
  internal/mcp/velocity_tools_test.go  (CREATE)
Do NOT modify tools.go, active.go, or any other file.

FIELD 2 — INTERFACES TO IMPLEMENT
Create internal/mcp/velocity_tools.go in package mcp with:

  func addVelocityTools(s *Server)

This function registers two tools on s:
  - "get_token_velocity"
  - "get_commit_attempt_ratio"

Response types:

  type TokenVelocityResult struct {
      SessionID       string  `json:"session_id"`
      Live            bool    `json:"live"`
      ElapsedMinutes  float64 `json:"elapsed_minutes"`
      TotalTokens     int     `json:"total_tokens"`
      TokensPerMinute float64 `json:"tokens_per_minute"`
      OutputPerMinute float64 `json:"output_tokens_per_minute"`
      Status          string  `json:"status"`
  }

  type CommitAttemptResult struct {
      SessionID         string  `json:"session_id"`
      Live              bool    `json:"live"`
      EditWriteAttempts int     `json:"edit_write_attempts"`
      GitCommits        int     `json:"git_commits"`
      Ratio             float64 `json:"ratio"`
      Assessment        string  `json:"assessment"`
  }

FIELD 3 — INTERFACES TO CALL
  - claude.FindActiveSessionPath(s.claudeHome) -> (string, error)
  - claude.ParseActiveSession(path) -> (*SessionMeta, error)
    Returns SessionMeta with: SessionID, InputTokens, OutputTokens,
    StartTime, UserMessageTimestamps, ProjectPath.
  - claude.ParseTimestamp(s) -> time.Time
  - claude.ParseLiveCommitAttempts(path) -> (*LiveCommitAttemptStats, error)
    (from Agent A) Returns: EditWriteAttempts, GitCommits, Ratio.
  - s.loadTags() -> map[string]string
  - resolveProjectName(sessionID, projectPath, tags) -> string

FIELD 4 — WHAT TO IMPLEMENT
handleGetTokenVelocity:
  - Find active session via FindActiveSessionPath + ParseActiveSession.
  - If no active session: return error "no active session found".
  - Compute elapsed time: time.Since(ParseTimestamp(meta.StartTime)).Minutes().
  - TotalTokens = meta.InputTokens + meta.OutputTokens.
  - TokensPerMinute = TotalTokens / elapsed (guard against elapsed == 0).
  - OutputPerMinute = meta.OutputTokens / elapsed.
  - Status: >= 5000 tokens/min -> "flowing", >= 1000 -> "slow", < 1000 -> "idle".
  - Return TokenVelocityResult with Live: true.

handleGetCommitAttemptRatio:
  - Find active session via FindActiveSessionPath.
  - If no active session: return error "no active session found".
  - Call ParseActiveSession for session ID.
  - Call claude.ParseLiveCommitAttempts(activePath) for the ratio data.
  - Assessment: ratio >= 0.3 -> "efficient", >= 0.1 -> "normal", < 0.1 -> "low".
    Special case: if EditWriteAttempts == 0, assessment = "no_changes".
  - Return CommitAttemptResult with Live: true.

Tool schemas: both use noArgsSchema (already defined in tools.go).

FIELD 5 — TESTS TO WRITE
Write internal/mcp/velocity_tools_test.go with:
  - TestGetTokenVelocity_NoActiveSession: no JSONL -> error
  - TestGetTokenVelocity_ActiveSession: write JSONL with known tokens and
    timestamp -> verify TotalTokens, Live=true, Status is one of the 3 values
  - TestGetTokenVelocity_Status: verify flowing/slow/idle thresholds
  - TestGetCommitAttemptRatio_NoActiveSession: no JSONL -> error
  - TestGetCommitAttemptRatio_WithData: write JSONL with Edit/Write tool_use
    blocks and a Bash git commit -> verify ratio and assessment

Use the existing test helpers: newTestServer, writeActiveJSONL (from
tools_test.go). For commit-attempt tests, write a custom JSONL with
assistant tool_use blocks containing Edit/Write/Bash names.

FIELD 6 — VERIFICATION GATE
Run:
  go build ./internal/mcp/...
  go vet ./internal/mcp/...
  go test ./internal/mcp/ -run "TestGetTokenVelocity|TestGetCommitAttempt" -v
All must pass with zero errors.

FIELD 7 — CONSTRAINTS
  - Do NOT modify tools.go or any other file. The orchestrator will add the
    addVelocityTools(s) call to tools.go post-merge.
  - Do NOT create new packages.
  - Reference noArgsSchema by name (it is package-level in tools.go).
  - Follow existing handler patterns (see handleGetSessionStats in tools.go).
  - Return errors for no-active-session rather than zero-value results.

FIELD 8 — REPORT
When done, report:
  - Tools registered and their handler names
  - Test count and pass/fail status
  - Output of go vet
  - Any deviations from spec
```

#### Agent C: Live Tools (Tool Error Rate + Friction Stream)

```
FIELD 0 — ISOLATION VERIFICATION
Before writing any code, run:
  git worktree list
Confirm you are working in a worktree, NOT in /Users/dayna.blackwell/code/claudewatch directly.
If you ARE in the main repo, STOP and report.

FIELD 1 — FILE OWNERSHIP
You may ONLY create or modify these files:
  internal/mcp/live_tools.go       (CREATE)
  internal/mcp/live_tools_test.go  (CREATE)
Do NOT modify tools.go, friction_tools.go, or any other file.

FIELD 2 — INTERFACES TO IMPLEMENT
Create internal/mcp/live_tools.go in package mcp with:

  func addLiveTools(s *Server)

This function registers two tools on s:
  - "get_live_tool_errors"
  - "get_live_friction"

Response types:

  type LiveToolErrorResult struct {
      SessionID       string         `json:"session_id"`
      Live            bool           `json:"live"`
      TotalToolUses   int            `json:"total_tool_uses"`
      TotalErrors     int            `json:"total_errors"`
      ErrorRate       float64        `json:"error_rate"`
      ErrorsByTool    map[string]int `json:"errors_by_tool"`
      ConsecutiveErrs int            `json:"consecutive_errors"`
      Severity        string         `json:"severity"`
  }

  type LiveFrictionResult struct {
      SessionID     string                    `json:"session_id"`
      Live          bool                      `json:"live"`
      Events        []claude.LiveFrictionEvent `json:"events"`
      TotalFriction int                       `json:"total_friction"`
      TopType       string                    `json:"top_type,omitempty"`
  }

FIELD 3 — INTERFACES TO CALL
  - claude.FindActiveSessionPath(s.claudeHome) -> (string, error)
  - claude.ParseActiveSession(path) -> (*SessionMeta, error)
  - claude.ParseLiveToolErrors(path) -> (*LiveToolErrorStats, error)
    (from Agent A) Returns: TotalToolUses, TotalErrors, ErrorRate,
    ErrorsByTool, ConsecutiveErrs.
  - claude.ParseLiveFriction(path) -> (*LiveFrictionStats, error)
    (from Agent A) Returns: Events ([]LiveFrictionEvent), TotalFriction.
  - s.loadTags() -> map[string]string
  - resolveProjectName(sessionID, projectPath, tags) -> string

FIELD 4 — WHAT TO IMPLEMENT
handleGetLiveToolErrors:
  - Find active session via FindActiveSessionPath.
  - If no active session: return error "no active session found".
  - Parse session ID via ParseActiveSession.
  - Call claude.ParseLiveToolErrors(activePath).
  - Compute Severity:
    * consecutive >= 4 OR error_rate > 0.3 -> "degraded"
    * error_rate > 0.1 -> "mild"
    * else -> "clean"
  - Return LiveToolErrorResult with Live: true, all stats from the parsed data.

handleGetLiveFriction:
  - Find active session via FindActiveSessionPath.
  - If no active session: return error "no active session found".
  - Parse session ID via ParseActiveSession.
  - Call claude.ParseLiveFriction(activePath).
  - Compute TopType: the type with the highest count among events. Ties broken
    alphabetically (same pattern as friction_tools.go).
  - Return LiveFrictionResult with Live: true.

Tool schemas: both use noArgsSchema.

FIELD 5 — TESTS TO WRITE
Write internal/mcp/live_tools_test.go with:
  - TestGetLiveToolErrors_NoActiveSession: no JSONL -> error
  - TestGetLiveToolErrors_Clean: JSONL with tool uses, no errors -> severity "clean"
  - TestGetLiveToolErrors_Degraded: JSONL with high error rate -> severity "degraded"
  - TestGetLiveToolErrors_ConsecutiveErrors: 4+ consecutive errors -> "degraded"
  - TestGetLiveFriction_NoActiveSession: no JSONL -> error
  - TestGetLiveFriction_NoFriction: clean session -> empty events, total 0
  - TestGetLiveFriction_WithErrors: session with tool errors -> events present

Write JSONL test data inline with assistant tool_use blocks and user tool_result
blocks (with and without is_error: true). Use t.TempDir() and the writeActiveJSONL
helper pattern.

FIELD 6 — VERIFICATION GATE
Run:
  go build ./internal/mcp/...
  go vet ./internal/mcp/...
  go test ./internal/mcp/ -run "TestGetLiveToolErrors|TestGetLiveFriction" -v
All must pass with zero errors.

FIELD 7 — CONSTRAINTS
  - Do NOT modify tools.go, friction_tools.go, or any other file.
  - Do NOT create new packages.
  - Reference noArgsSchema by name (package-level in tools.go).
  - Initialize all map fields (ErrorsByTool must never be nil).
  - Follow existing handler patterns from tools.go and friction_tools.go.

FIELD 8 — REPORT
When done, report:
  - Tools registered and their handler names
  - Test count and pass/fail status
  - Output of go vet
  - Any deviations from spec
```

---

### Orchestrator Post-Merge Checklist

After all agents complete, the orchestrator applies these changes:

1. **Add to `internal/mcp/tools.go`**, inside `addTools(s *Server)`, after
   the existing `addCostTools(s)` call:

```go
addVelocityTools(s)
addLiveTools(s)
```

2. **Run full verification:**

```bash
go build ./...
go vet ./...
go test ./... -count=1
```

3. **Verify tool count:** The `tools/list` response should include 4 new tools:
   `get_token_velocity`, `get_commit_attempt_ratio`, `get_live_tool_errors`,
   `get_live_friction`.

---

### Status Checklist

| Step | Status |
|------|--------|
| Wave 1: Agent A (live parsing helpers) | DONE |
| Wave 2: Agent B (velocity tools) | TODO |
| Wave 2: Agent C (live tools) | TODO |
| Orchestrator: tools.go registration | TODO |
| Full build + test verification | TODO |

---

### Agent A — Completion Report
```yaml
status: complete
commit: "uncommitted — solo agent on main"
files_created:
  - internal/claude/active_live.go
  - internal/claude/active_live_test.go
interface_deviations: []
out_of_scope_deps: []
tests_added:
  - TestParseLiveToolErrors_NoErrors
  - TestParseLiveToolErrors_WithErrors
  - TestParseLiveToolErrors_ConsecutiveErrors
  - TestParseLiveToolErrors_EmptyFile
  - TestParseLiveFriction_NoFriction
  - TestParseLiveFriction_ToolError
  - TestParseLiveFriction_Retry
  - TestParseLiveCommitAttempts_NoEdits
  - TestParseLiveCommitAttempts_WithCommits
verification: PASS (go build, go vet, go test — 9/9 tests pass, 0 errors)
```
