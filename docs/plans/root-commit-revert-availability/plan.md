---
created: 2026-10-28
status: complete
requirements:
  - REQ-WORKSPACES-ROOT-REVERT-001
system_design:
  - ../../specs/workspaces/system-design/root-commit-revert-availability.md
legacy_specs: []
---

# Implementation plan: Root commit revert availability

## Overview

Hide Revert for a latest local commit only when its parent is explicitly absent. The frontend guard and its rendered regressions are complete. A subsequent real-producer diagnostic found that a freshly created root commit's event carries Git error text instead of an empty parent; the same work order now includes correcting that producer and its focused regression.

Qualification at baseline `a7509ba288ece945cdbce63fd8981f48ee5309e6` observed two root Revert controls in real `mergeCommits` and `CommitRow` rendered under HappyDOM with a menu primitive double; the ordinary selected-repository dispatch control passed. The independent backend qualification observed root `HEAD~1` failure and ordinary success. Neither is a native browser result. Materialized checkout and current main were both `d179ce206fd46c7793bda916b025d753036c7f3b` after the sidebar-only merge. Permanent tests must be authored independently of the private qualification fixtures.

## Scope

- In scope: local ancestry projection, row eligibility, independently authored regression tests, and focused native desktop/coarse/phone verification.
- Also in scope: accurate first-parent metadata on successful local commit notifications, including root and merge commits.
- Required CI remediation: establish the existing resolver singleflight test's intended overlap before releasing its real HTTP response; do not change the resolver.
- Out of scope: new root undo, Git history/ref changes, new copy, provider policy, broad Git refactors, and unrelated work.

## Technical approach

`changes-panel-helpers.ts` types for `LocalCommitForMerge`, `MergedCommit`, `mapLocalCheckoutCommit`, and `separateCommitHistories` retain optional `parent_sha`; `commit-row.tsx` `CommitItem` does likewise. Both local mappers already spread runtime fields. Guard the effective revert action once at `CommitRow` and use it for row/context actions and action-presence checks; preserve existing source, pushed, latest, amend, reset, and repository dispatch gates. Inspect both `changes-timeline-history-row.tsx` and `changes-panel-repo-groups.tsx` adapters without changing them unless evidence requires it. Follow the system design for transport and compatibility.

In `apps/backend/internal/agentctl/server/process/git.go`, replace the parent lookup used by `GitOperator.Commit` with a successful `%P` parent-list query and select the first parent, including empty output for a root. Preserve existing error handling; do not claim a failed query establishes ancestry. Independently test the real operator and tracker notification in a neighboring Go test. No Git ref, persistence, or wire contract changes are included.

## Rendered behavior

In the existing Changes commit row, root has Amend and Reset when otherwise eligible but no Revert. A parented latest local row retains all eligible actions; older, pushed, and provider-only rows keep current restrictions. Desktop context menu and hover actions, and the existing coarse/phone long-press menu, share that result. This is an eligibility change, not a new mobile composition. AC .1-.3 map to the rendered regression and browser checks below.

## Tests

Add `apps/web/components/task/root-commit-revert.test.tsx` using real mappers and `CommitRow`, independently constructed records, and a menu double only where required by HappyDOM. A causal root assertion must fail before the fix while an ordinary local parented commit dispatches its SHA and repository. Cover empty, parented, missing metadata, older, pushed, provider-only, same-SHA local/provider, unified and separate histories, handler-only empty-menu handling, Amend and Reset. Add mapping assertions in the same test if practical. Check both history adapters using actual rendered integration or focused existing test patterns.

An independently authored focused Go test must create a root, an ordinary child, and a merge with two parents, then assert the real tracker notification against Git's actual graph: empty for root and the first parent for both ordinary and merge commits. It must demonstrate root-specific RED before the producer fix and GREEN after it.

The unchanged `TestAgentctlResolverSingleflightsConcurrentDownloads` failed in Backend Tests (2/4) with two requests rather than one. Its HTTP response was released after the first request entered, without proving that the second caller joined the flight. The artifact does not reveal the actual interleaving, and the resolver and test match the base. Repair the test's overlap barrier using the existing observed-context pattern, with bounded waits and cleanup; retain both real callers and the one-request assertion. This tests coalescing of two joined waiters, not the separate stale-cache-miss race or the precise CI scheduling.

## E2E tests

Add focused `apps/web/e2e/tests/git/root-commit-revert.spec.ts` (chromium, including `coarseDesktopTestPage`) and `mobile-root-commit-revert.spec.ts` (mobile-chrome), sharing a small sibling helper only if needed. Use deterministic environment-keyed session commit state, open Changes, exercise actual desktop context/hover and real touch long-press menu, and prove root absence with ordinary and other-action controls. Desktop validates selected-repository dispatch separately in the component regression; browser tests verify actual surfaces. Build freshly through the managed runner. Capture only actual rendered UI if capture is used; report any infrastructure limitation rather than presenting a DOM double as browser proof.

## Work orders

- [x] [Task 01: Guard root commit revert](task-01-guard-root-commit-revert.md)

## Verification results

Frontend and producer checks are recorded in [Task 01](task-01-guard-root-commit-revert.md#results). The independent producer regression failed for root before the correction while ordinary and merge controls passed, then all three passed after the correction. A required CI failure in an unchanged lifecycle test exposed an unproven overlap precondition; the test-only synchronization correction and its adjacent control passed locally.

## Risks

- Empty and absent metadata have different compatibility meanings; null or default coercion can hide legacy actions.
- A test that substitutes the menu primitive does not establish native touch behavior. A stale build cannot establish current browser behavior.
- Git HEAD may move after the list was read; backend validation remains authoritative.
- A failed parent query cannot be distinguished reliably from known ancestry by the existing string contract. Preserve existing failure handling without claiming that this correction repairs it.
- A synchronized test does not establish which interleaving caused the earlier two-request observation or rule out a separate cache-miss race.

## Public documentation

`docs/public/git-operations.md` already describes the parent-dependent `HEAD~1` operation, and `docs/public/sessions-and-review.md` describes the selected-repository behavior without promising root undo. No public documentation change is needed for this narrow eligibility correction; revisit if implementation changes the operation or copy.
