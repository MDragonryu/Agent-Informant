package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallMergesExistingClaudeSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"existing-hook"}]}]}}`), 0o644); err != nil { t.Fatal(err) }
	if _, err := Install(Claude, path, "/tmp/agent-informant"); err != nil { t.Fatal(err) }
	data, err := os.ReadFile(path); if err != nil { t.Fatal(err) }
	var root map[string]any; if err := json.Unmarshal(data, &root); err != nil { t.Fatal(err) }
	if root["theme"] != "dark" { t.Fatalf("existing setting lost: %#v", root) }
	hooks := root["hooks"].(map[string]any)
	pre := hooks["PreToolUse"].([]any)
	if len(pre) != 2 { t.Fatalf("expected existing hook plus Agent Informant hook, got %d", len(pre)) }
	for _, event := range installedEvents { if _, ok := hooks[event]; !ok { t.Fatalf("missing %s hook", event) } }
}

func TestInstallIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	if _, err := Install(Codex, path, "/tmp/agent-informant"); err != nil { t.Fatal(err) }
	if _, err := Install(Codex, path, "/tmp/agent-informant"); err != nil { t.Fatal(err) }
	data, _ := os.ReadFile(path); var root map[string]any; _ = json.Unmarshal(data, &root)
	hooks := root["hooks"].(map[string]any)
	for _, event := range installedEvents {
		entries := hooks[event].([]any)
		if len(entries) != 1 { t.Fatalf("%s expected one Agent Informant registration, got %d", event, len(entries)) }
	}
}
