---
id: "05-resume-nested-roots"
title: "Resume tasks with nested adopted worktrees"
status: complete
wave: 5
depends_on:
  - "04-prove-end-to-end"
plan: "plan.md"
requirements:
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-003
acceptance_criteria:
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-003.6
system_design:
  - ../../specs/tasks/system-design/agent-worktree-adoption.md
---

# Task 05: Resume tasks with nested adopted worktrees

## Summary

An agent may create a worktree below an attached checkout, for example
`<task-root>/<repo>/.tmp/<name>`. Adoption records it, but workspace recovery on resume
treated any checkout deeper than a direct child of the task root as a corrupted managed
placement. Every resume, diff, and commit read of that task then failed with
`persisted checkout path does not match its managed task-root identity`, and the session ended
in `FAILED`.

Recovery now treats a descendant of the owning task root that is not a direct child as contained
but unmanaged: a present checkout is validated in place and attached, and a missing one is never
recreated automatically. Paths in another task root, or the task root itself, still fail closed.

## In scope

- `missingCheckoutNameUnderTaskRoot` in `apps/backend/internal/worktree/missing_checkout_recovery.go`.
- Recovery-admission and attach tests for a nested adopted worktree.

## Out of scope

- Detaching an adopted worktree that was removed from disk while no runtime was observing it.
  Resume still fails closed for that slot.

## Acceptance

- Recovery admission succeeds and attach-only reuse returns the nested adopted worktree.
- A missing nested worktree is not recreated.
- Location classification keeps failing closed for another task root and the task root itself.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/worktree -run 'NestedAdopted|MissingNestedWorktree|ClassifiesTaskRootDescendants|MissingCheckout' -count=1)
bash scripts/lint-go-changed apps/backend/internal/worktree/missing_checkout_recovery.go apps/backend/internal/worktree/missing_checkout_nested_test.go
(cd apps/backend && make test)
```

## Files likely touched

- `apps/backend/internal/worktree/missing_checkout_recovery.go`
- `apps/backend/internal/worktree/missing_checkout_nested_test.go` (new)

## Dependencies

Task 04.

## Risks

- A Kandev-managed checkout that is somehow persisted at a nested path is no longer reported as
  corrupted. It is still validated in place against the task-root ownership marker and is never
  recreated, so the failure mode is a refused recovery rather than a write to the wrong location.

## Parallelism

`sequential`

## Inputs

- Production report: a task with two adopted worktrees under `<repo>/.tmp/` could not resume after
  idle reclaim released its runtime.

## Results

The new tests failed with the production error before the change and pass after it. Recovery
admission and attach-only reuse run against a copy of the affected production database: the
original code fails with the production error, and with the change all 13 checkouts attach,
including both nested adopted worktrees.
