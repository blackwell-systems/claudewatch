# Context Search: Unified Search Across All Sources

Context search gives Claude and developers a single query interface for finding relevant information across four independent sources: commit history, memory files, task history, and session transcripts. Instead of knowing which source contains the answer, you ask one question and get ranked, deduplicated results from all of them.

## What Context Search Provides

Traditional search requires you to know where to look:
- "Was this in a commit message?" → `git log --grep`
- "Did we discuss this in a session?" → grep through transcripts
- "Is this in memory?" → cat memory files
- "Was this a task we tracked?" → check task history

Context search eliminates this decision. You query once, search in parallel across all four sources, and get unified results ranked by relevance.

## The Four Sources

### 1. Memory (Semantic)

Searches `commitmux` memory files using semantic embeddings. Finds conceptually related content even when exact keywords don't match.

**What it searches:**
- `MEMORY.md` files in all indexed repositories
- Project-specific notes and context

**Source tool:** `commitmux_search_memory` (external MCP via commitmux binary)

**Result format:**
- Path to memory file
- Matching content excerpt
- Embedding distance (converted to 0-1 score)

**Use case:** "What did we decide about authentication architecture?"

### 2. Commits (Semantic)

Searches commit messages using semantic embeddings. Finds commits by intent or topic, not just keyword matching.

**What it searches:**
- All commit messages across indexed repositories
- Author, timestamp, repo metadata

**Source tool:** `commitmux_search_semantic` (external MCP via commitmux binary)

**Result format:**
- Commit SHA
- Message
- Author and timestamp
- Repository name
- Embedding distance (converted to 0-1 score)

**Use case:** "When did we add rate limiting?"

### 3. Task History (Keyword)

Searches task identifiers, solutions, and blocker descriptions. Uses substring matching, not embeddings.

**What it searches:**
- Task identifiers from `working-memory.json`
- Task solutions
- Blocker descriptions

**Source tool:** `get_task_history` (local MCP, internal call)

**Result format:**
- Task identifier
- Status (completed, in_progress, abandoned)
- Sessions involved
- Commits produced
- Blockers hit

**Use case:** "Did we try implementing OAuth before?"

### 4. Transcripts (Full-Text)

Searches session transcript content using SQLite FTS5. Finds exact text matches with phrase and boolean operators.

**What it searches:**
- All indexed session transcripts (messages, tool calls, results)
- Session metadata (project, timestamp)

**Source tool:** `search_transcripts` (local MCP, internal call)

**Result format:**
- Session ID
- Entry type (message, tool_use, tool_result)
- Timestamp
- Highlighted snippet
- FTS5 relevance rank

**Use case:** "Which sessions discussed error handling?"

## Parallel Execution

Context search fans out to all four sources **in parallel** using `golang.org/x/sync/errgroup`. This means:

- Total query time = slowest source, not sum of all sources
- Typical response time: <1 second for most queries
- Partial failures don't block other sources

Each source gets `limit / 4` initial results (default: 5 per source for a 20-result limit). After deduplication and ranking, the final list is truncated to `limit`.

## Deduplication

Multiple sources may return overlapping content:
- A commit SHA appears in both commit results and transcript snippets
- A task identifier appears in both task history and memory files

**Deduplication strategy:**

1. Compute content hash (SHA-256) for each result's text content (normalized: lowercase, trim whitespace)
2. Group results by hash
3. Within each group, keep the result with highest source priority
4. Priority order: **commit > memory > task_history > transcript**

**Rationale:** Primary sources (commits, memory) are more canonical than derived sources (transcripts). When the same content appears in both, prefer the original.

## Relevance Ranking

Results are scored and sorted by composite relevance:

**Base score by source type:**

- **Semantic sources (memory, commits)**: `score = 1.0 - embedding_distance` (distances are 0-1, lower = better)
- **FTS sources (transcripts)**: Use SQLite FTS rank directly (already normalized 0-1)
- **Task history**: Keyword match frequency (substring occurrence count / total query tokens)

**Recency boost:**

```
final_score = base_score * (1.0 + 0.2 * recency_factor)
where recency_factor = min(1.0, age_days / 365)
```

Recent results (days ago → 0) get up to 20% boost. Old results (365+ days) get no boost.

**Sort order:** Descending by final_score (highest relevance first).

## Source Attribution

Every result includes metadata identifying where it came from:

