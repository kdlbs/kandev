---
id: "01-pin-status-transitions"
title: "Pin the agent status transition contract"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AGENT-RECOVERY-002
acceptance_criteria:
  - AC-OFFICE-AGENT-RECOVERY-002.3
  - AC-OFFICE-AGENT-RECOVERY-002.4
  - AC-OFFICE-AGENT-RECOVERY-002.6
system_design:
  - ../../specs/office/system-design/agent-recovery.md
---

# Task 01: Pin the agent status transition contract

## Summary

Add a Go table test over `validateStatusTransition` so the transitions the
recovery control depends on are pinned before a caller exists. The recovery
control consumes this contract unchanged; this work order adds no production
code.

## In scope

- A test covering `paused -> idle`, `stopped -> idle`, and the same-status
  `idle -> idle` no-op.
- A test covering a transition the table refuses and an unknown source status.

## Out of scope

- Any change to `allowedTransitions`, `validateStatusTransition`,
  `UpdateAgentStatus`, the handler, or the repository.
- Adding an event publisher or activity entry.
- Anything in the web tree.

## Acceptance

- `validateStatusTransition` returns no error for `paused -> idle`,
  `stopped -> idle`, and `idle -> idle`.
- It returns an `ErrAgentStatusTransition` error for a transition absent from
  the table and for an unknown source status.
- No file outside the new test changes.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -count=1 -run TestValidateStatusTransition ./internal/office/agents/)
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/office/agents/service_status_transition_test.go`

## Dependencies

None.

## Risks

- `internal/office/service/agents.go` declares a second `allowedTransitions`
  table. This work order pins the one in `internal/office/agents`, which is the
  table the `PATCH /agents/:id/status` route reaches. Pinning the wrong package
  would produce evidence for a path the recovery control never uses.

## Parallelism

`parallel-safe`

Disjoint from Task 02: one Go test file against no shared schema, migration,
generated contract, lockfile, or package configuration.

## Inputs

- `docs/specs/office/requirements/agent-recovery.md`, REQ-OFFICE-AGENT-RECOVERY-002.
- `docs/specs/office/system-design/agent-recovery.md`, "Data and contracts".
- `apps/backend/internal/office/agents/service.go`, `allowedTransitions` and
  `validateStatusTransition`.

## Results

Pending.
