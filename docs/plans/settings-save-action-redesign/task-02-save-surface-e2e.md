---
id: "02-save-surface-e2e"
title: "Rendered desktop and mobile coverage"
status: done
wave: 2
depends_on: ["01-centered-save-surface"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-manual-save.md"
---

# Task 02: Rendered desktop and mobile coverage

Update the existing settings manual-save and Configuration Chat Playwright
coverage to prove the new centered surface in real desktop and phone viewports.
Keep tests focused on user outcomes and retain each test's existing API cleanup.

## Acceptance

1. Desktop coverage proves the surface is centered in the settings content pane,
   uses compact geometry, exposes Reset and Save changes, Reset leaves the
   persisted setting unchanged, and Save still persists it.
2. Mobile coverage proves the 390px surface fits within safe-area-aware bounds,
   has touch-sized controls, keeps the last editable control reachable, and
   creates no document-level horizontal overflow.
3. Desktop and mobile Configuration Chat coverage proves the hosted surface is
   above and non-intersecting with the open popover while remaining centered in
   its available host width.

## Verification

- If workspace dependencies are absent: `cd apps && pnpm install --frozen-lockfile`.
- Build fresh frontend assets before E2E: `make build-web`.
- `cd apps/web && pnpm e2e:run tests/settings/settings-manual-save.spec.ts tests/settings/config-chat-popover.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/settings/mobile-general-settings.spec.ts tests/settings/mobile-config-chat-popover.spec.ts`
- `git diff --check`

## Files likely touched

- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
- `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`
- `apps/web/e2e/tests/settings/config-chat-popover.spec.ts`
- `apps/web/e2e/tests/settings/mobile-config-chat-popover.spec.ts`

## Dependencies

Task 01 must be complete so the new Reset action and host geometry exist.

## Parallelism

Sequential after Task 01. The four specs share the same selectors and rendered
surface contract.

## Inputs

- `docs/specs/ui/requirements/settings-manual-save.md` — centered surface, Reset, and mobile
  scenarios.
- `docs/plans/settings-save-action-redesign/plan.md` — E2E Tests and Mobile
  parity sections.
- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts` — existing API
  persistence and cleanup patterns.
- `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts` — existing
  safe-area, final-control, and overflow assertions.
- `apps/web/e2e/tests/settings/config-chat-popover.spec.ts` and
  `mobile-config-chat-popover.spec.ts` — existing above-popover assertions.

## Output contract

Report the exact specs and assertions changed, commands and outcomes, screenshot
artifact names, API cleanup evidence, blockers, and synchronized Task 02/plan
status in the same conversation. Rebuild after frontend changes; do not report
stale-build E2E results.

## Results

- Added desktop assertions for centered geometry, neutral wrapper/green primary
  treatment, local Reset behavior, and persisted Save behavior.
- Added mobile assertions for safe viewport bounds, touch-sized Reset/Save
  controls, last-control clearance, and no document-level horizontal overflow.
- Added desktop and mobile Configuration Chat host-centering assertions while
  retaining the above-popover collision checks.
- `pnpm e2e:run --host --no-build tests/settings/settings-manual-save.spec.ts tests/settings/config-chat-popover.spec.ts` — 11 passed.
- `pnpm e2e:run --project mobile-chrome --no-build tests/settings/mobile-general-settings.spec.ts tests/settings/mobile-config-chat-popover.spec.ts` — 6 passed.
- `CAPTURE_PR_ASSETS=true pnpm e2e:raw` with all four settings specs — 17 passed; refreshed desktop/mobile screenshots are in the local PR asset manifest and are not source-controlled.
- `git diff --check` — passed.
