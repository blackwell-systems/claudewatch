# claudewatch Behavioral Enhancement Roadmap

> **Note:** This roadmap focuses on **behavioral interventions** (auto-injection, guardrails, forced reflection). For **analytical capabilities** (predictive intelligence, cost forecasting, platform features), see [docs/ROADMAP.md](docs/ROADMAP.md).

**Vision:** Transform claudewatch from a passive toolkit into an active cognitive scaffold that shapes AI agent behavior by making good behavior the default path.

**Current state:** Tools exist but require opt-in (agents must remember to call `get_project_health`, `get_blockers`, etc.). Adoption is low even with explicit instructions. SessionStart hook shows basic project info but not detailed health metrics.

**Target state:** Data is unavoidable, bad behavior is blocked, reflection is forced, learning is automatic.

---

## Core Principle

**Move from OPTIONAL to UNAVOIDABLE:**

- ❌ Tools agents can call → ✅ Data agents can't avoid seeing
- ❌ Suggestions agents can ignore → ✅ Blocks agents must acknowledge
- ❌ Reactive querying → ✅ Proactive injection
- ❌ "You should..." → ✅ "Here's what's happening..."
- ❌ Memory on demand → ✅ Memory when relevant
- ❌ End-of-session insights → ✅ Real-time feedback

---

## Phase 1: Foundation - Make Data Visible (v0.13.0)

**Goal:** Eliminate the need to query - inject baseline awareness by default

### 1.1 Auto-inject project health into session start briefing ✅ COMPLETE
**Impact:** 🔥🔥🔥 High - Fixes the core adoption problem
**Effort:** 🛠 Low - Hook already exists in `internal/app/startup.go`, extend it
**Dependencies:** None

**Status:** Implemented in `internal/app/startup.go`. SessionStart hook now shows: friction rate with label, top friction type, CLAUDE.md presence, agent success rate, per-type agent failures with "DO NOT SPAWN", new blocker count since last session, regression warnings, SAW correlation tips, and working memory auto-update.

**Implementation:**
- Extend `internal/app/startup.go` to show expanded metrics
- Change from "tip: call get_project_health" to showing the data directly
- Format:
  ```
  ╔ claudewatch | commitmux | friction: 40% ↑ (was 32%)
  ║ ⚠ HIGH FRICTION ENVIRONMENT - verify commands before execution
  ║ Top errors: buggy_code (45%), retry:Bash (32%)
  ║ Avg 112 tool errors/session - you are error-prone here
  ║
  ║ Agent failures: statusline-setup (0% success) - DO NOT SPAWN
  ║ 3 new blockers since last session → call get_blockers()
  ║ Last checkpoint: 45 min ago in session abc123
  ```

**Success metric:** Agent MCP calls to `get_project_health` drop to near zero (because data is already provided)

**Priority:** ⭐⭐⭐ DO FIRST

### 1.2 Contextual memory surfacing ✅ COMPLETE
**Impact:** 🔥🔥🔥 High - Reduces repeated failures
**Effort:** 🛠🛠 Medium - Pattern matching + injection
**Dependencies:** Task history extraction (exists)

**Status:** Implemented in `internal/memory/surface.go` (`SurfaceRelevantMemory`) and integrated into `startup.go` lines 271-313. Keyword extraction uses stop-word filtering + substring matching against `TaskIdentifier`. Displays matched tasks with status icons, blockers, solutions, and call-to-action.

**Implementation:**
- Hook into user message parsing at session start
- Extract keywords/topics from user message (first prompt or resume context)
- Query `get_task_history` automatically when keywords match prior tasks
- Inject matches into context above user message:
  ```
  📋 TASK HISTORY MATCH: "authentication"

  Found 2 prior attempts on this project:

    ✘ Session abc123 (1w ago): Tried JWT, hit rate limits
       Blocker: Auth0 quota exceeded, switched to sessions
       Status: Abandoned

    ✓ Session def456 (3d ago): Implemented session-based auth
       Solution: express-session + Redis
       Status: Completed, 4 commits

    Recommendation: Use sessions approach (proven success)
  ```

**Success metric:** "We tried this before" user complaints drop by 50%

**Priority:** ⭐⭐ DO NEXT

### 1.3 Real-time friction dashboard in briefing ✅ COMPLETE
**Impact:** 🔥🔥 Medium - Increases awareness during session
**Effort:** 🛠 Low - Use existing session stats
**Dependencies:** Live session reading (exists)

**Status:** Implemented in `internal/analyzer/dashboard.go` (`ComputeSessionDashboard`, `FormatDashboard`) and `internal/app/hook.go` (every 50 tool calls via `shouldDisplayDashboard`). Shows cost, commits, cost/commit, errors, duration, drift%, and traffic-light status (efficient/adequate/struggling).

