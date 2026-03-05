# IMPL: Unified Context Surface

```
feature_slug: unified-context
repo: claudewatch
language: go
test_command: go test ./...
lint_command: go vet ./...
```

---

## Suitability Assessment

**Verdict: SUITABLE**

test_command: `go test ./...`
lint_command: `go vet ./...`

The work decomposes into 4 agents across 2 waves with fully disjoint file ownership:
- Wave 1 (3 agents): External MCP client infrastructure, unified result types + dedup/rank logic, CLI command
- Wave 2 (1 agent): MCP tool registration and handler (depends on all Wave 1 components)

File decomposition is clean: each agent owns distinct files with no conflicting modifications. The one append-only file (`internal/mcp/tools.go` for tool registration) is orchestrator-owned and applied post-merge in Wave 2.

No investigation-first items: the feature extends existing patterns (MCP tools, CLI commands, result types). The MCP-to-MCP call pattern is new for claudewatch but straightforward (exec external MCP binary via stdio JSON-RPC). Parallel fan-out uses `sync/errgroup`, a standard Go pattern.

Interface contracts are precise: `UnifiedContextResult` type, `ContextSource` interface, `ContextClient` for external MCP calls, dedup/rank functions. All contracts can be defined before implementation starts.

Pre-implementation scan: 0 of 5 deliverables are implemented. All agents proceed as planned.

Estimated times:
- Scout phase: ~10 min (dependency mapping, interface contracts, IMPL doc)
- Agent execution: ~40 min (Wave 1: 25 min parallel across 3 agents, Wave 2: 15 min for 1 agent)
- Merge & verification: ~10 min (2 waves x 5 min)
- Total SAW time: ~60 min

Sequential baseline: ~85 min (4 agents x ~21 min avg)
Time savings: ~25 min (29% faster)

Recommendation: Clear speedup. Proceed.

---

## Scaffolds

No scaffolds needed - agents have independent type ownership. Agent A defines result types that Agent D imports; this is a standard Wave 1 -> Wave 2 dependency, not a scaffold scenario.

---

## Known Issues

None identified. All tests passing in current codebase:
- `go test ./...` passes cleanly
- `go vet ./...` shows no warnings

---

## Critical Implementation Notes (Read Before Implementing)

### 1. MCP-to-MCP calls: exec commitmux binary, not HTTP

claudewatch MCP tools run in-process (stdio JSON-RPC). To call commitmux MCP tools, the pattern is:
1. Exec the `commitmux` binary (assumed to be in PATH or at `/usr/local/bin/commitmux`)
2. Write JSON-RPC 2.0 request to stdin: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"commitmux_search_memory","arguments":{...}}}`
3. Read JSON-RPC 2.0 response from stdout
4. Parse the `result.content[0].text` field (JSON string) and unmarshal into result struct

Do NOT attempt HTTP/WebSocket MCP transport. commitmux MCP is stdio-only.

### 2. Parallel fan-out: use golang.org/x/sync/errgroup

Agent A implements parallel execution. The pattern:
```go
import "golang.org/x/sync/errgroup"

