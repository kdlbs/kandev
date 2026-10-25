---
id: "01-reset-source-executor"
title: "Reset executor on source transition"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-create-executor-default.md"
---

# Task 01: Reset Executor on Source Transition

## Intent

Invalidate the executor selected for repository-less mode when the user returns to Repo so the
existing destination-source policy can choose the correct profile.

## Acceptance

- Entering repository-less mode clears the previous executor and continues to resolve direct Local.
- Leaving repository-less mode also clears Local so Repo resolves the valid workspace default or
  Worktree fallback.
- Source flags, selected repositories and branches, remote rows, and last-used persistence behavior
  remain unchanged.

## TDD sequence

1. Add a focused `useDialogHandlers` regression for leaving repository-less mode with a populated
   Local executor.
2. Run the regression. Verify that it fails because the executor setters are not called.
3. Add or retain the paired entering-mode assertion.
4. Move the executor invalidation to the shared part of `handleToggleNoRepository` and run the
   focused handler and executor-policy suites to GREEN.
5. Run the web typecheck.

## Files likely touched

- `apps/web/components/task-create-dialog-handlers.ts`
- `apps/web/components/task-create-dialog-handlers.test.ts`

## Dependencies

None.

## Parallelism

`sequential`. This task owns the shared state transition required by Task 02.

## Inputs

- Spec scenario for Repo to repository-less mode to Repo.
- Plan sections `Confirmed root cause` and `Source transition reset`.
- Existing `useDefaultSelectionsEffect` policy and ADR
  `docs/decisions/2026-08-01-repository-task-executor-defaults.md`.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/task-create-dialog-handlers.test.ts components/task-create-dialog-effects-executor.test.ts && cd web && pnpm run typecheck
```

## Risks

- Do not select Worktree directly in the handler. Explicit workspace defaults and fallback policy
  belong to the existing effects.
- Assert both executor ID and profile invalidation because either populated field can suppress or
  conflict with recomputation.

## Output contract

Report the RED failure, changed files, exact results, blockers, and risks. Then update this task and
`plan.md` in the same conversation.

## Results

RED:

- `cd apps/web && pnpm test -- --run components/task-create-dialog-handlers.test.ts -t "clears the executor when leaving repository-less mode"`
- Failed with the expected assertion because the executor setters received no empty value.

GREEN and task checks:

- `cd apps/web && pnpm test -- --run components/task-create-dialog-handlers.test.ts -t "clears the executor when leaving repository-less mode"` — 1 passed.
- `cd apps && pnpm install --frozen-lockfile` — dependencies were ready.
- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-handlers.test.ts components/task-create-dialog-effects-executor.test.ts` — 37 passed across 2 files.
- `cd apps/web && pnpm run typecheck` — passed.

Changed files:

- `apps/web/components/task-create-dialog-handlers.ts`
- `apps/web/components/task-create-dialog-handlers.test.ts`

Cleanup and side effects: no temporary files or external services were created.
