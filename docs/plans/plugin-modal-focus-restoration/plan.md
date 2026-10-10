---
created: 2026-10-07
status: done
requirements:
  - REQ-PLUGINS-ACTION-UX-001
  - REQ-PLUGINS-ACTION-UX-003
system_design:
  - ../../specs/plugins/system-design/plugin-action-ux.md
legacy_specs: []
---

# Implementation plan: Plugin modal focus restoration

## Overview

Restore focus after a plugin control closes a host-owned modal.
[Issue #4220](https://github.com/kdlbs/kandev/issues/4220) reports focus loss
after Escape on status-right, main topbar, and task topbar Actions.
The issue is assigned to `carlosflorencio`.

Deliver one sequential work order with manager, component, and packaged-browser
coverage. The plugin system owns this repair because it owns `host.openModal`
and the imperative instance lifecycle. UI primitives remain shared dependencies.
This package extends the existing interaction contract instead of creating a
separate repair specification.

## Evidence and confirmed cause

Investigation source: `330e02a47808c11ca315ae30456fcce7f4806db5`.
The issue reports stable v0.97.0 and Chromium 154.0.8037.97.
It has no comments or image attachments at investigation time.

`PluginModalHost` mounts `Dialog open` without a `DialogTrigger`.
The installed Radix `@radix-ui/react-dialog` 1.1.15 close handler prevents the
FocusScope fallback and calls `context.triggerRef.current?.focus()`.
That trigger reference is empty for imperative plugin modals.
`PluginModalManager.open` stores only instance identity, owner, options, and
layout. Neither host presentation supplies `onCloseAutoFocus`.
There is therefore no restoration path to the initiating Action.

A temporary component reproduction mounted the real `PluginAction`, its surface
provider, and `PluginModalHost`. It focused an Action, activated it, confirmed
focus inside the dialog, and sent Escape. Both topbar and status-bar cases
closed the modal and failed the final opener-focus assertion with `BODY`.
Command: `(cd apps/web && pnpm exec vitest run components/plugins/issue-4220.repro.test.tsx)`.
Result: two expected failures in the browser-locales project.
The temporary reproduction was removed after investigation.
Native browser Enter activation remains part of implementation verification.

## Settled intent

The issue establishes the expected opener-focus behavior. The existing Action
contract already requires keyboard activation and focus continuity.
Add explicit closure, invalid-opener, and concurrent-surface criteria under
REQ-PLUGINS-ACTION-UX-003. Preserve compatibility under 001.1 and 001.5.
No material product question blocks planning.

## Scope

### In scope

- Capture the focused opener before publishing each imperative modal instance.
- Restore eligible openers through both content primitives' close-focus callbacks.
- Preserve remaining overlay focus when an opener is unavailable or behind another modal.
- Cover normal closure, nested instances, owner cleanup, and nondismissible surfaces.
- Verify all three reported desktop surfaces and the existing phone entry points.

### Out of scope

- Changing Action handlers, slot registration, plugin bundle contracts, or shared UI defaults.
- New public options, SDK types, persistence, backend behavior, runtime flags, or dependencies.
- Redesigning dialogs, drawers, navigation, status geometry, or opening autofocus.
- Restoring a removed control by identity, reopening parent surfaces, or certifying historical browsers.
- Plugin repository changes, publication, commits, PR creation, or production implementation in this design turn.

## Technical approach

Extend `OpenPluginModal` in `apps/web/lib/plugins/modal-manager.ts` with an
internal opener reference. Capture it in the shared `open` path before `notify`.
Keep DOM access guarded and preserve the current snapshot and handle behavior.

In `apps/web/components/plugins/plugin-modal-host.tsx`, share a close-focus
handler between `PluginDialog` and `PluginDrawer`.
Use `onCloseAutoFocus`, suppress triggerless primitive restoration, and focus
eligible openers without scrolling. Validate the target at callback time.
Consult the current modal snapshot and active rendered surfaces before restoring.
Do not restore behind a surviving overlay or from a stale cleanup callback.
Keep this logic local unless extraction is required by the existing function limits.

Use the shipped phone `MobilePluginNavSection` and `AppStatusDrawer` as entry points.
They already contain touch-sized plugin controls and a single scrolling body.
The modal content keeps its header and existing scroll body.
An explicitly requested plugin drawer keeps its inset presentation and safe-area padding.
The repair concerns focus ownership and requires no new mobile surface.

Add test-only focus-launch controls to the packaged fixture's
`apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`.
Keep existing toggles and geometry-test controls unchanged.
Use `host.ui.Action`, `host.openModal`, and real slot registration.
Add unique modal launchers for the main topbar, task topbar, and right status slot.
Phone fixtures must expose both default dialog and explicit drawer presentation.
Do not inject the modal manager through a browser test bridge.

## ASCII UI preview

### UI-01: Desktop Action, open and closed states

```text
Topbar or status: [Details*] --Enter--> +---------------------+
                                      | Plugin details      |
                                      | [content control*]  |
                                      +---------------------+
Before Escape: dialog closes -> BODY
After Escape:  dialog closes -> [Details*] -> Tab continues
```

### UI-02: Phone parent surface and plugin modal

```text
App menu / Status drawer     Plugin dialog or requested drawer
+-----------------------+    +-----------------------+
| Plugins / Status      |    | Plugin details        |
| [Details*]            | -> | [content control]     |
| other controls        |    +-----------------------+
+-----------------------+    close -> [Details*] in open parent
```

`*` means keyboard focus. Labels and spacing are illustrative.
The required structure is the existing header, control grouping, and scroll owner.
Closure returns focus within the still-open parent surface. It does not reopen it.
When the opener disappears, the active surviving overlay owns recovery.
These previews map to AC-PLUGINS-ACTION-UX-003.6 through .8.

## Tests

Test paths are relative to `apps/web`. Names below are implementation targets.

| Criteria | Evidence |
| --- | --- |
| 003.6 | `lib/plugins/modal-manager.test.ts`: capture opener before publication, distinct instance targets, shared task-link path, absent DOM globals |
| 003.6 | `components/plugins/plugin-modal-host.test.tsx`: `returns focus to the opening Action after Escape`, parameterized for topbar/status, dialog/drawer, and legacy button |
| 003.6 | Same component file: close control and returned handle restore the same opener without scrolling |
| 003.7 | Same component file: removed, disabled, hidden, and inert openers preserve available surface focus and never choose an unrelated control |
| 003.8 | Same component file: child closure restores parent opener, background closure preserves a focused foreground Popover, and old close cleanup after successor open preserves the successor |
| 001.5, 003.7-.8 | Same component file: owner bulk cleanup with multiple instances and another owner's surviving modal |
| 003.8 | Same component file: nondismissible Escape/outside behavior remains intact, explicit handle closure still works |

For the first permanent regression, mount an Action and host, focus the Action,
activate it, and await dialog focus. Send Escape and await content removal.
Then assert `document.activeElement === opener`. The uncorrected source must fail
only the final focus assertion. Do not mock Dialog, Drawer, or FocusScope.

## E2E tests

Extend existing `e2e/tests/plugins/plugin-action-ux.spec.ts` and
`mobile-plugin-action-ux.spec.ts`. Use the packaged fixture installation flow
and its cleanup helpers. Restore status-bar settings after each scenario.
Preserve existing geometry, toggle, and compatibility tests.

| Project | Flow | Criteria |
| --- | --- | --- |
| chromium | Status-right, main topbar, and task topbar: Tab to each Action, Enter, modal focus, Escape, opener focus, then continued Tab navigation | 003.6 |
| mobile-chrome | Plugins section and Status drawer: focus Action, activate details, close default dialog, return to visible parent control | 003.6-.8 |
| mobile-chrome | Explicit plugin drawer from a focused parent control: close and retain parent focus | 003.6-.8 |

Use `toBeFocused()` after closure so animations and deferred cleanup settle.
Confirm the parent remains visible and usable. Assert no document horizontal
overflow and existing touch target minimums for the new fixture controls.

## Work orders

- [x] [Task 01: Restore plugin modal opener focus](task-01-restore-opener-focus.md)

## Verification results

- Temporary reproduction: two expected failures, both received `BODY`.
- `(cd apps/web && pnpm exec vitest run components/plugins/plugin-modal-host.test.tsx lib/plugins/modal-manager.test.ts)`:
  two files passed, 17 tests passed.
- Permanent host regression failed against the original implementation with focus on `BODY`, then passed with opener restoration.
- `(cd apps/web && pnpm exec vitest run lib/plugins/modal-manager.test.ts components/plugins/plugin-modal-host.test.tsx)`: 2 files and 37 tests passed. Fake timers flush Radix close-autofocus callbacks before overlap, owner-cleanup, and successor-ordering assertions.
- A real Radix Popover regression failed before the overlay selection fix, then passed with focus and visibility retained after the background modal closed.
- Targeted ESLint across the manager, host, unit tests, and both browser tests: passed with zero warnings.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `(cd apps/web && pnpm run i18n:ratchet)`: passed; no changed-copy violations.
- Prettier check for the modified host and component test: passed.
- `(cd apps/web && pnpm e2e:run --project chromium tests/plugins/plugin-action-ux.spec.ts)`: production build and fixture packaging passed; all 4 Chromium tests passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-plugin-action-ux.spec.ts tests/plugins/mobile-plugin-modal.spec.ts)`: production build and fixture packaging passed; all 5 mobile Chrome tests passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed; `node scripts/validate-public-docs.mjs`: 47 published pages validated.
- `python3 scripts/list-docs.py validate`: 360 decisions and 1425 specifications validated; `python3 scripts/lint-spec-files.test.py`: 36 tests passed; `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- PR documentation evaluator: `covered`, zero errors; both requirement IDs and all acceptance references resolved.
- `git diff --check` and fixture source/package equality check: passed. Changes remain unstaged and uncommitted.

## Risks

- FocusScope cleanup is deferred. Restore at its callback boundary and prove a newer modal retains focus.
- Parent drawer focus traps can reject background focus. Verify actual parent recovery in mobile Chromium.
- An opener can disappear during plugin unload or responsive remount. Reject it without selecting a replacement by test ID.
- Packaged browser verification passed for the three reported desktop Actions and both phone parent-surface paths.

## Documentation impact

Internal specifications extend the existing Action interaction contract.
The original completed Action package remains historical evidence for its original scope.
During implementation, add a short focus-return note to `docs/public/plugins-authoring.md`
and `docs/plans/plugins/PLUGIN-API.md`. No SDK signature or release-number change is required.
