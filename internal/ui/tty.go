package ui

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsTTY returns true if both stdin and stdout are connected to a terminal.
// Uses mattn/go-isatty for platform-independent detection.
//
// NOTE: This is a minimal implementation to satisfy Agent B's build requirements.
// Agent C is the owner of this file and will provide the canonical implementation.
// This version is compatible with Agent C's interface contract.
func IsTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}
