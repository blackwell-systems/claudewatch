package watcher

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// Notify sends a desktop notification for the given alert. On macOS it uses
// osascript, on Linux it tries notify-send. If neither is available, it falls
// back to printing to stderr.
func Notify(alert Alert) error {
	switch runtime.GOOS {
	case "darwin":
		return notifyMacOS(alert)
	case "linux":
		return notifyLinux(alert)
	default:
		return notifyFallback(alert)
	}
}

// notifyMacOS sends a notification via osascript on macOS.
func notifyMacOS(alert Alert) error {
	script := fmt.Sprintf(
		`display notification %q with title "claudewatch" subtitle %q`,
		alert.Message, alert.Title,
	)
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		// Fall back to stderr if osascript fails.
		return notifyFallback(alert)
	}
	return nil
}

// notifyLinux sends a notification via notify-send on Linux.
func notifyLinux(alert Alert) error {
	_, err := exec.LookPath("notify-send")
	if err != nil {
		return notifyFallback(alert)
	}

	title := fmt.Sprintf("claudewatch: %s", alert.Title)
	cmd := exec.Command("notify-send", title, alert.Message)
	if err := cmd.Run(); err != nil {
		return notifyFallback(alert)
	}
	return nil
}

// notifyFallback prints the alert to stderr when no desktop notification
// system is available.
func notifyFallback(alert Alert) error {
	_, err := fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", alert.Level, alert.Title, alert.Message)
	return err
}
