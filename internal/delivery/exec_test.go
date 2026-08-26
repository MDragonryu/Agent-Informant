package delivery

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExecSendsPayloadAndEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific; Windows behavior is covered by platform CI build")
	}

	dir := t.TempDir()
	payloadFile := filepath.Join(dir, "payload.json")
	envFile := filepath.Join(dir, "env.txt")
	script := filepath.Join(dir, "hook.sh")
	body := "#!/bin/sh\ncat > \"$PAYLOAD_FILE\"\nprintf '%s' \"$AGENT_INFORMANT_STATE\" > \"$ENV_FILE\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	d := Exec{Path: script, Timeout: time.Second}
	err := d.Send(context.Background(), []byte(`{"state":"critical"}`), map[string]string{
		"PAYLOAD_FILE":          payloadFile,
		"ENV_FILE":              envFile,
		"AGENT_INFORMANT_STATE": "critical",
	})
	if err != nil {
		t.Fatal(err)
	}

	payload, err := os.ReadFile(payloadFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"state":"critical"}` {
		t.Fatalf("unexpected payload: %q", payload)
	}
	state, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(state) != "critical" {
		t.Fatalf("unexpected state env: %q", state)
	}
}

func TestExecTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-specific; Windows behavior is covered by platform CI build")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "slow.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := (Exec{Path: script, Timeout: 20 * time.Millisecond}).Send(context.Background(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}
