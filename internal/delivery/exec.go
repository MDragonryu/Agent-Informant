package delivery

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"time"
)

// Exec delivers an event to an external executable.
// The payload is written to stdin and metadata is exposed through environment variables.
// Child stdout is redirected to stderr so machine-readable Agent Informant stdout stays clean.
type Exec struct {
	Path    string
	Args    []string
	Timeout time.Duration
	Stdout  io.Writer
	Stderr  io.Writer
}

func (e Exec) Send(ctx context.Context, payload []byte, env map[string]string) error {
	if e.Path == "" {
		return fmt.Errorf("delivery executable is empty")
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(hookCtx, e.Path, e.Args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(), formatEnv(env)...)
	if e.Stdout != nil {
		cmd.Stdout = e.Stdout
	} else {
		cmd.Stdout = os.Stderr
	}
	if e.Stderr != nil {
		cmd.Stderr = e.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		if hookCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("delivery executable %q timed out after %s", e.Path, timeout)
		}
		return fmt.Errorf("delivery executable %q failed: %w", e.Path, err)
	}
	return nil
}

func formatEnv(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}
