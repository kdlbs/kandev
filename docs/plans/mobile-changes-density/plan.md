---
created: 2026-09-23
status: implemented
requirements:
  - REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-002
system_design:
  - ../../specs/ui/system-design/changes-file-row-containment.md
legacy_specs: []
---

# Implementation Plan: Mobile changes density

## Overview

Restore readable working-tree filenames with compact touch rows and one file
action menu. The user explicitly requested AFK planning and implementation in
one turn, followed by a PR with screenshots. Execute sequentially here.

## Scope

Change working-tree row presentation and its mobile action entry point. Keep
Git semantics, desktop composition, file ordering, and the task toolbar intact.
UI owns the existing reusable file-row presentation contract; no domain or
architecture boundary changes.

## Technical approach

Branch `FileRow` content using the canonical responsive hook. Add a small
touch-content component and shared DropdownMenu action surface, forwarding the
existing callbacks. Reuse translated action labels. Keep `PanelBody` as the
single scroll owner and existing viewport/safe-area handling. Update the public
review guide to describe the touch menu.

## ASCII UI preview

UI-01: Changes tab, unstaged list, phone/coarse pointer.

```text
Before: [+ large] folder/file... [discard] [edit]
After:
  UNSTAGED (8)                       Stage all
  [file] status-surface-metrics.test.ts    [...]
         apps/web/components/task  +12 -3
  [file] long-basename-wraps-onto-
         another-line.test.ts             [...]
         apps/web/components/task

Menu: full/path/status-surface-metrics.test.ts
      Stage file
      Edit
      Discard changes
```

UI-02: Fine-pointer desktop, existing compact row.

```text
  [+] folder/basename.ts              +12 -3 M
  Hover:                             [undo] [edit]
```

Structural requirements: basename receives available width; path/stats below;
one visible secondary action target; menu exposes the full path; 44px touch
targets and preserved desktop density. Pixel spacing is illustrative. Pending
staging shows a spinner and disables its menu item. Cancel discard preserves
the file and returns to the list. Covers AC-002.1 through AC-002.4.

## Tests

Existing `changes-panel-tree.test.tsx` guards tree action wiring. Rendering and
action routing use Playwright rather than additional component markup tests.

## E2E tests

`task/mobile-changes-panel.spec.ts` (mobile-chrome) gains compact-row geometry,
menu/action flow, and breakpoint assertions for all four criteria. Existing
`git/git-changes-panel.spec.ts` (chromium) guards desktop staging and unstaging
and adds the 767px/768px fine-pointer composition transition.

## Work orders

- [x] [Task 01: Compact touch file rows](task-01-compact-touch-file-rows.md)

## Verification results

Completed: 8 mobile browser tests, 4 focused desktop browser tests, 6 tree unit
tests, typecheck, ESLint, Prettier, staged i18n ratchet, documentation catalog,
36 specification-linter tests, full spec lint, and diff checks passed. The
[work order](task-01-compact-touch-file-rows.md#results) records commands, the
baseline failure, and a temporary desktop-discovery override needed for this
worktree's name. Four inspected screenshots use disposable E2E data and are
published separately from the product branch.

## Risks

Menu close can race confirmation focus; pass the persistent trigger as anchor.
Tree indentation and long paths must not clip touch controls. Existing mobile
tests using inline Unstage must use the menu.
