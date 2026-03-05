# Memory: Persistent Cross-Session Learning

Task memory is the **persistent layer** of claudewatch's three-layer AgentOps model. It captures what worked, what failed, and what blocked progress across sessions so Claude doesn't rediscover the same solutions—or repeat the same mistakes.

## What Task Memory Provides

Unlike hooks that alert in real-time or MCP tools that query current state, task memory **stores cross-session context** that survives beyond individual sessions. It answers questions like:

- "Did we try this approach before, and what happened?"
- "What blockers have we hit on this project, and how were they solved?"
- "What tasks are in progress, abandoned, or completed?"

This is not conversation archival. It's structured knowledge extraction: tasks, blockers, solutions, and outcomes stored as queryable data.

## Storage Model

Task memory lives in per-project working memory files:

```
~/.config/claudewatch/projects/<project>/working-memory.json
```

Each project gets its own memory file containing:

- **Tasks** - Descriptions, status, sessions, commits, solutions
- **Blockers** - Issues hit, files involved, solutions applied, timestamps

The file is human-readable JSON. You can edit or delete it at any time. No vendor lock-in, no proprietary format.

## The Three Memory Operations

### 1. Extract

Capture task and blocker memory from a session. This happens automatically via the SessionStart hook after completed sessions, or can be triggered manually.

**Automatic extraction:**

The SessionStart hook checks for completed sessions (closed more than 5 minutes ago) and extracts memory if:
- The session produced commits
- The session encountered friction
- No memory has been extracted from this session yet

**Manual extraction:**

```bash
# Extract from the active session
claudewatch memory extract

# Extract from a specific session
claudewatch memory extract --session-id abc123def456

# Override project name
claudewatch memory extract --project myproject
```

**What gets extracted:**

- **Task identifier** - Inferred from commit messages, user messages, or tool calls
- **Status** - `completed` (commits made), `in_progress` (edits without commits), `abandoned` (stuck/killed)
- **Sessions** - List of session IDs that touched this task
- **Commits** - SHA list for completed tasks
- **Solution** - Text extracted from the resolution (if task completed)
- **Blockers** - Friction events with file paths, error types, and solutions applied

**Session selection:**

When `--session-id` is not provided:
- **Multiple active sessions** (modified within 15 min):
  - **TTY environment**: Shows interactive numbered menu
  - **Non-TTY/piped**: Returns error with session list
- **Single active session**: Uses that session automatically
- **No active sessions**: Returns error (no fallback to historical sessions)

Extraction requires active work—you can't extract from a week-old session because the relevant context is gone.

### 2. Query

Retrieve memory via MCP tools. Claude queries task history and blockers mid-session to avoid rediscovering solutions or repeating failed approaches.

#### `get_task_history`

Returns tasks matching a description query, sorted by recency.

**MCP tool signature:**

```json
{
  "name": "get_task_history",
  "arguments": {
    "query": "authentication",
    "project": "myproject"  // optional
  }
}
```

**Returns:**

```json
{
  "tasks": [
    {
      "task_identifier": "implement JWT authentication",
      "status": "abandoned",
      "sessions": ["abc123", "def456"],
      "commits": [],
      "solution": null,
      "blockers_hit": ["rate_limit", "missing_dependency"],
      "created_at": "2026-02-28T10:15:00Z",
      "updated_at": "2026-03-01T14:22:00Z"
    }
  ],
  "count": 1
}
```

**Use case:** Before implementing a feature, query whether it was attempted before. If status is `abandoned`, read the blockers to understand what failed.

#### `get_blockers`

Returns known blockers for a project with their solutions.

**MCP tool signature:**

```json
{
  "name": "get_blockers",
  "arguments": {
    "project": "myproject"  // optional
  }
}
```

**Returns:**

```json
{
  "blockers": [
    {
      "friction_type": "retry:Bash",
      "file_or_command": "make test",
      "issue_description": "CGO_ENABLED=0 required for cross-compilation",
      "solution": "Set CGO_ENABLED=0 before running go build",
      "first_seen": "2026-02-20T08:30:00Z",
      "last_seen": "2026-03-01T16:45:00Z",
      "occurrence_count": 3
    }
  ],
  "count": 1
}
```