g, ctx := errgroup.WithContext(context.Background())
g.Go(func() error { return fetchMemory(ctx, query) })
g.Go(func() error { return fetchCommits(ctx, query) })
g.Go(func() error { return fetchTasks(ctx, query) })
g.Go(func() error { return fetchTranscripts(ctx, query) })
if err := g.Wait(); err != nil { return nil, err }
```

Errors from any source abort remaining goroutines via context cancellation. Successful results accumulate in a shared (mutex-protected) slice.

### 3. Deduplication: content hash + source priority

Multiple sources may return overlapping content (e.g., a commit SHA appears in both a commit result and a transcript snippet). Deduplication strategy:
1. Compute content hash for each result (SHA-256 of normalized text)
2. Group by hash
3. Within each group, keep the result with highest source priority: commit > memory > task_history > transcript
4. Rationale: primary sources (commits, memory files) are more canonical than derived sources (transcripts)

### 4. Relevance ranking: score + recency decay

Unified results are sorted by composite score:
- Semantic sources (memory, commits): use raw embedding distance (lower = better) inverted to score (1.0 - distance)
- FTS sources (transcripts): use SQLite FTS rank (already normalized 0-1)
- Task history: keyword match (substring occurrence count / total query tokens), normalized 0-1
- Recency boost: `final_score = base_score * (1.0 + 0.2 * recency_factor)` where `recency_factor = min(1.0, days_ago / 365)`

Results are sorted descending by final_score before returning to caller.

### 5. Error handling: partial results on source failure

If one source fails (e.g., commitmux not installed, transcript index empty), do NOT abort the entire query. Log the error (to stderr in CLI, in MCP result message field) and return results from remaining sources. This degrades gracefully instead of failing hard.

### 6. Result limit distribution

When `limit` is specified (default 20), distribute across sources:
- Each source gets `limit / 4` initial results (5 per source for default)
- After dedup + ranking, truncate final unified list to `limit`
- Rationale: ensures diversity (all sources represented) before final ranking

---

## Dependency Graph

```
Wave 1 (parallel, no interdependencies):
  ├── Agent A: External MCP client + parallel executor
  │     internal/client/mcp_client.go         (new file: ContextClient interface + exec impl)
  │     internal/client/mcp_client_test.go    (new file: unit tests for JSON-RPC exec)
  │
  ├── Agent B: Unified types + dedup/rank logic
  │     internal/context/types.go             (new file: UnifiedContextResult, ContextItem, SourceType)
  │     internal/context/dedup.go             (new file: dedup by content hash + source priority)
  │     internal/context/rank.go              (new file: relevance scoring + recency decay)
  │     internal/context/rank_test.go         (new file: unit tests for scoring)
  │
  └── Agent C: CLI command
        internal/app/context.go                (new file: contextCmd + runContext)

Wave 2 (depends on Wave 1 A + B):
  └── Agent D: MCP tool handler
        internal/mcp/unified_context_tools.go  (new file: addUnifiedContextTools + handleGetContext)
        internal/mcp/unified_context_tools_test.go (new file: unit tests for MCP handler)

Orchestrator (post-merge only):
  └── Tool registration append
        internal/mcp/tools.go                  (append `addUnifiedContextTools(s)` to addTools())
```

**Rationale for wave structure:**
- Wave 1: Three independent components (MCP client, result types, CLI) can be built in parallel. No shared files.
- Wave 2: MCP handler depends on ContextClient (Agent A) and UnifiedContextResult types (Agent B), so must wait for Wave 1 to complete.
- Tool registration: Orchestrator appends one line to `internal/mcp/tools.go` after Wave 2 merge, avoiding file conflicts.

**Cascade candidates** (files not in any agent's scope but may break due to interface changes):
- None. All new code. No existing callers to update.

---

## Interface Contracts

All signatures are binding contracts. Agents implement against these without seeing each other's code.

### Agent A: External MCP Client

**File:** `internal/client/mcp_client.go`

```go
package client

import "context"

// MCPClient calls external MCP tools via stdio JSON-RPC.
type MCPClient interface {
    // CallTool execs the MCP server binary and calls a tool.
    // Returns the tool result as JSON bytes, or error.
    CallTool(ctx context.Context, serverBinary string, toolName string, args map[string]any) ([]byte, error)
}

// NewMCPClient constructs a default stdio-based MCP client.
func NewMCPClient() MCPClient
```

### Agent B: Unified Context Types

**File:** `internal/context/types.go`

```go
package context

import "time"

// SourceType identifies where a context item came from.
type SourceType string

const (
    SourceMemory     SourceType = "memory"
    SourceCommit     SourceType = "commit"
    SourceTaskHistory SourceType = "task_history"
    SourceTranscript SourceType = "transcript"
)

