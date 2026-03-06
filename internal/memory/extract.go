package memory

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/store"
)

// ExtractTaskMemory converts a completed session into a TaskMemory entry.
// Works with or without facets - uses session metadata for inference when facets unavailable.
// Returns nil only if session has no identifiable task (no FirstPrompt, trivial session).
func ExtractTaskMemory(session claude.SessionMeta, facet *claude.SessionFacet, commits []string) (*store.TaskMemory, error) {
	// Skip trivial sessions with no clear task
	if session.FirstPrompt == "" && (session.GitCommits == 0 && session.FilesModified == 0 && session.ToolErrors < 3) {
		return nil, nil
	}

	taskID := DeriveTaskIdentifier(session, facet)

	var status string
	var blockersHit []string
	var solution string

	// Use facet data if available (rich AI analysis)
	if facet != nil {
		switch facet.Outcome {
		case "fully_achieved":
			status = "completed"
		case "not_achieved":
			status = "abandoned"
		default:
			status = "in_progress"
		}

		if facet.FrictionDetail != "" {
			blockersHit = append(blockersHit, facet.FrictionDetail)
		}

		if status == "completed" && len(commits) > 0 {
			solution = facet.BriefSummary
			if solution == "" {
				solution = facet.UnderlyingGoal
			}
		}
	} else {
		// Fallback: infer status from session metadata
		if session.GitCommits > 0 && session.FilesModified > 0 {
			status = "completed"
			// Infer solution from commit activity
			if len(commits) > 0 {
				solution = fmt.Sprintf("Made %d commit(s), modified %d file(s)", session.GitCommits, session.FilesModified)
			}
		} else if session.ToolErrors > 5 && session.GitCommits == 0 {
			status = "abandoned"
			blockersHit = append(blockersHit, fmt.Sprintf("%d tool errors without commits", session.ToolErrors))
		} else {
			status = "in_progress"
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
// Works with or without facets - uses session metadata for inference when facets unavailable.
// Severity thresholds:
// - Tool errors >= 5
// - Outcome == "not_achieved" + friction count > 0 (facet-only)
// - Chronic friction (>30% of recent sessions, facet-only)
// - No commits despite high activity (metadata fallback)
// - Specific tool error categories (metadata fallback)
func ExtractBlockers(session claude.SessionMeta, facet *claude.SessionFacet, projectName string, recentSessions []claude.SessionMeta, recentFacets []claude.SessionFacet) ([]*store.BlockerMemory, error) {
	var blockers []*store.BlockerMemory

	// Threshold 1: High tool errors (always check, facet not required)
	if session.ToolErrors >= 5 {
		issue := fmt.Sprintf("High tool error rate (%d errors)", session.ToolErrors)

		// Add error category breakdown if available
		if len(session.ToolErrorCategories) > 0 {
			var topErrors []string
			for cat, count := range session.ToolErrorCategories {
				if count > 0 {
					topErrors = append(topErrors, fmt.Sprintf("%s (%d)", cat, count))
				}
			}
			if len(topErrors) > 0 {
				issue = fmt.Sprintf("%s: %s", issue, strings.Join(topErrors, ", "))
			}
		}

		blockers = append(blockers, &store.BlockerMemory{
			File:        "",
			Issue:       issue,
			Solution:    "Review tool inputs and file paths before execution",
			Encountered: []string{time.Now().Format("2006-01-02")},
			LastSeen:    time.Now(),
		})
	}

	// Threshold 2: High activity but no commits (metadata-only detection)
	if facet == nil {
		totalToolCalls := 0
		for _, count := range session.ToolCounts {
			totalToolCalls += count
		}
		editWriteCalls := session.ToolCounts["Edit"] + session.ToolCounts["Write"]

		if totalToolCalls > 50 && editWriteCalls > 10 && session.GitCommits == 0 {
			blockers = append(blockers, &store.BlockerMemory{
				File:        "",
				Issue:       fmt.Sprintf("High activity (%d tools, %d edits) but zero commits", totalToolCalls, editWriteCalls),
				Solution:    "Review why changes weren't committed - tests failing? Incomplete work?",
				Encountered: []string{time.Now().Format("2006-01-02")},
				LastSeen:    time.Now(),
			})
		}
	}

	// Facet-only thresholds (richer analysis when available)
	if facet != nil {
		// Threshold 3: Not achieved outcome with friction
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

		// Threshold 4: Chronic friction patterns (>30% of recent sessions)
		if len(recentSessions) >= 3 {
			// Build session ID map for filtering
			sessionIDs := make(map[string]struct{})
			for _, s := range recentSessions {
				if filepath.Base(s.ProjectPath) == projectName {
					sessionIDs[s.SessionID] = struct{}{}
				}
			}

			// Count sessions with each friction type
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

			// Check if current session's friction is chronic
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

	// Fallback: use FirstPrompt directly (same normalization as facet.UnderlyingGoal)
	if session.FirstPrompt != "" {
		goal := strings.ToLower(strings.TrimSpace(session.FirstPrompt))
		if len(goal) > 100 {
			goal = goal[:100]
		}
		return goal
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

// GetCommitSHAsSince returns commit SHAs made after startTime in the given repo.
// Returns empty slice on error or if no commits found.
func GetCommitSHAsSince(repoPath, startTime string) []string {
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
