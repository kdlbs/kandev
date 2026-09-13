---
created: 2026-09-13
status: implemented
requirements:
  - REQ-TASKS-CREATION-AUTO-FOCUS-001
system_design:
  - ../../specs/tasks/system-design/creation-auto-focus.md
legacy_specs: []
---

# Implementation plan: Task creation auto-focus

## Overview

Deliver one vertical preference: persist a default-on setting, expose it in Task
Actions, and honor it at all shared task-creation completion boundaries. One
sequential work order keeps the preference and its behavior together.

## Scope

Include persistence, settings discovery, desktop/phone UI, localization,
creation completion, focused tests, and public task documentation. Exclude
per-task overrides, agent-start policy changes, Quick Chat, edit/session flows,
and automatic focus for background/API events.

## Technical approach

Follow the [design](../../specs/tasks/system-design/creation-auto-focus.md).
Use the existing settings JSON with a true missing-value default and the shared
save coordinator. Extend creation metadata with `autoFocus`; retain the
distinct meaning of `willNavigate`. Inspect every dialog caller before changes,
retain cache and completion work before suppression guards, and decouple plan
initialization from navigation.

The per-user scope and Task Actions placement follow existing preference
patterns. User-confirmed behavior is an option to prevent focus with current
behavior as the default; this package proposes those routine UI/storage details.

## ASCII UI preview

UI-01: Settings > General > Task Actions, saved default. Desktop and phone use
the same inline card composition:

```text
+-------------------------------------------+
| Auto-focus new tasks               [ ON ] |
| Open newly created tasks automatically.   |
| Turn this off to stay on your current     |
| view. Tasks and agents still start as     |
| requested.                               |
+-------------------------------------------+

After toggling: [ OFF ] (unsaved)
Existing shared action: [ Save changes ]
```

UI-02: Successful creation with the saved setting off:

```text
Current task or listing -> [Create task] -> Same task or listing
                          dialog closes   New task is available
                                          for manual opening
```

Card hierarchy, visible help, and shared save ownership are structural. Spacing
and line breaks are illustrative. On phone, text wraps in the existing page
scroll region; the switch has a >=44px touch target and the existing save
control clears the safe area. No nested scroller or new drawer. A save failure
retains dirty state and existing shared error UI; discard restores the saved
switch value. Creation failure retains the existing dialog error flow.

These views cover AC-TASKS-CREATION-AUTO-FOCUS-001.1–001.5 and are verified by
the E2E files below.

## Tests

Use TDD for changed logic. The work order owns exact commands.

| AC suffix | Test location and cases |
| --- | --- |
| .1, .4 | New `creation_auto_focus_test.go` tests in user models/store/service and boot projection: true default, absent JSON, false/true round trip, omitted patch preservation |
| .1, .4 | `lib/ssr/user-settings.test.ts`: missing value true and explicit false through mapping; new settings component test for save/discard/rejection |
| .2, .3 | `task-create-dialog-submit.test.tsx`, helpers and caller suites: enabled/default behavior, false/no callback suppression, no-agent, agent, passthrough, plan, queued/no session, error and edit/session regressions |
| .2, .3 | Caller tests: sidebar, canvas, task header, phone switcher and board, asserting retained cache upserts and no route/active task/session/layout change |

For new focused test files use the `creation-auto-focus.test.tsx` basename or
suffix so the work-order Vitest filter includes them. Extend existing suites
where their fixtures already cover the changed consumer.

## E2E tests

Add `e2e/tests/task/creation-auto-focus.spec.ts` (chromium) and
`e2e/tests/task/mobile-creation-auto-focus.spec.ts` (mobile-chrome).
Use existing task creation and mobile switcher fixtures, with no per-test
device overrides. Prove default-on behavior, saving off and reloading,
creation from a listing and an already active task, preserved route/selection,
manual opening, re-enabling, and unchanged explicit launch outcome. Verify
phone switch accessibility, >=44px target, containment, and focus return.
Unit/component tests cover the full caller and launch-mode matrix; E2E covers
the integrated user flows. Rebuild web and backend before browser tests.

## Work orders

- [x] [Task 01: Implement saved auto-focus preference](task-01-auto-focus-preference.md)

## Verification results

Implementation and verification completed on 2026-09-13. Backend persistence,
partial settings updates, boot projection, and settings catalog checks passed.
The broad frontend run passed 315 tests across 25 files; subsequent focused
runs verified final dialog focus restoration (11 tests), Azure launcher, and
settings fixture changes. TypeScript, changed-file ESLint, Prettier,
localization checks and ratchet, native backend builds, and web build passed.
Desktop and phone browser scenarios passed, including saved preference reload,
retained current view, manual opening, requested background agent execution,
re-enabled auto-focus, keyboard focus return, and phone touch/overflow checks.
Public documentation validation passed (62 tests, 46 pages). Specification
catalog/lint and whitespace validation passed. Exact commands and build scope
are recorded in the completed work order. Changes remain uncommitted.

## Risks

- Missing/default coercion must not replace explicit false with true.
- Parent callbacks must not interpret suppressed navigation as permission to navigate.
- Planning task setup must survive disabled navigation without switching the current layout.
- Filtered lists need not reveal a newly created task; cache updates must still occur normally.

## Documentation impact

Updated `docs/public/tasks-and-workflows.md` with the setting location, enabled
default, saved scope, and unchanged requested agent-start behavior.
