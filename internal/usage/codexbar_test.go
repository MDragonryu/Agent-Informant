package usage

import "testing"

func TestParseCodexBarJSONCommonShapes(t *testing.T) {
	data := []byte(`{
		"codex": {
			"provider": "codex",
			"primary": {"name":"session","usedPercent":72,"resetAt":"2026-08-27T01:00:00Z"},
			"secondary": {"name":"weekly","percentRemaining":14}
		}
	}`)

	snapshot, err := ParseCodexBarJSON(data, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Windows) != 2 {
		t.Fatalf("expected 2 windows, got %d: %#v", len(snapshot.Windows), snapshot.Windows)
	}
	if snapshot.Windows[0].Provider != "codex" {
		t.Fatalf("expected codex provider, got %q", snapshot.Windows[0].Provider)
	}
	if snapshot.Windows[0].PercentRemaining != 28 {
		t.Fatalf("expected 28 remaining, got %v", snapshot.Windows[0].PercentRemaining)
	}
}

func TestParseCodexBarJSONNormalizesFractions(t *testing.T) {
	data := []byte(`{"claude":{"weekly":{"utilization":0.84}}}`)
	snapshot, err := ParseCodexBarJSON(data, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Windows) != 1 {
		t.Fatalf("expected one window, got %d", len(snapshot.Windows))
	}
	if snapshot.Windows[0].PercentUsed != 84 || snapshot.Windows[0].PercentRemaining != 16 {
		t.Fatalf("unexpected normalized percentages: %#v", snapshot.Windows[0])
	}
}

func TestParseCodexBarJSONRejectsUnrecognizedPayload(t *testing.T) {
	_, err := ParseCodexBarJSON([]byte(`{"hello":"world"}`), "")
	if err == nil {
		t.Fatal("expected error for payload without usage windows")
	}
}
