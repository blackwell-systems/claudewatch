package app

import (
	"testing"
)

func TestHookCmd_Registered(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "hook" {
			return
		}
	}
	t.Fatal("hook subcommand not registered on rootCmd")
}

func TestHookCmd_Use(t *testing.T) {
	if hookCmd.Use != "hook" {
		t.Fatalf("expected hookCmd.Use == \"hook\", got %q", hookCmd.Use)
	}
}

func TestHookCmd_SilenceErrors(t *testing.T) {
	if !hookCmd.SilenceErrors {
		t.Fatal("expected hookCmd.SilenceErrors == true")
	}
}

func TestHookThreshold_Value(t *testing.T) {
	if hookThreshold != 3 {
		t.Fatalf("expected hookThreshold == 3, got %d", hookThreshold)
	}
}
