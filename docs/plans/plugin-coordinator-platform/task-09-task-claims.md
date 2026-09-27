---
id: "09-task-claims"
title: "Task management claims and human takeover"
status: complete
wave: 9
depends_on:
  ["01-exact-host-foundation", "06-workspace-observations", "07-task-commands"]
plan: "plan.md"
requirements:
  - REQ-TASKS-COORDINATION-001
acceptance_criteria:
  - AC-TASKS-COORDINATION-001.1
  - AC-TASKS-COORDINATION-001.2
  - AC-TASKS-COORDINATION-001.3
system_design:
  - ../../specs/tasks/system-design/coordination-controls.md
---

# Task 09: Task management claims and human takeover

## Summary

Implement optional task management claims independently of worker assignment. Add native inspection and explicit human takeover.

## In scope

- Add claim schema, compare-and-set domain commands, fencing generation, exact Host wrappers, and audit history.
- Guard every plugin task-management command and queued effect against the current claim generation. Keep human mutations available with audit.
- Add manager row and transfer/release controls in task detail for desktop and phone. Test disabled/uninstalled owner recovery.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- Two plugin instances cannot acquire or mutate through the same active claim generation.
- Transfer invalidates in-flight former-owner writes and does not alter worker assignment.
- A human can inspect, transfer, and release an unavailable owner without a plugin callback.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview). These excerpts keep its stable labels.

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

Applicable criteria: `AC-TASKS-COORDINATION-001.1`, `AC-TASKS-COORDINATION-001.2`, `AC-TASKS-COORDINATION-001.3`.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/task/service ./internal/plugins -run 'TestTaskManagementClaim|TestExactTaskCommands' -count=1)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/task-management-claims.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-task-management-claims.spec.ts)
```

Evidence to create:

- `apps/backend/internal/task/service/management_claims_test.go`: `TestTaskManagementClaimFencing`.
- `apps/web/e2e/tests/plugins/task-management-claims.spec.ts`: `claim conflict and human takeover`.
- `apps/web/e2e/tests/plugins/mobile-task-management-claims.spec.ts`: `phone transfer and release`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/task/models/`
- `apps/backend/internal/task/repository/`
- `apps/backend/internal/task/service/`
- `apps/backend/internal/plugins/host_write.go`
- `apps/web/components/task/`
- `apps/web/src/locales/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)
- [Task 06](task-06-workspace-observations.md)
- [Task 07](task-07-task-commands.md)

## Risks

Claim checks outside a mutation transaction allow a former owner to race a transfer. Fence queued effects as well as immediate writes.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/coordination-controls.md) and [design](../../specs/tasks/system-design/coordination-controls.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Implemented task-owned claims with compare-and-set generations, exact Host
commands, native transfer/release and history, legacy v1 write fencing, durable
queue-generation checks, WIP-deferred move checks, and desktop/phone claim UI.
Human mutations remain available and retain their audit actor. The phone claim
surface is placed below the fixed navigation bar. Manager and completion status
share a compact phone toolbar, preserving the 44px Manage/Inspect actions while
keeping the session composer above the bottom navigation.

Verification passed:

- `go test -race ./internal/backendapp ./internal/task/service ./internal/task/repository/sqlite ./internal/orchestrator/messagequeue ./internal/plugins -run 'Test(TaskManagementClaim|ExactTaskCommands|ExactQueueMessageCannotDispatchAfterManagerChange|ExactWIPMoveDoesNotPromoteAfterManagementTransfer)|TestPluginsMessenger_LegacyMessageRejectsActiveManagementClaim|TestPluginsTaskWriter_LegacyMove' -count=1`.
- `go test ./internal/backendapp` and `go test ./internal/orchestrator/messagequeue`.
- Focused service and SQLite regressions for queue-promotion metadata and
  rejecting a deferred WIP move after claim transfer.
- `pnpm run typecheck && pnpm run i18n:check`.
- Desktop claim E2E passed in the preceding verification session. The current
  phone claim E2E passed through the compact toolbar; it asserts the toolbar
  stays at or below 60px and keeps the Manage target at least 44px.
- Public documentation tests and page validation (62 tests, 47 pages),
  specification catalog validation (306 decisions and 1,162 specifications),
  full specification lint, and `git diff --check`.

The first broad SQLite/task-service rerun hit disk exhaustion and exposed two
promotion tests that lost their pending promotion metadata. After preserving
caller-prepared metadata while refreshing the persisted claim fence, both
promotion regressions and the claim-transfer WIP regression passed. The shared
Go build cache was cleared to recover disk space; it is regenerable.

### Review remediation (2026-09-27)

The Host now sends installation attribution separately from any management-owner
assertion. Never-claimed and released tasks accept exact updates with the Host's
authenticated installation, while active competing owners and stale claim
generations still conflict. Real Host-to-SQLite coverage and a service test that
synchronizes effects against transfer/release passed under the race detector.
