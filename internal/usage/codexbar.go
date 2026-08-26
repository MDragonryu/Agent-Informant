package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type CodexBarCollector struct {
	Executable string
	History    *HistoryStore
}

func NewCodexBarCollector() *CodexBarCollector {
	exe := os.Getenv("AGENT_INFORMANT_CODEXBAR")
	if exe == "" {
		exe = "codexbar"
	}
	store, _ := NewHistoryStore()
	return &CodexBarCollector{Executable: exe, History: store}
}

func (c *CodexBarCollector) Name() string { return "codexbar" }

func (c *CodexBarCollector) Collect(ctx context.Context, provider string) (Snapshot, error) {
	args := []string{"usage", "--format", "json"}
	if provider != "" {
		args = append(args, "--provider", provider)
	}
	cmd := exec.CommandContext(ctx, c.Executable, args...)
	out, err := cmd.Output()
	if err != nil {
		return Snapshot{}, fmt.Errorf("run %s: %w", c.Executable, err)
	}
	snapshot, err := ParseCodexBarJSON(out, provider)
	if err != nil {
		return Snapshot{}, err
	}
	if c.History != nil {
		_ = c.History.Append(snapshot)
		if history, loadErr := c.History.Load(snapshot.CollectedAt); loadErr == nil {
			snapshot.History = history
		}
	}
	return snapshot, nil
}

func ParseCodexBarJSON(data []byte, providerFilter string) (Snapshot, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return Snapshot{}, fmt.Errorf("parse codexbar json: %w", err)
	}

	now := time.Now().UTC()
	windows := make([]Window, 0)
	walkCodexBar(root, nil, providerFilter, &windows)
	windows = dedupeWindows(windows)
	if len(windows) == 0 {
		return Snapshot{}, fmt.Errorf("codexbar returned no recognizable usage windows")
	}
	return Snapshot{CollectedAt: now, Windows: windows}, nil
}

func walkCodexBar(v any, path []string, providerFilter string, out *[]Window) {
	switch x := v.(type) {
	case map[string]any:
		provider := firstString(x, "provider", "providerId", "providerID", "id")
		if provider == "" {
			provider = inferProvider(path)
		}
		if providerFilter != "" && provider != "" && !strings.EqualFold(provider, providerFilter) {
			for k, child := range x {
				walkCodexBar(child, append(path, k), providerFilter, out)
			}
			return
		}

		used, hasUsed := firstNumber(x, "percentUsed", "usedPercent", "usagePercent", "utilization", "used")
		remaining, hasRemaining := firstNumber(x, "percentRemaining", "remainingPercent", "remaining")
		if hasUsed || hasRemaining {
			if !hasUsed {
				used = 100 - normalizePercent(remaining)
			} else {
				used = normalizePercent(used)
			}
			if !hasRemaining {
				remaining = 100 - used
			} else {
				remaining = normalizePercent(remaining)
			}
			name := firstString(x, "window", "name", "title", "label", "period")
			if name == "" {
				name = inferWindow(path)
			}
			reset := parseTime(firstAny(x, "resetAt", "resetsAt", "reset", "resetTime"))
			if provider == "" {
				provider = "unknown"
			}
			if providerFilter == "" || strings.EqualFold(provider, providerFilter) || provider == "unknown" {
				*out = append(*out, Window{Provider: provider, Name: name, PercentUsed: clamp(used), PercentRemaining: clamp(remaining), ResetAt: reset, Source: "codexbar"})
			}
		}
		for k, child := range x {
			walkCodexBar(child, append(path, k), providerFilter, out)
		}
	case []any:
		for i, child := range x {
			walkCodexBar(child, append(path, strconv.Itoa(i)), providerFilter, out)
		}
	}
}

func firstAny(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func firstNumber(m map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch n := v.(type) {
			case float64:
				return n, true
			case string:
				clean := strings.TrimSpace(strings.TrimSuffix(n, "%"))
				if f, err := strconv.ParseFloat(clean, 64); err == nil {
					return f, true
				}
			}
		}
	}
	return 0, false
}

func normalizePercent(v float64) float64 {
	if v >= 0 && v <= 1 {
		return v * 100
	}
	return v
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func parseTime(v any) *time.Time {
	switch t := v.(type) {
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			if parsed, err := time.Parse(layout, t); err == nil {
				parsed = parsed.UTC()
				return &parsed
			}
		}
	case float64:
		sec := int64(t)
		if sec > 1_000_000_000_000 {
			sec /= 1000
		}
		parsed := time.Unix(sec, 0).UTC()
		return &parsed
	}
	return nil
}

func inferProvider(path []string) string {
	known := map[string]bool{"codex": true, "claude": true, "cursor": true, "gemini": true, "copilot": true, "opencode": true}
	for i := len(path) - 1; i >= 0; i-- {
		p := strings.ToLower(path[i])
		if known[p] {
			return p
		}
	}
	return ""
}

func inferWindow(path []string) string {
	for i := len(path) - 1; i >= 0; i-- {
		p := strings.ToLower(path[i])
		if strings.Contains(p, "week") || strings.Contains(p, "secondary") {
			return "weekly"
		}
		if strings.Contains(p, "session") || strings.Contains(p, "primary") || strings.Contains(p, "5h") {
			return "session"
		}
	}
	return "usage"
}

func dedupeWindows(in []Window) []Window {
	seen := map[string]bool{}
	out := make([]Window, 0, len(in))
	for _, w := range in {
		key := strings.ToLower(w.Provider) + "\x00" + strings.ToLower(w.Name) + "\x00" + fmt.Sprintf("%.4f", w.PercentUsed)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, w)
	}
	return out
}