**Use case:** When hitting an error, call `get_blockers()` before debugging. If a solution exists, apply it instead of rediscovering it.

#### `search_transcripts`

Full-text search over indexed session transcripts. Finds sessions where a specific topic, error, or tool was discussed.

**MCP tool signature:**

```json
{
  "name": "search_transcripts",
  "arguments": {
    "query": "error AND build",
    "limit": 20  // optional
  }
}
```

**Returns:**

```json
{
  "count": 5,
  "indexed_count": 142,
  "results": [
    {
      "session_id": "abc123",
      "project_hash": "def456",
      "entry_type": "message",
      "timestamp": "2026-03-01T14:22:00Z",
      "snippet": "...go build error: cannot find package...",
      "rank": 0.85
    }
  ]
}
```

**Index building:**

The transcript index must be built before querying. If the index is empty, the MCP tool returns an error directing you to run:

```bash
claudewatch search "your query"
```

This builds the FTS5 index automatically on first use. Subsequent CLI or MCP queries use the index.

**Use case:** "When did we discuss anomaly detection?" Search transcripts to find relevant sessions before starting work.

### 3. Review

Inspect stored memory via CLI.

#### `memory status`

Cross-project summary: total tasks and blockers, last extraction timestamp, most recent task, per-project breakdown.

```bash
claudewatch memory status
```

**Output:**

```
Cross-session Memory Status
─────────────────────────────────────────
Tasks stored:           8 (across 3 projects)
Blockers recorded:      4
Last extraction:        2 minutes ago
Most recent task:       "implement drift detection" (completed, claudewatch)

Projects with memory:
  claudewatch          5 tasks, 2 blockers
  commitmux           2 tasks, 1 blocker
  scout-and-wave      1 task, 1 blocker

Run 'claudewatch memory show --project <name>' for details
```

#### `memory show`

Detailed working memory for a project: task history with sessions, status, commits, solutions, and blockers; blocker list with file, issue, solution, and last-seen timestamp.

```bash
claudewatch memory show                  # current project (from cwd)
claudewatch memory show --project commitmux
```

**Output:**

```
Working Memory: claudewatch
─────────────────────────────────────────

Tasks (5):

  [1] implement drift detection (completed)
      Sessions: abc123def
      Commits: 3
      Solution: Track read/write ratio in last 20 tool calls, alert when ≥60% reads with 0 writes
      Created: 2 days ago
      Updated: 3 hours ago

  [2] add anomaly detection (in_progress)
      Sessions: ghi789jkl
      Commits: 0
      Solution: —
      Created: 1 day ago
      Updated: 1 hour ago

  [3] JWT authentication (abandoned)
      Sessions: mno012pqr, stu345vwx
      Commits: 0
      Solution: —
      Blockers: rate_limit, missing_dependency
      Created: 5 days ago
      Updated: 3 days ago

Blockers (2):

  [1] retry:Bash — make test
      Issue: CGO_ENABLED=0 required for cross-compilation
      Solution: Set CGO_ENABLED=0 before running go build
      First seen: 12 days ago
      Last seen: 3 days ago
      Occurrences: 3

  [2] wrong_approach — internal/mcp/tools.go
      Issue: Import cycle when mcp imports app
      Solution: MCP handlers build context inline, no app import
      First seen: 8 days ago
      Last seen: 8 days ago
      Occurrences: 1
```

#### `memory clear`

Delete working memory for a project. Prompts for confirmation before deletion.

```bash
claudewatch memory clear                  # current project
claudewatch memory clear --project myproject
```

**Use case:** Reset memory when pivoting to a new architecture or after resolving chronic patterns.

## How Memory Gets Created

Memory extraction happens through two paths:

### Path 1: Automatic (SessionStart Hook)

The SessionStart hook runs at the start of every session. It checks for recently completed sessions (closed >5 minutes ago) and extracts memory if:

1. Session produced commits OR encountered friction
2. Memory has not been extracted from this session yet
3. Session is not the currently active session

This runs in the background—no user action required. You work, the hook extracts, memory builds over time.

### Path 2: Manual (CLI Command)

You call `claudewatch memory extract` explicitly. Useful for:

