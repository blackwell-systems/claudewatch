# IMPL: Multi-Project Session Attribution

**Feature slug:** `multi-project-sessions`
**Test command:** `go test ./... -v`
**Build command:** `CGO_ENABLED=0 go build ./...`
**Lint command:** `go vet ./...`

## Suitability Assessment

### 1. File Decomposition

The work decomposes into these distinct areas:

1. **Repo extraction from transcript entries** — new parser in `claude/` that scans tool_use/tool_result blocks for file paths and extracts repo roots.
2. **Multi-project data model** — new type `ProjectActivity` and aggregation logic in `claude/`.
3. **Store layer** — new `SessionProjectWeights` store (JSON file, parallel to session-tags.json) for persisted multi-project weights.
4. **MCP tool updates** — updating `resolveProjectName` and project-filtering logic in `mcp/tools.go` and downstream tool handlers to support weighted multi-project attribution.
5. **Session meta enrichment** — enriching `parseJSONLToSessionMeta` to extract repo roots during its single-pass scan.

Files touched span `claude/`, `store/`, and `mcp/` packages — at least 3 agents with disjoint ownership are feasible.

Shared files: `mcp/tools.go` contains `resolveProjectName` and `loadTags` — this is orchestrator-owned for post-merge integration. `claude/types.go` needs a small addition but can be orchestrator-owned.

**Verdict: Decomposable.** 3+ agents with disjoint file ownership.

### 2. Investigation-First Items

None. The data format of JSONL transcripts is well-understood. Tool_use input fields for Read/Edit/Write/Bash contain file paths in documented JSON structures. The `ContentBlock.Input` field is `json.RawMessage` — we can inspect it for path extraction. No unknowns.

### 3. Interface Discoverability

Yes. The core interface is:
- A function that takes a `TranscriptEntry` and returns extracted file paths.
- A function that maps file paths to repo roots (using `git rev-parse --show-toplevel` or path-based heuristic).
- A type `ProjectWeight` that holds `{Project: string, Weight: float64, ToolCalls: int, Tokens: int}`.
- A store that persists `map[sessionID][]ProjectWeight`.

All can be defined before implementation.

### 4. Pre-Implementation Status Check

| Item | Status |
|------|--------|
| Extract file paths from tool_use inputs | **TO-DO** — not implemented anywhere |
| Map file paths to repo roots | **TO-DO** — `countGitCommitsSince` has `isGitRepo()` but no repo-root extraction |
| `ProjectWeight` type | **TO-DO** |
| Multi-project weight store | **TO-DO** — `SessionTagStore` only stores `string` (single project name) |
| `resolveProjectName` multi-project support | **TO-DO** — currently returns a single string |
| MCP tools reading multi-project weights | **TO-DO** |
| `parseJSONLToSessionMeta` repo extraction | **TO-DO** |

### 5. Parallelization Value Check

Sequential estimate: ~3-4 hours (parser + store + MCP integration).
SAW estimate: ~1.5-2 hours (3 agents in 2 waves).
**Verdict: Worthwhile.** ~50% time savings.

---

**OVERALL VERDICT: SUITABLE FOR SAW.** Proceed with full IMPL doc.

---

## Wave Graph

```
Scaffold Agent (Wave 1 scaffold)
  └─ creates: claude/repo_extract.go (ProjectWeight stub)
              ↓
┌─────────────────────────────────────────────────────┐
│  WAVE 1 (parallel)                                  │
│                                                     │
│  Agent A                    Agent B                 │
│  claude/repo_extract.go     store/project_weights   │
│  (replaces stub)            (new store)             │
│  ExtractFilePaths()         SessionProjectWeights   │
│  ResolveRepoRoot()          Store{Load,Set,Get}      │
│  ComputeProjectWeights()                            │
└───────────┬─────────────────────────┬───────────────┘
            │     (both merged)       │
            ▼                         ▼
┌─────────────────────────────────────────────────────┐
│  WAVE 2 (single agent)                              │
│                                                     │
│  Agent A                                            │
│  mcp/multiproject_tools.go                          │
│  get_session_projects MCP tool                      │
│  (reads transcript → computes weights on demand)    │
└─────────────────────────┬───────────────────────────┘
                          │ (merged)
                          ▼
Scaffold Agent (Wave 3 scaffold)
  └─ creates: mcp/project_filter.go
              (sessionMatchesProject, sessionPrimaryProject)
              ↓
┌─────────────────────────────────────────────────────┐
│  WAVE 3 (parallel)                                  │
│                                                     │
│  Agent A                    Agent B                 │
│  health_tools.go            project_tools.go        │
│  anomaly_tools.go           cost_tools.go           │
│  regression_tools.go        (group by primary       │
│  project_filter_test.go      project via weights)   │
│  (filter by project                                 │
│   via weights)                                      │
└───────────┬─────────────────────────┬───────────────┘
            │     (both merged)       │
            ▼                         ▼
Orchestrator post-merge
  └─ jsonrpc.go: add weightsStorePath field
  └─ tools.go: register tools, add helpers
  └─ go build && go test ./...
  └─ install binary
```

## Dependency Graph

```
claude/repo_extract.go (new)  ──────────────────────────┐
    extracts file paths from TranscriptEntry              │
    maps paths to repo roots                              │
    aggregates into []ProjectWeight                       │
                                                          ▼
claude/types.go (mod: orchestrator)  ◄── adds ProjectWeight type
                                                          │
store/project_weights.go (new)  ◄─────────────────────────┤
    persists map[sessionID][]ProjectWeight                │
    parallel to session-tags.json                         │
                                                          ▼
mcp/multiproject_tools.go (new)  ◄────────────────────────┘
    new MCP tool: get_session_projects
    helper: resolveProjectWeights()

mcp/tools.go (mod: orchestrator)
    update resolveProjectName to use weights as fallback
    update loadProjectWeights helper
```

