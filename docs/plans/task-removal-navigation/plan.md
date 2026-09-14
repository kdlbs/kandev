---
created: 2026-09-10
status: complete
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-001
  - REQ-TASKS-REMOVAL-NAVIGATION-002
  - REQ-TASKS-ARCHIVE-CONFIRMATION-001
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
  - ../../specs/tasks/system-design/archive-confirmation.md
legacy_specs: []
---

# Implementation plan: Task removal navigation

## Overview

Hide the outgoing task as soon as the user accepts archive/delete, then navigate
without exposing cleanup states. Build the shared operation logic first,
integrate the rendered surfaces second, and prove the complete desktop/phone
flows third. All work is sequential in the primary session.

Sources: [requirements](../../specs/tasks/requirements/removal-navigation.md),
[system design](../../specs/tasks/system-design/removal-navigation.md), and
[archive confirmation](../../specs/tasks/requirements/archive-confirmation.md).
The implementation package is complete.

## Scope

### In scope

- Shared local removal intent, guarded fallback navigation, and conditional recovery.
- Detail routes, board preview, sidebar, phone switcher, bulk actions, and existing
  task archive/delete entry points that invoke the shared hooks.
- Early HTTP/WS ordering, absent fallback, cascade exclusion, dirty conflicts,
  uncertain responses, duplicate submission, and later user navigation.
- Accessible loading/focus behavior and localized notifications.

### Out of scope

- Backend cleanup or API changes, permissions, discard-consent changes, undo,
  new archive settings, and general router redesign.
- Session-only deletion and Quick Chat lifecycle changes.
- Changing Office or remote/API/MCP removal UX. Preserve their existing
  lifecycle cleanup/refetch/redirect behavior with focused regression tests.
- New animations, desktop/mobile composition redesign, PR creation, or deployment.

## Technical approach

### Shared operation core

Add store-local operation transitions in proposed `apps/web/lib/state/task-removal.ts`.
Wire them into the existing store/slice types without persistence. Extend
`useTaskRemoval` / `useArchiveAndSwitchTask` into one action coordinator. Capture
the complete batch exclusion set, condition all asynchronous selection writes
on navigation revision, preserve layout/session ownership, and use SPA fallback.
Protect both direct and cascade deletion. Existing `switchOnly` tests must change
where they assert leaving the outgoing task visibly mounted.

### Presentation and caller integration

Place the removal boundary above live effects in `TaskPageContent` and the board
preview. Gate `useEnsureTaskSession` at enablement and dispatch. Preserve app
navigation while removing outgoing session/chrome content. Integrate
`useTaskCRUD`, desktop/phone action handlers, action-message delete, archive
confirmation callers, and sidebar bulk selection. Audit direct archive/delete
call sites to document any intentional exclusions, especially Quick Chat.

Task lifecycle WS handlers retain cache updates, storage cleanup, and Office
signals. Suppress only navigation already owned by a matching local departure.
Use proposed `task-removal-boundary.tsx` for shared neutral status if extraction
helps; this is a planned file, not an existing abstraction.

### Existing package reconciliation

[Cascade archive navigation](../cascade-archive-navigation/plan.md) already owns
descendant exclusion and final destination tests. Preserve those scenarios,
replace the old visible-wait assertion, and keep its historical results intact.
[Archive confirmation preference](../archive-confirmation-preference/plan.md)
remains complete; rerun its relevant desktop/mobile scenarios after integration.
This package records all new results rather than copying old passing counts.

## Tests

Names below are required scenario names to add or extend, not claims of existing tests.

| Acceptance criteria | Test file and scenario |
| --- | --- |
| 001.1, 001.2, 002.5 | `components/task/task-page-content.test.tsx`: outgoing subtree stays unmounted through early session/archive/delete events; no ensure dispatch |
| 001.3, 002.2 | `hooks/use-task-removal.test.ts`: safe workspace-scoped fallback; no candidate; stale validation/session response; leave-and-return revision |
| 001.3, 002.3 | `lib/state/task-removal.test.ts` (new): full batch/cascade exclusion; duplicate token rejection; per-target settlement |
| 001.4, 002.1 | `components/kanban-with-preview.test.ts`: immediate preview close; conditional restoration; unselected action preserves selection |
| 001.1, 002.1, 002.4 | `hooks/use-task-actions.test.ts`: intent precedes mutation; conflict rollback; uncertain success does not resurrect; notification ownership |
| 002.3 | `hooks/use-sidebar-multi-select.test.ts`: accepted batch protects active target before any mutation; partial failures remain selected |
| 002.2, 002.5 | `lib/ws/handlers/tasks-archive.test.ts` and `tasks.test.ts`: early lifecycle event cannot redirect a local departure; remote and Office behavior preserved |
| 001.1, 001.5 | `hooks/use-task-archive-confirm.test.ts` and dialog tests: cancellation and confirmation-free single execution |

All short IDs in this table expand to `AC-TASKS-REMOVAL-NAVIGATION-<ID>`.
Retain `AC-TASKS-ARCHIVE-CONFIRMATION-001.1` through `.7` as compatibility checks.
Use deferred requests with a real store for concurrency assertions. Each
implementation order starts with a behavioral RED test and records RED/GREEN.

