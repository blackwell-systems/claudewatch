# IMPL: Documentation Hub Structure

## Suitability Assessment

**Verdict: NOT SUITABLE**

**Estimated times:**
- Scout phase: ~8 min (analyzing 20+ files, creating coordination artifact)
- Agent execution: ~35 min (20 agents in 4 waves × ~2-3 min each, accounting for parallelism)
- Merge & verification: ~10 min
- **Total SAW time: ~53 min**

**Sequential baseline:** ~140 min (20 files × 7 min avg per file)
**Time savings:** ~87 min (62% faster)

**Recommendation:** SAW overhead dominates value for this workload.

### Why NOT SUITABLE

**1. Wrong kind of parallelism**

SAW is designed for code implementation with:
- Complex build/test cycles (>30s) that benefit from parallel execution
- Cross-agent dependencies requiring interface contracts
- Integration risk requiring careful merge verification

This work is:
- Pure content creation with zero build time
- No dependencies between files
- No integration risk (each file is independent)

**2. Task complexity mismatch**

Each documentation file is:
- Extract relevant sections from existing docs (cli.md, mcp.md, quickstart.md, CLAUDE.md)
- Reorganize into new structure per README navigation hub
- Add cross-links and formatting
- Estimate: 5-10 minutes per file

This is content extraction/reorganization, not complex implementation. SAW's coordination overhead (interface contracts, wave structure, completion reports) provides no value for tasks this straightforward.

**3. Better alternatives**

**Option A: Structured content plan** (Recommended)
- Create `docs/CONTENT_PLAN.md` mapping source → destination for each file
- Sequential creation in batches:
  - Wave 1: Features (7 files) - ~50 min
  - Wave 2: Guides (6 files) - ~40 min
  - Wave 3: Technical (4 files) - ~30 min
  - Wave 4: Comparison (3 files) - ~20 min
  - Wave 5: Contributing - ~10 min
- Total: ~150 min with tighter feedback loops
- Review checkpoint after each wave
- Adjust approach based on quality of early files

**Option B: Batch with rapid iteration**
- Create 3-4 sample files first (HOOKS.md, MCP_TOOLS.md, INSTALLATION.md)
- Get user feedback on structure, tone, cross-linking style
- Adjust approach based on feedback
- Complete remaining files with refined approach
- Total: ~2 hours with better quality through iteration

**4. Coordination value assessment**

SAW provides value through:
- ✅ Interface contracts (prevents conflicts) — NOT NEEDED: no cross-file dependencies
- ✅ Disjoint file ownership (prevents merge conflicts) — NOT NEEDED: files don't exist yet
- ✅ Parallel build/test execution (saves time) — NOT APPLICABLE: no build/test cycle
- ✅ Post-merge verification (catches integration failures) — NOT APPLICABLE: no integration to fail

The only SAW artifact with potential value is the **content plan** (what goes in each file). But this is better served by a lightweight outline document, not a full IMPL doc with interface contracts, wave structure, and completion report templates.

### Task characteristics

**Files to create:**

```
docs/features/
  - HOOKS.md
  - MCP_TOOLS.md
  - CLI.md
  - MEMORY.md
  - CONTEXT_SEARCH.md
  - METRICS.md
  - AGENTS.md

docs/guides/
  - INSTALLATION.md
  - QUICKSTART.md
  - CONFIGURATION.md
  - USE_CASE_FRICTION.md
  - USE_CASE_AGENTS.md
  - USE_CASE_CLAUDEMD.md

docs/technical/
  - ARCHITECTURE.md
  - HOOKS_IMPL.md
  - MCP_INTEGRATION.md
  - DATA_MODEL.md

docs/comparison/
  - VS_MEMORY_TOOLS.md
  - VS_OBSERVABILITY.md
  - VS_BUILTIN.md

docs/
  - CONTRIBUTING.md
```

**Existing sources:**
- `docs/cli.md` (647 lines) - complete CLI reference → extract to `features/CLI.md`
- `docs/mcp.md` (650 lines) - complete MCP tools reference → extract to `features/MCP_TOOLS.md`
- `docs/quickstart.md` (158 lines) - walkthrough → refine for `guides/QUICKSTART.md`
- `CLAUDE.md` (296 lines) - architecture, conventions → extract to `technical/` files
- `README.md` (284 lines) - feature summaries, positioning → extract to `comparison/` and `guides/`

