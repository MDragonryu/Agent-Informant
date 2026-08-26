package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Runtime string

const (
	Claude Runtime = "claude"
	Codex  Runtime = "codex"
)

var installedEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse"}

func DefaultPath(rt Runtime) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil { return "", err }
	switch rt {
	case Claude:
		return filepath.Join(home, ".claude", "settings.json"), nil
	case Codex:
		base := os.Getenv("CODEX_HOME")
		if base == "" { base = filepath.Join(home, ".codex") }
		return filepath.Join(base, "hooks.json"), nil
	default:
		return "", fmt.Errorf("unsupported runtime %q", rt)
	}
}

func Install(rt Runtime, path, executable string) (string, error) {
	if path == "" {
		var err error
		path, err = DefaultPath(rt)
		if err != nil { return "", err }
	}
	if executable == "" {
		var err error
		executable, err = os.Executable()
		if err != nil { return "", fmt.Errorf("resolve agent-informant executable: %w", err) }
	}
	abs, err := filepath.Abs(executable)
	if err == nil { executable = abs }

	root := map[string]any{}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &root); err != nil { return path, fmt.Errorf("parse %s: %w", path, err) }
	} else if !errors.Is(err, os.ErrNotExist) {
		return path, fmt.Errorf("read %s: %w", path, err)
	}

	hooks, _ := root["hooks"].(map[string]any)
	if hooks == nil { hooks = map[string]any{}; root["hooks"] = hooks }
	command := shellQuote(executable) + " usage hook --runtime " + string(rt)
	for _, event := range installedEvents {
		entries, _ := hooks[event].([]any)
		entries = removeAgentInformant(entries)
		entries = append(entries, map[string]any{
			"hooks": []any{map[string]any{
				"type": "command",
				"command": command,
				"timeout": 15,
			}},
		})
		hooks[event] = entries
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return path, err }
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil { return path, err }
	out = append(out, '\n')
	if err := os.WriteFile(path, out, 0o644); err != nil { return path, err }
	return path, nil
}

func removeAgentInformant(entries []any) []any {
	out := make([]any, 0, len(entries))
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok { out = append(out, entry); continue }
		handlers, _ := m["hooks"].([]any)
		found := false
		for _, handler := range handlers {
			h, _ := handler.(map[string]any)
			cmd, _ := h["command"].(string)
			if strings.Contains(cmd, "agent-informant") && strings.Contains(cmd, "usage hook") { found = true; break }
		}
		if !found { out = append(out, entry) }
	}
	return out
}

func shellQuote(path string) string {
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}