```json
{
  "source": "commit",
  "title": "commit: a1b2c3d - Add rate limiting to API endpoints",
  "snippet": "Implement token bucket algorithm with 100 req/min limit...",
  "timestamp": "2026-02-28T14:22:00Z",
  "metadata": {
    "sha": "a1b2c3d",
    "author": "user@example.com",
    "repo": "api-service"
  },
  "score": 0.92
}
```

**Metadata fields by source:**

- **Memory**: `path`, `repo`
- **Commit**: `sha`, `author`, `repo`
- **Task history**: `task_identifier`, `status`, `session_id`
- **Transcript**: `session_id`, `project_hash`, `entry_type`

Use metadata to jump directly to the original source for full context.

## MCP Tool Usage

### `get_context`

Unified context search tool for use inside Claude sessions.

**Input:**

```json
{
  "query": "authentication rate limiting",
  "project": "api-service",  // optional, filters commits and tasks to this project
  "limit": 20  // optional, default: 20, max results to return
}
```

**Output:**

```json
{
  "query": "authentication rate limiting",
  "items": [
    {
      "source": "commit",
      "title": "commit: a1b2c3d - Add rate limiting to auth endpoints",
      "snippet": "Implement token bucket with 100 req/min limit per IP...",
      "timestamp": "2026-02-28T14:22:00Z",
      "metadata": {
        "sha": "a1b2c3d",
        "author": "user@example.com",
        "repo": "api-service"
      },
      "score": 0.92
    },
    {
      "source": "task_history",
      "title": "task: implement OAuth rate limiting",
      "snippet": "Status: completed. Solution: Use Redis for distributed rate limit state...",
      "timestamp": "2026-02-20T10:15:00Z",
      "metadata": {
        "task_identifier": "implement OAuth rate limiting",
        "status": "completed",
        "session_id": "abc123"
      },
      "score": 0.87
    }
  ],
  "count": 2,
  "sources": ["memory", "commit", "task_history", "transcript"],
  "errors": ["transcript: index not built, run 'claudewatch search' first"]
}
```

**Partial failures:**

If one source fails (e.g., commitmux not installed, transcript index empty), the tool returns results from successful sources and includes error messages in the `errors` array. The tool only returns an error (fails the entire call) if all sources fail or the query is empty.

**When to call:**

- Before implementing a feature: "Did we try this before?"
- When debugging: "Have we seen this error?"
- When resuming work: "What did we decide about X?"
- When onboarding: "How does authentication work here?"

**Typical usage in Claude sessions:**

```
User: Add rate limiting to the API

Claude: Let me check if we've worked on rate limiting before.
[calls get_context("rate limiting")]

Result shows:
- Commit from 2 weeks ago: "Add rate limiting to auth endpoints"
- Task history: "implement OAuth rate limiting" (completed)

Claude: We already have rate limiting on auth endpoints using a token bucket
algorithm with Redis. Should I extend that to other endpoints, or are you
looking for something different?
```

## CLI Usage

### `claudewatch context`

Command-line interface for unified context search.

**Usage:**

```bash
claudewatch context "authentication rate limiting"
claudewatch context "error handling" --project api-service
claudewatch context "add caching" --limit 10
claudewatch context "redis setup" --json
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--project <name>` | (all projects) | Filter commits and tasks to specific project |
| `--limit <int>` | 20 | Maximum results to return |
| `--json` | false | Output as JSON for programmatic consumption |

**Table output:**

```
Source          Title                                    Timestamp            Snippet
─────────────────────────────────────────────────────────────────────────────────────────────
commit          a1b2c3d - Add rate limiting to auth...  2026-02-28 14:22:00  Implement token bucket with 100 req/min...
task_history    implement OAuth rate limiting            2026-02-20 10:15:00  Status: completed. Solution: Use Redis...
memory          MEMORY.md                                2026-02-15 08:30:00  Rate limiting architecture: token bucket...
transcript      Session abc123def                        2026-02-28 16:45:00  ...discussed rate limit per-IP vs per-user...
```

**JSON output:**

Full `UnifiedContextResult` struct with all metadata:

```json
{
  "query": "authentication rate limiting",
  "items": [...],
  "count": 4,
  "sources": ["memory", "commit", "task_history", "transcript"],
  "errors": []
}
```

**When to use CLI vs MCP:**

- **CLI**: Human analysis, exporting to files, scripting, integration with other tools
- **MCP**: Claude querying context mid-session for decision-making

## Result Limit Distribution

When you specify `--limit 20` (or via MCP tool `limit: 20`), the distribution works like this:

