package usage

import (
	"encoding/json"
	"fmt"
	"strings"
)

type RuntimeHookInput struct {
	HookEventName string `json:"hook_event_name"`
}

type runtimeHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

type runtimeHookOutput struct {
	HookSpecificOutput runtimeHookSpecificOutput `json:"hookSpecificOutput"`
}

func ParseRuntimeHookEvent(data []byte) (string, error) {
	var input RuntimeHookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return "", fmt.Errorf("parse hook input: %w", err)
	}
	if strings.TrimSpace(input.HookEventName) == "" {
		return "", fmt.Errorf("hook input does not contain hook_event_name")
	}
	return input.HookEventName, nil
}

// RuntimeHookOutput returns no bytes for green state. Hook runtimes interpret
// empty stdout plus exit code 0 as a successful no-op.
func RuntimeHookOutput(event string, advice Advice) ([]byte, error) {
	if advice.State == StateGreen {
		return nil, nil
	}
	if event == "" {
		return nil, fmt.Errorf("hook event is empty")
	}
	context := HookContext(advice)
	return json.Marshal(runtimeHookOutput{HookSpecificOutput: runtimeHookSpecificOutput{
		HookEventName: event,
		AdditionalContext: context,
	}})
}

func HookContext(advice Advice) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Agent Informant usage: %s] %s", advice.State, advice.Message)
	if advice.WorstWindow != nil {
		w := advice.WorstWindow
		fmt.Fprintf(&b, " Limiting window: %s/%s, %.1f%% remaining.", w.Provider, w.Name, w.PercentRemaining)
	}
	fmt.Fprintf(&b, " Required action: %s.", advice.Action)
	return b.String()
}
