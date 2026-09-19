---
created: 2026-09-05
status: completed
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
  - ../../specs/ui/system-design/pr-task-status-summary.md
legacy_specs: []
---

# Implementation Plan: Preserve PR Status Color Across Reload

## Overview

Make the persisted task PR summary use the same attention precedence as the
full PR record. An open PR with pending CI and blocked mergeability must remain
yellow in the sidebar, Kanban card, and mobile task switcher after a page
reload.

## Confirmed defect

The full PR renderer gives pending checks a yellow status before it considers
blocked branch protection. The backend summary projector returns `blocked`
before it evaluates pending checks. The frontend maps that compact aggregate
to gray.

The browser keeps full PR records only in memory. After a page reload, inactive
task rows first use the persisted compact summary. This data-source switch
exposes the precedence mismatch. The selected task can remain yellow because
its full PR record is loaded again.

The Kanban card also omitted the compact summary when rendering its PR icon.
While the full PR request was in flight after reload, the card therefore had no
fallback icon state at all.

## Scope

### In scope

- Make pending CI outrank blocked mergeability in the compact PR aggregate.
- Keep failure, pending, and review signals ahead of dirty mergeability.
- Keep multi-PR aggregate ranking aligned with the single-PR precedence.
- Treat a pending review without a passed CI signal as yellow, matching the
  full PR renderer.
- Preserve terminal, queued, draft, failure, review, ready, and passing rules.
- Add focused backend regression coverage for combined PR states.
- Pass the persisted compact summary to the Kanban PR icon while full PR data
  is unavailable.
- Keep the preview deploy command compatible with the current default-branch
  workflow's `--skip-description` flag.
- Prove reload parity in the desktop sidebar and Kanban card.
- Prove the same compact-summary color in the mobile task switcher.

### Out of scope

- Persisting full GitHub PR records in the browser.
- Changing PR icon artwork, color tokens, tooltip copy, or layouts.
- Changing GitHub polling, WebSocket delivery, or full-record hydration.
- Adding a new API field or aggregate state.

## Technical approach

### Summary precedence

Add a table-driven test beside the task status summary projector. Start with a
case whose checks are pending and whose mergeability is blocked. Assert a
`pending` aggregate. Include the adjacent failure case so blocked mergeability
cannot hide a stronger failure signal.

Update `pullRequestAggregateState` in
`apps/backend/internal/task/statussummary/projector_pr.go`. Evaluate the
stronger check and review signals before the general blocked- or
dirty-mergeability fallback. Require passed checks before classifying a PR as
awaiting review, and classify a pending review without a passed CI signal as
yellow. Align the multi-PR rank table with the same order. Keep terminal and
queued states first. Pass the compact task summary through the Kanban card's
existing `PRTaskIcon` fallback input. Do not change the aggregate schema or the
frontend color map.

### Desktop browser behavior

Extend `apps/web/e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts`. Seed an
inactive PR with pending checks and blocked mergeability. Assert that its icon
is yellow while the row uses the compact summary. Reload the page and assert
that the sidebar row remains yellow. Hydrate the full PR record and assert that
the color does not change.

Add the same reload assertion for the board icon in
`apps/web/e2e/tests/pr/pr-status-badge.spec.ts`. Scope the locator to the
Kanban board so the sidebar copy cannot satisfy the assertion. Hold the full
PR response until the compact-summary assertion finishes. Then release it and
assert that hydration keeps the icon yellow.

### Mobile behavior

Extend `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts` with the
same combined PR state. Open the existing mobile task switcher and assert a
yellow icon before full PR hydration.

This change does not alter mobile structure or interaction. The nearest mobile
exemplar remains
`apps/web/components/task/mobile/session-task-switcher-sheet.tsx`.

## Acceptance traceability

- `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.1`: the compact snapshot
  carries the correct visible PR status for desktop and mobile switchers.
- `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.2`: the backend derives the
  compact value from the authoritative stored PR state.
- `AC-UI-PR-TASK-STATUS-SUMMARY-001.5`: existing PR status precedence remains
  stable across the compact and full representations.
- `AC-UI-PR-TASK-STATUS-SUMMARY-001.8`: sidebar, Kanban, and mobile task rows
  show the same summary state.

## Work orders

- [x] [Task 01: Align compact PR status precedence](task-01-align-pr-status-precedence.md)

## Verification

```bash
cd apps/backend && go test -v -run TestPullRequestAggregateStatePrecedence ./internal/task/statussummary
cd apps/backend && go test ./internal/task/statussummary
cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-sidebar-hover-hydration.spec.ts -- --grep "keeps pending CI yellow"
cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-status-badge.spec.ts -- --grep "keeps pending CI yellow"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-status-summary.spec.ts -- --grep "keeps pending CI yellow"
```

Completed results:

- Focused projector precedence and multi-PR ranking: 10 tests passed.
- Full status summary package: 90 tests passed.
- Kanban component suite: 14 tests passed.
- Desktop reload regressions: 2 tests passed.
- Mobile task-switcher regression: 1 test passed.
- Production backend and E2E web builds completed.
- Frontend typecheck completed without diagnostics.
- `git diff --check` passed.

## Risks

- A broad reorder can change unrelated draft, terminal, or queued colors.
  Table-driven coverage must lock those boundaries before production changes.
- Browser coverage can pass on a full PR record and miss the compact path.
  The test must use an inactive task and assert before hydration.
- The summary is persisted. Existing rows receive the repaired aggregate on
  the next authoritative PR projection.

## Documentation impact

No public documentation or ADR change is required. The active requirements
already require one status result across summary consumers and full PR data.
This work repairs the implementation to match that contract.
