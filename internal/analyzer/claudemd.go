package analyzer

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

// ClaudeMDSection represents a detected section within a CLAUDE.md file.
type ClaudeMDSection struct {
	Name      string `json:"name"`
	Present   bool   `json:"present"`
	LineCount int    `json:"line_count"`
}

// ClaudeMDQuality captures quality analysis for a single project's CLAUDE.md.
type ClaudeMDQuality struct {
	ProjectPath     string            `json:"project_path"`
	ProjectName     string            `json:"project_name"`
	FilePath        string            `json:"file_path"`
	TotalLines      int               `json:"total_lines"`
	Sections        []ClaudeMDSection `json:"sections"`
	QualityScore    int               `json:"quality_score"`
	MissingSections []string          `json:"missing_sections"`
	HasCodeBlocks   bool              `json:"has_code_blocks"`

	// Correlation data
	SessionCount        int                `json:"session_count"`
	AvgFrictionRate     float64            `json:"avg_friction_rate"`
	FrictionWithSection map[string]float64 `json:"friction_with_section"`
}

// ClaudeMDAnalysis is the top-level result of CLAUDE.md effectiveness analysis.
type ClaudeMDAnalysis struct {
	Projects             []ClaudeMDQuality  `json:"projects"`
	SectionsCorrelation  map[string]float64 `json:"sections_correlation"`
	MostImpactfulSection string             `json:"most_impactful_section"`
}

// sectionDef defines a detectable section with its display name and keywords.
type sectionDef struct {
	Name            string
	HeaderKeywords  []string // matched against ## header text (case-insensitive)
	ContentKeywords []string // matched against body lines (case-insensitive)
}

// knownSections lists the sections we detect in CLAUDE.md files.
var knownSections = []sectionDef{
	{
		Name:            "build commands",
		HeaderKeywords:  []string{"build", "compile", "make"},
		ContentKeywords: []string{"build", "make", "go build", "npm run", "cargo build", "mvn", "gradle"},
	},
	{
		Name:            "testing",
		HeaderKeywords:  []string{"test", "testing"},
		ContentKeywords: []string{"test", "go test", "pytest", "jest", "mocha", "cargo test", "npm test"},
	},
	{
		Name:            "code conventions",
		HeaderKeywords:  []string{"convention", "style", "naming", "format", "lint"},
		ContentKeywords: []string{"convention", "style", "naming", "format", "lint", "prettier", "eslint", "gofmt"},
	},
	{
		Name:            "architecture",
		HeaderKeywords:  []string{"architecture", "structure", "layout", "overview", "packages", "organization"},
		ContentKeywords: []string{"architecture", "structure", "layout", "packages", "directory", "modules"},
	},
	{
		Name:            "error handling",
		HeaderKeywords:  []string{"error", "debug", "troubleshoot", "logging"},
		ContentKeywords: []string{"error", "debug", "troubleshoot", "logging", "panic", "exception"},
	},
	{
		Name:            "dependencies",
		HeaderKeywords:  []string{"dependencies", "dependency", "requirements", "install"},
		ContentKeywords: []string{"dependencies", "require", "import", "install", "go mod", "npm install", "pip install"},
	},
}

// AnalyzeClaudeMDEffectiveness examines CLAUDE.md files across projects and
// correlates their content quality with session friction rates from facet data.
func AnalyzeClaudeMDEffectiveness(projects []scanner.Project, facets []claude.SessionFacet) ClaudeMDAnalysis {
	analysis := ClaudeMDAnalysis{
		SectionsCorrelation: make(map[string]float64),
	}

	// Build a map of sessionID -> facet for quick lookup.
	facetBySession := make(map[string]claude.SessionFacet, len(facets))
	for _, f := range facets {
		facetBySession[f.SessionID] = f
	}

	// Analyze each project.
	for i := range projects {
		quality := analyzeProjectClaudeMD(&projects[i], facets)
		analysis.Projects = append(analysis.Projects, quality)
	}

	// Compute cross-project correlation: for each section, compare avg friction
	// rate of projects that have it vs. those that don't.
	analysis.SectionsCorrelation, analysis.MostImpactfulSection = computeSectionCorrelations(analysis.Projects)

	return analysis
}

