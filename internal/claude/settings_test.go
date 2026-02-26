package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSettings_ValidFile(t *testing.T) {
	dir := t.TempDir()
	data := `{
		"includeCoAuthoredBy": true,
		"permissions": {
			"allow_bash": true,
			"allow_read": true,
			"allow_write": false,
			"allow_mcp": true
		},
		"hooks": {
			"pre-commit": [
				{
					"matcher": "*.go",
					"hooks": [{"type":"command","command":"go vet ./..."}]
				}
			]
		},
		"enabledPlugins": {"my-plugin": true, "other-plugin": false},
		"preferences": {"theme": "dark"},
		"effortLevel": "high"
	}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	settings, err := ParseSettings(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings == nil {
		t.Fatal("expected non-nil settings")
	}
	if !settings.IncludeCoAuthoredBy {
		t.Error("expected IncludeCoAuthoredBy = true")
	}
	if !settings.Permissions.AllowBash {
		t.Error("expected AllowBash = true")
	}
	if settings.Permissions.AllowWrite {
		t.Error("expected AllowWrite = false")
	}
	if settings.EffortLevel != "high" {
		t.Errorf("EffortLevel = %q, want %q", settings.EffortLevel, "high")
	}
	if settings.Preferences["theme"] != "dark" {
		t.Errorf("Preferences[theme] = %q, want %q", settings.Preferences["theme"], "dark")
	}
	if !settings.EnabledPlugins["my-plugin"] {
		t.Error("expected enabledPlugins[my-plugin] = true")
	}
	if settings.EnabledPlugins["other-plugin"] {
		t.Error("expected enabledPlugins[other-plugin] = false")
	}
	hooks, ok := settings.Hooks["pre-commit"]
	if !ok || len(hooks) != 1 {
		t.Fatalf("expected 1 pre-commit hook group, got %d", len(hooks))
	}
	if hooks[0].Matcher != "*.go" {
		t.Errorf("hook matcher = %q, want %q", hooks[0].Matcher, "*.go")
	}
	if len(hooks[0].Hooks) != 1 || hooks[0].Hooks[0].Command != "go vet ./..." {
		t.Errorf("unexpected hook command: %+v", hooks[0].Hooks)
	}
}

func TestParseSettings_MissingFile(t *testing.T) {
	dir := t.TempDir()
	settings, err := ParseSettings(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if settings != nil {
		t.Errorf("expected nil settings for missing file, got %+v", settings)
	}
}

func TestParseSettings_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("not json"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	settings, err := ParseSettings(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if settings != nil {
		t.Errorf("expected nil settings on error, got %+v", settings)
	}
}

func TestParseSettings_EmptyJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	settings, err := ParseSettings(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings == nil {
		t.Fatal("expected non-nil settings for empty JSON object")
	}
	if settings.IncludeCoAuthoredBy {
		t.Error("expected default IncludeCoAuthoredBy = false")
	}
}

func TestParseSettings_MinimalFields(t *testing.T) {
	dir := t.TempDir()
	data := `{"effortLevel":"low"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	settings, err := ParseSettings(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.EffortLevel != "low" {
		t.Errorf("EffortLevel = %q, want %q", settings.EffortLevel, "low")
	}
}
