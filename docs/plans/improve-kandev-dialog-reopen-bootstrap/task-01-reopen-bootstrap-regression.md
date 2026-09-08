---
id: "01-reopen-bootstrap-regression"
title: "Re-run bootstrap when the dialog reopens"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 01: Re-run bootstrap when the dialog reopens

## Root cause

`useResetOnClose` in `apps/web/components/improve-kandev-dialog.tsx` resets
`workspaceChoiceConfirmed` to `false` every time the dialog closes. The effect
that auto-confirms the choice when the dedicated workspace exists only fires
when `improveWorkspaceMissing` changes, which is stable once the workspace is
known. On a reopen (e.g. filing a second bug), the bootstrap gate
(`!workspaceChoiceConfirmed`) early-returns and no bootstrap request is ever
sent: the dialog sits at the idle "Preparing kandev repository in background"
banner with submit blocked forever.

Regression path: dedicated `Improve Kandev` workspace exists (typically the
active workspace), intro dismissed (skip-intro in local storage), dialog
closed then reopened.

## Acceptance

1. Reopening the dialog with the dedicated workspace present re-runs
   `bootstrapImproveKandev` (component-level: a second call after
   open→close→open).
2. End-to-end, a second bug can be filed after the first one in the dedicated
   workspace: the contributor banner appears (bootstrap ready) and submit is
   enabled.
3. Existing behavior is unchanged: when the dedicated workspace is missing,
   the workspace-creation choice still gates bootstrap; the intro flow still
   sets the confirmation on Contribute.

## Verification

```bash
cd apps/web && pnpm vitest run components/improve-kandev-dialog.test.tsx
cd apps/web && pnpm exec eslint components/improve-kandev-dialog.tsx components/improve-kandev-dialog.test.tsx e2e/tests/improve-kandev.spec.ts
cd apps/web && pnpm run typecheck
# e2e (requires built backend bin/kandev + bin/mock-agent and web dist):
cd apps/web && pnpm e2e:raw tests/improve-kandev.spec.ts
```

## Files likely touched

- `apps/web/components/improve-kandev-dialog.tsx` — auto-confirm effect gains
  an `open` dep and `open &&` guard.
- `apps/web/components/improve-kandev-dialog.test.tsx` — reopen regression
  test.
- `apps/web/e2e/tests/improve-kandev.spec.ts` — second-bug e2e regression
  test.

## Dependencies

None.

## Parallelism

sequential.

## Inputs

- Spec regression scenario: "GIVEN the dedicated Improve Kandev workspace
  already exists and the intro has been dismissed, WHEN the user closes the
  dialog and reopens it, THEN bootstrap re-runs and submit becomes enabled."
- `useResetOnClose` and `useBootstrapKandev` in
  `apps/web/components/improve-kandev-dialog.tsx`.

## Output contract

Summary of change, exact commands run with results, task/plan status updated
in the same conversation.

## Results

- `cd apps/web && pnpm vitest run components/improve-kandev-dialog.test.tsx
  components/improve-kandev-dialog-model.test.ts
  components/app-sidebar/app-sidebar-new-task-item.test.tsx
  components/app-sidebar/app-sidebar-footer.test.tsx` → 4 files, 49 tests
  passed. The new reopen regression test failed before the fix
  (`bootstrapImproveKandev` called once, never again after open→close→open)
  and passes after.
- `pnpm exec eslint components/improve-kandev-dialog.tsx
  components/improve-kandev-dialog.test.tsx e2e/tests/improve-kandev.spec.ts`
  → clean.
- `NODE_OPTIONS="--max-old-space-size=8192" pnpm exec tsc --noEmit` → clean.
- `pnpm e2e:raw tests/improve-kandev.spec.ts tests/mobile-improve-kandev.spec.ts`
  → 15 passed (14 improve-kandev incl. the new second-bug test, 1 mobile).
  The new e2e test failed without the fix (contributor banner `@octocat` never
  appears on reopen — bootstrap never runs) and passes with it.
- Red/green evidence: both new tests were run against the pre-fix tree
  (stashed fix) and failed for the expected reason, then passed with the fix.
- PR fixup: comment-only remediation `5a034e86b` documenting the intentional
  per-open bootstrap re-probe (Claude review suggestion, non-blocking).
  Final head CI 27/27 green, threads resolved, mergeable clean.

Security/trust: none. External side-effect boundaries: none.