## Interface Contracts

### Package `claude` — `repo_extract.go`

```go
// ProjectWeight represents a single project's share of activity within a session.
type ProjectWeight struct {
    Project   string  `json:"project"`    // repo base name (e.g. "claudewatch")
    RepoRoot  string  `json:"repo_root"`  // absolute path to repo root
    Weight    float64 `json:"weight"`     // fraction of total activity [0.0, 1.0]
    ToolCalls int     `json:"tool_calls"` // number of tool calls touching this repo
}

// ExtractFilePaths returns deduplicated absolute file paths found in the
// tool_use input fields of a transcript entry's assistant message content blocks.
// Inspects Read, Edit, Write, Bash, Glob, Grep, NotebookEdit tool inputs.
// Returns nil for non-assistant entries or entries with no file path tool calls.
func ExtractFilePaths(entry TranscriptEntry) []string

// ResolveRepoRoot returns the git repository root for the given file path,
// or the path itself if it is not inside a git repository.
// Uses a cache to avoid repeated git calls for paths under the same repo.
// Falls back to walking parent directories for .git if git is unavailable.
func ResolveRepoRoot(filePath string) string

// ComputeProjectWeights aggregates file paths extracted from all entries in a
// session transcript into weighted project attribution. Each project's weight
// is the fraction of total tool calls that touched files in that repo.
// Returns nil if no file paths were found.
func ComputeProjectWeights(entries []TranscriptEntry, fallbackProject string) []ProjectWeight
```

### Package `store` — `project_weights.go`

```go
// SessionProjectWeightsStore reads and writes per-session multi-project weight data.
// Backed by a JSON file: {"session_id": [{"project":"x","weight":0.6,...}, ...]}.
// Concurrent-safe for single-process use.
type SessionProjectWeightsStore struct {
    // unexported fields
}

// NewSessionProjectWeightsStore returns a store backed by the given file path.
func NewSessionProjectWeightsStore(path string) *SessionProjectWeightsStore

// Load reads all weights from disk.
// Returns an empty non-nil map if the file does not exist.
func (s *SessionProjectWeightsStore) Load() (map[string][]claude.ProjectWeight, error)

// Set writes or updates the project weights for sessionID.
// Creates the file and parent directories if needed. Atomic write.
func (s *SessionProjectWeightsStore) Set(sessionID string, weights []claude.ProjectWeight) error

// GetWeights returns the weights for a single session, or nil if not found.
func (s *SessionProjectWeightsStore) GetWeights(sessionID string) ([]claude.ProjectWeight, error)
```

### Package `mcp` — `multiproject_tools.go`

```go
// SessionProjectsResult holds multi-project attribution for a session.
type SessionProjectsResult struct {
    SessionID      string                  `json:"session_id"`
    PrimaryProject string                  `json:"primary_project"`
    Projects       []claude.ProjectWeight  `json:"projects"`
    Live           bool                    `json:"live"`
}

// handleGetSessionProjects returns multi-project attribution for the active
// or most recent session. Computes weights on-the-fly from the transcript
// if not cached in the weights store.
func (s *Server) handleGetSessionProjects(args json.RawMessage) (any, error)
```

### Package `mcp` — `project_filter.go` (Wave 3 scaffold)

```go
// sessionMatchesProject returns true if the session is attributed to the given project.
// Priority: (1) tag override, (2) any weight entry with Project == filter (case-insensitive),
// (3) filepath.Base(projectPath) == filter (case-insensitive).
func sessionMatchesProject(sessionID, projectPath string, tags map[string]string, weights map[string][]store.ProjectWeight, filter string) bool

// sessionPrimaryProject returns the primary project name for a session.
// Priority: (1) tag override, (2) weights[0].Project (highest weight, already sorted desc),
// (3) filepath.Base(projectPath).
func sessionPrimaryProject(sessionID, projectPath string, tags map[string]string, weights map[string][]store.ProjectWeight) string
```

### Package `mcp` — `tools.go` (orchestrator-owned modifications)

```go
// loadProjectWeights loads the session project weights store.
// Returns an empty map on any error.
func (s *Server) loadProjectWeights() map[string][]claude.ProjectWeight

// resolveProjectWeights returns the weighted project list for a session.
// Priority: 1) weights store, 2) tag override (as single 100% weight),
// 3) projectPath base name (as single 100% weight).
func resolveProjectWeights(sessionID, projectPath string, tags map[string]string, weights map[string][]claude.ProjectWeight) []claude.ProjectWeight
```

## Scaffolds

| File | Contents | Import path | Status |
|------|----------|-------------|--------|
| `internal/claude/repo_extract.go` | `ProjectWeight` struct (stub only) | `github.com/blackwell-systems/claudewatch/internal/claude` | committed (ad7457b) |
| `internal/mcp/project_filter.go` | `sessionMatchesProject` + `sessionPrimaryProject` (stub only) | `github.com/blackwell-systems/claudewatch/internal/mcp` | committed (ad7457b — partial: uses local projectWeightRef, store.ProjectWeight pending Wave 1B) |

**Scaffold 1 (`claude/repo_extract.go`):** Installed before Wave 1. Wave 1 Agent A replaces it with the full implementation. Required so all agents can import `claude.ProjectWeight`.

```go
package claude

// ProjectWeight represents a single project's share of activity within a session.
type ProjectWeight struct {
    Project   string  `json:"project"`
    RepoRoot  string  `json:"repo_root"`
    Weight    float64 `json:"weight"`
    ToolCalls int     `json:"tool_calls"`
}
```

**Scaffold 2 (`mcp/project_filter.go`):** Installed before Wave 3. Wave 3 agents call these helpers without implementing them. The Scaffold Agent creates the full implementation from the interface contract below.

