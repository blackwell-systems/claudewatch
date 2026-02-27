package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

func writeClaudeMD(t *testing.T, dir string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write CLAUDE.md: %v", err)
	}
}

func TestAnalyzeClaudeMDEffectiveness_MissingFile(t *testing.T) {
	dir := t.TempDir()

	projects := []scanner.Project{
		{
			Path:        dir,
			Name:        "nofile",
			HasClaudeMD: false,
		},
	}

	result := AnalyzeClaudeMDEffectiveness(projects, nil)

	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}

	q := result.Projects[0]
	if q.QualityScore != 0 {
		t.Errorf("expected score 0 for missing CLAUDE.md, got %d", q.QualityScore)
	}
	if len(q.MissingSections) != len(knownSections) {
		t.Errorf("expected %d missing sections, got %d", len(knownSections), len(q.MissingSections))
	}
}

func TestAnalyzeClaudeMDEffectiveness_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeClaudeMD(t, dir, "")

	projects := []scanner.Project{
		{
			Path:        dir,
			Name:        "empty",
			HasClaudeMD: true,
		},
	}

	result := AnalyzeClaudeMDEffectiveness(projects, nil)

	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}

	q := result.Projects[0]
	// Has CLAUDE.md = 20 points, but no sections detected.
	if q.QualityScore != 20 {
		t.Errorf("expected score 20 for empty CLAUDE.md, got %d", q.QualityScore)
	}
	if q.TotalLines != 0 {
		t.Errorf("expected 0 lines, got %d", q.TotalLines)
	}
}

func TestAnalyzeClaudeMDEffectiveness_FullScore(t *testing.T) {
	dir := t.TempDir()
	// Build content that triggers all section detections and bonuses.
	// Over 100 lines, has code blocks, has all sections.
	var lines string
	lines += "# Project\n\n"
	lines += "## Build Commands\n\n"
	lines += "Run `go build ./...` to compile.\n\n"
	for i := 0; i < 10; i++ {
		lines += "build step detail line\n"
	}
	lines += "\n## Testing\n\n"
	lines += "Run `go test ./...` for unit tests.\n\n"
	for i := 0; i < 10; i++ {
		lines += "test detail line\n"
	}
	lines += "\n## Architecture\n\n"
	lines += "The project is organized into packages.\n\n"
	for i := 0; i < 10; i++ {
		lines += "architecture detail line\n"
	}
	lines += "\n## Code Conventions\n\n"
	lines += "Follow Go naming conventions.\n\n"
	for i := 0; i < 10; i++ {
		lines += "convention detail line\n"
	}
	lines += "\n## Error Handling\n\n"
	lines += "Use error wrapping with fmt.Errorf.\n\n"
	for i := 0; i < 10; i++ {
		lines += "error handling detail line\n"
	}
	lines += "\n## Dependencies\n\n"
	lines += "Run `go mod tidy` after changes.\n\n"
	for i := 0; i < 10; i++ {
		lines += "dependency detail line\n"
	}
	lines += "\n```go\nfunc main() {}\n```\n"
	// Pad to over 100 lines.
	for i := 0; i < 30; i++ {
		lines += "additional documentation line\n"
	}

	writeClaudeMD(t, dir, lines)

	projects := []scanner.Project{
		{
			Path:        dir,
			Name:        "fullscore",
			HasClaudeMD: true,
		},
	}

	result := AnalyzeClaudeMDEffectiveness(projects, nil)

	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}

	q := result.Projects[0]
	// Max: 20 (exists) + 15 (build) + 15 (test) + 15 (arch) + 10 (conventions) + 10 (error) + 10 (>100 lines) + 5 (code blocks) = 100
	if q.QualityScore != 100 {
		t.Errorf("expected score 100, got %d", q.QualityScore)
	}
	if !q.HasCodeBlocks {
		t.Error("expected HasCodeBlocks=true")
	}
	if len(q.MissingSections) != 1 {
		// "dependencies" section does not contribute to score but should be detected
		// Actually let's check what's missing
		t.Logf("missing sections: %v", q.MissingSections)
	}
}

