# IMPL: Explicit Session Project Tagging

**Feature:** Allow Claude (via MCP) or the developer (via CLI) to explicitly override the project name attributed to a session. Fixes wrong attribution when Claude Code is launched from one directory but work happens in another (SAW worktrees, subprojects, etc.).

**Root cause:** Every claudewatch tool that surfaces a session's project name calls `filepath.Base(session.ProjectPath)`, where `ProjectPath` is the Claude Code *launch* directory — recorded at session start and immutable. When SAW worktrees are used (launched from `~/code/gsm`, working in `~/code/brewprune`), all metrics are indexed under "gsm."

**Fix:** A lightweight lookup table (`~/.config/claudewatch/session-tags.json`) maps `session_id → project_name`. All project-name resolution points check this table first. The table is written by a new MCP tool (`set_session_project`) and a new CLI command (`claudewatch tag`).

---

## Suitability Assessment

**Verdict: SUITABLE**

The work decomposes cleanly into three disjoint file groups across two waves. Wave 1 builds the pure data layer (`internal/store/tags.go`) with no dependencies on new code. Wave 2 runs two agents in parallel: Agent B owns the entire `internal/mcp` package (tag tool + handler updates) and Agent C owns the new `internal/app/tag.go` CLI command. No file is touched by more than one agent. The store interface is fully specifiable before implementation. Build cycles are moderate (`go test ./...` ~5–15s), and Agent B has 6 files — enough implementation mass to justify parallelism.

**Estimated times:**
- Scout phase: ~25 min (this document)
- Agent execution: Wave 1 ~10 min, Wave 2 ~15 min (2 agents parallel)
- Merge & verification: ~5 min
- Total SAW time: ~55 min

Sequential baseline: ~40 min (waves run sequentially, no parallelism in Wave 2)
Time savings: ~10 min from Wave 2 parallelism + coordination value from IMPL doc as interface spec.

**Recommendation:** Clear speedup at Wave 2. Proceed.

---

## Known Issues

None identified. All tests pass on `main` as of the current HEAD.

---

## Dependency Graph

```
[new] internal/store/tags.go          ← root node; no deps on new code
         ↓
[new] internal/mcp/tag_tools.go       ← depends on store.SessionTagStore
[mod] internal/mcp/tools.go           ← depends on store.SessionTagStore (resolveProjectName)
[mod] internal/mcp/health_tools.go    ← depends on resolveProjectName (in tools.go)
[mod] internal/mcp/project_tools.go   ← depends on resolveProjectName (in tools.go)
[mod] internal/mcp/jsonrpc.go         ← adds tagStorePath field to Server struct

[new] internal/app/tag.go             ← depends on store.SessionTagStore (independent of mcp changes)
```

Agent B and Agent C are independent: they share only the `store.SessionTagStore` interface (delivered by Agent A) and do not touch each other's files.

