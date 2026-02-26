// Package fixer provides rule-based and AI-powered generation of CLAUDE.md additions.
package fixer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	claudeAPIURL   = "https://api.anthropic.com/v1/messages"
	claudeAPIVersion = "2023-06-01"
	defaultModel   = "claude-sonnet-4-20250514"
	maxTokens      = 4096
	apiTimeout     = 60 * time.Second
)

// FixOptions controls whether AI generation is used and with what configuration.
type FixOptions struct {
	UseAI  bool
	APIKey string
	Model  string
}

// aiSystemPrompt is the system prompt sent to Claude for generating CLAUDE.md content.
const aiSystemPrompt = `You are an expert at writing CLAUDE.md files — instruction files that Claude Code reads at the start of every session to understand a project's conventions, build system, and working patterns.

You are given analytics about how a developer has worked with Claude Code on a specific project. Your job is to generate CLAUDE.md sections that will reduce friction and improve productivity in future sessions.

Rules:
- Be specific to THIS project. Reference actual file paths, package names, and patterns from the project structure.
- If build/test commands are common in session data, document them exactly.
- If friction patterns like "wrong_approach" are recurring, write conventions that prevent them.
- If the developer frequently corrects Claude, add constraints that prevent those corrections.
- If sessions often produce zero commits, add a bias-toward-implementation convention.
- Do NOT generate generic boilerplate. Every line should be grounded in the data provided.
- Output valid JSON matching the schema below.

Output schema:
{
  "additions": [
    {
      "section": "## Section Name",
      "content": "The markdown content for this section",
      "reason": "Why this section is needed, grounded in data"
    }
  ]
}`

// GenerateAIFix takes a FixContext, builds a prompt from the analyzed data,
// calls the Claude API, and returns project-specific Addition entries.
func GenerateAIFix(ctx *FixContext, apiKey string, model string) ([]Addition, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required for AI fix generation")
	}
	if model == "" {
		model = defaultModel
	}

	userPrompt := buildUserPrompt(ctx)

	responseText, err := callClaudeAPI(apiKey, model, aiSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("calling Claude API: %w", err)
	}

	additions, err := parseAIResponse(responseText)
	if err != nil {
		return nil, fmt.Errorf("parsing AI response: %w", err)
	}

	// Set metadata on each addition.
	confidence := confidenceFromSessionCount(len(ctx.Sessions))
	for i := range additions {
		additions[i].Source = "ai_generation"
		additions[i].Confidence = confidence
		if additions[i].Impact == "" {
			additions[i].Impact = "AI-generated section based on observed session patterns."
		}
	}

	return additions, nil
}

