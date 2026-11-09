---
created: 2026-10-27
status: done
requirements:
  - REQ-UI-FILE-EDITOR-MUTATION-001
  - REQ-UI-FILE-EDITOR-MUTATION-002
system_design:
  - ../../specs/ui/system-design/file-editor-mutation-ownership.md
legacy_specs: []
---

# Implementation Plan: Open editor rename continuity

## Overview

After an accepted Files rename or move, keep each affected open editor attached
to the new path so its unsaved work can be saved. One sequential work order
owns the shared success boundary and its desktop and tablet consumers.

## Scope

### In scope

- Successful in-app file/folder rename and each successful move, including
  repository/environment scoping and segment-boundary matching.
- Existing dirty/clean, preview/promoted/pinned buffers, active selection,
  saved tab identity and subsequent Save/open behavior.
- Independent unit/integration regression and real desktop Files rename/save.

### Out of scope

- External rename detection, protocol changes, concurrent save ordering, general
  editor lifecycle redesign, phone editing or a new phone rename control.

## Technical approach

`hooks/use-file-operations.ts` is the shared admission and success boundary.
Compare tree paths with each editor's workspace-relative identity (`repo/path`
for scoped editors, `path` otherwise); retain repo only when the new path is
inside that same root. Capture selected-session visit and environment (exact
session fallback only when unmapped), host and editor incarnation. A narrow
app-store reservation prevents source/destination opens during the pending
rename; reject an already-open destination or overlapping reservation before
transport using existing rename-failure feedback. Distinct paths/environments
remain independent; each move root settles separately.

For Dockview, atomically rekey the buffer preserving its instance, insert the
new pinned panel in the same group/index with `api.addPanel` inactive, then
remove the old panel; restore group/global selection and persist only after the
transaction. Preview panels update their existing parameters. `SessionTabletLayout`
passes a registered `TaskCenterPanel` adapter to `TaskFilesPanel`, preserving
its local tab state and visit ownership. See the design for the full sequence,
including removals, persistence and in-flight open admission.

## ASCII UI preview

UI-01: Desktop Files rename; dirty file selected before and after. Existing
layout and controls are structural; spacing is illustrative. Criteria .1-.4.

```text
Before                               After success
Files            Editor              Files            Editor
old.ts           old.ts *            new.ts           new.ts *
[rename old.ts]  [dirty content]      new.ts selected  [dirty content]
                 [Save]                               [Save -> new.ts]
```

UI-02: Phone Files and document are separate focused surfaces. Existing Files
row actions and viewer scroll remain unchanged. Phone viewer is read-only and
the visible touch action menu has no rename; a real phone rename/save flow is
not available. No new surface or editing capability is introduced.

```text
Files                         Document
[file row] [actions]   ->     [path] [Close]
                              [read-only content]
```

Tablet retains its side-by-side Files and local editor tabs. Shared state
semantics, not responsive geometry, change. `session-mobile-layout.tsx` is the
nearest shipped focused viewer exemplar. Existing scroll, touch targets, safe
area and navigation remain owned by those surfaces.

## Tests

- `hooks/use-file-operations.test.ts`: success only, failure and partial move;
  capture environment and guard retired/replaced consumer.
- `lib/state/dockview-file-state.test.ts` and focused panel integration tests:
  exact and directory mappings, same-prefix and other-repository exclusions,
  dirty state/baseline, preview/promoted/pinned identity, active state,
  persistence, reopen, aliases, preflight destination collision, in-flight
  open reservation including previously admitted reads/restoration, unloaded
  source and no buffer loss. Cover the
  existing ownership criteria .1-.5 alongside new .1-.5.
- `components/task/task-center-panel-mutation-ownership.test.tsx`: tablet tab
  continuity, selected tab, scope and failed rename. Adjust exact test files if
  the selected owner requires a nearby existing suite; record every changed
  suite and run it in the work-order results.

## E2E tests

- `e2e/tests/task/file-editor-rename-continuity.spec.ts`, chromium: independently
  seed a task; open a file, save a positive control, edit, rename through Files,
  save again and verify renamed bytes and absence of old path. Check existing
  tab identity and reopen; map to .1 and .3.
- Phone: there is no editable phone file surface or visible touch Rename
  entry point. A phone rename/save test would require new product capabilities;
  do not fake it via a desktop context menu or backend call. This is a known
  coverage limitation for ROOT review. The existing mobile viewer remains
  read-only. Unit coverage checks the shared operation and scope.

## Work orders

- [x] [Task 01: Retarget open editors after Files rename](task-01-retarget-open-editors.md)

## Verification results

Completed. The independent desktop RED showed a successful pre-rename Save
and rename followed by an old-path Save failure. The fresh managed desktop run
passed one test after the fix. Targeted owner and related suites passed; the
promotion-specific tests cover both survival and closure of the unrelated preview,
plus close/reopen and replacement-host controls. Scoped ESLint, i18n ratchet,
8 GiB typecheck, catalog/spec lint, public-doc validators and diff checks passed.
A disposable tablet capture also passed its real rename and Save. The phone
viewer remains read-only and has no visible touch Rename action.
See [task results](task-01-retarget-open-editors.md#results) for command details.

## Risks

- Dockview replacement must retain group/tab selection, panel order and buffer
  incarnation through removal and persistence listeners.
- Files' environment-stable session differs from the selected session, and
  multi-repository tree paths need authoritative normalization.
- Phone does not provide the requested native rename/save path. Do not assert
  coverage that cannot be exercised through the current product.
- Existing failure feedback covers a temporarily reserved path or open
  destination without claiming that a file exists on disk. The public Files
  guide records the behavior; no new product copy was added.
