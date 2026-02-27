package scanner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

// ---------------------------------------------------------------------------
// DiscoverProjects
// ---------------------------------------------------------------------------

func TestDiscoverProjects_FindsGitRepos(t *testing.T) {
	root := t.TempDir()

	// Create two git repos.
	for _, name := range []string{"alpha", "bravo"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	// Sorted alphabetically.
	if projects[0].Name != "alpha" {
		t.Errorf("expected first project 'alpha', got %q", projects[0].Name)
	}
	if projects[1].Name != "bravo" {
		t.Errorf("expected second project 'bravo', got %q", projects[1].Name)
	}
	for _, p := range projects {
		if !p.HasGit {
			t.Errorf("project %q should have HasGit=true", p.Name)
		}
	}
}

func TestDiscoverProjects_SkipsNonGitDirs(t *testing.T) {
	root := t.TempDir()

	// Directory without .git.
	if err := os.MkdirAll(filepath.Join(root, "not-a-repo"), 0o755); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestDiscoverProjects_SkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()

	// Hidden directory that is a git repo.
	hidden := filepath.Join(root, ".hidden-repo")
	if err := os.MkdirAll(filepath.Join(hidden, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects (hidden dirs skipped), got %d", len(projects))
	}
}

func TestDiscoverProjects_SkipsFiles(t *testing.T) {
	root := t.TempDir()

	// A regular file at the root level should be skipped.
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestDiscoverProjects_NonExistentPathSkipped(t *testing.T) {
	projects, err := DiscoverProjects([]string{"/tmp/does-not-exist-claudewatch-test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects for non-existent path, got %d", len(projects))
	}
}

func TestDiscoverProjects_EmptyPaths(t *testing.T) {
	projects, err := DiscoverProjects(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects for nil paths, got %d", len(projects))
	}
}

func TestDiscoverProjects_DetectsClaudeMD(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "myproj")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := make([]byte, 600)
	for i := range content {
		content[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(project, "CLAUDE.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if !projects[0].HasClaudeMD {
		t.Error("expected HasClaudeMD=true")
	}
	if projects[0].ClaudeMDSize != 600 {
		t.Errorf("expected ClaudeMDSize=600, got %d", projects[0].ClaudeMDSize)
	}
}

func TestDiscoverProjects_DetectsDotClaude(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "myproj")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if !projects[0].HasDotClaude {
		t.Error("expected HasDotClaude=true")
	}
}

func TestDiscoverProjects_DetectsLocalSettings(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "myproj")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".claude", "settings.local.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if !projects[0].HasLocalSettings {
		t.Error("expected HasLocalSettings=true")
	}
}

func TestDiscoverProjects_DetectsLanguage(t *testing.T) {
	tests := []struct {
		file string
		lang string
	}{
		{"go.mod", "Go"},
		{"Cargo.toml", "Rust"},
		{"package.json", "JavaScript"},
		{"pyproject.toml", "Python"},
		{"setup.py", "Python"},
	}

	for _, tc := range tests {
		t.Run(tc.lang+"_"+tc.file, func(t *testing.T) {
			root := t.TempDir()
			project := filepath.Join(root, "proj")
			if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(project, tc.file), []byte(""), 0o644); err != nil {
				t.Fatal(err)
			}

			projects, err := DiscoverProjects([]string{root})
			if err != nil {
				t.Fatal(err)
			}
			if len(projects) != 1 {
				t.Fatalf("expected 1 project, got %d", len(projects))
			}
			if projects[0].PrimaryLanguage != tc.lang {
				t.Errorf("expected language %q, got %q", tc.lang, projects[0].PrimaryLanguage)
			}
		})
	}
}

func TestDiscoverProjects_NoLanguageIndicator(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "empty")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].PrimaryLanguage != "" {
		t.Errorf("expected empty language, got %q", projects[0].PrimaryLanguage)
	}
}

func TestDiscoverProjects_DeduplicatesSamePath(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "dup")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Pass the same root twice.
	projects, err := DiscoverProjects([]string{root, root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 project (deduplicated), got %d", len(projects))
	}
}

