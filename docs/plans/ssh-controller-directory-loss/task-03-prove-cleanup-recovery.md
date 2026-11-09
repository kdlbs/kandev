---
id: "03-prove-cleanup-recovery"
title: "Prove durable SSH cleanup and document recovery"
status: done
wave: 3
depends_on:
  - "02-reconcile-controller"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-EXECUTOR-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-EXECUTOR-001.18
  - AC-EXECUTORS-SSH-EXECUTOR-001.19
  - AC-EXECUTORS-SSH-EXECUTOR-001.20
  - AC-EXECUTORS-SSH-EXECUTOR-001.21
  - AC-EXECUTORS-SSH-EXECUTOR-001.22
  - AC-EXECUTORS-SSH-EXECUTOR-001.23
  - AC-EXECUTORS-SSH-EXECUTOR-001.24
system_design:
  - ../../specs/executors/system-design/ssh-agentctl-reconciliation.md
---

# Task 03: Prove durable SSH cleanup and document recovery

## Summary

Prove that the recorded receipt survives restart and cleanup retries. Exercise
the real SSH path and document the supported recovery and legacy limitations.

## In scope

- Drive existing durable cleanup with real SQLite/runtime inventory. Preserve
  receipt bytes through snapshot capture, reload, retry, and exact-owner settlement.
- Test one completed target beside an unresolved legacy or conflicting target.
  Later destructive cleanup must stay blocked. Preserve a successor runtime
  inserted before final settlement.
- Test a lost stop reply followed by confirmed absence, repeated cleanup,
  deadline failure, and shutdown with an agent child. Require child teardown
  before the runtime cleanup reports success.
- Add the plan's two real SSH/container E2E scenarios through the existing UI.
  Preserve sibling controllers and the task workspace. Use a disposable backend
  and fixture-owned remote processes only.
- Update the reference/explanation in `docs/public/executors.md` and link the
  supported outcome from its troubleshooting section. Explain legacy unresolved
  entries, unsupported pidfds, permission errors, and unconfirmed termination.
  Describe retry of the same operation after separately authorized intervention.

## Out of scope

New UI controls or mobile layout, new cleanup states, schema migrations,
automated operator intervention, and unrecorded-process sweeping.

## Acceptance

1. `TestSSHCleanupReconciliationRetainsMixedAndSuccessorTargets` proves that
   every target needs its own terminal evidence. Partial progress preserves
   remaining receipt/owner evidence and blocks destructive workspace cleanup.
2. `cleanup-reconciliation.spec.ts` proves original-process reconciliation after
   backend restart and session-directory loss. Its replacement scenario observes
   unresolved cleanup and preserves the replacement process and directory.
3. Public docs describe only behavior proved by these tests. All package
   cross-references and exact command results are recorded before delivery.

## Verification

Run from the repository root. Install dependencies once in a fresh worktree.
Run each command sequentially. The managed E2E runner rebuilds fresh artifacts.

```sh
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/service -run 'TestSSHCleanupReconciliation|TestStopTaskRuntimeTargets|TestExecuteTaskResourceCleanupJob' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestSSHLaunchReceipt|TestSSHDirectoryLoss|TestStopAgentWithReason.*PersistedSSH' -count=1)
pnpm --dir apps install --frozen-lockfile
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --shards 1 --project containers tests/ssh/cleanup-reconciliation.spec.ts tests/ssh/recovery.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` for this work order's changed Go files.
Run the PR documentation coverage preflight described in the plan's validation
record. If Docker or pidfd support blocks required evidence, keep this task
pending or in progress and record the concrete limitation.

## Files likely touched

- `apps/backend/internal/task/service/resource_cleanup_ssh_reconciliation_test.go` (new)
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_retained.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_ssh_restart_cleanup_test.go`
- `apps/web/e2e/tests/ssh/cleanup-reconciliation.spec.ts` (new)
- `apps/web/e2e/helpers/ssh.ts` only for a reusable owned-fixture helper
- `docs/public/executors.md`
- This plan and its work orders for status and exact evidence

## Dependencies

Task 02. Read `/e2e`, `/docs-maintainer`, the E2E README, and scoped guidance
before implementation. No rendered UI change is part of this work order.

## Risks

Fixture shutdown can hide a leaked remote process. Assert process identity,
exit, sibling preservation, and cleanup state before fixture teardown. Restore
every injected failure and remove only fixture-owned test resources.

## Parallelism

`sequential`

## Inputs

- [SSH requirements](../../specs/executors/requirements/ssh-executor.md), AC-001.18 through AC-001.24
- [Reconciliation design](../../specs/executors/system-design/ssh-agentctl-reconciliation.md)
- [Task cleanup contract](../../specs/tasks/system-design/runtime-cleanup.md) as a consumed contract
- `resource_cleanup_jobs_test.go`, `service_tasks_stop_test.go`, and `e2e/tests/ssh/recovery.spec.ts` as existing patterns

## Results

Implemented.

- `TestSSHCleanupReconciliationRetainsMixedAndSuccessorTargets` drives the real
  SQLite repository and runtime inventory. It proves the receipt survives
  snapshot capture and JSON reload byte for byte, that a legacy record acquires
  no invented proof, that one completed target beside an unresolved legacy
  target preserves only the unresolved one, and that a successor execution
  becomes the settlement target with its own receipt generation.
- `TestSSHStopRevalidatesExactOwnerBeforeEffects` and
  `TestSSHStopRecordChecksOwnerBeforeExternalEffects` prove the exact-owner
  boundary in the lifecycle manager: a successor execution, changed receipt,
  vanished row, or unreadable inventory fences settlement before any external
  effect.
- `apps/web/e2e/tests/ssh/cleanup-reconciliation.spec.ts` adds the two
  container scenarios: restart plus session-directory loss reconciles the
  original controller while a sibling stays intact, and a replacement
  directory plus its controller survive an unresolved archive. The success
  scenario waits for the exact executor row to disappear (durable settlement)
  in addition to controller exit. The refusal scenario waits for a
  target-correlated cleanup failure line naming the task and the pidfile
  evidence, not only the archive mutation. Both tests remove fixture-owned
  controllers and directories in `finally`.
- `docs/public/executors.md` documents the supported outcome, the unresolved
  legacy/permission/pidfd/termination cases, and tells operators to confirm
  controller identity before any manual action. It publishes no PID-only kill
  instruction.

Verification:

```sh
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run 'TestSSHCleanupReconciliation' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestSSHStop|TestSSHLaunchReceipt|TestSSHDirectoryLoss' -count=1)
node scripts/validate-public-docs.mjs
```

Container E2E evidence:

```sh
KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e:run --host --shards 1 --project containers tests/ssh/cleanup-reconciliation.spec.ts
```

Result: 2 passed in 1.4m. The first scenario observed the original controller
exit after backend restart plus session-directory loss, its durable executor
row disappear (exact-target settlement), and its sibling controller plus both
task workspaces stay intact. Because the helper now requires the controller's
recorded session children to drain, this also proves the agent child exited
before cleanup settled. The second waited for the target-correlated
cleanup refusal and observed the replacement directory and its controller
survive the unresolved archive. Both tests clean up fixture-owned state in
`finally`.

The E2E exposed one real interaction the unit tests could not: archive changes
the session state, which moved a pending remote-recovery snapshot and fenced
the explicit teardown before it reached the new route. The lifecycle stop now
re-reads the persisted exact owner when the snapshot fence is the only failure,
while an unrelated snapshot read error and any successor still fail closed. The
second run of the same spec passed both scenarios.
