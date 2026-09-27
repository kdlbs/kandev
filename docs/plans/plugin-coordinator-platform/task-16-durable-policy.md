---
id: "16-durable-policy"
title: "Reference proposals, reconciliation, and routine policy"
status: complete
wave: 16
depends_on: ["05-durable-input", "06-workspace-observations", "07-task-commands", "13-automation-destination", "15-reference-plugin"]
plan: "plan.md"
requirements:
  - REQ-PLUGINS-COORDINATION-011
acceptance_criteria:
  - AC-PLUGINS-COORDINATION-011.1
  - AC-PLUGINS-COORDINATION-011.2
system_design:
  - ../../specs/plugins/system-design/managed-coordination.md
---

# Task 16: Reference proposals, reconciliation, and routine policy

## Summary

Add durable decision policy inside the reference plugin. Make proposals, callbacks, and scheduled routines recoverable and customizable.

## In scope

- Implement SQLite proposals, memory records, watches, event cursors, and outbox. Persist intent before Host calls and reconcile their receipts after restart.
- Add proposal revision/approval compare-and-set, filtered/debounced callbacks, loop limits, pause, native recurring-routine destinations, concurrency limits, and soft measured/estimated usage budgets.
- Add proposal detail and policy controls with native mobile composition; test double approval, lost events, restart after effect, duplicate schedule delivery, and budget overshoot visibility.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- Duplicate approval or callback processing creates one durable intent and at most one delegated task.
- Reconciliation recovers missed events and uncertain receipts without inventing a new operation key.
- Pause, budget, and concurrency policy suppress new discretionary work while retaining history and accepted input.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview). These excerpts keep its stable labels.

### UI-05: Instance settings and proposals

```text
Desktop: Coordinator > Settings / proposal detail
+---------------------------------------------------------------+
| Instance: Delivery lead   Role: [Chief of staff v]              |
| Profile: [Supported agent v]  Executor: [Local v]               |
| Instructions: [User-defined coordination rules...]             |
| Scope: [Selected tasks v]   Budget: [USD 10 estimated]          |
| [Pause instance]                                    [Save]    |
| Proposal: Review fix  Revision 3   [Inspect] [Approve] [Reject] |
+---------------------------------------------------------------+
Phone: instance settings, separate full-height view
+------------------------------+
| < Delivery lead       [Save]|
| [Role v]                    |
| [Supported profile v]       |
| [Executor v]                |
| Instructions                |
| [User rules...]             |
| [Task scope v]              |
| [Budget and limits]         |
| [Pause instance]            |
+------------------------------+
```

Phone proposal detail is a full-height view with fixed Approve/Reject actions. Role/profile/executor/scope selection uses drawers. Unsupported profiles show a reason. Duplicate approval reads one receipt; stale proposals require inspection of the new revision. Pause preserves memory and pending inputs.

Applicable criteria: `AC-PLUGINS-COORDINATION-011.1`, `AC-PLUGINS-COORDINATION-011.2`.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd ../kandev-plugin-coordinator && go test -race ./server/... -run 'TestProposalOutboxRecovery|TestCoordinatorPolicyLimits' -count=1 && make test)
make -C ../kandev-plugin-coordinator KANDEV_SDK="$(pwd)/apps/backend" verify-package-host
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/reference-coordinator-policy.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-reference-coordinator-policy.spec.ts)
```

Evidence to create:

- `../kandev-plugin-coordinator/server/policy_test.go`: `TestProposalOutboxRecovery and TestCoordinatorPolicyLimits`.
- `apps/web/e2e/tests/plugins/reference-coordinator-policy.spec.ts`: `proposal approval, pause and schedule`.
- `apps/web/e2e/tests/plugins/mobile-reference-coordinator-policy.spec.ts`: `phone proposal inspection and approval`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `../kandev-plugin-coordinator/server/`
- `../kandev-plugin-coordinator/ui/`
- `apps/web/e2e/tests/plugins/reference-coordinator-policy.spec.ts`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 05](task-05-durable-input.md)
- [Task 06](task-06-workspace-observations.md)
- [Task 07](task-07-task-commands.md)
- [Task 13](task-13-automation-destination.md)
- [Task 15](task-15-reference-plugin.md)

## Risks

Callback loops can consume unbounded runs. Bound follow-up depth and report delayed usage; a soft budget is not a hard financial cap.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/managed-coordination.md) and [design](../../specs/plugins/system-design/managed-coordination.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

WO16 is complete. The coordinator now persists proposals, event cursors, memory,
watches, schedules, and Host-call receipts; it reconciles after restart, applies
revision-fenced approval, and bounds follow-up by pause, concurrency, and soft usage
policy. Coordinator recovery/limit tests, full package tests, vet, and
`verify-package-host` passed. Desktop and phone proposal-policy E2E passed, including
approval, pause, schedule creation, and routine management.
