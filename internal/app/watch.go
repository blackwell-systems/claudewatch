package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/blackwell-systems/claudewatch/internal/config"
	"github.com/blackwell-systems/claudewatch/internal/watcher"
	"github.com/spf13/cobra"
)

var (
	watchDaemon   bool
	watchInterval string
	watchStop     bool
	watchQuiet    bool
	watchBudget   float64
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Monitor session data and alert on friction spikes",
	Long: `Run a background monitor that periodically scans Claude Code session
data for changes. When notable events are detected (friction spikes, new
patterns, session completions), desktop notifications and/or terminal
alerts are emitted.

Examples:
  claudewatch watch                    # run in foreground (ctrl-c to stop)
  claudewatch watch --daemon           # run in background, write PID file
  claudewatch watch --interval 5m      # check every 5 minutes (default: 10m)
  claudewatch watch --budget 20        # alert if daily cost exceeds $20
  claudewatch watch --stop             # stop the background daemon`,
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().BoolVar(&watchDaemon, "daemon", false, "Run in background mode (write PID file, log to file)")
	watchCmd.Flags().StringVar(&watchInterval, "interval", "10m", "Check interval as duration string (e.g. 5m, 1h)")
	watchCmd.Flags().BoolVar(&watchStop, "stop", false, "Stop a running background daemon")
	watchCmd.Flags().BoolVar(&watchQuiet, "quiet", false, "Suppress terminal output, only send notifications")
	watchCmd.Flags().Float64Var(&watchBudget, "budget", 0, "Daily cost budget in USD; alert when exceeded (e.g. --budget 20)")
	rootCmd.AddCommand(watchCmd)
}

// pidFilePath returns the path to the daemon PID file.
func pidFilePath() string {
	return filepath.Join(config.ConfigDir(), "watch.pid")
}

// logFilePath returns the path to the daemon log file.
func logFilePath() string {
	return filepath.Join(config.ConfigDir(), "watch.log")
}

func runWatch(cmd *cobra.Command, args []string) error {
	if watchStop {
		return stopDaemon()
	}

	cfg, err := config.Load(flagConfig)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	interval, err := time.ParseDuration(watchInterval)
	if err != nil {
		return fmt.Errorf("invalid interval %q: %w", watchInterval, err)
	}
	if interval < 30*time.Second {
		return fmt.Errorf("interval must be at least 30s, got %s", interval)
	}

	if watchDaemon {
		return runDaemon(cfg, interval)
	}

	return runForeground(cfg, interval)
}

// runForeground runs the watcher in the foreground with live terminal output.
func runForeground(cfg *config.Config, interval time.Duration) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT/SIGTERM for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, shutdownSignals...)
	go func() {
		<-sigCh
		cancel()
	}()

	if !watchQuiet {
		fmt.Printf("claudewatch watching... (checking every %s)\n", interval)
	}

	alertFn := func(a watcher.Alert) {
		// Send desktop notification.
		_ = watcher.Notify(a)

		// Print to terminal unless quiet mode.
		if !watchQuiet {
			printAlert(a)
		}
	}

	w := watcher.New(cfg.ClaudeHome, interval, alertFn)
	w.BudgetUSD = watchBudget

	// Take initial snapshot and display baseline.
	initial, err := w.Snapshot()
	if err != nil {
		return fmt.Errorf("initial snapshot failed: %w", err)
	}

	if !watchQuiet {
		totalFriction := 0
		for _, count := range initial.FrictionCounts {
			totalFriction += count
		}
		fmt.Printf("[%s] %s No changes (%d sessions, %d friction events)\n",
			time.Now().Format("15:04:05"),
			checkMark(),
			initial.SessionCount,
			totalFriction)
	}

	err = w.Run(ctx)
	if err == context.Canceled {
		if !watchQuiet {
			fmt.Println("\nStopped.")
		}
		return nil
	}
	return err
}

// runDaemon sets up PID and log files, then runs the watcher. The actual
// backgrounding should be done by the caller (nohup, &, etc.) since Go
// cannot reliably fork.
func runDaemon(cfg *config.Config, interval time.Duration) error {
	// Ensure config directory exists.
	configDir := config.ConfigDir()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	// Check for existing daemon.
	if pid, err := readPID(); err == nil {
		if processExists(pid) {
			return fmt.Errorf("daemon already running (PID %d). Use --stop to stop it", pid)
		}
		// Stale PID file, remove it.
		_ = os.Remove(pidFilePath())
	}

	// Write PID file.
	pid := os.Getpid()
	if err := os.WriteFile(pidFilePath(), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}
	defer func() { _ = os.Remove(pidFilePath()) }()

	// Open log file for output.
	logFile, err := os.OpenFile(logFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer func() { _ = logFile.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT/SIGTERM for graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, shutdownSignals...)
	go func() {
		<-sigCh
		cancel()
	}()

	writeLog(logFile, "claudewatch daemon started (PID %d, interval %s)", pid, interval)

	alertFn := func(a watcher.Alert) {
		// Send desktop notification.
		_ = watcher.Notify(a)

		// Log to file.
		writeLog(logFile, "[%s] %s: %s", a.Level, a.Title, a.Message)
	}

	w := watcher.New(cfg.ClaudeHome, interval, alertFn)
	w.BudgetUSD = watchBudget

	err = w.Run(ctx)
	if err == context.Canceled {
		writeLog(logFile, "daemon stopped")
		return nil
	}
	return err
}

// readPID reads the daemon PID from the PID file.
func readPID() (int, error) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

// writeLog writes a timestamped line to the log file.
func writeLog(f *os.File, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	_, _ = fmt.Fprintf(f, "[%s] %s\n", timestamp, msg)
}

// printAlert formats and prints an alert to the terminal.
func printAlert(a watcher.Alert) {
	timestamp := a.Time.Format("15:04:05")
	icon := alertIcon(a.Level)
	fmt.Printf("[%s] %s %s\n", timestamp, icon, a.Title)
	if a.Message != "" {
		fmt.Printf("         %s\n", a.Message)
	}
}

// alertIcon returns the terminal indicator for an alert level.
func alertIcon(level string) string {
	switch level {
	case "critical":
		return "\xf0\x9f\x94\xb4" // red circle
	case "warning":
		return "\xe2\x9a\xa0\xef\xb8\x8f" // warning sign
	case "info":
		return "\xe2\x9c\x93" // check mark
	default:
		return " "
	}
}

// checkMark returns a terminal check mark indicator.
func checkMark() string {
	return "\xe2\x9c\x93"
}