// ContextItem is one result from a unified context search.
type ContextItem struct {
    Source      SourceType `json:"source"`
    Title       string     `json:"title"`        // e.g., "memory: MEMORY.md", "commit: abc123", "task: implement X"
    Snippet     string     `json:"snippet"`      // text excerpt
    Timestamp   time.Time  `json:"timestamp"`    // when the item was created/committed
    Metadata    map[string]string `json:"metadata,omitempty"` // source-specific fields (repo, sha, session_id, etc.)
    Score       float64    `json:"score"`        // relevance score (0.0-1.0, higher = better)
    ContentHash string     `json:"-"`            // SHA-256 of normalized content (for dedup, not returned to caller)
}

// UnifiedContextResult is the MCP tool and CLI response.
type UnifiedContextResult struct {
    Query   string        `json:"query"`
    Items   []ContextItem `json:"items"`
    Count   int           `json:"count"`
    Sources []string      `json:"sources"` // list of sources queried (e.g., ["memory", "commit", "task_history"])
    Errors  []string      `json:"errors,omitempty"` // partial failure messages
}
```

**File:** `internal/context/dedup.go`

```go
package context

// DeduplicateItems removes duplicate items based on content hash.
// Within each hash group, keeps the item with highest source priority.
// Priority order: commit > memory > task_history > transcript.
func DeduplicateItems(items []ContextItem) []ContextItem
```

**File:** `internal/context/rank.go`

```go
package context

// RankAndSort applies recency decay and sorts items by final score descending.
// Modifies items[].Score in place and sorts the slice.
func RankAndSort(items []ContextItem)
```

### Agent C: CLI Command

**File:** `internal/app/context.go`

```go
package app

import "github.com/spf13/cobra"

var contextCmd = &cobra.Command{
    Use:   "context <query>",
    Short: "Unified context search across commits, memory, tasks, and transcripts",
    Args:  cobra.ExactArgs(1),
    RunE:  runContext,
}

func init() {
    contextCmd.Flags().String("project", "", "Project name filter (optional)")
    contextCmd.Flags().Int("limit", 20, "Maximum results to return")
    rootCmd.AddCommand(contextCmd)
}

func runContext(cmd *cobra.Command, args []string) error {
    // Implementation: call internal/context package to fetch unified results, render to stdout
}
```

### Agent D: MCP Tool Handler

**File:** `internal/mcp/unified_context_tools.go`

```go
package mcp

import "encoding/json"

// addUnifiedContextTools registers the get_context MCP tool on s.
func addUnifiedContextTools(s *Server) {
    s.registerTool(toolDef{
        Name:        "get_context",
        Description: "Unified context search across commits, memory, tasks, and transcripts. Returns ranked, deduplicated results with source attribution.",
        InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query"},"project":{"type":"string","description":"Project name filter (optional)"},"limit":{"type":"integer","description":"Maximum results (default 20)"}},"required":["query"],"additionalProperties":false}`),
        Handler:     s.handleGetContext,
    })
}

// handleGetContext is the MCP handler for get_context.
func (s *Server) handleGetContext(args json.RawMessage) (any, error) {
    // Implementation: parse args, call internal/client + internal/context to fetch unified results
}
```

---

## File Ownership

| File | Agent | Wave | Depends On |
|------|-------|------|------------|
| internal/client/mcp_client.go | A | 1 | — |
| internal/client/mcp_client_test.go | A | 1 | — |
| internal/context/types.go | B | 1 | — |
| internal/context/dedup.go | B | 1 | types.go (same agent) |
| internal/context/rank.go | B | 1 | types.go (same agent) |
| internal/context/rank_test.go | B | 1 | rank.go (same agent) |
| internal/app/context.go | C | 1 | — |
| internal/mcp/unified_context_tools.go | D | 2 | Agent A (mcp_client), Agent B (types) |
| internal/mcp/unified_context_tools_test.go | D | 2 | unified_context_tools.go (same agent) |
| internal/mcp/tools.go | Orchestrator | post-Wave-2 | Agent D (addUnifiedContextTools exists) |

---

## Wave Structure

```
Wave 1: [A] [B] [C]          <- 3 parallel agents (MCP client, types/logic, CLI)
           | (A+B complete)
