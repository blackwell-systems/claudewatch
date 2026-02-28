package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/blackwell-systems/claudewatch/internal/claude"
	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/output"
	"github.com/blackwell-systems/claudewatch/internal/scanner"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check whether the claudewatch setup is healthy",
	Long: `Run a series of health checks against your claudewatch configuration
and Claude Code data directory. Prints a pass/fail line for each check
and a summary of how many checks passed.`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

// doctorCheck holds the result of a single health check.
type doctorCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// doctorOutput is the JSON-serializable result of the doctor command.
type doctorOutput struct {
	Checks      []doctorCheck `json:"checks"`
	PassedCount int           `json:"passed"`
	TotalCount  int           `json:"total"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	if flagNoColor {
		output.SetNoColor(true)
	}

	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var checks []doctorCheck

	// 1. Claude home directory — exists and is readable.
	checks = append(checks, checkClaudeHome(cfg.ClaudeHome))

	// 2. Session data — at least 1 session-meta file exists.
	checks = append(checks, checkSessionData(cfg.ClaudeHome))

	// 3. Stats cache — stats-cache.json exists and parses.
	checks = append(checks, checkStatsCache(cfg.ClaudeHome))

	// 4. Scan paths — each configured scan path exists.
	checks = append(checks, checkScanPaths(cfg.ScanPaths)...)

	// 5. SQLite database — config.DBPath() exists.
	checks = append(checks, checkDatabase())

	// 6. Watch daemon — PID file exists and process is running.
	checks = append(checks, checkWatchDaemon())

	// 7. CLAUDE.md coverage — count projects with vs without CLAUDE.md.
	checks = append(checks, checkClaudeMDCoverage(cfg.ScanPaths))

	// 8. API key — ANTHROPIC_API_KEY env var is set.
	checks = append(checks, checkAPIKey())

	// Count passes.
	passed := 0
	for _, c := range checks {
		if c.Passed {
			passed++
		}
	}

	if flagJSON {
		out := doctorOutput{
			Checks:      checks,
			PassedCount: passed,
			TotalCount:  len(checks),
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	// Render styled output.
	fmt.Println(output.Section("Doctor"))
	fmt.Println()

	for _, c := range checks {
		renderDoctorCheck(c)
	}

	fmt.Println()
	summary := fmt.Sprintf("%d/%d checks passed", passed, len(checks))
	if passed == len(checks) {
		fmt.Printf(" %s\n\n", output.StyleSuccess.Render(summary))
	} else {
		fmt.Printf(" %s\n\n", output.StyleWarning.Render(summary))
	}

	return nil
}

// renderDoctorCheck prints a single check result line.
func renderDoctorCheck(c doctorCheck) {
	var indicator string
	if c.Passed {
		indicator = output.StyleSuccess.Render("✓")
	} else {
		indicator = output.StyleWarning.Render("✗")
	}
	label := output.StyleBold.Render(c.Name)
	detail := output.StyleMuted.Render(c.Message)
	fmt.Printf("  %s  %-30s %s\n", indicator, label, detail)
}

// checkClaudeHome verifies that cfg.ClaudeHome exists and is a readable directory.
func checkClaudeHome(claudeHome string) doctorCheck {
	info, err := os.Stat(claudeHome)
	if err != nil {
		return doctorCheck{
			Name:    "Claude home directory",
			Passed:  false,
			Message: fmt.Sprintf("not found: %s", claudeHome),
		}
	}
	if !info.IsDir() {
		return doctorCheck{
			Name:    "Claude home directory",
			Passed:  false,
			Message: fmt.Sprintf("path exists but is not a directory: %s", claudeHome),
		}
	}
	return doctorCheck{
		Name:    "Claude home directory",
		Passed:  true,
		Message: claudeHome,
	}
}

// checkSessionData verifies that at least one session-meta file exists.
func checkSessionData(claudeHome string) doctorCheck {
	sessions, err := claude.ParseAllSessionMeta(claudeHome)
	if err != nil {
		return doctorCheck{
			Name:    "Session data",
			Passed:  false,
			Message: fmt.Sprintf("error reading sessions: %v", err),
		}
	}
	if len(sessions) == 0 {
		return doctorCheck{
			Name:    "Session data",
			Passed:  false,
			Message: "no session-meta files found",
		}
	}
	return doctorCheck{
		Name:    "Session data",
		Passed:  true,
		Message: fmt.Sprintf("%d sessions found", len(sessions)),
	}
}

// checkStatsCache verifies that stats-cache.json exists and parses successfully.
func checkStatsCache(claudeHome string) doctorCheck {
	sc, err := claude.ParseStatsCache(claudeHome)
	if err != nil {
		return doctorCheck{
			Name:    "Stats cache",
			Passed:  false,
			Message: fmt.Sprintf("parse error: %v", err),
		}
	}
	if sc == nil {
		return doctorCheck{
			Name:    "Stats cache",
			Passed:  false,
			Message: "stats-cache.json not found",
		}
	}
	return doctorCheck{
		Name:    "Stats cache",
		Passed:  true,
		Message: "stats-cache.json parsed successfully",
	}
}

// checkScanPaths verifies that each configured scan path exists.
func checkScanPaths(scanPaths []string) []doctorCheck {
	if len(scanPaths) == 0 {
		return []doctorCheck{{
			Name:    "Scan paths",
			Passed:  false,
			Message: "no scan paths configured",
		}}
	}

	var checks []doctorCheck
	for _, p := range scanPaths {
		_, err := os.Stat(p)
		if err != nil {
			checks = append(checks, doctorCheck{
				Name:    fmt.Sprintf("Scan path: %s", filepath.Base(p)),
				Passed:  false,
				Message: fmt.Sprintf("not found: %s", p),
			})
		} else {
			checks = append(checks, doctorCheck{
				Name:    fmt.Sprintf("Scan path: %s", filepath.Base(p)),
				Passed:  true,
				Message: p,
			})
		}
	}
	return checks
}

// checkDatabase verifies that the SQLite database file exists.
func checkDatabase() doctorCheck {
	dbPath := config.DBPath()
	_, err := os.Stat(dbPath)
	if err != nil {
		return doctorCheck{
			Name:    "SQLite database",
			Passed:  false,
			Message: fmt.Sprintf("not found at %s (run 'claudewatch track' to create)", dbPath),
		}
	}
	return doctorCheck{
		Name:    "SQLite database",
		Passed:  true,
		Message: dbPath,
	}
}

// checkWatchDaemon checks whether the watch daemon PID file exists and the process is running.
func checkWatchDaemon() doctorCheck {
	pidPath := filepath.Join(config.ConfigDir(), "watch.pid")

	data, err := os.ReadFile(pidPath)
	if err != nil {
		return doctorCheck{
			Name:    "Watch daemon",
			Passed:  false,
			Message: "not running (no PID file)",
		}
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return doctorCheck{
			Name:    "Watch daemon",
			Passed:  false,
			Message: fmt.Sprintf("invalid PID in file: %q", pidStr),
		}
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return doctorCheck{
			Name:    "Watch daemon",
			Passed:  false,
			Message: fmt.Sprintf("PID %d not found", pid),
		}
	}

	// Signal 0 checks process existence without sending an actual signal.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return doctorCheck{
			Name:    "Watch daemon",
			Passed:  false,
			Message: fmt.Sprintf("PID %d is not running (stale PID file)", pid),
		}
	}

	return doctorCheck{
		Name:    "Watch daemon",
		Passed:  true,
		Message: fmt.Sprintf("running (PID %d)", pid),
	}
}

// checkClaudeMDCoverage counts projects with and without CLAUDE.md.
func checkClaudeMDCoverage(scanPaths []string) doctorCheck {
	if len(scanPaths) == 0 {
		return doctorCheck{
			Name:    "CLAUDE.md coverage",
			Passed:  false,
			Message: "no scan paths configured",
		}
	}

	projects, err := scanner.DiscoverProjects(scanPaths)
	if err != nil {
		return doctorCheck{
			Name:    "CLAUDE.md coverage",
			Passed:  false,
			Message: fmt.Sprintf("error discovering projects: %v", err),
		}
	}

	if len(projects) == 0 {
		return doctorCheck{
			Name:    "CLAUDE.md coverage",
			Passed:  false,
			Message: "no projects found in scan paths",
		}
	}

	withClaude := 0
	withoutClaude := 0
	for _, p := range projects {
		if p.HasClaudeMD {
			withClaude++
		} else {
			withoutClaude++
		}
	}

	pct := float64(withClaude) / float64(len(projects)) * 100
	passed := withoutClaude == 0 || pct >= 50

	return doctorCheck{
		Name:   "CLAUDE.md coverage",
		Passed: passed,
		Message: fmt.Sprintf("%d/%d projects have CLAUDE.md (%.0f%%)",
			withClaude, len(projects), pct),
	}
}

// checkAPIKey verifies that ANTHROPIC_API_KEY is set.
func checkAPIKey() doctorCheck {
	val := os.Getenv("ANTHROPIC_API_KEY")
	if val == "" {
		return doctorCheck{
			Name:    "API key",
			Passed:  false,
			Message: "ANTHROPIC_API_KEY is not set (needed for 'fix --ai')",
		}
	}
	// Show only the first few characters for security.
	masked := val[:min(8, len(val))] + "..."
	return doctorCheck{
		Name:    "API key",
		Passed:  true,
		Message: fmt.Sprintf("ANTHROPIC_API_KEY set (%s)", masked),
	}
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
