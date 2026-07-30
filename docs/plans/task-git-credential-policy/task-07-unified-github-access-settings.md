---
id: "07-unified-github-access-settings"
title: "Group task credentials with Workspace GitHub access"
status: done
wave: 5
depends_on: ["04-settings-explanation", "06-e2e-and-documentation"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 07: Group Task Credentials With Workspace GitHub Access

## Acceptance

- Workspace GitHub access shows one compact read-only summary containing the active automation
  identity and effective task access mode, with no standalone Task Git credentials section.
- Change GitHub connection contains the managed/executor task-access controls and an explicit
  dialog submission; success refreshes the summary, failure preserves the draft, closing without
  saving restores the persisted mode, and neither task-policy changes nor connection changes
  silently mutate the other setting.
- Desktop and mobile preserve the same capability. The phone flow uses the existing full-height
  Drawer with one scroll owner, safe-area clearance, touch-reachable controls, and no horizontal
  overflow. Public GitHub integration docs point users to the new location.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/github/github-connection-dialog.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/integrations/github-workspace-settings.spec.ts -- --project=chromium --grep "task Git credential policy"
cd apps/web && pnpm e2e:run tests/integrations/mobile-github-workspace-settings.spec.ts -- --project=mobile-chrome --grep "task Git credential"
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/components/github/github-settings.tsx`
- `apps/web/components/github/github-status.tsx`
- `apps/web/components/github/github-task-credentials-section.tsx`
- `apps/web/components/github/github-connection-dialog.tsx`
- `apps/web/components/github/github-connection-dialog.test.tsx`
- `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-github-workspace-settings.spec.ts`
- `docs/public/integrations.md`
- this task file and `plan.md`

## Dependencies

Tasks 04 and 06 provide the shipped policy controls, responsive GitHub connection surface, E2E
fixtures, and public explanation that this task regroups without changing backend behavior.

## Parallelism

Sequential. The component, desktop/mobile E2E, and documentation edits describe one user-visible
flow and should land together.

## Inputs

- Spec `What`, `UX And Mobile Contract`, and grouped-access scenarios.
- ADR `docs/decisions/2026-07-27-task-git-credential-policy.md`: automation identity and task policy
  remain behaviorally separate even when shown together.
- ADR `docs/decisions/0046-settings-route-save-coordinator.md` and
  `docs/specs/ui/settings-manual-save.md`: dialog-level explicit submissions remain named immediate
  actions and do not use the route floating-save contributor.
- `apps/web/components/github/github-connection-dialog.tsx` as the existing desktop Dialog/mobile
  full-height Drawer composition.

## Mobile Design Contract

- **Outcome and entry point:** the Workspace GitHub access summary exposes the saved task mode;
  the same Change connection button opens configuration on desktop and mobile.
- **Exemplar:** reuse `GitHubConnectionDialog` itself for the shipped full-height mobile Drawer,
  including its fixed header, single `min-h-0` scroll body, and safe-area bottom padding.
- **Hierarchy and action:** workspace automation method first, task access second; the task section
  has a clearly labeled explicit submission and does not compete with connection-method actions.
- **Surface rationale:** both choices are infrequent workspace-level GitHub configuration, so one
  bounded dialog/full-height phone Drawer is more appropriate than a second page section or stacked
  overlay.
- **Shared versus responsive behavior:** share task-mode state, validation, persistence, and labels;
  only the existing Dialog/Drawer shell differs by viewport.
- **Proof:** desktop and `mobile-chrome` E2E save executor mode through the dialog, observe the
  compact summary, reopen to prove persistence, and assert the mobile single-scroll/no-overflow
  geometry.

## Risks

- The dialog contains independent workspace-connection and task-policy submissions. Labels and
  callbacks must not imply one atomic save or close/discard the other draft unexpectedly.
- Workspace settings are also read by repository-scope controls. Use partial updates and avoid
  introducing a competing whole-resource draft owner.
- App installation may navigate away from Kandev. The task-policy draft must remain explicitly
  unsaved unless its own submission succeeds.

## Output contract

Report the summary wording, dialog hierarchy, submission behavior, mobile geometry, RED/GREEN
component and E2E results, docs validation, files changed, risks, and task/plan status updates.

## Verification results

- `pnpm --filter @kandev/web test -- --run components/github/github-connection-dialog.test.tsx` —
  passed (6 tests).
- `pnpm run typecheck` — passed.
- Chromium E2E for `configures task Git access from the workspace connection dialog` — passed after
  a deliberate RED failure for the missing summary.
- `mobile-chrome` E2E for `configures task Git access in the connection drawer` — passed, including
  single-scroll, 44px-control, and no-overflow assertions.
- `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs` — passed (58 tests; 41 published docs pages).
- `git diff --check` — passed.
