# claudewatch

[![Blackwell Systems™](https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg)](https://github.com/blackwell-systems)
[![CI](https://github.com/blackwell-systems/claudewatch/actions/workflows/ci.yml/badge.svg)](https://github.com/blackwell-systems/claudewatch/actions)
[![Release](https://img.shields.io/github/v/release/blackwell-systems/claudewatch)](https://github.com/blackwell-systems/claudewatch/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/blackwell-systems/claudewatch)](https://goreportcard.com/report/github.com/blackwell-systems/claudewatch)

**AgentOps for Claude Code.** Real-time monitoring and behavioral intervention for AI agents + post-session analytics for developers.

NOT a memory MCP replacement. NOT infrastructure observability. NOT LLM API monitoring. Operations for the AI agent itself during development. Cost is inferred from Claude Code's local usage data, not from API-side tracing.

## The Gap We Fill

| | Memory Tools | LLM Observability | **claudewatch (AgentOps)** |
|---|---|---|---|
| **Category** | Storage | API monitoring | **Agent operations** |
| **When** | After session | After API call | **During session + after** |
| **For** | AI (read past) | Humans (API dashboards) | **AI (live feedback) + Humans (ops dashboards)** |
| **Monitors** | Conversations | API costs/latency | **Agent behavior + workflow friction** |
| **Examples** | `claude-memory-mcp` | LangSmith, Langfuse | **PostToolUse interventions, drift alerts, agent performance, CLAUDE.md effectiveness** |

### What is AgentOps?

Like **DevOps** is operations for software delivery and **MLOps** is operations for ML models, **AgentOps is operations for AI agents**:

- **Monitor agent behavior** - Error rates, drift patterns, context pressure, cost velocity
- **Intervene automatically** - Block retry loops, surface known blockers, detect stuck states
- **Provide analytics** - Friction trends, cost per commit, agent success rates, exportable metrics
- **Enable self-awareness** - Agent queries its own performance mid-session via MCP tools

claudewatch brings AgentOps to the development experience—monitoring Claude Code sessions during your workflow, not production API calls.

---

**Three concrete examples:**

1. **Error loops** - Memory tools store that you hit an error. Observability tools log the retry count. claudewatch fires a PostToolUse hook on the third consecutive error and tells Claude "you're looping, call get_blockers() to check for known solutions" - during the session where it can act.

2. **Drift detection** - Memory tools archive the 15 files you read. Observability tools chart read/write ratios. claudewatch detects 8 consecutive reads with zero writes and alerts Claude "you're exploring without implementing, stuck or avoiding?" - before 20 more reads burn your context budget.

3. **Agent performance** - Memory tools store transcripts containing agent launches. Observability tools count API calls. claudewatch parses agent lifecycles from transcripts, computes success rates by type, and exposes `get_agent_performance()` so Claude queries "plan agents get killed 40% of the time on this project" and skips plan mode - before spawning an agent that will fail.

**The differentiation:** Other tools give humans dashboards. claudewatch gives Claude queryable access to its own performance inside the session where decisions are being made.

---

![Demo GIF placeholder - shows PostToolUse hook firing on error loop, Claude calling get_blockers(), finding documented solution, applying fix instead of rediscovering it]

_Demo coming soon: real-time intervention cycle from error detection to blocker lookup to solution application_

---

## How It Works

claudewatch reads local session data from `~/.claude/` and turns it into actionable insights through three layers:

### 1. Push (Hooks)
**Automatic intervention** - SessionStart briefing on project health + PostToolUse alerts on error loops, context pressure, cost spikes, drift. Claude doesn't need to remember to check - the system tells it when something is wrong.

### 2. Pull (MCP Tools)
**Self-reflection API** - 29 MCP tools let Claude query its own metrics mid-session: `get_project_health`, `get_drift_signal`, `get_task_history`, `get_blockers`, `get_agent_performance`. No other tool gives an AI agent this kind of introspective access.

### 3. Persistent (Ops Memory)
**Cross-session learning** - Task history, blockers, and solutions tracked automatically. Claude queries "did we try this before?" and gets "yes, JWT approach hit rate limits, pivoted to sessions" - without you having to remember or explain.

**All local.** Reads `~/.claude/` files on disk. No network calls. No telemetry.

## Quick Start

```bash
# Get a baseline on all your projects
claudewatch scan

# Find what's costing you time
claudewatch gaps

# See top 3 improvements ranked by impact
claudewatch suggest --limit 3
```

**Enable Claude's self-monitoring** (one-time setup):

```bash
# Install behavioral rules + MCP server config
claudewatch install

# Restart Claude Code to load the MCP server
```

That's it. Claude now has real-time awareness of its own behavior.

## Core Capabilities

<table>
<tr>
<td width="50%">

### Real-Time Alerts
**PostToolUse hooks detect problems during execution**

- Error loops (3+ consecutive failures)
- Context pressure (window filling)
- Cost velocity spikes
- Drift (stuck reading without writing)

→ [Hook Implementation](/docs/features/HOOKS.md)

</td>
<td width="50%">

### Self-Reflection Tools
**29 MCP tools for mid-session queries**

- `get_project_health` - friction rate, agent success
- `get_drift_signal` - exploring vs implementing
- `get_session_dashboard` - all live metrics at once
- `get_cost_velocity` - burn rate over last 10 min

→ [MCP Tools Reference](/docs/features/MCP_TOOLS.md)

</td>
</tr>

<tr>
<td>

### Ops Memory
**Cross-session history and blocker tracking**

- Query previous attempts by description
- Retrieve known blockers with solutions
- Checkpoint progress mid-session
- Full-text transcript search

→ [Memory System](/docs/features/MEMORY.md)

</td>
<td>

### Friction Analysis
**Measure what's slowing you down**

- Session trends (friction rate, corrections)
- Tool error patterns by type
- Zero-commit session tracking
- Stale problems that persist across weeks

→ [Metrics & Analytics](/docs/features/METRICS.md)

</td>
</tr>

<tr>
<td>

### Agent Performance
**Multi-agent workflow analytics**

- Success rates by agent type
- Kill patterns and cost per task
- Parallelization ratios
- Duration and token breakdowns

→ [Agent Analytics](/docs/features/AGENTS.md)

</td>
<td>

### Unified Context Search
**Parallel search across all context sources**

- Commits, memory, task history, transcripts
- Deduplicated, relevance-ranked results
- Source attribution for every match
- Sub-second response times

→ [Context Search](/docs/features/CONTEXT_SEARCH.md)

</td>
</tr>
</table>

## Documentation Hub

### 📘 Getting Started
- [Installation](/docs/guides/INSTALLATION.md) - Homebrew, direct download, from source
- [First Session](/docs/guides/QUICKSTART.md) - Complete walkthrough from scan to fix
- [Configuration](/docs/guides/CONFIGURATION.md) - Hooks, MCP setup, behavioral rules

### 🎯 Use Cases
- [Reduce Friction](/docs/guides/USE_CASE_FRICTION.md) - From 45% friction rate to 28%
- [Improve Agent Success](/docs/guides/USE_CASE_AGENTS.md) - Track and optimize agent workflows
- [Optimize CLAUDE.md](/docs/guides/USE_CASE_CLAUDEMD.md) - Data-driven improvements with effectiveness scoring

### 🔧 Features
- [Hooks](/docs/features/HOOKS.md) - SessionStart briefings + PostToolUse alerts
- [MCP Tools](/docs/features/MCP_TOOLS.md) - All 29 tools with usage patterns
- [CLI Commands](/docs/features/CLI.md) - scan, metrics, gaps, suggest, fix, track, watch, export
- [Memory System](/docs/features/MEMORY.md) - Task history, blockers, cross-session persistence
- [Context Search](/docs/features/CONTEXT_SEARCH.md) - Unified search across all context sources
- [Metrics & Analytics](/docs/features/METRICS.md) - Friction, cost-per-outcome, effectiveness scoring
- [Agent Analytics](/docs/features/AGENTS.md) - Success rates, timing, parallelization

### 🏗️ Technical
- [Architecture](/docs/technical/ARCHITECTURE.md) - How the three layers work together
- [Hooks Implementation](/docs/technical/HOOKS_IMPL.md) - Rate limiting, chronic pattern detection
- [MCP Integration](/docs/technical/MCP_INTEGRATION.md) - Server setup, tool design, data freshness
- [Data Model](/docs/technical/DATA_MODEL.md) - Session parsing, friction scoring, attribution

### 🆚 Comparison
- [vs Memory Tools](/docs/comparison/VS_MEMORY_TOOLS.md) - Archive vs live feedback
- [vs Observability Platforms](/docs/comparison/VS_OBSERVABILITY.md) - Dashboards vs agent introspection
- [vs Built-in Claude Features](/docs/comparison/VS_BUILTIN.md) - What Claude Code provides vs what's missing

### 🤝 Community
- [Contributing](/docs/CONTRIBUTING.md) - How to contribute code, docs, bug reports
- [Roadmap](/docs/ROADMAP.md) - Planned features and improvements
- [Changelog](/docs/CHANGELOG.md) - Version history and release notes

## Example: Friction Reduction Cycle

```bash
# 1. Baseline - where are you now?
claudewatch scan
# → Project "shelfctl" scores 42/100, friction rate 45%

# 2. Diagnose - what's causing friction?
claudewatch gaps
# → Missing: testing section in CLAUDE.md
# → Stale pattern: "go vet" errors in 55% of sessions

# 3. Fix - apply data-driven patches
claudewatch fix shelfctl --dry-run
claudewatch fix shelfctl
# → Added testing section
# → Added pre-edit lint hook

# 4. Measure - did it work?
claudewatch track
# ... work for a week ...
claudewatch track --compare
# → Friction rate: 45% → 28% (-17%)
# → Tool errors/session: 4.2 → 1.1 (-74%)
```

## Installation

**Homebrew (macOS/Linux):**
```bash
brew install blackwell-systems/tap/claudewatch
```

**Direct download:**
```bash
# Download from https://github.com/blackwell-systems/claudewatch/releases/latest
tar -xzf claudewatch_*_$(uname -s)_$(uname -m).tar.gz
sudo mv claudewatch /usr/local/bin/
```

**From source (requires Go 1.26+):**
```bash
go install github.com/blackwell-systems/claudewatch/cmd/claudewatch@latest
```

See [Installation Guide](/docs/guides/INSTALLATION.md) for detailed instructions and troubleshooting.

## Privacy

Zero network calls. Reads only local files under `~/.claude/`. Writes only to a local SQLite database for snapshot storage. No telemetry, no analytics, no crash reporting. Nothing leaves your machine.

## Related Projects

**[commitmux](https://github.com/blackwell-systems/commitmux)** - Semantic commit search across repositories. Find "when did we add authentication?" without remembering branch names or grep patterns.

**[scout-and-wave](https://github.com/blackwell-systems/scout-and-wave)** - Protocol for safely parallelizing human-guided agentic workflows. Orchestrator + Scout + Wave agents with explicit handoff contracts.

## License

Dual-licensed under [MIT](LICENSE) and [Apache 2.0](LICENSE-APACHE).

---

**Questions? Issues? Contributions?** Open an issue or PR. We respond to everything.