1. **Initial fetch**: Each of the 4 sources gets `limit / 4 = 5` results
2. **Deduplication**: Remove duplicates, keeping highest-priority source per hash group
3. **Ranking**: Apply recency boost and sort by final_score descending
4. **Truncation**: Keep top `limit` results after ranking

This ensures:
- **Diversity**: All sources are represented before ranking
- **Quality**: Final list is ranked by relevance, not source order
- **Bounded**: Total results never exceed `limit`

**Example with limit=20:**

- Memory source: 5 initial results
- Commit source: 5 initial results
- Task history: 5 initial results
- Transcript: 5 initial results
- After dedup: 18 unique results (2 were duplicates)
- After ranking: top 18 sorted by score
- Truncate to 20: return all 18 (under limit)

## Error Handling

### Partial Failures

If one source fails, the query continues with the others:

**Example: commitmux not installed**

```json
{
  "query": "authentication",
  "items": [
    // results from task_history and transcript only
  ],
  "count": 8,
  "sources": ["memory", "commit", "task_history", "transcript"],
  "errors": [
    "memory: exec commitmux: executable file not found in $PATH",
    "commit: exec commitmux: executable file not found in $PATH"
  ]
}
```

This degrades gracefully instead of failing hard. You get partial results and know which sources failed.

**Example: transcript index not built**

```json
{
  "errors": [
    "transcript: index not built, run 'claudewatch search <query>' first"
  ]
}
```

The first `claudewatch search` CLI invocation auto-builds the FTS5 index. Subsequent queries (CLI or MCP) use the index.

### Total Failures

The tool returns an error (fails the entire call) only if:

1. **Query is empty** - `"query": ""` or missing
2. **All sources fail** - All 4 sources returned errors and zero results

In all other cases, partial results are returned with error messages in the `errors` array.

## Index Building

### Transcript Index

The transcript search uses SQLite FTS5, which requires building an index before the first query.

**Auto-indexing on CLI:**

```bash
claudewatch search "your query"
```

Prints "Indexing transcripts…" and builds the index before running the query. Suppressed with `--json`.

**Manual indexing:**

There is no explicit "build index" command. Indexing happens automatically on first `search` or `context` CLI invocation if the index is empty.

**Index location:**

`~/.config/claudewatch/claudewatch.db` - SQLite database, `transcript_index` FTS5 table.

**Index updates:**

The index is not automatically refreshed. To pick up new sessions:

```bash
# Force reindex (not yet implemented - currently requires manual SQLite cleanup)
rm ~/.config/claudewatch/claudewatch.db
claudewatch search "test"  # rebuilds index
```

## Dependencies

Context search requires:

1. **claudewatch binary** - For local MCP tools and CLI
2. **commitmux binary** - For memory and commit semantic search (optional, graceful degradation)
3. **Transcript index** - Built on first use via `claudewatch search`

**Checking dependencies:**

```bash
# claudewatch installed?
claudewatch --version

# commitmux installed?
commitmux --version

# Transcript index built?
claudewatch search "test" --limit 1
# If prints "Indexing transcripts…", index is being built now
```

**Installing commitmux:**

```bash
brew install blackwell-systems/tap/commitmux
```