**Implementation:**
- Add live metrics to PostToolUse hook output (every 50 tool calls or on ⚠ trigger)
- Show:
  ```
  SESSION EFFICIENCY DASHBOARD
  ─────────────────────────────
  Cost so far:       $2.40
  Commits:           2
  Cost per commit:   $1.20 (target: <$1.50) ✓
  Tool errors:       8 (avg: 4 at this point) ⚠
  Friction events:   3 (error loops, retries)
  Time in drift:     12 min (20% of session) ⚠

  Status: ADEQUATE - watch error rate
  ```
- Traffic light status: 🟢 efficient / 🟡 adequate / 🔴 struggling

**Success metric:** Agents reference efficiency metrics in decision-making

**Priority:** DO FIRST

**Deliverable:** v0.13.0 - "Active Awareness Release"

### 1.4 Per-model cost accuracy ✅ COMPLETE
**Impact:** 🔥🔥 Medium - Eliminates cost estimation error for multi-model sessions
**Effort:** 🛠 Low - Model names already in transcript, pricing table exists
**Dependencies:** Cache token fix (done), ModelStats per-model tracking (done)

**Status:** Implemented. Live session cost calculations (dashboard, cost velocity, hook) now use per-model pricing. Each assistant turn is priced at its actual model tier (opus/sonnet/haiku) via the `model` field in the JSONL transcript. Sonnet pricing used as fallback only when model field is absent. Changes: `claude/active_live_cost.go` (added `ModelPricingMap`, `PricingForModel`, per-turn costing), `analyzer/dashboard.go` (per-model via `SessionMeta.ModelUsage`), `app/hook.go` and `mcp/cost_velocity_tools.go` and `mcp/dashboard_tools.go` (removed hardcoded Sonnet constants).

**Implementation:**
- Parse `model` field from each assistant turn in JSONL (already extracted in `session_meta.go`)
- Map model name strings (e.g. `claude-sonnet-4-6`, `claude-opus-4-6`) to pricing tier via `analyzer.ClassifyModelTier()`
- Compute cost per-model using correct tier pricing, then sum
- Update `ComputeAttribution`, `ComputeSessionDashboard`, `ParseLiveCostVelocity` to use per-model pricing instead of single-tier default
- `ModelStats` already tracks per-model input/output/cache tokens — wire into cost formulas

**Success metric:** Cost attribution matches `/cost` within 5% (currently ~30% off for multi-model sessions)

**Priority:** BACKLOG (quick win after v0.13.0 ships)

**Deliverable:** v0.13.1 - "Cost Accuracy Patch"

---

## Phase 2: Guardrails - Prevent Bad Behavior (v0.14.0)

**Goal:** Block known failures before they happen, preserve memory across compactions

### 2.1 Drift intervention
**Impact:** 🔥🔥 Medium-High - Reduces wasted time exploring without implementing
**Effort:** 🛠 Low - PostToolUse hook extension
**Dependencies:** Drift detection (exists via `get_drift_signal`)

**Implementation:**
- PostToolUse hook tracks read/write ratio in rolling 15-tool window
- After 10 consecutive reads (no writes), inject warning:
  ```
  🛑 DRIFT DETECTED: 10 reads, 0 writes in 15 minutes

  You are exploring without implementing. Common causes:
  - Unclear requirements → Ask user for clarification
  - Overwhelmed by complexity → Scope down to smallest first step
  - Avoiding implementation → What's blocking you?

  Actions:
  [ ] Ask user for direction
  [ ] Scope down and implement smallest piece
  [ ] Call get_blockers() - is there a known issue?
  ```
- Escalate every 5 additional reads without writes

**Success metric:** Drift sessions (>20% time in read-only mode) drop by 40%

**Priority:** DO FIRST

### 2.2 Repetitive error blocking
**Impact:** 🔥🔥 Medium - Stops error loops early
**Effort:** 🛠 Low - PostToolUse hook logic
**Dependencies:** Live friction tracking (exists)

**Implementation:**
- Track (tool_name, error_category) tuples in current session
- After 3rd occurrence of same pattern:
  ```
  ⚠ PATTERN DETECTED: 3 Bash retries in 10 min

    Your friction rate this session: 40%
    Your normal rate: 25%
    → You're struggling 60% more than usual

    Common cause on this project: commands not verified before execution

    Action: Call get_blockers() to check for known issues
  ```

**Success metric:** Sessions with 5+ repetitive errors drop by 50%

**Priority:** DO FIRST

### 2.3 Auto-extract memory on compaction (pulled forward from 3.5.1)
**Impact:** 🔥🔥🔥 High - Solves the #1 memory loss problem
**Effort:** 🛠 Low - Hook + context pressure detection
**Dependencies:** `extract_current_session_memory` (exists), context pressure detection (exists)