**Cascade candidates (files not in any agent's scope that reference changed interfaces):**

- `internal/app/mcp.go` — calls `mcp.NewServer(cfg, mcpBudget)`. `NewServer` signature does NOT change (tagStorePath is computed internally from `config.ConfigDir()`). No change needed.
- `internal/mcp/tools_test.go` — tests existing handlers. Some tests verify `ProjectName` values. If any tests hardcode `filepath.Base(projectPath)` behavior, they may need updating. Agent B must check and update these.
- `internal/mcp/saw_tools_test.go` — may test SAW session project name resolution. Agent B checks and updates.

---

## Interface Contracts

### Agent A delivers (Wave 1)

```go
// package store

// SessionTagStore reads and writes session project name overrides.
// Backed by a JSON file containing a flat map: {"session_id": "project_name"}.
// Concurrent-safe for single-process use.
type SessionTagStore struct { /* unexported fields */ }

// NewSessionTagStore returns a store backed by the given file path.
// The file need not exist yet; Load returns an empty map if absent.
func NewSessionTagStore(path string) *SessionTagStore

// Load reads all tags from disk.
// Returns an empty non-nil map if the file does not exist.
// Returns an error only for I/O or JSON parse failures on an existing file.
func (s *SessionTagStore) Load() (map[string]string, error)

// Set writes or updates the project name override for sessionID.
// Creates the file and any parent directories if they do not exist.
// Reads the current tags, merges, and writes atomically (write-to-temp + rename).
func (s *SessionTagStore) Set(sessionID, projectName string) error
```

### Agent B delivers (Wave 2 — mcp package)

```go
// package mcp

// resolveProjectName returns the tagged project name for sessionID if an override
// exists in tags, falling back to filepath.Base(projectPath).
// tags must be pre-loaded via loadTags(). This is a package-level helper (not method).
func resolveProjectName(sessionID, projectPath string, tags map[string]string) string

// loadTags loads the session tag store and returns its contents.
// Returns an empty map on any error (non-fatal: missing tags file is fine).
func (s *Server) loadTags() map[string]string

// handleSetSessionProject implements the set_session_project MCP tool.
// Accepts: {"session_id": string, "project_name": string}
// Returns: {"session_id": string, "project_name": string, "ok": true}
func (s *Server) handleSetSessionProject(args json.RawMessage) (any, error)
```

**Server struct addition (in jsonrpc.go):**
```go
type Server struct {
    tools        []toolDef
    claudeHome   string
    budgetUSD    float64
    tagStorePath string   // NEW: path to session-tags.json
}
```

**NewServer update (in jsonrpc.go):**
```go
func NewServer(cfg *config.Config, budgetUSD float64) *Server {
    s := &Server{
        claudeHome:   cfg.ClaudeHome,
        budgetUSD:    budgetUSD,
        tagStorePath: filepath.Join(config.ConfigDir(), "session-tags.json"), // NEW
    }
    addTools(s)
    return s
}
```

**set_session_project tool schema:**
```json
{
  "type": "object",
  "properties": {
    "session_id": {
      "type": "string",
      "description": "The session ID to tag. Use the session_id from get_session_stats."
    },
    "project_name": {
      "type": "string",
      "description": "The project name to attribute this session to (e.g. 'brewprune')."
    }
  },
  "required": ["session_id", "project_name"],
  "additionalProperties": false
}
```

### Agent C delivers (Wave 2 — app/tag.go)

```go
// package app

// tagCmd is registered with rootCmd as "tag".
// Usage: claudewatch tag --project <name> [--session <id>]
// If --session is omitted, uses the most recent session from session meta.
var tagCmd *cobra.Command
```

---

## File Ownership

| File | Agent | Wave | Action | Depends On |
|------|-------|------|--------|------------|
| `internal/store/tags.go` | A | 1 | create | — |
| `internal/store/tags_test.go` | A | 1 | create | — |
| `internal/mcp/jsonrpc.go` | B | 2 | modify | A: SessionTagStore |
| `internal/mcp/tag_tools.go` | B | 2 | create | A: SessionTagStore |
| `internal/mcp/tag_tools_test.go` | B | 2 | create | A: SessionTagStore |
| `internal/mcp/tools.go` | B | 2 | modify | A: SessionTagStore |
| `internal/mcp/health_tools.go` | B | 2 | modify | B: resolveProjectName |
| `internal/mcp/project_tools.go` | B | 2 | modify | B: resolveProjectName |
| `internal/mcp/tools_test.go` | B | 2 | modify (if needed) | — |
| `internal/mcp/saw_tools_test.go` | B | 2 | modify (if needed) | — |
| `internal/app/tag.go` | C | 2 | create | A: SessionTagStore |

---

## Wave Structure

```
Wave 1: [A]                         ← store data layer (gates Wave 2)
              | (A completes)
Wave 2: [B]         [C]             ← mcp + cli in parallel
        (mcp pkg)   (app/tag.go)
```

Wave 2 is unblocked by Agent A completing. Agents B and C are fully independent.

---

## Agent Prompts

---

### Agent A — Wave 1: Session Tag Store

```
# Wave 1 Agent A: Session Tag Store

You are Wave 1 Agent A. Implement the SessionTagStore data layer in internal/store/tags.go.

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
  echo "ISOLATION FAILURE: Wrong directory (even after cd attempt)"
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

**If verification fails:** Write error to completion report and exit immediately.

## 1. File Ownership

You own these files. Do not touch any other files.
- `internal/store/tags.go` — create
- `internal/store/tags_test.go` — create

## 2. Interfaces You Must Implement

```go
package store

type SessionTagStore struct { /* unexported */ }

func NewSessionTagStore(path string) *SessionTagStore
func (s *SessionTagStore) Load() (map[string]string, error)
func (s *SessionTagStore) Set(sessionID, projectName string) error
```

## 3. Interfaces You May Call

Existing store package patterns only. Look at `internal/store/db.go` for package conventions.

## 4. What to Implement

Implement `SessionTagStore` in `internal/store/tags.go`. This is a simple JSON file backed map: the file contains `{"session_id": "project_name", ...}`.

**NewSessionTagStore(path string):** Store the path. Do not read the file at construction time.

**Load():** Read the file at `s.path`. If the file does not exist (`os.IsNotExist`), return an empty non-nil map and nil error. If the file exists but fails to parse, return nil and the error. Return the parsed map on success.

**Set(sessionID, projectName string):** This must be atomic to prevent corruption from concurrent writes (MCP server and CLI could theoretically race). Implementation:
1. Lock a sync.Mutex on the store.
2. Load current tags (tolerate file-not-found).
3. Set `tags[sessionID] = projectName`.
4. Marshal to JSON (indented for readability).
5. Write to a temp file in the same directory as `s.path` using `os.CreateTemp`.
6. `os.Rename` the temp file to `s.path` (atomic on POSIX).
7. Ensure parent directory exists before writing: `os.MkdirAll(filepath.Dir(s.path), 0o755)`.
8. Unlock.

The store holds a `sync.Mutex` for the Set operation. Load does not hold the mutex (callers in the MCP server are single-goroutine per request).

Read `internal/store/db.go` briefly to understand the package's import style and error handling conventions before writing.

## 5. Tests to Write

In `internal/store/tags_test.go`:

1. `TestNewSessionTagStore_LoadEmpty` — Load from a nonexistent path returns empty map and nil error
2. `TestSessionTagStore_SetAndLoad` — Set a tag, then Load returns it
3. `TestSessionTagStore_SetMultiple` — Set multiple tags, Load returns all
4. `TestSessionTagStore_SetOverwrite` — Set the same session ID twice, Load returns the updated name
5. `TestSessionTagStore_LoadInvalidJSON` — Load from a file with invalid JSON returns an error

Use `t.TempDir()` for all file paths. No mocks needed.

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-A
go build ./...
go vet ./...
go test ./internal/store -run TestSessionTagStore -run TestNewSessionTagStore -v
```

All must pass. If any test panics or fails, fix before reporting.

## 7. Constraints

- Do not import any claudewatch internal packages (this is a leaf package).
- Standard library only (`encoding/json`, `os`, `path/filepath`, `sync`).
- `Set` must be safe for concurrent callers. `Load` need not be.
- Return empty map (not nil) when the file is absent in `Load`.
- Do not add a `Delete` or `Clear` method — not needed for this feature.

## 8. Report

Commit and append your completion report to `docs/IMPL-session-project-tagging.md`:

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave1-agent-A
git add internal/store/tags.go internal/store/tags_test.go
git commit -m "wave1-agent-A: session tag store"
```

### Agent A — Completion Report

```yaml
status: complete | partial | blocked
worktree: .claude/worktrees/wave1-agent-A
commit: {sha}
files_changed: []
files_created:
  - internal/store/tags.go
  - internal/store/tags_test.go
interface_deviations: []
out_of_scope_deps: []
tests_added:
  - TestNewSessionTagStore_LoadEmpty
  - TestSessionTagStore_SetAndLoad
  - TestSessionTagStore_SetMultiple
  - TestSessionTagStore_SetOverwrite
  - TestSessionTagStore_LoadInvalidJSON
verification: PASS | FAIL
```
```

---

### Agent B — Wave 2: MCP Tag Tool + Project Name Resolution

```
# Wave 2 Agent B: MCP Tag Tool and Project Name Resolution

You are Wave 2 Agent B. Add the set_session_project MCP tool and fix project name resolution across all MCP handlers to use session tag overrides.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-B 2>/dev/null || true
```

**Step 2: Verify isolation**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-B"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave2-agent-B"

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

You own these files. Do not touch any other files except as noted below.
- `internal/mcp/jsonrpc.go` — modify (add tagStorePath to Server struct + set in NewServer)
- `internal/mcp/tag_tools.go` — create (set_session_project handler)
- `internal/mcp/tag_tools_test.go` — create (tests for the handler)
- `internal/mcp/tools.go` — modify (register tool, add resolveProjectName, update handlers)
- `internal/mcp/health_tools.go` — modify (use resolveProjectName)
- `internal/mcp/project_tools.go` — modify (use resolveProjectName)
- `internal/mcp/tools_test.go` — modify if tests hardcode old ProjectName behavior
- `internal/mcp/saw_tools_test.go` — modify if tests hardcode old ProjectName behavior

## 2. Interfaces You Must Implement

```go
// In internal/mcp/tools.go (package-level helper):
func resolveProjectName(sessionID, projectPath string, tags map[string]string) string

// In internal/mcp/tools.go (Server method):
func (s *Server) loadTags() map[string]string

// In internal/mcp/tag_tools.go (Server method):
func (s *Server) handleSetSessionProject(args json.RawMessage) (any, error)
```

## 3. Interfaces You May Call

From Wave 1 (Agent A):
```go
// internal/store
func NewSessionTagStore(path string) *store.SessionTagStore
func (s *store.SessionTagStore) Load() (map[string]string, error)
func (s *store.SessionTagStore) Set(sessionID, projectName string) error
```

Existing:
```go
// internal/claude
func ParseAllSessionMeta(claudeHome string) ([]SessionMeta, error)
func ParseAllFacets(claudeHome string) ([]SessionFacet, error)

// internal/config
func ConfigDir() string
```

## 4. What to Implement

### 4a. Server struct + NewServer (jsonrpc.go)

Add `tagStorePath string` to `Server`. In `NewServer`, set:
```go
tagStorePath: filepath.Join(config.ConfigDir(), "session-tags.json"),
```
Add `"path/filepath"` import to jsonrpc.go if not present. `config` is already imported.

### 4b. loadTags + resolveProjectName (tools.go)

Add to `tools.go`:

```go
// loadTags loads the session tag override store. Returns empty map on error.
func (s *Server) loadTags() map[string]string {
    ts := store.NewSessionTagStore(s.tagStorePath)
    tags, err := ts.Load()
    if err != nil || tags == nil {
        return map[string]string{}
    }
    return tags
}

// resolveProjectName returns tags[sessionID] if set, else filepath.Base(projectPath).
func resolveProjectName(sessionID, projectPath string, tags map[string]string) string {
    if name, ok := tags[sessionID]; ok && name != "" {
        return name
    }
    return filepath.Base(projectPath)
}
```

Add `"github.com/blackwell-systems/claudewatch/internal/store"` import to tools.go.

### 4c. Update handlers (tools.go)

In every handler that calls `filepath.Base(session.ProjectPath)` or `filepath.Base(sess.ProjectPath)`:

1. Load tags once at the top of the handler: `tags := s.loadTags()`
2. Replace every `filepath.Base(session.ProjectPath)` with `resolveProjectName(session.SessionID, session.ProjectPath, tags)`

Affected handlers in tools.go (check by searching for `filepath.Base`):
- `handleGetSessionStats` — line ~187
- `handleGetRecentSessions` — line ~291
- `handleGetSAWSessions` — line ~334 (metaMap lookup) and ~340 (projectName assignment)

For `handleGetSAWSessions`: the project name comes from `metaMap[session.SessionID]`. Update the metaMap to use `resolveProjectName`:
```go
for _, meta := range metas {
    metaMap[meta.SessionID] = resolveProjectName(meta.SessionID, meta.ProjectPath, tags)
}
```

### 4d. Update health_tools.go

In `handleGetProjectHealth`:
- Add `tags := s.loadTags()` near the top (after loading sessions).
- Replace `filepath.Base(sorted[0].ProjectPath)` (default project) with `resolveProjectName(sorted[0].SessionID, sorted[0].ProjectPath, tags)`.
- Replace the project-matching filter `filepath.Base(s.ProjectPath) == project` with `resolveProjectName(s.SessionID, s.ProjectPath, tags) == project`.

### 4e. Update project_tools.go

In `handleGetProjectComparison`, replace:
```go
name := filepath.Base(sess.ProjectPath)
```
with:
```go
name := resolveProjectName(sess.SessionID, sess.ProjectPath, tags)
```
Add `tags := s.loadTags()` before the loop.

### 4f. tag_tools.go — set_session_project handler

```go
package mcp

import (
    "encoding/json"
    "errors"

    "github.com/blackwell-systems/claudewatch/internal/store"
)

// SetSessionProjectResult is the response for set_session_project.
type SetSessionProjectResult struct {
    SessionID   string `json:"session_id"`
    ProjectName string `json:"project_name"`
    OK          bool   `json:"ok"`
}

// handleSetSessionProject sets a project name override for a session.
func (s *Server) handleSetSessionProject(args json.RawMessage) (any, error) {
    var params struct {
        SessionID   string `json:"session_id"`
        ProjectName string `json:"project_name"`
    }
    if err := json.Unmarshal(args, &params); err != nil {
        return nil, err
    }
    if params.SessionID == "" {
        return nil, errors.New("session_id is required")
    }
    if params.ProjectName == "" {
        return nil, errors.New("project_name is required")
    }

    ts := store.NewSessionTagStore(s.tagStorePath)
    if err := ts.Set(params.SessionID, params.ProjectName); err != nil {
        return nil, err
    }

    return SetSessionProjectResult{
        SessionID:   params.SessionID,
        ProjectName: params.ProjectName,
        OK:          true,
    }, nil
}
```

### 4g. Register the tool (tools.go addTools)

Add to `addTools(s)`:
```go
s.registerTool(toolDef{
    Name:        "set_session_project",
    Description: "Override the project name attributed to a session. Use when the session was launched from a different directory than the project being worked on (e.g. SAW worktrees). Call with get_session_stats session_id and the correct project name.",
    InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string","description":"The session ID to tag. Use the session_id from get_session_stats."},"project_name":{"type":"string","description":"The project name to attribute this session to (e.g. 'brewprune')."}},"required":["session_id","project_name"],"additionalProperties":false}`),
    Handler:     s.handleSetSessionProject,
})
```

Register it after the existing tools (before or after `addAnalyticsTools` — end of `addTools` is fine).

## 5. Tests to Write

In `internal/mcp/tag_tools_test.go`:

1. `TestHandleSetSessionProject_OK` — valid call writes tag and returns OK:true
2. `TestHandleSetSessionProject_MissingSessionID` — empty session_id returns error
3. `TestHandleSetSessionProject_MissingProjectName` — empty project_name returns error
4. `TestHandleSetSessionProject_InvalidJSON` — malformed args returns error

For test setup: create a temp dir for `tagStorePath`, construct a `Server` directly with the temp path. You can construct the server and set `tagStorePath` directly in tests (use an unexported test helper or set the field directly — the test is in `package mcp`).

Also check `internal/mcp/tools_test.go` and `internal/mcp/saw_tools_test.go` for any tests that assert `project_name` values and update them if the test setup doesn't include a tag store (they should still pass because `loadTags` returns empty map when the file doesn't exist, so `resolveProjectName` falls back to `filepath.Base`).

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-B
go build ./...
go vet ./...
go test ./internal/mcp -v -timeout 2m
```