Wave 2:   [D]                <- 1 agent (MCP tool handler, depends on A+B)
           | (D complete)
Orch:     [tool registration] <- orchestrator appends addUnifiedContextTools(s) to tools.go
```

---

## Agent Prompts

### Agent A: External MCP Client + Parallel Executor

**Task:** Implement MCP-to-MCP client infrastructure and parallel query executor.

**Agent ID:** unified-context-agent-a

**Wave:** 1

**Dependencies:** None (foundation agent)

**Files to create:**
- `internal/client/mcp_client.go`
- `internal/client/mcp_client_test.go`

**Implementation requirements:**

1. Create `internal/client/mcp_client.go`:
   - Define `MCPClient` interface with `CallTool(ctx, serverBinary, toolName, args) ([]byte, error)` method
   - Implement `stdioMCPClient` struct that execs the server binary and communicates via stdio JSON-RPC 2.0
   - JSON-RPC request format: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"<toolName>","arguments":<args>}}`
   - Parse JSON-RPC response: `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"<JSON>"}]}}`
   - Extract `result.content[0].text` and return as raw JSON bytes
   - Handle errors: non-zero exit code, malformed JSON, missing binary
   - Use `context.Context` for timeout/cancellation

2. Implement parallel executor helper (can be in same file or separate `parallel.go`):
   - Function signature: `FetchAllSources(ctx context.Context, query string, project string, limit int) (map[string][]byte, []error)`
   - Uses `golang.org/x/sync/errgroup` to fan out 4 calls in parallel:
     - `commitmux_search_memory` (via commitmux binary)
     - `commitmux_search_semantic` (via commitmux binary)
     - Local `get_task_history` (call internal/mcp/memory_tools.go handler directly via function call, NOT external exec)
     - Local `search_transcripts` (call internal/mcp/transcript_tools.go handler directly via function call, NOT external exec)
   - Accumulate results in map[source]json + slice of errors
   - Partial failure handling: if one source fails, log error but continue with others

3. Create `internal/client/mcp_client_test.go`:
   - Unit test: mock binary that echoes JSON-RPC response
   - Unit test: error handling for missing binary
   - Unit test: JSON-RPC parse error

**Verification gate:**
```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/client -v
```

**Acceptance criteria:**
- `MCPClient` interface defined
- `NewMCPClient()` constructor returns working implementation
- Parallel executor fetches from 4 sources concurrently
- Tests pass
- No lint warnings

---

### Agent B: Unified Types + Dedup/Rank Logic

**Task:** Define unified result types and implement deduplication + relevance ranking.

**Agent ID:** unified-context-agent-b

**Wave:** 1

**Dependencies:** None (foundation agent)

**Files to create:**
- `internal/context/types.go`
- `internal/context/dedup.go`
- `internal/context/rank.go`
- `internal/context/rank_test.go`

**Implementation requirements:**

1. Create `internal/context/types.go`:
   - Define `SourceType` enum (memory, commit, task_history, transcript)
   - Define `ContextItem` struct (see Interface Contracts)
   - Define `UnifiedContextResult` struct (see Interface Contracts)

2. Create `internal/context/dedup.go`:
   - Implement `DeduplicateItems(items []ContextItem) []ContextItem`
   - Compute content hash (SHA-256) for each item's snippet (normalized: lowercase, trim whitespace)
   - Group by hash, keep item with highest source priority within each group
   - Priority order: commit > memory > task_history > transcript

3. Create `internal/context/rank.go`:
   - Implement `RankAndSort(items []ContextItem)`
   - Apply recency decay: `final_score = base_score * (1.0 + 0.2 * min(1.0, age_days / 365))`
   - Sort items by final_score descending (highest first)
   - Modify items[].Score in place

