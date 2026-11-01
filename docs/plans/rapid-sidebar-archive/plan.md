---
created: 2026-10-28
status: complete
requirements:
  - REQ-TASKS-REMOVAL-NAVIGATION-001
  - REQ-TASKS-REMOVAL-NAVIGATION-002
  - REQ-TASKS-REMOVAL-NAVIGATION-003
  - REQ-TASKS-REMOVAL-NAVIGATION-004
system_design:
  - ../../specs/tasks/system-design/removal-navigation.md
legacy_specs: []
---

# Implementation plan: Rapid sidebar archive

## Overview

Correct rapid individual sidebar archives through the existing removal coordinator.
One vertical work order covers eligibility, pending controls, and desktop/phone proof.
Tasks owns this contract because replacement eligibility and pending lifetime follow task removal.
This package extends the [requirements](../../specs/tasks/requirements/removal-navigation.md).

## Evidence and limits

A temporary real-store test reproduced this sequence:

1. Archive selected task A while the HTTP liveness read for replacement B remains pending.
2. Independently archive B before that read completes.
3. Return an active B from the delayed read.
4. Observe selection of B while B still has a pending removal token.

`rapid-sidebar-archive-diagnostic.test.ts` failed at the expected assertion:
`expected 'B' not to be 'B'`. The temporary file was removed.
Command: `(cd apps/web && pnpm exec vitest run hooks/rapid-sidebar-archive-diagnostic.test.ts)`.
The permanent regression belongs in `hooks/use-task-removal-coordinator.test.ts`.

The root cause is a stale candidate decision. Selection checks the captured operation's
exclusions and navigation revision, but omits newer independent pending removals.
Separate archive booleans in desktop and phone adapters also describe only the latest request.

The previous diagnostic pass ran three focused suites: 27 tests passed.
That baseline did not cover the delayed-candidate overlap above.
Retained request timings and a synthetic store probe do not identify the dominant CPU consumer.
This package makes no claim that the navigation correction removes the host CPU spike.

## Scope

Include current pending membership in replacement eligibility and per-task archive controls.
Preserve pending rows in place, confirmation, progress feedback, cascade checks, and authoritative reconciliation.
Use one canonical task model and the existing store-local removal state.

Exclude offline support, a new state library, a general command framework, a bulk endpoint,
request throttling, backend cleanup changes, and global event batching.
Profile the exact sidebar interaction before proposing further CPU changes.

## Technical approach

Follow the design's Rapid independent sidebar archives section.
Add pure selectors to `lib/state/task-removal.ts` and reuse them at candidate checks and UI adapters.
Recheck eligibility after liveness and session awaits, before any destination commit.
Keep user-navigation ownership separate from a candidate that becomes ineligible.
Continue existing candidate order without repeating a rejected candidate.

Keep dialog targets local. Derive archive busy controls from accepted removal membership.
Remove competing desktop/phone archive flags where that membership supplies the same fact.
Preserve callers of `useTaskMenuActions.pendingTaskId`, including `task-management-surface.tsx`.
Do not replace the shared removal coordinator or the existing refresh scheduler.

## ASCII UI preview

UI-01: A and B accepted; C remains usable. Maps to AC-001.6, AC-002.6, and AC-003.1.

```text
Desktop sidebar              Phone Tasks drawer (reopened)
[~ A          ]              [Tasks                 Close]
[~ B          ]              [~ A                       ]
[o C       ...]              [~ B                       ]
                             [o C                    ...]

A succeeds: remove A; B stays busy; C remains usable.
B fails: show B's current ordinary row; preserve later navigation.
```

The spinner, dimming, and stable pending-row space are existing behavior.
Spacing is illustrative. The main content never selects pending B as a replacement.
Phone keeps its fixed drawer header, single list scroller, safe areas, and visible action menu.
Acceptance closes the current phone menu; reopening shows the same operation state.
No composition or control-size change is planned.

## Tests

All short IDs use the `AC-TASKS-REMOVAL-NAVIGATION` prefix.

- `use-task-removal-coordinator.test.ts`: delayed B validation, delayed B session load,
  a pending candidate skipped for the next eligible task, no surviving candidate,
  later manual navigation, and a pending target in another workspace (001.6, 002.2).
