# IMPL: Cross-Session Memory for claudewatch

<!-- scout v0.3.0 — generated 2026-03-03 -->

**Feature:** Persistent working memory system that tracks task history, blockers, partial progress, and learnings across Claude Code sessions. Turns claudewatch from a mirror (showing current state) into a journal (remembering past attempts).

---

## Suitability Assessment

**Verdict: SUITABLE**

All five gate questions resolve with workable solutions.

### Q1: Does SessionEnd hook exist in Claude Code?

**NO.** Claude Code only provides `SessionStart` and `PostToolUse` hooks (verified in `~/.claude/settings.json` and codebase inspection). No SessionEnd hook exists.

**Workaround:** Use **SessionStart-triggered lazy evaluation**. When a session starts, check if the previous session (by project) has completed and is not yet in working memory. If so, extract its memory before injecting the current briefing. This adds negligible latency to SessionStart and requires no new hooks.

### Q2: How to detect task boundaries?

**Feasible via multi-signal heuristics.**

Data sources available:
- `SessionFacet.UnderlyingGoal` — AI-generated task description (already computed by Claude Code)
- `SessionMeta.FirstPrompt` — first user message
- Git branch detection via `internal/claude/repo_extract.go` pattern
- Commit messages from `SessionMeta.GitCommits`

**Task identifier strategy:**
1. **Primary:** Use `SessionFacet.UnderlyingGoal` directly (e.g., "Add rate limiting to API middleware")
2. **Secondary:** Hash of `(FirstPrompt + ProjectPath + GitBranch)` when facet is missing
3. **Fallback:** Use session ID if neither is available

**Why this works:** `UnderlyingGoal` is already computed by Claude Code's facet analysis and provides human-readable task descriptions. No additional LLM calls needed.

### Q3: How to detect blockers?

**Strong signals available in SessionFacet data.**

Blocker detection rules:
1. **Abandoned task:** `Outcome == "not_achieved"` + `FrictionCounts["user_rejected_action"] > 0`
2. **Consecutive tool errors:** Already tracked by `hook.go` — 3+ consecutive errors indicates active blocker
3. **Chronic friction:** Same friction type in >30% of last 10 sessions (already implemented in `hook.go:hookChronicPatternNote`)
4. **Stuck state:** Zero commits + high tool error rate (>5 errors per session)

Friction types from `SessionFacet.FrictionCounts`:
- `"wrong_approach"`, `"buggy_code"`, `"user_rejected_action"`, `"tool_error"`
- `"retry:Bash"`, `"retry:Edit"`, `"retry:Read"`, etc.

`FrictionDetail` string provides human-readable description for storage.

### Q4: What's the test suite command?

`make test` (which runs `go test ./... -v`).

Build time: ~5-15 seconds (pure Go, no CGO).

### Q5: Workaround architecture for missing SessionEnd hook?

**Option A: SessionStart lazy evaluation** (RECOMMENDED)
- On SessionStart, check if previous session for this project has completed
- If completed and not yet in working memory, extract it
- Update working-memory.json before printing briefing
- Pros: No polling, leverages existing hook, simple
- Cons: One-session delay (acceptable)

**Option B: Scan command enhancement**
- Add `claudewatch scan --update-memory` flag
- User runs manually or via cron
- Pros: Simple, reuses existing infrastructure
- Cons: Manual, not automatic

**Decision: Use Option A** (SessionStart lazy evaluation) as primary mechanism. Implement Option B as a CLI escape hatch for manual updates.

---

### Pre-implementation Scan

**Existing implementations to reference:**
- ✓ Store pattern: `internal/store/project_weights.go` (atomic write, JSON backing)
- ✓ Transcript parsing: `internal/claude/transcripts.go`, `internal/claude/facets.go`
- ✓ Session extraction: `internal/claude/session_meta.go`
- ✓ SessionStart hook: `internal/app/startup.go` (briefing injection point)
- ✓ Friction detection: `internal/app/hook.go:hookChronicPatternNote`

**Not yet implemented:**
- Working memory store
- Task extraction from facets
- Blocker aggregation across sessions
- SessionStart memory loading
- MCP memory query tools
- CLI memory commands

**Implementation coverage:** 0% (all new code)

---

### Estimated Times

- Scout phase: ~35 min (this document)
- Wave 1 (Store): ~15 min
- Wave 2 (Extraction + SessionStart): ~30 min (2 agents parallel)
- Wave 3 (MCP tools): ~20 min
- Wave 4 (CLI commands): ~15 min
- Merge & verification: ~10 min
- **Total SAW time:** ~125 min

Sequential baseline: ~140 min (all waves sequential)
Time savings: ~15 min from Wave 2 parallelism + interface contract safety

**Recommendation:** Proceed with 4-wave structure.

---

## Known Issues

None identified. All tests pass on `main` as of 2026-03-03.

`go test ./...` is clean (verified via CI status).

---

## Dependency Graph

```
Wave 1 (foundation)
internal/store/working_memory.go         ← Agent A — NEW, no dependencies
internal/store/working_memory_test.go    ← Agent A — NEW

Wave 2 (extraction + startup integration) — PARALLEL
internal/app/memory_extract.go           ← Agent B — NEW, depends on Wave 1 store
internal/app/memory_extract_test.go      ← Agent B — NEW
internal/app/startup.go                  ← Agent B — MODIFY, adds lazy memory update

internal/app/memory.go                   ← Agent C — NEW (CLI commands only, independent of Agent B)

Wave 3 (MCP integration)
internal/mcp/memory_tools.go             ← Agent D — NEW, depends on Wave 1 store
internal/mcp/tools.go                    ← Agent D — MODIFY (register new tools)

Wave 4 (CLI integration)
internal/app/memory.go                   ← Agent E — MODIFY (if needed, else skip wave)
```

