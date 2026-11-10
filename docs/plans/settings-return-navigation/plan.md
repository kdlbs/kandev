---
created: 2026-10-21
status: implemented
requirements:
  - REQ-UI-SETTINGS-RETURN-001
system_design:
  - ../../specs/ui/system-design/settings-return-navigation.md
legacy_specs: []
---

# Settings return navigation plan

## Overview

Close settings will return to the page that opened Settings, including its task
session URL. One sequential work order delivers shared tracking, desktop and
phone controls, and regression coverage.

## Evidence and root cause

Source trace confirms `useSettingsGearToggle` in
`apps/web/components/app-sidebar/app-sidebar-footer.tsx` opens `/settings` but
always closes with `workspaceHomeHref(activeWorkspace, startupPage)`.
`useSettingsTakeover` tracks sidebar mode, not an originating URL.

Reproduction: open `/t/<task-id>?sessionId=<session-id>`, activate the sidebar
Settings gear, then activate Close settings. The close branch selects Home.
Nested Settings navigation does not affect that branch.

Existing `app-sidebar-footer.test.tsx` tests start on Settings and assert Home.
They do not cover task entry followed by close. The existing task-entry E2E in
`settings-gear-only.spec.ts` stops after opening Settings.
This is source-trace evidence, not a completed browser reproduction.

No existing criterion defines Settings return behavior. The new
[requirement](../../specs/ui/requirements/settings-return-navigation.md) fills
that gap. UI owns this reusable page-navigation contract. Tasks and Platform
retain task state and runtime ownership.

## Scope

Include transient origin tracking, guarded close navigation, a phone close
action, and focused desktop/mobile coverage. Preserve explicit Home navigation
and direct-load fallback. Exclude cross-refresh restoration, task lifecycle
changes, and general Settings restyling.

## Technical approach

Implement the [design](../../specs/ui/system-design/settings-return-navigation.md)
through a provider in `src/app-shell.tsx`, shared Settings route transition
logic, and commit notifications from the existing history wrapper. The provider survives route and
viewport changes. The footer and phone Settings header use the same destination.
No server or persisted-state changes are required.

## UI preview

UI-01, desktop Settings entered from Task A:

```text
Before: Task A -> Settings -> [Close settings] -> Home
After:  Task A -> Settings -> [Close settings] -> Task A
        Settings sidebar | Existing settings form
        [Close settings] | Existing save controls
```

UI-02, phone Settings entered through Task A's navigation drawer:

```text
Task A -> Menu -> Settings
+----------------------------------+
| Home / Settings              [X] |  Fixed header
|----------------------------------|
| Settings tree or current form     |  One content scroller
|                                  |
+----------------------------------+
[X] Close settings -> Task A
```

The X has the accessible name Close settings and a minimum 44px square target.
Home remains Home. Nested Settings pages retain their breadcrumbs. The drawing
illustrates spacing only. Existing primitives, tokens, and localized labels
define the actual presentation. UI-01 covers .1-.5. UI-02 also covers .6.

## Tests

- `lib/settings/settings-return-navigation.test.ts`: task URL preservation,
  nested pages, fallback, and repeated visits (.1, .2, .4, .5).
- `components/settings/settings-navigation-provider.test.tsx`: committed events,
  same-tick redirects, blocked requests, query-only changes, history, remounts,
  silent `replaceTaskUrl` session updates, and consumer lifetime (.1-.5).
- `components/app-sidebar/app-sidebar-footer.test.tsx`: close to the recorded
  origin, unchanged fallback, and cancellation timing (.1, .3, .4).
- `components/settings/settings-layout-client.test.tsx`: phone close wiring,
  existing header actions, and unchanged Home breadcrumbs (.3, .5, .6).

## E2E tests

- Extend `e2e/tests/settings/settings-gear-only.spec.ts` in `chromium` with
  `returns to the originating task and session after closing settings` (.1-.3).
  Seed two sessions, select the nondefault session, visit another Settings page,
  close, and verify the selected session and document marker.
- Add `e2e/tests/settings/mobile-settings-return.spec.ts` in `mobile-chrome`
  with `returns to the originating task from the phone settings header` (.1-.3, .6).
  Use the real task navigation drawer. Check a nested page, cancel a dirty
  departure, then permit it and assert task/session identity.
