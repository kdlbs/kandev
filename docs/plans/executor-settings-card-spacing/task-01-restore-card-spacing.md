---
id: "01-restore-card-spacing"
title: "Restore executor card spacing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/executor-settings-card-spacing.md"
---

# Task 01: Restore executor card spacing

Restore the layout box around the modern executor profile card fragments and add rendered regression coverage for edit and create forms on desktop and mobile.

## Acceptance

- Modern executor profile edit and create pages render at least the existing `space-y-8` rhythm between every adjacent visible settings card for all shared executor profile sections.
- Existing fieldset disabled state, manual-save behavior, card content, and route behavior remain unchanged.
- Desktop and mobile geometry checks pass, and the mobile profile form has no document-level horizontal overflow.

## Verification

Bootstrap a fresh worktree once if dependencies are absent:

```bash
cd apps && pnpm install --frozen-lockfile
```

Write and run the regression specs before the production class change (RED):

```bash
cd apps/web && pnpm e2e:run --project chromium tests/settings/executor-profile-spacing.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-executor-profile-spacing.spec.ts
```

Apply the minimal fieldset layout fix, then run the same commands (GREEN), followed by:

```bash
cd apps/web && pnpm run typecheck
git diff --check
```

The managed E2E runner rebuilds the production web assets before each change-aware run.

## Files likely touched

- `apps/web/app/settings/executors/[profileId]/page.tsx`
- `apps/web/app/settings/executors/new/[type]/page.tsx`
- `apps/web/e2e/tests/settings/executor-profile-spacing.spec.ts`
- `apps/web/e2e/tests/settings/executor-profile-spacing-helpers.ts`
- `apps/web/e2e/tests/settings/mobile-executor-profile-spacing.spec.ts`
- `docs/specs/ui/executor-settings-card-spacing.md`
- `docs/specs/INDEX.md`
- `docs/plans/executor-settings-card-spacing/plan.md`
- `docs/plans/executor-settings-card-spacing/task-01-restore-card-spacing.md`

## Dependencies

None.

## Parallelism

Sequential. The regression specs and the two production class changes share the same rendered surface and must be validated as one red-green pass.

## Inputs

- Repair contract and scenarios in `docs/specs/ui/executor-settings-card-spacing.md`.
- Root cause and file-level approach in `docs/plans/executor-settings-card-spacing/plan.md`.
- Existing settings mobile shell in `apps/web/components/settings/settings-layout-client.tsx` and mobile settings E2E conventions in `apps/web/e2e/tests/settings/mobile-settings-sidebar.spec.ts`.
- Existing worker fixture field `seedData.worktreeExecutorProfileId` and `ApiClient.createExecutorProfile` for the create-form setup.

## Output contract

Report the root cause confirmation, files changed, exact red and green commands with outcomes, typecheck and diff-check results, any blockers or risks, and synchronized task/plan status in the same conversation.

## Results

- Confirmed the `contents` fieldset class removed the layout box needed for the parent card stack spacing, producing `0px` gaps between adjacent cards.
- Replaced `contents` with `space-y-8` on both modern executor profile edit and create fieldsets. Disabled-state and manual-save behavior remain on the same fieldsets.
- RED desktop and mobile E2E runs both failed on the expected `0px` card gap.
- GREEN desktop and mobile E2E runs both passed (`1 passed` each).
- `pnpm run typecheck` and `git diff --check` passed.
