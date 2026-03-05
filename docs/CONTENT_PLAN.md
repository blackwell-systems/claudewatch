# Documentation Hub Content Plan

Mapping of existing sources → new documentation structure for README navigation hub.

## Batch 1: Core Features (Most Referenced)

### features/HOOKS.md

**Purpose:** Deep dive on SessionStart briefings and PostToolUse alerts (push observability layer)

**Primary sources:**
- README.md lines 95-106 (capability table)
- CLAUDE.md lines 17-20 (three-layer architecture)
- CLAUDE.md line 183 (hook cooldown configuration)

**Structure:**
1. What hooks are (push observability)
2. SessionStart briefing (project health, friction rate, agent success)
3. PostToolUse alerts (error loops, context pressure, cost spikes, drift)
4. Configuration (enabling/disabling, rate limiting)
5. Examples with sample output

**Estimated time:** 8 minutes

---

### features/MCP_TOOLS.md

**Purpose:** Comprehensive reference for all 29 MCP tools (pull observability layer)

**Primary sources:**
- docs/mcp.md (650 lines) - complete tool reference

**Structure:**
1. Overview (what MCP tools provide, how Claude uses them)
2. Session awareness tools (get_session_stats, get_session_dashboard, etc.)
3. Project health tools (get_project_health, get_project_comparison, etc.)
4. Memory and history tools (get_task_history, get_blockers, search_transcripts, etc.)
5. Agent analytics tools (get_agent_performance, get_saw_sessions, etc.)
6. Usage patterns (recommended call sequences)

**Estimated time:** 12 minutes (mostly restructuring existing content)

---

### features/CLI.md

**Purpose:** Complete CLI command reference

**Primary sources:**
- docs/cli.md (647 lines) - complete CLI reference

**Structure:**
1. Overview (CLI vs MCP layer)
2. Core commands (scan, metrics, gaps, suggest)
3. Action commands (fix, track, watch)
4. Export and integration (export, log)
5. Global flags and configuration

**Estimated time:** 10 minutes (mostly restructuring existing content)

---

## Batch 2: Remaining Features

### features/MEMORY.md

**Purpose:** Cross-session task memory and blocker tracking (persistent layer)

**Primary sources:**
- README.md lines 125-136 (task memory capability)
- CLAUDE.md line 21 (persistent layer description)
- docs/mcp.md memory tools section

**Structure:**
1. What is task memory (cross-session persistence)
2. Automatic extraction (extract_current_session_memory)
3. Querying history (get_task_history)
4. Blocker tracking (get_blockers)
5. Storage location and format

**Estimated time:** 8 minutes

---

### features/CONTEXT_SEARCH.md

**Purpose:** Unified context search across all sources

**Primary sources:**
- README.md lines 167-179 (context search capability)
- docs/IMPL-unified-context.md (implementation details)
- internal/context/ package code

**Structure:**
1. Overview (4 parallel sources: commits, memory, tasks, transcripts)
2. MCP tool usage (get_context)
3. CLI usage (claudewatch context)
4. Deduplication and ranking
5. Source attribution

**Estimated time:** 10 minutes

---

### features/METRICS.md

**Purpose:** Friction analysis and cost-per-outcome metrics

**Primary sources:**
- README.md lines 138-150 (friction analysis capability)
- docs/cli.md metrics section
- CLAUDE.md analyzer/ package description

**Structure:**
1. Friction scoring (what counts as friction)
2. Session trends (friction rate over time)
3. Tool error patterns
4. Zero-commit tracking
5. Stale problems detection
6. Cost-per-outcome metrics

**Estimated time:** 10 minutes

---

### features/AGENTS.md

**Purpose:** Multi-agent workflow analytics

**Primary sources:**
- README.md lines 154-166 (agent performance capability)
- CLAUDE.md lines 99-119 (multi-agent analytics section)
- docs/mcp.md agent tools section

**Structure:**
1. What agent analytics tracks
2. Success/kill rates by type
3. Parallelization analysis
4. Duration and token breakdown
5. SAW session observability

**Estimated time:** 10 minutes

---

## Batch 3: Getting Started Guides

### guides/INSTALLATION.md

**Purpose:** Installation instructions with troubleshooting

**Primary sources:**
- README.md lines 247-264 (installation section)
- CLAUDE.md lines 160-183 (blockers & solutions)

**Structure:**
1. Homebrew installation
2. Direct download
3. From source (Go 1.26+)
4. Verification (claudewatch --version)
5. Common installation issues
6. Next steps (configuration)

**Estimated time:** 8 minutes

---

### guides/QUICKSTART.md

**Purpose:** Complete walkthrough from scan to fix

**Primary sources:**
- docs/quickstart.md (158 lines) - existing walkthrough
- README.md lines 220-244 (friction reduction cycle)

**Structure:**
1. Initial scan (claudewatch scan)
2. Understanding the output (friction rate, health score)
3. Diagnosing issues (claudewatch gaps)
4. Applying fixes (claudewatch fix)
5. Measuring improvement (claudewatch track)

**Estimated time:** 6 minutes (light restructuring)

---

### guides/CONFIGURATION.md

**Purpose:** Hooks, MCP setup, behavioral rules

**Primary sources:**
- README.md lines 85-96 (quick start setup)
- docs/mcp.md lines 7-24 (MCP setup)
- CLAUDE.md lines 183-195 (configuration locations)