- `use-task-removal-completed-removals.test.ts`: an independent archive, deletion,
  or delayed session reply that fully settles before the stale candidate reply
  arrives never becomes the destination (001.6).
- `use-task-removal-cascade-ancestry.test.ts`: a cascade removal observed during
  another archive's selection keeps its descendants out of the destination,
  including after release and immediately before the final switch (001.6).
- `task-removal-candidates.test.ts`, `use-task-removal.test.ts` and
  `use-task-removal-sidebar-fallback.test.ts`: retain recent-use order, authoritative
  liveness, bounded fallback pages, cascade ancestry, and a candidate that becomes
  pending during its liveness read (001.3, 001.6).
- `lib/state/task-removal.test.ts`: valid membership, stale tokens, workspace isolation,
  action filtering, and independent release (002.6).
- `use-task-menu-actions.test.ts`, `task-session-sidebar-archive-actions.test.tsx`, and
  `session-task-switcher-sheet-archive-actions.test.ts`: A completion does not clear B's
  controls, workspace isolation, duplicate suppression, and current task-management
  caller behavior (002.3, 002.6).
- `task-switcher-context-menu.test.tsx`: the row's own pending removal disables its
  archive action.
- Existing pending-row and progress-hook tests retain geometry, accessibility,
  early live events, error recovery, and progress lifetime (003.1-.3, 004.1-.3).

## E2E tests

Extend `task/sidebar-immediate-archive.spec.ts` and
`task/mobile-sidebar-immediate-archive.spec.ts` through their shared helper.
Retain their existing scenarios. Hold five individual archive routes before server processing.
Archive one row at a time through the real sidebar or phone menu.
Before release, prove independent busy rows, usable remaining actions, scrolling, and manual navigation.
Release mixed results out of order and prove that only the settled target changes (002.6, 003.2).

Add the selected-A/delayed-B scenario with a real backend candidate response.
If B becomes pending during the held read, B must never become the rendered destination (001.6).
Use causal waits and explicit route release, not sleeps. Inspect desktop and phone renders against UI-01.
Record an isolated Chromium performance trace of the rapid-click flow before and after correction.
Separate frontend trace results from unmeasured host CPU attribution.

## Work orders

- [x] [Task 01: Coordinate rapid sidebar archives](task-01-coordinate-rapid-archives.md)

## Verification results

Implemented on 2026-10-28 and review-corrected on 2026-10-29. The recorded trace totals below are single-run
interaction profiles, not a controlled CPU comparison.

### Reproduction and regression

- RED before the correction:
  `(cd apps/web && pnpm exec vitest run hooks/use-task-removal-coordinator.test.ts -t "independently pending archive")`
  failed both permanent regressions with `expected 'task-b' not to be 'task-b'`,
  the independently pending target was selected as the destination.
- GREEN after the correction: the same two tests pass, and the full coordinator
  suite passes.
- RED before the attempt-membership correction:
  `(cd apps/web && pnpm exec vitest run hooks/use-task-removal-completed-removals.test.ts)`
  failed all three cases with `expected 'task-b' to be 'task-c'`, because an
  independent archive or deletion that fully settled during the delayed read was
  selected. GREEN after the selection attempt retains observed removals.
- RED before the cascade-ancestry correction:
  `(cd apps/web && pnpm exec vitest run hooks/use-task-removal-cascade-ancestry.test.ts)`
  failed all four cases with `expected 'task-child' to be 'task-cached-candidate'`,
  because a server-page descendant of a cascade removal was selected. GREEN after
  cascade coverage is retained and candidate ancestry is checked immediately
  before the switch.
- Permanent regression names:
  `does not select another independently pending archive after a delayed candidate read`,
  `does not select another independently pending archive after a delayed session load`,
  and the completed-removal suite's archive, deletion, and delayed-session cases.

### Frontend checks

- Targeted suites:
  `(cd apps/web && pnpm exec vitest run lib/state/task-removal.test.ts hooks/use-task-removal.test.ts hooks/use-task-removal-coordinator.test.ts hooks/use-task-removal-completed-removals.test.ts hooks/task-removal-candidates.test.ts hooks/use-task-removal-sidebar-fallback.test.ts hooks/use-task-menu-actions.test.ts hooks/use-task-actions.test.ts components/task/task-session-sidebar-archive-actions.test.tsx components/task/mobile/session-task-switcher-sheet-archive-actions.test.ts components/task/task-item-archive-pending.test.tsx components/task/task-switcher-row-render-identity.test.tsx components/task/task-switcher-context-menu.test.tsx components/task/task-management-surface.test.tsx)`:
  15 files and 125 tests passed.
