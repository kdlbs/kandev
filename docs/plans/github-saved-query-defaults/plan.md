---
spec: docs/specs/ui/requirements/github-saved-query-defaults.md
created: 2026-08-07
status: completed
---

# Implementation Plan: GitHub Saved-Query Default Views

## Overview

Extend the existing saved-query JSON objects with an `isDefault` marker, keep
one default per GitHub result kind, and resolve that view on dashboard entry or
kind switches. Reuse the existing workspace/user settings queues and shared
desktop scope bar while keeping GitLab behavior unchanged.

## Architecture

1. A pure saved-preset model validates legacy data, enforces the per-kind
   invariant, mutates default markers, and resolves default search targets.
2. `useSavedPresets` continues to own workspace/user persistence and adds an
   acknowledged default mutation that publishes only after a successful write.
3. A dedicated selection hook applies initial and kind-specific defaults while
   preserving manual interaction against late hydration.
4. GitHub wrappers opt into default controls on the shared desktop scope bar;
   GitLab omits the optional contract. Mobile renders sibling 44px row actions.

## Mobile design contract

Desktop outcome is a set/clear action in the saved-query menu. Mobile entry
remains the existing GitHub filter sheet; nearest geometry reference is
`apps/web/components/kanban/mobile-menu-sheet.tsx`. The saved list owns vertical
scrolling, row selection remains primary, and star/delete controls are visible
siblings rather than nested row buttons. Shared state and mutations serve both
presentations. Mobile Playwright proves touch reachability, 44px geometry,
state isolation, reload persistence, and no document horizontal overflow.

## Tasks

- [x] [Task 01: Persist default saved queries](task-01-persist-default-saved-queries.md)
- [x] [Task 02: Apply defaults during selection](task-02-apply-default-selection.md)
- [x] [Task 03: Expose desktop and mobile controls](task-03-default-view-controls.md)
- [x] [Task 04: Document and verify behavior](task-04-docs-and-verification.md)

Execution is sequential in the primary conversation. No subagents authorized.

## Risks

- Saved settings hydrate asynchronously; a late response must not override a
  search or default action the user already performed.
- Full-array saved-query writes use existing last-write-wins queues; the new
  acknowledged mutation must not publish a marker before persistence succeeds.
- Shared scope-bar changes must remain optional so GitLab gains no new behavior.
- Legacy arrays may omit repository/default fields or contain duplicate defaults.
- Mobile sibling controls must remain touch-sized without horizontal clipping.