- **Checkpointing long sessions** - Save progress mid-session before risky operations
- **Before destructive operations** - `git reset --hard`, large refactors, etc.
- **After completing major work** - Feature shipped, milestone reached

The behavioral rules in `~/.claude/rules/claudewatch-session-protocol.md` instruct Claude to checkpoint memory before destructive operations and after milestones.

## Memory in the Decision Flow

Here's how memory integrates into a typical workflow:

**Without memory:**

1. User: "Add authentication"
2. Claude: Implements JWT authentication
3. Hits rate limit error from auth provider
4. Debugs for 20 minutes, burns $2 in tokens
5. Pivots to session-based auth
6. Ships feature
7. Next session: User asks for OAuth
8. Claude doesn't know JWT was tried and failed
9. Repeats the same rate limit debugging

**With memory:**

1. User: "Add authentication"
2. SessionStart hook: Shows 42 sessions, friction moderate, tip: "verify API rate limits before third-party integrations"
3. Claude calls `get_task_history("authentication")`
4. Finds task: "implement JWT authentication" (abandoned, blockers: rate_limit, missing_dependency)
5. Claude: "We tried JWT before and hit rate limits. Let's use session-based auth instead."
6. Ships feature without rediscovering the blocker
7. Next session: User asks for OAuth
8. Claude queries memory, finds session-based auth is working
9. Implements OAuth on top of existing session system

Memory turns repeated failures into learned patterns.

## Memory vs Other Tools

| | Task Memory | Memory Tools | Observability |
|---|---|---|---|
| **Category** | Cross-session learning | Conversation archive | API monitoring |
| **Stores** | Tasks + blockers | Full conversations | API logs |
| **Queried by** | Claude (MCP) + Humans (CLI) | Claude (MCP) | Humans (dashboards) |
| **Persists** | Indefinitely (until cleared) | Indefinitely | 30-90 days |
| **Purpose** | Avoid repeating failures | Recall past conversations | Track API costs |

Task memory is not a replacement for conversation archives like `claude-memory-mcp`. It's structured knowledge extraction optimized for decision-making, not full recall.

## Storage Location and Format

**Per-project memory:**

```
~/.config/claudewatch/projects/<project>/working-memory.json
```

**Schema:**

```json
{
  "project": "claudewatch",
  "tasks": [
    {
      "task_identifier": "implement drift detection",
      "status": "completed",
      "sessions": ["abc123def456"],
      "commits": ["a1b2c3d", "e4f5g6h", "i7j8k9l"],
      "solution": "Track read/write ratio in last 20 tool calls, alert when ≥60% reads with 0 writes",
      "blockers_hit": [],
      "created_at": "2026-03-01T10:00:00Z",
      "updated_at": "2026-03-01T14:30:00Z"
    }
  ],
  "blockers": [
    {
      "friction_type": "retry:Bash",
      "file_or_command": "make test",
      "issue_description": "CGO_ENABLED=0 required for cross-compilation",
      "solution": "Set CGO_ENABLED=0 before running go build",
      "first_seen": "2026-02-20T08:30:00Z",
      "last_seen": "2026-03-01T16:45:00Z",
      "occurrence_count": 3
    }
  ]
}
```

**Editing manually:**

You can edit the JSON file directly to:
- Update task descriptions
- Add missing solutions
- Remove outdated blockers
- Merge duplicate tasks

Changes take effect immediately—next MCP tool call reads the updated file.

## Data Privacy

All memory files live in `~/.config/claudewatch/projects/` on your machine. No network calls. No cloud sync. No telemetry.

**Sensitive data warning:**

Task descriptions and blocker solutions may contain:
- Code snippets
- Error messages with file paths
- API endpoint names
- Third-party service names

Never share working-memory.json files publicly or upload them to external services unless you've reviewed the contents for sensitive information.

## Memory Retention Policy

claudewatch does not auto-delete memory. Files persist until you explicitly clear them via:

```bash
claudewatch memory clear --project <name>
```

Or by deleting the file manually:

```bash
rm ~/.config/claudewatch/projects/<project>/working-memory.json
```

**Recommended retention:**

- **Active projects**: Keep indefinitely
- **Completed projects**: Clear after shipping or archiving
- **Pivots**: Clear when changing architecture (old blockers no longer relevant)

