# Architecture

Agent Informant is organized around domains, collectors, normalized models, policies, sparse events, and delivery transports.

## Command model

```text
agent-informant <domain> <command> [flags]
```

`usage` is the first domain. Future domains can expose other information agents benefit from without forcing those concepts into the usage model.

## Usage pipeline

```text
upstream collector
      |
      v
collector adapter
      |
      v
normalized UsageSnapshot
      |
      +----> status renderer
      |
      +----> policy evaluator ----> advice renderer / exit code
                     |
                     +------------> watch state engine ----> sparse events
                                                          |
                                                          +----> stdout/jsonl
                                                          |
                                                          +----> delivery transport
```

The CLI must never expose an upstream collector's raw schema as its contract. Upstream formats can change; Agent Informant's normalized structures are the compatibility boundary.

Likewise, delivery is downstream of the domain event model. A notification mechanism does not get to redefine usage states or collector behavior.

## Initial collector

The first collector is CodexBar. It is deliberately treated as an executable dependency rather than as the architecture itself.

The adapter invokes:

```text
codexbar usage --format json
```

and discovers provider/window data from the returned JSON. The parser is intentionally tolerant of nested objects and common key variants so small upstream payload changes are less likely to break the CLI.

Additional collectors implement the same `Collector` interface.

Potential future sources include direct provider APIs, ccusage-compatible sources, local telemetry, or a daemon/API endpoint.

## Normalized model

A normalized usage window contains:

- provider
- name
- percent used
- percent remaining
- reset timestamp, when known
- source

Percentages are represented as values from 0 to 100.

## Policy

The policy maps remaining capacity to three states:

```text
green      remaining > draining threshold
draining   critical threshold < remaining <= draining threshold
critical   remaining <= critical threshold
```

Defaults:

```text
draining: 25%
critical: 10%
```

For multiple windows, the most restrictive state wins. A provider with a healthy short window but nearly exhausted long window must not be reported as healthy.

Each state has two outputs:

1. A stable machine-oriented action (`continue`, `finish-current-work`, or `checkpoint-and-stop`).
2. A user-configurable message intended to be inserted directly into an agent context.

The default messages are useful out of the box, but they are policy presentation, not hard-coded agent behavior. Users can replace all three messages in the config file or temporarily override them with CLI flags.

## Configuration

Configuration is loaded from the platform user config directory via Go's `os.UserConfigDir`, under `agent-informant/config.json`. A different file can be supplied with `--config`.

The configuration currently owns:

- draining threshold
- critical threshold
- green/draining/critical messages
- watch polling interval

Defaults are applied before the file is decoded, so partial configuration files remain valid.

## Watch model

`usage watch` repeatedly collects a normalized snapshot and applies exactly the same policy as `usage advise`.

The watcher is deliberately edge-triggered rather than sample-triggered:

- It emits one `initial` event.
- It emits `state_changed` only when the evaluated state changes.
- It emits collection/evaluation failures as `error` events.
- It does not emit unchanged samples.

This keeps a long-running monitor cheap to feed into an agent or orchestrator.

Watch JSONL events are also intentionally smaller than `usage advise` output. They contain only transition metadata, state, action, configured message, and the limiting window. They do not repeat the full usage snapshot. Agents that need full detail can call `usage status` or `usage advise` explicitly.

Individual collector calls are bounded by a timeout so a hung upstream collector does not freeze the watcher indefinitely.

## Delivery model

Delivery transports consume emitted domain events after policy evaluation. The first transport executes a local program through `usage watch --exec`.

The executable transport:

- receives canonical compact JSON on stdin;
- receives convenience `AGENT_INFORMANT_*` environment variables;
- has a bounded execution timeout;
- redirects child stdout away from Agent Informant's machine-readable stdout stream;
- treats delivery failure as non-fatal so monitoring continues.

The hook is invoked for the initial event as well as transitions and collection errors. This makes startup in an already-critical state immediately observable downstream.

The implementation lives in a generic delivery package rather than the usage package. Future domains can therefore reuse the same transport without depending on usage-specific types.

Additional transports should consume the same compact event contract rather than bypassing the watch state engine.

## Exit codes

`usage status`:

- `0`: usage data retrieved successfully
- `1`: invocation or parsing error

`usage advise`:

- `0`: green
- `10`: draining
- `20`: critical
- `1`: invocation or parsing error / no usable data

The non-zero draining/critical codes make the command usable as a shell guard while JSON/text output remains suitable for agents.

`usage watch` is long-running and exits `0` when stopped cleanly by an interrupt/termination signal. Collector failures are emitted as events and polling continues.

## Future direction

Possible additions:

- Named policy profiles for agents with different risk tolerances.
- Collector discovery and health diagnostics.
- A dedicated one-line/token-minimal probe format for extremely frequent agent checks.
- Persistent delivery configuration instead of CLI-only hooks.
- Local HTTP/webhook delivery.
- MCP transport layered on top of the same event model.
- Named pipes / Unix domain sockets for low-overhead local integrations.
- A general event model so future domains can expose agent-facing operational signals without changing the CLI architecture.