```go
package mcp

import "github.com/blackwell-systems/claudewatch/internal/store"

// sessionMatchesProject returns true if the session is attributed to the given project.
// Priority: (1) tag override, (2) multi-project weights (any weight > 0),
// (3) filepath.Base of projectPath. Case-insensitive match.
func sessionMatchesProject(sessionID, projectPath string, tags map[string]string, weights map[string][]store.ProjectWeight, filter string) bool

// sessionPrimaryProject returns the primary project name for a session.
// Priority: (1) tag override, (2) highest-weight project name, (3) filepath.Base of projectPath.
func sessionPrimaryProject(sessionID, projectPath string, tags map[string]string, weights map[string][]store.ProjectWeight) string
```

## File Ownership

| File | Owner | Action |
|------|-------|--------|
| `internal/claude/repo_extract.go` | Wave 1 Agent A | Create (replaces scaffold stub) |
| `internal/claude/repo_extract_test.go` | Wave 1 Agent A | Create |
| `internal/store/project_weights.go` | Wave 1 Agent B | Create |
| `internal/store/project_weights_test.go` | Wave 1 Agent B | Create |
| `internal/mcp/multiproject_tools.go` | Wave 2 Agent A | Create |
| `internal/mcp/multiproject_tools_test.go` | Wave 2 Agent A | Create |
| `internal/mcp/project_filter.go` | Wave 3 Scaffold Agent | Create (replaces scaffold stub) |
| `internal/mcp/project_filter_test.go` | Wave 3 Agent A | Create |
| `internal/mcp/health_tools.go` | Wave 3 Agent A | Modify |
| `internal/mcp/anomaly_tools.go` | Wave 3 Agent A | Modify |
| `internal/mcp/regression_tools.go` | Wave 3 Agent A | Modify |
| `internal/mcp/project_tools.go` | Wave 3 Agent B | Modify |
| `internal/mcp/cost_tools.go` | Wave 3 Agent B | Modify |
| `internal/mcp/tools.go` | Orchestrator | Modify (add loadProjectWeights, register tools, add weightsStorePath field) |
| `internal/mcp/jsonrpc.go` | Orchestrator | Modify (add weightsStorePath to Server struct) |

## Wave Structure

### Wave 1: Foundation (2 agents, parallel)

- **Agent A:** Repo extraction parser (`claude/repo_extract.go`)
- **Agent B:** Project weights store (`store/project_weights.go`)

No dependencies between A and B. Both depend only on existing types.

### Wave 2: MCP Integration (1 agent)

- **Agent A:** Multi-project MCP tool (`mcp/multiproject_tools.go`)

Depends on Wave 1 Agent A (ComputeProjectWeights) and Wave 1 Agent B (SessionProjectWeightsStore).

### Wave 3: Existing Tool Integration (2 agents, parallel)

Before Wave 3 launches, the Scaffold Agent creates `internal/mcp/project_filter.go` with the `sessionMatchesProject` and `sessionPrimaryProject` helpers.

- **Agent A:** Filter tools — `health_tools.go`, `anomaly_tools.go`, `regression_tools.go` + `project_filter_test.go`
- **Agent B:** Group tools — `project_tools.go`, `cost_tools.go`

No cross-dependency between A and B. Both depend on the `project_filter.go` scaffold.

### Orchestrator Post-Merge

- Wire `weightsStorePath` into `Server` struct
- Register `get_session_projects` tool in `addTools`
- Add `loadProjectWeights` and `resolveProjectWeights` helpers to `tools.go`
- Run full test suite

---

## Agent Prompts

### Wave 1 Agent A: Repo Extraction Parser

#### 0. CRITICAL: Isolation Verification (RUN FIRST)
```
cd <worktree-path>
git branch --show-current   # must be your agent branch
pwd                         # must be the worktree, not main
```

#### 1. File Ownership
- `internal/claude/repo_extract.go` (CREATE)
- `internal/claude/repo_extract_test.go` (CREATE)

#### 2. Interfaces You Must Implement
```go
// In internal/claude/repo_extract.go

package claude

import (
    "encoding/json"
    "os/exec"
    "path/filepath"
    "strings"
    "sync"
)

// ProjectWeight represents a single project's share of activity within a session.
type ProjectWeight struct {
    Project   string  `json:"project"`
    RepoRoot  string  `json:"repo_root"`
    Weight    float64 `json:"weight"`
    ToolCalls int     `json:"tool_calls"`
}

// ExtractFilePaths returns deduplicated absolute file paths found in the
// tool_use input fields of a transcript entry's assistant message content blocks.
// Inspects Read, Edit, Write, Bash, Glob, Grep, NotebookEdit tool inputs.
// Returns nil for non-assistant entries or entries with no file path tool calls.
func ExtractFilePaths(entry TranscriptEntry) []string

// ResolveRepoRoot returns the git repository root for the given file path,
// or the path itself if it is not inside a git repository.
// Uses a process-scoped cache to avoid repeated git calls for paths in the same repo.
// Falls back to walking parent directories looking for .git/ if git is unavailable.
func ResolveRepoRoot(filePath string) string

// ComputeProjectWeights aggregates file paths extracted from all entries in a
// session transcript into weighted project attribution. Each project's weight
// is the fraction of total tool calls that touched files in that repo.
// fallbackProject is used as the sole project (weight 1.0) if no file paths
// are extracted. Returns a non-nil slice sorted by Weight descending.
func ComputeProjectWeights(entries []TranscriptEntry, fallbackProject string) []ProjectWeight
```

#### 3. Interfaces You May Call
- `claude.TranscriptEntry` (existing type in `transcripts.go`)
- `claude.AssistantMessage`, `claude.ContentBlock` (existing types)

#### 4. What to Implement

