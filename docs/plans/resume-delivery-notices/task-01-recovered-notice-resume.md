---
id: "01-recovered-notice-resume"
title: "Restore recovered delivery notices on native Resume"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.6
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 01: Restore recovered delivery notices on native Resume

## Summary

Let the authenticated native-resume transaction restore a delivery notice that
is already `recovered` but whose execution was lost, instead of refusing the
whole block resolution and stranding the session.

## In scope

- `nativeResumeNoticeMatches` accepts the `recovered` phase alongside
  `uncertain` and `reconnecting`. Generation, stream, and submission matching
  are unchanged.

## Out of scope

Orchestrator Retry handling, recovery publication, UI.

## Acceptance

1. A `recovered` notice with a matching generation, stream, and retired
   submission is restored by explicit Resume on the current ready execution, and
   the delivery block is resolved.
2. The restored notice keeps its historical execution ID and submission; the
   submission's state and payload hash are unchanged.
3. A `settled` notice still refuses resolution and leaves the block open.

## Verification

- `go test ./internal/task/repository/sqlite/ -run NativeResumeRestoresRecoveredNoticeAfterExecutionLoss -count=1`
  fails on the base revision (`expected: true, actual: false`) and passes with
  the fix.
- `go test ./internal/task/repository/sqlite/ -count=1`.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/native_resume_binding.go`
- `apps/backend/internal/task/repository/sqlite/native_resume_recovered_notice_test.go` (new)
- `docs/specs/platform/requirements/durable-restart-reconciliation.md`
- `docs/specs/platform/system-design/session-recovery-convergence.md`
- `docs/plans/resume-delivery-notices/plan.md`
- `docs/plans/resume-delivery-notices/task-01-recovered-notice-resume.md`

## Dependencies

None.

## Inputs

- [Owning requirement](../../specs/platform/requirements/durable-restart-reconciliation.md)
- [Owning design](../../specs/platform/system-design/session-recovery-convergence.md)
- [Plan](plan.md)

## Risks

Accepting a settled or foreign notice would overwrite unrelated history; only the
`recovered` phase is added and every other proof check remains.

## Parallelism

`sequential`
