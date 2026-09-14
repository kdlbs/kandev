---
created: 2026-09-05
status: implemented
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
legacy_specs: []
---

# Implementation Plan: Draft PR icon color

## Overview

Move the GitHub task-row draft check ahead of non-terminal review and CI failure
checks in the existing `getPRStatusColor` precedence. Add a focused unit
regression and a rendered desktop scenario, then run the existing mobile task
status scenario to confirm that the passive mobile indicator remains intact.

The bug is localized to the shared frontend display helper. No backend state,
API payload, PR status chip semantics, queue behavior, or mobile composition
changes are required.

## Scope

### In scope

- Keep an open draft PR's task-row icon muted when CI reports failure.
- Preserve terminal-state and active-merge-queue precedence.
- Preserve red coloring for a non-draft PR with failing checks.
- Cover the pure helper and the rendered sidebar icon.

### Out of scope

- Changing the CI-specific `PRStatusChip` status or its failure messaging.
- Changing merge readiness, merge actions, queue state, backend projections, or
  provider synchronization.
- Adding a new mobile interaction or changing the existing task-row geometry.

## Technical approach

`getPRStatusColor` already owns the task-row icon color and `isPRDraft` already
recognizes an open draft through `mergeable_state === "draft"`. Keep terminal
states and `isPRQueued` ahead of the draft branch, then evaluate draft status
before the combined `changes_requested`/failed-CI branch. This is the minimum
precedence correction and leaves all later mergeability and readiness branches
unchanged.

The mobile design contract is unchanged because this is content-only styling:
the task-switcher row remains the primary touch target, the existing PR status
drawer remains the detailed mobile surface, and no new hit area or scroll owner
is introduced. The existing `mobile-task-status-summary.spec.ts` is the nearest
mobile proof for the passive indicator and no-overflow behavior.

## Tests

- `apps/web/components/github/pr-task-icon-draft.test.ts` will prove that an
  open draft with `checks_state: "failure"` returns
  `text-muted-foreground`, while the existing non-draft failure coverage keeps
  the red behavior.
- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts` will prove the actual sidebar
  icon class for a seeded draft PR with failing CI.

## E2E tests

- Desktop Chromium: extend the PR status badge suite with the draft/failing-CI
  scenario, mapped to `AC-UI-PR-TASK-STATUS-SUMMARY-001.20`.
- Mobile `mobile-chrome`: run the existing task-status-summary scenario. It
  already covers the shared PR indicator inside the task-switcher row, normal
  task navigation, and document horizontal containment; no new mobile spec is
  needed for this content-only color change.

## Work orders

- [x] [Task 01: Restore muted draft PR icon precedence](task-01-draft-pr-icon-color.md)

## Verification results

- Focused frontend unit tests passed: 2 files, 72 tests.
- Desktop Chromium E2E passed: 1 draft PR icon regression test.
- Mobile Chromium E2E passed: 1 existing task-status-summary test.
- Frontend TypeScript typecheck passed.
- Specification lint and `git diff --check` passed before implementation.

## Risks

- Moving the draft branch must not move it ahead of terminal states or an
  active queue entry, which have stronger lifecycle meaning.
- `PRTaskIcon` is shared by sidebar, Kanban, and rich task-list surfaces, so the
  helper regression must remain independent of a single mounting surface.
- The CI status chip intentionally remains failure-oriented; changing it would
  hide information outside this request.
