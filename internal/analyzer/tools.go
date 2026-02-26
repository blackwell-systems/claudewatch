package analyzer

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

// AnalyzeToolUsage computes per-project tool usage patterns by matching
// sessions to discovered projects and aggregating tool counts, ratios, and
// anomalies for each project.
func AnalyzeToolUsage(sessions []claude.SessionMeta, projects []scanner.Project) ToolAnalysis {
	// Build a lookup from project path to project metadata.
	projectByPath := make(map[string]scanner.Project, len(projects))
	for _, p := range projects {
		projectByPath[p.Path] = p
	}

	// Group sessions by project path.
	sessionsByProject := make(map[string][]claude.SessionMeta)
	for _, s := range sessions {
		path := claude.NormalizePath(s.ProjectPath)
		if path == "" {
			continue
		}
		sessionsByProject[path] = append(sessionsByProject[path], s)
	}

	var profiles []ToolProfile
	for path, sess := range sessionsByProject {
		profile := buildToolProfile(path, sess, projectByPath)
		profiles = append(profiles, profile)
	}

	// Sort profiles by total sessions descending for stable output.
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].TotalSessions != profiles[j].TotalSessions {
			return profiles[i].TotalSessions > profiles[j].TotalSessions
		}
		return profiles[i].ProjectName < profiles[j].ProjectName
	})

	anomalies := detectAnomalies(profiles, projectByPath)

	return ToolAnalysis{
		Projects:  profiles,
		Anomalies: anomalies,
	}
}

// buildToolProfile constructs a ToolProfile for a single project from its sessions.
func buildToolProfile(projectPath string, sessions []claude.SessionMeta, projectByPath map[string]scanner.Project) ToolProfile {
	name := filepath.Base(projectPath)
	if p, ok := projectByPath[projectPath]; ok {
		name = p.Name
	}

	profile := ToolProfile{
		ProjectPath:    projectPath,
		ProjectName:    name,
		TotalSessions:  len(sessions),
		ToolCounts:     make(map[string]int),
		ToolPerSession: make(map[string]float64),
	}

	// Aggregate tool counts across all sessions.
	for _, s := range sessions {
		for tool, count := range s.ToolCounts {
			profile.ToolCounts[tool] += count
		}
	}

	// Compute per-session averages and find dominant tool.
	n := float64(len(sessions))
	maxCount := 0
	for tool, count := range profile.ToolCounts {
		profile.ToolPerSession[tool] = float64(count) / n
		if count > maxCount {
			maxCount = count
			profile.DominantTool = tool
		}
	}

	// Compute Edit/Read ratio.
	readCount := profile.ToolCounts["Read"]
	editCount := profile.ToolCounts["Edit"]
	if readCount > 0 {
		profile.EditToReadRatio = float64(editCount) / float64(readCount)
	}

	// Compute Bash ratio (Bash calls / total tool calls).
	totalTools := 0
	for _, count := range profile.ToolCounts {
		totalTools += count
	}
	bashCount := profile.ToolCounts["Bash"]
	if totalTools > 0 {
		profile.BashRatio = float64(bashCount) / float64(totalTools)
	}

	return profile
}

// detectAnomalies identifies projects where tool usage deviates significantly
// from expected patterns based on project language and type.
func detectAnomalies(profiles []ToolProfile, projectByPath map[string]scanner.Project) []ToolAnomaly {
	var anomalies []ToolAnomaly

	for _, profile := range profiles {
		// Skip projects with very few sessions (not enough data).
		if profile.TotalSessions < 3 {
			continue
		}

		proj, known := projectByPath[profile.ProjectPath]
		lang := ""
		if known {
			lang = strings.ToLower(proj.PrimaryLanguage)
		}

		// Anomaly: Go project with zero Bash calls (no builds/tests).
		if lang == "go" && profile.ToolCounts["Bash"] == 0 {
			anomalies = append(anomalies, ToolAnomaly{
				ProjectName: profile.ProjectName,
				Tool:        "Bash",
				Expected:    0.15,
				Actual:      0,
				Message:     fmt.Sprintf("%s is a Go project with zero Bash calls across %d sessions — no builds or tests being run", profile.ProjectName, profile.TotalSessions),
			})
		}

		// Anomaly: Go project with unusually low Bash ratio (< 5%).
		if lang == "go" && profile.BashRatio > 0 && profile.BashRatio < 0.05 {
			anomalies = append(anomalies, ToolAnomaly{
				ProjectName: profile.ProjectName,
				Tool:        "Bash",
				Expected:    0.15,
				Actual:      profile.BashRatio,
				Message:     fmt.Sprintf("%s is a Go project with only %.0f%% Bash usage — builds and tests may be underutilized", profile.ProjectName, profile.BashRatio*100),
			})
		}

		// Anomaly: very low Edit/Read ratio suggests mostly exploring, not producing.
		totalTools := 0
		for _, c := range profile.ToolCounts {
			totalTools += c
		}
		if totalTools > 20 && profile.ToolCounts["Read"] > 0 && profile.EditToReadRatio < 0.1 {
			anomalies = append(anomalies, ToolAnomaly{
				ProjectName: profile.ProjectName,
				Tool:        "Edit",
				Expected:    0.3,
				Actual:      profile.EditToReadRatio,
				Message:     fmt.Sprintf("%s has a very low Edit/Read ratio (%.2f) — sessions are mostly exploration with little editing", profile.ProjectName, profile.EditToReadRatio),
			})
		}

		// Anomaly: documentation-like project (no code language detected) with high Bash usage.
		if lang == "" && profile.BashRatio > 0.4 {
			anomalies = append(anomalies, ToolAnomaly{
				ProjectName: profile.ProjectName,
				Tool:        "Bash",
				Expected:    0.1,
				Actual:      profile.BashRatio,
				Message:     fmt.Sprintf("%s has no detected language but %.0f%% Bash usage — unusual for a documentation or config project", profile.ProjectName, profile.BashRatio*100),
			})
		}

		// Anomaly: Rust/Go project with zero Read calls (not reading code before editing).
		if (lang == "go" || lang == "rust") && profile.ToolCounts["Read"] == 0 && profile.ToolCounts["Edit"] > 0 {
			anomalies = append(anomalies, ToolAnomaly{
				ProjectName: profile.ProjectName,
				Tool:        "Read",
				Expected:    1.0,
				Actual:      0,
				Message:     fmt.Sprintf("%s has Edit calls but zero Read calls — editing code without reading it first", profile.ProjectName),
			})
		}
	}

	// Sort anomalies by project name for stable output.
	sort.Slice(anomalies, func(i, j int) bool {
		if anomalies[i].ProjectName != anomalies[j].ProjectName {
			return anomalies[i].ProjectName < anomalies[j].ProjectName
		}
		return anomalies[i].Tool < anomalies[j].Tool
	})

	return anomalies
}