**Content mapping:**

| New File | Primary Sources | Extraction Type |
|----------|----------------|-----------------|
| features/HOOKS.md | README lines 95-106, CLAUDE lines 17-20 | Expand with examples |
| features/MCP_TOOLS.md | docs/mcp.md (650 lines) | Restructure + categorize |
| features/CLI.md | docs/cli.md (647 lines) | Restructure |
| features/MEMORY.md | README lines 125-136, CLAUDE lines 21 | Expand cross-session learning |
| features/CONTEXT_SEARCH.md | README lines 167-179 | Expand unified search |
| features/METRICS.md | README lines 138-150, docs/cli.md metrics section | Consolidate |
| features/AGENTS.md | README lines 154-166, CLAUDE lines 99-119 | Expand agent analytics |
| guides/INSTALLATION.md | README lines 247-264 | Expand with troubleshooting |
| guides/QUICKSTART.md | docs/quickstart.md | Light restructure |
| guides/CONFIGURATION.md | README lines 85-96, cli.md config | Consolidate |
| guides/USE_CASE_FRICTION.md | README lines 220-244, quickstart | Narrative expansion |
| guides/USE_CASE_AGENTS.md | New content | Write from scratch |
| guides/USE_CASE_CLAUDEMD.md | New content | Write from scratch |
| technical/ARCHITECTURE.md | CLAUDE lines 15-32 | Expand three-layer model |
| technical/HOOKS_IMPL.md | CLAUDE lines 183 | Expand implementation |
| technical/MCP_INTEGRATION.md | mcp.md + CLAUDE lines 73-89 | Consolidate |
| technical/DATA_MODEL.md | CLAUDE lines 149-152, DATA_SOURCES.md | Extract + organize |
| comparison/VS_MEMORY_TOOLS.md | README lines 12-21 | Expand comparison table |
| comparison/VS_OBSERVABILITY.md | README lines 12-21 | Expand comparison table |
| comparison/VS_BUILTIN.md | New content | Write from scratch |
| CONTRIBUTING.md | CLAUDE lines 223-295 | Extract + add PR guidelines |

### Alternate approach: Content plan

Create `docs/CONTENT_PLAN.md` with this structure for each file:

```markdown
## features/HOOKS.md

**Purpose:** Deep dive on SessionStart briefings and PostToolUse alerts

**Primary sources:**
- README.md lines 95-106 (capability table)
- CLAUDE.md lines 17-20 (architecture section)
- CLAUDE.md lines 183 (hook cooldown mention)

**Structure:**
1. What hooks are (push observability)
2. SessionStart briefing (project health, friction rate, agent success)
3. PostToolUse alerts (error loops, context pressure, cost spikes, drift)
4. Configuration (enabling/disabling, rate limiting)
5. Examples with sample output

**Estimated time:** 8 minutes
```

Then create files sequentially with the plan as reference, adjusting as you go.

### Why sequential is better here

1. **Feedback loops:** After creating 3-4 files, user can review and request tone/structure adjustments. SAW locks in the approach for all 20 files upfront.

2. **Content evolution:** Writing one file often reveals better ways to structure the next. Sequential allows learning; SAW demands complete planning upfront.

3. **Quality over speed:** Documentation quality matters more than shipping fast. Taking 150 min to do it right beats 53 min to do it wrong.

4. **Low stakes:** Documentation is easy to revise. SAW's value is catching merge conflicts and integration failures - neither apply here.

### Conclusion

SAW is the wrong tool for this job. Use a content plan + sequential batched creation with review checkpoints.

The 87-minute time savings SAW provides is marginal compared to the coordination overhead and reduced flexibility. This work needs **structured planning**, not **parallelization**.

---

## Recommended Next Steps

1. Create `docs/CONTENT_PLAN.md` with detailed source mapping for each file
2. Create first batch: HOOKS.md, MCP_TOOLS.md, INSTALLATION.md (most important/referenced)
3. Get user feedback on quality, tone, cross-linking approach
4. Adjust plan based on feedback
5. Complete remaining files in batches with review between batches

Estimated total time: 2-2.5 hours with higher quality output and tighter feedback loops than SAW would provide.