func TestDiscoverProjects_MultiplePaths(t *testing.T) {
	root1 := t.TempDir()
	root2 := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root1, "proj-a", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root2, "proj-b", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects([]string{root1, root2})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestDiscoverProjects_SortedCaseInsensitive(t *testing.T) {
	root := t.TempDir()

	for _, name := range []string{"Zebra", "alpha", "Bravo"} {
		if err := os.MkdirAll(filepath.Join(root, name, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	projects, err := DiscoverProjects([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(projects))
	}
	expected := []string{"alpha", "Bravo", "Zebra"}
	for i, want := range expected {
		if projects[i].Name != want {
			t.Errorf("position %d: expected %q, got %q", i, want, projects[i].Name)
		}
	}
}

// ---------------------------------------------------------------------------
// FilterFacetsByProject
// ---------------------------------------------------------------------------

func TestFilterFacetsByProject_MatchesBySessionID(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/proj/a"},
		{SessionID: "s2", ProjectPath: "/proj/b"},
		{SessionID: "s3", ProjectPath: "/proj/a"},
	}
	facets := []claude.SessionFacet{
		{SessionID: "s1", BriefSummary: "facet1"},
		{SessionID: "s2", BriefSummary: "facet2"},
		{SessionID: "s3", BriefSummary: "facet3"},
		{SessionID: "s4", BriefSummary: "orphan"},
	}

	result := FilterFacetsByProject(facets, sessions, "/proj/a")
	if len(result) != 2 {
		t.Fatalf("expected 2 facets for /proj/a, got %d", len(result))
	}
	if result[0].BriefSummary != "facet1" || result[1].BriefSummary != "facet3" {
		t.Errorf("unexpected facets: %+v", result)
	}
}

func TestFilterFacetsByProject_NoMatchingProject(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/proj/a"},
	}
	facets := []claude.SessionFacet{
		{SessionID: "s1", BriefSummary: "facet1"},
	}

	result := FilterFacetsByProject(facets, sessions, "/proj/nonexistent")
	if len(result) != 0 {
		t.Errorf("expected 0 facets, got %d", len(result))
	}
}

func TestFilterFacetsByProject_EmptyInputs(t *testing.T) {
	// All empty.
	result := FilterFacetsByProject(nil, nil, "/proj/a")
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}

	// Sessions but no facets.
	sessions := []claude.SessionMeta{{SessionID: "s1", ProjectPath: "/proj/a"}}
	result = FilterFacetsByProject(nil, sessions, "/proj/a")
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}

	// Facets but no sessions.
	facets := []claude.SessionFacet{{SessionID: "s1"}}
	result = FilterFacetsByProject(facets, nil, "/proj/a")
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestFilterFacetsByProject_PathNormalization(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/proj/a/"},
	}
	facets := []claude.SessionFacet{
		{SessionID: "s1", BriefSummary: "facet1"},
	}

	// Path without trailing slash should still match.
	result := FilterFacetsByProject(facets, sessions, "/proj/a")
	if len(result) != 1 {
		t.Errorf("expected 1 facet with normalized path, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// filterByProject
// ---------------------------------------------------------------------------

func TestFilterByProject_Empty(t *testing.T) {
	result := filterByProject(nil, "/proj/a")
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestFilterByProject_MultipleMatches(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/proj/a", StartTime: "2025-01-03T00:00:00Z"},
		{SessionID: "s2", ProjectPath: "/proj/b", StartTime: "2025-01-02T00:00:00Z"},
		{SessionID: "s3", ProjectPath: "/proj/a", StartTime: "2025-01-01T00:00:00Z"},
	}

	result := filterByProject(sessions, "/proj/a")
	if len(result) != 2 {
		t.Fatalf("expected 2, got %d", len(result))
	}
	// Should be sorted ascending by StartTime.
	if result[0].SessionID != "s3" {
		t.Errorf("expected s3 first (earliest), got %q", result[0].SessionID)
	}
	if result[1].SessionID != "s1" {
		t.Errorf("expected s1 second (latest), got %q", result[1].SessionID)
	}
}

func TestFilterByProject_NoMatch(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/proj/a"},
	}

	result := filterByProject(sessions, "/proj/b")
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestFilterByProject_PathNormalization(t *testing.T) {
	sessions := []claude.SessionMeta{
		{SessionID: "s1", ProjectPath: "/proj/a/"},
	}

	result := filterByProject(sessions, "/proj/a")
	if len(result) != 1 {
		t.Errorf("expected 1 match with normalized path, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// recencyWeight (additional boundary tests)
// ---------------------------------------------------------------------------

func TestRecencyWeight_ExactlyZeroDays(t *testing.T) {
	// A time in the very near future should return 1.0.
	future := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	w := recencyWeight(future)
	if w != 1.0 {
		t.Errorf("expected 1.0 for future time, got %v", w)
	}
}

func TestRecencyWeight_Exactly30Days(t *testing.T) {
	thirtyDays := time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	w := recencyWeight(thirtyDays)
	if w != 0.0 {
		t.Errorf("expected 0.0 for exactly 30 days ago, got %v", w)
	}
}

func TestRecencyWeight_DateOnlyFormat(t *testing.T) {
	// Today's date in YYYY-MM-DD format (the fallback parser).
	today := time.Now().Format("2006-01-02")
	w := recencyWeight(today)
	if w < 0.9 || w > 1.0 {
		t.Errorf("expected weight near 1.0 for today's date-only format, got %v", w)
	}
}

func TestRecencyWeight_DateOnlyOld(t *testing.T) {
	old := time.Now().Add(-45 * 24 * time.Hour).Format("2006-01-02")
	w := recencyWeight(old)
	if w != 0.0 {
		t.Errorf("expected 0.0 for 45-day-old date-only format, got %v", w)
	}
}

func TestRecencyWeight_OneDayAgo(t *testing.T) {
	oneDay := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	w := recencyWeight(oneDay)
	// Should be approximately 1 - 1/30 ≈ 0.967
	if w < 0.93 || w > 1.0 {
		t.Errorf("expected weight ~0.967 for 1 day ago, got %v", w)
	}
}

func TestRecencyWeight_TwentyNineDays(t *testing.T) {
	d := time.Now().Add(-29 * 24 * time.Hour).Format(time.RFC3339)
	w := recencyWeight(d)
	// Should be approximately 1 - 29/30 ≈ 0.033
	if w < 0.0 || w > 0.1 {
		t.Errorf("expected small positive weight for 29 days ago, got %v", w)
	}
}

// ---------------------------------------------------------------------------
// hasRelevantPlugin
// ---------------------------------------------------------------------------

func TestHasRelevantPlugin_Match(t *testing.T) {
	settings := &claude.GlobalSettings{
		EnabledPlugins: map[string]bool{
			"go-lint":    true,
			"py-checker": false,
		},
	}
	if !hasRelevantPlugin(settings, "Go") {
		t.Error("expected true for Go with go-lint enabled")
	}
}

func TestHasRelevantPlugin_DisabledPlugin(t *testing.T) {
	settings := &claude.GlobalSettings{
		EnabledPlugins: map[string]bool{
			"python-lint": false,
		},
	}
	if hasRelevantPlugin(settings, "Python") {
		t.Error("expected false when plugin is disabled")
	}
}

func TestHasRelevantPlugin_NoPlugins(t *testing.T) {
	settings := &claude.GlobalSettings{
		EnabledPlugins: map[string]bool{},
	}
	if hasRelevantPlugin(settings, "Go") {
		t.Error("expected false with empty plugin map")
	}
}

func TestHasRelevantPlugin_NilPlugins(t *testing.T) {
	settings := &claude.GlobalSettings{}
	if hasRelevantPlugin(settings, "Go") {
		t.Error("expected false with nil plugin map")
	}
}

func TestHasRelevantPlugin_EmptyLanguage(t *testing.T) {
	settings := &claude.GlobalSettings{
		EnabledPlugins: map[string]bool{
			"go-plugin": true,
		},
	}
	if hasRelevantPlugin(settings, "") {
		t.Error("expected false with empty language")
	}
}

func TestHasRelevantPlugin_CaseInsensitive(t *testing.T) {
	settings := &claude.GlobalSettings{
		EnabledPlugins: map[string]bool{
			"JavaScript-Prettier": true,
		},
	}
	if !hasRelevantPlugin(settings, "javascript") {
		t.Error("expected true for case-insensitive match")
	}
}

func TestHasRelevantPlugin_NoMatchingLanguage(t *testing.T) {
	settings := &claude.GlobalSettings{
		EnabledPlugins: map[string]bool{
			"go-lint": true,
		},
	}
	if hasRelevantPlugin(settings, "Rust") {
		t.Error("expected false when no plugin matches language")
	}
}

// ---------------------------------------------------------------------------
// ComputeReadiness (additional edge cases)
// ---------------------------------------------------------------------------

func TestComputeReadiness_ClaudeMDSizeBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		expected float64
	}{
		{"size_0", 0, 30},     // 30 for HasClaudeMD, 0 for size
		{"size_100", 100, 30}, // exactly 100 is not > 100
		{"size_101", 101, 35}, // 30 + 5
		{"size_500", 500, 35}, // exactly 500 is not > 500
		{"size_501", 501, 40}, // 30 + 10
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Project{
				Path:         "/home/user/test",
				HasClaudeMD:  true,
				ClaudeMDSize: tc.size,
			}
			score := ComputeReadiness(p, nil, nil, nil)
			if score != tc.expected {
				t.Errorf("ClaudeMDSize=%d: expected score %v, got %v", tc.size, tc.expected, score)
			}
		})
	}
}

func TestComputeReadiness_CommitBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		commits int
		extra   float64
	}{
		{"zero_commits", 0, 0},
		{"one_commit", 1, 2},
		{"five_commits", 5, 2}, // exactly 5 is not > 5
		{"six_commits", 6, 5},
		{"twenty_commits", 20, 5}, // exactly 20 is not > 20
		{"twentyone_commits", 21, 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Project{
				Path:              "/home/user/test",
				CommitsLast30Days: tc.commits,
			}
			score := ComputeReadiness(p, nil, nil, nil)
			if score != tc.extra {
				t.Errorf("commits=%d: expected score %v, got %v", tc.commits, tc.extra, score)
			}
		})
	}
}