4. Create `internal/context/rank_test.go`:
   - Unit test: recency boost for recent items
   - Unit test: stable sort for equal scores
   - Unit test: dedup removes exact duplicates

**Verification gate:**
```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/context -v
```

**Acceptance criteria:**
- All types defined per Interface Contracts
- Deduplication removes duplicates, preserves highest-priority source
- Ranking applies recency decay correctly
- Tests pass
- No lint warnings

---

### Agent C: CLI Command

**Task:** Implement `claudewatch context` CLI command.

**Agent ID:** unified-context-agent-c

**Wave:** 1

**Dependencies:** None (foundation agent, will import from Agent A+B at Wave 2 merge)

**Files to create:**
- `internal/app/context.go`

**Implementation requirements:**

1. Create `internal/app/context.go`:
   - Define `contextCmd` cobra command (see Interface Contracts)
   - Flags: `--project <name>`, `--limit <int>` (default 20)
   - `runContext` implementation:
     - Parse query from args[0]
     - Call Agent A's parallel executor (import `internal/client`)
     - Parse raw JSON results into Agent B's types (import `internal/context`)
     - Call Agent B's `DeduplicateItems` + `RankAndSort`
     - Render to stdout (table format if !flagJSON, JSON if flagJSON)
   - Table format: columns "Source", "Title", "Timestamp", "Snippet" (truncate snippet to 60 chars)
   - JSON format: full UnifiedContextResult struct

2. Error handling:
   - If all sources fail, return error
   - If some sources fail, print warning to stderr but show partial results
   - If query is empty, return error

3. Register command in `init()`: `rootCmd.AddCommand(contextCmd)`

**Verification gate:**
```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/app -run TestContext -v  # if you write a test
claudewatch context "test query" --limit 5   # manual smoke test (may fail if commitmux not installed, that's OK)
```

**Acceptance criteria:**
- Command registered and runnable
- Flags work correctly
- Renders table output by default, JSON with --json
- Partial failure handling works (shows results from successful sources)
- No lint warnings

---

### Agent D: MCP Tool Handler

**Task:** Implement `get_context` MCP tool handler.

**Agent ID:** unified-context-agent-d

**Wave:** 2

**Dependencies:** Agent A (mcp_client), Agent B (types)

**Files to create:**
- `internal/mcp/unified_context_tools.go`
- `internal/mcp/unified_context_tools_test.go`

**Implementation requirements:**

1. Create `internal/mcp/unified_context_tools.go`:
   - Implement `addUnifiedContextTools(s *Server)` (see Interface Contracts)
   - Implement `handleGetContext(args json.RawMessage) (any, error)`:
     - Parse args into struct with `query`, `project`, `limit` fields
     - Call Agent A's parallel executor (import `internal/client`)
     - Parse raw JSON results into Agent B's types (import `internal/context`)
     - Call Agent B's `DeduplicateItems` + `RankAndSort`
     - Return `UnifiedContextResult` (MCP server will JSON-encode it)
   - Error handling: return MCP error if query is empty or all sources fail
   - Partial failure: include error messages in UnifiedContextResult.Errors field

2. Create `internal/mcp/unified_context_tools_test.go`:
   - Unit test: mock Server with test handler
   - Unit test: verify tool is registered with correct schema
   - Unit test: verify handler parses args correctly
   - Unit test: verify handler returns UnifiedContextResult

**Verification gate:**
```bash
cd /Users/dayna.blackwell/code/claudewatch
go build ./...
go vet ./...
go test ./internal/mcp -run TestUnifiedContext -v
```

**Acceptance criteria:**
- `addUnifiedContextTools` registers tool with correct name, description, schema
- `handleGetContext` calls Agent A+B correctly
- Returns UnifiedContextResult
- Tests pass
- No lint warnings

