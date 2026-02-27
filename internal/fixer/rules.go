package fixer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

// rule is a function that examines a FixContext and returns zero or more
// proposed additions. Each rule is independent and focuses on a single
// type of improvement.
type rule func(ctx *FixContext) []Addition

// ruleMissingBuildCommands generates a "## Build & Test" section when the
// CLAUDE.md lacks build commands and the tool profile shows significant Bash usage.
func ruleMissingBuildCommands(ctx *FixContext) []Addition {
	// Skip if build section already exists.
	if hasSection(ctx.ExistingClaudeMD, "build", "compile", "make") {
		return nil
	}

	// Need tool profile data showing Bash usage.
	if ctx.ToolProfile == nil || ctx.ToolProfile.BashRatio < 0.10 {
		return nil
	}

	// Extract common commands from session tool counts.
	commands := extractBuildCommands(ctx.Sessions)
	if len(commands) == 0 {
		return nil
	}

	// Build the code block content.
	var sb strings.Builder
	sb.WriteString("```bash\n")
	for _, cmd := range commands {
		sb.WriteString(cmd)
		sb.WriteString("\n")
	}
	sb.WriteString("```")

	sessionCount := len(ctx.Sessions)
	bashPct := int(ctx.ToolProfile.BashRatio * 100)

	return []Addition{
		{
			Section:    "## Build & Test",
			Content:    sb.String(),
			Reason:     fmt.Sprintf("Bash usage is %d%% of tool calls across %d sessions, but no build commands are documented.", bashPct, sessionCount),
			Impact:     "Projects with build sections have 30% less friction on average.",
			Source:     "missing_build_commands",
			Confidence: confidenceFromSessionCount(sessionCount),
		},
	}
}

// rulePlanModeWarning adds a plan mode warning to Conventions when agent analysis
// shows Plan agents with a high kill rate.
func rulePlanModeWarning(ctx *FixContext) []Addition {
	if len(ctx.AgentTasks) == 0 {
		return nil
	}

	// Count Plan-type agents and their kill rate.
	var planTotal, planKilled int
	for _, task := range ctx.AgentTasks {
		agentType := strings.ToLower(task.AgentType)
		if strings.Contains(agentType, "plan") {
			planTotal++
			if task.Status == "killed" {
				planKilled++
			}
		}
	}

	if planTotal == 0 {
		return nil
	}

	killRate := float64(planKilled) / float64(planTotal)
	if killRate < 0.30 {
		return nil
	}

	killPct := int(killRate * 100)

	return []Addition{
		{
			Section:    "## Conventions",
			Content:    "- Do not enter plan mode for this project. Implement directly.",
			Reason:     fmt.Sprintf("Plan agents have %d%% kill rate across %d plan tasks in this project.", killPct, planTotal),
			Impact:     "Eliminating wasted plan cycles reduces session time and user frustration.",
			Source:     "plan_mode_warning",
			Confidence: confidenceFromSessionCount(len(ctx.Sessions)),
		},
	}
}

// ruleKnownFrictionPatterns generates a "## Known Patterns" section from stale
// friction that has persisted for 3+ weeks without improving.
func ruleKnownFrictionPatterns(ctx *FixContext) []Addition {
	if ctx.FrictionPatterns == nil || ctx.FrictionPatterns.StaleCount == 0 {
		return nil
	}

	// Skip if a known patterns section already exists.
	if hasSection(ctx.ExistingClaudeMD, "known pattern", "known issue", "gotcha", "pitfall") {
		return nil
	}

	var warnings []string
	for _, pattern := range ctx.FrictionPatterns.Patterns {
		if !pattern.Stale {
			continue
		}
		frictionLabel := humanizeFrictionType(pattern.FrictionType)
		warnings = append(warnings, fmt.Sprintf(
			"- **%s** — recurring for %d consecutive weeks across %d sessions. Watch for this pattern and take extra care.",
			frictionLabel,
			pattern.ConsecutiveWeeks,
			pattern.OccurrenceCount,
		))
	}

	if len(warnings) == 0 {
		return nil
	}

	return []Addition{
		{
			Section:    "## Known Patterns",
			Content:    strings.Join(warnings, "\n"),
			Reason:     fmt.Sprintf("%d friction patterns have persisted for 3+ weeks without improvement.", ctx.FrictionPatterns.StaleCount),
			Impact:     "Documenting known friction patterns helps Claude avoid repeating mistakes.",
			Source:     "known_friction_patterns",
			Confidence: confidenceFromSessionCount(len(ctx.Sessions)),
		},
	}
}

