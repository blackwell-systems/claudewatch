package ui

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsTTY returns true if both stdin and stdout are connected to a terminal.
//
// Checking both prevents unwanted interactive behavior:
//   - stdin check prevents prompts in piped input: `echo "data" | claudewatch attribute`
//   - stdout check prevents menu in piped output: `claudewatch attribute | jq`
func IsTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}