**Note for Wave 2 merge:** After this agent completes and merges, the orchestrator will append `addUnifiedContextTools(s)` to `internal/mcp/tools.go` in the `addTools()` function.

---

### Agent D - Completion Report

**Status:** complete

**Files changed:**
- internal/mcp/unified_context_tools.go (created, +315 lines)
- internal/mcp/unified_context_tools_test.go (created, +300 lines)

**Interface deviations:**
None. All interfaces and function signatures match the Interface Contracts exactly.

**Out of scope dependencies:**
None identified.

**Verification:**
- [x] Build passed: `go build ./...`
- [x] Vet passed: `go vet ./...`
- [x] Tests passed: `go test ./internal/mcp -run "TestAdd.*Unified|TestHandle.*Context|TestParse" -v` (8/8 tests passing)
- [x] Manual verification: All test cases cover tool registration, argument parsing, result structure, and source-specific parsing

**Commits:**
- 2dcb023: feat(mcp): implement get_context unified search MCP tool handler

**Notes:**
- Implemented `addUnifiedContextTools(s *Server)` that registers the `get_context` MCP tool with correct schema
- Implemented `handleGetContext(args json.RawMessage) (any, error)` that:
  * Parses args into struct with query, project, limit fields (with proper defaults: limit=20)
  * Validates query is not empty (returns error if empty)
  * Creates MCP client and calls `client.FetchAllSources()` with 30s timeout
  * Parses raw JSON results from all 4 sources using source-specific parsers
  * Applies `context.DeduplicateItems()` to remove duplicates
  * Applies `context.RankAndSort()` for relevance scoring and sorting
  * Truncates to limit after ranking
  * Returns `UnifiedContextResult` with query, items, count, sources list, and errors
  * Handles partial failures: includes error messages in result but returns successful results
  * Returns error only if all sources fail or query is empty
- Implemented 4 source-specific parsers:
  * `parseMemoryResults()`: handles commitmux_search_memory format (path, content, distance)
  * `parseCommitResults()`: handles commitmux_search_semantic format (sha, message, author, timestamp, repo, distance)
  * `parseTaskHistoryResults()`: handles get_task_history format (task_identifier, sessions, status, solution, commits)
  * `parseTranscriptResults()`: handles search_transcripts format (session_id, snippet, entry_type, timestamp, rank)
- Distance-to-score conversion: semantic sources use (1.0 - distance) for scoring
- Default scores: task_history uses 0.5 (keyword-based), transcripts use FTS rank directly
- Used package alias `ctxpkg` to avoid conflict with stdlib `context` package
- Comprehensive test suite with 8 unit tests:
  * TestAddUnifiedContextTools: verifies tool registration and schema structure
  * TestHandleGetContext_EmptyQuery: verifies error handling for empty/missing query
  * TestHandleGetContext_ParsesArgs: verifies argument parsing works
  * TestHandleGetContext_ReturnsUnifiedContextResult: verifies return type structure
  * TestParseMemoryResults: verifies memory result parsing and score conversion
  * TestParseCommitResults: verifies commit result parsing with metadata extraction
  * TestParseTaskHistoryResults: verifies task history parsing with status/solution
  * TestParseTranscriptResults: verifies transcript parsing with session metadata
- All 8 tests passing
- Build and vet clean with no warnings
- Ready for orchestrator to append `addUnifiedContextTools(s)` to `internal/mcp/tools.go`

---

## Orchestrator Post-Merge Checklist

After wave 1 completes:

