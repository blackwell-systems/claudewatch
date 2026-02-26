package scanner

import (
	"testing"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/claude"
)

func TestComputeReadiness_PerfectScore(t *testing.T) {
	p := &Project{
		Path:              "/home/user/myproject",
		HasClaudeMD:       true,
		ClaudeMDSize:      1000,
		HasDotClaude:      true,
		HasLocalSettings:  true,
		CommitsLast30Days: 25,
	}

	sessions := []claude.SessionMeta{
		{
			SessionID:   "s1",
			ProjectPath: "/home/user/myproject",
			StartTime:   time.Now().Format(time.RFC3339),
		},
	}

	facets := []claude.SessionFacet{
		{SessionID: "s1"},
	}

	settings := &claude.GlobalSettings{
		Hooks: map[string][]claude.HookGroup{
			"pre-commit": {{Hooks: []claude.Hook{{Type: "command", Command: "lint"}}}},
		},
		EnabledPlugins: map[string]bool{
			"go-plugin": true,
		},
	}

	p.PrimaryLanguage = "go"

	score := ComputeReadiness(p, sessions, facets, settings)

	// 30 (claude.md) + 10 (size>500) + 10 (.claude) + 5 (local settings)
	// + 15 (recent session) + 10 (facets) + 10 (commits>20) + 5 (hooks) + 5 (plugin)
	// = 100
	if score != 100 {
		t.Errorf("expected perfect score 100, got %v", score)
	}
}

func TestComputeReadiness_NoSessions(t *testing.T) {
	p := &Project{
		Path:         "/home/user/bare",
		HasClaudeMD:  true,
		ClaudeMDSize: 600,
	}

	score := ComputeReadiness(p, nil, nil, nil)
	// 30 (claude.md) + 10 (size>500) = 40
	if score != 40 {
		t.Errorf("expected 40, got %v", score)
	}
}

func TestComputeReadiness_NoClaudeMD(t *testing.T) {
	p := &Project{
		Path:        "/home/user/nomd",
		HasClaudeMD: false,
	}

	score := ComputeReadiness(p, nil, nil, nil)
	if score != 0 {
		t.Errorf("expected 0, got %v", score)
	}
}

func TestComputeReadiness_SmallClaudeMD(t *testing.T) {
	p := &Project{
		Path:         "/home/user/small",
		HasClaudeMD:  true,
		ClaudeMDSize: 150,
	}

	score := ComputeReadiness(p, nil, nil, nil)
	// 30 + 5 (size 100-500) = 35
	if score != 35 {
		t.Errorf("expected 35, got %v", score)
	}
}

func TestComputeReadiness_MediumCommits(t *testing.T) {
	p := &Project{
		Path:              "/home/user/med",
		HasClaudeMD:       true,
		ClaudeMDSize:      50,
		CommitsLast30Days: 10,
	}

	score := ComputeReadiness(p, nil, nil, nil)
	// 30 + 0 (size<100) + 5 (commits 5-20) = 35
	if score != 35 {
		t.Errorf("expected 35, got %v", score)
	}
}

func TestComputeReadiness_FewCommits(t *testing.T) {
	p := &Project{
		Path:              "/home/user/few",
		HasClaudeMD:       true,
		ClaudeMDSize:      50,
		CommitsLast30Days: 2,
	}

	score := ComputeReadiness(p, nil, nil, nil)
	// 30 + 0 + 2 = 32
	if score != 32 {
		t.Errorf("expected 32, got %v", score)
	}
}

func TestRecencyWeight(t *testing.T) {
	tests := []struct {
		name      string
		startTime string
		minWeight float64
		maxWeight float64
	}{
		{
			name:      "now",
			startTime: time.Now().Format(time.RFC3339),
			minWeight: 0.9,
			maxWeight: 1.0,
		},
		{
			name:      "15 days ago",
			startTime: time.Now().Add(-15 * 24 * time.Hour).Format(time.RFC3339),
			minWeight: 0.4,
			maxWeight: 0.6,
		},
		{
			name:      "31 days ago",
			startTime: time.Now().Add(-31 * 24 * time.Hour).Format(time.RFC3339),
			minWeight: 0.0,
			maxWeight: 0.0,
		},
		{
			name:      "empty",
			startTime: "",
			minWeight: 0.0,
			maxWeight: 0.0,
		},
		{
			name:      "invalid",
			startTime: "not-a-date",
			minWeight: 0.0,
			maxWeight: 0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := recencyWeight(tc.startTime)
			if w < tc.minWeight || w > tc.maxWeight {
				t.Errorf("recencyWeight(%q) = %v, want [%v, %v]", tc.startTime, w, tc.minWeight, tc.maxWeight)
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/project/", "/home/user/project"},
		{"/home/user/project", "/home/user/project"},
		{"///", ""},
	}

	for _, tc := range tests {
		got := normalizePath(tc.input)
		if got != tc.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
