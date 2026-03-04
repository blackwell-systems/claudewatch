package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/spf13/cobra"
)

var (
	memoryFlagProject string
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Query and manage working memory for a project",
	Long: `Working memory stores task history, blockers, and context hints
for each project. Use 'memory show' to view stored memory, and
'memory clear' to delete it.`,
}

var memoryShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display working memory for a project",
	Long: `Show task history, blockers, and context hints stored in working memory.
If --project is not specified, derives project name from current directory.`,
	RunE: runMemoryShow,
}

var memoryClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete working memory for a project",
	Long: `Delete the working memory file for a project.
Prompts for confirmation before deletion.
If --project is not specified, derives project name from current directory.`,
	RunE: runMemoryClear,
}

func init() {
	memoryCmd.PersistentFlags().StringVar(&memoryFlagProject, "project", "", "Project name (defaults to basename of current directory)")

	memoryCmd.AddCommand(memoryShowCmd)
	memoryCmd.AddCommand(memoryClearCmd)

	rootCmd.AddCommand(memoryCmd)
}

// getProjectName returns the project name from the --project flag,
// or derives it from the current working directory.
func getProjectName() (string, error) {
	if memoryFlagProject != "" {
		return memoryFlagProject, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting current directory: %w", err)
	}

	return filepath.Base(cwd), nil
}

// getMemoryStorePath returns the full path to the working-memory.json file
// for the given project name.
func getMemoryStorePath(projectName string) string {
	return filepath.Join(config.ConfigDir(), "projects", projectName, "working-memory.json")
}

func runMemoryShow(cmd *cobra.Command, args []string) error {
	if flagNoColor {
		output.SetNoColor(true)
	}

	projectName, err := getProjectName()
	if err != nil {
		return err
	}

	storePath := getMemoryStorePath(projectName)
	memStore := store.NewWorkingMemoryStore(storePath)

	wm, err := memStore.Load()
	if err != nil {
		return fmt.Errorf("loading working memory: %w", err)
	}

	fmt.Printf("# Working Memory — %s\n\n", projectName)

	// Check if memory is empty
	if len(wm.Tasks) == 0 && len(wm.Blockers) == 0 && len(wm.ContextHints) == 0 {
		fmt.Println("No task history or blockers recorded yet.")
		return nil
	}

	// Tasks section
	if len(wm.Tasks) > 0 {
		fmt.Printf("## Tasks (%d)\n\n", len(wm.Tasks))

		for _, task := range wm.Tasks {
			fmt.Printf("### \"%s\"\n", task.TaskIdentifier)

			// Sessions
			if len(task.Sessions) > 0 {
				sessionIDs := make([]string, len(task.Sessions))
				for i, sid := range task.Sessions {
					if len(sid) > 7 {
						sessionIDs[i] = sid[:7]
					} else {
						sessionIDs[i] = sid
					}
				}
				fmt.Printf("  Sessions: %d (%s)\n", len(task.Sessions), strings.Join(sessionIDs, ", "))
			}

			// Status
			fmt.Printf("  Status:   %s\n", task.Status)

			// Commits
			if len(task.Commits) > 0 {
				commitSHAs := make([]string, len(task.Commits))
				for i, sha := range task.Commits {
					if len(sha) > 7 {
						commitSHAs[i] = sha[:7]
					} else {
						commitSHAs[i] = sha
					}
				}
				fmt.Printf("  Commits:  %d (%s)\n", len(task.Commits), strings.Join(commitSHAs, ", "))
			}

			// Solution
			if task.Solution != "" {
				fmt.Printf("  Solution: %s\n", task.Solution)
			}

			// Blockers
			if len(task.BlockersHit) > 0 {
				fmt.Printf("  Blockers: %s\n", strings.Join(task.BlockersHit, ", "))
			}

			fmt.Println()
		}
	}

	// Blockers section
	if len(wm.Blockers) > 0 {
		fmt.Printf("## Blockers (%d)\n\n", len(wm.Blockers))

		for _, blocker := range wm.Blockers {
			filePrefix := ""
			if blocker.File != "" {
				filePrefix = fmt.Sprintf("**%s** — ", blocker.File)
			}

			fmt.Printf("- %s%s\n", filePrefix, blocker.Issue)

			if blocker.Solution != "" {
				fmt.Printf("  Solution: %s\n", blocker.Solution)
			}

			if !blocker.LastSeen.IsZero() {
				fmt.Printf("  Last seen: %s\n", blocker.LastSeen.Format("2006-01-02"))
			}

			fmt.Println()
		}
	}

	// Context Hints section
	if len(wm.ContextHints) > 0 {
		fmt.Printf("## Context Hints (%d)\n\n", len(wm.ContextHints))

		for _, hint := range wm.ContextHints {
			fmt.Printf("- %s\n", hint)
		}
		fmt.Println()
	}

	return nil
}

func runMemoryClear(cmd *cobra.Command, args []string) error {
	projectName, err := getProjectName()
	if err != nil {
		return err
	}

	storePath := getMemoryStorePath(projectName)

	// Check if file exists
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		fmt.Printf("No working memory found for %s.\n", projectName)
		return nil
	}

	// Prompt for confirmation
	fmt.Printf("Delete working memory for %s? (y/N): ", projectName)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading confirmation: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response != "y" && response != "yes" {
		fmt.Println("Cancelled.")
		return nil
	}

	// Delete the file
	if err := os.Remove(storePath); err != nil {
		return fmt.Errorf("deleting working memory: %w", err)
	}

	fmt.Printf("Working memory cleared for %s.\n", projectName)
	return nil
}