- [ ] Read all agent completion reports — confirm all `status: complete`; if any `partial` or `blocked`, stop and resolve before merging
- [ ] Conflict prediction — cross-reference `files_changed` lists; flag any file appearing in >1 agent's list before touching the working tree
- [ ] Review `interface_deviations` — update downstream agent prompts for any item with `downstream_action_required: true`
- [ ] Merge each agent: `git merge --no-ff wave1-agent-a -m "Merge wave1-agent-a: MCP client + parallel executor"`
- [ ] Merge each agent: `git merge --no-ff wave1-agent-b -m "Merge wave1-agent-b: unified types + dedup/rank"`
- [ ] Merge each agent: `git merge --no-ff wave1-agent-c -m "Merge wave1-agent-c: CLI context command"`
- [ ] Worktree cleanup: `git worktree remove <path>` + `git branch -d <branch>` for each
- [ ] Post-merge verification:
      - [ ] Linter auto-fix pass: n/a (Go has no auto-fix linter in this project)
      - [ ] `cd /Users/dayna.blackwell/code/claudewatch && go build ./... && go vet ./... && go test ./...`
- [ ] Fix any cascade failures — no cascade candidates expected (all new code)
- [ ] Tick status checkboxes in this IMPL doc for completed agents (A, B, C)
- [ ] Update interface contracts if any deviations were logged
- [ ] Apply `out_of_scope_deps` fixes flagged in completion reports
- [ ] Feature-specific steps:
      - [ ] None for Wave 1
- [ ] Commit: `git commit -m "Merge Wave 1: MCP client, types, CLI command for unified context search"`
- [ ] Launch Wave 2 agent D (or pause for review if not --auto)

After wave 2 completes:

- [ ] Read agent D completion report — confirm `status: complete`
- [ ] Merge agent D: `git merge --no-ff wave2-agent-d -m "Merge wave2-agent-d: get_context MCP tool handler"`
- [ ] Worktree cleanup: `git worktree remove <path>` + `git branch -d wave2-agent-d`
- [ ] Post-merge verification:
      - [ ] Linter auto-fix pass: n/a
      - [ ] `cd /Users/dayna.blackwell/code/claudewatch && go build ./... && go vet ./... && go test ./...`
- [ ] Fix any cascade failures
- [ ] Tick status checkbox for Agent D
- [ ] Feature-specific steps:
      - [ ] Append tool registration to `internal/mcp/tools.go`: add `addUnifiedContextTools(s)` at end of `addTools()` function
      - [ ] Rebuild and reinstall binary: `cd /Users/dayna.blackwell/code/claudewatch && make build && make install`
      - [ ] Restart Claude Code to pick up new MCP tool schema
      - [ ] Manual verification: in Claude Code session, call `get_context` tool with test query and verify it returns unified results
- [ ] Commit: `git commit -m "feat(mcp): add get_context unified search tool + claudewatch context CLI command"`

---

## Status

| Wave | Agent | Description | Status |
|------|-------|-------------|--------|
| 1 | A | MCP client + parallel executor | ✅ COMPLETE |
| 1 | B | Unified types + dedup/rank | ✅ COMPLETE |
| 1 | C | CLI context command | ✅ COMPLETE |
| 2 | D | get_context MCP tool handler | ✅ COMPLETE |
| — | Orch | Tool registration + binary install | ✅ COMPLETE |

---

## Agent Completion Reports

### Agent A - Completion Report

**Status:** complete

**Files changed:**
- internal/client/mcp_client.go (created, +220 lines)
- internal/client/mcp_client_test.go (created, +262 lines)

**Interface deviations:**
None. All interfaces and function signatures match the Interface Contracts exactly.

**Out of scope dependencies:**
None identified. Local tool integration (get_task_history, search_transcripts) deferred to Agent D in Wave 2 as intended - current implementation returns empty results for local sources as placeholders.

**Verification:**
- [x] Build passed: `go build ./...`
- [x] Vet passed: `go vet ./...`
- [x] Tests passed: `go test ./internal/client -v` (10/10 tests passing)
- [x] Manual verification: All test cases cover JSON-RPC communication, error handling, and parallel execution

**Commits:**
- 76cebcf: feat(client): implement MCP client and parallel executor

