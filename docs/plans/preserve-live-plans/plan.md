---
created: 2026-10-21
status: done
requirements:
  - REQ-TASKS-DOCUMENTS-003
system_design:
  - ../../specs/tasks/system-design/plan-write-lifecycle.md
legacy_specs: []
---
# Implementation Plan: Preserve observed live plans

## Overview

The refresh path unconditionally publishes `task.plan.get` and marks it seen
after independent live notifications publish newer state. Parent's update/delete
causal probes failed on 43774c11258e. User explicitly authorizes bounded
package/TDD implementation without a separate approval handoff. One sequential
work order, no delegation.

## Scope

Plan snapshot admission, immediate store bookkeeping, existing refresh callers,
read status/errors and real editor consumption. Excludes backend/schema,
transport ordering, Query migration, revision pagination, concurrent-writer
merge and dirty-draft policy changes.

## Technical approach

Session-slice-local pending tokens fence older snapshots after same-task reads,
`setTaskPlan`, or `clearTaskPlan`. Only admitted reads publish and mark seen;
read errors additionally require the originating task view. Accepted background
success survives navigation/unmount. No timestamp/server-version ordering.

## Tests

`use-task-plan.refresh.test.tsx` owns AC-TASKS-DOCUMENTS-003.4/.8: real request
transport boundary, hook/draft/store/registered notifications, live update,
delete/create, fresh success/null/error, overlapping consumers, isolated scopes,
navigation/unmount and clear. Existing save/draft/size suites protect .1-.7.

## E2E tests

Real panel/TipTap `task-plan-panel.snapshot.test.tsx` proves desktop/phone
consumption. This pure state/data correction qualifies for mobile-parity's
exception. No composition or controls change; no ASCII preview or new browser
coverage is needed and no unrun browser evidence is claimed.

## Work orders

- [x] [Task 01: Snapshot admission](task-01-snapshot-admission.md)

## Verification results

Permanent RED: four causal cases failed (nine assertions), then passed after
the guard. Final regression: 118 tests in 11 suites passed, including real
Plan panel/TipTap on desktop and phone. Changed-file ESLint, typecheck with
8 GiB Node heap, documentation catalog/spec lint, PR documentation coverage
preflight and diff checks passed. Exact commands and intermediate failures are
recorded in the work order. Normal commit hooks are required; their receipt accompanies the parent handoff.

Frozen install passed per retained
parent receipt; crash reconciliation found dependencies present and no surviving
owned test/build/install process. No incomplete check is counted as passed.
Public Plan editing/autosave instructions remain accurate; internal docs suffice.
Normal commit precedes parent actual-diff validation; no push/PR before approval.

## Risks

Admission must cover seen/loading/errors as well as content. Provider isolation,
background reads and own-save/comment identity contracts must be preserved.


## Review follow-up

Corrected the requirement-mapping table conjunction and clarified that the
creation test models an earlier absence response; the same token admission
rejects non-null stale data in the existing update/delete cases. Retained the
task-ID predicate: a previously captured refetch callback can be invoked after
navigation, when its closure task differs from the current view. Added a real
hook/store regression proving that neither starting nor failing that outgoing
read clears/replaces the current view's error. Temporarily removing only this
predicate failed both assertions; restoring production code passed. Production
logic is unchanged. Local targeted regression now contains 118 tests in the
same eleven suites. Remote CI/review evidence must be refreshed after the push.
