package main

import (
	"testing"
	"time"

	"github.com/MDragonryu/Agent-Informant/internal/usage"
)

func TestWatchEventEnv(t *testing.T) {
	previous := usage.StateGreen
	reset := time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)
	event := usage.WatchEvent{
		Type:          usage.WatchStateChange,
		ObservedAt:    time.Date(2026, 8, 26, 20, 0, 0, 0, time.UTC),
		PreviousState: &previous,
		Advice: &usage.Advice{
			State:   usage.StateCritical,
			Action:  "checkpoint-and-stop",
			Message: "checkpoint now",
			WorstWindow: &usage.Window{
				Provider:         "codex",
				Name:             "weekly",
				PercentRemaining: 8.5,
				PercentUsed:      91.5,
				ResetAt:          &reset,
			},
		},
	}

	env := watchEventEnv(event)
	checks := map[string]string{
		"AGENT_INFORMANT_EVENT":             "state_changed",
		"AGENT_INFORMANT_PREVIOUS_STATE":    "green",
		"AGENT_INFORMANT_STATE":             "critical",
		"AGENT_INFORMANT_ACTION":            "checkpoint-and-stop",
		"AGENT_INFORMANT_MESSAGE":           "checkpoint now",
		"AGENT_INFORMANT_PROVIDER":          "codex",
		"AGENT_INFORMANT_WINDOW":            "weekly",
		"AGENT_INFORMANT_PERCENT_REMAINING": "8.5",
		"AGENT_INFORMANT_PERCENT_USED":      "91.5",
		"AGENT_INFORMANT_RESET_AT":          "2026-08-27T01:02:03Z",
	}
	for key, want := range checks {
		if got := env[key]; got != want {
			t.Fatalf("%s: expected %q, got %q", key, want, got)
		}
	}
}

func TestWatchErrorEnv(t *testing.T) {
	event := usage.WatchEvent{Type: usage.WatchError, ObservedAt: time.Now().UTC(), Error: "collector unavailable"}
	env := watchEventEnv(event)
	if env["AGENT_INFORMANT_ERROR"] != "collector unavailable" {
		t.Fatalf("unexpected error env: %#v", env)
	}
	if _, ok := env["AGENT_INFORMANT_STATE"]; ok {
		t.Fatalf("error event must not fabricate a state: %#v", env)
	}
}
