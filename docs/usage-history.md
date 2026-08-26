# Usage history and drain rate

Agent Informant records every successful real usage collection in a small rolling JSONL history. This happens below the command layer, so `usage status`, `usage advise`, `usage watch`, and installed Claude/Codex lifecycle hooks all contribute samples automatically.

The default history path is under the platform user cache directory:

```text
<user-cache>/agent-informant/usage-history.jsonl
```

Set `AGENT_INFORMANT_HISTORY` to override it.

History is bounded to recent data. Agent Informant keeps at most 24 hours / 5000 snapshots and opportunistically compacts the file when it grows beyond roughly 2 MiB. Malformed or partially written JSONL rows are ignored rather than making usage checks fail.

## Drain-rate analysis

For each relevant provider/window, Agent Informant compares recent samples from the same reset cycle. A reset timestamp change breaks the trend so a replenished quota is never interpreted as negative consumption.

The default analysis window is 30 minutes and requires at least two samples spanning 20 seconds. Small changes of 0.05 percentage points or less are treated as noise.

Derived information includes:

- observed sample count and duration
- percentage points consumed per hour
- qualitative rate (`slow`, `elevated`, `fast`, `extreme`)
- estimated minutes until the configured critical threshold
- estimated minutes until exhaustion

`usage advise` and lifecycle hooks append a compact rate summary when enough history exists. For example:

```text
Drain rate is extreme at 42.0%/hour; projected exhaustion in about 18 minutes.
```

## Rate-aware policy

Absolute remaining quota still matters, but it is no longer the only input. The default policy also escalates based on projected exhaustion:

```text
projected exhaustion <= 30 minutes  -> at least draining
projected exhaustion <= 10 minutes  -> critical
```

This is intended to catch the dangerous case where 40-50% quota technically remains but the current agent is consuming it rapidly enough that another large reasoning/tool cycle could exhaust the window.

The rate context is included in the same user-configurable draining/critical instruction that structural runtime hooks inject into Claude or Codex. No additional agent probing is required.