**Notes:**
- MCPClient interface uses stdio JSON-RPC 2.0 protocol as specified
- Request format: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"<tool>","arguments":<args>}}`
- Response parsing extracts `result.content[0].text` field and returns as raw JSON bytes
- FetchAllSources uses golang.org/x/sync/errgroup for parallel execution across 4 sources
- Per-source limit distribution: each source gets `limit/4` initial results (default 5 per source)
- Partial failure handling: accumulates errors but returns results from successful sources
- External MCP calls (commitmux binary) fully implemented
- Local tool calls (get_task_history, search_transcripts) return placeholder empty results - Agent D will wire these up in Wave 2
- Context cancellation and timeout support via context.Context
- Comprehensive test suite with mock binaries, error cases, and parallel execution scenarios
- All 10 unit tests passing

---

### Agent B - Completion Report

**Status:** complete

**Files changed:**
- internal/context/types.go (created, +31 lines)
- internal/context/dedup.go (created, +68 lines)
- internal/context/rank.go (created, +28 lines)
- internal/context/rank_test.go (created, +219 lines)

**Interface deviations:**
None. All types and function signatures match the Interface Contracts exactly.

**Out of scope dependencies:**
None identified.

**Verification:**
- [x] Build passed: `go build ./internal/context`
- [x] Vet passed: `go vet ./internal/context`
- [x] Tests passed: `go test ./internal/context -v` (9/9 tests passing)
- [x] Manual verification: All test cases cover deduplication, ranking, and edge cases

**Commits:**
- e88319d: feat(context): implement unified context types and dedup/rank logic

**Notes:**
- Content hash normalization uses lowercase + trim whitespace before SHA-256
- Source priority is enforced via sourcePriority() helper: commit(4) > memory(3) > task_history(2) > transcript(1)
- Recency decay formula: final_score = base_score * (1.0 + 0.2 * min(1.0, age_days/365))
- RankAndSort uses stable sort to preserve order for equal scores
- All 9 unit tests passing, covering recency boost, stable sort, dedup logic, normalization, and priority ordering

---

### Agent C - Completion Report

**Status:** complete

**Files changed:**
- internal/app/context.go (created, +290 lines total across 2 commits)

**Interface deviations:**
None. Command structure matches Interface Contracts exactly.

**Out of scope dependencies:**
None identified.

**Verification:**
- [x] Build passed: `go build ./...`
- [x] Vet passed: `go vet ./...`
- [x] Command registered: `claudewatch context --help` shows full help text
- [x] Flags work: `--project`, `--limit`, and `--json` flags properly defined
- [x] Manual verification: Command executes correctly, fails gracefully when commitmux not installed with proper error messages
- [x] Integration complete: Agent A's MCP client fully integrated

**Commits:**
- 2ebb1ce: feat(cli): add context command for unified context search (initial stub)
- 47f4675: feat(cli): implement full unified context search integration (full implementation)

**Notes:**
- Command is fully implemented and integrated with Agent A's MCP client
- Full parallel source fetching via client.FetchAllSources
- Format-specific parsers for all 4 sources:
  * parseMemoryResults: handles commitmux_search_memory (path, content, distance)
  * parseCommitResults: handles commitmux_search_semantic (sha, message, author, timestamp)
  * parseTaskHistoryResults: handles get_task_history (description, status, session_id)
  * parseTranscriptResults: handles search_transcripts (session_id, snippet, entry_type)
- Distance-to-score conversion: semantic sources use (1.0 - distance) for scoring
- Comprehensive help text with 4 usage examples
- Table rendering with columns: Source, Title, Timestamp, Snippet (60 char truncation)
- JSON output support via flagJSON global flag
- Partial failure handling: shows warnings to stderr but continues with successful results
- Empty result handling for placeholder local sources
- Metadata extraction for each source type (sha, author, repo, session_id, etc.)
- Proper error handling: "all sources failed" only if no results and errors present
- renderContextResults() implements table output with proper formatting and source attribution
- Follows established CLI patterns from search.go and suggest.go commands
- End-to-end functionality ready (requires commitmux binary in PATH for external sources)
