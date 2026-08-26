# Architecture

Agent Informant is organized around domains, collectors, normalized models, and policies.

## Command model

```text
agent-informant <domain> <command> [flags]
```

`usage` is the first domain. Future domains might expose other information agents benefit from, without forcing those concepts into the usage model.

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
```

The CLI must never expose an upstream collector's raw schema as its contract. Upstream formats can change; Agent Informant's normalized structures are the compatibility boundary.

## Initial collector

The first collector is CodexBar. It is deliberately treated as an executable dependency rather than as the architecture itself.

The adapter invokes:

```text
codexbar usage --format json
```

and discovers provider/window data from the returned JSON. The parser is intentionally tolerant of nested objects and common key variants so small upstream payload changes are less likely to break the CLI.

Additional collectors should implement the same `UsageCollector` interface.

Potential future sources include direct provider APIs, ccusage-compatible sources, local telemetry, or a daemon/API endpoint.

## Normalized model

A usage snapshot contains zero or more providers. Each provider contains one or more windows.

A normalized window has:

- provider
- name
- percent used
- percent remaining
- reset timestamp, when known
- source

Percentages are represented as values from 0 to 100.

## Policy

The initial policy maps remaining capacity to three states:

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

For multiple windows, the most restrictive state wins. This is deliberate: a provider with a healthy short window but nearly exhausted long window should not be reported as healthy.

Policy outputs contain both a machine-friendly state/action and a short instruction suitable for inserting into an agent context.

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

## Future direction

Possible additions:

- `usage watch` to emit state transitions.
- Configurable policies stored in a user config file.
- Named profiles for agents with different risk tolerances.
- Collector discovery and health diagnostics.
- MCP or local HTTP transport, layered on top of the same domain services.
- A general event model so future domains can expose agent-facing operational signals without changing the CLI architecture.
