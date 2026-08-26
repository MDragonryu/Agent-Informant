package usage

import (
	"testing"
	"time"
)

func TestPolicyUsesMostRestrictiveWindow(t *testing.T) {
	snapshot := Snapshot{
		CollectedAt: time.Now(),
		Windows: []Window{
			{Provider: "codex", Name: "session", PercentRemaining: 80},
			{Provider: "codex", Name: "weekly", PercentRemaining: 18},
		},
	}

	advice, err := DefaultPolicy().Evaluate(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if advice.State != StateDraining {
		t.Fatalf("expected draining, got %s", advice.State)
	}
	if advice.WorstWindow == nil || advice.WorstWindow.Name != "weekly" {
		t.Fatalf("expected weekly limiting window, got %#v", advice.WorstWindow)
	}
	if ExitCode(advice.State) != 10 {
		t.Fatalf("expected exit code 10, got %d", ExitCode(advice.State))
	}
}

func TestPolicyCritical(t *testing.T) {
	snapshot := Snapshot{Windows: []Window{{Provider: "claude", Name: "weekly", PercentRemaining: 9}}}
	advice, err := DefaultPolicy().Evaluate(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if advice.State != StateCritical || advice.Action != "checkpoint-and-stop" {
		t.Fatalf("unexpected advice: %#v", advice)
	}
	if ExitCode(advice.State) != 20 {
		t.Fatalf("expected exit code 20, got %d", ExitCode(advice.State))
	}
}

func TestPolicyUsesConfiguredMessage(t *testing.T) {
	policy := DefaultPolicy()
	policy.Messages.Draining = "wrap up and write the handoff now"
	advice, err := policy.Evaluate(Snapshot{Windows: []Window{{Provider: "codex", Name: "session", PercentRemaining: 20}}})
	if err != nil {
		t.Fatal(err)
	}
	if advice.Message != "wrap up and write the handoff now" {
		t.Fatalf("expected configured message, got %q", advice.Message)
	}
}

func TestPolicyRejectsInvalidThresholds(t *testing.T) {
	_, err := (Policy{DrainingRemaining: 5, CriticalRemaining: 10}).Evaluate(Snapshot{Windows: []Window{{PercentRemaining: 50}}})
	if err == nil {
		t.Fatal("expected invalid threshold error")
	}
}