func TestComputeReadiness_HooksNoPlugins(t *testing.T) {
	p := &Project{Path: "/home/user/test"}
	settings := &claude.GlobalSettings{
		Hooks: map[string][]claude.HookGroup{
			"pre-commit": {{Hooks: []claude.Hook{{Type: "command", Command: "lint"}}}},
		},
	}

	score := ComputeReadiness(p, nil, nil, settings)
	// Only hooks: 5
	if score != 5 {
		t.Errorf("expected 5 (hooks only), got %v", score)
	}
}

func TestComputeReadiness_PluginsNoHooks(t *testing.T) {
	p := &Project{
		Path:            "/home/user/test",
		PrimaryLanguage: "Go",
	}
	settings := &claude.GlobalSettings{
		EnabledPlugins: map[string]bool{
			"go-plugin": true,
		},
	}

	score := ComputeReadiness(p, nil, nil, settings)
	// Only plugin: 5
	if score != 5 {
		t.Errorf("expected 5 (plugin only), got %v", score)
	}
}

func TestComputeReadiness_SessionsButNoFacets(t *testing.T) {
	p := &Project{
		Path:         "/home/user/test",
		HasClaudeMD:  true,
		ClaudeMDSize: 600,
	}
	sessions := []claude.SessionMeta{
		{
			SessionID:   "s1",
			ProjectPath: "/home/user/test",
			StartTime:   time.Now().Format(time.RFC3339),
		},
	}

	score := ComputeReadiness(p, sessions, nil, nil)
	// 30 (claudemd) + 10 (size) + ~15 (session recency) + 0 (no facets)
	if score < 54 || score > 55.1 {
		t.Errorf("expected score ~55 (no facets), got %v", score)
	}
}

