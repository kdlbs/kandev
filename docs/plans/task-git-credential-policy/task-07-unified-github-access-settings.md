---
id: "07-unified-github-access-settings"
title: "Group task credentials with Workspace GitHub access"
status: done
wave: 5
depends_on: ["04-settings-explanation", "06-e2e-and-documentation"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 07: Group Task Credentials With Workspace GitHub Access

## Acceptance

- Workspace GitHub access shows one compact read-only summary containing the active automation
  identity and effective task access mode, with no standalone Task Git credentials section.
- PAT/CLI connections describe their shared human identity inside that summary and do not render a
  separate My GitHub identity section. App automation keeps the separate, connectable personal
  identity section.
- The workspace identity and task-access summary lines provide accessible help: a tooltip on
  hover/keyboard focus and an equivalent 44px-target Drawer interaction on touch devices.
- The connection status restores the workspace-scoped GitHub API, GraphQL query, and Search quota
  disclosure through the same tooltip/Drawer pattern when snapshots are available.
- Change GitHub connection contains the managed/executor task-access controls and one **Save
  changes** action for PAT/CLI and task-access drafts; success refreshes the summary, failure
  preserves unsaved drafts, closing without saving restores persisted values, and neither setting
  silently determines the other. The action remains visible in a fixed bottom row while one
  content region scrolls behind a bottom fade. App create/import/install remain workflow actions.
- Task Git access includes responsive supplementary help that accurately explains the injected Git
  credential helper, scoped broker lease, `gh` shim, executor inheritance, and launch/resume timing.
  Option descriptions match the dialog's explanatory typography.
- The desktop dialog is widened so the method cards and descriptions are readable without cramped
  columns. GitHub CLI is the first method card, and the task-access option gap is compact.
- Desktop and mobile preserve the same capability. The phone flow uses the existing full-height
  Drawer with one scroll owner, safe-area clearance, touch-reachable controls, and no horizontal
  overflow. Public GitHub integration docs point users to the new location.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/github/github-connection-dialog.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/integrations/github-workspace-settings.spec.ts -- --project=chromium --grep "configures task Git access from the workspace connection dialog"
cd apps/web && pnpm e2e:run tests/integrations/mobile-github-workspace-settings.spec.ts -- --project=mobile-chrome --grep "configures task Git access in the connection drawer"
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/components/github/github-settings.tsx`
- `apps/web/components/github/github-status.tsx`
- `apps/web/components/github/github-rate-limit.tsx`
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
  `docs/specs/ui/requirements/settings-manual-save.md`: dialog-level explicit submissions remain named immediate
  actions and do not use the route floating-save contributor.
- `apps/web/components/github/github-connection-dialog.tsx` as the existing desktop Dialog/mobile
  full-height Drawer composition.

## Mobile Design Contract

- **Outcome and entry point:** the Workspace GitHub access summary exposes the saved task mode;
  the same Change connection button opens configuration on desktop and mobile.
- **Exemplar:** reuse `GitHubConnectionDialog` itself for the shipped full-height mobile Drawer,
  including its fixed header, single `min-h-0` scroll body, and safe-area bottom padding. Reuse the
  responsive help-control pattern from `RepositoryScopeHelp` for fine-pointer tooltips and
  coarse-pointer Drawers.
- **Hierarchy and action:** GitHub CLI is the first workspace method, followed by PAT and App;
  task access comes second. One **Save changes** action stays fixed in the bottom row while content
  scrolls above a gradient cue. PAT/CLI and task access no longer compete with separate submission
  buttons. App create/import/install remain explicit external workflow actions.
- **Surface rationale:** both choices are infrequent workspace-level GitHub configuration, so one
  bounded dialog/full-height phone Drawer is more appropriate than a second page section or stacked
  overlay.
- **Shared versus responsive behavior:** share connection/task drafts, validation, persistence,
  footer, fade, and labels; only the existing Dialog/Drawer shell padding differs by viewport.
  The footer clears the mobile safe area. Help has the same content on every viewport, with 44px
  touch targets on mobile.
- **Proof:** desktop and `mobile-chrome` E2E prove CLI-first order, compact task-option spacing,
  fixed footer geometry before and after content scrolling, and the fade position; they then select
  a named CLI account and executor mode, save, and reopen to prove persistence.

## Risks

- The backend contracts remain independent, so one dialog submission may encounter a partial
  failure. Refresh persisted successes, keep failed drafts available for retry, and never report
  complete success unless every changed draft succeeds.
- Workspace settings are also read by repository-scope controls. Use partial updates and avoid
  introducing a competing whole-resource draft owner.
- App installation may navigate away from Kandev. The task-policy draft must remain explicitly
  unsaved unless its own submission succeeds.

## Output contract

Report the summary wording, dialog hierarchy, submission behavior, mobile geometry, RED/GREEN
component and E2E results, docs validation, files changed, risks, and task/plan status updates.

## Verification results

- `pnpm exec vitest run components/github` — passed (32 files, 289 tests). The focused dialog
  tests prove one action can save PAT and task-access drafts together and preserve a failed PAT
  draft after a partial save; that regression first failed RED with an empty token. The CLI-form
  test proves the preferred account becomes the draft without a separate **Use account** action.
- `pnpm --filter @kandev/web test -- --run components/github/github-status.test.tsx` — passed
  (3 tests) after a deliberate RED failure proved PAT/CLI still rendered the redundant personal
  identity section.
- `pnpm run typecheck`, focused ESLint for every changed frontend/E2E file, and the earlier full
  web lint — passed.
- A fresh `pnpm run build:vite` production build — passed.
- Chromium E2E for `configures task Git access from the workspace connection dialog` — passed
  after deliberate RED failures proved the missing summary/API-limit controls, old 638px width,
  separate **Use account** action, and PAT-first card order. It now verifies the identity,
  task-access and rate-limit tooltips, wider dialog, CLI-first order, typography parity, compact
  task-option spacing, a fixed footer and aligned fade before/after scrolling, one **Save changes**
  action, combined CLI/task persistence, and reopen state.
- Both mobile GitHub settings E2E files — passed (4 tests), including the nested credential-helper
  and API-limit Drawers, 44px controls, one scroll owner, no overflow, combined persistence, and
  safe parent-Drawer behavior. The focused production-build rerun first failed RED because no fixed
  footer existed, then passed with the safe-area footer, fade, CLI-first order, compact option gap,
  and unchanged footer position after scrolling.
- `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs` — passed (58 tests; 41 published docs pages).
- `git diff --check` — passed.
