---
id: "05-summarize-recovery-outcomes"
title: "Summarize startup recovery outcomes"
status: done
wave: 5
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-PLATFORM-STARTUP-PROGRESS-001
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4
  - AC-PLATFORM-STARTUP-PROGRESS-001.22
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/platform/system-design/startup-progress-visibility.md
---

# Task 05: Summarize startup recovery outcomes

## Summary

Startup reported 0/39 recovered sessions, but the inventory was stale and
existing safety rules correctly retained uncertain records. Emit one aggregate
outcome summary so future operators can interpret the warning.

## In scope

- Count candidates, retracked instances, and not-retracked records in the
  lifecycle recovery pass; classify only reasons the pass proves.
- Cover zero-candidate, partial, and unknown-liveness cases in focused tests.

## Out of scope

- Counting skipped records as done, suppressing the progress warning, or
  deleting/stopping uncertain runtime records.

## Acceptance

- One summary per pass reconciles candidate and outcome counts.
- Unmatched backend records remain unknown unless a specific reason is proven.
- Existing startup progress and guarded recovery behavior are unchanged.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test.*Recovery' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/startup -run 'Test.*Step' -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_recovery_sessions_step_test.go`
- `apps/backend/internal/startup/step_lifecycle_test.go`

## Dependencies

None.

## Risks

- The recovery backend cannot report why every record was declined; use an
  explicit unknown bucket rather than fabricating a reason.

## Parallelism

`sequential`

## Inputs

- Runtime-failure-attribution and startup-progress designs.
- Executor survival requirements for unknown-liveness records.

## Results

Startup recovery now emits one aggregate summary with candidate-count
certainty, re-tracked count, known refusal counts, and the unknown remainder.
Missing inventory data is marked unknown; it does not become an empty
candidate set. Existing progress totals, warnings, liveness guards, and stop
decisions remain unchanged. Verification passed:

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test.*Recovery' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/startup -run 'Test.*Step' -count=1)
```
