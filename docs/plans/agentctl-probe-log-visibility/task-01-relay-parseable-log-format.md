---
id: "01-relay-parseable-log-format"
title: "Pin a relay-parseable agentctl log format"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002
acceptance_criteria:
  - AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002.1
  - AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002.2
system_design:
  - ../../specs/agents/system-design/probe-failure-diagnostics.md
---

# Task 01: Pin a relay-parseable agentctl log format

## Summary

The launcher owns the agentctl child's log format because the child's stdout
feeds the launcher's relay. Pin `AGENTCTL_LOG_FORMAT` to the JSON format the
relay parses and drop any inherited value, so an operator's ambient environment
cannot break lifecycle record ingestion.

## In scope

- Add `AGENTCTL_LOG_FORMAT=json` to `Launcher.childEnvOverrides` in
  `internal/agent/runtime/agentctl/launcher/launcher.go`.
- Regression test proving the override pins the format and replaces an
  inherited value.

## Out of scope

- The relay's record parsing (landed upstream in #4223 and #4263).
- Any probe-side behavior (Task 02).

## Applicable criteria

- AC-AGENTS-PROBE-FAILURE-DIAGNOSTICS-002.1 and 002.2
- System design: [Probe Failure Diagnostics](../../specs/agents/system-design/probe-failure-diagnostics.md)

## Acceptance conditions

1. `TestChildEnvOverridesPinRelayParseableLogFormat` fails before the pin and
   passes after.
2. An inherited `AGENTCTL_LOG_FORMAT` value cannot survive into the child
   environment.
3. The JSON relay's existing privacy projection still excludes arbitrary
   structured fields; approved probe diagnostics survive in the safe message.

## Verification commands

```bash
cd apps/backend
go test -race -trimpath ./internal/agent/runtime/agentctl/launcher/ -run 'TestChildEnvOverridesPinRelayParseableLogFormat|TestPipeOutputPreservesSeverityWithNestedDuplicateComponentFields' -count=1
```

## Likely files

- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher.go`
- `apps/backend/internal/agent/runtime/agentctl/launcher/launcher_test.go`

## Results

The original console pin in `33980ff74` is superseded by a JSON pin to retain
the relay's structured-field privacy boundary. The existing environment test
now covers duplicate inherited console/text values. Hosted RED and final
verification results are retained in PR evidence.