// buildUserPrompt constructs the user message from the FixContext, including
// project metadata, session statistics, friction data, and project structure.
func buildUserPrompt(ctx *FixContext) string {
	var sb strings.Builder

	// Project overview.
	sb.WriteString("## Project Overview\n\n")
	sb.WriteString(fmt.Sprintf("- Name: %s\n", ctx.Project.Name))
	sb.WriteString(fmt.Sprintf("- Path: %s\n", ctx.Project.Path))
	sb.WriteString(fmt.Sprintf("- Primary language: %s\n", ctx.Project.PrimaryLanguage))
	sb.WriteString(fmt.Sprintf("- Has CLAUDE.md: %v\n", ctx.Project.HasClaudeMD))
	sb.WriteString(fmt.Sprintf("- Readiness score: %d/100\n", int(ctx.Project.Score)))
	sb.WriteString("\n")

	// Project structure.
	structure := scanProjectStructure(ctx.Project.Path)
	if structure != "" {
		sb.WriteString("## Project Structure\n\n")
		sb.WriteString(structure)
		sb.WriteString("\n\n")
	}

	// Existing CLAUDE.md content.
	if ctx.ExistingClaudeMD != "" {
		sb.WriteString("## Existing CLAUDE.md Content (first 50 lines)\n\n")
		lines := strings.Split(ctx.ExistingClaudeMD, "\n")
		limit := 50
		if len(lines) < limit {
			limit = len(lines)
		}
		sb.WriteString(strings.Join(lines[:limit], "\n"))
		sb.WriteString("\n\n")
	}

	// Session statistics.
	sb.WriteString("## Session Statistics\n\n")
	sessionCount := len(ctx.Sessions)
	sb.WriteString(fmt.Sprintf("- Total sessions: %d\n", sessionCount))

	if sessionCount > 0 {
		var totalDuration, totalUserMsgs, totalAssistantMsgs int
		var totalToolErrors int
		toolTotals := make(map[string]int)
		langTotals := make(map[string]int)

		for _, s := range ctx.Sessions {
			totalDuration += s.DurationMinutes
			totalUserMsgs += s.UserMessageCount
			totalAssistantMsgs += s.AssistantMessageCount
			totalToolErrors += s.ToolErrors
			for tool, count := range s.ToolCounts {
				toolTotals[tool] += count
			}
			for lang, count := range s.Languages {
				langTotals[lang] += count
			}
		}

		avgDuration := totalDuration / sessionCount
		sb.WriteString(fmt.Sprintf("- Average session duration: %d minutes\n", avgDuration))
		sb.WriteString(fmt.Sprintf("- Total user messages: %d\n", totalUserMsgs))
		sb.WriteString(fmt.Sprintf("- Total assistant messages: %d\n", totalAssistantMsgs))
		sb.WriteString(fmt.Sprintf("- Total tool errors: %d\n", totalToolErrors))

		// Tool usage breakdown.
		if len(toolTotals) > 0 {
			sb.WriteString("\n### Tool Usage\n\n")
			for tool, count := range toolTotals {
				sb.WriteString(fmt.Sprintf("- %s: %d calls\n", tool, count))
			}
		}

		// Languages detected.
		if len(langTotals) > 0 {
			sb.WriteString("\n### Languages Detected\n\n")
			for lang, count := range langTotals {
				sb.WriteString(fmt.Sprintf("- %s: %d files\n", lang, count))
			}
		}
	}
	sb.WriteString("\n")

	// Commit analysis.
	if ctx.CommitAnalysis != nil {
		sb.WriteString("## Commit Analysis\n\n")
		sb.WriteString(fmt.Sprintf("- Total sessions: %d\n", ctx.CommitAnalysis.TotalSessions))
		sb.WriteString(fmt.Sprintf("- Sessions with commits: %d\n", ctx.CommitAnalysis.SessionsWithCommits))
		sb.WriteString(fmt.Sprintf("- Zero-commit rate: %.0f%%\n", ctx.CommitAnalysis.ZeroCommitRate*100))
		sb.WriteString(fmt.Sprintf("- Average commits per session: %.1f\n", ctx.CommitAnalysis.AvgCommitsPerSession))
		sb.WriteString("\n")
	}

	// Friction patterns.
	if ctx.FrictionPatterns != nil && len(ctx.FrictionPatterns.Patterns) > 0 {
		sb.WriteString("## Friction Patterns\n\n")
		sb.WriteString(fmt.Sprintf("- Stale patterns (3+ weeks): %d\n", ctx.FrictionPatterns.StaleCount))
		sb.WriteString(fmt.Sprintf("- Improving patterns: %d\n", ctx.FrictionPatterns.ImprovingCount))
		sb.WriteString(fmt.Sprintf("- Worsening patterns: %d\n", ctx.FrictionPatterns.WorseningCount))
		sb.WriteString("\n### Pattern Details\n\n")
		for _, p := range ctx.FrictionPatterns.Patterns {
			sb.WriteString(fmt.Sprintf("- %s: frequency=%.2f, trend=%s, consecutive_weeks=%d, stale=%v, occurrences=%d\n",
				p.FrictionType, p.Frequency, p.WeeklyTrend, p.ConsecutiveWeeks, p.Stale, p.OccurrenceCount))
		}
		sb.WriteString("\n")
	}

	// Agent tasks.
	if len(ctx.AgentTasks) > 0 {
		sb.WriteString("## Agent Tasks\n\n")
		agentCounts := make(map[string]int)
		agentKilled := make(map[string]int)
		var totalAgentDurationMs int64
		for _, t := range ctx.AgentTasks {
			agentCounts[t.AgentType]++
			if t.Status == "killed" {
				agentKilled[t.AgentType]++
			}
			totalAgentDurationMs += t.DurationMs
		}
		for agentType, count := range agentCounts {
			killed := agentKilled[agentType]
			killRate := 0.0
			if count > 0 {
				killRate = float64(killed) / float64(count) * 100
			}
			sb.WriteString(fmt.Sprintf("- %s: %d tasks, %d killed (%.0f%% kill rate)\n",
				agentType, count, killed, killRate))
		}
		if len(ctx.AgentTasks) > 0 {
			avgDurationMin := totalAgentDurationMs / int64(len(ctx.AgentTasks)) / 60000
			sb.WriteString(fmt.Sprintf("- Average agent task duration: %d minutes\n", avgDurationMin))
		}
		sb.WriteString("\n")
	}

	// Tool profile.
	if ctx.ToolProfile != nil {
		sb.WriteString("## Tool Profile\n\n")
		sb.WriteString(fmt.Sprintf("- Dominant tool: %s\n", ctx.ToolProfile.DominantTool))
		sb.WriteString(fmt.Sprintf("- Bash ratio: %.2f\n", ctx.ToolProfile.BashRatio))
		sb.WriteString(fmt.Sprintf("- Edit/Read ratio: %.2f\n", ctx.ToolProfile.EditToReadRatio))
		sb.WriteString("\n")
	}

	// Conversation data.
	if ctx.ConversationData != nil {
		sb.WriteString("## Conversation Quality\n\n")
		sb.WriteString(fmt.Sprintf("- Average correction rate: %.0f%%\n", ctx.ConversationData.AvgCorrectionRate*100))
		sb.WriteString(fmt.Sprintf("- Average long message rate: %.0f%%\n", ctx.ConversationData.AvgLongMsgRate*100))
		sb.WriteString(fmt.Sprintf("- High-correction sessions: %d\n", ctx.ConversationData.HighCorrectionSessions))
		sb.WriteString("\n")
	}

	// CLAUDE.md quality analysis.
	if ctx.ClaudeMDQuality != nil {
		sb.WriteString("## CLAUDE.md Quality Analysis\n\n")
		sb.WriteString(fmt.Sprintf("- Quality score: %d\n", ctx.ClaudeMDQuality.QualityScore))
		sb.WriteString(fmt.Sprintf("- Total lines: %d\n", ctx.ClaudeMDQuality.TotalLines))
		sb.WriteString(fmt.Sprintf("- Has code blocks: %v\n", ctx.ClaudeMDQuality.HasCodeBlocks))
		if len(ctx.ClaudeMDQuality.MissingSections) > 0 {
			sb.WriteString(fmt.Sprintf("- Missing sections: %s\n", strings.Join(ctx.ClaudeMDQuality.MissingSections, ", ")))
		}
		if len(ctx.ClaudeMDQuality.Sections) > 0 {
			sb.WriteString("- Existing sections: ")
			var sectionNames []string
			for _, s := range ctx.ClaudeMDQuality.Sections {
				sectionNames = append(sectionNames, s.Name)
			}
			sb.WriteString(strings.Join(sectionNames, ", "))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// scanProjectStructure reads the top-level directory listing and key config files
// to give the AI context about the project layout. Output is truncated to ~2000 chars.
func scanProjectStructure(projectPath string) string {
	var sb strings.Builder

	// List top-level files and directories.
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return ""
	}

	sb.WriteString("### Top-Level Contents\n\n")
	for _, entry := range entries {
		name := entry.Name()
		// Skip hidden files except .claude.
		if strings.HasPrefix(name, ".") && name != ".claude" {
			continue
		}
		if entry.IsDir() {
			sb.WriteString(fmt.Sprintf("- %s/ (directory)\n", name))
		} else {
			sb.WriteString(fmt.Sprintf("- %s\n", name))
		}
	}

	// Read key config files (first 20 lines each).
	keyFiles := []string{"go.mod", "package.json", "pyproject.toml", "Cargo.toml", "Makefile"}
	for _, name := range keyFiles {
		path := filepath.Join(projectPath, name)
		content := readFirstNLines(path, 20)
		if content == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n### %s (first 20 lines)\n\n```\n%s\n```\n", name, content))
	}

	result := sb.String()

	// Truncate to ~2000 chars.
	if len(result) > 2000 {
		result = result[:2000] + "\n... (truncated)"
	}

	return result
}

// readFirstNLines reads the first n lines of a file. Returns empty string if
// the file does not exist or cannot be read.
func readFirstNLines(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// claudeAPIRequest is the request body for the Claude Messages API.
type claudeAPIRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    string              `json:"system"`
	Messages  []claudeAPIMessage  `json:"messages"`
}

// claudeAPIMessage is a single message in the Claude Messages API request.
type claudeAPIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeAPIResponse is the response body from the Claude Messages API.
type claudeAPIResponse struct {
	ID      string                   `json:"id"`
	Type    string                   `json:"type"`
	Content []claudeAPIContentBlock  `json:"content"`
	Error   *claudeAPIError          `json:"error,omitempty"`
}

// claudeAPIContentBlock is a single content block in the API response.
type claudeAPIContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// claudeAPIError represents an error response from the Claude API.
type claudeAPIError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// callClaudeAPI sends a request to the Claude Messages API and returns the
// text content of the response. It uses net/http with no external dependencies.
func callClaudeAPI(apiKey, model, systemPrompt, userPrompt string) (string, error) {
	reqBody := claudeAPIRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages: []claudeAPIMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", claudeAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", claudeAPIVersion)
	req.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: apiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var apiResp claudeAPIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return "", fmt.Errorf("unmarshaling response: %w", err)
	}

	if apiResp.Error != nil {
		return "", fmt.Errorf("API error: %s: %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	// Extract text from content blocks.
	var textParts []string
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			textParts = append(textParts, block.Text)
		}
	}

	if len(textParts) == 0 {
		return "", fmt.Errorf("no text content in API response")
	}

	return strings.Join(textParts, ""), nil
}

