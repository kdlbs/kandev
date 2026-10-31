---
created: 2026-10-30
status: implemented
requirements:
  - REQ-PLATFORM-NOTIFICATIONS-001
system_design:
  - ../../specs/platform/system-design/notification-permission-feedback.md
legacy_specs: []
---

# Implementation Plan: Notification Permission Layout

## Overview

Remove the empty warning bar below Desktop Notifications while retaining permission feedback.
One work order delivers the component correction and its desktop and phone coverage.
Platform owns the contract because the warning follows notification permission state.

## Evidence and assumptions

The supplied screenshots show an empty bordered bar, including a highlighted 16px inner container.
`DesktopNotificationsSection` supplies only conditional warning paragraphs as its body.
For `default` and `granted`, each paragraph is absent.
`SettingsGroupContent` still creates its default card and inner `py-2` container.
This source trace explains the screenshot without an instance mutation or temporary instrumentation.

The confirmed repair scope is the empty notification warning surface.
The existing labels and controls remain the interaction model.
No material product question blocks this package.

## Scope

- Derive the warning and frame together inside `DesktopNotificationsSection`.
- Preserve localized feedback for native/browser denial, unsupported APIs, and failed permission checks.
- Cover state transitions and rendered desktop and phone layouts.

Backend delivery, sound preferences, providers, global settings primitives, and button redesign are outside this repair.

## Technical approach

Use the existing `SettingsGroup` frame property as described in the [design](../../specs/platform/system-design/notification-permission-feedback.md).
Derive one translated warning value and reuse its presence to select the frame, hide the empty
content element, and render the paragraph.
Do not change `SettingsGroup`, introduce another state variable, or add a child-inspection utility.
Retain the section's discovery target and header actions.

## ASCII UI preview

UI-01: Settings > Notifications, normal permission state.
The current empty bar is supported by both the screenshots and source.

```text
Before, desktop:
Desktop Notifications                 [Enable] [Refresh] [Test]
Notify this device ...
[                     empty warning card                     ]

After, desktop:
Desktop Notifications                 [Enable] [Refresh] [Test]
Notify this device ...

Notification Sound

After, phone:
Desktop Notifications
Notify this device ...
[Enable] [Refresh] [Test]

Notification Sound
```

UI-02: Denied, unsupported, or failed permission check, both viewports.

```text
Desktop Notifications                 [actions on desktop]
Notify this device ...
[actions on phone]
[ Localized warning wraps within the available width. ]

Notification Sound
```

No empty card or reserved warning padding is structural, under AC-PLATFORM-NOTIFICATIONS-001.17.
Visible state-specific feedback and phone containment are structural, under AC-PLATFORM-NOTIFICATIONS-001.18.
The page scrolls as before. Spacing and abbreviated labels in the previews are illustrative.

## Tests

Extend `apps/web/components/settings/notification-permission-section.test.tsx` with these cases:

- `omits the warning card when permission is default or granted`: AC-PLATFORM-NOTIFICATIONS-001.17.
- `shows the corresponding warning for denied, unsupported, and error states`: AC-PLATFORM-NOTIFICATIONS-001.18.
- `updates the warning frame when permission changes`: AC-PLATFORM-NOTIFICATIONS-001.18.
- Existing action suites continue to prove permission, refresh, and test handlers: AC-PLATFORM-NOTIFICATIONS-001.17.

The first case must fail against the current component because the card exists.

## E2E tests

Add `permission feedback has no empty frame and follows refresh` to both existing notification type-scale specs.
The desktop file uses `chromium`; the `mobile-` file uses `mobile-chrome`.
Both cover normal permission, denied feedback, refresh to granted, zero normal-body padding, a normal inter-section gap, and viewport containment.
The phone case preserves the existing stacked header and one page scroll owner.
Use existing screenshot capture fixtures to compare UI-01 and UI-02 against the actual render.

## Work orders

- [x] [Task 01: Correct notification permission framing](task-01-permission-frame.md) (`done`, sequential, no dependencies).

## Verification results

- New component regressions failed against the empty card and pass after the correction: 15 tests in the permission file, 54 across the three notification suites, and 79 across a wider six-file notification run.
- Desktop Chromium and `mobile-chrome` notification permission specs passed 2 tests each; the existing settings composition specs passed 8 desktop and 6 phone tests.
- Captures confirm UI-01 (no warning card or padding) and UI-02 (wrapping warning card) on both viewports.
- Changed-file ESLint passed with zero problems, `pnpm run typecheck` passed, and `pnpm run i18n:ratchet` passed.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

Fixup applied a current-head review finding: the undecided/granted path still rendered an empty content element inside `SettingsGroup`, whose internal spacing is margin-collapsed today but remains fragile. The section now hides that empty body, and the component and browser suites assert `display: none` plus a normal 24px header-to-next-section gap. The 54 component tests and both desktop and phone permission specs were rerun after the change and passed.

Product checks were recorded against the implemented component and the rendered E2E captures.

## Risks

- Removing the card unconditionally can hide warning styling. Derive the frame from the same warning that supplies its content.
- Ambient browser permission differs by environment. Browser tests must set and restore a deterministic permission stub.
- A global empty-child fix can alter unrelated settings. Keep the correction in the notification section.
