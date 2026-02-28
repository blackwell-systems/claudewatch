package app

import (
	"testing"
)

func TestMCPCmd_Registered(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "mcp" {
			return
		}
	}
	t.Fatal("mcp subcommand not registered on rootCmd")
}
