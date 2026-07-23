# Multi-PR Review selector

**Spec:** [Multi-branch tasks](../../specs/tasks/multi-branch/spec.md)

## Outcome

Replace Review's implicit `prs[0]` behavior with a task-scoped selected PR. Keep one coherent PR revision visible at a time, preserve local/committed source precedence, and make selection reachable on desktop, phone, and tablet.

## Decisions

- Default to `getPrimaryTaskPR()` for compatibility.
- Store only an in-session override keyed by task ID and stable owner/repo/number key.
- Do not aggregate sibling PR diffs; review and comment identity is not PR-qualified.
- Exact PR timeline rows carry PR identity through desktop and mobile diff routing.
- No production backend change.

## Waves

### Wave 1

- [Task 01 — selection foundation](task-01-selection-foundation.md) — done

### Wave 2

- [Task 02 — Review selector UI](task-02-review-selector-ui.md) — done; depends on Task 01
- [Task 03 — inline review and PR routing](task-03-inline-review-routing.md) — done; depends on Task 01

Tasks 02 and 03 own disjoint production files and may run in parallel after Task 01.

### Wave 3

- [Task 04 — E2E, QA, and verification](task-04-e2e-verification.md) — done; depends on Tasks 02 and 03

## Verification

From `apps/`, run focused Vitest files listed in each task. From `apps/web/`, run the two focused E2E specs. Finish from repository root with `make fmt`, then `make typecheck test lint`.

## Risks

- Switching clears PR files while fetching; Review must not auto-close during that transient empty state.
- Deferred requests must not flash files from the prior PR.
- Same-path review marks/comments remain current-visible-revision semantics, not independent per-PR history.
- Review toolbars need intentional phone wrapping, 44px touch targets, one content scroller, and no horizontal page overflow.