All must pass. If `internal/store` symbols are missing (Agent A not yet merged), note in completion report as `out_of_scope_build_blockers`.

## 7. Constraints

- `internal/mcp` must NOT import `internal/app`. This is a pre-existing rule.
- `loadTags()` is non-fatal: always returns a map (empty on error). Never return error from loadTags.
- `resolveProjectName` is a pure function — no side effects, no I/O.
- Do not change `NewServer`'s signature — `internal/app/mcp.go` calls it as `mcp.NewServer(cfg, mcpBudget)`.
- The tag store file is in `~/.config/claudewatch/session-tags.json` (config.ConfigDir()). Do not hardcode the path.

## 8. Report

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-B
git add internal/mcp/
git commit -m "wave2-agent-B: set_session_project tool + resolveProjectName"
```

Append your completion report under `### Agent B — Completion Report` in `docs/IMPL-session-project-tagging.md`.

```yaml
### Agent B — Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave2-agent-B
commit: {sha}
files_changed:
  - internal/mcp/jsonrpc.go
  - internal/mcp/tools.go
  - internal/mcp/health_tools.go
  - internal/mcp/project_tools.go
  - (+ any test files updated)
files_created:
  - internal/mcp/tag_tools.go
  - internal/mcp/tag_tools_test.go
interface_deviations: []
out_of_scope_deps: []
out_of_scope_build_blockers: []
tests_added:
  - TestHandleSetSessionProject_OK
  - TestHandleSetSessionProject_MissingSessionID
  - TestHandleSetSessionProject_MissingProjectName
  - TestHandleSetSessionProject_InvalidJSON
verification: PASS | FAIL
```
```

---

### Agent C — Wave 2: CLI Tag Command

```
# Wave 2 Agent C: CLI Tag Command