// ruleScopeConstraints adds scope constraint guidance to Conventions when
// the correction rate is high.
func ruleScopeConstraints(ctx *FixContext) []Addition {
	if ctx.ConversationData == nil {
		return nil
	}

	if ctx.ConversationData.AvgCorrectionRate < 0.3 {
		return nil
	}

	corrPct := int(ctx.ConversationData.AvgCorrectionRate * 100)

	return []Addition{
		{
			Section:    "## Conventions",
			Content:    "- Make only the changes explicitly requested. Do not add, remove, or restructure sections not mentioned in the prompt.",
			Reason:     fmt.Sprintf("Correction rate is %d%% — you frequently redirect Claude in this project.", corrPct),
			Impact:     "Scope constraints reduce over-eager changes and correction cycles.",
			Source:     "scope_constraints",
			Confidence: confidenceFromSessionCount(len(ctx.Sessions)),
		},
	}
}

// ruleMissingTestingSection generates a "## Testing" section when the CLAUDE.md
// lacks a testing section and session data shows test-related commands.
func ruleMissingTestingSection(ctx *FixContext) []Addition {
	if hasSection(ctx.ExistingClaudeMD, "test", "testing") {
		return nil
	}

	// Look for test commands in session tool usage.
	testCommands := extractTestCommands(ctx.Sessions)
	if len(testCommands) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("```bash\n")
	for _, cmd := range testCommands {
		sb.WriteString(cmd)
		sb.WriteString("\n")
	}
	sb.WriteString("```")

	return []Addition{
		{
			Section:    "## Testing",
			Content:    sb.String(),
			Reason:     "Test commands detected in session data but not documented in CLAUDE.md.",
			Impact:     "Documented test commands reduce friction and enable Claude to verify changes.",
			Source:     "missing_testing_section",
			Confidence: confidenceFromSessionCount(len(ctx.Sessions)),
		},
	}
}

// ruleMissingArchitectureSection generates an "## Architecture" stub when the
// project has significant session history but no architecture section.
func ruleMissingArchitectureSection(ctx *FixContext) []Addition {
	if hasSection(ctx.ExistingClaudeMD, "architecture", "structure", "layout", "overview", "organization") {
		return nil
	}

	// Only suggest for projects with enough sessions to justify the effort.
	if len(ctx.Sessions) < 10 {
		return nil
	}

	// Detect project type for a tailored stub.
	content := generateArchitectureStub(ctx.Project)
	if content == "" {
		return nil
	}

	return []Addition{
		{
			Section:    "## Architecture",
			Content:    content,
			Reason:     fmt.Sprintf("This project has %d sessions but no architecture documentation. Claude performs better with structural context.", len(ctx.Sessions)),
			Impact:     "Architecture sections reduce exploratory Read/Grep cycles at the start of sessions.",
			Source:     "missing_architecture_section",
			Confidence: confidenceFromSessionCount(len(ctx.Sessions)),
		},
	}
}

// ruleActionBias adds an action bias instruction when the zero-commit rate
// is above 50%.
func ruleActionBias(ctx *FixContext) []Addition {
	if ctx.CommitAnalysis == nil {
		return nil
	}

	if ctx.CommitAnalysis.ZeroCommitRate < 0.50 {
		return nil
	}

	zeroPct := int(ctx.CommitAnalysis.ZeroCommitRate * 100)

	return []Addition{
		{
			Section:    "## Conventions",
			Content:    "- Bias toward implementation. Start writing code immediately rather than planning or analyzing.",
			Reason:     fmt.Sprintf("%d%% of sessions in this project produce zero commits.", zeroPct),
			Impact:     "Action bias reduces unproductive exploration sessions.",
			Source:     "action_bias",
			Confidence: confidenceFromSessionCount(len(ctx.Sessions)),
		},
	}
}

