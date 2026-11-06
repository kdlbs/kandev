---
id: "01-permission-frame"
title: "Correct notification permission framing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATIONS-001.17
  - AC-PLATFORM-NOTIFICATIONS-001.18
system_design:
  - ../../specs/platform/system-design/notification-permission-feedback.md
---

# Task 01: Correct Notification Permission Framing

## Summary

Remove the empty warning card in normal permission states.
Retain the existing warning card whenever the section displays permission feedback.

## In scope

- Derive the localized warning and frame together in `DesktopNotificationsSection`.
- Add component regressions for every permission state and warning transitions.
- Add focused rendered checks to the existing desktop and phone notification specs.

## Out of scope

- Shared settings components, button redesign, notification delivery, sound, and provider persistence.
- New copy, translation keys, permission prompts, or public API changes.

## Acceptance

1. The normal-state component regression fails before the fix and passes after it, with no warning card or padded warning body.
2. Each warning state retains its localized message, native/browser distinction, and existing header actions across transitions.
3. Desktop and phone rendered checks prove UI-01 and UI-02, with no horizontal overflow and captures of the result.

## ASCII UI preview

These excerpts use [UI-01 and UI-02 from the plan](plan.md#ascii-ui-preview).
They implement AC-PLATFORM-NOTIFICATIONS-001.17 and AC-PLATFORM-NOTIFICATIONS-001.18.

```text
UI-01, desktop normal:
Desktop Notifications                 [Enable] [Refresh] [Test]
Notify this device ...

Notification Sound

UI-01, phone normal:
Desktop Notifications
Notify this device ...
[Enable] [Refresh] [Test]

Notification Sound

UI-02, warning:
[Header and actions retain the viewport composition above.]
[Localized permission warning wraps inside its card.]
```

Normal states have no warning border or warning padding.
The page retains its scroll owner. Preview spacing is illustrative.

## Verification

Run from the repository root. If workspace dependencies are absent, first run `(cd apps && pnpm install --frozen-lockfile)`.
First add the normal-state regression and run the component suite to record its expected failure.
After the correction, run these checks sequentially:

```bash
(cd apps/web && pnpm exec vitest run components/settings/notification-permission-section.test.tsx components/settings/notifications-settings-actions.test.tsx components/settings/notification-test-delivery.test.tsx)
(cd apps/web && pnpm exec eslint components/settings/notification-permission-section.tsx components/settings/notification-permission-section.test.tsx e2e/tests/settings/notifications-type-scale.spec.ts e2e/tests/settings/mobile-notifications-type-scale.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/notifications-type-scale.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-notifications-type-scale.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E commands build fresh assets and clean up their isolated instances.
Do not overlap these runs. Record actual discovered test counts and final results.

## Files likely touched

- `apps/web/components/settings/notification-permission-section.tsx`
- `apps/web/components/settings/notification-permission-section.test.tsx`
- `apps/web/e2e/tests/settings/notifications-type-scale.spec.ts`
- `apps/web/e2e/tests/settings/mobile-notifications-type-scale.spec.ts`
- This work order and its plan for status and results.
- The linked design, promoted from `draft` to `current` after successful implementation.

## Dependencies

None.

## Risks

The permission stub must preserve the notification constructor and must not leak across browser contexts.
Native warning selection needs component mocks because browser E2E does not run inside Tauri.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/notifications.md), criteria 17 and 18.
- [Design](../../specs/platform/system-design/notification-permission-feedback.md).
- Existing permission component and action tests.
- Existing notification type-scale specs and screenshot fixtures.

## Results

The normal-state regressions were added first: the `default` and `granted` cases failed against the
existing empty card, then passed after `DesktopNotificationsSection` resolved one warning value and
selected `frame="card"` only when that value exists. The warning states, native/browser distinction,
and transitions remained green throughout.

The three notification component suites passed 54 tests, 15 of them in the permission file. A wider
notification regression run passed 79 tests across six files, and the existing desktop and phone
settings composition specs passed 8 and 6 tests. The desktop Chromium and `mobile-chrome` permission
specs passed 2 tests each with deterministic `Notification.permission` stubs. Captures show no
warning card or warning padding in the normal states, a wrapping browser-denial warning inside its
card after denial, and no warning again after refresh to granted. Action reachability, the stacked
phone header, the single settings scroll owner, and the absence of horizontal overflow are asserted.

Changed-file ESLint passed with zero problems, typecheck passed, `i18n:ratchet` passed, and the
specification catalog and linter checks passed.

Fixup addressed a current-head review finding that the normal path still rendered an empty content
element. That element's internal margin collapses today, so no visible gap was measurable, but the
section now hides the empty body instead of relying on margin collapsing. The component regression
asserts the hidden class and the browser check asserts computed `display: none` plus a normal 24px
header-to-next-section gap. The 54 component tests and both permission specs passed again.
