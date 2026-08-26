package usage

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeHookGreenIsSilent(t *testing.T) {
	out, err := RuntimeHookOutput("PreToolUse", Advice{State: StateGreen})
	if err != nil { t.Fatal(err) }
	if len(out) != 0 { t.Fatalf("expected silent green hook, got %s", out) }
}

func TestRuntimeHookInjectsDrainingContext(t *testing.T) {
	advice := Advice{State: StateDraining, Action: "finish-current-work", Message: "checkpoint soon", WorstWindow: &Window{Provider:"codex", Name:"weekly", PercentRemaining:19.5}}
	out, err := RuntimeHookOutput("PreToolUse", advice); if err != nil { t.Fatal(err) }
	var decoded map[string]any; if err := json.Unmarshal(out, &decoded); err != nil { t.Fatal(err) }
	specific := decoded["hookSpecificOutput"].(map[string]any)
	if specific["hookEventName"] != "PreToolUse" { t.Fatalf("wrong hook event: %#v", specific) }
	ctx := specific["additionalContext"].(string)
	if !strings.Contains(ctx, "checkpoint soon") || !strings.Contains(ctx, "19.5% remaining") { t.Fatalf("missing advice context: %s", ctx) }
}

func TestParseRuntimeHookEvent(t *testing.T) {
	event, err := ParseRuntimeHookEvent([]byte(`{"hook_event_name":"UserPromptSubmit"}`)); if err != nil { t.Fatal(err) }
	if event != "UserPromptSubmit" { t.Fatalf("got %q", event) }
}
