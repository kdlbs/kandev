---
id: "01-restore-side-padding"
title: "Restore layout profile side padding"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-COMPOSITION-001
  - REQ-UI-SETTINGS-COMPOSITION-004
acceptance_criteria:
  - AC-UI-SETTINGS-COMPOSITION-001.1
  - AC-UI-SETTINGS-COMPOSITION-004.2
  - AC-UI-SETTINGS-COMPOSITION-004.4
system_design:
  - ../../specs/ui/system-design/settings-composition.md
---

# Task 01: Restore side padding

## Summary

Retain the shared horizontal padding around the Layout profiles content.
Replace its `p-0` override with `py-0`, retaining `divide-y-0`.

## In scope

- Correct the profiles group's class override in `layout-settings.tsx`.
- Measure the rendered inset before and after the change.
- Check desktop, phone, and the existing grid breakpoint for clipping.

## Out of scope

Shared component changes, Sidebar-tab changes, new copy, editor commands, and persisted layout changes.

## Acceptance

1. Both content edges use the shared `px-4` inset. Existing vertical spacing remains intact.
2. Desktop retains its list/editor columns. Phone retains its single-column flow with reachable controls and no horizontal page overflow.
3. Existing focused desktop and mobile layout-profile tests pass against a fresh build.

## UI preview

See the [full preview](plan.md#ui-preview).
UI-01, desktop: equal outer insets surround the profile list, existing column gap, and selected editor.
UI-02, phone: equal outer insets surround the profile actions/list followed by the selected editor.
The page owns scrolling. Existing controls and localized labels retain their roles.
These views map to the three acceptance criteria listed in frontmatter.

## Verification

Run from the repository root. Install once before the first package command.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec eslint components/settings/layouts/layout-settings.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/layout-profiles.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-layout-profiles.spec.ts)
git diff --check
```

Run E2E commands sequentially. The first managed run rebuilds the application.
Use a disposable browser capture in the isolated E2E runtime to measure both insets and inspect screenshots.
Measure before and after the correction at desktop and the configured Pixel 5 viewport.
Check 1023px and 1024px widths around the existing grid breakpoint.
Compare the list and editor bounds with the card's inner bounds, accounting for its border and root-font scaling.
Remove disposable capture code after recording evidence. Do not add a unit test that asserts class strings.

## Files likely touched

- `apps/web/components/settings/layouts/layout-settings.tsx`
- This work order and `plan.md` for implementation status and evidence.

## Dependencies

None. The user authorized implementation on 2026-10-22.

## Risks

Reduced editor width can expose clipped controls on phones. Rendered bounds and existing mobile flows must pass.

## Parallelism

`sequential`

## Inputs

- [Settings composition requirements](../../specs/ui/requirements/settings-composition.md).
- [Settings composition design](../../specs/ui/system-design/settings-composition.md), Shared presentation and rollout.
- `apps/web/components/settings/settings-group.tsx` and `apps/packages/ui/src/card.tsx`.
- `apps/web/e2e/pages/layout-settings-page.ts` and the existing layout-profile suites.

## Results

Completed on 2026-10-22. The profiles group now uses `py-0 divide-y-0` and retains the shared horizontal inset.

- `pnpm install --frozen-lockfile` from `apps/`: passed.
- `pnpm exec eslint components/settings/layouts/layout-settings.tsx` from `apps/web`: passed.
- Temporary browser checks failed before the correction on desktop and phone: both side insets measured 0px instead of 16px.
- `python3 /tmp/run-layout-padding-check.py pnpm --dir apps/web e2e:run --project chromium tests/settings/layout-profiles.spec.ts tests/settings/layout-padding-capture.spec.ts`: passed, 8 tests (7 existing scenarios and 1 temporary capture).
- `python3 /tmp/run-layout-padding-check.py pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/settings/mobile-layout-profiles.spec.ts tests/settings/mobile-layout-padding-capture.spec.ts`: passed, 6 tests (5 existing scenarios and 1 temporary capture).
- Both insets measured 16px at viewport widths 393, 1023, 1024, and 1440px. Document horizontal overflow measured 0px at every width.
- Desktop and phone screenshots were inspected, including the phone editor controls. The rendered structure matches UI-01 and UI-02.
- `git diff --check`: passed. Untracked plan files also passed a whitespace check.
- PR publication capture: `CAPTURE_PR_ASSETS=1 python3 /tmp/run-layout-padding-check.py pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-layout-padding-pr-capture.spec.ts`: passed, 1 disposable scenario with desktop, phone, and phone-editor assets.
- Temporary capture specs were removed. No permanent tests were added for this class-only correction.

The local runner helper supplied `KANDEV_SERVER_HOST=127.0.0.1` and NixOS Chromium library paths through `LD_LIBRARY_PATH`.
The first setup attempt failed because the backend bound to a network interface. The second lacked Chromium shared libraries.
Both setup issues were resolved before the recorded browser measurements. The helper and logs are temporary validation artifacts, not product changes.

Evidence remains in `/tmp/layout-padding-desktop.log`, `/tmp/layout-padding-mobile.log`, and `/tmp/layout-padding-after-*.png`.
The existing requirements remain active and the settings-composition design remains current.
