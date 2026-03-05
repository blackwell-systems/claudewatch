package app

import (
	"strings"
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

func TestHookCmd_LongMentionsRepetitiveErrors(t *testing.T) {
	if !strings.Contains(hookCmd.Long, "Repetitive error patterns") {
		t.Fatal("hookCmd.Long should mention 'Repetitive error patterns'")
	}
}

func TestHookCmd_LongMentionsAutoExtract(t *testing.T) {
	if !strings.Contains(hookCmd.Long, "auto-extract") {
		t.Fatal("hookCmd.Long should mention 'auto-extract'")
	}
}

func TestHookCmd_PriorityOrdering(t *testing.T) {
	// Verify the Long description lists priorities in correct order.
	long := hookCmd.Long
	idx1 := strings.Index(long, "Consecutive tool errors")
	idx15 := strings.Index(long, "Repetitive error patterns")
	idx2 := strings.Index(long, "Context pressure")
	idx3 := strings.Index(long, "Cost velocity")
	idx4 := strings.Index(long, "Drift")

	if idx1 < 0 || idx15 < 0 || idx2 < 0 || idx3 < 0 || idx4 < 0 {
		t.Fatal("hookCmd.Long missing one or more priority descriptions")
	}
	if !(idx1 < idx15 && idx15 < idx2 && idx2 < idx3 && idx3 < idx4) {
		t.Fatalf("priority ordering in Long description is wrong: 1=%d, 1.5=%d, 2=%d, 3=%d, 4=%d",
			idx1, idx15, idx2, idx3, idx4)
	}
}