**Critical paths:**
- Wave 1 must complete before Waves 2–4 begin (all depend on WorkingMemoryStore)
- Wave 2 agents (B and C) are independent and can run in parallel
- Wave 3 can begin once Wave 1 is merged (does not depend on Wave 2)
- Wave 4 is optional (CLI may be complete from Wave 2C)

**Cascade candidates:**
- None identified. All new code or isolated modifications.

---

## Interface Contracts

### Agent A delivers (Wave 1)

**File:** `internal/store/working_memory.go`

```go
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TaskMemory represents a single task's history across sessions.
type TaskMemory struct {
	TaskIdentifier string    `json:"task_identifier"`
	Sessions       []string  `json:"sessions"`        // session IDs that worked on this task
	Status         string    `json:"status"`          // "completed", "abandoned", "in_progress"
	BlockersHit    []string  `json:"blockers_hit"`    // descriptions of blockers encountered
	Solution       string    `json:"solution"`        // how it was resolved (empty if abandoned)
	Commits        []string  `json:"commits"`         // commit SHAs produced
	LastUpdated    time.Time `json:"last_updated"`
}

// BlockerMemory represents a known blocker for this project.
type BlockerMemory struct {
	File        string    `json:"file"`         // file path (if file-specific)
	Issue       string    `json:"issue"`        // description of the problem
	Solution    string    `json:"solution"`     // how to resolve (if known)
	Encountered []string  `json:"encountered"`  // dates encountered (YYYY-MM-DD format)
	LastSeen    time.Time `json:"last_seen"`
}

// WorkingMemory is the root structure stored in working-memory.json.
type WorkingMemory struct {
	Tasks        map[string]*TaskMemory `json:"tasks"`          // keyed by task_identifier
	Blockers     []*BlockerMemory       `json:"blockers"`
	ContextHints []string               `json:"context_hints"`  // frequently needed files
	LastScanned  time.Time              `json:"last_scanned"`   // last time memory was updated
}

// WorkingMemoryStore reads and writes per-project working memory.
// Backed by JSON file at ~/.config/claudewatch/projects/<project>/working-memory.json.
// Thread-safe for single-process use.
type WorkingMemoryStore struct {
	path string
	mu   sync.Mutex
}

// NewWorkingMemoryStore returns a store backed by the given file path.
// The file need not exist yet; Load returns an empty WorkingMemory if absent.
func NewWorkingMemoryStore(path string) *WorkingMemoryStore

// Load reads the working memory from disk.
// Returns an empty initialized WorkingMemory if the file does not exist.
// Returns an error only for I/O or JSON parse failures on an existing file.
func (s *WorkingMemoryStore) Load() (*WorkingMemory, error)

// Save writes the working memory to disk atomically (write-to-temp + rename).
// Creates the file and any parent directories if they do not exist.
func (s *WorkingMemoryStore) Save(wm *WorkingMemory) error

// AddOrUpdateTask adds a new task or updates an existing one.
// If the task already exists, merges sessions/blockers/commits.
func (s *WorkingMemoryStore) AddOrUpdateTask(task *TaskMemory) error

// AddBlocker appends a blocker if not already present (deduped by Issue field).
func (s *WorkingMemoryStore) AddBlocker(blocker *BlockerMemory) error

// GetTaskHistory returns all tasks matching a substring of task_identifier.
// Returns empty slice if no matches.
func (s *WorkingMemoryStore) GetTaskHistory(taskSubstring string) ([]*TaskMemory, error)

// GetRecentBlockers returns blockers seen within the last N days.
func (s *WorkingMemoryStore) GetRecentBlockers(days int) ([]*BlockerMemory, error)
```

**Implementation notes:**
- Follow atomic write pattern from `project_weights.go` exactly (write to temp file + rename)
- Use `sync.Mutex` for concurrent access safety
- Default path: `~/.config/claudewatch/projects/<project-name>/working-memory.json`
- Initialize empty WorkingMemory with `Tasks: make(map[string]*TaskMemory)`

---

### Agent B delivers (Wave 2 — memory extraction + startup integration)

**New file:** `internal/app/memory_extract.go`

```go
package app

import (
	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// ExtractTaskMemory converts a completed session's metadata and facet data
// into a TaskMemory entry. Returns nil if the session has insufficient data
// (e.g., no facet, no identifiable task).
func ExtractTaskMemory(session claude.SessionMeta, facet *claude.SessionFacet, commits []string) (*store.TaskMemory, error)

// ExtractBlockers analyzes a session's friction data and returns zero or more
// BlockerMemory entries. Only returns blockers that meet severity thresholds:
// - Consecutive tool errors >= 3
// - Outcome == "not_achieved" + friction count > 0
// - Chronic friction (same type in >30% of recent sessions)
func ExtractBlockers(session claude.SessionMeta, facet *claude.SessionFacet, projectName string, recentSessions []claude.SessionMeta, recentFacets []claude.SessionFacet) ([]*store.BlockerMemory, error)

// DeriveTaskIdentifier produces a stable task identifier from session data.
// Priority: SessionFacet.UnderlyingGoal > hash(FirstPrompt+ProjectPath) > SessionID
func DeriveTaskIdentifier(session claude.SessionMeta, facet *claude.SessionFacet) string
```

**Modification:** `internal/app/startup.go`

Add to `runStartup` function (before printing briefing):