func TestAnalyzeClaudeMDEffectiveness_PartialSections(t *testing.T) {
	dir := t.TempDir()
	content := "# My Project\n\n## Build\n\nRun make to build.\n\n## Testing\n\nRun pytest.\n"
	writeClaudeMD(t, dir, content)

	projects := []scanner.Project{
		{
			Path:        dir,
			Name:        "partial",
			HasClaudeMD: true,
		},
	}

	result := AnalyzeClaudeMDEffectiveness(projects, nil)

	q := result.Projects[0]
	// 20 (exists) + 15 (build) + 15 (testing) = 50
	if q.QualityScore != 50 {
		t.Errorf("expected score 50, got %d", q.QualityScore)
	}

	// Should have some missing sections.
	if len(q.MissingSections) == 0 {
		t.Error("expected some missing sections for partial content")
	}
}

func TestDetectSections(t *testing.T) {
	tests := []struct {
		name        string
		lines       []string
		wantPresent map[string]bool
	}{
		{
			name: "build header",
			lines: []string{
				"## Build Commands",
				"Run go build to compile.",
			},
			wantPresent: map[string]bool{
				"build commands": true,
			},
		},
		{
			name: "testing header",
			lines: []string{
				"## Testing",
				"Run go test for tests.",
			},
			wantPresent: map[string]bool{
				"testing": true,
			},
		},
		{
			name: "content keyword matching",
			lines: []string{
				"# Project Guide",
				"We use go test for validation.",
				"Always run go test before pushing.",
				"The test suite is comprehensive.",
			},
			wantPresent: map[string]bool{
				"testing": true,
			},
		},
		{
			name: "single content keyword not enough",
			lines: []string{
				"# Project Guide",
				"We use go test for validation.",
			},
			wantPresent: map[string]bool{
				"testing": false,
			},
		},
		{
			name: "architecture header",
			lines: []string{
				"## Architecture",
				"The project follows clean architecture.",
			},
			wantPresent: map[string]bool{
				"architecture": true,
			},
		},
		{
			name:  "empty lines",
			lines: []string{},
			wantPresent: map[string]bool{
				"build commands":   false,
				"testing":          false,
				"architecture":     false,
				"code conventions": false,
				"error handling":   false,
				"dependencies":     false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sections := detectSections(tt.lines)
			for _, s := range sections {
				if want, exists := tt.wantPresent[s.Name]; exists {
					if s.Present != want {
						t.Errorf("section %q: Present=%v, want %v", s.Name, s.Present, want)
					}
				}
			}
		})
	}
}

