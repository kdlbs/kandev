---
id: "04-plugin-dialog"
title: "Contain plugin dialog content"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/dialog-content-containment.md"
design: "../../specs/ui/system-design/dialog-content-containment.md"
---

# Task 04: Contain Plugin Dialog Content

## Outcome

Opaque plugin content scrolls inside host dialog presentation while the
host-owned title and permitted close control remain visible on desktop and
phone.

## In scope

- Extend `PluginModalHost` component tests for dialog-body composition and
  unchanged dismissibility behavior.
- Extend the packaged E2E fixture with a long-content modal path that exercises
  the real `host.openModal` contract.
- Add desktop and mobile-Chrome geometry regressions for the dialog
  presentation.
- Put `PluginErrorBoundary` and plugin content inside one bounded scroll body;
  add stable host layout selectors.

## Exclusions

- Do not change `PluginModalOptions`, modal-manager behavior, or plugin APIs.
- Do not infer or extract a plugin-owned footer.
- Do not change `PluginDrawer`, presentation selection, or nondismissible
  Escape/outside-interaction guards.

## Applicable requirements

- `REQ-UI-DIALOG-CONTENT-CONTAINMENT-001`
- `AC-UI-DIALOG-CONTENT-CONTAINMENT-001.1`, `.2`, `.4` through `.9`

## Implementation acceptance

1. Long fixture content creates one plugin-dialog body scroll range while the
   outer dialog, host header, and dismissible close control stay in both
   viewports.
2. The final plugin-owned control is reachable and operable after scrolling.
3. Dismissible and nondismissible behavior, error isolation, short content, and
   drawer presentation remain unchanged; phone has no document overflow.

## TDD sequence

1. Add long plugin fixture content, stable selectors, and desktop/phone cases.
2. RED: record current outer overflow and unreachable final plugin control.
3. GREEN: cap `PluginDialog` and wrap the error boundary/content in the body.
4. Run fixture packaging as required, focused component/E2E, type, lint, i18n,
   and diff checks.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- components/plugins/plugin-modal-host.test.tsx)
(cd apps/web && pnpm e2e:run --host -- tests/plugins/plugins.spec.ts --grep "long plugin modal")
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/plugins/mobile-plugin-modal.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/plugins/plugin-modal-host.tsx components/plugins/plugin-modal-host.test.tsx e2e/tests/plugins/plugins.spec.ts e2e/tests/plugins/mobile-plugin-modal.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- `apps/web/components/plugins/plugin-modal-host.tsx`
- `apps/web/components/plugins/plugin-modal-host.test.tsx`
- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`
- `apps/web/e2e/tests/plugins/plugins.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-plugin-modal.spec.ts`
- `docs/plans/dialog-content-containment/plan.md`
- `docs/plans/dialog-content-containment/task-04-plugin-dialog.md`

## Dependencies

None.

## Parallelism

`sequential`. This work order owns one responsive RED-GREEN cycle and the
shared packaged plugin fixture.

## Output contract

Record RED/GREEN geometry, final plugin-control reachability, dismissibility
and drawer checks, focused results, files changed, cleanup, and residual risks.

## Results

- RED: Component coverage initially found no host dialog/body layout seams, and
  the packaged long-modal browser case found no bounded body for the opaque
  plugin content.
- GREEN: `PluginDialog` now has a dynamic cap and one scroll body containing the
  existing error boundary and plugin content. The host-owned title and
  dismissible close control remain outside it; nondismissible guards and the
  existing drawer are unchanged.
- The plugin host component suite, packaged fixture desktop Chromium case, and
  mobile-Chrome case passed. The final plugin action remains operable after
  scrolling and the modal can be dismissed when allowed.
- The fixture change is limited to long modal content and its completion
  control; no plugin API or SDK contract changed.
