package app

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/store"
	"github.com/spf13/cobra"
)

var (
	tagProject string
	tagSession string
)

var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Override the project name for a session",
	Long: `Set or update the project name attributed to a session in claudewatch metrics.

Use this when Claude Code was launched from a different directory than the
project you were actually working in (e.g. SAW worktrees, subprojects).

If --session is omitted, the most recent session is tagged.`,
	RunE: runTag,
}

func init() {
	tagCmd.Flags().StringVar(&tagProject, "project", "", "Project name to attribute the session to (required)")
	tagCmd.Flags().StringVar(&tagSession, "session", "", "Session ID to tag (default: most recent session)")
	_ = tagCmd.MarkFlagRequired("project")
	rootCmd.AddCommand(tagCmd)
}

func runTag(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	sessionID := tagSession
	if sessionID == "" {
		sessions, err := claude.ParseAllSessionMeta(cfg.ClaudeHome)
		if err != nil {
			return fmt.Errorf("parsing session meta: %w", err)
		}
		if len(sessions) == 0 {
			return fmt.Errorf("no sessions found; use --session to specify a session ID")
		}
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].StartTime > sessions[j].StartTime
		})
		sessionID = sessions[0].SessionID
	}

	tagStorePath := filepath.Join(config.ConfigDir(), "session-tags.json")
	if err := store.NewSessionTagStore(tagStorePath).Set(sessionID, tagProject); err != nil {
		return fmt.Errorf("writing tag: %w", err)
	}

	fmt.Printf("Tagged: %s\nProject: %s\n", sessionID, tagProject)
	return nil
}
