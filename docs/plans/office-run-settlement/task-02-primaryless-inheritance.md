---
id: "02-primaryless-inheritance"
title: "Resolve primaryless inherited workspaces"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUN-SETTLE-002
acceptance_criteria:
  - AC-OFFICE-RUN-SETTLE-002.1
  - AC-OFFICE-RUN-SETTLE-002.2
  - AC-OFFICE-RUN-SETTLE-002.3
  - AC-OFFICE-RUN-SETTLE-002.4
  - AC-OFFICE-RUN-SETTLE-002.5
system_design:
  - ../../specs/office/system-design/run-settlement-integrity.md
---

# Task 02: Resolve primaryless inherited workspaces

## Outcome

An `inherit_parent` child launches into its parent's environment when the
parent has no primary session, and an unsafe reuse is escalated at once.

## In scope

- Session and parent-owned environment fallbacks with owner validation.
- Immediate escalation of `ErrWorkspaceReuseUnsafe` in `HandleRunFailure`.

## Exclusions

- Primary promotion of Office sessions.

## Acceptance

- Primaryless parents resolve through their session environment or the
  parent-owned environment; existing fail-closed regressions still pass.
- A permanent unsafe workspace reuse fails the run without a retry; a live
  parent without any environment yet keeps the backoff and is recorded once.

## Verification

```bash
cd apps/backend
go test -count=1 ./internal/orchestrator/ -run 'Inherit|SharedGroup'
go test -count=1 ./internal/office/service/ -run 'TestHandleRunFailure'
```

## Likely files

- `apps/backend/internal/orchestrator/handoff_inheritance.go`
- `apps/backend/internal/office/service/retry.go`

## Results

Both primaryless inheritance tests failed before the change with the production
error (`parent task parent workspace could not be resolved`).
`TestHandleRunFailure_WorkspaceReuseUnsafeDoesNotRetry` failed with
`run status = "queued"`. All pass after the change, with the dangling and
archived fail-closed regressions.

Refinement: `TestInheritFromParentEnvironment_LiveParentWithoutEnvironmentIsPending`
failed before it (`...workspace could not be resolved, want unsafe reuse marked
pending`) and `TestHandleRunFailure_PendingInheritedWorkspaceRetriesVisibly`
failed with `run status = "failed" retry_count = 0, want queued with one
retry`. Both pass after it, with the terminal-parent and conflicting-environment
permanent tests.
