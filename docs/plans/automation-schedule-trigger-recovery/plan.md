---
created: 2026-11-23
status: done
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
system_design:
  - ../../specs/office/system-design/automations-settings-03.md
legacy_specs: []
---

# Schedule trigger recovery plan

## Overview

The scheduler requires two enabled flags. The automation flag permits the
automation to run. The trigger flag permits its cron schedule to start a run.
If the first flag is on and the second is off, Runs shows a disabled schedule.
Add **Turn on schedule** to that status in Run detail. [GitHub issue #5455](https://github.com/kdlbs/kandev/issues/5455)

## Reproduction and cause

1. Start with an automation and its configured schedule both disabled.
2. Enable the automation from the workspace list and save.
3. Open that automation in Runs. Its next-firing status reads “Schedule trigger
   is switched off.”

`useAutomationEnabledDrafts` submits `automation.update` for the parent.
`ListEnabledTriggersByType` requires both flags to be true. `nextFiring`
reports the trigger flag from the saved row. The editor exposes the cron
fields and lacks a control for this flag.

A temporary Go probe used the real WebSocket handlers, SQLite store, and
scheduler with a fixed clock. The parent update left the trigger off and
produced zero runs. An `enabled: true` trigger update preserved the cron
expression, timezone, and cursor. Two subsequent checks admitted one
catch-up run and emitted one event. Both temporary tests were removed.

## Technical approach

In the shared `RunDetailDisclosure`, show one action for the disabled trigger
identified by `scheduleBinding`. Send `{ enabled: true }` for that trigger.
Use the existing activity refresh after success and an error with retry
after failure. Document the action in the public Schedule section.
[Schedule trigger recovery design](../../specs/office/system-design/automations-settings-03.md#schedule-trigger-recovery)

### Scope

The work covers one recovery action for the schedule shown in Run detail.
The existing button and request patterns work for both the desktop rail and
phone drawer.

### Out of scope

- Automation editor changes and a separate schedule pause control.
- Scheduler policy, API changes, and database repair.

## ASCII UI preview

UI-01: Disabled schedule recovery. On desktop, open **Run detail** in the
Runs rail. On a phone, open the Runs drawer, then **Run detail**.
The preview covers [AC-001.28 through .30](../../specs/office/requirements/automations-settings.md#req-office-automations-settings-001-automations-in-settings).

Current Run detail

```text
Schedule trigger is switched off
```

Proposed Run detail on desktop and phone

```text
Schedule trigger is switched off
[Turn on schedule]
```

The existing disclosure owns placement in the rail or drawer. The button
has a touch target of at least 44 px on phones and coarse pointers. After
success, the refreshed next-firing line replaces the reason and action.

## Tests

- **.28–.30** In `automation-detail-page.test.tsx`, open Run detail, fail and
  retry the action, and check the trigger payload and refreshed status.
- **.22 and .26** Reuse the Go boundary and catch-up tests.

## E2E tests

- **.28 and .29** Recover a disabled schedule in
  `automations-run-detail.spec.ts` on `chromium` and
  `mobile-automation-detail.spec.ts` on `mobile-chrome`. Verify the refreshed
  status and persistence. Check the mobile tap target.

## Work orders

- [x] [Task 01. Enable a disabled schedule](task-01-enable-schedule.md)

## Verification results

The recovery action is implemented in shared desktop and phone Run detail.
The page regression, scheduler tests, desktop and mobile E2E checks, build,
localization checks, public documentation validation, typecheck, and lint pass.
Review fixes fence activity refreshes to the active automation, keep schedule
recovery on the Activity tab, and use shared button sizing for the 44px coarse
pointer target. The late-write test loads automation B before resolving
automation A's write. The Configure test confirms the editor remains mounted
without exposing schedule recovery there.

## Risks

Re-enabling a schedule can admit one missed occurrence on the next check.
