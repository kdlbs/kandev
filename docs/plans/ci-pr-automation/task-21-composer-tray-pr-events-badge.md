---
id: "21-composer-tray-pr-events-badge"
title: "Composer tray PR events badge"
status: done
wave: 13
depends_on:
  - "20-role-aware-automation-controls"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 21: Composer tray PR events badge

## Intent

Show active review-requested, merged, and closed prompt automations in the PR
status chip above the chat input without making the composer tray overflow on
phones or narrow windows.

## Acceptance

- The single- and multi-PR status chips render one translated `PR events N/3`
  badge when one or more lifecycle prompt options are enabled, render none when
  all are disabled, retain the existing auto-fix/auto-merge badges, and name
  each enabled lifecycle event in the chip's accessible description.
- The grouped badge remains part of the existing PR-chip tap/hover target; the
  popover/drawer still exposes the independent `PR events` switches and
  no new automation overlay or stored state is introduced.
- With all five automations and other composer-tray controls present, the tray
  keeps complete controls visible or wraps them without document-level
  horizontal overflow on desktop and the configured phone viewport.

## Verification

Bootstrap a fresh worktree before the first pnpm command when dependencies are
absent:

```bash
cd apps && pnpm install --frozen-lockfile
```

Follow Red-Green-Refactor: add the focused component and browser assertions,
run them to observe the missing grouped badge/containment behavior, implement
the UI, then rerun the same commands:

```bash
cd apps && pnpm --filter @kandev/web test -- pr-status-chip pr-status-automation-badges
cd apps && pnpm --filter @kandev/web i18n:check
cd apps/web && node --max-old-space-size=4096 node_modules/typescript/bin/tsc --noEmit
cd apps/web && pnpm e2e:run tests/pr/ci-automation-options.spec.ts -- --grep "composer tray"
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-pr-ci-chip.spec.ts -- --grep "PR event automations"
```

Confirm Playwright discovers both intended scenarios. The managed E2E runner
must run the desktop scenario first to rebuild the production frontend. After
that successful build, the mobile scenario may use `--no-build` to reuse the
verified production build.

## Files likely touched

- `apps/web/components/github/pr-status-chip.tsx`
- `apps/web/components/github/pr-status-automation-badges.tsx`
- `apps/web/components/github/pr-status-automation-badges.test.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/src/locales/en/github.json`
- `apps/web/src/locales/pseudo/github.json`
- `apps/web/e2e/tests/pr/ci-automation-options.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts`

## Dependencies

- Task 20's shared `PR events` controls and task-wide lifecycle
  booleans.
- Existing `PRStatusChipDrawer`, `PRCIPopover`, and `MultiPRCIPopover`
  presentation paths.

## Parallelism

Sequential. Component derivation, translated copy, status-row geometry, and
the two E2E scenarios share the same user-visible contract and should land as
one focused vertical slice.

## Inputs

- Spec: `What`, the lifecycle badge and narrow-tray `Scenarios`, and `Out of
scope` in `../../specs/ui/requirements/ci-pr-automation.md`.
- Plan: `Composer tray automation summary`, `Tests`, `E2E Tests`, and `Risks`
  in `plan.md`.
- Patterns: `AutomationFlagBadges`, `automationForPR`, and
  `automationForPRs` in
  `apps/web/components/github/pr-status-automation-badges.tsx`; the
  single/multi drawer branches in `apps/web/components/github/pr-status-chip.tsx`;
  `ChatStatusBar` in
  `apps/web/components/task/chat/chat-input-area.tsx`; existing PR-chip page
  object methods in `apps/web/e2e/pages/session-page.ts`.
- Mobile exemplar: the existing `PRStatusChipDrawer`; preserve its inset
  bottom-drawer composition and single `min-h-0 overflow-y-auto` body.

## Risks

- Lifecycle options are task-wide; the summary must not vary with the selected
  PR or multiply for multi-PR tasks.
- `PR events N/3` is deliberately compact. The accessible description must
  preserve the exact enabled-event information that the visual count omits.
- Status-row wrapping must keep the PR chip internally intact, preserve the
  right-control group, and avoid a second horizontal or vertical scroll owner.
- New visible and accessible copy must use i18n keys and remain reactive to
  locale changes; do not call `t()` at module scope.

## Output contract

Report the implementation summary, files changed, exact component/i18n/typecheck
and E2E commands with results, Playwright test counts and artifact paths, any
cleanup performed, blockers, and residual risks. Reconcile this file's likely
files with the actual diff, set `status: done`, replace `## Results`, and update
the Wave 13 checkbox plus `Verification Results` in `plan.md` in the same
conversation.

## Results

- Added one violet `PR events N/3` badge derived from the three task-wide
  lifecycle switches. Single- and multi-PR chips share the same derivation,
  retain the independent auto-fix/auto-merge badges, and expose translated
  accessible descriptions that enumerate the enabled lifecycle events.
- Kept the existing PR chip as the only hover/tap target. Desktop still opens
  the existing popover and touch still opens `PRStatusChipDrawer` with all five
  switches independently reachable.
- Made `ChatStatusBar` wrap complete controls and made the PR chip non-shrinking
  and non-breaking. The compact desktop and configured Pixel 5 scenarios assert
  containment, a real center-point hit target, and zero document overflow.
- Red evidence: the component suite failed on the missing PR events badge; both
  new browser scenarios failed because the tray computed to `flex-wrap:
nowrap`.
- Green evidence: 47 focused Vitest tests passed in 3 files; i18n check and
  ratchet passed; targeted ESLint passed with zero findings; isolated TypeScript
  checking passed with a 4 GB heap; 1 compact desktop Chromium test and 1 Pixel
  5 `mobile-chrome` test passed against the final production build.
- The host lacked NSS/NSPR and Docker, so the authorized E2E run unpacked
  `libnspr4` and `libnss3` into a task-local `/tmp` runtime. No host packages
  were changed. The runtime and ignored screenshot artifacts were removed after
  verification.
- Residual risk: the existing advisory locale backlog remains outside this task;
  the grouped badge and responsive tray behavior passed the recorded checks.
