---
created: 2026-10-14
status: complete
requirements:
  - REQ-AGENTS-PROBE-FAILURE-DIAGNOSTICS-001
  - REQ-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002
system_design:
  - ../../specs/agents/system-design/probe-failure-diagnostics.md
legacy_specs: []
---

# Implementation Plan: Fix agentctl probe log visibility

## Overview

Make failed capability and profile probes diagnosable from backend logs. Two
work orders restore the operator's window into child-side probe failures and
protect the record ingestion path that depends on the agentctl child's stdout
format.

The agent system owns this repair because it owns the agent capability surface:
the probe executor and the agentctl child launcher are agent-system
components, and the failed outcome is an operator who cannot diagnose a probe
failure. The intended behavior was missing from requirements, so this package
adds the smallest requirement set that defines it
([Probe Failure Diagnostics](../../specs/agents/requirements/probe-failure-diagnostics.md));
no existing requirement or acceptance criterion covers probe failure logging or
the child log format contract.

## Evidence and root cause

A probe whose child dies before answering reported only "peer disconnected
before response". The child's actual reason, such as an `npx` resolution
failure, a missing runtime, or a panic, went to the void: the UI's "check agent
logs" advice pointed at logs that never carried it. The launcher also needs a fixed child format:
console records bypass the
JSON relay's intentional structured-field privacy projection, while a JSON
record drops context fields that are not in its trusted envelope.

The repair pins JSON and puts only approved, sanitized probe diagnostics in
the message retained by that projection. Raw evidence remains local to
classification; unrelated child structured fields remain excluded.

The relay's parsing of agentctl JSON log records landed separately upstream
(#4223, #4263) and needed no change.

## Work orders

- [Task 01: Pin a relay-parseable agentctl log format](task-01-relay-parseable-log-format.md)
- [Task 02: Log sanitized probe failure diagnostics](task-02-probe-failure-log-diagnostics.md)

Both work orders are independent (different files, different contracts) and
run in wave 1.

## Verification strategy

Each work order owns its regression tests at the owning boundary; there is no
UI change, so no rendered checks apply. Commands:

```bash
cd apps/backend
go test -race -trimpath ./internal/agent/runtime/agentctl/launcher/ -run TestChildEnvOverridesPinRelayParseableLogFormat -count=1
go test -race -trimpath ./internal/common/securityutil/ ./internal/agent/runtime/routingerr/ -count=1
go test -race -trimpath ./internal/agentctl/server/utility/ -run 'ProbeFailureLog|ProbeModelDiscoveryFailureLog|CarriesScrubbedStderr|StderrBuffer' -count=1
```

## Risks and exclusions

- Log sanitization must not destroy the diagnostic text the log exists for:
  the provider-diagnostic redaction tier preserves error codes and ordinary
  failure descriptions while masking opaque credentials and private paths.
- Managed child records must use the JSON envelope allowlist. Sanitized probe
  details belong in the message retained by that projection.
- Client responses stay sanitized; no subprocess detail crosses the client
  boundary.
- Out of scope: the relay's record parsing (upstream), client-visible probe
  error copy, and persistent-session stderr streaming.

## Results

The original work orders are implemented. Review remediation extends provider
diagnostic redaction to opaque credentials, private URLs and paths, structured
protocol errors, and secondary model-discovery errors. A truncated first line
is excluded from log output while the raw bounded tail still drives failure
classification. The managed format is pinned to JSON, sanitized diagnostics
survive in the message, and shared sanitizer ownership moves to
`common/securityutil` with existing runtime APIs retained. Hosted RED assertions and final hosted verification are
tracked in PR evidence; no local contributor execution is used.