```go
// Lazy memory update: check if the most recent completed session for this
// project is missing from working memory. If so, extract and store it.
func updateWorkingMemoryIfNeeded(cfg *config.Config, projectName string, sessions []claude.SessionMeta, facets []claude.SessionFacet) error
```

Insert call at line ~40 (after project detection, before friction rate calculation):

```go
if err := updateWorkingMemoryIfNeeded(cfg, projectName, sessions, facets); err != nil {
	// Non-fatal: log to stderr, continue with briefing.
	_, _ = fmt.Fprintf(os.Stderr, "claudewatch: memory update failed: %v\n", err)
}
```

**Logic for `updateWorkingMemoryIfNeeded`:**
1. Find most recent completed session for project (not current session)
2. Load working memory store
3. Check if that session ID is already in any task's Sessions list
4. If not present: extract task + blockers, update store
5. Return

---

### Agent C delivers (Wave 2 — CLI commands)

**New file:** `internal/app/memory.go`

```go
package app

import (
	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Query and manage cross-session working memory",
	Long:  `View task history, blockers, and context hints for a project.`,
}

var memoryShowCmd = &cobra.Command{
	Use:   "show [--project <name>]",
	Short: "Display working memory for a project",
	Run:   runMemoryShow,
}

var memoryClearCmd = &cobra.Command{
	Use:   "clear [--project <name>]",
	Short: "Clear working memory for a project",
	Run:   runMemoryClear,
}

func init() {
	rootCmd.AddCommand(memoryCmd)
	memoryCmd.AddCommand(memoryShowCmd)
	memoryCmd.AddCommand(memoryClearCmd)

	memoryShowCmd.Flags().StringP("project", "p", "", "Project name (defaults to current directory)")
	memoryClearCmd.Flags().StringP("project", "p", "", "Project name (defaults to current directory)")
}

func runMemoryShow(cmd *cobra.Command, args []string)
func runMemoryClear(cmd *cobra.Command, args []string)
```

**Output format for `memory show`:**

```
# Working Memory — myproject

## Tasks (3)

### "Add rate limiting to API middleware"
  Sessions: 2 (abc123, def456)
  Status:   completed
  Commits:  3 (a3f9c12, b8c21d3, c4e87f2)
  Solution: Used token bucket algorithm, 100 req/min default

### "Fix PostgreSQL connection pooling"
  Sessions: 1 (789ghi)
  Status:   abandoned
  Blockers: pg_hba.conf misconfiguration, SSL cert issues

## Blockers (2)

- **src/db/pool.go** — pg_hba.conf requires md5 auth, but code uses scram-sha-256
  Solution: Update postgresql.conf or use md5 auth in code
  Last seen: 2026-03-01

- **internal/api/handlers.go** — Rate limiter deadlocks under high concurrency
  Solution: Use sync.RWMutex instead of sync.Mutex
  Last seen: 2026-02-28

## Context Hints (5)

- src/middleware/auth.go
- config/database.yml
- internal/api/routes.go
- tests/integration/rate_limit_test.go
- docs/ARCHITECTURE.md
```

---

### Agent D delivers (Wave 3 — MCP tools)

**New file:** `internal/mcp/memory_tools.go`

```go
package mcp

import (
	"encoding/json"
)

// TaskHistoryResult is returned by get_task_history.
type TaskHistoryResult struct {
	Tasks []TaskMemoryResult `json:"tasks"`
}

type TaskMemoryResult struct {
	TaskIdentifier string   `json:"task_identifier"`
	Sessions       []string `json:"sessions"`
	Status         string   `json:"status"`
	BlockersHit    []string `json:"blockers_hit"`
	Solution       string   `json:"solution"`
	Commits        []string `json:"commits"`
}

// BlockersResult is returned by get_blockers.
type BlockersResult struct {
	Blockers []BlockerMemoryResult `json:"blockers"`
}

type BlockerMemoryResult struct {
	File        string   `json:"file"`
	Issue       string   `json:"issue"`
	Solution    string   `json:"solution"`
	Encountered []string `json:"encountered"`
}

// addMemoryTools registers get_task_history and get_blockers on s.
func addMemoryTools(s *Server)

// handleGetTaskHistory implements get_task_history MCP tool.
// Input: {"query": string, "project": string (optional)}
// Returns: TaskHistoryResult
func (s *Server) handleGetTaskHistory(args json.RawMessage) (any, error)

// handleGetBlockers implements get_blockers MCP tool.
// Input: {"project": string (optional), "days": int (default 30)}
// Returns: BlockersResult
func (s *Server) handleGetBlockers(args json.RawMessage) (any, error)
```

**Tool schemas:**

**get_task_history:**
```json
{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Search term to match against task descriptions (substring match, case-insensitive)"
    },
    "project": {
      "type": "string",
      "description": "Project name (optional, defaults to current session's project)"
    }
  },
  "required": ["query"],
  "additionalProperties": false
}
```

**get_blockers:**
```json
{
  "type": "object",
  "properties": {
    "project": {
      "type": "string",
      "description": "Project name (optional, defaults to current session's project)"
    },
    "days": {
      "type": "integer",
      "description": "Show blockers seen within last N days (default 30)"
    }
  },
  "additionalProperties": false
}
```

**Modification:** `internal/mcp/tools.go`

Add to `addTools` function:

```go
func addTools(s *Server) {
	// ... existing tools ...
	addMemoryTools(s)
}
```

---

## File Ownership Table

