---
id: "01-flatten-settings"
title: "Flatten plugin settings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CONTROL-SIZING-001
acceptance_criteria:
  - AC-UI-CONTROL-SIZING-001.1
  - AC-UI-CONTROL-SIZING-001.3
  - AC-UI-CONTROL-SIZING-001.4
  - AC-UI-CONTROL-SIZING-001.5
  - AC-UI-CONTROL-SIZING-001.6
  - AC-UI-CONTROL-SIZING-001.7
  - AC-UI-CONTROL-SIZING-001.8
system_design:
  - ../../specs/ui/system-design/control-sizing.md
---

# Task 01: Flatten plugin settings

## Summary and scope

Remove redundant frames, place Install beside the heading, keep secondary
actions compact, and render installed plugins as a divided list. Own the host
settings layout, focused browser regressions, documentation, and PR screenshots.
Preserve every existing plugin action and permission boundary. API, SDK,
persistence, and plugin implementation changes are out of scope.

## Acceptance

- Install sits beside the heading; Sync and update checks share a compact row
  on phones and join the heading on wider screens. Controls stay touch-safe.
- Automatic updates and plugin rows are no longer enclosed in nested cards;
  names, versions, descriptions, state, and all actions remain available.
- Existing install, check, disable, settings-navigation, and uninstall flows
  pass on mobile and desktop, with no horizontal overflow.

## ASCII UI preview

UI-01, excerpt of the [combined preview](plan.md#ascii-ui-preview):

```text
Phone                                  Desktop
Installed plugins [Install plugin]     Installed plugins [Install plugin]  Sync  Check updates
Sync  Check for updates                Automatic updates                             [switch]
Automatic updates       [switch]       -----------------------------------------------------
--------------------------------      Plugin name / state / version / controls
Plugin name / state / controls         -----------------------------------------------------
--------------------------------
```

The existing settings surface owns vertical scrolling. Keep 44px phone/coarse
targets and 28px desktop toolbar controls per the listed acceptance criteria.

## Verification

```bash
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/settings/mobile-plugin-updates.spec.ts tests/plugins/mobile-plugin-settings-row.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/plugins/plugins.spec.ts --retries=0)
(cd apps/web && pnpm exec vitest run app/settings/plugins/page.test.tsx components/settings/plugins/plugin-row.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/settings/plugins/plugins-settings.tsx components/settings/plugins/plugin-row.tsx e2e/tests/settings/mobile-plugin-updates.spec.ts e2e/tests/plugins/plugins.spec.ts --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/settings/plugins/plugins-settings.tsx`
- `apps/web/components/settings/plugins/plugin-row.tsx`
- `apps/web/e2e/tests/settings/mobile-plugin-updates.spec.ts`
- `apps/web/e2e/tests/plugins/plugins.spec.ts`
- `docs/public/plugins.md`
- `docs/screenshots/plugin-settings.png`

## Dependencies and parallelism

None. Sequential execution in the primary session.

## Inputs and risks

Read the linked control-sizing design, existing settings primitives, and plugin
E2E fixtures. Keep overlay links below inline controls; check translated label
wrapping and plugin IDs after removing padding.

## Results

Completed on 2026-09-24. All commands in Verification passed:

- Mobile Playwright: 2 passed, including 320px, 767px, and 768px containment and
  touch-control checks in addition to the canonical phone viewport.
- Desktop Playwright: 17 passed, including 28px toolbar geometry, keyboard
  activation, installation, synchronization, updates, persistence, and uninstall.
- Focused Vitest: 54 passed. The existing version assertion now checks the row's
  combined text because ID and version occupy separate wrapping spans.
- Typecheck, focused ESLint, and all localization checks passed.
- Public docs: 47 published pages validated. Catalog validation checked 304
  decisions and 1138 specifications; all specification files passed lint.
- Documentation coverage validator: covered; no reference errors.
- `git diff --check` passed.

RED evidence: the original mobile toolbar failed the same-row assertion with
Sync at y=314.5 and Check for updates at y=366.5. The final page preserves both
44px touch targets while placing these actions side by side.

A disposable capture spec passed against the production build with isolated,
synthetic plugin records. Four compressed screenshots show Installed and Browse
on phone and desktop; PR comparison assets are published on a separate media
branch. The rendered phone composition matches UI-01. The public plugin guide
describes the compact toolbar and flat list and includes the new desktop
Installed screenshot. Its duplicate illustration of the previous layout was
removed during PR review.
