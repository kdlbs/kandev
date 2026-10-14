---
id: "02-drop-structurally-unavailable-entries"
title: "Drop structurally unavailable entries"
status: in_progress
wave: 2
depends_on:
  - "01-drop-invalid-launch-replays"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
acceptance_criteria:
  - AC-AGENTS-SESSION-CEILING-001.5
  - AC-AGENTS-SESSION-CEILING-001.12
system_design:
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 02: Drop structurally unavailable entries

## Summary

Separate a workflow entry that can never become valid from one whose reads
failed. Drop the first and keep the second for a later sweep.

## In scope

- Add `ceilingEntryPermanentlyUnavailable` and return it for a legacy
  sessionless `start`, an underivable or incomplete binding, an unrouted entry
  whose step now routes sessions, and a missing durable entry identity.
- Return read errors from `deriveCeilingEntryBinding` and add
  `readWorkflowEntryIdentity`, so a failed step, transition, or task read stays
  `unavailable`.
- Treat the new disposition like `superseded` in the sweep's drop reasons, the
  claimed replay load, the post-claim validation, and the claimed binding check.

## Out of scope

- Moves into steps that cannot run an agent. The existing start drop reason
  already covers them.
- An operator route to discard a record.

## Acceptance

- A sweep drops a sessionless `start` or an underivable binding instead of
  retrying it.
- A sweep keeps a record when the step or transition read fails during entry
  validation.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/ -run 'CeilingReplay|CeilingEntry|WorkflowEntryIdentity'
```

## Likely files

- `apps/backend/internal/orchestrator/ceiling_entry.go`
- `apps/backend/internal/orchestrator/ceiling_replay.go`
- `apps/backend/internal/orchestrator/workflow_session_target.go`
- `apps/backend/internal/orchestrator/task_operations.go` (call-site signature)
- `apps/backend/internal/orchestrator/ceiling_entry_terminal_test.go`

## Results

- `ceiling_entry_terminal_test.go` covers the sweep and the drop reasons.
  Before the change a sessionless legacy `start` that the sweep could not bind,
  and an underivable binding, were kept on every sweep; now they are dropped as
  superseded without a launch.
- A failed step read, during validation or binding derivation, still keeps the
  record.
- A failed transition read during validation used to fall back to the creation
  identity, so a valid record looked superseded and was dropped. It is now kept
  for a later sweep.
- `readValidCeilingDeferredLaunch`, Send Now, and the claimed binding check
  already treat any non-valid disposition as not pending, so the new
  disposition needs no change there.

### PR review follow-up

An already bound start now preserves a failed auto-start eligibility read.
The permanent sweep regression verifies the queued record remains and no agent
launch occurs. Genuine deleted-step and readable non-auto-start dispositions
remain terminal. Local bounded exact-source proof passes; hosted validation is
pending.

Stored workflow-binding decode and incompleteness failures are now terminal.
A four-field durable sweep regression prevents these records from retrying;
genuine repository read failures remain retained. Bounded exact decoder/prefix
RED/GREEN proof passes with no package or SQL coverage credit.