// extractBuildCommands scans session metadata for common build-related Bash
// commands. It returns a deduplicated, sorted list of the most frequently
// observed commands.
func extractBuildCommands(sessions []claude.SessionMeta) []string {
	// We look at session-level tool counts and first prompts for build patterns.
	// Since we don't have raw Bash command text in session meta, we infer from
	// the project language and common patterns.
	commandFreq := make(map[string]int)

	for _, s := range sessions {
		// Check first prompt for build-related commands.
		prompt := strings.ToLower(s.FirstPrompt)

		// Infer from detected languages.
		for lang := range s.Languages {
			switch strings.ToLower(lang) {
			case "go":
				commandFreq["go build ./..."]++
				commandFreq["go test ./..."]++
				if strings.Contains(prompt, "lint") {
					commandFreq["golangci-lint run ./..."]++
				}
			case "python":
				commandFreq["python -m pytest"]++
				if strings.Contains(prompt, "lint") || strings.Contains(prompt, "format") {
					commandFreq["ruff check ."]++
				}
			case "javascript", "typescript":
				commandFreq["npm run build"]++
				commandFreq["npm test"]++
				if strings.Contains(prompt, "lint") {
					commandFreq["npm run lint"]++
				}
			case "rust":
				commandFreq["cargo build"]++
				commandFreq["cargo test"]++
				if strings.Contains(prompt, "lint") {
					commandFreq["cargo clippy"]++
				}
			}
		}
	}

	// Also check tool counts for Bash presence as a signal.
	bashSessions := 0
	for _, s := range sessions {
		if s.ToolCounts["Bash"] > 0 {
			bashSessions++
		}
	}

	if len(commandFreq) == 0 {
		return nil
	}

	// Sort by frequency and take top commands.
	type cmdCount struct {
		cmd   string
		count int
	}
	var sorted []cmdCount
	for cmd, count := range commandFreq {
		sorted = append(sorted, cmdCount{cmd, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	// Deduplicate test commands (they'll appear in both build and test rules).
	seen := make(map[string]bool)
	var result []string
	limit := 5
	for _, sc := range sorted {
		if seen[sc.cmd] {
			continue
		}
		seen[sc.cmd] = true
		result = append(result, sc.cmd)
		if len(result) >= limit {
			break
		}
	}

	return result
}

// extractTestCommands scans sessions for test-related commands specifically.
func extractTestCommands(sessions []claude.SessionMeta) []string {
	commandFreq := make(map[string]int)

	for _, s := range sessions {
		for lang := range s.Languages {
			switch strings.ToLower(lang) {
			case "go":
				commandFreq["go test ./..."]++
				commandFreq["go test -v ./..."]++
			case "python":
				commandFreq["python -m pytest"]++
				commandFreq["python -m pytest -v"]++
			case "javascript", "typescript":
				commandFreq["npm test"]++
			case "rust":
				commandFreq["cargo test"]++
			}
		}
	}

	if len(commandFreq) == 0 {
		return nil
	}

	// Sort by frequency.
	type cmdCount struct {
		cmd   string
		count int
	}
	var sorted []cmdCount
	for cmd, count := range commandFreq {
		sorted = append(sorted, cmdCount{cmd, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	var result []string
	limit := 3
	for _, sc := range sorted {
		result = append(result, sc.cmd)
		if len(result) >= limit {
			break
		}
	}

	return result
}

// generateArchitectureStub creates a project-type-aware architecture stub
// based on the project's detected language and common file structure.
func generateArchitectureStub(project scanner.Project) string {
	switch project.PrimaryLanguage {
	case "Go":
		return "This is a Go project.\n\n" +
			"Key directories:\n" +
			"- `cmd/` — Application entry points\n" +
			"- `internal/` — Private application packages\n" +
			"- `pkg/` — Public library packages (if applicable)\n\n" +
			"TODO: Fill in the specific package layout for this project."
	case "Python":
		return "This is a Python project.\n\n" +
			"Key directories:\n" +
			"- `src/` — Source packages\n" +
			"- `tests/` — Test files\n\n" +
			"TODO: Fill in the specific module layout for this project."
	case "JavaScript", "TypeScript":
		return "This is a JavaScript/TypeScript project.\n\n" +
			"Key directories:\n" +
			"- `src/` — Source files\n" +
			"- `test/` or `__tests__/` — Test files\n\n" +
			"TODO: Fill in the specific directory layout for this project."
	case "Rust":
		return "This is a Rust project.\n\n" +
			"Key directories:\n" +
			"- `src/` — Source files\n" +
			"- `tests/` — Integration tests\n\n" +
			"TODO: Fill in the specific module layout for this project."
	default:
		return "TODO: Document the project architecture and directory layout here."
	}
}

// humanizeFrictionType converts a snake_case friction type identifier into
// a human-readable label.
func humanizeFrictionType(frictionType string) string {
	// Replace underscores with spaces and capitalize first letter.
	label := strings.ReplaceAll(frictionType, "_", " ")
	if len(label) > 0 {
		label = strings.ToUpper(label[:1]) + label[1:]
	}
	return label
}
