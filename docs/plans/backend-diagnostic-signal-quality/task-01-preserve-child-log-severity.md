---
id: "01-preserve-child-log-severity"
title: "Preserve child log severity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/expected-runtime-log-severity.md"
---

# Task 01: Preserve child log severity

Extend the agentctl launcher parser to recognize Go `slog` text records.

## Scope

- Extend `childLogLevel` with an anchored Go `slog` text-record parser.
- Keep the existing Zap console parser and ANSI handling.
- Keep unrecognized stderr on the warning fallback.

## Exclusions

- Do not change child logger configuration.
- Do not parse arbitrary level-like words from messages.
- Do not change stdout handling.

## Traceability

- `REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001`
- `AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.9`
- `AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.10`
- `AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.13`
- `AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.14`
- Design: `docs/specs/platform/system-design/expected-runtime-log-severity.md`

## Acceptance

- Recognized Go `slog` `DEBUG`, `INFO`, `WARN`, and `ERROR` records use the
  matching parent severity.
- Existing Zap console records keep their current classification.
- Malformed and unstructured stderr remains at warning level.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/agentctl/launcher -run 'Test(ChildLogLevel|PipeOutput)' -count=1
```

The new `slog` case must fail before the production change because the current
parser accepts only tab-separated Zap console records.

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_loglevel_test.go`

## Dependencies

None.

## Results

Implemented the anchored Go `slog` text-record parser while preserving the
existing Zap parser and warning fallback. Added coverage for all standard
`slog` levels and malformed/unstructured records.

Verification:

```text
cd apps/backend && go test ./internal/agent/runtime/agentctl/launcher -run 'Test(ChildLogLevel|PipeOutput)' -count=1
Go test: 23 passed in 1 packages
```
