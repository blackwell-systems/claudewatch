package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/fixer"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
)

var (
	fixFlagDryRun bool
	fixFlagAll    bool
	fixFlagJSON   bool
)

var fixCmd = &cobra.Command{
	Use:   "fix [project-path-or-name]",
	Short: "Generate CLAUDE.md improvements from session data",
	Long: `Analyze session data for a project and generate concrete CLAUDE.md
additions based on observed friction patterns, missing sections, recurring
corrections, and wasted sessions.

The fix command never removes existing content — it only proposes additions.
By default it presents proposed changes for confirmation before writing.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFix,
}

func init() {
	fixCmd.Flags().BoolVar(&fixFlagDryRun, "dry-run", false, "Print proposed additions without applying")
	fixCmd.Flags().BoolVar(&fixFlagAll, "all", false, "Fix all projects with score < 50")
	fixCmd.Flags().BoolVar(&fixFlagJSON, "json", false, "Output proposed changes as JSON")
	rootCmd.AddCommand(fixCmd)
}

func runFix(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if flagNoColor {
		output.SetNoColor(true)
	}

	// Discover all projects.
	projects, err := scanner.DiscoverProjects(cfg.ScanPaths)
	if err != nil {
		return fmt.Errorf("discovering projects: %w", err)
	}

	if len(projects) == 0 {
		return fmt.Errorf("no projects found in scan paths")
	}

	// Determine which projects to fix.
	var targets []scanner.Project

	if fixFlagAll {
		for _, p := range projects {
			if p.Score < 50 {
				targets = append(targets, p)
			}
		}
		if len(targets) == 0 {
			fmt.Println(" All projects have a readiness score >= 50. Nothing to fix.")
			return nil
		}
	} else {
		if len(args) == 0 {
			return fmt.Errorf("specify a project path or name, or use --all")
		}
		target, err := resolveProject(args[0], projects, cfg.ScanPaths)
		if err != nil {
			return err
		}
		targets = []scanner.Project{*target}
	}

	// Process each target project.
	for _, target := range targets {
		if err := fixProject(target, cfg); err != nil {
			fmt.Fprintf(os.Stderr, " Error fixing %s: %v\n", target.Name, err)
			continue
		}

		// Add spacing between projects in --all mode.
		if fixFlagAll && len(targets) > 1 {
			fmt.Println()
		}
	}

	return nil
}

// fixProject generates and applies fixes for a single project.
func fixProject(project scanner.Project, cfg *config.Config) error {
	// Build analysis context.
	ctx, err := fixer.BuildFixContext(project, cfg)
	if err != nil {
		return fmt.Errorf("building fix context: %w", err)
	}

	// Generate proposed fixes.
	fix, err := fixer.GenerateFix(ctx)
	if err != nil {
		return fmt.Errorf("generating fix: %w", err)
	}

	if len(fix.Additions) == 0 {
		fmt.Printf(" %s: no improvements identified.\n", project.Name)
		return nil
	}

	// JSON output mode.
	if fixFlagJSON || flagJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(fix)
	}

	// Render terminal output.
	renderFixProposal(fix, ctx)

	// In dry-run mode, stop here.
	if fixFlagDryRun {
		return nil
	}

	// Ask for confirmation.
	if !confirmApply() {
		fmt.Println(" Changes not applied.")
		return nil
	}

	// Apply the changes.
	return applyFix(fix, ctx)
}

// resolveProject finds a project by name or path from the discovered projects list.
func resolveProject(nameOrPath string, projects []scanner.Project, scanPaths []string) (*scanner.Project, error) {
	// Try exact path match first.
	absPath, err := filepath.Abs(nameOrPath)
	if err == nil {
		for i := range projects {
			if projects[i].Path == absPath {
				return &projects[i], nil
			}
		}
	}

	// Expand ~ in the path.
	if strings.HasPrefix(nameOrPath, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			expanded := filepath.Join(home, nameOrPath[2:])
			for i := range projects {
				if projects[i].Path == expanded {
					return &projects[i], nil
				}
			}
		}
	}

	// Try name match.
	var matches []scanner.Project
	for _, p := range projects {
		if strings.EqualFold(p.Name, nameOrPath) {
			matches = append(matches, p)
		}
	}

	if len(matches) == 1 {
		return &matches[0], nil
	}

	if len(matches) > 1 {
		var paths []string
		for _, m := range matches {
			paths = append(paths, m.Path)
		}
		return nil, fmt.Errorf("ambiguous project name %q matches %d projects: %s\nSpecify the full path instead", nameOrPath, len(matches), strings.Join(paths, ", "))
	}

	// Try partial name match.
	for i := range projects {
		if strings.Contains(strings.ToLower(projects[i].Name), strings.ToLower(nameOrPath)) {
			return &projects[i], nil
		}
	}

	return nil, fmt.Errorf("project %q not found in scan paths", nameOrPath)
}

// renderFixProposal displays the proposed additions in a styled box format.
func renderFixProposal(fix *fixer.ProposedFix, ctx *fixer.FixContext) {
	fmt.Println(output.Section("CLAUDE.md Fix"))
	fmt.Println()
	fmt.Printf(" %s %s %s\n",
		output.StyleLabel.Render("Project:"),
		output.StyleBold.Render(fix.ProjectName),
		output.StyleMuted.Render(fmt.Sprintf("(score: %d/100)", fix.CurrentScore)))
	fmt.Printf(" %s %s\n",
		output.StyleLabel.Render("Sessions analyzed:"),
		output.StyleValue.Render(fmt.Sprintf("%d", len(ctx.Sessions))))

	claudeMDPath := filepath.Join(fix.ProjectPath, "CLAUDE.md")
	if ctx.ExistingClaudeMD == "" {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("CLAUDE.md:"),
			output.StyleWarning.Render("does not exist (will be created)"))
	} else {
		fmt.Printf(" %s %s\n",
			output.StyleLabel.Render("CLAUDE.md:"),
			output.StyleMuted.Render(claudeMDPath))
	}

	fmt.Printf("\n %s\n\n",
		output.StyleBold.Render("Proposed additions:"))

	for _, a := range fix.Additions {
		renderAdditionBox(a)
	}
}

// renderAdditionBox renders a single addition in a bordered box.
func renderAdditionBox(a fixer.Addition) {
	boxWidth := 63

	// Top border.
	fmt.Printf("  %s%s%s\n", output.StyleMuted.Render("\u250c"), output.StyleMuted.Render(strings.Repeat("\u2500", boxWidth)), output.StyleMuted.Render("\u2510"))

	// Content lines.
	printBoxLine(a.Section, boxWidth)
	printBoxLine("", boxWidth)

	// Wrap and print the content.
	for _, line := range strings.Split(a.Content, "\n") {
		printBoxLine(line, boxWidth)
	}

	printBoxLine("", boxWidth)

	// Reason (wrapped).
	reasonPrefix := "Reason: "
	reasonLines := wrapText(reasonPrefix+a.Reason, boxWidth-2)
	for _, line := range reasonLines {
		printBoxLine(line, boxWidth)
	}

	// Confidence.
	confLine := fmt.Sprintf("Confidence: %.1f", a.Confidence)
	if a.Confidence >= 0.9 {
		confLine += " (high)"
	} else if a.Confidence >= 0.7 {
		confLine += " (moderate)"
	} else {
		confLine += " (low)"
	}
	printBoxLine(confLine, boxWidth)

	// Bottom border.
	fmt.Printf("  %s%s%s\n\n", output.StyleMuted.Render("\u2514"), output.StyleMuted.Render(strings.Repeat("\u2500", boxWidth)), output.StyleMuted.Render("\u2518"))
}

// printBoxLine prints a single line within a box, padded to the box width.
func printBoxLine(text string, width int) {
	// Truncate if needed.
	if len(text) > width-2 {
		text = text[:width-5] + "..."
	}
	padding := width - len(text) - 2
	if padding < 0 {
		padding = 0
	}
	fmt.Printf("  %s %s%s %s\n",
		output.StyleMuted.Render("\u2502"),
		text,
		strings.Repeat(" ", padding),
		output.StyleMuted.Render("\u2502"))
}

// wrapText breaks text into lines of at most maxWidth characters, breaking
// at word boundaries.
func wrapText(text string, maxWidth int) []string {
	if len(text) <= maxWidth {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	current := ""

	for _, word := range words {
		if current == "" {
			current = word
		} else if len(current)+1+len(word) <= maxWidth {
			current += " " + word
		} else {
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}

	return lines
}

// confirmApply prompts the user for confirmation before applying changes.
func confirmApply() bool {
	fmt.Print("  Apply these changes? [y/n] ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

// applyFix writes the proposed additions to the project's CLAUDE.md file.
func applyFix(fix *fixer.ProposedFix, ctx *fixer.FixContext) error {
	claudeMDPath := filepath.Join(fix.ProjectPath, "CLAUDE.md")
	hasExisting := ctx.ExistingClaudeMD != ""

	markdown := fixer.RenderMarkdown(fix, hasExisting)

	if hasExisting {
		// Append to existing file.
		f, err := os.OpenFile(claudeMDPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("opening CLAUDE.md for append: %w", err)
		}
		defer f.Close()

		if _, err := f.WriteString(markdown); err != nil {
			return fmt.Errorf("writing additions: %w", err)
		}
	} else {
		// Create new file.
		if err := os.WriteFile(claudeMDPath, []byte(markdown), 0644); err != nil {
			return fmt.Errorf("creating CLAUDE.md: %w", err)
		}
	}

	fmt.Printf("\n %s Changes written to %s\n",
		output.StyleSuccess.Render("\u2713"),
		claudeMDPath)

	return nil
}