**Current state:** `extract_current_session_memory` must be called manually before compaction. Agents rarely remember to do this. The CLAUDE.md instructions say "At 'pressure' level, call `extract_current_session_memory` before compaction" but compliance is low.

**Implementation:**
- PostToolUse hook already detects context pressure via `ParseLiveContextPressure`
- When pressure transitions from "comfortable" → "pressure", auto-call `extract_current_session_memory` before the agent's next turn
- Store a state file (`~/.cache/claudewatch-compaction-extracted`) to prevent duplicate extraction
- Format: inject into hook output:
  ```
  ⚠ Context pressure at 75% — auto-extracting session memory before compaction...
  ✓ Saved: 3 tasks, 2 blockers, 1 architectural insight
  ```
- Reset state file when session ID changes

**Success metric:** Memory extraction rate goes from <10% to 90%+ for sessions that hit compaction

**Priority:** DO FIRST

### 2.4 Agent spawn prevention via auto-generated rules
**Impact:** 🔥🔥🔥 High - Prevents wasted cost on failing agents
**Effort:** 🛠 Low - Rules file generation, no hook needed
**Dependencies:** Agent performance data (exists)

**Rationale:** PreToolUse hooks don't exist in Claude Code, so hook-based blocking isn't possible. Instead, prevent bad spawns at the instruction layer — auto-generate project-specific `.claude/rules/` files that tell agents which agent types to avoid. Rules are processed as high-priority instructions, making them preventive rather than reactive.

**Implementation:**
- `claudewatch install` (or new `claudewatch rules generate`) analyzes agent performance data
- For agent types with <30% success rate on the current project, generate a rule file:
  ```
  # .claude/rules/claudewatch-agent-blocklist.md

  ## Agent Types to Avoid on This Project

  DO NOT spawn the following agent types — they have consistently failed:

  - **statusline-setup** — 0% success (0/2). Common error: permission denied.
    Alternative: manual configuration per CLAUDE.md instructions.

  Last updated: 2026-03-05 by claudewatch
  ```
- Re-generate on each `claudewatch install` or `claudewatch scan` run
- Project-specific (`.claude/rules/`) so it doesn't affect other repos
- Users can edit or delete the file to override

**Success metric:** Zero spawns of agents with <30% success rate (via instruction compliance, not blocking)

**Priority:** DO NEXT

**Deliverable:** v0.14.0 - "Guardrails Release"

---

## Phase 3: Metacognition - Force Reflection (v0.15.0)

**Goal:** Build memory habits through mandatory pauses

### 3.1 Reflection checkpoints
**Impact:** 🔥🔥🔥 High - Dramatically improves memory extraction quality and frequency
**Effort:** 🛠🛠 Medium - Requires interruption mechanism
**Dependencies:** `extract_current_session_memory` (exists)

**Implementation:**
- Timer-based triggers: 30 min, 60 min, 90 min
- PostToolUse hook injects checkpoint prompt:
  ```
  ⏸ REFLECTION CHECKPOINT (30 min elapsed)

  What have you learned that's worth preserving?
  - New blockers discovered and solutions?
  - Patterns that worked or failed?
  - Architectural insights about this codebase?

  Template:
  ## Blocker: [brief name]
  File: [path if relevant]
  Issue: [what went wrong]
  Solution: [how it was fixed]

  ## Pattern: [what worked/failed]
  Context: [when this applies]
  Outcome: [result]
  ```
- Agent writes response, system auto-calls `extract_current_session_memory` with notes
- Snooze option: "Remind me in 15 min"

**Success metric:** Memory extraction rate goes from <10% to 60% of sessions >30min

**Priority:** ⭐ DO NEXT

### 3.2 Success pattern learning
**Impact:** 🔥🔥 Medium - Reinforces effective behaviors
**Effort:** 🛠🛠🛠 High - Requires pattern analysis engine
**Dependencies:** Historical session data (exists)

**Implementation:**
- New analyzer: `internal/analyzer/patterns.go`
- Correlate tool usage sequences with outcomes (commits, friction, cost)
- Store "pattern effectiveness" by project in local DB
- Inject when relevant pattern detected in current session:
  ```
  💡 YOUR PATTERN: When you use Edit tool + tests on this project
     Success rate: 92% (23/25 sessions)
     Avg cost: $1.20

     When you use Write tool (rewrites) on this project
     Success rate: 60% (6/10 sessions)
     Avg cost: $2.80

     → Prefer Edit tool for this codebase
  ```
- Track: Edit→Test, Read→Write, Grep→Read→Edit sequences

**Success metric:** Adoption rate of high-success patterns (>80% success) increases 30%

**Priority:** BACKLOG