| File | Agent | Wave | Action |
|------|-------|------|--------|
| `internal/store/working_memory.go` | A | 1 | CREATE |
| `internal/store/working_memory_test.go` | A | 1 | CREATE |
| `internal/app/memory_extract.go` | B | 2 | CREATE |
| `internal/app/memory_extract_test.go` | B | 2 | CREATE |
| `internal/app/startup.go` | B | 2 | MODIFY |
| `internal/app/memory.go` | C | 2 | CREATE |
| `internal/mcp/memory_tools.go` | D | 3 | CREATE |
| `internal/mcp/tools.go` | D | 3 | MODIFY |

No other files are modified. No cascade candidates identified.

---

## Wave Structure

```
Wave 1 (foundation — 1 agent)
└── Agent A: internal/store/working_memory.go + tests
    Delivers: WorkingMemoryStore, TaskMemory, BlockerMemory types

Wave 2 (extraction + integration — 2 agents PARALLEL)
├── Agent B: internal/app/memory_extract.go + startup.go modification
│   Delivers: ExtractTaskMemory, ExtractBlockers, SessionStart lazy update
│
└── Agent C: internal/app/memory.go (CLI commands)
    Delivers: memory show, memory clear commands

Wave 3 (MCP integration — 1 agent, blocked on Wave 1 only)
└── Agent D: internal/mcp/memory_tools.go + tools.go modification
    Delivers: get_task_history, get_blockers MCP tools

Wave 4 (optional — verify CLI completeness)
└── Review Wave 2C output. If CLI commands are complete, skip this wave.
```

**Merge strategy:**
- Wave 1 → main (single agent, no conflicts)
- Wave 2: Merge Agent B and Agent C branches simultaneously (no file overlap)
- Wave 3 → main (single agent, depends only on Wave 1 store)

---

## Agent Prompts

All prompts follow the 9-field format: Isolation Verification, Context, Goal, Constraints, Dependencies, Tests, Output, Verification Gate, Completion Signal.

---

### Wave 1 Agent A: Working Memory Store

