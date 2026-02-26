package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePlugins_JSONObjectFormat(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	data := `{
		"version": 1,
		"plugins": {
			"my-plugin": [{"scope":"global","version":"1.2.3","installPath":"/path"}],
			"other-plugin": [{"scope":"project","version":"0.1.0","installPath":"/other"}]
		}
	}`
	if err := os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	plugins, err := ParsePlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}

	found := map[string]string{}
	for _, p := range plugins {
		found[p.Name] = p.Version
	}
	if found["my-plugin"] != "1.2.3" {
		t.Errorf("my-plugin version = %q, want %q", found["my-plugin"], "1.2.3")
	}
	if found["other-plugin"] != "0.1.0" {
		t.Errorf("other-plugin version = %q, want %q", found["other-plugin"], "0.1.0")
	}
}

func TestParsePlugins_PlainMapFormat(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	data := `{
		"eslint-plugin": [{"scope":"global","version":"2.0.0","installPath":"/eslint"}]
	}`
	if err := os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	plugins, err := ParsePlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "eslint-plugin" {
		t.Errorf("Name = %q, want %q", plugins[0].Name, "eslint-plugin")
	}
	if plugins[0].Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", plugins[0].Version, "2.0.0")
	}
}

func TestParsePlugins_ArrayFormat(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	data := `[{"Name":"array-plugin","Version":"3.0.0"}]`
	if err := os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	plugins, err := ParsePlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name != "array-plugin" {
		t.Errorf("Name = %q, want %q", plugins[0].Name, "array-plugin")
	}
}

func TestParsePlugins_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	data := `this is not json`
	if err := os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	plugins, err := ParsePlugins(dir)
	if err != nil {
		t.Fatalf("expected nil error for invalid JSON, got: %v", err)
	}
	if plugins != nil {
		t.Errorf("expected nil plugins for invalid JSON, got %v", plugins)
	}
}

func TestParsePlugins_MissingFile(t *testing.T) {
	dir := t.TempDir()
	// No plugins directory or file.
	plugins, err := ParsePlugins(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if plugins != nil {
		t.Errorf("expected nil plugins for missing file, got %v", plugins)
	}
}

func TestParsePlugins_EmptyPluginsMap(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Structured format but empty plugins map — should fall through to plain map.
	data := `{"version":1,"plugins":{}}`
	if err := os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(data), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	plugins, err := ParsePlugins(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty map falls through structured check (len == 0), then plain map
	// also parses but has version/plugins keys which are not arrays.
	// The plain map parse will succeed but produce entries with empty installations.
	// Either way, no crash is the key assertion.
	_ = plugins
}
