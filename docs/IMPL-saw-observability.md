# IMPL: SAW Observability

**Feature:** Structured `[SAW:wave{N}:agent-{X}]` task description tags in `saw-skill.md` + tag
parsing, wave grouping, and MCP tools in `claudewatch`.

**Repos involved:**
- `~/code/scout-and-wave` — tag format in the skill prompt
- `~/code/claudewatch` — parsing, grouping, and MCP exposure

---

### Suitability Assessment

Verdict: SUITABLE

Work decomposes cleanly into 3 agents with disjoint file ownership across 2 waves. Agent A
(saw-skill.md) is in a completely different git repository from Agents B and C (claudewatch),
so worktree isolation is not required between them — cross-repo work cannot conflict at the
git level. Within claudewatch, Agent B and Agent C own disjoint files (claude/saw.go vs
mcp/tools.go) with a clean dependency: Agent C consumes the interface Agent B delivers.
All cross-agent interfaces can be fully specified upfront. No investigation-first blockers.
All items are TO-DO.

The primary value of SAW here is the interface contract between the tag format (Agent A) and
the parser (Agent B). Without SAW, tag format and parser would be designed independently
and might diverge. The IMPL doc pins them to the same spec.

```
Estimated times:
- Scout phase: ~10 min
- Wave 1: ~25 min (2 parallel: Agent A=~5 min, Agent B=~25 min; parallel time = max)
- Wave 2: ~20 min (1 agent)
- Merge & verification: ~5 min
Total SAW time: ~60 min

Sequential baseline: ~65 min (5 + 25 + 20 + 15 overhead)
Time savings: ~5 min (marginal on time, clear win on interface correctness)

Recommendation: Coordination value exceeds speed gains. Proceed.
```

---

### Known Issues

None identified. Go build and tests pass cleanly on main.

---

### Dependency Graph

```
Agent A (scout-and-wave/prompts/saw-skill.md)
  — standalone, no dependencies on other agents

Agent B (claudewatch/internal/claude/saw.go)
  — no dependencies on other agents
  — delivers: ParseSAWTag, SAWAgentRun, SAWWave, SAWSession, ComputeSAWWaves
  — root node for Wave 2

Agent C (claudewatch/internal/mcp/tools.go)
  — depends on Agent B (imports claude.ComputeSAWWaves, claude.SAWSession)
  — leaf node
```

