---
created: 2026-11-03
status: completed
requirements:
  - REQ-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001
system_design:
  - ../../specs/platform/system-design/notification-settings-layout.md
legacy_specs: []
---

# Implementation plan: Notification settings layout

## Overview

Replace repeated introductions with one page description and two compact device-preference rows.
One sequential work order delivers the layout, translations, and regression coverage together.
The user accepted the desktop and phone ASCII proposal on 2026-11-03.
Implementation completed on 2026-11-03; the delivered layout follows the accepted previews.

## Scope

### In scope

- One page introduction, compact permission controls, and sound controls before Push notifications.
- Stable sound controls in enabled and disabled states.
- Intentional desktop and phone layouts with accessible controls and retained drafts.
- All supported translations, targeted component tests, and browser evidence.

### Out of scope

- Notification delivery, provider persistence, event subscriptions, and sound playback policy.
- Push, external-provider, webhook, global settings, and navigation redesigns.
- Additional settings state owners or save actions.

## Technical approach

`NotificationsSettings` keeps `SettingsPageTemplate`, updates its translated description, and reorders the existing sound component before push.
`DesktopNotificationsSection` retains permission state decisions and action handlers.
`NotificationSoundSection` retains its contributor and renders disabled controls instead of conditionally mounting them.
A notification-local row component can share labels, borders, dirty feedback, and discovery registration between the two preferences.
Reuse shared typography and control primitives without changing their global layout.
Keep `GENERAL_SETTINGS_TARGETS.desktopNotifications` and `.notificationSound` attached to the respective visible rows.

The [system design](../../specs/platform/system-design/notification-settings-layout.md) defines state and mobile boundaries.
[ADR 0046](../../decisions/0046-settings-route-save-coordinator.md) retains Save coordination authority.
The [permission-layout plan](../notification-permission-layout/plan.md) remains historical delivery evidence for warning behavior.
The [draft-preservation plan](../notification-save-drafts/plan.md) remains evidence for provider reconciliation.
This package changes their layout-dependent assertions, without rewriting their recorded results.

## ASCII UI preview

Entry point for all views: Settings > Preferences > Notifications.
The existing route alias `/settings/general/notifications` remains supported.

### UI-00: Current affected regions

Source: `notification-permission-section.tsx` and `notification-sound-section.tsx`.

```text
Notifications
Configure providers and choose which events should alert you.

Desktop Notifications                       [Enable] [Refresh] [Test]
Notify this device when a selected agent turn, question, or Office event occurs.

Push Notifications ...

Notification Sound
Play a sound on this device when a selected agent turn, question, or Office event occurs.
+---------------------------------------------------------------------+
| [ON]                                                                |
| [Plim v] [Play]                                                     |
+---------------------------------------------------------------------+
```

### UI-01: Desktop, sound enabled

```text
Notifications
Choose how this device alerts you to agent turns, questions, and Office events.

+---------------------------------------------------------------------+
| Desktop notifications                    [Enable] [Refresh] [Test]  |
+---------------------------------------------------------------------+
| Notification sound                [ON]  [Plim v]  [Play]             |
+---------------------------------------------------------------------+

Push notifications
Receive alerts even when Kandev is closed.
...
External providers ...
Webhooks ...
Notification events ...
```

### UI-02: Desktop, sound disabled and permission denied

```text
+---------------------------------------------------------------------+
| Desktop notifications                    [Enable] [Refresh] [Test]  |
| Notifications are blocked. Enable them in your browser site settings.|
+---------------------------------------------------------------------+
| Notification sound                [OFF] [Plim v]  [Play]             |
+---------------------------------------------------------------------+
                                         disabled  disabled
```

The warning is conditional and uses the existing localized browser or OS message.
Normal permission states contain no warning element or reserved warning space.
Both sound states have the same control layout and row height for unchanged text.

### UI-03: Phone, sound enabled

```text
Notifications
Choose how this device alerts you to
agent turns, questions, and Office events.

+--------------------------------------+
| Desktop notifications                |
| [Enable]  [Refresh]  [Test]           |
+--------------------------------------+
| Notification sound            [ON]   |
| [Plim                       v] [Play]|
+--------------------------------------+

Push notifications ...
```

### UI-04: Phone, sound disabled and permission denied

```text
+--------------------------------------+
| Desktop notifications                |
| [Enable]  [Refresh]  [Test]           |
| Notifications are blocked. Enable    |
| them in your browser site settings.  |
+--------------------------------------+
| Notification sound            [OFF]  |
| [Plim                       v] [Play]|
+--------------------------------------+
  selector and preview disabled
```

Control order, row grouping, stable disabled controls, and separate phone composition are required.
Borders, ASCII spacing, capitalization, and example subtitle wrapping are illustrative.
Refresh, test, and preview can retain existing icons with translated accessible names.
Keep existing provider sections below these rows; their shortened descriptions here are illustrative.
The page content scrolls within `settings-scroll-container`; the existing Save surface retains its fixed position and safe-area handling.
Phone and coarse-pointer controls need 44px active targets; ordinary fine-pointer desktop controls remain 28px.

