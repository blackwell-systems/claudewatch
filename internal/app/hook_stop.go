package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/memory"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "hook-stop",
	Short: "Prompt for memory extraction at session close",
	Long: `Detects significant sessions and prompts Claude to extract memory.

Significant session criteria (any of):
- Duration > 30 minutes
- Tool calls > 50
- Commits made > 0
- Errors encountered and resolved (>5 errors but friction not critical)

Skip conditions:
- Trivial session (< 10 min AND < 20 tool calls)
- Already checkpointed (extract_current_session_memory called)
- Pure research (zero Edit/Write calls)

Exit 0 always (non-blocking). Prints context-aware prompt to stderr if significant.

Intended for use as a Claude Code Stop shell hook.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run:           runHookStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

// stopHookInput is the JSON payload passed by Claude Code to Stop hooks via stdin.
type stopHookInput struct {
	SessionID      string `json:"session_id"`
	StopHookActive bool   `json:"stop_hook_active"`
	TranscriptPath string `json:"transcript_path"`
}

func runHookStop(cmd *cobra.Command, args []string) {
	// Read JSON input from stdin (passed by Claude Code)
	var input stopHookInput
	inputBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return
	}

	if err := json.Unmarshal(inputBytes, &input); err != nil {
		return
	}

	// Prevent infinite loop: if stop_hook_active is true, always allow stop
	if input.StopHookActive {
		return
	}

	// Read session-meta file which has full metadata (commits, tool counts, errors, duration)
	// Session-meta path: ~/.claude/usage-data/session-meta/{session_id}.json
	claudeHome := os.Getenv("CLAUDE_HOME")
	if claudeHome == "" {
		claudeHome = os.ExpandEnv("$HOME/.claude")
	}

	metaPath := claudeHome + "/usage-data/session-meta/" + input.SessionID + ".json"
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		// Session-meta not available yet (very recent session)
		return
	}

	var meta claude.SessionMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return
	}

	// Check if session should be skipped or is insignificant
	if shouldSkipSession(&meta, input.TranscriptPath) || !isSignificant(&meta) {
		return
	}

	// Try auto-extraction (never block - facet usually not ready at Stop time)
	extracted, missingFacet := autoExtractMemory(claudeHome, input.SessionID, &meta)

	if extracted {
		// Success! Memory extracted automatically
		fmt.Fprintln(os.Stderr, "✓ Session memory extracted automatically")
	} else if missingFacet {
		// Facet not ready yet (expected for fresh sessions) - log gentle reminder
		fmt.Fprintln(os.Stderr, "ℹ Memory extraction pending (AI analysis not ready). Run 'claudewatch memory extract' after session closes.")
	} else {
		// Other error (e.g., git failure) - allow closing but log
		fmt.Fprintln(os.Stderr, "⚠ Memory extraction skipped (non-critical error)")
	}

	// Always exit 0 - never block session close
}

// autoExtractMemory extracts task and blocker memory from the given session.
// Returns (extracted, missingFacet) where:
// - extracted: true if memory was successfully extracted
// - missingFacet: true if extraction failed because facet (AI analysis) doesn't exist yet
func autoExtractMemory(claudeHome, sessionID string, meta *claude.SessionMeta) (bool, bool) {
	// Load all sessions for this project to get context
	allSessions, err := claude.ParseAllSessionMeta(claudeHome)
	if err != nil {
		return false, false
	}

	projectName := filepath.Base(meta.ProjectPath)

	// Filter to project sessions
	var projectSessions []claude.SessionMeta
	for _, sess := range allSessions {
		if filepath.Base(sess.ProjectPath) == projectName {
			projectSessions = append(projectSessions, sess)
		}
	}

	// Load facet (AI analysis) for this session
	allFacets, err := claude.ParseAllFacets(claudeHome)
	if err != nil {
		return false, false
	}

	var sessionFacet *claude.SessionFacet
	for i := range allFacets {
		if allFacets[i].SessionID == sessionID {
			sessionFacet = &allFacets[i]
			break
		}
	}

	if sessionFacet == nil {
		// Session too new, facet not written yet - return missingFacet=true
		return false, true
	}

	// Extract commits from git
	commits := memory.GetCommitSHAsSince(meta.ProjectPath, meta.StartTime)

	// Open working memory store
	storePath := filepath.Join(config.ConfigDir(), "projects", projectName, "working-memory.json")
	memStore := store.NewWorkingMemoryStore(storePath)

	// Extract task memory
	task, err := memory.ExtractTaskMemory(*meta, sessionFacet, commits)
	if err == nil && task != nil {
		_ = memStore.AddOrUpdateTask(task)
	}

	// Extract blockers (use last 10 sessions for context)
	recentSessions := projectSessions
	if len(recentSessions) > 10 {
		recentSessions = recentSessions[:10]
	}

	blockers, err := memory.ExtractBlockers(*meta, sessionFacet, projectName, recentSessions, allFacets)
	if err == nil && len(blockers) > 0 {
		for _, blocker := range blockers {
			_ = memStore.AddBlocker(blocker)
		}
	}

	return true, false
}

// shouldSkipSession returns true if the session should not prompt for extraction.
func shouldSkipSession(meta *claude.SessionMeta, activePath string) bool {
	duration := computeDuration(meta)
	toolCalls := sumToolCalls(meta.ToolCounts)

	// Trivial: < 10 min AND < 20 tool calls
	if duration < 10 && toolCalls < 20 {
		return true
	}

	// Already checkpointed: extract called
	if wasRecentlyCheckpointed(meta, activePath) {
		return true
	}

	// Pure research: zero Edit/Write calls
	editWrites := meta.ToolCounts["Edit"] + meta.ToolCounts["Write"]
	return editWrites == 0
}

// isSignificant returns true if the session meets any significance criteria.
func isSignificant(meta *claude.SessionMeta) bool {
	duration := computeDuration(meta)
	toolCalls := sumToolCalls(meta.ToolCounts)

	if duration > 30 {
		return true
	}
	if toolCalls > 50 {
		return true
	}
	if meta.GitCommits > 0 {
		return true
	}
	// Errors resolved: >5 errors but not abandoned
	if meta.ToolErrors > 5 {
		// Check facet outcome if available
		// Consider "resolved" if friction exists but work continued
		return true
	}

	return false
}

// computeDuration returns session duration in minutes.
func computeDuration(meta *claude.SessionMeta) float64 {
	if meta.DurationMinutes > 0 {
		return float64(meta.DurationMinutes)
	}
	// For live sessions where DurationMinutes isn't set yet,
	// return 0 (will be handled by other significance checks)
	return 0
}

// sumToolCalls returns total tool calls across all tools.
func sumToolCalls(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

// wasRecentlyCheckpointed returns true if extract_current_session_memory was called.
func wasRecentlyCheckpointed(meta *claude.SessionMeta, activePath string) bool {
	// Simple heuristic: if extract was called at all, consider it checkpointed.
	// Future enhancement: parse JSONL to check if within last 20 minutes.
	extractCount := meta.ToolCounts["extract_current_session_memory"]
	return extractCount > 0
}

// determinePrompt generates a context-aware prompt based on session characteristics.
func determinePrompt(meta *claude.SessionMeta, activePath string) string {
	if shouldSkipSession(meta, activePath) {
		return ""
	}

	if !isSignificant(meta) {
		return ""
	}

	// Determine session outcome
	commits := meta.GitCommits
	errors := meta.ToolErrors
	duration := int(computeDuration(meta))
	toolCalls := sumToolCalls(meta.ToolCounts)

	// Task completed (commits > 0)
	if commits > 0 {
		if duration > 0 {
			return fmt.Sprintf("✓ Session completed with %d commit(s) in %d minutes. Extract memory for future sessions? Call extract_current_session_memory", commits, duration)
		}
		return fmt.Sprintf("✓ Session completed with %d commit(s). Extract memory for future sessions? Call extract_current_session_memory", commits)
	}

	// Task abandoned (zero commits, high tool errors)
	if commits == 0 && errors > 5 {
		return fmt.Sprintf("⚠ Session ended with zero commits and %d tool errors. Worth extracting blockers? Call extract_current_session_memory", errors)
	}

	// Task in-progress (active work, no clear resolution)
	if duration > 0 {
		return fmt.Sprintf("📋 Session has significant work in progress (%d tool calls, %d min). Extract checkpoint before closing? Call extract_current_session_memory", toolCalls, duration)
	}
	return fmt.Sprintf("📋 Session has significant work in progress (%d tool calls). Extract checkpoint before closing? Call extract_current_session_memory", toolCalls)
}
