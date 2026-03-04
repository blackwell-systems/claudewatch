package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// ExtractTaskMemory converts a completed session into a TaskMemory entry.
// Returns nil if session has insufficient data (no facet, no identifiable task).
func ExtractTaskMemory(session claude.SessionMeta, facet *claude.SessionFacet, commits []string) (*store.TaskMemory, error) {
	if facet == nil {
		return nil, nil
	}

	taskID := DeriveTaskIdentifier(session, facet)

	// Determine status from outcome.
	var status string
	switch facet.Outcome {
	case "fully_achieved":
		status = "completed"
	case "not_achieved":
		status = "abandoned"
	default:
		status = "in_progress"
	}

	// Extract blockers from friction detail.
	var blockersHit []string
	if facet.FrictionDetail != "" {
		blockersHit = append(blockersHit, facet.FrictionDetail)
	}

	// Populate solution only if completed AND commits > 0.
	var solution string
	if status == "completed" && len(commits) > 0 {
		solution = facet.BriefSummary
		if solution == "" {
			solution = facet.UnderlyingGoal
		}
	}

	return &store.TaskMemory{
		TaskIdentifier: taskID,
		Sessions:       []string{session.SessionID},
		Status:         status,
		BlockersHit:    blockersHit,
		Solution:       solution,
		Commits:        commits,
		LastUpdated:    time.Now(),
	}, nil
}

// ExtractBlockers analyzes friction data and returns blocker entries.
// Severity thresholds:
// - Consecutive tool errors >= 3
// - Outcome == "not_achieved" + friction count > 0
// - Chronic friction (>30% of recent sessions)
func ExtractBlockers(session claude.SessionMeta, facet *claude.SessionFacet, projectName string, recentSessions []claude.SessionMeta, recentFacets []claude.SessionFacet) ([]*store.BlockerMemory, error) {
	if facet == nil {
		return nil, nil
	}

	var blockers []*store.BlockerMemory

	// Threshold 1: High tool errors.
	if session.ToolErrors >= 5 {
		blockers = append(blockers, &store.BlockerMemory{
			File:        "",
			Issue:       fmt.Sprintf("High tool error rate (%d errors)", session.ToolErrors),
			Solution:    "Review tool inputs and file paths before execution",
			Encountered: []string{time.Now().Format("2006-01-02")},
			LastSeen:    time.Now(),
		})
	}

	// Threshold 2: Not achieved outcome with friction.
	if facet.Outcome == "not_achieved" && len(facet.FrictionCounts) > 0 {
		if facet.FrictionDetail != "" {
			blockers = append(blockers, &store.BlockerMemory{
				File:        "",
				Issue:       fmt.Sprintf("Session abandoned: %s", facet.FrictionDetail),
				Solution:    "",
				Encountered: []string{time.Now().Format("2006-01-02")},
				LastSeen:    time.Now(),
			})
		}
	}

	// Threshold 3: Chronic friction patterns (>30% of recent sessions).
	if len(recentSessions) >= 3 {
		// Build session ID map for filtering.
		sessionIDs := make(map[string]struct{})
		for _, s := range recentSessions {
			if filepath.Base(s.ProjectPath) == projectName {
				sessionIDs[s.SessionID] = struct{}{}
			}
		}

		// Count sessions with each friction type.
		frictionSessionCount := make(map[string]int)
		for _, f := range recentFacets {
			if _, ok := sessionIDs[f.SessionID]; !ok {
				continue
			}
			seen := make(map[string]bool)
			for ft, count := range f.FrictionCounts {
				if count > 0 && !seen[ft] {
					seen[ft] = true
					frictionSessionCount[ft]++
				}
			}
		}

		// Check if current session's friction is chronic.
		for ft, count := range facet.FrictionCounts {
			if count == 0 {
				continue
			}
			sessionCount := len(recentSessions)
			rate := float64(frictionSessionCount[ft]) / float64(sessionCount)
			if rate > 0.3 {
				issue := fmt.Sprintf("Chronic friction: %s (%.0f%% of recent sessions)", ft, rate*100)
				solution := suggestSolutionForFriction(ft)
				blockers = append(blockers, &store.BlockerMemory{
					File:        "",
					Issue:       issue,
					Solution:    solution,
					Encountered: []string{time.Now().Format("2006-01-02")},
					LastSeen:    time.Now(),
				})
			}
		}
	}

	return blockers, nil
}

// DeriveTaskIdentifier produces stable task identifier.
// Priority: facet.UnderlyingGoal > hash(FirstPrompt+ProjectPath) > SessionID
func DeriveTaskIdentifier(session claude.SessionMeta, facet *claude.SessionFacet) string {
	if facet != nil && facet.UnderlyingGoal != "" {
		// Normalize: lowercase, trim, limit length.
		goal := strings.ToLower(strings.TrimSpace(facet.UnderlyingGoal))
		if len(goal) > 100 {
			goal = goal[:100]
		}
		return goal
	}

	// Fallback: hash FirstPrompt + ProjectPath.
	if session.FirstPrompt != "" {
		input := session.FirstPrompt + session.ProjectPath
		hash := sha256.Sum256([]byte(input))
		return "task-" + hex.EncodeToString(hash[:8])
	}

	// Last resort: session ID.
	return session.SessionID
}

// suggestSolutionForFriction returns a suggested solution for common friction types.
func suggestSolutionForFriction(frictionType string) string {
	switch {
	case strings.HasPrefix(frictionType, "retry:Bash"):
		return "Verify bash commands before running; check paths and permissions"
	case strings.HasPrefix(frictionType, "retry:Edit"):
		return "Read files before editing to confirm structure"
	case strings.HasPrefix(frictionType, "retry:Read"):
		return "Verify file paths exist before reading"
	case strings.Contains(frictionType, "tool_error"):
		return "Check tool inputs carefully; review error messages"
	default:
		return ""
	}
}

// getCommitSHAsSince returns commit SHAs made after startTime in the given repo.
// Returns empty slice on error or if no commits found.
func getCommitSHAsSince(repoPath, startTime string) []string {
	if repoPath == "" || startTime == "" {
		return nil
	}

	cmd := exec.Command("git", "-C", repoPath, "log", "--format=%H", "--after="+startTime)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}

	return lines
}