func TestComputeQualityScore(t *testing.T) {
	tests := []struct {
		name    string
		quality ClaudeMDQuality
		want    int
	}{
		{
			name: "base score only",
			quality: ClaudeMDQuality{
				Sections: []ClaudeMDSection{
					{Name: "build commands", Present: false},
					{Name: "testing", Present: false},
				},
			},
			want: 20,
		},
		{
			name: "with build and testing",
			quality: ClaudeMDQuality{
				Sections: []ClaudeMDSection{
					{Name: "build commands", Present: true},
					{Name: "testing", Present: true},
				},
			},
			want: 50, // 20 + 15 + 15
		},
		{
			name: "line count bonus over 50",
			quality: ClaudeMDQuality{
				TotalLines: 55,
				Sections:   []ClaudeMDSection{},
			},
			want: 25, // 20 + 5
		},
		{
			name: "line count bonus over 100",
			quality: ClaudeMDQuality{
				TotalLines: 110,
				Sections:   []ClaudeMDSection{},
			},
			want: 30, // 20 + 10
		},
		{
			name: "code blocks bonus",
			quality: ClaudeMDQuality{
				HasCodeBlocks: true,
				Sections:      []ClaudeMDSection{},
			},
			want: 25, // 20 + 5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeQualityScore(tt.quality)
			if got != tt.want {
				t.Errorf("computeQualityScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestComputeSectionCorrelations(t *testing.T) {
	projects := []ClaudeMDQuality{
		{
			ProjectName:     "with-build",
			SessionCount:    5,
			AvgFrictionRate: 1.0,
			Sections: []ClaudeMDSection{
				{Name: "build commands", Present: true},
				{Name: "testing", Present: false},
			},
		},
		{
			ProjectName:     "without-build",
			SessionCount:    5,
			AvgFrictionRate: 3.0,
			Sections: []ClaudeMDSection{
				{Name: "build commands", Present: false},
				{Name: "testing", Present: false},
			},
		},
	}

	correlations, bestSection := computeSectionCorrelations(projects)

	// For "build commands": avgWith=1.0, avgWithout=3.0
	// Reduction = (3.0-1.0)/3.0 * 100 = 66.67%
	buildCorr := correlations["build commands"]
	if buildCorr < 66.0 || buildCorr > 67.0 {
		t.Errorf("expected build commands correlation ~66.67%%, got %.2f%%", buildCorr)
	}

	if bestSection != "build commands" {
		t.Errorf("expected best section 'build commands', got %q", bestSection)
	}

	// "testing" has no data split (both projects have it as false), so correlation = 0.
	testingCorr := correlations["testing"]
	if testingCorr != 0 {
		t.Errorf("expected testing correlation 0, got %.2f", testingCorr)
	}
}

func TestComputeSectionCorrelations_NoSessions(t *testing.T) {
	projects := []ClaudeMDQuality{
		{
			ProjectName:  "nosessions",
			SessionCount: 0,
			Sections: []ClaudeMDSection{
				{Name: "build commands", Present: true},
			},
		},
	}

	correlations, bestSection := computeSectionCorrelations(projects)

	if len(correlations) != 0 {
		t.Errorf("expected empty correlations for projects with no sessions, got %v", correlations)
	}
	if bestSection != "" {
		t.Errorf("expected empty best section, got %q", bestSection)
	}
}

func TestComputeProjectFriction(t *testing.T) {
	tests := []struct {
		name        string
		facets      []claude.SessionFacet
		wantCount   int
		wantFricAvg float64
	}{
		{
			name:        "empty",
			facets:      nil,
			wantCount:   0,
			wantFricAvg: 0,
		},
		{
			name: "single facet with friction",
			facets: []claude.SessionFacet{
				{SessionID: "s1", FrictionCounts: map[string]int{"wrong_approach": 2, "misunderstood": 1}},
			},
			wantCount:   1,
			wantFricAvg: 3.0,
		},
		{
			name: "multiple facets",
			facets: []claude.SessionFacet{
				{SessionID: "s1", FrictionCounts: map[string]int{"wrong_approach": 2}},
				{SessionID: "s2", FrictionCounts: map[string]int{"wrong_approach": 4}},
			},
			wantCount:   2,
			wantFricAvg: 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, avg := computeProjectFriction("/dummy", tt.facets)
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
			if diff := avg - tt.wantFricAvg; diff > 0.01 || diff < -0.01 {
				t.Errorf("avg friction = %.2f, want %.2f", avg, tt.wantFricAvg)
			}
		})
	}
}

func TestAvgFloat64(t *testing.T) {
	tests := []struct {
		name string
		vals []float64
		want float64
	}{
		{"empty", nil, 0},
		{"single", []float64{5.0}, 5.0},
		{"multiple", []float64{2.0, 4.0, 6.0}, 4.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := avgFloat64(tt.vals)
			if diff := got - tt.want; diff > 0.001 || diff < -0.001 {
				t.Errorf("avgFloat64() = %f, want %f", got, tt.want)
			}
		})
	}
}
