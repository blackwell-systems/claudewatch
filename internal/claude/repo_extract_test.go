package claude

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

// makeAssistantEntry constructs a TranscriptEntry of type "assistant" with the
// given content blocks marshaled into the Message field.
func makeAssistantEntry(blocks []ContentBlock) TranscriptEntry {
	msg := AssistantMessage{
		Role:    "assistant",
		Content: blocks,
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return TranscriptEntry{
		Type:    "assistant",
		Message: json.RawMessage(raw),
	}
}

// makeToolUseBlock constructs a ContentBlock of type "tool_use" with the given
// tool name and input map.
func makeToolUseBlock(name string, inputMap map[string]string) ContentBlock {
	raw, err := json.Marshal(inputMap)
	if err != nil {
		panic(err)
	}
	return ContentBlock{
		Type:  "tool_use",
		Name:  name,
		Input: json.RawMessage(raw),
	}
}

func TestExtractFilePaths_ReadTool(t *testing.T) {
	entry := makeAssistantEntry([]ContentBlock{
		makeToolUseBlock("Read", map[string]string{"file_path": "/Users/x/code/foo/main.go"}),
	})
	paths := ExtractFilePaths(entry)
	if len(paths) != 1 || paths[0] != "/Users/x/code/foo/main.go" {
		t.Errorf("expected [/Users/x/code/foo/main.go], got %v", paths)
	}
}

func TestExtractFilePaths_EditTool(t *testing.T) {
	entry := makeAssistantEntry([]ContentBlock{
		makeToolUseBlock("Edit", map[string]string{"file_path": "/Users/x/code/bar/service.go"}),
	})
	paths := ExtractFilePaths(entry)
	if len(paths) != 1 || paths[0] != "/Users/x/code/bar/service.go" {
		t.Errorf("expected [/Users/x/code/bar/service.go], got %v", paths)
	}
}

func TestExtractFilePaths_WriteTool(t *testing.T) {
	entry := makeAssistantEntry([]ContentBlock{
		makeToolUseBlock("Write", map[string]string{"file_path": "/tmp/output/result.txt"}),
	})
	paths := ExtractFilePaths(entry)
	if len(paths) != 1 || paths[0] != "/tmp/output/result.txt" {
		t.Errorf("expected [/tmp/output/result.txt], got %v", paths)
	}
}

func TestExtractFilePaths_BashTool(t *testing.T) {
	entry := makeAssistantEntry([]ContentBlock{
		makeToolUseBlock("Bash", map[string]string{"command": "cat /Users/x/code/bar/README.md"}),
	})
	paths := ExtractFilePaths(entry)
	if len(paths) == 0 {
		t.Fatal("expected at least one path, got none")
	}
	found := false
	for _, p := range paths {
		if p == "/Users/x/code/bar/README.md" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected /Users/x/code/bar/README.md in %v", paths)
	}
}

func TestExtractFilePaths_NonAssistant(t *testing.T) {
	msg := AssistantMessage{
		Role: "user",
		Content: []ContentBlock{
			makeToolUseBlock("Read", map[string]string{"file_path": "/tmp/something.go"}),
		},
	}
	raw, _ := json.Marshal(msg)
	entry := TranscriptEntry{
		Type:    "user",
		Message: json.RawMessage(raw),
	}
	paths := ExtractFilePaths(entry)
	if paths != nil {
		t.Errorf("expected nil for non-assistant entry, got %v", paths)
	}
}

func TestExtractFilePaths_NoToolUse(t *testing.T) {
	entry := makeAssistantEntry([]ContentBlock{
		{Type: "text", Text: "Here is my analysis of the code."},
	})
	paths := ExtractFilePaths(entry)
	if paths != nil {
		t.Errorf("expected nil for entry with no tool_use blocks, got %v", paths)
	}
}

func TestExtractFilePaths_Deduplication(t *testing.T) {
	entry := makeAssistantEntry([]ContentBlock{
		makeToolUseBlock("Read", map[string]string{"file_path": "/Users/x/code/foo/main.go"}),
		makeToolUseBlock("Edit", map[string]string{"file_path": "/Users/x/code/foo/main.go"}),
		makeToolUseBlock("Write", map[string]string{"file_path": "/Users/x/code/foo/main.go"}),
	})
	paths := ExtractFilePaths(entry)
	if len(paths) != 1 {
		t.Errorf("expected 1 deduplicated path, got %d: %v", len(paths), paths)
	}
}

// initGitRepo initializes a git repo in dir and creates a dummy commit so that
// git rev-parse --show-toplevel returns a valid root.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, out)
		}
	}
}

