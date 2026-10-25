---
id: "02-marketplace-sources"
title: "Contain Marketplace sources"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/dialog-content-containment.md"
design: "../../specs/ui/system-design/dialog-content-containment.md"
---

# Task 02: Contain Marketplace Sources

## Outcome

A long Marketplace source list scrolls inside its dialog while the title,
close control, and add-source form remain visible on desktop and phone.

## In scope

- Add focused component coverage for local row composition and unchanged source
  actions.
- Add desktop and mobile-Chrome browser regressions by intercepting the existing
  marketplace catalog response with many real `MarketplaceSource` shapes.
- Make the source list the scroll body and keep `AddSourceForm` in the final
  persistent row.
- Add stable dialog and source-list selectors; keep source row controls
  reachable and touch-safe.

## Exclusions

- Do not change marketplace APIs, reload behavior, validation, or source order.
- Do not change add, toggle, or remove semantics.
- Do not change the shared Dialog primitive.

## Applicable requirements

- `REQ-UI-DIALOG-CONTENT-CONTAINMENT-001`
- `AC-UI-DIALOG-CONTENT-CONTAINMENT-001.1` through
  `AC-UI-DIALOG-CONTENT-CONTAINMENT-001.8`

## Implementation acceptance

1. Many sources create one source-list scroll range while the dialog, header,
   close control, and add form remain within both viewports.
2. The final source and its controls are reachable; the add form remains usable
   before and after scrolling.
3. Short lists, mutation callbacks, source order, and dismissal remain
   unchanged, with no phone document overflow.

## TDD sequence

1. Add the long catalog fixtures, stable selectors, and desktop/phone cases.
2. RED: capture the current oversized dialog and displaced add form.
3. GREEN: apply the capped three-row composition and list overflow boundary.
4. Run focused component, E2E, type, lint, i18n, and diff checks.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- components/settings/plugins/marketplace-sources-dialog.test.tsx)
(cd apps/web && pnpm e2e:run --host -- tests/settings/plugin-marketplace-sources.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/settings/mobile-plugin-marketplace-sources.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/plugins/marketplace-sources-dialog.tsx components/settings/plugins/marketplace-sources-dialog.test.tsx e2e/tests/settings/plugin-marketplace-sources.spec.ts e2e/tests/settings/mobile-plugin-marketplace-sources.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- `apps/web/components/settings/plugins/marketplace-sources-dialog.tsx`
- `apps/web/components/settings/plugins/marketplace-sources-dialog.test.tsx`
- `apps/web/e2e/tests/settings/plugin-marketplace-sources.spec.ts`
- `apps/web/e2e/tests/settings/mobile-plugin-marketplace-sources.spec.ts`
- `docs/plans/dialog-content-containment/plan.md`
- `docs/plans/dialog-content-containment/task-02-marketplace-sources.md`

## Dependencies

None.

## Parallelism

`sequential`. This work order owns one responsive RED-GREEN cycle.

## Output contract

Record RED/GREEN geometry, source and add-form reachability, focused results,
files changed, cleanup, blockers, and residual risks.

## Results

- RED: The long catalog cases showed the source list growing the dialog beyond
  the available viewport before the local containment boundary existed.
- GREEN: Source rows now occupy one capped scrolling body. The title, close
  control, and Add Source form remain outside that body, and source toggles and
  remove actions keep their existing callbacks with phone-sized controls.
- The focused component test, desktop Chromium case, and mobile-Chrome case
  passed, including final-source reachability and add-form availability after
  scrolling.
- Marketplace loading, mutation, ordering, validation, and dismissal behavior
  remain unchanged.
