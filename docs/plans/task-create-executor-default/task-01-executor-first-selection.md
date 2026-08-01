---
id: "01-executor-first-selection"
title: "Enforce executor-first profile selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/task-create-executor-default.md"
---

# Task 01: Enforce executor-first profile selection

## Intent

Prevent a portable last-used profile from changing the executor selected by task-source and
workspace policy while preserving intentional workspace and local-source defaults.

## Acceptance

- An ordinary repository-backed task with no workspace default selects Worktree even when the
  backend last-used profile is Local.
- Explicit Local workspace defaults, explicit unmanaged local paths, and repository-less tasks
  still select Local.
- A saved profile is restored only within the resolved executor, with existing eligible fallback
  behavior preserved when that executor has no profile.

## TDD sequence

1. Add the Local-last-used/Worktree-default regression to
   `task-create-dialog-effects-executor.test.ts` and run it to confirm RED for the expected Local
   selection.
2. Add the workspace-default and source-constraint cases.
3. Change the smallest selection helpers/effects required in
   `task-create-dialog-effects.ts` and run the focused suite to GREEN.
4. Refactor helper names/log metadata only where necessary to describe executor compatibility.

## Files likely touched

- `apps/web/components/task-create-dialog-effects.ts`
- `apps/web/components/task-create-dialog-effects-executor.test.ts`

## Dependencies

None.

## Parallelism

`sequential` — this task owns the shared selection behavior required by the E2E task.

## Inputs

- Spec sections `What`, `Persistence guarantees`, and `Scenarios`.
- Plan sections `Confirmed root cause` and `Executor policy and profile restoration`.
- ADR 0028, ADR 0041, and ADR-2026-08-01-repository-task-executor-defaults.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-effects-executor.test.ts`
- `cd apps/web && pnpm run typecheck`

## Risks

- Independent effects can produce order-sensitive setter calls; assert the settled executor/profile
  pair through rerenders where needed.
- Do not remove or rewrite backend last-used persistence as part of this task.

## Output contract

Report the RED failure, final files changed, focused test/typecheck results, remaining risks, and
update this task plus `plan.md` status in the same conversation.