UI-01 and UI-02 map to AC .1-.3 and .6.
UI-03 and UI-04 map to AC .3-.6.
The browser scenarios below provide rendered evidence.

## Tests

All criterion suffixes refer to `AC-PLATFORM-NOTIFICATION-SETTINGS-LAYOUT-001`.

- `.1`, `.2`, `.6`: `notifications-settings.test.tsx`, proposed case `shows one page introduction and compact device rows before push`.
- `.3`: `notification-sound-section.test.tsx`, proposed cases `keeps preset and preview disabled while sound is off` and `preserves the preset across toggles`.
- `.3`, `.5`: Extend the existing sound Save tests with preset selection, Reset, and disabled preview assertions.
- `.6`: `notification-permission-section.test.tsx`, retain native/browser/error cases and refresh/action callbacks while replacing obsolete markup assertions.
- `.6`: Retain target IDs and use the existing search-target registration. Assert both target roots remain registered after the layout change.

## E2E tests

Extend the existing notification specs rather than create another suite.

- `notifications-type-scale.spec.ts`, project `chromium`: one introduction; compact desktop alignment; enabled/disabled sound geometry; preview and Save/reload.
- `mobile-notifications-type-scale.spec.ts`, project `mobile-chrome`: the two-row sound layout; 44px hit targets; selector containment; Reset; no horizontal overflow.
- In the desktop spec, add narrow fine-pointer coverage at 767px and desktop coverage at 768px.
  Assert the phone composition and touch target sizes at 767px, even with a fine pointer.
- Both specs: refresh denied permission back to granted and retain warning readability.
- Both specs: retain a sound draft across desktop/phone resizing, then Save and reload.
- Keep current provider draft tests as regressions because sound moves within the same save route.

Use semantic controls and existing permission stubs. Do not open an OS permission prompt.
Update `notifications-type-scale-helpers.ts` to assert warning behavior without obsolete empty-body markup.
Measure bounding boxes and computed sizes; class names alone cannot prove alignment or touch geometry.
Capture UI-01 through UI-04 with `prCapture` and compare their structure with these previews.
Browser checks use a fresh managed build. Do not overlap runners or override their worker budget.

## Work orders

- [x] [Task 01: Compact notification preferences](task-01-compact-notification-preferences.md), wave 1, sequential, done.

## Verification results

Design validation completed on 2026-11-03:

- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/list-docs.py specs --system platform --text notification-settings-layout --format paths`: found both new specs.
- `git diff --check -- docs/specs docs/plans/notification-settings-layout`: passed.
- `.github/scripts/pr-docs.cjs` local `validateCoverage` preflight: passed with status `covered`.
  Input included the four new artifacts and the prospective `notifications-settings.tsx` source path.
  The actual documentation-only change is exempt; the prospective input validates the planned implementation's reference chain without editing source.

Implementation validation completed on 2026-11-03:

- Targeted component suites: 4 files, 47 tests passed.
- Web typecheck, ESLint, `i18n:check`, and `i18n:ratchet`: passed.
- Desktop notification type-scale E2E: 4 tests passed.
- Mobile notification type-scale E2E: 3 tests passed.
- Desktop and phone notification save-draft E2E: 1 test passed in each project.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`: passed.
- `prCapture` calls in both type-scale specs cover the enabled and disabled sound rows; the disabled state is also captured with a denied permission warning.
- Traditional Chinese catalogs were generated with `pnpm run i18n:zh-hant`; the pseudo catalog was regenerated with `pnpm run i18n:pseudo`.
- `pnpm run typecheck` needed `NODE_OPTIONS=--max-old-space-size=8192` on this host because the default 4 GB heap ran out during the rich-output catalog check.
- Review remediation on 2026-11-03: the phone preset selector now grows to fill its control row beside the preview, and unrelated Traditional Chinese automation-copy regeneration was restored to HEAD. The review pass itself did not run builds or tests.
- PR preparation on 2026-11-03 reran the four targeted component suites (47 passed) and both notification type-scale E2E specs with `CAPTURE_PR_ASSETS=true` (4 desktop and 3 mobile tests passed), producing fresh desktop and phone screenshots.
- PR fixup on 2026-11-03 kept desktop preference rows on one line, explained why a disabled sound preview is unavailable, and restored measured normal-row bounds plus a bounded same-row overlap assertion. Local component suites (47 passed) and both type-scale E2E specs with fresh captures (4 desktop and 3 mobile tests passed) cover the fix.
- CI fixup updated the settings-composition E2E to expect four shared settings groups plus the two compact notification preference rows; the focused spec passed (8 tests).

The exact implementation commands are in Task 01.

## Documentation impact

Internal requirements, design, plan, and work order change.
The public developer-tools guide describes remote-provider setup, which remains accurate.
No current screenshot catalog entry depicts these notification preferences.
Public docs need no edit for this package.

## Risks

- Existing permission tests assume section headings and empty body elements; update selectors without weakening warning assertions.
- Dirty feedback and discovery registration must survive replacing `SettingsGroup` wrappers.
- Long translations can squeeze controls near 768px; allow labels to wrap and measure containment.
