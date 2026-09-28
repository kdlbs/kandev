---
id: "17-reference-operations"
title: "Reference adoption, evidence, writeback, and outcomes"
status: complete
wave: 17
depends_on:
  - "09-task-claims"
  - "10-completion-gates"
  - "11-workspace-admin"
  - "12-source-writeback"
  - "15-reference-plugin"
  - "16-durable-policy"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-011
acceptance_criteria:
  - AC-PLUGINS-MANAGED-COORDINATION-011.3
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 17: Reference adoption, evidence, writeback, and outcomes

## Summary

Complete the reference coordinator operational tools using optional Host capabilities. Keep task completion and ownership enforced by native services.

## In scope

- Add explicit task adoption, criteria proposal/evidence submission, optional workspace administration, and linked-issue writeback tools.
- Build outcome reporting from authoritative task/execution/usage evidence, including uncertain external writes and stale verification.
- Exercise workflow completion through normal UI and automation, human claim takeover, and separately approved issue-write permissions.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- A coordinator adopts work and submits versioned evidence while host gates block stale completion.
- Optional administration and issue writes require their own capabilities and surface uncertain outcomes.
- Outcomes distinguish verified/claimed completion and measured/estimated/unknown usage on desktop and phone.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview). These excerpts keep its stable labels.

### UI-02: Workspace conversation

```text
Desktop: plugin navigation > Coordinator
+---------------------------------------------------------------------+
| Product workspace   [Delivery lead v] [Settings] [Pause]              |
| Outcomes: 8 completed  1 blocked  Usage: USD 2.10 measured             |
+----------------------------------+----------------------------------+
| Chat                             | Tasks  [Search] [Filters]        |
| You: follow up the review        | Blocked                          |
| Agent: proposal ready            | [Task A] Waiting for answer      |
| [Proposal: Review fix] [Approve] | Running                          |
|                                  | [Task B] Implementing            |
| Input queued: 1  [Cancel]         | Completed                        |
| [Message composer]        [Send] | [Task C] Verified                |
+----------------------------------+----------------------------------+
Phone: plugin navigation > Coordinator
+------------------------------+
| < Product [Delivery lead v] : |
| [Chat] [Tasks] [Outcomes]     |
+------------------------------+
| Chat messages                |
| Proposal: Review fix         |
| [Inspect] [Approve]          |
| Input queued: 1 [Cancel]     |
+------------------------------+
| [Message]             [Send] |
+------------------------------+
Phone instance/filter selection: bottom drawer
+------------------------------+
| Choose instance       [Done] |
| (o) Delivery lead            |
| ( ) Reviewer                 |
| [Add instance]               |
+------------------------------+
```

Desktop split composition and phone tabs are required; widths and wording are illustrative. Header/composer are pinned, only the active tab body scrolls, and selectors use bottom drawers. Empty: create an instance. Loading: retain the shell. Disconnected: retain draft and retry identity. Paused: show retained input count and Resume. Revoked/unsupported: explain the blocked capability and link to host settings. Pending human interactions use native shared controls.

### UI-03: Task management and completion

```text
Desktop: task detail > management and completion
+----------------------------------------------------------------+
| Manager: Delivery lead                       [Transfer] [Release]|
| Completion: 1 of 2 verified                                      |
| [ok] Regression test passes     [Evidence]                        |
| [!] Review resolved             [Inspect blocker]                |
| Cannot complete: current review evidence is missing               |
| [Record human override...]                      [Complete: off]   |
+----------------------------------------------------------------+
Phone: task detail > completion
+------------------------------+
| Manager: Delivery lead [Manage] |
| Completion: 1 of 2    [Inspect] |
+------------------------------+
```

Show claims independently from the worker assignee. On phone, the manager and completion summaries share one compact toolbar below the fixed top bar; both 44px actions open their native bottom drawers. Desktop retains full detail rows and dialogs. An override requires a reason and targets one observed move. Stale evidence and an unavailable manager remain visible; no hidden automatic takeover.

Applicable criteria: `AC-PLUGINS-MANAGED-COORDINATION-011.3`.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd ../kandev-plugin-coordinator && go test -race ./server/... -run TestCoordinatorOperationalTools -count=1 && make test)
make -C ../kandev-plugin-coordinator KANDEV_SDK="$(pwd)/apps/backend" verify-package-host
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/reference-coordinator-operations.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-reference-coordinator-operations.spec.ts)
```

Evidence to create:

- `../kandev-plugin-coordinator/server/operations_test.go`: `TestCoordinatorOperationalTools`.
- `apps/web/e2e/tests/plugins/reference-coordinator-operations.spec.ts`: `adoption, evidence, issue writeback and outcomes`.
- `apps/web/e2e/tests/plugins/mobile-reference-coordinator-operations.spec.ts`: `phone blockers and outcomes`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `../kandev-plugin-coordinator/server/`
- `../kandev-plugin-coordinator/ui/`
- `apps/web/e2e/tests/plugins/reference-coordinator-operations.spec.ts`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 09](task-09-task-claims.md)
- [Task 10](task-10-completion-gates.md)
- [Task 11](task-11-workspace-admin.md)
- [Task 12](task-12-source-writeback.md)
- [Task 15](task-15-reference-plugin.md)
- [Task 16](task-16-durable-policy.md)

## Risks

Agent prose must not become verification or user consent. Preserve actor and subject provenance in every operation and report.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

WO17 is complete. The packaged coordinator supports explicit adoption, claim-aware
task operations, versioned completion criteria/evidence, workspace administration,
linked Jira/Linear writeback, retry receipts, and authoritative outcome reports. The
coordinator package tests/vet and the required Host/source-writeback race and adapter
tests passed. Final desktop and phone E2E passed for adoption, evidence, and outcome
reporting; desktop also verified Jira comments and transitions. The package manifest
declares the exact `usage` read grant required by outcome reports, and the refreshed
archive passed host-package validation.
