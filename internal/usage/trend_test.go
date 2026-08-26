package usage

import (
	"testing"
	"time"
)

func TestAnalyzeDrainRate(t *testing.T) {
	now := time.Date(2026, 8, 26, 20, 0, 0, 0, time.UTC)
	reset := now.Add(2 * time.Hour)
	history := []Snapshot{
		{CollectedAt: now.Add(-10 * time.Minute), Windows: []Window{{Provider: "codex", Name: "session", PercentRemaining: 70, ResetAt: &reset}}},
		{CollectedAt: now, Windows: []Window{{Provider: "codex", Name: "session", PercentRemaining: 50, ResetAt: &reset}}},
	}
	rate := AnalyzeDrainRate(history, history[1].Windows[0], 10, now)
	if rate == nil { t.Fatal("expected drain rate") }
	if rate.PercentPerHour < 119 || rate.PercentPerHour > 121 { t.Fatalf("expected about 120%%/hour, got %.2f", rate.PercentPerHour) }
	if rate.EstimatedMinutesToExhaust == nil || *rate.EstimatedMinutesToExhaust < 24 || *rate.EstimatedMinutesToExhaust > 26 {
		t.Fatalf("unexpected exhaustion ETA: %#v", rate.EstimatedMinutesToExhaust)
	}
	if rate.Level != "extreme" { t.Fatalf("expected extreme, got %s", rate.Level) }
}

func TestAnalyzeDrainRateIgnoresDifferentResetCycle(t *testing.T) {
	now := time.Date(2026, 8, 26, 20, 0, 0, 0, time.UTC)
	oldReset := now.Add(-time.Minute)
	newReset := now.Add(2 * time.Hour)
	history := []Snapshot{
		{CollectedAt: now.Add(-10 * time.Minute), Windows: []Window{{Provider: "codex", Name: "session", PercentRemaining: 5, ResetAt: &oldReset}}},
		{CollectedAt: now, Windows: []Window{{Provider: "codex", Name: "session", PercentRemaining: 95, ResetAt: &newReset}}},
	}
	if rate := AnalyzeDrainRate(history, history[1].Windows[0], 10, now); rate != nil {
		t.Fatalf("expected reset boundary to suppress rate, got %#v", rate)
	}
}

func TestPolicyEscalatesFromBurnRate(t *testing.T) {
	now := time.Date(2026, 8, 26, 20, 0, 0, 0, time.UTC)
	reset := now.Add(2 * time.Hour)
	current := Window{Provider: "codex", Name: "session", PercentRemaining: 40, ResetAt: &reset}
	snapshot := Snapshot{
		CollectedAt: now,
		Windows: []Window{current},
		History: []Snapshot{
			{CollectedAt: now.Add(-5 * time.Minute), Windows: []Window{{Provider: "codex", Name: "session", PercentRemaining: 90, ResetAt: &reset}}},
			{CollectedAt: now, Windows: []Window{current}},
		},
	}
	advice, err := DefaultPolicy().EvaluateWithHistory(snapshot, snapshot.History, now)
	if err != nil { t.Fatal(err) }
	if advice.State != StateCritical {
		t.Fatalf("expected critical from projected exhaustion, got %s", advice.State)
	}
	if advice.DrainRate == nil { t.Fatal("expected drain-rate context") }
}