```markdown
# Wave 1 Agent A: Working Memory Store

You are Wave 1 Agent A. Your task is to implement the persistent working memory store for claudewatch.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

Your working directory MUST be:
/Users/dayna.blackwell/code/claudewatch

Verify isolation NOW:

```bash
pwd
```

Expected output EXACTLY:
/Users/dayna.blackwell/code/claudewatch

If the output does NOT match, you are in the WRONG directory. STOP immediately and report the error. Do NOT proceed with any file operations.

If isolation is confirmed, proceed to Context.

---

## 1. Context

claudewatch tracks Claude Code session metrics. This feature adds **cross-session memory**: a persistent store that remembers task history, blockers, and progress across sessions.

You are implementing the data layer only: the JSON-backed store with atomic writes.

**Reference implementation:** `internal/store/project_weights.go`
- Atomic write pattern (write-to-temp + rename)
- sync.Mutex for concurrency safety
- Load returns empty non-nil value if file absent

**Data structures (from IMPL doc):**
- `WorkingMemory` — root container
- `TaskMemory` — task history across sessions
- `BlockerMemory` — known blockers for project

**Storage location:** `~/.config/claudewatch/projects/<project-name>/working-memory.json`

---

## 2. Goal

Create two files:
1. `internal/store/working_memory.go` — WorkingMemoryStore implementation
2. `internal/store/working_memory_test.go` — unit tests

**Deliverables:**

`working_memory.go` must export:
- `type WorkingMemory struct` (root)
- `type TaskMemory struct` (task history)
- `type BlockerMemory struct` (blocker record)
- `type WorkingMemoryStore struct` (store)
- `func NewWorkingMemoryStore(path string) *WorkingMemoryStore`
- `func (s *WorkingMemoryStore) Load() (*WorkingMemory, error)`
- `func (s *WorkingMemoryStore) Save(wm *WorkingMemory) error`
- `func (s *WorkingMemoryStore) AddOrUpdateTask(task *TaskMemory) error`
- `func (s *WorkingMemoryStore) AddBlocker(blocker *BlockerMemory) error`
- `func (s *WorkingMemoryStore) GetTaskHistory(taskSubstring string) ([]*TaskMemory, error)`
- `func (s *WorkingMemoryStore) GetRecentBlockers(days int) ([]*BlockerMemory, error)`

**Exact struct definitions are in the IMPL doc Interface Contracts section.**

---

## 3. Constraints

- **NO modifications to any existing files** — only create new files
- Follow atomic write pattern from `project_weights.go` exactly
- Use `sync.Mutex` for thread safety
- `Load()` returns empty initialized struct if file doesn't exist (not an error)
- `AddOrUpdateTask` merges sessions/blockers/commits if task exists
- `AddBlocker` deduplicates by Issue field (case-insensitive match)
- All timestamps use `time.Time` type
- JSON field names use snake_case (e.g., `task_identifier`, `last_updated`)

---

## 4. Dependencies

**Read these files for reference patterns:**
- `internal/store/project_weights.go` — atomic write pattern, store structure
- `internal/store/tags.go` — simpler JSON store example

**Do NOT import or modify:**
- `internal/claude/*` — not needed for this wave
- `internal/app/*` — not needed for this wave
- `internal/mcp/*` — not needed for this wave

---

## 5. Tests

`working_memory_test.go` must cover:
1. `Load()` on non-existent file returns empty WorkingMemory
2. `Save()` creates file and parent directories
3. `Load()` after `Save()` returns same data
4. `AddOrUpdateTask()` creates new task
5. `AddOrUpdateTask()` merges sessions for existing task
6. `AddBlocker()` adds new blocker
7. `AddBlocker()` deduplicates by Issue field
8. `GetTaskHistory()` substring match (case-insensitive)
9. `GetRecentBlockers()` filters by LastSeen date

Use Go's `testing` package. Create temp directories with `t.TempDir()`.

**Run tests:**
```bash
cd /Users/dayna.blackwell/code/claudewatch
go test ./internal/store -v -run TestWorkingMemory
```

All tests must pass.

---

## 6. Output Format

Two files at these exact paths:
- `/Users/dayna.blackwell/code/claudewatch/internal/store/working_memory.go`
- `/Users/dayna.blackwell/code/claudewatch/internal/store/working_memory_test.go`

Use Write tool for both files (Read not required — new files).

---

## 7. Verification Gate

Before declaring completion, run:

```bash
cd /Users/dayna.blackwell/code/claudewatch
go test ./internal/store -v -run TestWorkingMemory
go vet ./internal/store
```

**Exit criteria:**
- All tests pass
- `go vet` reports no issues
- Files compile without errors

If any check fails, fix and re-verify.

---

## 8. Completion Signal

When verification passes, output EXACTLY:

```
AGENT A COMPLETE
Files: internal/store/working_memory.go, internal/store/working_memory_test.go
Tests: PASS
```

Then STOP. Do not proceed to Wave 2 work.
```

---

### Wave 2 Agent B: Memory Extraction + SessionStart Integration

```markdown
# Wave 2 Agent B: Memory Extraction + SessionStart Integration

You are Wave 2 Agent B. Your task is to implement task/blocker extraction from completed sessions and integrate lazy memory updates into the SessionStart hook.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

Your working directory MUST be:
/Users/dayna.blackwell/code/claudewatch

Verify isolation NOW:

```bash
pwd
```

Expected output EXACTLY:
/Users/dayna.blackwell/code/claudewatch

If the output does NOT match, STOP immediately and report the error.

**Verify Wave 1 dependency:**

```bash
test -f internal/store/working_memory.go && echo "Wave 1 MERGED" || echo "ERROR: Wave 1 not merged"
```

Expected output: `Wave 1 MERGED`

If not, STOP. Wave 1 must merge before you proceed.

If both checks pass, proceed to Context.

---

## 1. Context

claudewatch tracks Claude Code session metrics. You are implementing the **extraction and integration layer** for cross-session memory:

1. **Extraction:** Parse completed sessions → TaskMemory + BlockerMemory
2. **Integration:** Update SessionStart hook to lazily extract memory from previous session

**Key data sources:**
- `SessionMeta` — session metadata (tokens, commits, timing)
- `SessionFacet` — AI analysis (UnderlyingGoal, FrictionCounts, Outcome)
- Wave 1 Store — `WorkingMemoryStore` (already merged)

**Reference implementations:**
- `internal/app/hook.go:hookChronicPatternNote` — friction pattern detection
- `internal/app/startup.go:runStartup` — SessionStart hook implementation
- `internal/claude/facets.go` — facet parsing

---

## 2. Goal

Create two new files + modify one existing file:

**New files:**
1. `internal/app/memory_extract.go` — extraction functions
2. `internal/app/memory_extract_test.go` — unit tests

**Modify:**
3. `internal/app/startup.go` — add lazy memory update before briefing

**Deliverables (memory_extract.go):**

```go
// ExtractTaskMemory converts a completed session into a TaskMemory entry.
// Returns nil if session has insufficient data (no facet, no identifiable task).
func ExtractTaskMemory(session claude.SessionMeta, facet *claude.SessionFacet, commits []string) (*store.TaskMemory, error)

// ExtractBlockers analyzes friction data and returns blocker entries.
// Severity thresholds:
// - Consecutive tool errors >= 3
// - Outcome == "not_achieved" + friction count > 0
// - Chronic friction (>30% of recent sessions)
func ExtractBlockers(session claude.SessionMeta, facet *claude.SessionFacet, projectName string, recentSessions []claude.SessionMeta, recentFacets []claude.SessionFacet) ([]*store.BlockerMemory, error)

// DeriveTaskIdentifier produces stable task identifier.
// Priority: facet.UnderlyingGoal > hash(FirstPrompt+ProjectPath) > SessionID
func DeriveTaskIdentifier(session claude.SessionMeta, facet *claude.SessionFacet) string
```

**Modification (startup.go):**

Add function:
```go
// updateWorkingMemoryIfNeeded checks if the most recent completed session
// for this project is missing from working memory. If so, extracts and stores it.
func updateWorkingMemoryIfNeeded(cfg *config.Config, projectName string, sessions []claude.SessionMeta, facets []claude.SessionFacet) error
```

Insert call in `runStartup` after project detection (around line 40), before friction rate calculation:

```go
if err := updateWorkingMemoryIfNeeded(cfg, projectName, sessions, facets); err != nil {
	// Non-fatal: log to stderr, continue.
	_, _ = fmt.Fprintf(os.Stderr, "claudewatch: memory update failed: %v\n", err)
}
```

---

## 3. Constraints

- **Task identifier:** Use `SessionFacet.UnderlyingGoal` if present, else hash `FirstPrompt`
- **Status determination:**
  - `Outcome == "fully_achieved"` → `"completed"`
  - `Outcome == "not_achieved"` → `"abandoned"`
  - Otherwise → `"in_progress"`
- **Blocker extraction:** Only extract if friction is significant:
  - Tool errors >= 5 per session, OR
  - `Outcome == "not_achieved"`, OR
  - Chronic pattern (same friction type in >30% of last 10 sessions)
- **Solution field:** Populate only if status is "completed" AND commits > 0
- **Memory update:** Only update if most recent session is NOT current session
- **Error handling:** Memory update failures are non-fatal (log to stderr, continue)

---

## 4. Dependencies

**Wave 1 (must be merged):**
- `internal/store/working_memory.go` — WorkingMemoryStore, TaskMemory, BlockerMemory

**Read but not modify:**
- `internal/claude/types.go` — SessionMeta, SessionFacet
- `internal/claude/facets.go` — ParseAllFacets
- `internal/claude/session_meta.go` — ParseAllSessionMeta
- `internal/app/hook.go` — hookChronicPatternNote pattern
- `internal/config/config.go` — ConfigDir()

---

## 5. Tests

`memory_extract_test.go` must cover:

1. `DeriveTaskIdentifier` with UnderlyingGoal present
2. `DeriveTaskIdentifier` fallback to FirstPrompt hash
3. `ExtractTaskMemory` completed session (commits > 0)
4. `ExtractTaskMemory` abandoned session (not_achieved outcome)
5. `ExtractBlockers` with high tool errors
6. `ExtractBlockers` with not_achieved outcome
7. `ExtractBlockers` returns empty slice for successful session

Use table-driven tests. Mock SessionMeta and SessionFacet structs.

**Run tests:**
```bash
cd /Users/dayna.blackwell/code/claudewatch
go test ./internal/app -v -run TestMemoryExtract
go test ./internal/app -v -run TestStartup
```

All tests must pass.

---

## 6. Output Format

Three files modified/created:
- `internal/app/memory_extract.go` (CREATE)
- `internal/app/memory_extract_test.go` (CREATE)
- `internal/app/startup.go` (MODIFY)

Use Read tool on `startup.go` before Edit tool.

---

## 7. Verification Gate

Before declaring completion:

```bash
cd /Users/dayna.blackwell/code/claudewatch
go test ./internal/app -v -run 'TestMemoryExtract|TestStartup'
go vet ./internal/app
```

**Exit criteria:**
- All tests pass
- `go vet` clean
- No compilation errors
- `updateWorkingMemoryIfNeeded` is called in `runStartup` before briefing

If any check fails, fix and re-verify.

---

## 8. Completion Signal

When verification passes, output EXACTLY:

```
AGENT B COMPLETE
Files: internal/app/memory_extract.go, internal/app/memory_extract_test.go, internal/app/startup.go
Tests: PASS
```

Then STOP.
```

---

### Wave 2 Agent C: CLI Commands

```markdown
# Wave 2 Agent C: CLI Memory Commands

You are Wave 2 Agent C. Your task is to implement CLI commands for querying and managing working memory.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

Your working directory MUST be:
/Users/dayna.blackwell/code/claudewatch

Verify isolation NOW:

```bash
pwd
```

Expected output EXACTLY:
/Users/dayna.blackwell/code/claudewatch

If the output does NOT match, STOP immediately and report the error.

**Verify Wave 1 dependency:**

```bash
test -f internal/store/working_memory.go && echo "Wave 1 MERGED" || echo "ERROR: Wave 1 not merged"
```

Expected output: `Wave 1 MERGED`

If not, STOP. Wave 1 must merge before you proceed.

If both checks pass, proceed to Context.

---

## 1. Context

claudewatch tracks Claude Code session metrics. You are implementing **CLI commands** for working memory:

- `claudewatch memory show` — display tasks, blockers, context hints
- `claudewatch memory clear` — delete working memory for a project

**Reference implementations:**
- `internal/app/scan.go` — CLI command structure with Cobra
- `internal/app/gaps.go` — formatted output with tables
- Wave 1 Store — `WorkingMemoryStore` (already merged)

---

## 2. Goal

Create one new file:
- `internal/app/memory.go` — CLI commands

**Deliverables:**

Commands:
```bash
claudewatch memory show [--project <name>]
claudewatch memory clear [--project <name>]
```

If `--project` is omitted, derive from current directory (`os.Getwd()` → `filepath.Base()`).

**Output format for `memory show`:**

```
# Working Memory — myproject

## Tasks (3)

### "Add rate limiting to API middleware"
  Sessions: 2 (abc123, def456)
  Status:   completed
  Commits:  3 (a3f9c12, b8c21d3, c4e87f2)
  Solution: Used token bucket algorithm, 100 req/min default

### "Fix PostgreSQL connection pooling"
  Sessions: 1 (789ghi)
  Status:   abandoned
  Blockers: pg_hba.conf misconfiguration, SSL cert issues

## Blockers (2)

- **src/db/pool.go** — pg_hba.conf requires md5 auth
  Solution: Update postgresql.conf or use md5 auth
  Last seen: 2026-03-01

## Context Hints (5)

- src/middleware/auth.go
- config/database.yml
- internal/api/routes.go
```

If working memory is empty, print:
```
# Working Memory — myproject

No task history or blockers recorded yet.
```

**`memory clear` behavior:**
- Prompt for confirmation: `Delete working memory for <project>? (y/N): `
- If confirmed, delete the working-memory.json file
- Print: `Working memory cleared for <project>.`

---

## 3. Constraints

- Use Cobra for CLI structure (`var memoryCmd = &cobra.Command{...}`)
- Register subcommands in `init()` function
- Add `memoryCmd` to `rootCmd` in `init()`
- Project path: `~/.config/claudewatch/projects/<project-name>/working-memory.json`
- Use `config.ConfigDir()` to get base path
- Truncate long commit SHAs to 7 characters for display
- Truncate long session IDs to 7 characters for display
- Use `fmt.Printf` for formatted output (not a table library)

---

## 4. Dependencies

**Wave 1 (must be merged):**
- `internal/store/working_memory.go` — WorkingMemoryStore

**Read but not modify:**
- `internal/config/config.go` — ConfigDir()
- `internal/app/scan.go` — Cobra command pattern
- `internal/app/gaps.go` — formatted output pattern

---

## 5. Tests

Manual testing only (CLI commands are end-to-end). No unit tests required for this agent.

**Verify commands exist:**
```bash
cd /Users/dayna.blackwell/code/claudewatch
go run ./cmd/claudewatch memory --help
go run ./cmd/claudewatch memory show --help
go run ./cmd/claudewatch memory clear --help
```

All should print help text without errors.

---

## 6. Output Format

One file:
- `internal/app/memory.go` (CREATE)

Use Write tool (new file, no Read needed).

---

## 7. Verification Gate

Before declaring completion:

```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./cmd/claudewatch
./bin/claudewatch memory --help
./bin/claudewatch memory show --help
./bin/claudewatch memory clear --help
```

**Exit criteria:**
- Binary compiles without errors
- All three commands show help text
- No runtime panics

If any check fails, fix and re-verify.

---

## 8. Completion Signal

When verification passes, output EXACTLY:

```
AGENT C COMPLETE
Files: internal/app/memory.go
Build: SUCCESS
```

Then STOP.
```

---

### Wave 3 Agent D: MCP Tools

```markdown
# Wave 3 Agent D: MCP Memory Tools

You are Wave 3 Agent D. Your task is to implement MCP tools for querying working memory from within Claude Code sessions.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

Your working directory MUST be:
/Users/dayna.blackwell/code/claudewatch

Verify isolation NOW:

```bash
pwd
```

Expected output EXACTLY:
/Users/dayna.blackwell/code/claudewatch

If the output does NOT match, STOP immediately and report the error.

**Verify Wave 1 dependency:**

```bash
test -f internal/store/working_memory.go && echo "Wave 1 MERGED" || echo "ERROR: Wave 1 not merged"
```

Expected output: `Wave 1 MERGED`

If not, STOP. Wave 1 must merge before you proceed.

If both checks pass, proceed to Context.

---

## 1. Context

claudewatch exposes 26 MCP tools to Claude Code. You are adding two new tools for cross-session memory:

- `get_task_history` — query previous task attempts
- `get_blockers` — list known blockers for project

**Reference implementations:**
- `internal/mcp/multiproject_tools.go` — tool registration pattern
- `internal/mcp/health_tools.go` — project-scoped query pattern
- `internal/mcp/tools.go` — addTools function
- Wave 1 Store — `WorkingMemoryStore` (already merged)

---

## 2. Goal

Create one new file + modify one existing file:

**New file:**
1. `internal/mcp/memory_tools.go` — tool handlers and types

**Modify:**
2. `internal/mcp/tools.go` — register new tools in `addTools` function

**Deliverables (memory_tools.go):**

Types:
```go
type TaskHistoryResult struct {
	Tasks []TaskMemoryResult `json:"tasks"`
}

type TaskMemoryResult struct {
	TaskIdentifier string   `json:"task_identifier"`
	Sessions       []string `json:"sessions"`
	Status         string   `json:"status"`
	BlockersHit    []string `json:"blockers_hit"`
	Solution       string   `json:"solution"`
	Commits        []string `json:"commits"`
}

type BlockersResult struct {
	Blockers []BlockerMemoryResult `json:"blockers"`
}

type BlockerMemoryResult struct {
	File        string   `json:"file"`
	Issue       string   `json:"issue"`
	Solution    string   `json:"solution"`
	Encountered []string `json:"encountered"`
}
```

Functions:
```go
func addMemoryTools(s *Server)
func (s *Server) handleGetTaskHistory(args json.RawMessage) (any, error)
func (s *Server) handleGetBlockers(args json.RawMessage) (any, error)
```

**Tool schemas (from IMPL doc Interface Contracts section).**

**Modification (tools.go):**

Add to `addTools` function:
```go
addMemoryTools(s)
```

---

## 3. Constraints

- **Project resolution:** If `project` param is empty, use current session's project from cwd
- **Store path:** `~/.config/claudewatch/projects/<project-name>/working-memory.json`
- **Query behavior (`get_task_history`):**
  - Substring match on `TaskIdentifier` (case-insensitive)
  - Return all matching tasks sorted by LastUpdated desc
- **Blockers behavior (`get_blockers`):**
  - Filter by `LastSeen` within last N days
  - Sort by LastSeen desc
  - Default days: 30
- **Error handling:** Return empty results if working memory file doesn't exist (not an error)

---

## 4. Dependencies

**Wave 1 (must be merged):**
- `internal/store/working_memory.go` — WorkingMemoryStore

**Read but not modify:**
- `internal/mcp/tools.go` — tool registration
- `internal/mcp/multiproject_tools.go` — tool pattern
- `internal/mcp/jsonrpc.go` — Server struct
- `internal/config/config.go` — ConfigDir()

---

## 5. Tests

Manual testing via MCP protocol. No unit tests required (MCP server is tested end-to-end).

**Verify tools are registered:**
```bash
cd /Users/dayna.blackwell/code/claudewatch
echo '{"jsonrpc":"2.0","method":"tools/list","id":1}' | go run ./cmd/claudewatch mcp | jq '.result.tools[] | select(.name | startswith("get_task") or startswith("get_blockers"))'
```

Should return tool definitions for `get_task_history` and `get_blockers`.

---

## 6. Output Format

Two files:
- `internal/mcp/memory_tools.go` (CREATE)
- `internal/mcp/tools.go` (MODIFY)

Use Read tool on `tools.go` before Edit tool.

---

## 7. Verification Gate

Before declaring completion:

```bash
cd /Users/dayna.blackwell/code/claudewatch
go test ./internal/mcp -v
go vet ./internal/mcp
echo '{"jsonrpc":"2.0","method":"tools/list","id":1}' | go run ./cmd/claudewatch mcp | jq '.result.tools[] | select(.name == "get_task_history" or .name == "get_blockers")'
```

**Exit criteria:**
- Tests pass
- `go vet` clean
- MCP server lists both new tools
- No compilation errors

If any check fails, fix and re-verify.

---

## 8. Completion Signal

When verification passes, output EXACTLY:

```
AGENT D COMPLETE
Files: internal/mcp/memory_tools.go, internal/mcp/tools.go
Tests: PASS
MCP: get_task_history, get_blockers registered
```

Then STOP.
```

---

## Final Integration Test

After all waves merge, run:

```bash
cd /Users/dayna.blackwell/code/claudewatch
make test
go build ./cmd/claudewatch

# CLI test
./bin/claudewatch memory show --project test-project

# MCP test
echo '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"get_task_history","arguments":{"query":"test"}},"id":1}' | ./bin/claudewatch mcp

# SessionStart integration test
cd /tmp/test-project
claudewatch startup
```

**Success criteria:**
- All tests pass
- CLI commands work
- MCP tools are callable
- SessionStart hook runs without errors

---

## Implementation Notes

### Task Status Determination

From `SessionFacet.Outcome` field:
- `"fully_achieved"` → `"completed"`
- `"not_achieved"` → `"abandoned"`
- `"mostly_achieved"` → `"in_progress"`
- `"partially_achieved"` → `"in_progress"`

### Blocker Severity Thresholds

Extract blocker if ANY of:
1. Tool errors >= 5 (from `SessionMeta.ToolErrors`)
2. Outcome == "not_achieved" AND friction count > 0
3. Same friction type in >30% of last 10 sessions (chronic pattern)

### Context Hints Extraction

Not implemented in initial version. Placeholder field in store for future enhancement.

Future strategy:
- Track files touched in completed tasks
- Count frequency across tasks
- Top 10 most-touched files → context hints

### Memory File Location

Per-project working memory stored at:
```
~/.config/claudewatch/projects/<project-name>/working-memory.json
```

Example:
```
~/.config/claudewatch/projects/commitmux/working-memory.json
~/.config/claudewatch/projects/claudewatch/working-memory.json
```

### SessionStart Update Timing

The lazy update happens **before** the briefing is printed:

1. SessionStart hook fires
2. `updateWorkingMemoryIfNeeded()` checks last session
3. If needed, extracts and stores memory (takes ~10-50ms)
4. Briefing is printed (includes project health)
5. Control returns to Claude Code

One-session delay is acceptable: memory from session N is available in session N+1.

### Concurrent Safety

`WorkingMemoryStore` uses `sync.Mutex` for thread safety. Only one claudewatch process should write to a project's memory file at a time. Since claudewatch is typically invoked via hooks (one session at a time per project), race conditions are unlikely.

If concurrent writes are detected (file lock contention), the store will fail gracefully and log to stderr without crashing the session.

---

## Success Metrics

Post-deployment, measure:
1. **Memory coverage:** % of completed sessions that have task entries
2. **Blocker detection rate:** % of abandoned sessions with extracted blockers
3. **Query usage:** How often Claude calls `get_task_history` / `get_blockers`
4. **Context effectiveness:** Do sessions with memory context have lower friction?

Track these via claudewatch's own session analysis (meta-observation: claudewatch watching itself).

---

## Future Enhancements

Not in scope for initial implementation:

1. **Context hints extraction** — auto-detect frequently needed files
2. **Solution mining** — extract solution patterns from commit diffs
3. **Blocker resolution tracking** — detect when blockers are fixed
4. **Cross-project blocker correlation** — identify common patterns across repos
5. **MCP briefing injection** — add memory summary to SessionStart briefing automatically
6. **Blocker prediction** — warn before hitting known blockers

These require additional data pipelines and ML/heuristic analysis. Defer to post-v1.

---

## Rollout Strategy

1. **Wave 1:** Merge store, deploy, verify file writes work
2. **Wave 2:** Merge extraction + CLI, test manually on real projects
3. **Wave 3:** Merge MCP tools, test via Claude Code session
4. **Production:** Update ~/.claude/CLAUDE.md with memory query instructions
5. **Monitor:** Track usage via claudewatch metrics for 2 weeks
6. **Iterate:** Based on friction patterns and query usage data

---

## Appendix: Data Flow Diagram

```
Session N completes
       ↓
Session N+1 starts (SessionStart hook fires)
       ↓
claudewatch startup runs
       ↓
updateWorkingMemoryIfNeeded() checks:
  - Is Session N in working memory? NO
       ↓
  - Load Session N metadata + facet
       ↓
  - ExtractTaskMemory() → TaskMemory
  - ExtractBlockers() → [BlockerMemory]
       ↓
  - WorkingMemoryStore.AddOrUpdateTask()
  - WorkingMemoryStore.AddBlocker()
       ↓
  - Save working-memory.json
       ↓
SessionStart briefing prints (includes project health)
       ↓
Claude receives briefing + has MCP tools available
       ↓
Claude calls get_task_history("rate limiting")
       ↓
MCP handler → WorkingMemoryStore.GetTaskHistory()
       ↓
Returns: [{task: "Add rate limiting", status: "completed", commits: [...]}]
       ↓
Claude: "I see we implemented rate limiting before in commit a3f9c12. I'll use that pattern."
```

---

## End of IMPL Document

**Scout:** Agent complete. Proceed with Wave 1.