func TestComputeReadiness_OldSessionLowRecency(t *testing.T) {
	p := &Project{
		Path: "/home/user/test",
	}
	sessions := []claude.SessionMeta{
		{
			SessionID:   "s1",
			ProjectPath: "/home/user/test",
			StartTime:   time.Now().Add(-25 * 24 * time.Hour).Format(time.RFC3339),
		},
	}

	score := ComputeReadiness(p, sessions, nil, nil)
	// Session recency: 15 * (1 - 25/30) = 15 * 0.167 ≈ 2.5
	if score < 2.0 || score > 3.0 {
		t.Errorf("expected score ~2.5 for old session, got %v", score)
	}
}

func TestComputeReadiness_NilSettings(t *testing.T) {
	p := &Project{
		Path:            "/home/user/test",
		PrimaryLanguage: "Go",
	}

	// Nil settings should not panic and should contribute 0 for hooks/plugins.
	score := ComputeReadiness(p, nil, nil, nil)
	if score != 0 {
		t.Errorf("expected 0 with nil settings and no other signals, got %v", score)
	}
}

func TestComputeReadiness_EmptyHooksMap(t *testing.T) {
	p := &Project{Path: "/home/user/test"}
	settings := &claude.GlobalSettings{
		Hooks: map[string][]claude.HookGroup{},
	}

	score := ComputeReadiness(p, nil, nil, settings)
	if score != 0 {
		t.Errorf("expected 0 with empty hooks map, got %v", score)
	}
}

// ---------------------------------------------------------------------------
// detectLanguage (via DiscoverProjects indirectly, but also direct tests)
// ---------------------------------------------------------------------------

func TestDetectLanguage_Priority(t *testing.T) {
	// When both go.mod and package.json exist, go.mod should win (first match).
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	lang := detectLanguage(root)
	if lang != "Go" {
		t.Errorf("expected Go (higher priority), got %q", lang)
	}
}

func TestDetectLanguage_Empty(t *testing.T) {
	root := t.TempDir()
	lang := detectLanguage(root)
	if lang != "" {
		t.Errorf("expected empty string, got %q", lang)
	}
}