Or see [commitmux installation docs](https://github.com/blackwell-systems/commitmux#installation).

## Performance

Typical query times:

- **Memory + Commit (semantic)**: 100-300ms (depends on index size)
- **Task history (keyword)**: <10ms
- **Transcript (FTS5)**: 50-150ms (depends on index size)
- **Total (parallel)**: ~300ms (bottlenecked by slowest source)

**Optimizations:**

- Parallel execution reduces total time to max(source_times), not sum
- Per-source limits reduce embedding search scope
- FTS5 is highly optimized for full-text queries
- Task history is in-memory (JSON file reads)

**Scaling:**

- 100 sessions: <200ms typical
- 1,000 sessions: <500ms typical
- 10,000 sessions: <1s typical (FTS5 scales well)

## Use Cases

### 1. Feature Discovery

**Question:** "Did we implement caching before?"

```bash
claudewatch context "caching"
```

**Results:**
- Task history: "add Redis caching" (abandoned, blocker: redis not installed)
- Commit: "Implement in-memory cache for API responses"
- Memory: "Caching architecture: decided on in-memory for now, Redis later"

**Outcome:** Know the history before starting work.

### 2. Error Archaeology

**Question:** "Have we seen this 'database is locked' error before?"

```bash
claudewatch context "database is locked"
```

**Results:**
- Transcript: Session abc123, "...SQLite lock error, resolved by closing connection..."
- Task history: "fix SQLite concurrency" (completed, solution: use WAL mode)

**Outcome:** Apply known solution instead of rediscovering it.

### 3. Decision Recall

**Question:** "Why did we choose JWT over sessions?"

```bash
claudewatch context "JWT vs sessions"
```

**Results:**
- Memory: "Authentication decision: JWT for stateless API, sessions for web app"
- Commit: "Switch from sessions to JWT for API auth"
- Transcript: Session def456, "...discussed JWT token expiry, decided 1 hour..."

**Outcome:** Understand past decisions and their context.

### 4. Onboarding

**Question:** "How does authentication work in this codebase?"

```bash
claudewatch context "authentication" --project api-service
```

**Results:**
- Multiple commits: auth implementation, middleware, rate limiting
- Task history: completed auth tasks with solutions
- Memory: architecture decisions and constraints
- Transcripts: discussions about edge cases

**Outcome:** Comprehensive view without asking a human.

## Comparison with Single-Source Search

### `git log --grep`

- **Scope**: Commits only
- **Search**: Keyword matching
- **Ranking**: Chronological
- **Use case**: "Find the commit that added X"

### `grep -r` (transcript files)

- **Scope**: Transcripts only
- **Search**: Regex/text matching
- **Ranking**: None (file order)
- **Use case**: "Find raw text in session files"

### `cat MEMORY.md`

- **Scope**: Memory files only
- **Search**: Manual reading
- **Ranking**: None
- **Use case**: "Read project-specific notes"

### `claudewatch context`

- **Scope**: All four sources
- **Search**: Semantic + keyword + FTS
- **Ranking**: Relevance + recency
- **Use case**: "Find anything related to X, ranked by importance"

## Integration with Other Features

Context search integrates with:

- **Memory system** - Task history is one of four sources
- **Agent analytics** - Transcripts include agent spans and metadata
- **Project attribution** - Multi-repo sessions route to dominant project
- **Effectiveness scoring** - Find sessions before/after CLAUDE.md changes

## Known Limitations

### 1. No cross-project memory index

Memory files are searched per-project. There's no global index across all projects. If you want to find "authentication" across all repos, you must query without `--project` filter and rely on commitmux's cross-repo indexing.

### 2. Transcript index staleness

The FTS5 index is not automatically refreshed. New sessions don't appear in search results until you rebuild the index (currently requires deleting the database and re-running `search`).

**Mitigation:** Rebuild the index periodically if you rely on transcript search heavily.

### 3. Semantic search requires commitmux

Memory and commit semantic search are powered by commitmux's embedding index. If commitmux is not installed, these sources return empty results with an error message. Context search still works, but only for task history and transcripts.

**Mitigation:** Install commitmux for full functionality.

### 4. No phrase search in semantic sources

Memory and commit sources use embedding similarity, not keyword matching. Queries like `"exact phrase"` are not supported—the embeddings compute conceptual similarity, not literal matching.

**Mitigation:** Use transcript search (FTS5) for exact phrase matching.

## Debugging Search Issues

**No results returned?**

1. Verify query is not empty: `claudewatch context "test"`
2. Check source availability: look for errors in CLI output or `errors` array in JSON
3. Verify data exists: `claudewatch memory status`, `git log --oneline`, `claudewatch search "test"`
4. Check project filter: `--project` may be filtering out results

**Wrong results ranked first?**

1. Recency bias: recent low-relevance results may rank higher than old high-relevance results
2. Source priority: commits rank higher than transcripts even with equal scores
3. Deduplication: one source's result may have been deduplicated in favor of another

**Transcript results missing?**

1. Index not built: run `claudewatch search "test"` to build
2. Sessions not indexed: delete DB and rebuild (index staleness)
3. Query syntax: FTS5 uses boolean operators, test with simple keyword first

**Semantic search not working?**

1. commitmux not installed: `which commitmux`
2. commitmux index not built: `commitmux index`
3. Query too vague: embeddings work best with specific concepts, not single words

## Related Documentation

- [MCP Tools Reference](/docs/features/MCP_TOOLS.md) - `get_context` tool details
- [Memory System](/docs/features/MEMORY.md) - Task history as searchable memory
- [CLI Commands](/docs/features/CLI.md) - `context` and `search` command details
- [Technical: MCP Integration](/docs/technical/MCP_INTEGRATION.md) - External MCP call pattern