You are Wave 2 Agent C. Implement the `claudewatch tag` CLI command that lets a developer explicitly set the project name for a session from the terminal.

## 0. CRITICAL: Isolation Verification (RUN FIRST)

⚠️ **MANDATORY PRE-FLIGHT CHECK - Run BEFORE any file modifications**

**Step 1: Attempt environment correction**

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-C 2>/dev/null || true
```

**Step 2: Verify isolation**

```bash
ACTUAL_DIR=$(pwd)
EXPECTED_DIR="/Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-C"

if [ "$ACTUAL_DIR" != "$EXPECTED_DIR" ]; then
  echo "ISOLATION FAILURE: Wrong directory"
  echo "Expected: $EXPECTED_DIR"
  echo "Actual: $ACTUAL_DIR"
  exit 1
fi

ACTUAL_BRANCH=$(git branch --show-current)
EXPECTED_BRANCH="wave2-agent-C"

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
- `internal/app/tag.go` — create

## 2. Interfaces You Must Implement

```go
// package app

// tagCmd is the cobra command registered as a subcommand of rootCmd.
// Registered via init() in tag.go.
var tagCmd *cobra.Command
```

## 3. Interfaces You May Call

From Wave 1 (Agent A):
```go
// internal/store
func NewSessionTagStore(path string) *store.SessionTagStore
func (s *store.SessionTagStore) Set(sessionID, projectName string) error

// internal/config
func ConfigDir() string

// internal/claude
func ParseAllSessionMeta(claudeHome string) ([]claude.SessionMeta, error)
```