### 3.3 Cost/benefit visibility
**Impact:** 🔥 Low-Medium - Awareness, not behavior change
**Effort:** 🛠 Low - Dashboard formatting (reuse from 1.3)
**Dependencies:** Live session stats (exists)

**Implementation:**
- Extend 1.3 dashboard with comparison to historical averages
- Add target thresholds (cost/commit <$1.50, friction <30%, etc.)
- Show trend arrows (↑↓) vs recent sessions

**Success metric:** Agents reference cost metrics when making tool choices

**Priority:** BACKLOG

**Deliverable:** v0.15.0 - "Metacognition Release"

---

## Phase 3.5: Persistent Memory - Survive Compaction (v0.15.5)

**Goal:** Eliminate post-compaction amnesia and make project knowledge searchable

**Problem statement:** Claude loses all working context on compaction (every ~60-90 min in long sessions). The compaction summary preserves ~2000 tokens of what was a 200k token context window. Debugging insights, architectural discoveries, and decision rationale vanish. Memory files exist but are write-only — written and rarely consulted because there's no way to know what's relevant without reading everything.

**Key integration:** [commitmux](https://github.com/blackwell-systems/commitmux) already provides the embedding infrastructure (Ollama + sqlite-vec + 768-dim vectors), full-text commit search, and an MCP server. Rather than building a parallel vector store, claudewatch should extend commitmux's existing infrastructure for memory embedding and leverage its commit history as a complementary knowledge layer.

### 3.5.1 Auto-extract on compaction ✅ MOVED TO PHASE 2 (2.3)
**Pulled forward to v0.14.0 as item 2.3.** High impact, low effort, no reason to wait for Phase 3.5.

**Priority:** Shipped in Phase 2

### 3.5.2 Semantic memory search (via commitmux integration)
**Impact:** 🔥🔥🔥 High - Transforms memory from write-only to searchable
**Effort:** 🛠🛠 Medium - Leverages commitmux's existing embedding pipeline
**Dependencies:** Memory files (exists), session transcripts (exists), commitmux (exists)

**Current state:** Memory is scattered across `~/.claude/projects/*/memory/*.md` files, working memory JSON, and session transcripts. No unified search. Agent must know the exact filename to read relevant memory. `search_transcripts` does keyword matching but misses conceptual connections.

Meanwhile, commitmux already has exactly the infrastructure we'd need to build:
- Ollama embedding via `nomic-embed-text` (768-dim vectors)
- sqlite-vec for kNN cosine similarity search
- Async embedding pipeline with batch processing and error recovery
- MCP server with semantic search tool (`commitmux_search_semantic`)

**Implementation — extend commitmux rather than build parallel infrastructure:**
- Add a new `memory` table to commitmux's SQLite store alongside `commits`:
  - Schema: `memory_docs(doc_id, source, project, content, created_at)`
  - Source types: `"session_summary"`, `"task"`, `"blocker"`, `"memory_file"`, `"decision"`
  - Corresponding vec0 table: `memory_embeddings(embed_id, embedding FLOAT[768], +doc_id, +source, +project, +content_preview, +created_at)`
- New commitmux ingest source: claudewatch memory files
  - `commitmux ingest-memory --claude-home ~/.claude` — scans `projects/*/memory/*.md`, working memory JSON, session summaries
  - Reuses existing `Embedder` and `build_embed_doc` pattern from commit embedding
  - Incremental: tracks last-indexed mtime per file, only re-embeds changed files
- New commitmux MCP tool: `commitmux_search_memory(query, project?, source?, limit?) → []MemoryMatch`
  - Searches memory_embeddings via same kNN pattern as `commitmux_search_semantic`
  - Filterable by project and source type
- claudewatch MCP wrapper: `search_memory(query, top_k)` — calls commitmux's tool internally or queries the shared SQLite directly
- New CLI: `claudewatch memory search "authentication approach"` → delegates to commitmux store
- Auto-reindex: claudewatch PostToolUse hook triggers incremental re-embed when memory files change

**Why extend commitmux instead of building from scratch:**
- Eliminates duplicate Ollama dependency management (model config, endpoint config, connection error handling)
- Eliminates duplicate sqlite-vec setup (schema, migrations, vector storage format)
- Agents already have commitmux MCP tools available — no new MCP server to configure
- Shared embedding model means commit search and memory search return comparable similarity scores
- Single SQLite database simplifies backup, migration, and debugging

**Success metric:** Agent finds relevant prior knowledge in 80% of cases where it exists (vs ~10% today)

**Priority:** ⭐⭐ DO NEXT

### 3.5.3 Codebase map generation (augmented by commitmux)
**Impact:** 🔥🔥 Medium - Eliminates repeated codebase re-discovery
**Effort:** 🛠🛠 Medium - Pattern analysis from edit history + commit history
**Dependencies:** File history (exists in `~/.claude/file-history/`), session metadata (exists), commitmux commit_files table (exists)

**Current state:** Every new session, Claude re-reads files to understand module boundaries, conventions, and relationships. There's no persistent "this is how the codebase works" document. Agents spend 10-20% of early session time on orientation.

**Implementation — dual data source approach:**
- **Source 1: Claude session edit co-occurrence** (claudewatch)
  - Files edited together in the same session are functionally related
  - Data from `~/.claude/file-history/` and session transcripts
- **Source 2: Git commit co-occurrence** (commitmux)
  - Files changed in the same commit are structurally related
  - Query via `commitmux_touches` and `commit_files` table
  - Commit messages provide semantic labels for relationships ("refactor auth", "add caching")
  - Covers history before claudewatch was installed (git history is deeper)
- Build a weighted relationship graph: `internal/analyzer/codemap.go`
  - Session co-edits weighted higher (reflect active development patterns)
  - Commit co-changes provide baseline coverage and historical depth
- Auto-generate `memory/architecture.md` per project:
  ```markdown
  ## Module Map (auto-generated from 45 sessions + 1,200 commits)

  ### internal/claude/ — JSONL transcript parsing
  Core files: active.go, session_meta.go, types.go
  Frequently co-edited with: internal/app/hook.go, internal/mcp/
  Conventions: assistantMsg* structs for JSON parsing, Parse* public API
  Recent commit themes: "cache token", "live session", "cost velocity"

  ### internal/analyzer/ — Metric computation
  Core files: cost.go, models.go, outcomes.go
  Frequently co-edited with: internal/mcp/*_tools.go
  Conventions: Compute* functions, DefaultPricing map
  ```
- New CLI command: `claudewatch map [project]`
- Auto-refresh: append new discoveries after each session via PostToolUse or SessionEnd

**Success metric:** Session orientation time (reads before first edit) drops 30%

**Priority:** BACKLOG

### 3.5.4 Memory consolidation
**Impact:** 🔥 Low-Medium - Keeps memory clean and accurate
**Effort:** 🛠🛠 Medium - Dedup + staleness detection
**Dependencies:** 3.5.2 (semantic search for dedup detection)

**Current state:** Memory files grow monotonically. Stale entries (resolved blockers, outdated patterns) accumulate. No mechanism to prune or merge related entries.

**Implementation:**
- Periodic consolidation pass (weekly or on `claudewatch memory consolidate`):
  - Detect duplicate/near-duplicate entries via embedding similarity
  - Flag blockers with matching solutions as "resolved"
  - Merge entries about the same topic into single authoritative entry
  - Archive (don't delete) consolidated entries
- Staleness heuristic: blockers not referenced in 30 days → suggest archival
- New MCP tool: `get_memory_health()` → stale count, duplicate count, total entries

**Success metric:** Memory file size stays bounded; no duplicate entries across topic files

**Priority:** BACKLOG

### 3.5.5 Unified context surface (commitmux + claudewatch MCP bridge)
**Impact:** 🔥🔥🔥 High - Single query answers "what do I need to know?"
**Effort:** 🛠🛠 Medium - MCP tool composition + result ranking
**Dependencies:** 3.5.2 (semantic memory search), commitmux MCP tools (exists)

**Current state:** Agents must query two separate MCP servers to get full context: claudewatch for session history/blockers/friction and commitmux for commit history/code changes. They rarely query both, missing connections like "this blocker was resolved in commit abc123" or "the approach that worked in session X produced commits Y and Z."

**Implementation:**
- New claudewatch MCP tool: `get_context(query, project?, limit?) → UnifiedContext`
  - Fans out to multiple sources in parallel:
    1. `search_memory` (3.5.2) — prior task attempts, blockers, session summaries
    2. `commitmux_search_semantic` — relevant commits by meaning
    3. `get_task_history` — matching task records
    4. `search_transcripts` — keyword matches in session transcripts
  - Deduplicates and ranks results by relevance score (embedding distance + recency decay)
  - Returns unified result with source attribution:
    ```json
    {
      "query": "authentication",
      "results": [
        {"source": "task_history", "summary": "JWT auth attempted, switched to sessions", "session_id": "abc123", "relevance": 0.92},
        {"source": "commit", "summary": "feat: add session-based auth middleware", "sha": "def456", "relevance": 0.88},
        {"source": "memory", "summary": "Auth0 rate limits hit at 100 req/min", "file": "blockers.md", "relevance": 0.85},
        {"source": "commit", "summary": "fix: handle expired session tokens", "sha": "ghi789", "relevance": 0.81}
      ]
    }
    ```
- Cross-reference linking: match commit SHAs mentioned in session transcripts to commitmux commit details
- New CLI: `claudewatch context "authentication"` — unified search across all knowledge sources
- Integration with 1.2 (contextual memory surfacing): session start keyword extraction queries `get_context` instead of just `get_task_history`

**Why this matters:**
- "What happened" (claudewatch sessions) + "what changed" (commitmux commits) = complete picture
- Agents get implementation history (commits) alongside decision history (sessions) in one call
- Reduces from 4+ MCP calls to 1, lowering tool-call overhead and context consumption

**Success metric:** Agents query `get_context` instead of individual tools; relevant prior knowledge surfaced in 90%+ of cases

**Priority:** ⭐ DO NEXT (after 3.5.2)

**Deliverable:** v0.15.5 - "Persistent Memory Release"

---

## Phase 4: Collective Intelligence (v0.16.0)

**Goal:** Learn from aggregate patterns across users (opt-in)

### 4.1 Anonymized blocker sharing
**Impact:** 🔥🔥🔥 High - Crowdsourced solutions to common problems
**Effort:** 🛠🛠🛠🛠 Very High - Requires opt-in telemetry infrastructure
**Dependencies:** Privacy-preserving aggregation system

**Implementation:**
- New command: `claudewatch share --anonymous`
- Privacy safeguards:
  - Hash project names (preserve uniqueness, not identity)
  - Strip absolute file paths (keep relative structure only)
  - Remove all user identifiers, session IDs, timestamps
  - Only upload: blocker pattern, solution, success/failure
- Upload to central registry (hosted service or distributed DHT)
- Query registry in `get_blockers()`:
  ```
  🌐 COLLECTIVE INSIGHT: This blocker seen 47 times across 12 projects

  Most effective solution (80% success):
    "Run go fmt before git commit, add pre-commit hook"

  Attempted solutions that failed:
    - Manual formatting (40% success - forgot steps)
    - IDE auto-format (55% success - inconsistent)

  Average time to resolve: 8 minutes
  ```
- Local-first fallback: if no network, use local data only

**Success metric:**
- 1000+ shared blockers in registry
- Blocker resolution time drops 40% for participants

**Priority:** LONG TERM

### 4.2 Pattern effectiveness benchmarking
**Impact:** 🔥🔥 Medium - Community learning from aggregate patterns
**Effort:** 🛠🛠🛠 High - Aggregation + privacy + analysis
**Dependencies:** Anonymized sharing infrastructure (4.1)

**Implementation:**
- Aggregate: (tool sequence, project type, language) → outcomes
- Community benchmarks:
  ```
  📊 COMMUNITY BENCHMARK: Go projects

  Users who use "Edit → Test → Commit" pattern:
    Success rate: 87%
    Avg cost/commit: $1.15

  Users who use "Write → Manual verify → Commit" pattern:
    Success rate: 72%
    Avg cost/commit: $1.85

  Your pattern: Edit → Test (matches high-performing group)
  ```
- Show percentile rankings for friction rate, cost efficiency

**Success metric:** Participants adopt community best practices, friction drops 25%

**Priority:** LONG TERM

**Deliverable:** v0.16.0 - "Collective Intelligence Release"

---

## Phase 5: Adaptive Behavior (v0.17.0)

**Goal:** Personalized nudges based on individual agent patterns

### 5.1 Behavioral nudges
**Impact:** 🔥🔥 Medium - Reduces friction through personalization
**Effort:** 🛠🛠 Medium - Personalization engine + feedback loop
**Dependencies:** Individual behavior tracking across sessions

**Implementation:**
- Track effectiveness of each nudge type per agent/project:
  - Was the nudge shown? (yes)
  - Was the action taken? (yes/no)
  - Did it improve outcome? (friction/cost comparison)
- Learn: which nudges work for this agent on this project
- Adapt: show more of what works, suppress what's ignored
- Personalized friction reduction strategies
- Example: if agent always ignores drift warnings but responds to blocker alerts, prioritize blocker notifications

**Success metric:** Nudge response rate increases from 40% to 70%

**Priority:** LONG TERM

### 5.2 Proactive task history surfacing
**Impact:** 🔥🔥🔥 High - Zero-friction memory access
**Effort:** 🛠🛠🛠 High - Intent detection + ML
**Dependencies:** Contextual memory surfacing (1.2)

**Implementation:**
- ML model (or heuristic classifier): user message → task intent
- Categories: authentication, API integration, database migration, testing, etc.
- Auto-query task history on high-confidence intent match
- Inject BEFORE agent processes message (not after agent asks)
- Feedback loop: did agent use the information? (track if referenced in response)
- Learn from feedback to improve classification

**Success metric:**
- Task history surfaced automatically in 80% of relevant cases
- Agent references prior attempts in 70% of those cases

**Priority:** RESEARCH / LONG TERM

**Deliverable:** v0.17.0 - "Adaptive Behavior Release"

---

## Implementation Priority Matrix

### High Impact, Low Effort (DO FIRST - v0.13.0 ✅ SHIPPED)
- ⭐⭐⭐ 1.1 Auto-inject project health ✅
- 1.2 Contextual memory surfacing ✅
- 1.3 Real-time friction dashboard ✅
- 1.4 Per-model cost accuracy ✅

### High Impact, Low Effort (DO FIRST - v0.14.0)
- 2.1 Drift intervention
- 2.2 Repetitive error blocking
- 2.3 Auto-extract on compaction (pulled from 3.5.1)
- 2.4 Agent spawn prevention via auto-generated rules

### High Impact, Medium Effort (DO NEXT - v0.15.0-v0.15.5)
- ⭐ 3.1 Reflection checkpoints
- ⭐⭐ 3.5.2 Semantic memory search (via commitmux — reduced effort)
- ⭐ 3.5.5 Unified context surface (commitmux + claudewatch bridge)

### Medium Impact (BACKLOG - v0.15.0-v0.16.0)
- 3.2 Success pattern learning
- 3.3 Cost/benefit visibility
- 3.5.3 Codebase map generation (augmented by commitmux)
- 3.5.4 Memory consolidation
- 5.1 Behavioral nudges

### High Effort, Long Term (RESEARCH - v0.16.0+)
- 4.1 Anonymized blocker sharing
- 4.2 Pattern effectiveness benchmarking
- 5.2 Proactive task history surfacing

---

## Milestones & Timeline

### v0.13.0 - "Active Awareness" (Target: 2-3 weeks)
**Deliverables:**
- Auto-inject project health into session start briefing
- Contextual memory surfacing (keyword-based)
- Real-time friction dashboard in PostToolUse hook

**Success metrics:**
- Agent sees baseline data in 100% of sessions (without calling get_project_health)
- Memory surfacing triggers in 40% of sessions with prior task history
- Friction dashboard visible every 50 tool calls

**Technical approach:**
- Extend `internal/hooks/session_start.go` to call MCP tools internally
- Add keyword extraction to user message parser
- Extend PostToolUse hook with dashboard formatting

---

### v0.14.0 - "Guardrails" (Target: 1-2 weeks after v0.13.0)
**Deliverables:**
- Drift intervention alerts (PostToolUse hook extension)
- Repetitive error blocking (PostToolUse hook extension)
- Auto-extract memory on compaction (context pressure trigger)
- Agent spawn prevention via auto-generated rules files

**Success metrics:**
- 40% reduction in drift sessions (>20% time in read-only)
- 50% reduction in sessions with 5+ repetitive errors
- 90%+ memory extraction rate for sessions hitting compaction (vs <10% today)
- Zero spawns of agents with <30% success rate (via instruction compliance)

**Technical approach:**
- Extend PostToolUse hook with rolling window read/write tracking (drift)
- Pattern detection for (tool, error) tuples in PostToolUse (error blocking)
- Context pressure transition detection → auto-call extract_current_session_memory
- Auto-generate `.claude/rules/claudewatch-agent-blocklist.md` from agent performance data
- No PreToolUse hook needed — all features use PostToolUse or rules-layer prevention

---

### v0.15.0 - "Metacognition" (Target: 4-6 weeks after v0.14.0)
**Deliverables:**
- Reflection checkpoints (timer-based)
- Success pattern learning engine
- Enhanced cost/benefit visibility

**Success metrics:**
- 60% of sessions >30min have memory extraction
- 30% increase in adoption of high-success patterns (>80% success rate)
- Cost/commit improves 15% from baseline

**Technical approach:**
- Timer integration with PostToolUse hook
- New pattern analyzer: `internal/analyzer/patterns.go`
- Historical pattern correlation engine

---

### v0.15.5 - "Persistent Memory" (Target: 2-3 weeks after v0.15.0)
**Deliverables:**
- Auto-extract memory on compaction (context pressure trigger)
- Semantic memory search via commitmux embedding infrastructure
- Codebase map generation (edit + commit co-occurrence analysis)
- Unified context surface bridging claudewatch + commitmux

**Success metrics:**
- 90%+ memory extraction rate for sessions hitting compaction (vs <10% today)
- 80% relevant knowledge retrieval rate via `search_memory`
- 30% reduction in session orientation time (reads before first edit)
- Single `get_context` call replaces 4+ separate MCP queries

**Technical approach:**
- Context pressure transition detection in PostToolUse hook
- Extend commitmux SQLite store with `memory_docs` + `memory_embeddings` tables (reuse existing Ollama/nomic-embed-text/sqlite-vec pipeline)
- New commitmux ingest source: `commitmux ingest-memory --claude-home ~/.claude`
- New commitmux MCP tool: `commitmux_search_memory(query, project?, source?, limit?)`
- Dual-source codebase map: Claude session edits (claudewatch) + git commit co-changes (commitmux)
- Unified `get_context` MCP tool: fan-out to memory search, commit search, task history, transcripts
- New CLI: `claudewatch map`, `claudewatch memory search`, `claudewatch context`

---

### v0.16.0 - "Collective Intelligence" (Target: 8-12 weeks after v0.15.0)
**Deliverables:**
- Anonymized blocker sharing (opt-in)
- Community pattern benchmarks
- Privacy-preserving aggregation

**Success metrics:**
- 1000+ shared blockers in registry
- 100+ active participants
- 40% faster blocker resolution for participants
- 25% friction reduction through community best practices

**Technical approach:**
- Central registry design (hosted service or DHT)
- Privacy safeguards: hashing, path stripping, anonymization
- Local-first fallback for offline operation
- Opt-in consent flow

---

### v0.17.0 - "Adaptive Behavior" (Target: 12+ weeks after v0.16.0)
**Deliverables:**
- Personalized behavioral nudges
- Proactive task history surfacing
- Feedback-driven optimization

**Success metrics:**
- Nudge response rate 70% (up from 40%)
- Task history surfaced automatically in 80% of relevant cases
- 50% friction rate reduction vs baseline

**Technical approach:**
- Personalization engine: track nudge effectiveness per agent
- Intent classification (ML or heuristic-based)
- Feedback loop: measure if surfaced info was used

---

## Next Actions

### Start Today
1. Create GitHub issues for Phase 1 features (1.1, 1.2, 1.3)
2. Draft technical spec for auto-injected project health format
3. Design contextual memory surfacing keyword matching algorithm

### This Week
1. Implement 1.1 (auto-inject project health)
   - Extend `internal/hooks/session_start.go`
   - Call `mcp.GetProjectHealth()` internally
   - Format rich baseline output
2. Test with dogfooding: verify agent sees data without querying
3. Create branch: `feature/auto-inject-health`

### This Month
1. Complete Phase 1 (v0.13.0-alpha)
2. Dogfood for 1 week, gather feedback
3. Ship v0.13.0 stable
4. Begin Phase 2: prototype agent spawn blocking

### This Quarter
1. Ship v0.13.0 and v0.14.0
2. Begin Phase 3 (reflection checkpoints)
3. Gather community feedback on injection vs query tradeoffs
4. Refine behavioral nudge strategies based on real usage

---

## The North Star

### By v0.15.0, an agent using claudewatch should:
- ✅ **SEE** project health without asking (auto-injected at session start)
- ✅ **KNOW** past attempts when relevant (auto-surfaced on keyword match)
- ✅ **STOP** before repeating failures (blocked by guardrails)
- ✅ **PAUSE** to reflect regularly (forced checkpoints every 30min)
- ✅ **LEARN** from patterns (shown personal success rates)

### By v0.15.5, add:
- ✅ **REMEMBER** across compactions (auto-extract before context loss)
- ✅ **SEARCH** project knowledge semantically (via commitmux embedding infrastructure)
- ✅ **ORIENT** instantly in any codebase (persistent architecture map from edits + commits)
- ✅ **CONNECT** session history with code history (unified context: claudewatch + commitmux)

### By v0.17.0, add:
- ✅ Learn from the community (collective intelligence, opt-in)
- ✅ Adapt to individual behavior (personalized nudges)
- ✅ Zero-friction memory (proactive surfacing)

---

## Open Questions

1. **PreToolUse hook:** Does Claude Code support this? If not, can we intercept via MCP server?
2. **Interruption mechanism:** How do we force a pause for reflection checkpoints? PostToolUse hook output only?
3. **Central registry:** Self-hosted vs cloud service? DHT for decentralization?
4. **Intent classification:** ML model (cost/complexity) vs heuristic rules (simpler)?
5. **Feedback loop:** How do we measure if agent used surfaced information? Parse assistant message?

---

## Contributing

Interested in implementing a roadmap feature? See:
- [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines
- [Phase 1 tracking issue](#) for current work
- [Roadmap discussions](#) for open questions and design debates

**Priority areas for contributors:**
1. Auto-inject project health (1.1) - highest impact, clearest spec
2. Drift intervention (2.2) - low effort, high value
3. Reflection checkpoints (3.1) - UX design needed

---

## Feedback

This roadmap represents current thinking based on observed agent behavior and adoption challenges. It will evolve based on:
- Real-world usage data
- Community feedback
- Technical constraints discovered during implementation
- New insights from behavioral experiments

Join the discussion: [GitHub Discussions](#) or [Discord](#)
