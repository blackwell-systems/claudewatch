// Package fixer provides rule-based and AI-powered generation of CLAUDE.md
// additions from observed session data. It analyzes friction patterns, tool
// usage, commit history, and conversation signals to produce concrete,
// actionable additions to a project's CLAUDE.md file.
package fixer

import (
	"fmt"
	"os"
	"strings"
)

// ProposedFix holds the complete set of proposed CLAUDE.md additions for a project.
type ProposedFix struct {
	ProjectPath  string     `json:"project_path"`
	ProjectName  string     `json:"project_name"`
	CurrentScore int        `json:"current_score"`
	Additions    []Addition `json:"additions"`
}

// Addition represents a single proposed CLAUDE.md section or content block.
type Addition struct {
	Section    string  `json:"section"`    // e.g., "## Build & Test", "## Conventions"
	Content    string  `json:"content"`    // the actual markdown to add
	Reason     string  `json:"reason"`     // why this is being suggested
	Impact     string  `json:"impact"`     // expected impact description
	Source     string  `json:"source"`     // which rule produced this suggestion
	Confidence float64 `json:"confidence"` // 0-1, based on data strength
}

// GenerateFix analyzes the FixContext and returns a ProposedFix containing
// all applicable additions. Each rule is applied independently and may produce
// zero or more additions. Additions for sections that already exist in the
// current CLAUDE.md are skipped.
//
// When opts is non-nil and opts.UseAI is true, the function first runs
// rule-based generation as a baseline, then calls the Claude API for
// AI-generated additions. AI additions take precedence over rule-based
// additions for the same section header. If the API call fails, it falls
// back to rule-based results only.
func GenerateFix(ctx *FixContext, opts *FixOptions) (*ProposedFix, error) {
	if ctx == nil {
		return nil, fmt.Errorf("fix context is nil")
	}

	fix := &ProposedFix{
		ProjectPath:  ctx.Project.Path,
		ProjectName:  ctx.Project.Name,
		CurrentScore: int(ctx.Project.Score),
	}

	// Apply all rules in priority order (baseline).
	rules := []rule{
		ruleMissingBuildCommands,
		rulePlanModeWarning,
		ruleKnownFrictionPatterns,
		ruleScopeConstraints,
		ruleMissingTestingSection,
		ruleMissingArchitectureSection,
		ruleActionBias,
	}

	for _, r := range rules {
		additions := r(ctx)
		fix.Additions = append(fix.Additions, additions...)
	}

	// Merge rule-based additions that target the same section header.
	fix.Additions = mergeAdditions(fix.Additions)

	// If AI mode is enabled, generate AI additions and merge them in.
	if opts != nil && opts.UseAI {
		aiAdditions, err := GenerateAIFix(ctx, opts.APIKey, opts.Model)
		if err != nil {
			// Log the error but fall back to rule-based results.
			fmt.Fprintf(os.Stderr, "  Warning: AI generation failed, using rule-based results: %v\n", err)
		} else if len(aiAdditions) > 0 {
			fix.Additions = mergeAIAdditions(fix.Additions, aiAdditions)
		}
	}

	return fix, nil
}

// mergeAIAdditions combines rule-based and AI-generated additions. For sections
// that exist in both sets, the AI addition takes precedence. Rule-based additions
// for sections not covered by AI are preserved.
func mergeAIAdditions(ruleBased, aiGenerated []Addition) []Addition {
	// Index AI additions by normalized section header.
	aiBySection := make(map[string]Addition)
	aiOrder := make([]string, 0)
	for _, a := range aiGenerated {
		key := strings.ToLower(strings.TrimSpace(a.Section))
		if _, exists := aiBySection[key]; !exists {
			aiOrder = append(aiOrder, key)
		}
		aiBySection[key] = a
	}

	// Build result: keep rule-based additions unless AI covers the same section.
	coveredByAI := make(map[string]bool)
	var result []Addition

	for _, a := range ruleBased {
		key := strings.ToLower(strings.TrimSpace(a.Section))
		if _, hasAI := aiBySection[key]; hasAI {
			coveredByAI[key] = true
			continue // AI version will be added later.
		}
		result = append(result, a)
	}

	// Add all AI additions (they take precedence for overlapping sections).
	for _, key := range aiOrder {
		result = append(result, aiBySection[key])
	}

	return result
}

// mergeAdditions combines additions that target the same section header into
// a single addition with concatenated content and reasons.
func mergeAdditions(additions []Addition) []Addition {
	sectionOrder := make([]string, 0)
	bySection := make(map[string][]Addition)

	for _, a := range additions {
		if _, exists := bySection[a.Section]; !exists {
			sectionOrder = append(sectionOrder, a.Section)
		}
		bySection[a.Section] = append(bySection[a.Section], a)
	}

	merged := make([]Addition, 0, len(sectionOrder))
	for _, section := range sectionOrder {
		group := bySection[section]
		if len(group) == 1 {
			merged = append(merged, group[0])
			continue
		}

		// Merge multiple additions for the same section.
		var contentParts []string
		var reasonParts []string
		var sources []string
		bestConfidence := 0.0

		for _, a := range group {
			contentParts = append(contentParts, a.Content)
			reasonParts = append(reasonParts, a.Reason)
			sources = append(sources, a.Source)
			if a.Confidence > bestConfidence {
				bestConfidence = a.Confidence
			}
		}

		merged = append(merged, Addition{
			Section:    section,
			Content:    strings.Join(contentParts, "\n"),
			Reason:     strings.Join(reasonParts, " "),
			Impact:     group[0].Impact,
			Source:     strings.Join(sources, ", "),
			Confidence: bestConfidence,
		})
	}

	return merged
}

// RenderMarkdown produces the complete markdown text to append to CLAUDE.md.
// If the project has no existing CLAUDE.md, it includes a top-level header.
func RenderMarkdown(fix *ProposedFix, hasExisting bool) string {
	var sb strings.Builder

	if !hasExisting {
		fmt.Fprintf(&sb, "# %s\n\n", fix.ProjectName)
		sb.WriteString("Claude Code instructions for this project.\n\n")
	}

	for _, a := range fix.Additions {
		sb.WriteString("\n")
		sb.WriteString(a.Section)
		sb.WriteString("\n\n")
		sb.WriteString(a.Content)
		sb.WriteString("\n")
	}

	return sb.String()
}