Existing app package utilities:
```go
// internal/config
func Load(cfgFile string) (*Config, error)

// internal/output
output.StyleBold, output.StyleMuted, output.StyleValue — for consistent formatting
```

## 4. What to Implement

Create `internal/app/tag.go` with the `claudewatch tag` command.

**Command signature:**
```
claudewatch tag --project <name> [--session <id>]
```

**Flags:**
- `--project <name>` (required): The project name to attribute the session to.
- `--session <id>` (optional): The session ID to tag. If omitted, use the most recent session from `claude.ParseAllSessionMeta`.

**Behavior:**

1. Load config: `cfg, err := config.Load(flagConfig)`
2. Resolve session ID:
   - If `--session` was provided, use it directly.
   - Otherwise, call `claude.ParseAllSessionMeta(cfg.ClaudeHome)`, sort descending by StartTime, take `sessions[0].SessionID`. If no sessions exist, return an error: "no sessions found; use --session to specify a session ID".
3. Validate `--project` is non-empty (cobra Required flag handles this).
4. Tag store path: `filepath.Join(config.ConfigDir(), "session-tags.json")`
5. Call `store.NewSessionTagStore(path).Set(sessionID, projectName)`
6. Print success: `Tagged: <sessionID>\nProject: <projectName>\n`

