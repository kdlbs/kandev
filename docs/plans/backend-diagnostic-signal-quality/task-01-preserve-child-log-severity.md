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

Extend the agentctl launcher parser to recognize the Go `slog` formats used by
the child process.

## Scope

- Extend `childLogLevel` with anchored Go `slog` TextHandler and default-handler
  parsers.
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
- Both `slog.TextHandler` key/value output and the default handler's standard
  log date/time output are recognized.
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

Implemented anchored Go `slog.TextHandler` and default-handler parsing while
preserving the existing Zap parser and warning fallback. The text parser is
quote-aware so a `msg=` substring inside an attribute cannot change severity.
Added coverage for actual `slog.Default()` output, all standard TextHandler
levels, and malformed/unstructured records.

Verification:

```text
cd apps/backend && go test ./internal/agent/runtime/agentctl/launcher -run 'Test(ChildLogLevel|PipeOutput)' -count=1
Go test: 28 passed in 1 packages
```
