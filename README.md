# Agent Informant

Agent Informant is a small, agent-facing CLI that turns operational information from existing tools into stable, easy-to-consume signals.

The project is intentionally broader than quota monitoring. Its command structure is:

```text
agent-informant <domain> <command>
```

The first domain is `usage`.

## Why

Agentic workflows often fail badly at usage-limit boundaries: an agent starts a substantial task, quota runs out mid-turn, and the next agent inherits a half-finished state without context.

Agent Informant does not try to replace provider-specific collectors. Instead, it consumes existing collectors such as CodexBar, normalizes their output, and gives agents a small stable interface for deciding whether they should continue, drain current work, or checkpoint and stop.

## Commands

```bash
agent-informant usage status
agent-informant usage status --provider codex
agent-informant usage status --format json

agent-informant usage advise
agent-informant usage advise --provider claude
agent-informant usage advise --format json

agent-informant usage watch
agent-informant usage watch --provider codex
agent-informant usage watch --format jsonl
agent-informant usage watch --interval 30
agent-informant usage watch --exec /path/to/agent-hook --no-output

agent-informant config path
agent-informant config init
agent-informant config show
```

`usage status` reports normalized usage windows.

`usage advise` performs one policy evaluation and returns one of three states:

- `green`: normal operation is safe.
- `draining`: finish the current coherent unit of work; do not start substantial new work.
- `critical`: stop implementation, checkpoint/handoff, and terminate cleanly.

`usage watch` keeps polling the collector. To stay token-efficient it emits one initial result and then only state changes or collection errors. Text mode is compact; `--format jsonl` is intended for orchestrators and agent runners.

Example transition:

```text
state_changed:green->draining draining 19.4% codex/weekly finish-current-work | Finish the current coherent unit of work, avoid substantial new work or delegation, then checkpoint before continuing later.
```

`usage watch --exec PATH` actively delivers each emitted event to another local program. The hook receives compact JSON on stdin and `AGENT_INFORMANT_*` environment variables. Use `--no-output` when the executable is the only consumer.

For example:

```powershell
agent-informant usage watch `
  --provider codex `
  --exec pwsh `
  --exec-arg -NoProfile `
  --exec-arg -File `
  --exec-arg C:\Tools\agent-hook.ps1 `
  --no-output
```

Hook failures are reported but do not stop monitoring. See [`docs/delivery.md`](docs/delivery.md) for the delivery contract and integration examples.

The initial collector is CodexBar. Agent Informant invokes `codexbar usage --format json`, so it inherits CodexBar's provider support without coupling the rest of the application to CodexBar's schema.

## Configuration

Run:

```bash
agent-informant config init
```

This creates a platform-native user configuration file. `agent-informant config path` prints its location.

The default configuration is equivalent to:

```json
{
  "usage": {
    "draining_remaining": 25,
    "critical_remaining": 10,
    "messages": {
      "green": "Usage headroom is sufficient for normal operation.",
      "draining": "Finish the current coherent unit of work, avoid substantial new work or delegation, then checkpoint before continuing later.",
      "critical": "Stop implementation at the nearest safe point. Make the current state coherent, record a handoff/checkpoint, and do not start new work."
    },
    "watch_interval_seconds": 60
  }
}
```

The messages are deliberately user-configurable. Agent Informant owns the state classification, but the user decides what instruction an agent receives when that state is reached.

For temporary overrides, `advise` and `watch` also accept:

```bash
--draining 30
--critical 15
--message-green "..."
--message-draining "..."
--message-critical "..."
--config /path/to/config.json
```

This allows an orchestrator to provide task-specific instructions without modifying persistent configuration.

## Design goals

- Agent-first CLI output.
- Stable normalized schema independent of upstream collectors.
- Sparse watch output to minimize agent-context/token cost.
- Active event delivery without coupling to one agent framework.
- Small dependency footprint.
- Cross-platform operation.
- Collector interfaces so future sources can be added without changing commands.
- Domain-oriented CLI so future capabilities can live alongside `usage`.
- Conservative behavior when usage information is missing.

See [`docs/architecture.md`](docs/architecture.md) for the initial architecture.

## Build

Requires Go 1.24 or newer.

```bash
go build ./cmd/agent-informant
```

CodexBar must currently be installed and available on `PATH`. Override the executable with:

```bash
AGENT_INFORMANT_CODEXBAR=/path/to/codexbar agent-informant usage advise
```

## Status

Early implementation. The CLI and normalized schema should be treated as pre-1.0 while additional collectors and real-world provider payloads are exercised.