// aiResponseSchema is the expected JSON structure from the AI response.
type aiResponseSchema struct {
	Additions []aiAddition `json:"additions"`
}

// aiAddition is a single addition from the AI response.
type aiAddition struct {
	Section string `json:"section"`
	Content string `json:"content"`
	Reason  string `json:"reason"`
}

// parseAIResponse extracts Addition entries from the AI's JSON response.
// It handles cases where the JSON may be wrapped in markdown code fences.
func parseAIResponse(responseText string) ([]Addition, error) {
	// Strip markdown code fences if present.
	text := strings.TrimSpace(responseText)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	var schema aiResponseSchema
	if err := json.Unmarshal([]byte(text), &schema); err != nil {
		return nil, fmt.Errorf("parsing AI JSON response: %w (response was: %.200s)", err, text)
	}

	if len(schema.Additions) == 0 {
		return nil, fmt.Errorf("AI response contained no additions")
	}

	var additions []Addition
	for _, a := range schema.Additions {
		if a.Section == "" || a.Content == "" {
			continue
		}
		additions = append(additions, Addition{
			Section: a.Section,
			Content: a.Content,
			Reason:  a.Reason,
		})
	}

	if len(additions) == 0 {
		return nil, fmt.Errorf("AI response contained no valid additions (all had empty section or content)")
	}

	return additions, nil
}
