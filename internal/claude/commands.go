package claude

import (
	"os"
	"path/filepath"
	"strings"
)

// ListCommands lists custom slash command files from ~/.claude/commands/*.md.
func ListCommands(claudeHome string) ([]CommandFile, error) {
	dir := filepath.Join(claudeHome, "commands")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var commands []CommandFile
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		commands = append(commands, CommandFile{
			Name:    strings.TrimSuffix(entry.Name(), ".md"),
			Path:    path,
			Content: string(data),
		})
	}
	return commands, nil
}