**Error handling:** All errors returned via `cmd.RunE`, displayed by cobra. No silent failures.

**Registration:** In `init()`, register `tagCmd` with `rootCmd.AddCommand(tagCmd)`.

Read `internal/app/mcp.go` and `internal/app/sessions.go` briefly to understand the command registration pattern before writing.

## 5. Tests to Write

In `internal/app/tag.go` — this is a CLI integration test. Because CLI testing requires executing cobra commands, write tests in a way consistent with the existing `internal/app/mcp_test.go`:

Check `internal/app/mcp_test.go` to understand the testing pattern used in this package. Follow the same approach.

Tests to write (in `internal/app/tag_test.go` if the pattern supports it — otherwise document why tests are deferred):

1. `TestTagCmd_MissingProject` — invoking without --project returns error
2. `TestTagCmd_WithExplicitSession` — with --session and --project, tags file is written correctly
3. `TestTagCmd_DefaultsToMostRecent` — without --session, picks the most recent session

Note: If `mcp_test.go` shows that app command tests are not written (or are integration-only), document this in your completion report and explain what manual verification you did instead.

## 6. Verification Gate

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claire/worktrees/wave2-agent-C
go build ./...
go vet ./...
go test ./internal/app -run TestTag -v -timeout 2m
```

If no tests exist for app commands (check mcp_test.go), run:
```bash
go build ./...
go vet ./...
go test ./internal/app -v -timeout 2m
```

All must pass. If Agent A's store package is missing, note as `out_of_scope_build_blockers`.

## 7. Constraints

- `internal/app/tag.go` only — do not touch any other files.
- Use `flagConfig` (the persistent flag from root.go) to load config — do not add a new config flag.
- If `--session` is omitted and no sessions exist, return a clear error. Do not tag with an empty session ID.
- Sort sessions by `StartTime` descending to find "most recent" — same approach as tools.go.
- Do not add output color that other commands don't use. Follow the established output style.

## 8. Report

```bash
cd /Users/dayna.blackwell/code/claudewatch/.claude/worktrees/wave2-agent-C
git add internal/app/tag.go
git commit -m "wave2-agent-C: claudewatch tag CLI command"
```

Append your completion report under `### Agent C — Completion Report` in `docs/IMPL-session-project-tagging.md`.