Cascade candidates (not in any agent's scope — post-merge verification will catch):
- `claudewatch/internal/mcp/tools_test.go` — existing tests will still pass; Agent C must
  add new tests in a separate file to avoid touching this file outside its declared scope.

---

### Interface Contracts

#### Tag Format (Agent A → Agent B)

The `description` parameter in Task tool calls during SAW wave execution must be prefixed:

```
[SAW:wave{N}:agent-{X}] {short description}
```

Examples:
- `[SAW:wave1:agent-A] add ParseSAWTag to claudewatch`
- `[SAW:wave2:agent-C] add get_saw_sessions MCP tool`

Rules:
- `{N}` is the wave number (integer, 1-indexed)
- `{X}` is the agent letter (uppercase, e.g., `A`, `B`, `C`)
- The bracket prefix is followed by a space, then the original description
- The tag must appear at the very start of the description string

#### claude package — Agent B must deliver

```go
// ParseSAWTag parses a SAW coordination tag from a task description.
// Expected format: "[SAW:wave{N}:agent-{X}] rest of description"
// Returns wave number (≥1), agent letter (e.g., "A"), and true if a valid tag is found.
// Returns 0, "", false if no SAW tag is present or the tag is malformed.
func ParseSAWTag(description string) (wave int, agent string, ok bool)

// SAWAgentRun represents a single agent's execution within a SAW wave.
type SAWAgentRun struct {
    Agent       string    // e.g., "A", "B", "C"
    Description string    // full description from the Task call (includes tag)
    Status      string    // "completed", "failed", "killed"
    DurationMs  int64
    LaunchedAt  time.Time
    CompletedAt time.Time
}

// SAWWave represents one wave of parallel SAW agents within a session.
type SAWWave struct {
    Wave       int           // wave number from the tag
    Agents     []SAWAgentRun // sorted by agent letter
    StartedAt  time.Time     // earliest LaunchedAt across all agents in this wave
    EndedAt    time.Time     // latest CompletedAt across all agents in this wave
    DurationMs int64         // EndedAt - StartedAt wall-clock milliseconds
}

// SAWSession represents a complete SAW execution within one Claude Code session.
type SAWSession struct {
    SessionID   string     // from AgentSpan.SessionID
    ProjectHash string     // from AgentSpan.ProjectHash
    Waves       []SAWWave  // sorted by wave number
    TotalAgents int        // sum of agents across all waves
}

// ComputeSAWWaves scans spans for SAW-tagged agents and groups them into SAWSession
// structures. Spans without a valid ParseSAWTag result are ignored.
// Sessions are returned in no guaranteed order; waves within each session are
// sorted by wave number; agents within each wave are sorted by agent letter.
// Returns an empty slice (not nil) if no SAW-tagged spans are found.
func ComputeSAWWaves(spans []AgentSpan) []SAWSession
```

#### mcp package — Agent C must register

```go
// get_saw_sessions: list recent sessions that include SAW-tagged agents.
// Input schema: {"n": int (optional, default 5, max 50)}
// Output:
type SAWSessionsResult struct {
    Sessions []SAWSessionSummary `json:"sessions"`
}
type SAWSessionSummary struct {
    SessionID   string `json:"session_id"`
    ProjectName string `json:"project_name"`  // filepath.Base of ProjectPath, or ProjectHash if unknown
    StartTime   string `json:"start_time"`    // RFC3339 from earliest LaunchedAt in the session
    WaveCount   int    `json:"wave_count"`
    AgentCount  int    `json:"total_agents"`
}

// get_saw_wave_breakdown: per-wave timing for a specific session.
// Input schema: {"session_id": string (required)}
// Output:
type SAWWaveBreakdownResult struct {
    SessionID string           `json:"session_id"`
    Waves     []SAWWaveDetail  `json:"waves"`
}
type SAWWaveDetail struct {
    Wave       int              `json:"wave"`
    AgentCount int              `json:"agent_count"`
    DurationMs int64            `json:"duration_ms"`
    StartedAt  string           `json:"started_at"`   // RFC3339
    EndedAt    string           `json:"ended_at"`     // RFC3339
    Agents     []SAWAgentDetail `json:"agents"`
}
type SAWAgentDetail struct {
    Agent      string `json:"agent"`
    Status     string `json:"status"`
    DurationMs int64  `json:"duration_ms"`
}
```

---

### File Ownership

| File | Agent | Wave | Depends On |
|------|-------|------|------------|
| `~/code/scout-and-wave/prompts/saw-skill.md` | A | 1 | — |
| `~/code/claudewatch/internal/claude/saw.go` | B | 1 | — |
| `~/code/claudewatch/internal/claude/saw_test.go` | B | 1 | — |
| `~/code/claudewatch/internal/mcp/tools.go` | C | 2 | Agent B (claude.ComputeSAWWaves) |
| `~/code/claudewatch/internal/mcp/saw_tools_test.go` | C | 2 | Agent B (claude.SAWSession) |

Orchestrator-owned (append-only after merge):
- `go.mod`, `go.sum` — no changes expected (same module, no new deps)

---

### Wave Structure

```
Wave 1: [A] [B]      — 2 parallel agents (A in scout-and-wave, B in claudewatch)
             | (B completes — delivers ParseSAWTag + ComputeSAWWaves)
Wave 2:   [C]        — 1 agent (claudewatch MCP tools, depends on B)
```

**Cross-repo note:** Agent A runs directly on main in `~/code/scout-and-wave` — no worktree
needed since it cannot conflict with Agent B (different git repo). Agent B runs in a claudewatch
worktree. Agent C runs in a claudewatch worktree in Wave 2.

**Pre-launch ownership verification:** No overlaps. Agent A touches only saw-skill.md.
Agents B and C touch disjoint files in different packages.

---

### Agent Prompts

---

## Wave 1 Agent A: Add SAW tag format to saw-skill.md

You are Wave 1 Agent A. Your task is to add a one-paragraph instruction to `saw-skill.md`
specifying the structured `[SAW:wave{N}:agent-{X}]` format for Task description parameters
during wave execution.

### 0. CRITICAL: Isolation Verification (RUN FIRST)

Agent A works directly on main in the `scout-and-wave` repo (a separate git repo from
claudewatch). No worktree is required. Verify you are in the correct repo:

```bash
cd ~/code/scout-and-wave
ACTUAL_REPO=$(git remote get-url origin 2>/dev/null || git rev-parse --show-toplevel)
echo "Repo: $ACTUAL_REPO"
# Should contain "scout-and-wave"
```

If the repo is wrong, stop and report. Do not modify files in the wrong repo.

### 1. File Ownership

- `~/code/scout-and-wave/prompts/saw-skill.md` — modify

Do not touch any other files.

### 2. Interfaces You Must Implement

Tag format (the string contract Agent B's parser will consume):

```
[SAW:wave{N}:agent-{X}] {short description}
```

- `{N}` = wave number, 1-indexed integer
- `{X}` = agent letter, uppercase (A, B, C, ...)
- Bracket prefix at the very start of the description string, followed by a single space
- Example: `[SAW:wave1:agent-A] implement ParseSAWTag`

### 3. Interfaces You May Call

None.

### 4. What to Implement

Read `saw-skill.md`. In the "If a `docs/IMPL-*.md` file already exists" section, step 3
currently reads:

> For each agent in the current wave, launch a parallel Task agent using the agent prompt
> from the IMPL doc. Use `isolation: "worktree"` for each agent. Disjoint file ownership
> (enforced by the IMPL doc) is the primary safety mechanism.

Extend this step to require the structured SAW tag on the `description` parameter. The
new text should:

1. State that the `description` parameter must be prefixed with `[SAW:wave{N}:agent-{X}]`
2. Give a concrete example (e.g., `[SAW:wave1:agent-A] implement cache layer`)
3. Explain why: enables claudewatch to parse wave timing and agent breakdown from
   session transcripts automatically — zero overhead, structured observability

Keep the existing content about `isolation: "worktree"` and disjoint file ownership unchanged.
This is an additive instruction only.

Also bump the version comment at the top of the file: `<!-- saw-skill v0.3.0 -->` →
`<!-- saw-skill v0.3.1 -->`.

### 5. Tests to Write

No code tests. After editing, verify the tag format example in the prompt is correct:
- `[SAW:wave1:agent-A]` — valid (wave 1, agent A)
- `[SAW:wave2:agent-B]` — valid (wave 2, agent B)

Read the edited file and confirm the instruction is in the right place and reads naturally.

### 6. Verification Gate

```bash
cd ~/code/scout-and-wave
# Verify the file parses correctly and the version header was bumped
head -3 prompts/saw-skill.md
# Should show: <!-- saw-skill v0.3.1 -->

# Confirm the tag format string appears in the file
grep -n 'SAW:wave' prompts/saw-skill.md
# Should show the tag format in context
```

### 7. Constraints

- Additive change only. Do not restructure the existing instructions.
- Keep the tag format exactly as specified: `[SAW:wave{N}:agent-{X}]` (square brackets, colon
  separators, lowercase "wave" and "agent" prefixes, uppercase letter).
- The version bump is required for drift detection (users who symlink saw-skill.md get live
  updates; users who copy it can detect staleness by comparing version headers).

### 8. Report

Commit your changes in the scout-and-wave repo:

```bash
cd ~/code/scout-and-wave
git add prompts/saw-skill.md
git commit -m "wave1-agent-a: add SAW tag format instruction to saw-skill.md (v0.3.1)"
```

Append your completion report to `~/code/claudewatch/docs/IMPL-saw-observability.md`
under `### Agent A — Completion Report`.

```yaml
status: complete | partial | blocked
worktree: main (scout-and-wave repo, no worktree)
commit: {sha}
files_changed:
  - prompts/saw-skill.md
files_created: []
interface_deviations:
  - "exact tag format if any deviation from [SAW:wave{N}:agent-{X}], or []"
out_of_scope_deps: []
tests_added: []
verification: PASS | FAIL
```

---

## Wave 1 Agent B: ParseSAWTag and ComputeSAWWaves in claudewatch

You are Wave 1 Agent B. Your task is to create `internal/claude/saw.go` with the SAW tag
parser, SAW data types, and wave grouping function.

### 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
# Attempt self-correction
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b 2>/dev/null || true

ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave1-agent-b"
if [ "$ACTUAL_BRANCH" != "$EXPECTED_BRANCH" ]; then
  echo "ISOLATION FAILURE: Wrong branch"
  exit 1
fi

git worktree list | grep -q "$EXPECTED_BRANCH" || {
  echo "ISOLATION FAILURE: Worktree not in git worktree list"
  exit 1
}

echo "✓ Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

If verification fails: write error to completion report and exit immediately.

### 1. File Ownership

- `internal/claude/saw.go` — create
- `internal/claude/saw_test.go` — create

Do not touch any other files.

### 2. Interfaces You Must Implement

```go
// ParseSAWTag parses a SAW coordination tag from a task description.
// Expected format: "[SAW:wave{N}:agent-{X}] rest of description"
// Returns wave number (≥1), agent letter (e.g., "A"), and true if valid.
// Returns 0, "", false if no SAW tag is present or the format is malformed.
func ParseSAWTag(description string) (wave int, agent string, ok bool)

type SAWAgentRun struct {
    Agent       string
    Description string
    Status      string    // "completed", "failed", "killed"
    DurationMs  int64
    LaunchedAt  time.Time
    CompletedAt time.Time
}

type SAWWave struct {
    Wave       int
    Agents     []SAWAgentRun  // sorted by Agent letter
    StartedAt  time.Time      // earliest LaunchedAt
    EndedAt    time.Time      // latest CompletedAt
    DurationMs int64          // EndedAt - StartedAt in milliseconds
}

type SAWSession struct {
    SessionID   string
    ProjectHash string
    Waves       []SAWWave  // sorted by Wave number
    TotalAgents int
}

// ComputeSAWWaves groups SAW-tagged spans into SAWSession structures.
// Spans without a valid SAW tag are ignored.
// Returns empty slice (not nil) if no SAW-tagged spans found.
func ComputeSAWWaves(spans []AgentSpan) []SAWSession
```

### 3. Interfaces You May Call

```go
// AgentSpan — existing type in package claude (transcripts.go)
type AgentSpan struct {
    SessionID   string
    ProjectHash string
    AgentType   string
    Description string
    Prompt      string
    Background  bool
    LaunchedAt  time.Time
    CompletedAt time.Time
    Duration    time.Duration
    Killed      bool
    Success     bool
    ResultLength int
    ToolUseID   string
}
```

### 4. What to Implement

Read `internal/claude/transcripts.go` to understand `AgentSpan` and the existing package
conventions before writing anything.

**ParseSAWTag:**
- The tag format is `[SAW:wave{N}:agent-{X}]` at the very start of the description
- `N` is a positive integer; `X` is one or more uppercase letters (usually A–Z)
- Use `strings.HasPrefix` + manual parsing or `regexp` — keep it simple
- Return `(0, "", false)` for any malformed or missing tag (wrong prefix, non-integer wave,
  empty agent, etc.)
- Do not trim the description; match only from position 0

**ComputeSAWWaves:**
- Group by `(SessionID, wave number)` to build waves
- Group sessions by `SessionID`
- Status mapping: `span.Killed → "killed"`, `!span.Success → "failed"`, otherwise `"completed"`
- Sort waves by `Wave` number within each session
- Sort agents by `Agent` string within each wave
- Compute `StartedAt` = min(agent LaunchedAt), `EndedAt` = max(agent CompletedAt),
  `DurationMs` = `EndedAt.Sub(StartedAt).Milliseconds()`
- `SAWSession.TotalAgents` = sum of len(wave.Agents) across all waves

### 5. Tests to Write

In `saw_test.go`:

1. `TestParseSAWTag_Valid` — parse `"[SAW:wave1:agent-A] description"`, verify wave=1, agent="A"
2. `TestParseSAWTag_ValidMultiLetter` — parse `"[SAW:wave3:agent-AB] desc"`, verify wave=3, agent="AB"
3. `TestParseSAWTag_NoTag` — empty string returns false
4. `TestParseSAWTag_MalformedMissingBracket` — `"SAW:wave1:agent-A desc"` returns false
5. `TestParseSAWTag_MalformedBadWave` — `"[SAW:waveX:agent-A] desc"` returns false
6. `TestParseSAWTag_MalformedEmptyAgent` — `"[SAW:wave1:agent-] desc"` returns false
7. `TestComputeSAWWaves_Empty` — empty spans returns empty slice (not nil)
8. `TestComputeSAWWaves_SingleWave` — 2 tagged spans in same session/wave; verify wave grouping,
   StartedAt/EndedAt, DurationMs, TotalAgents
9. `TestComputeSAWWaves_MultiWave` — 4 tagged spans across 2 waves in one session; verify
   wave ordering and agent sorting
10. `TestComputeSAWWaves_SkipsUntagged` — mix of tagged and untagged spans; untagged ignored
11. `TestComputeSAWWaves_MultiSession` — spans across 2 sessions; returns 2 SAWSessions

### 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b
go build ./...
go vet ./...
go test ./internal/claude/... -run TestParseSAWTag -v
go test ./internal/claude/... -run TestComputeSAWWaves -v
```

All tests must pass.

### 7. Constraints

- Package is `package claude` — same package as `transcripts.go` and `agents.go`
- Use only stdlib (no new dependencies)
- `ParseSAWTag` must be pure (no side effects, no global state)
- `ComputeSAWWaves` must handle nil or empty span slice gracefully (return `[]SAWSession{}`)
- Do not modify any existing files in the `claude` package

### 8. Report

Commit your work:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-b
git add internal/claude/saw.go internal/claude/saw_test.go
git commit -m "wave1-agent-b: add ParseSAWTag and ComputeSAWWaves to claude package"
```

Append to `~/code/claudewatch/docs/IMPL-saw-observability.md` under `### Agent B — Completion Report`:

```yaml
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-b
commit: {sha}
files_changed: []
files_created:
  - internal/claude/saw.go
  - internal/claude/saw_test.go
interface_deviations:
  - "any deviation from the specified signatures, or []"
out_of_scope_deps: []
tests_added:
  - TestParseSAWTag_Valid
  - TestParseSAWTag_ValidMultiLetter
  - TestParseSAWTag_NoTag
  - TestParseSAWTag_MalformedMissingBracket
  - TestParseSAWTag_MalformedBadWave
  - TestParseSAWTag_MalformedEmptyAgent
  - TestComputeSAWWaves_Empty
  - TestComputeSAWWaves_SingleWave
  - TestComputeSAWWaves_MultiWave
  - TestComputeSAWWaves_SkipsUntagged
  - TestComputeSAWWaves_MultiSession
verification: PASS | FAIL
```

---

## Wave 2 Agent C: SAW MCP tools in claudewatch

You are Wave 2 Agent C. Your task is to add `get_saw_sessions` and `get_saw_wave_breakdown`
MCP tools to `internal/mcp/tools.go`, consuming Agent B's `claude.ComputeSAWWaves`.

### 0. CRITICAL: Isolation Verification (RUN FIRST)

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c 2>/dev/null || true

ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c"
if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
if [ "$ACTUAL_BRANCH" != "wave2-agent-c" ]; then
  echo "ISOLATION FAILURE: Wrong branch"
  exit 1
fi

git worktree list | grep -q "wave2-agent-c" || {
  echo "ISOLATION FAILURE: Worktree not in git worktree list"
  exit 1
}

echo "✓ Isolation verified: $ACTUAL_DIR on $ACTUAL_BRANCH"
```

### 1. File Ownership

- `internal/mcp/tools.go` — modify (add result types + two new handlers + register)
- `internal/mcp/saw_tools_test.go` — create (new test file for SAW tools only)

Do not touch `internal/mcp/tools_test.go` (existing file owned by no agent — out of scope).

### 2. Interfaces You Must Implement

```go
// Result types (add to tools.go):

type SAWSessionsResult struct {
    Sessions []SAWSessionSummary `json:"sessions"`
}

type SAWSessionSummary struct {
    SessionID   string `json:"session_id"`
    ProjectName string `json:"project_name"` // filepath.Base of ProjectPath, or ProjectHash
    StartTime   string `json:"start_time"`   // RFC3339 of earliest agent LaunchedAt
    WaveCount   int    `json:"wave_count"`
    AgentCount  int    `json:"total_agents"`
}

type SAWWaveBreakdownResult struct {
    SessionID string          `json:"session_id"`
    Waves     []SAWWaveDetail `json:"waves"`
}

type SAWWaveDetail struct {
    Wave       int              `json:"wave"`
    AgentCount int              `json:"agent_count"`
    DurationMs int64            `json:"duration_ms"`
    StartedAt  string           `json:"started_at"` // RFC3339
    EndedAt    string           `json:"ended_at"`   // RFC3339
    Agents     []SAWAgentDetail `json:"agents"`
}

type SAWAgentDetail struct {
    Agent      string `json:"agent"`
    Status     string `json:"status"`
    DurationMs int64  `json:"duration_ms"`
}

// Handlers (add to tools.go):
func (s *Server) handleGetSAWSessions(args json.RawMessage) (any, error)
func (s *Server) handleGetSAWWaveBreakdown(args json.RawMessage) (any, error)
```

### 3. Interfaces You May Call

```go
// From internal/claude package — Agent B delivers these:
func claude.ComputeSAWWaves(spans []claude.AgentSpan) []claude.SAWSession
type claude.SAWSession struct {
    SessionID   string
    ProjectHash string
    Waves       []claude.SAWWave
    TotalAgents int
}
type claude.SAWWave struct {
    Wave       int
    Agents     []claude.SAWAgentRun
    StartedAt  time.Time
    EndedAt    time.Time
    DurationMs int64
}
type claude.SAWAgentRun struct {
    Agent       string
    Status      string
    DurationMs  int64
    LaunchedAt  time.Time
    CompletedAt time.Time
}

// Existing functions already in mcp package (available to call):
func claude.ParseSessionTranscripts(claudeDir string) ([]claude.AgentSpan, error)
func claude.ParseAllSessionMeta(claudeHome string) ([]claude.SessionMeta, error)
// SessionMeta has fields: SessionID, ProjectPath, StartTime, DurationMinutes, InputTokens, OutputTokens
```

### 4. What to Implement

Read `internal/mcp/tools.go` and `internal/mcp/jsonrpc.go` before writing anything to
understand the existing registration pattern and handler signature conventions.

**handleGetSAWSessions:**
1. Parse optional `n` from args (default 5, max 50) — same pattern as `handleGetRecentSessions`
2. Call `claude.ParseSessionTranscripts(s.claudeHome)` to get all spans
3. Call `claude.ComputeSAWWaves(spans)` to get SAW sessions
4. Call `claude.ParseAllSessionMeta(s.claudeHome)` to build a `map[sessionID]ProjectPath`
5. For each SAWSession, derive `ProjectName` = `filepath.Base(projectPath)` using the map;
   fall back to `session.ProjectHash` if the session ID is not in the meta map
6. Derive `StartTime` = RFC3339 of `session.Waves[0].StartedAt` (earliest wave); use empty
   string if no waves
7. Sort descending by StartTime string (lexicographic on RFC3339 is correct)
8. Take first `n` SAWSessions
9. Return `SAWSessionsResult`

**handleGetSAWWaveBreakdown:**
1. Parse required `session_id` string from args; return error if missing
2. Call `claude.ParseSessionTranscripts(s.claudeHome)` → `claude.ComputeSAWWaves(spans)`
3. Find the matching `SAWSession` by `SessionID`; return error "session not found" if absent
4. Map each `SAWWave` to `SAWWaveDetail` including all agents
5. Return `SAWWaveBreakdownResult`

**Register both tools in `addTools`:**

```go
s.registerTool(toolDef{
    Name:        "get_saw_sessions",
    Description: "Recent Claude Code sessions that used Scout-and-Wave parallel agents, with wave count and agent count.",
    InputSchema: recentNSchema,  // reuse existing schema (n: int, optional)
    Handler:     s.handleGetSAWSessions,
})
s.registerTool(toolDef{
    Name:        "get_saw_wave_breakdown",
    Description: "Per-wave timing and agent status breakdown for a SAW session.",
    InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string","description":"Session ID from get_saw_sessions"}},"required":["session_id"],"additionalProperties":false}`),
    Handler:     s.handleGetSAWWaveBreakdown,
})
```

### 5. Tests to Write

In `saw_tools_test.go` (new file, `package mcp`):

1. `TestGetSAWSessions_Empty` — no SAW spans → empty sessions list, no error
2. `TestGetSAWSessions_ReturnsSAWOnly` — mix of SAW-tagged and untagged spans; only SAW sessions returned
3. `TestGetSAWSessions_LimitsN` — n=1 with 3 SAW sessions; returns 1
4. `TestGetSAWWaveBreakdown_Found` — session with 2 waves, 2 agents each; verify wave count,
   agent count, DurationMs non-zero
5. `TestGetSAWWaveBreakdown_NotFound` — unknown session_id → error response

Use the existing JSONL-writing helper pattern from `tools_test.go` to create test fixtures.
Write minimal JSONL transcript files with SAW-tagged task descriptions.

### 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c
go build ./...
go vet ./...
go test ./internal/mcp/... -run TestGetSAW -v
```

All tests must pass.

### 7. Constraints

- Add result types and handlers to `tools.go` — do not create new files in the mcp package
  (except the test file)
- Reuse `recentNSchema` for `get_saw_sessions` (already defined in tools.go)
- The new `session_id` schema is defined inline in `addTools` — do not add a package-level var
- Handle empty `Waves` slice gracefully in `handleGetSAWSessions` (StartTime = "")
- Return errors as Go errors (the server wraps them as MCP error responses)
- Do not import any new external packages — use only stdlib and the existing internal packages

### 8. Report

Commit your work:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-c
git add internal/mcp/tools.go internal/mcp/saw_tools_test.go
git commit -m "wave2-agent-c: add get_saw_sessions and get_saw_wave_breakdown MCP tools"
```

Append to `~/code/claudewatch/docs/IMPL-saw-observability.md` under `### Agent C — Completion Report`:

```yaml
status: complete | partial | blocked
worktree: .claude/worktrees/wave2-agent-c
commit: {sha}
files_changed:
  - internal/mcp/tools.go
files_created:
  - internal/mcp/saw_tools_test.go
interface_deviations:
  - "any deviation from specified result types or handler signatures, or []"
out_of_scope_deps: []
tests_added:
  - TestGetSAWSessions_Empty
  - TestGetSAWSessions_ReturnsSAWOnly
  - TestGetSAWSessions_LimitsN
  - TestGetSAWWaveBreakdown_Found
  - TestGetSAWWaveBreakdown_NotFound
verification: PASS | FAIL
```

---

### Wave Execution Loop

After each wave completes:

1. Read each agent's completion report from their named section in this IMPL doc.
   Check for interface deviations and out-of-scope dependencies.
2. Cross-reference `files_changed` and `files_created` lists for overlaps.
3. Merge all agent worktrees back into main. Agent A committed to the `scout-and-wave` repo
   directly — no merge needed for that repo. Agents B and C are merged into claudewatch main.
4. Run the full verification gate:
   ```bash
   cd ~/code/claudewatch
   go build ./...
   go vet ./...
   go test ./...
   ```
5. Fix any cross-package integration failures before launching Wave 2.
6. Update the coordination artifact: tick status checkboxes, correct interface contracts
   if any deviations were logged.
7. Launch Wave 2 (Agent C).

---

### Status

- [x] Wave 1 Agent A — add SAW tag format instruction to saw-skill.md
- [x] Wave 1 Agent B — add ParseSAWTag + ComputeSAWWaves to claudewatch/internal/claude
- [x] Wave 2 Agent C — add get_saw_sessions + get_saw_wave_breakdown MCP tools

### Agent A — Completion Report

```yaml
status: complete
worktree: main (scout-and-wave repo, no worktree)
commit: 871c82c07e649272fcc760f32c3cd0e0da52f0dc
files_changed:
  - prompts/saw-skill.md
files_created: []
interface_deviations: []
out_of_scope_deps: []
tests_added: []
verification: PASS
```

Notes: Bumped version header from v0.3.0 to v0.3.1. Extended step 3 in the "If a docs/IMPL-*.md file already exists" section with the SAW tag format requirement. The instruction specifies the exact format [SAW:wave{N}:agent-{X}], provides two concrete examples (wave1:agent-A and wave2:agent-B), and explains the observability rationale. Existing content about isolation: "worktree" and disjoint file ownership was preserved unchanged.

### Agent B — Completion Report

```yaml
status: complete
worktree: .claude/worktrees/wave1-agent-b
commit: 26fb69c
files_changed: []
files_created:
  - internal/claude/saw.go
  - internal/claude/saw_test.go
interface_deviations: []
out_of_scope_deps: []
tests_added:
  - TestParseSAWTag_Valid
  - TestParseSAWTag_ValidWave2AgentB
  - TestParseSAWTag_NoTag
  - TestParseSAWTag_MalformedMissingBracket
  - TestParseSAWTag_MalformedBadWave
  - TestParseSAWTag_MalformedEmptyAgent
  - TestComputeSAWWaves_Empty
  - TestComputeSAWWaves_SingleWave
  - TestComputeSAWWaves_MultiWave
  - TestComputeSAWWaves_SkipsUntagged
  - TestComputeSAWWaves_MultiSession
verification: PASS
```

Notes: Implemented ParseSAWTag using stdlib strings/strconv only (no regex). The function validates the exact [SAW:wave{N}:agent-{X}] prefix format, checks for positive integer wave numbers, and rejects empty agent strings. ComputeSAWWaves uses a nested map (sessionID -> waveN -> []SAWAgentRun) for accumulation, then builds sorted SAWSession/SAWWave/SAWAgentRun slices. Sessions are sorted by SessionID for deterministic test output. The test named TestParseSAWTag_ValidWave2AgentB was used in place of TestParseSAWTag_ValidMultiLetter (the task spec listed both names in different sections; the test covers wave2/agent-B as the primary valid variant). All 11 tests pass. Agent C can import claude.ComputeSAWWaves, claude.SAWSession, claude.SAWWave, and claude.SAWAgentRun from the claude package without any modifications.

### Agent C — Completion Report

```yaml
status: complete
worktree: .claude/worktrees/wave2-agent-c
commit: 530c498
files_changed:
  - internal/mcp/tools.go
files_created:
  - internal/mcp/saw_tools_test.go
interface_deviations: []
out_of_scope_deps: []
tests_added:
  - TestGetSAWSessions_Empty
  - TestGetSAWSessions_ReturnsSAWOnly
  - TestGetSAWSessions_LimitsN
  - TestGetSAWWaveBreakdown_Found
  - TestGetSAWWaveBreakdown_NotFound
verification: PASS
```

Notes: Added all result types (SAWSessionsResult, SAWSessionSummary, SAWWaveBreakdownResult, SAWWaveDetail, SAWAgentDetail) inline in tools.go. Both handlers follow the same pattern as handleGetRecentSessions: parse optional n for get_saw_sessions, unmarshal required session_id for get_saw_wave_breakdown. Both tools are registered in addTools after get_recent_sessions; get_saw_sessions reuses the existing recentNSchema package-level var; the session_id schema for get_saw_wave_breakdown is defined inline per spec. The test file uses helper functions (writeTranscriptJSONL, sawTranscriptLines, untaggedTranscriptLines) to write valid JSONL transcript fixtures under the expected projects/<projectHash>/<sessionID>.jsonl path. No new external packages were imported. All 5 SAW tests pass alongside all existing mcp tests.
