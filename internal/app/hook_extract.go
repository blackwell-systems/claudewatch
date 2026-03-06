package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/memory"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// stateFilePath returns the path to the context pressure state file.
func stateFilePath() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "claudewatch-ctx-state")
	}
	return filepath.Join(os.Getenv("HOME"), ".cache", "claudewatch-ctx-state")
}

// isElevatedPressure returns true if the status is "pressure" or "critical".
func isElevatedPressure(status string) bool {
	return status == "pressure" || status == "critical"
}

// tryAutoExtract checks whether context pressure has transitioned to
// "pressure" or "critical" since the last check and performs memory
// extraction on transitions. Returns a human-readable message if extraction
// occurred, or "" if skipped/failed. Errors are swallowed.
func tryAutoExtract(activePath string, claudeHome string) string {
	// Read current pressure.
	ctx, err := claude.ParseLiveContextPressure(activePath)
	if err != nil {
		return ""
	}
	currentStatus := ctx.Status

	// Read previous status from state file.
	sf := stateFilePath()
	previousStatus := ""
	if data, err := os.ReadFile(sf); err == nil {
		previousStatus = strings.TrimSpace(string(data))
	}

	// Always write current status to state file.
	_ = os.MkdirAll(filepath.Dir(sf), 0o755)
	_ = os.WriteFile(sf, []byte(currentStatus), 0o644)

	// Detect transition: current is elevated AND previous was NOT elevated.
	if !isElevatedPressure(currentStatus) {
		return ""
	}
	if isElevatedPressure(previousStatus) {
		return "" // Already at elevated level, no transition.
	}

	// Transition detected — perform extraction.
	meta, err := claude.ParseActiveSession(activePath)
	if err != nil || meta == nil {
		return ""
	}

	projectName := filepath.Base(meta.ProjectPath)
	sessionID := strings.TrimSuffix(filepath.Base(activePath), ".jsonl")

	allSessions, _ := claude.ParseAllSessionMeta(claudeHome)
	allFacets, _ := claude.ParseAllFacets(claudeHome)

	// Find matching session and facet by sessionID.
	var matchedSession claude.SessionMeta
	var matchedFacet *claude.SessionFacet
	foundSession := false

	for _, s := range allSessions {
		if s.SessionID == sessionID {
			matchedSession = s
			foundSession = true
			break
		}
	}
	if !foundSession {
		// Use meta directly as session.
		matchedSession = *meta
	}

	for i := range allFacets {
		if allFacets[i].SessionID == sessionID {
			matchedFacet = &allFacets[i]
			break
		}
	}

	// If no facet found (common for very new sessions), return silently.
	if matchedFacet == nil {
		return ""
	}

	commits := memory.GetCommitSHAsSince(meta.ProjectPath, meta.StartTime)

	// Build transcript path for semantic extraction
	transcriptPath := ""
	if meta.ProjectPath != "" {
		projectHash := filepath.Base(meta.ProjectPath)
		transcriptPath = filepath.Join(claudeHome, "projects", projectHash, sessionID+".jsonl")
	}

	storePath := filepath.Join(config.ConfigDir(), "projects", projectName, "working-memory.json")
	memStore := store.NewWorkingMemoryStore(storePath)

	task, _ := memory.ExtractTaskMemory(matchedSession, matchedFacet, commits, transcriptPath)
	if task != nil {
		_ = memStore.AddOrUpdateTask(task)
	}

	blockers, _ := memory.ExtractBlockers(matchedSession, matchedFacet, projectName, allSessions, allFacets, transcriptPath)
	for _, b := range blockers {
		_ = memStore.AddBlocker(b)
	}

	blockerCount := len(blockers)

	if task != nil {
		return fmt.Sprintf("Auto-extracted memory (context at %s): task '%s' with %d blocker(s)", currentStatus, task.TaskIdentifier, blockerCount)
	}
	if blockerCount > 0 {
		return fmt.Sprintf("Auto-extracted memory (context at %s): %d blocker(s) saved", currentStatus, blockerCount)
	}

	return ""
}
