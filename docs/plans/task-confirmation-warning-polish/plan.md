---
created: 2026-08-24
status: complete
requirements:
  - ../../specs/ui/requirements/confirmation-warning-hierarchy.md
system_design:
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
legacy_specs: []
---

# Implementation Plan: Task Confirmation Warning Polish

## Overview

Reduce the visual weight of the shared still-working warning so archive and
delete confirmations keep title and primary cleanup copy as the dominant
hierarchy. Give the fine-pointer archive popover modest extra width and remove
its source sidebar-row layout jitter. Deliver shared style/placement changes,
regression coverage, and focused desktop and phone rendered evidence without
changing text or behavior.

## Scope

### In scope

- Compact shared warning typography, wrapping, spacing, and icon scale.
- Archive-only fine-pointer popover width opt-in with viewport containment.
- Fine-pointer archive mounting that keeps the originating sidebar row height
  stable while preserving coarse-pointer inline expansion.
- Regression assertions for the shared warning contract in archive and delete
  dialog tests.
- Regression assertions for archive width and fine-pointer sidebar row-height
  stability.
- Focused production-build desktop and phone checks, screenshots, computed
  hierarchy, containment, action reachability, and zero document horizontal
  overflow.

### Out of scope

- Warning copy, translations, in-flight detection, archive/delete callbacks,
  global confirmation-popover width, dialog geometry, focus, Escape, safe-area,
  action sizes, or animation.

## Technical approach

Change `apps/web/components/task/task-still-working-warning.tsx` for the shared
visual contract. Keep the existing semantic classes and markup identity; use
compact body typography with `text-pretty`, a readable explicit line height,
and proportionally smaller padding, gap, and icon.

Add a width/size contract to
`apps/web/components/confirmation/action-confirm-popover.tsx` with `w-64` as
the default. Pass the wider archive-only variant from
`TaskArchiveConfirmation`, with a viewport-aware max width. Keep title/body
pretty-wrapped and action geometry unchanged.

At `TaskItemWithContextMenu`, use `useResponsiveBreakpoint` to keep the
fine-pointer archive confirmation outside the `TaskItem` action slot while
retaining the same anchor/focus refs and shared confirmation node. Continue
injecting the node into `TaskItem` for coarse-pointer inline confirmation.
This targets the observed `flex-wrap` plus `basis-full` extra-line cause, not
the row's dimensions.

Add a style-hierarchy regression to the existing archive and delete warning
tests. Run it against current main first so the old `text-sm`, `p-3`, and 16px
icon contract fails, then make the shared style change and rerun the focused
tests green. No catalog changes are expected, but the affected-file i18n
ratchet/check remains part of verification.

Use the real desktop sidebar archive flow and the phone
sidebar-task-actions inline confirmation coverage. Rebuild the production web
assets before Playwright. Capture before and after screenshots of the full
desktop dialog/popover and phone inline confirmation with the warning visible,
and inspect computed font size/line height, warning-to-description hierarchy,
popover width and viewport bounds, source-row height before/open/cancel,
button reachability, and document `scrollWidth`.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-TASKS-CONFIRMATION-WARNING-001.1` | Archive and delete component warning tests assert compact class contract and existing warning text. |
| `AC-TASKS-CONFIRMATION-WARNING-001.2` | Existing archive/delete activity tests assert localized warning presence and `role=alert`; shared component keeps test ID and semantics. |
| `AC-TASKS-CONFIRMATION-WARNING-001.3` | Desktop card-action confirmation E2E plus phone inline confirmation E2E and rendered geometry checks. |
| `AC-TASKS-CONFIRMATION-SURFACE-002.1` | Action-confirm-popover width-contract test plus desktop sidebar computed width/bounds assertion; non-archive default remains `w-64`. |
| `AC-TASKS-CONFIRMATION-SURFACE-002.2` | Desktop sidebar E2E records exact source-row height before/open/cancel and fails on the current extra flex-line behavior. |
| `AC-TASKS-CONFIRMATION-SURFACE-002.3` | Desktop anchor/focus/callback checks plus phone inline 44px and zero-overflow checks. |

## E2E tests

- Desktop: `tests/task/archive-confirmation-preference.spec.ts` with a real
  sidebar task row, fine-pointer archive popover width/bounds, and
  before/open/cancel row-height measurements. Retain the existing
  `tests/kanban/card-menu-delete-archive.spec.ts` archive/delete behavior
  coverage and add full-dialog warning hierarchy checks where the seeded
  in-flight task can open it.
- Phone: `tests/task/mobile-sidebar-task-actions.spec.ts` existing inline
  confirmation scenario, extended only if needed to expose the warning state;
  preserve 44px actions, in-viewport checks, and zero document overflow.
- Manual rendered evidence: capture `.pr-assets` before/after screenshots for
  desktop full dialog and phone inline confirmation. Do not commit screenshot
  binaries to the feature branch.

## Work orders

- [x] [Task 01: Compact shared confirmation warning and stable archive surface](task-01-compact-warning.md)

## Verification results

- RED: the new archive/delete compactness assertions and wide-popover contract
  failed against the unchanged implementation (3 failed, 38 passed); the
  desktop row-height regression measured 54.203125px before vs 62.203125px
  while the old fine-pointer popover was open.
- GREEN: focused Vitest passed 5 files and 59 tests; affected ESLint passed;
  web typecheck passed; `i18n:ratchet` passed with 0 new violations and 644
  guard entries; `make build-web` passed.
- GREEN rendered checks against rebuilt assets: desktop archive geometry/full
  dialog 2/2, phone inline archive flows 2/2, and card archive/delete behavior
  5/5. Desktop assertions verified stable row height before/open/cancel,
  popover width above 256px, viewport containment, and zero document overflow.
  Phone assertions verified compact 12px/20px warning type, intentional inline
  row expansion, 44px-or-larger in-viewport actions, and zero overflow.
- Fresh compressed media is captured under `apps/web/.pr-assets/`; screenshot
  binaries remain uncommitted and are published through the PR media ref.

## Risks

- Shared styling affects delete as well as archive. Preserve the semantic
  yellow treatment and run both dialog suites.
- Longer locale strings may wrap differently. `text-pretty`, viewport checks,
  and the pseudo-locale build must remain clean.