```yaml
### Agent C — Completion Report
status: complete | partial | blocked
worktree: .claude/worktrees/wave2-agent-C
commit: {sha}
files_changed: []
files_created:
  - internal/app/tag.go
interface_deviations: []
out_of_scope_deps: []
out_of_scope_build_blockers: []
tests_added:
  - (list or "see notes")
verification: PASS | FAIL
```
```

---

## Wave Execution Loop

After each wave:
1. Read each agent's completion report from `### Agent {letter} — Completion Report`.
2. Merge each agent's worktree branch into main: `git merge wave{N}-agent-{letter} --no-ff`.
3. Run full verification: `go build ./... && go vet ./... && go test ./...`
4. Fix any integration issues (especially: cascade candidates in `internal/app/mcp.go`, existing test assertions about project names).
5. **Linter auto-fix (orchestrator, not agents):** Run `golangci-lint run --fix` if configured in CI, or `gofmt -w .` to normalize formatting. Commit any style changes before running tests.
6. Tick status checkboxes below, update any interface deviations.
7. Launch next wave.

**Do not launch Wave 2 if Wave 1 verification fails.**

---

## Status

- [ ] Wave 1 Agent A — Session Tag Store (`internal/store/tags.go`)
- [ ] Wave 2 Agent B — MCP Tag Tool + resolveProjectName (`internal/mcp/`)
- [ ] Wave 2 Agent C — CLI Tag Command (`internal/app/tag.go`)

---

### Agent A — Completion Report

```yaml
status: complete
worktree: main (solo agent, no worktree)
commit: 7f5dd7f297d9eb8f025d5eb59faa67ae8abc52c4
files_changed: []
files_created:
  - internal/store/tags.go
  - internal/store/tags_test.go
interface_deviations: []
out_of_scope_deps: []
tests_added:
  - TestNewSessionTagStore_LoadEmpty
  - TestSessionTagStore_SetAndLoad
  - TestSessionTagStore_SetMultiple
  - TestSessionTagStore_SetOverwrite
  - TestSessionTagStore_LoadInvalidJSON
verification: PASS (go test ./internal/store/... — 5/5 tests)
```

Design decisions: Load does not hold the mutex as specified — callers in the MCP server are single-goroutine per request and Load is read-only. Set uses write-to-temp-then-rename (os.Rename) for atomic POSIX writes, cleaning up the temp file on any intermediate error. The test package is `store_test` (external test package), consistent with Go convention for black-box testing and matching the absence of any existing test files in the package to copy from.

---

### Agent B — Completion Report

*(To be filled by Agent B)*

---

### Agent C — Completion Report

*(To be filled by Agent C)*
