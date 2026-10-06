---
id: "02-shared-reveal-control"
title: "Shared reveal control and persisted preference"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-HIDDEN-FOLDERS-001
acceptance_criteria:
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.1
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.2
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.3
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.4
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.5
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.6
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.7
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.8
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.9
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.10
  - AC-WORKSPACES-HIDDEN-FOLDERS-001.12
system_design:
  - ../../specs/workspaces/system-design/hidden-folder-browsing.md
---

# Task 02: Shared reveal control and persisted preference

## Summary

One persisted preference, read by the shared listing hook and written by one
control in the shared browser body. The three existing consumers gain the
capability without a new prop and without owning state.

## Scope

- The persisted UI preference, read at slice-module load and written by one
  action, defaulting off.
- The API client's optional visibility argument, appended only when active.
- `useDirectoryListing` depends on the preference and re-lists on change.
- The control in the shared browser body, pinned to the breadcrumb row's
  trailing edge, with a localized action label as its accessible name.
- No create-folder change: the browser re-lists through `list-dir`, which already
  carries the preference.
- Localized copy in every shipped locale, the pseudo locale, and the
  Traditional Chinese pair.
- Frontend unit tests for the request URLs, the re-list, persistence across
  remount, the native path, and the phone target.

## Exclusions

- No new component prop on `DirectoryBrowserBody`, `useDirectoryListing`, or
  `FolderPicker`, and no consumer-level state.
- No path input, no tilde handling, no search box, and no listing filter.
- No change to the desktop native picker trigger or the Rust command.
- No Files panel, Changes, or editor changes.

## Implementation acceptance conditions

1. With the preference off, the list request URL is byte-identical to today's,
   and the browser renders no hidden entry. With it on, the request carries
   `include_hidden=true`, and the switch reports its state with `aria-checked`.
   A visibility refresh keeps the breadcrumb, folder selection, and any new-folder
   draft in place while it loads.
2. The preference survives closing and reopening a browser and a page reload,
   and every directory browser reflects the same value.
3. The control is a keyboard-reachable switch with the stable localized name
   `Hidden folders` and `aria-checked`. It keeps the pinned trailing-edge slot on
   desktop and has a 44px target at phone widths and for coarse pointers. The
   Tauri native-picker path renders no control and issues no list request.

## Verification commands

```bash
cd apps/web
pnpm run typecheck
pnpm run lint
pnpm run test components/folder-picker.test.tsx components/create-local-repository-surface.test.tsx
pnpm run i18n:check
pnpm run i18n:ratchet
```

## Likely files

- `apps/web/lib/api/domains/fs-api.ts`
- `apps/web/components/directory-browser/show-hidden-toggle.tsx` (new module)
- `apps/web/components/folder-picker.tsx` and its test
- `apps/web/lib/state/app-state-types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts` and its types
- `apps/web/lib/local-storage.ts`
- `apps/web/components/create-local-repository-surface.tsx` and its test
- `apps/web/src/locales/*/common.json`

## Dependencies and risks

- The two new keys must exist in every shipped locale or the i18n check fails;
  the Traditional Chinese pair uses the project's existing conversion command.
- UI punctuation stays plain, with no em dash in any locale value.
- The folder-picker test file is close to its size budget; extract a component
  rather than growing one function.
- Task 01 defines the wire contract this task targets. The frontend unit tests
  stub the API client, so this work order does not block on task 01.

## Results

Done.

RED was observed before each production change: the fs-api reveal cases failed
with `'/api/v1/fs/list-dir' to be '/api/v1/fs/list-dir?include_hidden=tr...'`,
the three slice preference tests failed, and the component suite reported four
`Unable to find ... directory-browser-show-hidden` failures with the 43 and 8
pre-existing tests still green.

Three real defects were found by the tests rather than by review:

1. The slice's initial state is a module-level object, so a stored preference is
   read once per page load. The persistence test now re-imports the module
   through `vi.resetModules()` to model a reload, instead of building a second
   store from already-evaluated state.
2. The preference key was declared below the initial state it feeds, which threw
   `Cannot access ... before initialization` at module load. The key moved above it.
3. `AppState` enumerates UI fields explicitly, so the new field and action had to
   be declared there too or typecheck failed.

Changed files: `fs-api.ts`, `ui-slice.ts`, `ui/types.ts`,
`app-state-types.ts`, `directory-browser/show-hidden-toggle.tsx`,
`folder-picker.tsx`, three test files, and `common.json` for en, ja, pt-pt,
pseudo, zh-cn, zh-hk, zh-tw. The Chinese "hide" value was changed to
"\u4e0d\u663e\u793a\u9690\u85cf\u6587\u4ef6\u5939" to avoid the redundant reading, then zh-hant and pseudo were regenerated with the project commands. Unrelated generated drift in three `settings.json` files was reverted to keep the change focused.

The control was extracted into its own module because `folder-picker.tsx` had
crossed the 600-line lint budget.

Code review identified an accessibility contract issue. A changing action name
and `aria-pressed` can announce conflicting states. The control now uses the
stable localized name "Hidden folders" and reports its state with
`aria-checked`, which gives keyboard and screen-reader users the same switch
contract at every state.

Review also added the missing coverage for AC-WORKSPACES-HIDDEN-FOLDERS-001.4:
`create-local-repository-surface.test.tsx` now asserts the third directory
browser exposes the control and re-lists with the shared preference.

`pnpm run test` for the four targeted files is now 88 passed.

Verification:

| Command | Result |
| --- | --- |
| `pnpm run test components/folder-picker.test.tsx components/create-local-repository-surface.test.tsx lib/api/domains/fs-api.test.ts lib/state/slices/ui/ui-slice.test.ts` | 88 passed (4 files) |
| `pnpm run typecheck` | clean |
| `pnpm run lint` | clean, 0 warnings |
| `pnpm run i18n:check` | keys OK, 6 locales complete, pseudo in sync, no em dashes, no non-JSX copy |
| `pnpm run i18n:ratchet` | 0 added + 5 modified files clean, guard allowlist intact |

The two existing directory-browser consumers needed the store provider in their
tests, because the shared browser body now reads the preference from the store.