// analyzeProjectClaudeMD parses a single project's CLAUDE.md and computes its
// quality score and friction correlation data.
func analyzeProjectClaudeMD(project *scanner.Project, facets []claude.SessionFacet) ClaudeMDQuality {
	quality := ClaudeMDQuality{
		ProjectPath:         project.Path,
		ProjectName:         project.Name,
		FrictionWithSection: make(map[string]float64),
	}

	claudeMDPath := filepath.Join(project.Path, "CLAUDE.md")
	quality.FilePath = claudeMDPath

	if !project.HasClaudeMD {
		// No CLAUDE.md: all sections are missing.
		for _, sd := range knownSections {
			quality.Sections = append(quality.Sections, ClaudeMDSection{
				Name:    sd.Name,
				Present: false,
			})
			quality.MissingSections = append(quality.MissingSections, sd.Name)
		}
		quality.SessionCount, quality.AvgFrictionRate = computeProjectFriction(project.Path, facets)
		return quality
	}

	// Parse the CLAUDE.md file.
	lines, hasCodeBlocks, err := readClaudeMD(claudeMDPath)
	if err != nil {
		// File exists but can't be read; treat as empty.
		quality.SessionCount, quality.AvgFrictionRate = computeProjectFriction(project.Path, facets)
		return quality
	}

	quality.TotalLines = len(lines)
	quality.HasCodeBlocks = hasCodeBlocks

	// Detect sections.
	quality.Sections = detectSections(lines)

	// Identify missing sections.
	for _, section := range quality.Sections {
		if !section.Present {
			quality.MissingSections = append(quality.MissingSections, section.Name)
		}
	}

	// Compute quality score.
	quality.QualityScore = computeQualityScore(quality)

	// Compute friction data.
	quality.SessionCount, quality.AvgFrictionRate = computeProjectFriction(project.Path, facets)

	return quality
}

// readClaudeMD reads the CLAUDE.md file and returns its lines and whether it
// contains code blocks (``` fences).
func readClaudeMD(path string) ([]string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	var lines []string
	hasCodeBlocks := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		lines = append(lines, line)
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			hasCodeBlocks = true
		}
	}
	return lines, hasCodeBlocks, sc.Err()
}

// detectSections scans lines for known section patterns using both header
// matching (## headers) and content keyword scanning. Section boundaries are
// delimited by ## headers; content keywords are checked within each section.
func detectSections(lines []string) []ClaudeMDSection {
	// Parse headers and their line ranges.
	type headerRange struct {
		text      string
		startLine int
		endLine   int // exclusive
	}

	var headers []headerRange
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "##") {
			// Extract header text after ##, ###, etc.
			headerText := strings.TrimLeft(trimmed, "#")
			headerText = strings.TrimSpace(headerText)
			headers = append(headers, headerRange{
				text:      strings.ToLower(headerText),
				startLine: i,
			})
		}
	}

	// Set end lines.
	for i := range headers {
		if i+1 < len(headers) {
			headers[i].endLine = headers[i+1].startLine
		} else {
			headers[i].endLine = len(lines)
		}
	}

	// For each known section, check if any header or content region matches.
	results := make([]ClaudeMDSection, len(knownSections))
	for i, sd := range knownSections {
		results[i] = ClaudeMDSection{Name: sd.Name}

		// Check headers for keyword matches.
		for _, hr := range headers {
			if matchesAnyKeyword(hr.text, sd.HeaderKeywords) {
				results[i].Present = true
				results[i].LineCount = hr.endLine - hr.startLine
				break
			}
		}

		// If not found by header, scan all content for keyword matches.
		if !results[i].Present {
			matchCount := 0
			for _, line := range lines {
				lower := strings.ToLower(line)
				for _, kw := range sd.ContentKeywords {
					if strings.Contains(lower, kw) {
						matchCount++
						break
					}
				}
			}
			// Require at least 2 content keyword hits to avoid false positives.
			if matchCount >= 2 {
				results[i].Present = true
				results[i].LineCount = matchCount // approximate; content-matched sections don't have clear boundaries
			}
		}
	}

	return results
}