## E2E tests

| File under `apps/web/e2e/tests/` | Project | Required scenarios and criteria |
| --- | --- | --- |
| `task/delete-task-redirect.spec.ts` | chromium | Active and last task; early session teardown before delayed response; dirty refusal; manual navigation before failure (001.1-.3, 002.1-.2, 002.5) |
| `task/archive-task-redirect.spec.ts` | chromium | Active and last task; cascade while a child is selected; recent descendant exclusion; slow destination hydration (001.1-.3, 002.2) |
| `task/mobile-archive-task-redirect.spec.ts` | mobile-chrome | Same archive departure through touch sheet; later navigation; no hard reload (001.1-.3, 001.5) |
| `task/mobile-delete-task-redirect.spec.ts` (new) | mobile-chrome | Active and last delete; failure restoration; sheet dismissal; status/focus and usable next task (001.1-.3, 001.5, 002.1) |
| `kanban/card-menu-delete-archive.spec.ts` | chromium | Preview removal and unrelated card control case (001.4) |
| `task/sidebar-multi-select.spec.ts` | chromium | Batch including selected task, no selected fallback, partial failure (002.3-.4) |
| Existing desktop/mobile archive preference specs | chromium / mobile-chrome separately | Confirmation disabled still protects the outgoing task; cancelled dialog leaves it intact (001.1, 001.5; archive compatibility) |

Install transport/DOM observation before acceptance. Hold HTTP responses with a
test-controlled release and assert the departure while they remain pending.
Observe outgoing-task subtree mounts/mutations throughout the interval, then
release in `finally`. Include an early WS event; the test must fail if only the
final destination is correct. Assert no extra outgoing `session.ensure` request.
Use fixture helpers, causal waits, and real isolated backend state; no fixed sleep.
Mobile checks use `.tap()`, existing phone project geometry, status accessibility,
and focus recovery. Capture a rendered phone state for visual inspection.

## Work orders

- [x] [Task 01: Coordinate local removal](task-01-removal-coordinator.md), wave 1, done.
- [x] [Task 02: Integrate departure presentation](task-02-departure-presentation.md), wave 2, done.
- [x] [Task 03: Prove removal transitions](task-03-removal-e2e.md), wave 3, done.

Exact runnable checks are in each work order. Fresh worktrees run
`(cd apps && rtk pnpm install --frozen-lockfile)` once before package commands.
E2E commands use the managed runner and rebuild the production assets.

## Verification results

Implementation checks completed on 2026-09-10:

- Coordinator unit coverage: 9 files, 86 tests passed, including the new
  `use-task-removal-coordinator.test.ts` suite (4 tests).
- Presentation unit/component coverage: 9 files, 94 tests passed.
- Web lint: passed.
- i18n checks and new-code ratchet: passed; 140 existing orphan catalog entries
  remain reported by the checker.
- Desktop removal E2E matrix: 23 tests passed on Chromium.
- Mobile removal E2E matrix: 5 tests passed on mobile Chromium.
- Targeted E2E sleep lint for changed files: passed.
- Specification tests: 36 passed; full specification lint passed.
- `rtk git diff --check`: passed.
- Repository typecheck remains blocked by pre-existing duplicate declarations in
  `apps/web/lib/types/http.ts` and a duplicate workflow object key in
  `apps/web/lib/ws/handlers/workflows.ts`. No changed file introduced those
  diagnostics.
- Repository-wide E2E-sleep lint remains blocked by existing violations and
  missing rule definitions outside this change; changed E2E files pass targeted
  lint.

The direct-call audit leaves the task list page as a list-only transport and
refresh flow. Quick Chat deletion and session-only deletion remain separate
lifecycle exclusions defined by the requirements and system design.

Design-package validation on 2026-09-10:

- `rtk python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `rtk python3 scripts/lint-spec-files.py --all`: passed.
- `rtk git diff --check -- docs/specs docs/plans`: passed.
- `rtk git status --short -- docs/specs docs/plans`: inspected; three pending
  work orders, the manifest, and the paired specifications are present.
- Read-only link/reference audit: passed for all six new documents, including
  AC references, work-order frontmatter, and system-design paths.
- Verification paths checked against the repository; new test files are marked
  as planned in the work orders. Product unit, typecheck, and E2E commands are
  specified for implementation and were not run during document authoring.

## Risks

- An early-return below data hooks cannot stop ensure-session effects; boundary
  placement and dispatch-time gating both need proof.
- ID-only rollback guards miss leave-and-return navigation. Revision ownership
  must cover route, workspace, task, session, and preview intents.
- Bulk requests and cascade ancestry can invalidate an apparently safe candidate.
  Exclude the full known set and reject uncertain lineage before committing it.
- Destination-fetch failure is not mutation failure. Keep outcomes separate.
- Broad WS suppression would break other tabs and Office; match only local
  departure ownership while preserving all cache and cleanup effects.
