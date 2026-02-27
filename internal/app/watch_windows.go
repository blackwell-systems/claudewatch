//go:build windows

package app

import (
	"fmt"
	"os"
)

// shutdownSignals are the OS signals that trigger graceful shutdown.
var shutdownSignals = []os.Signal{os.Interrupt}

// stopDaemon reads the PID file and sends a termination signal to the running daemon.
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

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process (PID %d): %w", pid, err)
	}

	// On Windows, Kill terminates the process immediately (no graceful SIGTERM equivalent).
	if err := proc.Kill(); err != nil {
		return fmt.Errorf("failed to stop daemon (PID %d): %w", pid, err)
	}

	// Remove PID file after successful termination.
	os.Remove(pidFilePath())
	fmt.Printf("Stopped daemon (PID %d)\n", pid)
	return nil
}

// processExists checks whether a process with the given PID is running.
func processExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Windows, FindProcess always succeeds, so we need to try sending a signal.
	// Signal(nil) returns an error if the process doesn't exist.
	err = proc.Signal(os.Signal(nil))
	return err == nil
}