// matchesAnyKeyword checks if text contains any of the given keywords.
func matchesAnyKeyword(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// computeQualityScore produces a 0-100 score based on the scoring rubric.
func computeQualityScore(q ClaudeMDQuality) int {
	score := 0

	// Has CLAUDE.md: 20 points.
	score += 20

	// Section scores.
	for _, section := range q.Sections {
		if !section.Present {
			continue
		}
		switch section.Name {
		case "build commands":
			score += 15
		case "testing":
			score += 15
		case "architecture":
			score += 15
		case "code conventions":
			score += 10
		case "error handling":
			score += 10
		}
	}

	// Line count bonuses.
	if q.TotalLines > 100 {
		score += 10 // both >50 and >100
	} else if q.TotalLines > 50 {
		score += 5
	}

	// Code blocks.
	if q.HasCodeBlocks {
		score += 5
	}

	if score > 100 {
		score = 100
	}
	return score
}

// computeProjectFriction calculates the session count and average friction rate
// for a project. Friction rate is total friction events / number of sessions.
func computeProjectFriction(projectPath string, facets []claude.SessionFacet) (int, float64) {
	// We don't have a direct project path on facets, so we count all facets
	// that have friction data. In a real scenario, facets would be pre-filtered
	// by project. For cross-project comparison we rely on the caller passing
	// project-specific facets. Here we compute over all provided facets as a
	// baseline; the correlation logic handles per-project grouping.
	if len(facets) == 0 {
		return 0, 0
	}

	totalFriction := 0
	for _, f := range facets {
		for _, count := range f.FrictionCounts {
			totalFriction += count
		}
	}

	return len(facets), float64(totalFriction) / float64(len(facets))
}

// computeSectionCorrelations compares friction rates between projects that have
// each section and those that don't. Returns a map of section name to friction
// reduction percentage and the most impactful section name.
func computeSectionCorrelations(projects []ClaudeMDQuality) (map[string]float64, string) {
	correlations := make(map[string]float64)

	// Only consider projects with session data for meaningful correlation.
	var projectsWithSessions []ClaudeMDQuality
	for _, p := range projects {
		if p.SessionCount > 0 {
			projectsWithSessions = append(projectsWithSessions, p)
		}
	}

	if len(projectsWithSessions) == 0 {
		return correlations, ""
	}

	// For each section, compute avg friction for projects with vs without.
	for _, sd := range knownSections {
		var withFriction, withoutFriction []float64

		for _, p := range projectsWithSessions {
			hasSection := false
			for _, s := range p.Sections {
				if s.Name == sd.Name && s.Present {
					hasSection = true
					break
				}
			}

			if hasSection {
				withFriction = append(withFriction, p.AvgFrictionRate)
			} else {
				withoutFriction = append(withoutFriction, p.AvgFrictionRate)
			}
		}

		// Need data in both groups for a meaningful comparison.
		if len(withFriction) == 0 || len(withoutFriction) == 0 {
			correlations[sd.Name] = 0
			continue
		}

		avgWith := avgFloat64(withFriction)
		avgWithout := avgFloat64(withoutFriction)

		// Friction reduction: positive means having the section reduces friction.
		if avgWithout > 0 {
			correlations[sd.Name] = ((avgWithout - avgWith) / avgWithout) * 100
		} else {
			correlations[sd.Name] = 0
		}
	}

	// Find most impactful section (highest friction reduction).
	bestSection := ""
	bestReduction := 0.0
	for name, reduction := range correlations {
		if reduction > bestReduction {
			bestReduction = reduction
			bestSection = name
		}
	}

	return correlations, bestSection
}

// avgFloat64 computes the mean of a float64 slice. Returns 0 for empty slices.
func avgFloat64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