- Retain direct-load, brand-to-Home, and collapsed-sidebar coverage. Add a
  query-context assertions. Provider tests cover repeated visits from Task B (.2, .4, .5).
- Measure the phone target at the configured device and at 767px with a fine
  pointer. Check header containment and no document horizontal overflow (.6).

## Work orders

- [x] [Task 01: Restore Settings return navigation](task-01-restore-return-navigation.md)

## Verification results

Completed on 2026-10-21.

- RED: `app-sidebar-footer.test.tsx` failed because close selected Home instead
  of the recorded task/session URL. GREEN: all 72 targeted unit tests passed
  across the seven work-order files. The six provider tests passed again after
  a test-only constant extraction.
- Production-file ESLint and targeted test-file ESLint passed. Prettier checks
  passed for the new helpers, provider, close button, and history changes.
- `pnpm run typecheck` passed with `NODE_OPTIONS=--max-old-space-size=8192`.
  The first attempt exhausted Node's default 4 GiB heap.
- `pnpm run i18n:check` passed. `node scripts/validate-public-docs.mjs`
  validated all 48 public pages. Catalog and specification lint passed.
- Desktop: all 6 tests in `settings-gear-only.spec.ts` passed (30.5 seconds).
  Log: `/tmp/kandev-run.e2e.IxO7q2gA.log`.
- Phone: all 6 tests across `mobile-settings-return.spec.ts` and
  `mobile-settings-index.spec.ts` passed (41.0 seconds).
  Log: `/tmp/kandev-run.e2e.mNKwdW4w.log`.
- Browser assets were freshly built by the initial managed run. Later runs used
  `--no-build` with those same unchanged production assets. Host configuration
  required `KANDEV_SERVER_HOST=127.0.0.1`. Chromium required `LD_LIBRARY_PATH`
  entries for existing Nix libraries. Both were process-local overrides.
- The phone capture matches UI-02: Home/Settings breadcrumbs, a separate X,
  and one content scroller. E2E measured header containment, the 44px minimum,
  and zero document horizontal overflow. No new motion or surface treatment.

The user requested a ready PR on 2026-10-22. A fresh isolated desktop/phone
capture passed before publication. Both compressed assets were inspected and
contain only synthetic data. [PR #4682](https://github.com/kdlbs/kandev/pull/4682) is ready for review. Its base advanced
before publication, so the app-shell conflict was resolved by retaining both
IntegratedControlProvider and SettingsNavigationProvider. Targeted validation
was repeated on base `edafc7900de0328a9997401f11a0d76cb8c548d3`:
72 targeted unit tests, typecheck with the 8 GiB heap, app-shell ESLint, and
all 12 desktop/phone browser tests passed. The fresh managed build served
both browser suites. Logs: `/tmp/kandev-run.e2e.au5SqyOn.log` (phone, 6 passed)
and `/tmp/kandev-run.e2e.ozCEfo8W.log` (desktop, 6 passed).

## Risks

React render batching can lose the origin if tracking relies only on pathname
effects. Settings redirects, cancelled navigation, and hidden desktop chrome
need explicit tests. A refresh intentionally loses transient origin context.
Missing or inaccessible tasks retain existing destination error handling.

## Documentation impact

Updated the Settings section in `docs/public/use-kandev.md` (how-to guide).
It explains desktop and phone close actions, task/session restoration, unsaved
changes, and direct-load/refresh fallback. Public validation passed.

## PR review remediation

Greptile identified a cancelled multi-entry history traversal whose separate
fragment event could overwrite the Settings origin. The regression drives the
real navigation guard, holds restoration at the history boundary, and models
the fragment event that happy-dom omits. RED selected Task B instead of Task A.
The provider now accepts guard-approved popstate events and committed history
mutations, without accepting hashchange as departure evidence. The actual
source URL continues to preserve fragments when Settings opens.

The regression and existing targeted unit tests pass (73 tests). Typecheck
with the 8 GiB heap, focused ESLint, catalog validation, and specification lint
passed. A fresh managed build passed all 6 phone tests
(`/tmp/kandev-run.e2e.OYKePFbC.log`), and the same assets passed all 6 desktop
tests (`/tmp/kandev-run.e2e.Y7kZQiwv.log`). No rendered layout changed, so the
existing desktop/phone captures remain representative. Public documentation
needs no change because this preserves the existing guarded-navigation contract.