**Structure:**
1. MCP server configuration (~/.claude.json)
2. Hook installation (claudewatch install)
3. Configuration files (config.yaml)
4. Budget limits and thresholds
5. Custom metrics

**Estimated time:** 10 minutes

---

### guides/USE_CASE_FRICTION.md

**Purpose:** Case study: reducing friction from 45% to 28%

**Primary sources:**
- README.md lines 220-244 (friction reduction cycle)
- docs/quickstart.md (workflow examples)

**Structure:**
1. Baseline measurement
2. Identifying patterns
3. Applying targeted fixes
4. Measuring impact
5. Iterating on improvements

**Estimated time:** 12 minutes (narrative expansion)

---

### guides/USE_CASE_AGENTS.md

**Purpose:** Track and optimize agent workflows

**Primary sources:**
- New content based on agent analytics features

**Structure:**
1. Problem: agent kill rates, wasted tokens
2. Diagnosis: get_agent_performance, get_saw_sessions
3. Optimization: identify failing agent types
4. Measurement: success rate improvement

**Estimated time:** 12 minutes (write from scratch)

---

### guides/USE_CASE_CLAUDEMD.md

**Purpose:** Data-driven CLAUDE.md improvements

**Primary sources:**
- New content based on effectiveness scoring

**Structure:**
1. Problem: guessing what works in CLAUDE.md
2. Diagnosis: get_effectiveness, before/after scoring
3. Optimization: data-driven iteration
4. Measurement: friction reduction attribution

**Estimated time:** 12 minutes (write from scratch)

---

## Batch 4: Technical Deep Dives

### technical/ARCHITECTURE.md

**Purpose:** Three-layer AgentOps model deep dive

**Primary sources:**
- CLAUDE.md lines 15-32 (architecture section)
- README.md lines 53-66 (how it works)

**Structure:**
1. Three-layer model (push/pull/persistent)
2. Data flow (parsers → analyzers → store → output)
3. Key design principles
4. Package structure
5. Why Go + SQLite

**Estimated time:** 10 minutes

---

### technical/HOOKS_IMPL.md

**Purpose:** Rate limiting, chronic pattern detection

**Primary sources:**
- CLAUDE.md line 183 (hook cooldown)
- internal/app/hook.go code

**Structure:**
1. Hook execution model
2. Rate limiting (30s cooldown)
3. Chronic pattern detection
4. Alert thresholds
5. Timestamp caching

**Estimated time:** 10 minutes

---

### technical/MCP_INTEGRATION.md

**Purpose:** Server setup, tool design, data freshness

**Primary sources:**
- docs/mcp.md lines 7-24 (setup)
- CLAUDE.md lines 73-89 (adding MCP tools)
- internal/mcp/ package code

**Structure:**
1. MCP server lifecycle
2. Tool registration pattern
3. JSON-RPC 2.0 protocol
4. Live session detection
5. Data freshness strategies

**Estimated time:** 10 minutes

---

### technical/DATA_MODEL.md

**Purpose:** Session parsing, friction scoring, attribution

**Primary sources:**
- CLAUDE.md lines 149-152 (important files)
- docs/DATA_SOURCES.md
- claude/ package types

**Structure:**
1. Session metadata format
2. Facets (friction, satisfaction)
3. Transcript JSONL format
4. Agent span extraction
5. Multi-project attribution

**Estimated time:** 12 minutes

---

## Batch 5: Competitive Positioning

### comparison/VS_MEMORY_TOOLS.md

**Purpose:** Archive vs live feedback comparison

**Primary sources:**
- README.md lines 12-21 (comparison table)
- GitHub competitive research from previous session

**Structure:**
1. What memory tools are (passive archives)
2. What claudewatch provides (active feedback)
3. When to use both
4. Examples (claude-memory-mcp vs claudewatch)

**Estimated time:** 8 minutes

---

### comparison/VS_OBSERVABILITY.md

**Purpose:** Dashboards vs agent introspection

**Primary sources:**
- README.md lines 12-21 (comparison table)
- GitHub competitive research

**Structure:**
1. What observability platforms are (API monitoring for humans)
2. What claudewatch provides (agent self-awareness + human analytics)
3. Complementary use cases
4. Examples (LangSmith vs claudewatch)

**Estimated time:** 8 minutes

---

### comparison/VS_BUILTIN.md

**Purpose:** What Claude Code provides vs what's missing

**Primary sources:**
- New content based on Claude Code features

**Structure:**
1. Built-in Claude features (task memory, auto memory)
2. What claudewatch adds (behavioral intervention, friction analysis, agent analytics)
3. Why both are needed
4. Integration points

**Estimated time:** 10 minutes (write from scratch)

---

## Batch 6: Community

### CONTRIBUTING.md

**Purpose:** How to contribute code, docs, bug reports

**Primary sources:**
- CLAUDE.md lines 223-295 (quality checklist, self-reflection)

**Structure:**
1. Ways to contribute
2. Setting up development environment
3. Testing requirements
4. Documentation standards
5. PR process
6. Code of conduct

**Estimated time:** 10 minutes

---

## Summary

**Total files:** 20
**Estimated total time:** 2-2.5 hours

**Batch execution order:**
1. Core features (30 min) - most referenced, establish tone
2. Remaining features (38 min)
3. Getting started guides (60 min)
4. Technical deep dives (42 min)
5. Competitive positioning (26 min)
6. Community (10 min)

**Review checkpoints:** After batches 1, 3, and 4
