---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-SHORTCUT-SETTINGS-FILTER-001
system_design:
  - ../../specs/ui/system-design/keyboard-shortcut-settings-filter.md
legacy_specs: []
---

# Implementation Plan: Keyboard Shortcut Settings Filter

## Overview

Add a local inline filter to the general keyboard shortcut card. One sequential
vertical work order covers the UI, behavioral tests, public guidance, and real
settings-route proof. Implementation starts only after a later explicit release.

## Scope

### In scope

- Filter translated core and dynamic integration labels; preserve full-set
  conflict calculations, overrides, draft/save behavior, and original order.
- Accessible clear and single no-match recovery on desktop and phone.
- Component RED, focused tests, browser evidence, and a concise public docs update.

### Out of scope

Plugin editor filtering, key-chord search, backend and storage changes, shortcut
recording/layout sweeps, settings loading, and save-lifecycle changes.

## Technical approach

Update `KeyboardShortcutsCard` using shared `Input`, `Button`, control sizing,
existing translation keys and the `SidebarShortcutPicker` label-matching pattern.
`useShortcutConflictLabels` continues to receive the complete entry sets. Filter
only the core IDs and integration entries used for rendering. Parent draft and
recorder callbacks stay unchanged. A tiny pure helper is optional if it reduces
component complexity; its tests do not replace the actual mounted-card RED.

Update the keyboard section of `docs/public/sessions-and-review.md`, a task-oriented
how-to, preserving concurrent changes in other sections. Existing
`docs/screenshots.md` references a screenshot; assess whether it remains accurate
without replacing unrelated assets. Recheck the fresh full PR file inventory before
publication, including all pages, and reconcile actual overlapping patches.

## ASCII UI preview

UI-01: Desktop, Settings > Preferences > Keyboard Shortcuts, matching query
(AC .1, .3-.5). The card is part of the existing settings scroll region.

```text
Keyboard Shortcuts
[ Search shortcuts: github                 ] [clear]
Integrations
Open GitHub                       [ Unbound ]
```

UI-02: Phone, same route and search, using the same vertical settings scroll
owner (AC .1, .3-.5). The field and clear action are at least 44px; no overlay.

```text
Keyboard Shortcuts
[ Search shortcuts: github ] [clear]
Integrations
Open GitHub
[ Unbound ]
```

UI-03: Desktop and phone, query with no matches (AC .2, .5):

```text
[ Search shortcuts: unmatched ] [clear]
No matching shortcuts
```

Control order, inline clear, one result state, and single scroll owner are
required; text and spacing illustrate localized existing primitives. Phone
integration row stacking already exists. Do not extend this task to redesign
core recorder rows. Rendered desktop/phone screenshots and measured controls
must be compared with these compositions during verification.

## Tests

`keyboard-shortcuts-card.test.tsx` mounts the real card. Add a failing search
interaction first, capture its actual failure, then cover AC .1-.4: translated
labels and dynamic entries, case/whitespace and list order, a single empty state
and clear, no `onChange` during filtering, a hidden pending override and conflict
peers from all three entry sources. Preserve tests of recording/reset/clear. If a
helper is added, cover its nontrivial normalization separately.

## E2E tests

Add `apps/web/e2e/tests/settings/keyboard-shortcuts-filter.spec.ts` for the
`chromium` project and `apps/web/e2e/tests/settings/mobile-keyboard-shortcuts-filter.spec.ts`
for `mobile-chrome`. Both use isolated fixture data and the real settings route,
search core and integration labels with case/whitespace, check no-match and clear,
restore the ordinary list, and show a hidden pending override and conflict after
filtering (AC .1-.4). Phone additionally measures both search and clear target
boxes at >=44px, containment, and zero document overflow, and captures an actual
screenshot (AC .5). Desktop measures 28px controls and captures its composition. The Chromium project
also tests a 767px fine-pointer viewport, measuring the search and clear controls
at 44px, containment, zero overflow, and the same filter/clear outcome.
No test changes a real developer instance. Preserve each run's owned screenshot,
manifest, and hashes externally before another managed capture clears assets;
inspect the images and record findings, not merely screenshot existence.

## Work orders

- [x] [Task 01: Filter keyboard shortcut settings](task-01-filter-shortcut-settings.md)

## Verification results

Task 01 passed its focused component, lint, typecheck, i18n, documentation,
specification, and browser checks. Desktop Chromium passed two scenarios and
mobile-chrome passed one; screenshot inspection and full results are recorded
in the work order.

## Risks

- Filtering input to conflict detection would hide real conflicts; test core,
  integration and plugin peers independently.
- Conditional row unmounting must preserve parent-owned drafts; the recorder's
  active capture is not a new continuity contract.
- The managed runner can clear captured assets; retain owned evidence before
  subsequent capture and avoid unowned files or services.
