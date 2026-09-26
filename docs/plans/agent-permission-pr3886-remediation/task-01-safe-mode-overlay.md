---
id: "01-safe-mode-overlay"
title: "Make the mode overlay safe"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.8
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.12
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.15
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.17
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 01: Make the mode overlay safe

## Summary

Create a session-owned settings overlay from empty settings or an explicitly
selected bundle. Preserve authentication and a configured agent directory, or
report that the start mode is unavailable.

## In scope

- Remove implicit host configuration linking and copying from start-mode setup.
- Validate the settings root and write the overlay atomically with mode `0600`.
- Resolve effective launch environment, selected bundle input, and auth
  compatibility before advertising delivery.

## Out of scope

- Remote transfer and selected-bundle sequencing, owned by Task 02.
- A new provider startup channel.

## Acceptance

1. No unselected host setting or credential enters a session because a mode was set.
2. JSON `null`, scalar, array, and malformed roots cannot panic preparation.
3. A host auth or explicit config-directory conflict returns an unavailable reason, without a false delivery claim.

## Verification

```bash
cd apps/backend
go test ./internal/agent/runtime/lifecycle/initialmode ./internal/agent/runtime/lifecycle -run 'Test(InitialMode|Materialize)' -count=1
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/initial_mode.go`
- `apps/backend/internal/agent/runtime/lifecycle/initialmode/materialize.go`
- `apps/backend/internal/agent/runtime/lifecycle/initialmode/materialize_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/initial_mode_test.go`

## Dependencies

None.

## Risks

The provider may bind authentication to a configuration directory. Preserve
the existing authenticated path when no safe overlay channel exists.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/agent-permission-control-integrity.md), `002.8`, `002.12`, `002.15`, `002.17`.
- [System design](../../specs/agents/system-design/agent-permission-control-integrity.md), initial-mode delivery.
- [ADR](../../decisions/2026-09-25-session-mode-configuration-boundary.md).

## Results

Completed in this work session. The materializer now accepts only explicit
selected settings bytes (or no settings), validates that the input root is a
JSON object, returns controlled errors for null/scalar/array/malformed input,
and writes the result atomically with mode `0600`. The launch path no longer
reads or links the backend user's agent configuration. Explicit config-dir
overrides and standalone host authentication paths are preserved with a clear
unavailable result. The executor-specific installation and final delivery
report remain in Task 02.