- Supporting suites (task commands, mobile sheet, sidebar rename ownership,
  task switcher, context-menu variants): 18 files and 149 tests passed.
- `(cd apps/web && pnpm exec tsc --noEmit)`: clean.
- `(cd apps/web && pnpm exec eslint <every changed source and test file>)`:
  clean, no warnings.
- `(cd apps/web && pnpm run i18n:check)`: passed; no new copy was added.

### Browser checks

- Desktop:
  `(cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/task/sidebar-immediate-archive.spec.ts e2e/tests/task/archive-task-redirect.spec.ts e2e/tests/task/sidebar-immediate-delete.spec.ts e2e/tests/task/threads-task-actions.spec.ts)`:
  14 tests passed.
- Phone:
  `(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-sidebar-immediate-archive.spec.ts e2e/tests/task/mobile-archive-task-redirect.spec.ts e2e/tests/task/mobile-sidebar-immediate-delete.spec.ts e2e/tests/task/mobile-threads-task-actions.spec.ts)`:
  12 tests passed.
- The extended archive specs hold five archive requests, archive one row at a
  time, prove independent busy rows, an enabled remaining action, scrolling, and
  manual navigation, then release mixed results out of order.
- `sidebar-immediate-archive.spec.ts` holds the real candidate read for the
  independently archived task and proves the rendered destination is the next
  eligible task. The phone flow accepts the second archive first because the
  phone surface replaces the list with its departure status; it asserts the same
  eligibility rule against a real backend candidate.
- After forwarding the second archive, both viewports await the archive response
  and the confirmed backend `archived_at`, reopen the phone picker when needed,
  and assert the viewport-aware row surface while an eligible row is visibly
  loaded. The removed row must be gone from that surface, not from a hidden
  desktop sidebar.

### Trace

A temporary, env-gated CDP trace harness recorded isolated Chromium traces of
the rapid flow on the pre-correction build and the corrected build:
`KANDEV_E2E_ARCHIVE_TRACE=1 pnpm e2e:run --host --project chromium
e2e/tests/task/sidebar-immediate-archive.spec.ts -- -g "rapid individual archives"`.
The harness was removed after recording. The generated traces lived under the
Playwright output directory and are not committed; the recorded totals are
below.

| Run | Test time | `RunTask` total | `FunctionCall` total | `Layout` total | `UpdateLayoutTree` total | `RunTask` over 50 ms |
| --- | --- | --- | --- | --- | --- | --- |
| Before | 31.4 s | 40.2 s | 13.6 s | 0.9 s | 4.4 s | 106 |
| After | 9.3 s | 6.4 s | 2.8 s | 0.1 s | 0.7 s | 16 |

The two runs were not load-controlled: the machine was shared with unrelated
Playwright work, and the recorded totals track the overall test duration rather
than one code path. These numbers are interaction evidence only. They do not
attribute the host CPU bottleneck, and this package makes no CPU-improvement
claim.

### Design and docs

- `python3 scripts/list-docs.py validate`: 413 decisions and 1802 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- `node scripts/validate-public-docs.mjs`: 54 pages validated.
- `git diff --check`: passed.

## Risks

- Candidate eligibility must be rechecked after every await; a guard only before
  HTTP would admit a task that becomes pending during session hydration.
- Treating lost navigation ownership as candidate rejection can trigger unwanted extra selection.
- Changing shared props can affect task-management, delete, and bulk callers.
- The host CPU cause remains unmeasured; a new scheduler can hide latency without reducing work.

## Documentation and related packages

Public docs now state that several archives can be pending at once, that each
row keeps its own spinner and duplicate protection, and that Kandev does not
switch to a task that is also being archived.
No new ADR is required because the existing state and lifecycle boundaries remain intact.

The completed [removal navigation](../task-removal-navigation/plan.md),
[pending archive](../immediate-sidebar-archive/plan.md),
[progress feedback](../archive-progress-feedback/plan.md), and
[pending delete](../sidebar-delete-loading/plan.md) packages remain historical records.
Keep their scenarios and recorded results; this work records its new checks here.