**ExtractFilePaths:**
- Parse the entry. Only process `entry.Type == "assistant"`.
- Unmarshal `entry.Message` into `AssistantMessage`.
- For each `ContentBlock` with `Type == "tool_use"`, inspect `block.Input`:
  - **Read:** extract `file_path` (string field)
  - **Edit:** extract `file_path` (string field)
  - **Write:** extract `file_path` (string field)
  - **NotebookEdit:** extract `notebook_path` (string field)
  - **Glob:** extract `path` (string field, optional — may be empty)
  - **Grep:** extract `path` (string field, optional)
  - **Bash:** extract `command` field, scan for absolute paths using a heuristic (paths starting with `/` that don't look like flags). Keep this simple — regex for `/[a-zA-Z0-9._/-]+` tokens in the command string, filter to those that contain at least one `/` after the leading slash.
- Deduplicate and return only absolute paths (starting with `/`).

**ResolveRepoRoot:**
- Maintain a package-level `sync.Map` cache: `repoRootCache map[string]string` keyed by directory path.
- For a file path, take `filepath.Dir(filePath)`.
- Check cache for this dir and all parent dirs up to `/`.
- If cache miss, run `git -C <dir> rev-parse --show-toplevel` with a 2-second timeout.
- On success, cache the result for the dir. On failure (not a git repo, git unavailable), cache the dir itself as its own "root".
- Return `filepath.Base(repoRoot)` is NOT what this function does — return the full absolute repo root path. The `Project` field (base name) is computed by the caller.

**ComputeProjectWeights:**
- Call `ExtractFilePaths` on each entry.
- For each path, call `ResolveRepoRoot` to get the repo root.
- Count tool calls per unique repo root.
- Compute weight as `toolCalls[repo] / totalToolCalls`.
- Build `[]ProjectWeight` with `Project: filepath.Base(repoRoot)`.
- Sort by Weight descending.
- If no paths found, return `[]ProjectWeight{{Project: filepath.Base(fallbackProject), RepoRoot: fallbackProject, Weight: 1.0, ToolCalls: 0}}`.

#### 5. Tests to Write
In `internal/claude/repo_extract_test.go`:

- **TestExtractFilePaths_ReadTool** — entry with a Read tool_use containing `{"file_path":"/Users/x/code/foo/main.go"}` returns `["/Users/x/code/foo/main.go"]`.
- **TestExtractFilePaths_EditTool** — entry with Edit tool_use returns the file_path.
- **TestExtractFilePaths_WriteTool** — entry with Write tool_use returns the file_path.
- **TestExtractFilePaths_BashTool** — entry with Bash tool_use containing `{"command":"cat /Users/x/code/bar/README.md"}` extracts the path.
- **TestExtractFilePaths_NonAssistant** — user-type entry returns nil.
- **TestExtractFilePaths_NoToolUse** — assistant entry with only text returns nil.
- **TestExtractFilePaths_Deduplication** — multiple tool_uses with same path returns one entry.
- **TestComputeProjectWeights_MultiRepo** — entries touching 2 repos returns 2 weights summing to 1.0.
- **TestComputeProjectWeights_SingleRepo** — entries all in one repo returns weight 1.0.
- **TestComputeProjectWeights_Fallback** — no file paths returns fallback project with weight 1.0.
- **TestResolveRepoRoot_Cache** — calling twice with same dir only runs git once (verify via counting).

**Note:** For `ResolveRepoRoot` tests, create a temp dir with `git init` inside the test to avoid depending on external repos. Use `t.TempDir()`.

#### 6. Verification Gate
```bash
cd <worktree-path>
go build ./internal/claude/...
go vet ./internal/claude/...
go test ./internal/claude/... -v -run TestExtract -run TestCompute -run TestResolve
```

#### 7. Constraints
- Do NOT modify any existing files. Only create `repo_extract.go` and `repo_extract_test.go`.
- Do NOT import `internal/mcp` or `internal/store` or `internal/app`.
- The `ProjectWeight` type MUST be defined in `repo_extract.go` (not `types.go`) — the orchestrator will move it if needed.
- `ResolveRepoRoot` must handle `git` not being installed (fall back to `.git` directory walk).
- `ResolveRepoRoot` must use a context timeout of 2 seconds max per git call.
- Bash path extraction should be best-effort, not perfect. Do not attempt to parse shell syntax.
- All result slices must be `[]T{}` (not nil) for clean JSON marshaling.

#### 8. Report
```yaml
status: complete | partial | blocked
files_created:
  - internal/claude/repo_extract.go
  - internal/claude/repo_extract_test.go
tests_passed: <number>
tests_failed: <number>
notes: <free text>
```

---

### Wave 1 Agent B: Project Weights Store

#### 0. CRITICAL: Isolation Verification (RUN FIRST)
```
cd <worktree-path>
git branch --show-current   # must be your agent branch
pwd                         # must be the worktree, not main
```

#### 1. File Ownership
- `internal/store/project_weights.go` (CREATE)
- `internal/store/project_weights_test.go` (CREATE)

#### 2. Interfaces You Must Implement
```go
// In internal/store/project_weights.go

package store

import (
    "encoding/json"
    "os"
    "path/filepath"
    "sync"
)

// ProjectWeight is a local copy of the type to avoid importing claude package.
// Fields must match claude.ProjectWeight exactly for JSON compatibility.
type ProjectWeight struct {
    Project   string  `json:"project"`
    RepoRoot  string  `json:"repo_root"`
    Weight    float64 `json:"weight"`
    ToolCalls int     `json:"tool_calls"`
}

// SessionProjectWeightsStore reads and writes per-session multi-project weight data.
// Backed by a JSON file: {"session_id": [ProjectWeight, ...]}.
// Concurrent-safe for single-process use (Set holds a mutex).
type SessionProjectWeightsStore struct {
    path string
    mu   sync.Mutex
}

// NewSessionProjectWeightsStore returns a store backed by the given file path.
// The file need not exist yet; Load returns an empty map if absent.
func NewSessionProjectWeightsStore(path string) *SessionProjectWeightsStore

// Load reads all weights from disk.
// Returns an empty non-nil map if the file does not exist.
// Returns an error only for I/O or JSON parse failures on an existing file.
func (s *SessionProjectWeightsStore) Load() (map[string][]ProjectWeight, error)

// Set writes or updates the project weights for sessionID.
// Creates the file and any parent directories if they do not exist.
// Reads current data, merges, and writes atomically (write-to-temp + rename).
func (s *SessionProjectWeightsStore) Set(sessionID string, weights []ProjectWeight) error

// GetWeights returns the weights for a single session, or nil if not found.
func (s *SessionProjectWeightsStore) GetWeights(sessionID string) ([]ProjectWeight, error)
```

#### 3. Interfaces You May Call
- Standard library only (`encoding/json`, `os`, `path/filepath`, `sync`).

#### 4. What to Implement

This is structurally identical to the existing `SessionTagStore` in `store/tags.go`. Follow the same patterns:
- JSON file backed, lazy-create, atomic write via temp file + rename.
- `Load()` returns `map[string][]ProjectWeight{}` on file-not-found.
- `Set()` acquires mutex, calls `Load()`, merges entry, marshals with `json.MarshalIndent`, writes to temp, renames.
- `GetWeights()` calls `Load()`, returns the slice for the given session ID or nil.

**Important:** Define `ProjectWeight` locally in this file (not importing `claude` package) to match the existing pattern where `store` does not import `claude`. The fields are identical — JSON serialization ensures compatibility.

#### 5. Tests to Write
In `internal/store/project_weights_test.go`:

- **TestSessionProjectWeightsStore_LoadEmpty** — Load on non-existent file returns empty map, no error.
- **TestSessionProjectWeightsStore_SetAndLoad** — Set weights for a session, Load returns them.
- **TestSessionProjectWeightsStore_SetMultipleSessions** — Set weights for 2 sessions, Load returns both.
- **TestSessionProjectWeightsStore_SetOverwrite** — Set weights twice for same session, second overwrites first.
- **TestSessionProjectWeightsStore_GetWeights** — GetWeights returns correct data for existing session.
- **TestSessionProjectWeightsStore_GetWeightsNotFound** — GetWeights returns nil for unknown session.
- **TestSessionProjectWeightsStore_AtomicWrite** — Verify file exists after Set (no partial writes).
- **TestSessionProjectWeightsStore_CreatesParentDirs** — Set creates parent directories if missing.

Use `t.TempDir()` for all tests. No external dependencies.

#### 6. Verification Gate
```bash
cd <worktree-path>
go build ./internal/store/...
go vet ./internal/store/...
go test ./internal/store/... -v -run TestSessionProjectWeights
```

#### 7. Constraints
- Do NOT modify any existing files. Only create `project_weights.go` and `project_weights_test.go`.
- Do NOT import `internal/claude`, `internal/mcp`, or `internal/app`.
- Follow the exact same atomic-write pattern as `SessionTagStore` in `tags.go`.
- The `ProjectWeight` struct must have identical JSON tags to the one defined in `claude/repo_extract.go`.
- Result maps/slices must never be nil — always return `map[string][]ProjectWeight{}` or `[]ProjectWeight{}`.

#### 8. Report
```yaml
status: complete | partial | blocked
files_created:
  - internal/store/project_weights.go
  - internal/store/project_weights_test.go
tests_passed: <number>
tests_failed: <number>
notes: <free text>
```

---

### Wave 2 Agent A: Multi-Project MCP Tool

#### 0. CRITICAL: Isolation Verification (RUN FIRST)
```
cd <worktree-path>
git branch --show-current   # must be your agent branch
pwd                         # must be the worktree, not main
```

#### 1. File Ownership
- `internal/mcp/multiproject_tools.go` (CREATE)
- `internal/mcp/multiproject_tools_test.go` (CREATE)

#### 2. Interfaces You Must Implement
```go
// In internal/mcp/multiproject_tools.go

package mcp

import (
    "encoding/json"
    "sort"

    "github.com/blackwell-systems/claudewatch/internal/claude"
    "github.com/blackwell-systems/claudewatch/internal/store"
)

// SessionProjectsResult holds multi-project attribution for a session.
type SessionProjectsResult struct {
    SessionID      string                  `json:"session_id"`
    PrimaryProject string                  `json:"primary_project"`
    Projects       []claude.ProjectWeight  `json:"projects"`
    Live           bool                    `json:"live"`
}

// addMultiProjectTools registers the get_session_projects tool on s.
func addMultiProjectTools(s *Server)

// handleGetSessionProjects returns multi-project attribution for the active
// or most recent session. Computes weights on-the-fly from the transcript
// if not cached in the weights store.
func (s *Server) handleGetSessionProjects(args json.RawMessage) (any, error)
```

#### 3. Interfaces You May Call
From Wave 1 Agent A (`claude/repo_extract.go`):
```go
func claude.ExtractFilePaths(entry claude.TranscriptEntry) []string
func claude.ResolveRepoRoot(filePath string) string
func claude.ComputeProjectWeights(entries []claude.TranscriptEntry, fallbackProject string) []claude.ProjectWeight
```

From Wave 1 Agent B (`store/project_weights.go`):
```go
func store.NewSessionProjectWeightsStore(path string) *store.SessionProjectWeightsStore
func (s *store.SessionProjectWeightsStore) Load() (map[string][]store.ProjectWeight, error)
func (s *store.SessionProjectWeightsStore) Set(sessionID string, weights []store.ProjectWeight) error
func (s *store.SessionProjectWeightsStore) GetWeights(sessionID string) ([]store.ProjectWeight, error)
```

From existing code:
```go
func claude.FindActiveSessionPath(claudeHome string) (string, error)
func claude.ParseActiveSession(path string) (*claude.SessionMeta, error)
func claude.ParseAllSessionMeta(claudeHome string) ([]claude.SessionMeta, error)
// readLiveJSONL is unexported — use the public functions or re-read the file
```

#### 4. What to Implement

**addMultiProjectTools:**
- Register a single tool `get_session_projects` with description: "Multi-project attribution for a session. Returns weighted project breakdown showing which repos were touched and how much activity each received."
- Input schema: `{"type":"object","properties":{"session_id":{"type":"string","description":"Session ID to analyze (optional, defaults to active/most recent)"}},"additionalProperties":false}`

**handleGetSessionProjects:**
1. Parse optional `session_id` argument.
2. If `session_id` is empty, find the active session via `FindActiveSessionPath` + `ParseActiveSession`. If no active session, fall back to most recent from `ParseAllSessionMeta`.
3. Check the weights store (`s.weightsStorePath` — see constraints) for cached weights.
4. If cached, return them directly.
5. If not cached, read the JSONL transcript file for the session:
   - Use `store.findTranscriptFile(sessionID, s.claudeHome)` — but this is unexported. Instead, locate the file by walking `projects/<hash>/<sessionID>.jsonl` (same pattern as `ParseSingleTranscript`).
   - Actually, read the transcript entries using the same approach as `readLiveJSONL` (read file, scan JSONL lines, unmarshal TranscriptEntry).
   - Call `ComputeProjectWeights(entries, meta.ProjectPath)`.
6. Cache the computed weights via the store.
7. Return `SessionProjectsResult` with `PrimaryProject` set to the highest-weight project.

**Important:** Since `readLiveJSONL` is unexported, you need to implement a local helper `readTranscriptEntries(path string) ([]claude.TranscriptEntry, error)` that reads and parses the JSONL file. Follow the same pattern: ReadFile, truncate at last newline, scan with 10MB buffer, skip malformed lines.

**Note on weightsStorePath:** The `Server` struct does not yet have a `weightsStorePath` field. For now, derive it as `filepath.Join(filepath.Dir(s.tagStorePath), "session-project-weights.json")`. The orchestrator will add the proper field during post-merge.

#### 5. Tests to Write
In `internal/mcp/multiproject_tools_test.go`:

- **TestHandleGetSessionProjects_SingleProject** — write a JSONL transcript with tool_use entries all referencing files in one repo. Verify result has 1 project with weight 1.0.
- **TestHandleGetSessionProjects_MultiProject** — write a JSONL transcript with tool_use entries referencing files in 2 different directories. Verify 2 projects returned with weights summing to 1.0.
- **TestHandleGetSessionProjects_NoFilePaths** — write a JSONL transcript with only text entries (no tool_use). Verify fallback to project path with weight 1.0.
- **TestHandleGetSessionProjects_CachedWeights** — pre-populate the weights store, verify the handler returns cached data without needing a JSONL file.

Use the existing test helpers from `tools_test.go`: `newTestServer`, `writeSessionMeta`. Write JSONL transcript files using `writeTranscriptJSONL` helper or create new ones.

**Important:** Since these tests call `ResolveRepoRoot` which runs `git`, the test transcript entries should use paths under `t.TempDir()` where you can `git init` to control the repo root. Alternatively, use paths that won't resolve to git repos — the fallback behavior (path is its own root) is acceptable for testing weight distribution.

#### 6. Verification Gate
```bash
cd <worktree-path>
go build ./internal/mcp/...
go vet ./internal/mcp/...
go test ./internal/mcp/... -v -run TestHandleGetSessionProjects
```

#### 7. Constraints
- Do NOT modify any existing files. Only create `multiproject_tools.go` and `multiproject_tools_test.go`.
- The tool must NOT be registered in `addTools` — that is orchestrator-owned. Instead, export `addMultiProjectTools(s *Server)` for the orchestrator to call.
- Derive `weightsStorePath` from `s.tagStorePath` (same directory, different filename) until orchestrator adds the field.
- Convert between `store.ProjectWeight` and `claude.ProjectWeight` as needed — fields are identical, just different packages. Use JSON round-trip or manual field copy.
- Data loading is non-fatal: on error, return zero-value result, not error.
- Result slices must be `[]T{}` (not nil) for clean JSON.

#### 8. Report
```yaml
status: complete | partial | blocked
files_created:
  - internal/mcp/multiproject_tools.go
  - internal/mcp/multiproject_tools_test.go
tests_passed: <number>
tests_failed: <number>
notes: <free text>
```

---

---

### Wave 3 Agent A: Filter Tool Integration

#### 0. CRITICAL: Isolation Verification (RUN FIRST)
```
cd <worktree-path>
git branch --show-current   # must be wave3-agent-A
pwd                         # must be the worktree, not main
```

#### 1. File Ownership
- `internal/mcp/health_tools.go` (MODIFY)
- `internal/mcp/anomaly_tools.go` (MODIFY)
- `internal/mcp/regression_tools.go` (MODIFY)
- `internal/mcp/project_filter_test.go` (CREATE)

#### 2. Interfaces You Must Implement
No new interfaces. Modify existing handlers to use `sessionMatchesProject` and `sessionPrimaryProject`.

#### 3. Interfaces You May Call
From `project_filter.go` scaffold (committed to HEAD before this wave):
```go
func sessionMatchesProject(sessionID, projectPath string, tags map[string]string, weights map[string][]store.ProjectWeight, filter string) bool
func sessionPrimaryProject(sessionID, projectPath string, tags map[string]string, weights map[string][]store.ProjectWeight) string
```

From `store/project_weights.go` (Wave 1 Agent B):
```go
func store.NewSessionProjectWeightsStore(path string) *store.SessionProjectWeightsStore
func (s *store.SessionProjectWeightsStore) Load() (map[string][]store.ProjectWeight, error)
```

Existing:
```go
func (s *Server) loadTags() map[string]string   // already in tools.go
```

#### 4. What to Implement

In all three files (`health_tools.go`, `anomaly_tools.go`, `regression_tools.go`), the change pattern is identical:

1. **Load weights** at the start of the handler, after loading tags:
   ```go
   allWeights := s.loadAllProjectWeights()  // new helper — see below
   ```

2. **Replace session filter loop** from:
   ```go
   if resolveProjectName(sess.SessionID, sess.ProjectPath, tags) == project {
   ```
   to:
   ```go
   if sessionMatchesProject(sess.SessionID, sess.ProjectPath, tags, allWeights[sess.SessionID], project) {
   ```

3. **Replace project name resolution** from:
   ```go
   project = resolveProjectName(meta.SessionID, meta.ProjectPath, tags)
   ```
   to:
   ```go
   project = sessionPrimaryProject(meta.SessionID, meta.ProjectPath, tags, allWeights[meta.SessionID])
   ```

The `loadAllProjectWeights` helper loads all weights keyed by sessionID. Add it to each file locally (small enough to inline) or call `store.NewSessionProjectWeightsStore` directly:
```go
func loadAllWeights(weightsPath string) map[string][]store.ProjectWeight {
    ws := store.NewSessionProjectWeightsStore(weightsPath)
    m, err := ws.Load()
    if err != nil || m == nil {
        return map[string][]store.ProjectWeight{}
    }
    return m
}
```

The weights path is `filepath.Join(filepath.Dir(s.tagStorePath), "session-project-weights.json")` until the orchestrator adds `s.weightsStorePath`.

#### 5. Tests to Write
In `internal/mcp/project_filter_test.go`:
- **TestSessionMatchesProject_TagOverride** — tag set to "foo", filter "foo" → true
- **TestSessionMatchesProject_WeightMatch** — no tag, weights include "bar" → filter "bar" → true
- **TestSessionMatchesProject_PathFallback** — no tag, no weights, projectPath "/home/x/code/baz" → filter "baz" → true
- **TestSessionMatchesProject_NoMatch** — none of the above match → false
- **TestSessionPrimaryProject_TagOverride** — returns tag value
- **TestSessionPrimaryProject_WeightFirst** — no tag, weights present, returns highest-weight project
- **TestSessionPrimaryProject_PathFallback** — no tag, no weights → returns filepath.Base(projectPath)

#### 6. Verification Gate
```bash
cd <worktree-path>
go build ./internal/mcp/...
go vet ./internal/mcp/...
go test ./internal/mcp/... -v -run TestSessionMatchesProject -run TestSessionPrimaryProject -run TestHandleGetProjectHealth -run TestHandleGetProjectAnomalies -run TestHandleGetRegression
```

#### 7. Constraints
- Do NOT modify `tools.go`, `jsonrpc.go`, `project_filter.go`, `project_tools.go`, or `cost_tools.go`.
- Do NOT change function signatures of existing handlers.
- The filter must be case-insensitive: `strings.EqualFold`.
- If the weights store fails to load, fall back to existing behavior (filepath.Base match). Non-fatal.
- For the default project resolution (when no filter arg provided), `sessionPrimaryProject` replaces `resolveProjectName`.

#### 8. Report
```yaml
status: complete | partial | blocked
files_changed:
  - internal/mcp/health_tools.go
  - internal/mcp/anomaly_tools.go
  - internal/mcp/regression_tools.go
files_created:
  - internal/mcp/project_filter_test.go
tests_passed: <number>
tests_failed: <number>
notes: <free text>
```

---

### Wave 3 Agent B: Group Tool Integration

#### 0. CRITICAL: Isolation Verification (RUN FIRST)
```
cd <worktree-path>
git branch --show-current   # must be wave3-agent-B
pwd                         # must be the worktree, not main
```

#### 1. File Ownership
- `internal/mcp/project_tools.go` (MODIFY)
- `internal/mcp/cost_tools.go` (MODIFY)

#### 2. Interfaces You Must Implement
No new interfaces. Modify existing grouping logic to use `sessionPrimaryProject`.

#### 3. Interfaces You May Call
From `project_filter.go` scaffold:
```go
func sessionPrimaryProject(sessionID, projectPath string, tags map[string]string, weights map[string][]store.ProjectWeight) string
```

From `store/project_weights.go` (Wave 1 Agent B):
```go
func store.NewSessionProjectWeightsStore(path string) *store.SessionProjectWeightsStore
func (s *store.SessionProjectWeightsStore) Load() (map[string][]store.ProjectWeight, error)
```

#### 4. What to Implement

**`project_tools.go`** — `handleGetProjectComparison`:
- After loading tags, load weights map.
- Replace `resolveProjectName(sess.SessionID, sess.ProjectPath, tags)` with `sessionPrimaryProject(sess.SessionID, sess.ProjectPath, tags, allWeights[sess.SessionID])`.
- This correctly groups cross-repo sessions under their primary project.

**`cost_tools.go`** — `handleGetCostSummary`:
- `cost_tools.go` currently uses `filepath.Base(session.ProjectPath)` directly (doesn't call `resolveProjectName`). This is the most broken.
- After loading tags, load weights map.
- Replace `filepath.Base(session.ProjectPath)` with `sessionPrimaryProject(session.SessionID, session.ProjectPath, tags, allWeights[session.SessionID])`.

Use the same `loadAllWeights` inline helper as Agent A:
```go
func loadAllWeights(weightsPath string) map[string][]store.ProjectWeight {
    ws := store.NewSessionProjectWeightsStore(weightsPath)
    m, err := ws.Load()
    if err != nil || m == nil {
        return map[string][]store.ProjectWeight{}
    }
    return m
}
```

Weights path: `filepath.Join(filepath.Dir(s.tagStorePath), "session-project-weights.json")`.

#### 5. Tests to Write
No new test files. Verify existing tests still pass. The behavioral change is transparent to existing tests since old single-project sessions have no weights and fall back to `filepath.Base` (identical to previous behavior).

If existing tests use mock servers with `tagStorePath` set, ensure the weights path derivation doesn't panic (it won't — it just returns an empty map if the file doesn't exist).

#### 6. Verification Gate
```bash
cd <worktree-path>
go build ./internal/mcp/...
go vet ./internal/mcp/...
go test ./internal/mcp/... -v -run TestHandleGetProjectComparison -run TestHandleGetCostSummary
```

#### 7. Constraints
- Do NOT modify `tools.go`, `jsonrpc.go`, `project_filter.go`, `health_tools.go`, `anomaly_tools.go`, or `regression_tools.go`.
- Case-insensitive project name matching (`strings.EqualFold`) in `sessionPrimaryProject`.
- Non-fatal weights load — fall back to `filepath.Base` on error.
- Do not change the shape of result types.

#### 8. Report
```yaml
status: complete | partial | blocked
files_changed:
  - internal/mcp/project_tools.go
  - internal/mcp/cost_tools.go
tests_passed: <number>
tests_failed: <number>
notes: <free text>
```

---

## Wave Execution Loop

### Pre-Wave: Orchestrator Scaffold

Before launching Wave 1, the orchestrator writes the scaffold file:

```
internal/claude/repo_extract.go (stub with just ProjectWeight type)
```

This ensures both Wave 1 agents can compile against the type.

### Wave 1: Launch Agents A and B in parallel

Both agents create new files only. No shared file conflicts.

**Merge order:** Either order works — no compile dependency between them.

**Verification after merge:**
```bash
go build ./...
go test ./internal/claude/... -v
go test ./internal/store/... -v
```

### Wave 2: Launch Agent A

Depends on both Wave 1 outputs being merged.

**Verification after merge:**
```bash
go build ./...
go test ./internal/mcp/... -v -run TestHandleGetSessionProjects
```

### Post-Wave: Orchestrator Integration

See checklist below.

---

## Orchestrator Post-Merge Checklist

After all waves are merged, the orchestrator performs these integration steps:

1. **Add `weightsStorePath` to `Server` struct** in `internal/mcp/jsonrpc.go`:
   ```go
   type Server struct {
       tools            []toolDef
       claudeHome       string
       budgetUSD        float64
       tagStorePath     string
       weightsStorePath string  // NEW
   }
   ```

2. **Initialize `weightsStorePath` in `NewServer`** in `internal/mcp/jsonrpc.go`:
   ```go
   func NewServer(cfg *config.Config, budgetUSD float64) *Server {
       s := &Server{
           claudeHome:       cfg.ClaudeHome,
           budgetUSD:        budgetUSD,
           tagStorePath:     filepath.Join(config.ConfigDir(), "session-tags.json"),
           weightsStorePath: filepath.Join(config.ConfigDir(), "session-project-weights.json"),
       }
       addTools(s)
       return s
   }
   ```

3. **Register the tool** in `addTools` in `internal/mcp/tools.go`:
   ```go
   addMultiProjectTools(s)
   ```

4. **Add `loadProjectWeights` helper** to `internal/mcp/tools.go`:
   ```go
   func (s *Server) loadProjectWeights() map[string][]store.ProjectWeight {
       ws := store.NewSessionProjectWeightsStore(s.weightsStorePath)
       weights, err := ws.Load()
       if err != nil || weights == nil {
           return map[string][]store.ProjectWeight{}
       }
       return weights
   }
   ```

5. **Run full verification:**
   ```bash
   go build ./...
   go vet ./...
   go test ./... -v
   ```

6. **Install updated binary:**
   ```bash
   go build -o /opt/homebrew/bin/claudewatch ./cmd/claudewatch
   ```

---

## Known Issues

1. **store.ProjectWeight vs claude.ProjectWeight:** Two identical structs exist to avoid an import cycle (`store` cannot import `claude`). This is consistent with the existing pattern (`store.ModelPricing` vs `analyzer.ModelPricing`). JSON round-trip or manual field copy bridges them.

2. **ResolveRepoRoot shells out to git:** This adds latency. The sync.Map cache mitigates this — after the first call per directory tree, subsequent calls are cached. For transcripts with hundreds of tool calls, the cache hit rate should be >95%.

3. **Bash path extraction is heuristic:** Regex-based path extraction from shell commands will miss some paths (piped commands, variable expansion) and may false-positive on flag arguments that look like paths. This is acceptable for attribution weighting — precision matters less than coverage.

4. **Live session weight computation:** For active sessions, the JSONL file is being written to concurrently. The line-atomic truncation pattern (truncate at last newline) handles this safely, same as `ParseActiveSession`.

5. **Cost tool does not use weights yet:** `handleGetCostSummary` still uses `filepath.Base(session.ProjectPath)` for project grouping. Updating it to use weights is a follow-up task, not part of this feature scope.

---

## Status

| Wave | Agent | Description | Status |
|------|-------|-------------|--------|
| 1 | A | Repo extraction parser (`claude/repo_extract.go`) | TO-DO |
| 1 | B | Project weights store (`store/project_weights.go`) | TO-DO |
| 2 | A | Multi-project MCP tool (`mcp/multiproject_tools.go`) | TO-DO |
| 3 | A | Filter tool integration (health, anomaly, regression) | TO-DO |
| 3 | B | Group tool integration (comparison, cost) | TO-DO |
| — | Orch | Post-merge integration + binary install | TO-DO |
