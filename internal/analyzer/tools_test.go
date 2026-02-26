package analyzer

import (
	"testing"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

func TestAnalyzeToolUsage_Empty(t *testing.T) {
	result := AnalyzeToolUsage(nil, nil)
	if len(result.Projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(result.Projects))
	}
	if len(result.Anomalies) != 0 {
		t.Errorf("expected 0 anomalies, got %d", len(result.Anomalies))
	}
}

func TestAnalyzeToolUsage_SingleProject(t *testing.T) {
	sessions := []claude.SessionMeta{
		{
			SessionID:   "s1",
			ProjectPath: "/home/user/projects/myapp",
			ToolCounts:  map[string]int{"Read": 10, "Edit": 5, "Bash": 3},
		},
		{
			SessionID:   "s2",
			ProjectPath: "/home/user/projects/myapp",
			ToolCounts:  map[string]int{"Read": 6, "Edit": 4, "Bash": 2},
		},
	}
	projects := []scanner.Project{
		{
			Path:            "/home/user/projects/myapp",
			Name:            "myapp",
			PrimaryLanguage: "Go",
		},
	}

	result := AnalyzeToolUsage(sessions, projects)

	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}

	profile := result.Projects[0]
	if profile.ProjectName != "myapp" {
		t.Errorf("expected project name 'myapp', got %q", profile.ProjectName)
	}
	if profile.TotalSessions != 2 {
		t.Errorf("expected 2 sessions, got %d", profile.TotalSessions)
	}
	if profile.ToolCounts["Read"] != 16 {
		t.Errorf("expected Read=16, got %d", profile.ToolCounts["Read"])
	}
	if profile.ToolCounts["Edit"] != 9 {
		t.Errorf("expected Edit=9, got %d", profile.ToolCounts["Edit"])
	}

	// Edit/Read ratio = 9/16 = 0.5625
	wantRatio := 9.0 / 16.0
	if diff := profile.EditToReadRatio - wantRatio; diff > 0.001 || diff < -0.001 {
		t.Errorf("expected EditToReadRatio ~%.4f, got %.4f", wantRatio, profile.EditToReadRatio)
	}

	// Bash ratio = 5/30
	wantBash := 5.0 / 30.0
	if diff := profile.BashRatio - wantBash; diff > 0.001 || diff < -0.001 {
		t.Errorf("expected BashRatio ~%.4f, got %.4f", wantBash, profile.BashRatio)
	}
}

func TestAnalyzeToolUsage_MultipleProjects(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/projects/alpha", ToolCounts: map[string]int{"Read": 5}},
		{SessionID: "s2", ProjectPath: "/projects/alpha", ToolCounts: map[string]int{"Read": 3}},
		{SessionID: "s3", ProjectPath: "/projects/beta", ToolCounts: map[string]int{"Bash": 10}},
	}
	projects := []scanner.Project{
		{Path: "/projects/alpha", Name: "alpha"},
		{Path: "/projects/beta", Name: "beta"},
	}

	result := AnalyzeToolUsage(sessions, projects)

	if len(result.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(result.Projects))
	}

	// Projects sorted by total sessions descending; alpha has 2, beta has 1.
	if result.Projects[0].ProjectName != "alpha" {
		t.Errorf("expected first project 'alpha', got %q", result.Projects[0].ProjectName)
	}
	if result.Projects[1].ProjectName != "beta" {
		t.Errorf("expected second project 'beta', got %q", result.Projects[1].ProjectName)
	}
}

func TestAnalyzeToolUsage_EmptyProjectPath(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "", ToolCounts: map[string]int{"Read": 5}},
	}
	result := AnalyzeToolUsage(sessions, nil)
	if len(result.Projects) != 0 {
		t.Errorf("expected 0 projects for empty project path, got %d", len(result.Projects))
	}
}

func TestAnalyzeToolUsage_AnomalyGoZeroBash(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/projects/goapp", ToolCounts: map[string]int{"Read": 10, "Edit": 5}},
		{SessionID: "s2", ProjectPath: "/projects/goapp", ToolCounts: map[string]int{"Read": 8, "Edit": 4}},
		{SessionID: "s3", ProjectPath: "/projects/goapp", ToolCounts: map[string]int{"Read": 7, "Edit": 3}},
	}
	projects := []scanner.Project{
		{Path: "/projects/goapp", Name: "goapp", PrimaryLanguage: "Go"},
	}

	result := AnalyzeToolUsage(sessions, projects)

	foundBashAnomaly := false
	for _, a := range result.Anomalies {
		if a.ProjectName == "goapp" && a.Tool == "Bash" {
			foundBashAnomaly = true
			if a.Actual != 0 {
				t.Errorf("expected Actual=0 for zero Bash anomaly, got %f", a.Actual)
			}
		}
	}
	if !foundBashAnomaly {
		t.Error("expected Bash anomaly for Go project with zero Bash calls")
	}
}

func TestAnalyzeToolUsage_AnomalyLowEditReadRatio(t *testing.T) {
	// Total tools > 20, Read > 0, Edit/Read < 0.1
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/projects/explorer", ToolCounts: map[string]int{"Read": 15, "Edit": 0, "Bash": 10}},
		{SessionID: "s2", ProjectPath: "/projects/explorer", ToolCounts: map[string]int{"Read": 10, "Edit": 1, "Bash": 5}},
		{SessionID: "s3", ProjectPath: "/projects/explorer", ToolCounts: map[string]int{"Read": 8, "Edit": 0, "Bash": 3}},
	}
	projects := []scanner.Project{
		{Path: "/projects/explorer", Name: "explorer"},
	}

	result := AnalyzeToolUsage(sessions, projects)

	foundEditAnomaly := false
	for _, a := range result.Anomalies {
		if a.ProjectName == "explorer" && a.Tool == "Edit" {
			foundEditAnomaly = true
		}
	}
	if !foundEditAnomaly {
		t.Error("expected Edit anomaly for very low Edit/Read ratio")
	}
}

func TestAnalyzeToolUsage_NoAnomalyForFewSessions(t *testing.T) {
	// Fewer than 3 sessions should not trigger anomalies.
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/projects/goapp", ToolCounts: map[string]int{"Read": 10}},
		{SessionID: "s2", ProjectPath: "/projects/goapp", ToolCounts: map[string]int{"Read": 8}},
	}
	projects := []scanner.Project{
		{Path: "/projects/goapp", Name: "goapp", PrimaryLanguage: "Go"},
	}

	result := AnalyzeToolUsage(sessions, projects)

	if len(result.Anomalies) != 0 {
		t.Errorf("expected 0 anomalies for < 3 sessions, got %d", len(result.Anomalies))
	}
}

func TestAnalyzeToolUsage_DominantTool(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/projects/app", ToolCounts: map[string]int{"Read": 2, "Bash": 20}},
	}
	projects := []scanner.Project{
		{Path: "/projects/app", Name: "app"},
	}

	result := AnalyzeToolUsage(sessions, projects)

	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}
	if result.Projects[0].DominantTool != "Bash" {
		t.Errorf("expected dominant tool 'Bash', got %q", result.Projects[0].DominantTool)
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"clean path", "/home/user/project", "/home/user/project"},
		{"trailing slash", "/home/user/project/", "/home/user/project"},
		{"double dots", "/home/user/../user/project", "/home/user/project"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := claude.NormalizePath(tt.input)
			if got != tt.want {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