## Known Limitations

### 1. Task identifier extraction is heuristic

Task identifiers are inferred from:
- Commit messages (most reliable)
- User messages mentioning "implement X", "fix Y", "add Z"
- File names from Edit/Write tool calls

This works well for most sessions but may misidentify tasks in:
- Research sessions with no clear task
- Multi-task sessions (mixing unrelated work)
- Sessions with vague commit messages ("wip", "fix", "update")

**Mitigation:** Write clear commit messages. The first commit message in a session becomes the task identifier.

### 2. Blocker solutions require manual curation

Solutions are extracted from:
- Text following a blocker resolution (e.g., message after fixing a Bash error)
- Commit messages that mention "fix" or "solve"

But solutions often lack context. You may need to manually edit the blocker entry to add:
- Why the solution works
- When to apply it
- Exceptions or edge cases

**Mitigation:** Run `claudewatch memory show`, review blockers, edit the JSON to add detail.

### 3. No deduplication across projects

If two projects hit the same blocker (e.g., "go vet fails on unused imports"), each stores it independently. There's no cross-project blocker index.

**Mitigation:** Manually copy blocker solutions between project memory files if they're reusable.

## Debugging Memory Issues

**Memory not being extracted?**

1. Check SessionStart hook is installed: `grep SessionStart ~/.claude/settings.json`
2. Verify session produced commits or friction: `claudewatch metrics --days 1`
3. Check memory file exists: `ls ~/.config/claudewatch/projects/<project>/working-memory.json`
4. Manually extract: `claudewatch memory extract`

**MCP tool returns empty results?**

1. Verify memory file exists and is not empty: `cat ~/.config/claudewatch/projects/<project>/working-memory.json`
2. Check query matches task identifier: task identifiers are substring-matched, not full-text search
3. Verify project name matches: `claudewatch memory status` to see all projects with memory

**Blocker solutions not showing up?**

1. Check blocker was extracted: `claudewatch memory show --project <name>`
2. Manually add solution: edit `working-memory.json` and add to `blockers[].solution` field
3. Verify blocker has `first_seen` and `last_seen` timestamps

## Integration with Other Features

Memory integrates with:

- **Hooks** - SessionStart briefing mentions agent success rate (derived from task completion)
- **Effectiveness scoring** - CLAUDE.md changes are effective if they reduce blocker recurrence
- **Stale patterns** - Blockers that recur without CLAUDE.md updates appear in `get_stale_patterns`
- **Unified context search** - `get_context` includes task history as one of four parallel sources

## Example Workflow

**Day 1: Feature attempt**

```bash
# User asks Claude to add Redis caching
# Claude implements, hits "redis: connection refused"
# Spends 15 minutes debugging
# Discovers Redis not installed
# User: "Let's skip Redis for now"
# Session ends without commits
```

Memory extracted: Task "add Redis caching" (abandoned), blocker "missing_dependency: redis-server not installed"

**Day 7: Feature retry**

```bash
# User asks Claude to add caching
# SessionStart hook shows 48 sessions, friction moderate
# Claude calls get_task_history("caching")
# Finds abandoned task: "add Redis caching" with blocker "missing_dependency"
# Claude: "We tried Redis before but it wasn't installed. Should we install it first, or use an in-memory cache?"
# User: "Use in-memory"
# Claude implements in-memory cache, ships in 10 minutes
```

Saved: 15 minutes debugging + $1.50 in tokens

**Day 14: Review**

```bash
claudewatch memory show --project myproject
# See all tasks and blockers
# Notice blocker recurrence: "redis not installed" appeared 3 times
# Decide: install Redis or document the decision to skip it in CLAUDE.md
```

## Related Documentation

- [MCP Tools Reference](/docs/features/MCP_TOOLS.md) - `get_task_history`, `get_blockers`, `search_transcripts` tool details
- [Hooks](/docs/features/HOOKS.md) - SessionStart briefing includes memory-derived signals
- [Context Search](/docs/features/CONTEXT_SEARCH.md) - Task history as one of four parallel sources
- [CLI Commands](/docs/features/CLI.md) - `memory status`, `memory show`, `memory extract`, `memory clear`
