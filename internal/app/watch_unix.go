//go:build !windows

package app

import (
	"fmt"
	"os"
	"syscall"
)

// shutdownSignals are the OS signals that trigger graceful shutdown.
var shutdownSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}

// stopDaemon reads the PID file and sends SIGTERM to the running daemon.
func stopDaemon() error {
	pid, err := readPID()
	if err != nil {
		return fmt.Errorf("no daemon running (could not read PID file: %v)", err)
	}

	if !processExists(pid) {
		// Clean up stale PID file.
		os.Remove(pidFilePath())
		return fmt.Errorf("no daemon running (PID %d is not active, cleaned up stale PID file)", pid)
	}

	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon (PID %d): %w", pid, err)
	}

	// Remove PID file after successful signal.
	os.Remove(pidFilePath())
	fmt.Printf("Stopped daemon (PID %d)\n", pid)
	return nil
}

// processExists checks whether a process with the given PID is running.
func processExists(pid int) bool {
	// Sending signal 0 checks for process existence without actually signaling.
	err := syscall.Kill(pid, 0)
	return err == nil
}
