---
id: "04-compatibility"
title: "Legacy compatibility evidence"
status: done
wave: 4
depends_on:
  - "03-status-actions"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ACTION-UX-001
  - REQ-PLUGINS-ACTION-UX-002
  - REQ-PLUGINS-ACTION-UX-003
acceptance_criteria:
  - AC-PLUGINS-ACTION-UX-001.1
  - AC-PLUGINS-ACTION-UX-001.2
  - AC-PLUGINS-ACTION-UX-001.3
  - AC-PLUGINS-ACTION-UX-001.4
  - AC-PLUGINS-ACTION-UX-001.5
  - AC-PLUGINS-ACTION-UX-003.2
  - AC-PLUGINS-ACTION-UX-003.5
system_design:
  - ../../specs/plugins/system-design/plugin-action-ux.md
---

# Task 04: Legacy compatibility evidence

## Summary

Prove the additive API against legacy controls and a host without the new exports.
Record the distinction between representative fixtures and actual released-plugin certification.

## In scope

- Add immutable, attributed legacy UI fixtures based on the audited source shapes: host Button, raw styled button, status contribution, and controlled disclosure.
- Keep fixture source unchanged while testing the upgraded host; do not convert legacy fixtures to Action.
- Test feature detection against an old-host stub and single-path registration.
- Add mixed-owner disable/re-enable, state isolation, saved status order, and null contribution cases.
- Extend the packaged fixture for mixed old/new browser controls and retain prior interaction scenarios.

## Out of scope

Live external accounts, plugin publication, unsupported-host certification, and generic full-suite verification.

## Acceptance

1. Legacy fixtures execute unchanged with preserved geometry, callbacks, context, and order on the new host.
2. Fallback code chooses exactly one control on hosts with and without Action; duplicate registration never occurs.
3. Mixed plugin cleanup and state isolation pass browser tests, including saved status ordering after reload.

## Verification

Run from the repository root. Use TDD for new behavior and record the initial
behavioral failure. New test paths in this order are files to create.
For a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run lib/plugins/action-compatibility.test.tsx lib/plugins/sdk-contract.test.ts components/plugins/plugin-slot.test.tsx components/plugins/mobile-plugin-nav-section.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/plugin-action-compatibility.spec.ts e2e/tests/plugins/composer-actions.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/plugins/mobile-composer-actions.spec.ts)
(cd apps/backend && go test ./cmd/plugin-fixture)
```

Run any other test file changed by this work order with the same targeted runner.
For rendered changes, inspect the focused desktop and phone screenshots against
the assigned previews. Do not infer CSS geometry from unit tests.

## Files likely touched

- New `apps/web/lib/plugins/action-compatibility.test.tsx`
- New attributed fixtures under `apps/web/lib/plugins/__fixtures__/action-compatibility/`
- `apps/web/lib/plugins/sdk-contract.test.ts`
- `apps/web/components/plugins/{plugin-slot.test.tsx,mobile-plugin-nav-section.test.tsx}`
- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`
- New `apps/web/e2e/tests/plugins/plugin-action-compatibility.spec.ts`

## Dependencies

03-status-actions.

## Risks

Source snapshots do not prove released package behavior. Preserve source attribution and report precisely which combinations were tested.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/plugin-action-ux.md)
- [System design](../../specs/plugins/system-design/plugin-action-ux.md)
- [Plan and source audit](plan.md)
- Existing plugin-slot, SDK consumer, and packaged plugin E2E patterns.

## Results

Complete. Attributed source-shaped legacy controls remain unchanged beside
Actions. The old-host stub selects one component path, while the packaged
fixture covers separate legacy/new state, disable/re-enable, and saved status
order after reload. Geometry checks preserve the existing 24px `icon-sm` host
Button and 28px raw metric button.

Validation passed:

- Focused plugin/slot Vitest: 9 files, 57 tests; the final Action contract and
  compatibility run passed 3 files, 14 tests.
- Managed Chromium E2E rebuilt the backend, Vite assets, and fixture package;
  the compatibility plus desktop composer suite passed 7/7 tests.
- Mobile composer E2E passed 3/3 tests.
- `(cd apps/backend && go test ./cmd/plugin-fixture)` passed.
- Desktop compatibility rendering was inspected; the temporary screenshot was
  removed after review. Phone composer workflows passed under Playwright.
- Fixture hashes, host base revision, exact limits, and commands are recorded
  in [the adoption map](plugin-adoption.md).

The initial compatibility component test lacked the app's `TooltipProvider`
in its harness. After adding the provider, its cases passed. The first browser
assertion expected 28px from the existing `icon-sm` Button; measurement showed
its established 24px size. The assertion now preserves that legacy geometry.
Neither failure required changing plugin behavior.

Review remediation (2026-09-25): compatibility cleanup now captures and
restores the full prior `system_metrics_display` value, including
`simplified`, and verifies the restored setting even when the test body fails.
The neighboring mobile Status drawer spec uses the same cleanup pattern. Its
hard-coded `/work/...` screenshot was removed. After remediation, the
Chromium action and compatibility tests passed 4/4, and the mobile action and
Status drawer tests passed 5/5; both managed runs rebuilt the host and fixture.
