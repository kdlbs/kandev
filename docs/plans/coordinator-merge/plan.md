---
created: 2026-11-08
status: draft
requirements:
  - REQ-COORDINATOR-MERGE-001
  - REQ-COORDINATOR-MERGE-002
  - REQ-COORDINATOR-MERGE-003
system_design:
  - ../../specs/coordinator/system-design/merge.md
legacy_specs: []
---

# Implementation Plan: Coordinator merge stewardship

## Overview

After card 7480b441's answer proposal branch, deliver repository-scoped merge stewardship. The coordinator owns policy, proposals, wakes and projections; provider adapters own reads and merge dispatch. Work sequentially so the UI uses the completed guarded backend. This package does not authorize Build before the dependency is ready.

## Scope

In scope: provider state and protection observations, change wakes, repository policy, declared merge order, guarded merge proposals and outcome UI. Out of scope: pushes, releases, tagging, provider bypass and automatic edits to repository instructions. Inferred dependencies require manager confirmation.

## Technical approach

Extend coordinator policy, proposal executor, activity, turn context, Queue and phase 3 wake recorder under `apps/backend/internal/coordinator`; extend workspace-scoped GitHub and GitLab adapters for fresh observations, protections and expected-head dispatch. Persist policy, dependencies, observation episodes and dispatch evidence in both supported database dialects. Add `coordinatorMerge` to runtime flags and profiles. The answer work owns its own files and contract; reconcile its final branch before implementation.

| Provider | Identity and transport | Execution and fallback |
| --- | --- | --- |
| GitHub | workspace credential, host, repository and PR number | fresh checks, `MergePRRequest.ExpectedHeadSHA`; unknown status refuses, provider protections reported separately |
| GitLab | workspace credential, configured host, project and MR IID | extend `MergeMR` with expected SHA; unknown status refuses, provider protections reported separately |

The provider's current rules enforce what they can at acceptance. Kandev's separate reads are not atomic; settings and the ADR state the remaining window. Uncertain dispatch is reconciled without replay. A provider rejection lowers an automatic repository setting.

## ASCII UI preview

UI-01: coordinator settings, merge policy (AC-COORDINATOR-MERGE-002.1, .5):

```text
Desktop: Settings > Permissions
Repository       Merge policy        Provider protections
service-a        [Requires approval v] Pipeline: enforced; reviews: unknown
service-b        [Denied v]            Threads: not enforced   [Retry]
Conditions not enforced by the provider can change before merge acceptance.
[Save]

Phone: Settings > Permissions
service-a
Merge policy [Requires approval v]
Pipeline: enforced | Reviews: unknown
Threads: not enforced [Retry]
Conditions not enforced by the provider can change before merge acceptance.
[Save]
```

UI-02: Queue and proposal (AC-COORDINATOR-MERGE-001.1, -003.5):

```text
Queue: Task A
Predecessors: task B / PR #41 [Edit]
PR #42 -> main  head abc123
Pipeline: green  Conflicts: none  Threads: 0
Approvals: met  Draft: no  Behind: 0

Merge proposal: Task A / PR #42
Head abc123  Method squash
Merges cannot be undone here. [Approve] [Reject]
```

The phone uses stacked repository cards and the same Queue content, including the manager dependency editor, not a horizontally scrolling table; settings has one vertical scroll owner, safe-area clearance and 44px touch targets. Reuse the existing coordinator settings mobile composition in `e2e/tests/coordinator/mobile-watch-projects.spec.ts`, shared state and accessible controls. Error, loading and unavailable states retain the card and offer retry. Layout and spacing are illustrative; ordering, warning and accessible actions are requirements.

## Tests

Backend tests exercise each provider, unknowns, stale SHA, all ordered refusal conditions, concurrent approval, independent repository policies, lowering and dispatch uncertainty, wake recurrence and restart. Frontend tests cover policy and state presentation. Each work order names its tests per AC.

## E2E tests

Add `apps/web/e2e/tests/coordinator/merge.spec.ts` for policy, Queue status, approval, stale head and outcome, and `mobile-merge.spec.ts` for the same settings and approval value on `mobile-chrome` (001.1, 002.1, 002.5, 003.4, 003.5). Mock provider data; provider rejection and concurrency are covered by backend integration tests. Run `make build-web build-backend` before the targeted specs.

## Work orders

- [ ] [Task 01: State and wakes](task-01-state.md)
- [ ] [Task 02: Policy, dependencies and flag](task-02-policy.md)
- [ ] [Task 03: Guarded merge](task-03-merge.md)
- [ ] [Task 04: Manager surfaces](task-04-ui.md)

## Verification results

Pending implementation.

## Risks

Provider branch protections may not expose every rule and can change after observation. The provider's expected-SHA guard must be verified on both integrations. Ambiguous transport outcomes must not trigger a second request. Parallel answer work may change proposal schemas; rebase and reconcile before Build.
