# Event delivery

`usage watch` can actively deliver emitted events to another local program.

This is intentionally implemented as a generic executable delivery mechanism rather than an agent-framework-specific integration. A wrapper can forward the event into T3 Code, OpenCode, an MCP bridge, a local socket, a notification system, or any other environment without changing Agent Informant's usage model.

## Executable hook

```bash
agent-informant usage watch \
  --provider codex \
  --exec /path/to/agent-hook
```

Each emitted watch event launches the configured executable. Agent Informant:

1. writes one compact JSON event to the executable's stdin;
2. exports `AGENT_INFORMANT_*` environment variables;
3. waits up to 10 seconds by default;
4. reports hook failures to stderr but keeps watching.

Use repeated `--exec-arg` flags for arguments:

```bash
agent-informant usage watch \
  --exec /path/to/agent-hook \
  --exec-arg --session \
  --exec-arg coding-agent-1
```

Override the hook timeout with:

```bash
agent-informant usage watch --exec ./hook --exec-timeout 5
```

When another process is the only consumer, suppress normal watch stdout:

```bash
agent-informant usage watch --exec ./hook --no-output
```

`--no-output` requires `--exec` so accidentally starting a silent watcher is harder.

## JSON stdin

The hook receives the same compact event contract as `usage watch --format jsonl`.

Example transition:

```json
{"type":"state_changed","observed_at":"2026-08-26T20:00:00Z","previous_state":"green","state":"draining","action":"finish-current-work","message":"Finish the current coherent unit of work...","limiting_window":{"provider":"codex","name":"weekly","percent_used":78.4,"percent_remaining":21.6,"source":"codexbar"}}
```

The payload is terminated by a newline.

## Environment variables

Every hook receives:

```text
AGENT_INFORMANT_EVENT
AGENT_INFORMANT_OBSERVED_AT
```

State events additionally provide, when available:

```text
AGENT_INFORMANT_PREVIOUS_STATE
AGENT_INFORMANT_STATE
AGENT_INFORMANT_ACTION
AGENT_INFORMANT_MESSAGE
AGENT_INFORMANT_PROVIDER
AGENT_INFORMANT_WINDOW
AGENT_INFORMANT_PERCENT_REMAINING
AGENT_INFORMANT_PERCENT_USED
AGENT_INFORMANT_RESET_AT
```

Collection error events provide:

```text
AGENT_INFORMANT_ERROR
```

The JSON payload is the canonical event representation. Environment variables are convenience fields for scripts that do not need to parse JSON.

## PowerShell example

```powershell
# agent-hook.ps1
$eventJson = [Console]::In.ReadToEnd()
$event = $eventJson | ConvertFrom-Json

if ($env:AGENT_INFORMANT_STATE -eq 'critical') {
    # Replace this with the integration that can message or interrupt your agent.
    $event.message | Set-Content -Path "$env:TEMP\agent-informant-critical.txt"
}
```

Run it through PowerShell explicitly:

```powershell
agent-informant usage watch `
  --provider codex `
  --exec pwsh `
  --exec-arg -NoProfile `
  --exec-arg -File `
  --exec-arg C:\Tools\agent-hook.ps1 `
  --no-output
```

## Shell example

```sh
#!/bin/sh
payload=$(cat)

if [ "$AGENT_INFORMANT_STATE" = "critical" ]; then
    printf '%s\n' "$payload" >> "$HOME/.local/state/agent-informant/critical.jsonl"
fi
```

## Delivery behavior

Hook failures are deliberately non-fatal. A broken notification integration should not stop quota monitoring and hide later state changes. Agent Informant writes delivery errors to stderr and continues polling.

The hook is invoked for the initial event as well as later state transitions. This matters when a watcher starts while the provider is already draining or critical: the downstream agent integration receives that state immediately instead of waiting for another threshold crossing.

Collection-error events are delivered too, allowing an integration to distinguish "usage is healthy" from "usage could not be checked".

## Future transports

Executable delivery is the first transport. The intended architecture allows other transports to consume the same compact event contract later, for example:

- local HTTP/webhook delivery;
- MCP notifications or tools;
- named pipes / Unix domain sockets;
- agent-framework-specific adapters.

Those transports should remain separate from usage collection and policy evaluation.