func TestComputeProjectWeights_MultiRepo(t *testing.T) {
	// Create two separate temp git repos.
	repoA := t.TempDir()
	repoB := t.TempDir()
	initGitRepo(t, repoA)
	initGitRepo(t, repoB)

	// Clear cache entries that may have been set from previous tests.
	repoRootCache.Delete(repoA)
	repoRootCache.Delete(repoB)

	// Create file paths inside each repo.
	fileA := filepath.Join(repoA, "main.go")
	fileB := filepath.Join(repoB, "service.go")

	// 2 entries touching repoA, 1 touching repoB.
	entries := []TranscriptEntry{
		makeAssistantEntry([]ContentBlock{
			makeToolUseBlock("Read", map[string]string{"file_path": fileA}),
		}),
		makeAssistantEntry([]ContentBlock{
			makeToolUseBlock("Edit", map[string]string{"file_path": fileA}),
		}),
		makeAssistantEntry([]ContentBlock{
			makeToolUseBlock("Read", map[string]string{"file_path": fileB}),
		}),
	}

	weights := ComputeProjectWeights(entries, "/fallback")
	if weights == nil {
		t.Fatal("expected non-nil result")
	}
	if len(weights) != 2 {
		t.Fatalf("expected 2 weights, got %d: %v", len(weights), weights)
	}

	// Weights must sum to ~1.0.
	sum := 0.0
	for _, w := range weights {
		sum += w.Weight
	}
	if sum < 0.999 || sum > 1.001 {
		t.Errorf("weights should sum to 1.0, got %f", sum)
	}

	// Highest weight first.
	if weights[0].Weight < weights[1].Weight {
		t.Errorf("expected descending order: %v", weights)
	}
}

func TestComputeProjectWeights_SingleRepo(t *testing.T) {
	repo := t.TempDir()
	initGitRepo(t, repo)
	repoRootCache.Delete(repo)

	fileA := filepath.Join(repo, "a.go")
	fileB := filepath.Join(repo, "b.go")

	entries := []TranscriptEntry{
		makeAssistantEntry([]ContentBlock{
			makeToolUseBlock("Read", map[string]string{"file_path": fileA}),
		}),
		makeAssistantEntry([]ContentBlock{
			makeToolUseBlock("Edit", map[string]string{"file_path": fileB}),
		}),
	}

	weights := ComputeProjectWeights(entries, "/fallback")
	if len(weights) != 1 {
		t.Fatalf("expected 1 weight for single repo, got %d: %v", len(weights), weights)
	}
	if weights[0].Weight != 1.0 {
		t.Errorf("expected weight 1.0, got %f", weights[0].Weight)
	}
}

func TestComputeProjectWeights_Fallback(t *testing.T) {
	// Entries with no tool_use blocks that produce file paths.
	entries := []TranscriptEntry{
		makeAssistantEntry([]ContentBlock{
			{Type: "text", Text: "No file operations here."},
		}),
	}

	weights := ComputeProjectWeights(entries, "/Users/x/code/myproject")
	if weights == nil {
		t.Fatal("expected non-nil result")
	}
	if len(weights) != 1 {
		t.Fatalf("expected 1 fallback weight, got %d: %v", len(weights), weights)
	}
	if weights[0].Weight != 1.0 {
		t.Errorf("expected weight 1.0, got %f", weights[0].Weight)
	}
	if weights[0].Project != "myproject" {
		t.Errorf("expected project 'myproject', got %q", weights[0].Project)
	}
	if weights[0].RepoRoot != "/Users/x/code/myproject" {
		t.Errorf("expected RepoRoot '/Users/x/code/myproject', got %q", weights[0].RepoRoot)
	}
	if weights[0].ToolCalls != 0 {
		t.Errorf("expected ToolCalls 0, got %d", weights[0].ToolCalls)
	}
}

func TestResolveRepoRoot_Cache(t *testing.T) {
	repo := t.TempDir()
	initGitRepo(t, repo)

	// Clear any cached entry for this dir before testing.
	repoRootCache.Delete(repo)

	filePath := filepath.Join(repo, "somefile.go")

	// First call — should compute and cache.
	root1 := ResolveRepoRoot(filePath)
	if root1 == "" {
		t.Fatal("expected a non-empty repo root")
	}

	// Manually verify the cache is populated.
	cached, ok := repoRootCache.Load(repo)
	if !ok {
		t.Fatal("expected cache entry after first call")
	}
	if cached.(string) != root1 {
		t.Errorf("cached value %q does not match returned root %q", cached.(string), root1)
	}

	// Second call — should return the same value (from cache).
	root2 := ResolveRepoRoot(filePath)
	if root2 != root1 {
		t.Errorf("second call returned %q, first returned %q", root2, root1)
	}
}
