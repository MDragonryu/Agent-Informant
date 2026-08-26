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

## Initial commands

```bash
agent-informant usage status
agent-informant usage status --provider codex
agent-informant usage status --format json

agent-informant usage advise
agent-informant usage advise --provider claude
agent-informant usage advise --draining 25 --critical 10
agent-informant usage advise --format json
```

`usage status` reports normalized usage windows.

`usage advise` applies a policy and returns one of three states:

- `green`: normal operation is safe.
- `draining`: finish the current coherent unit of work; do not start substantial new work.
- `critical`: stop implementation, checkpoint/handoff, and terminate cleanly.

The initial collector is CodexBar. Agent Informant invokes `codexbar usage --format json`, so it inherits CodexBar's provider support without coupling the rest of the application to CodexBar's schema.

## Design goals

- Agent-first CLI output.
- Stable normalized schema independent of upstream collectors.
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
