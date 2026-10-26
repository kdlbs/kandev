---
id: "07-cleanup-quarantine"
title: "Cooperative cancellation and quarantine"
status: done
wave: 7
depends_on:
  - "06-admission-disconnect"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMAD-002
  - REQ-EXECUTORS-NOMAD-003
  - REQ-EXECUTORS-NOMAD-004
acceptance_criteria:
  - AC-EXECUTORS-NOMAD-002.2
  - AC-EXECUTORS-NOMAD-003.1
  - AC-EXECUTORS-NOMAD-003.2
  - AC-EXECUTORS-NOMAD-003.3
  - AC-EXECUTORS-NOMAD-003.4
  - AC-EXECUTORS-NOMAD-004.3
system_design:
  - ../../specs/executors/system-design/nomad-native-executor.md
---

# Task 07: Cooperative cancellation and quarantine

## Summary

Complete native positive and negative cleanup with actual managed processes. Persist a fenced successful drain receipt before whole-job stop and retain all uncertain outcomes.

## In scope

- Current cleanup claim obtains exact ServerIdentity seal/drain, persists resource/generation/operation/claim receipt, removes only owned bootstrap secrets and stops whole job purge=false. Confirm all exact allocations/tasks terminal on original responsive node before exact worker-fence compare-owner release and inventory release.
- Test cancellation/archive/delete/reset and failed-launch rollback, duplicate cleanup, stale receipt/claim, child/grandchild plus shell/LSP/Git/provisional owners and rejected post-seal starts.
- Fail/timeout drain, lose reply before/after checkpoint, crash/change runtime, lose node/history or add unexpected allocation. Assert no false absence, retained cleanup_pending and actual node ineligibility/interlock before terminalization.
- Document explicit externally verified operator readmission; no auto reboot/kill-all/re-enable shortcut.

## Out of scope

- Rendered UI, Docker/VM execution, plugin implementation, provider SDK/protobuf changes, model-backed workloads, and production rollout.

## Owner-retention integration

`ResetTaskEnvironment` and `teardownEnvironmentResources` in
`internal/task/service/service_task_environments.go` must recognize retained
Nomad environments before deleting their owner rows. An idle or ordinarily
stopped session can still own a live allocation. Add the corresponding Nomad
inventory guard to SQLite `DeleteTaskEnvironment`; absence of a local active
session is insufficient cleanup evidence.

Task deletion and cascade cleanup currently capture durable cleanup intent but
can cascade-delete the session/environment rows required by
`validateNomadExecutorCheckpointOwner`. Keep the canonical cleanup authority
available until the receipt and exact remote absence are checkpointed. Cover
reset, delete and cascade with retained ready/stopped inventory, cleanup failure
and successful cleanup; an orphan executor row alone does not satisfy this
contract. Reuse the existing durable cleanup jobs and reset claims.

## Acceptance

- Cooperative cancellation verifies managed child/grandchild groups absent before durable receipt and exact whole-job stop, then releases only correct inventory; repeated cleanup is safe.
- Missing/mismatched/stale evidence and runtime/node/history uncertainty never confirm absence; failures retain identities, quarantine and retry information through backend restart.
- Sanitized evidence exposes no secrets/payload; operator recovery requires exact external process absence and dedicated ownership before explicit readmission.

## Verification

All new paths/test names are future deliverables. Run the complete block from
any shell with the named absolute root and fixture-only config set. Use TDD for
changed logic; no-tests-to-run or a skipped required real scenario fails the
work order. Do not mark done based only on existing tests.

```bash
: "${KANDEV_ROOT:?Set absolute Kandev worktree path}"
: "${KANDEV_NOMAD_TEST_CONFIG:?Set absolute fixture-only JSON config path}"
(cd "$KANDEV_ROOT/apps/backend" && go test -race ./internal/agentctl/server/api ./internal/agentctl/server/instance ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/agent/nomad -run 'TestRuntimeSealAndDrain|TestRuntimeDrainAdmission|TestNomadCleanupEvidence|TestNomadWorkerQuarantine|TestNomadDestroyIdentity' -count=1)
(cd "$KANDEV_ROOT" && scripts/nomad-native-e2e run --require-real --scenario cleanup-positive --config "$KANDEV_NOMAD_TEST_CONFIG")
(cd "$KANDEV_ROOT" && scripts/nomad-native-e2e run --require-real --scenario cleanup-negative --config "$KANDEV_NOMAD_TEST_CONFIG")
```

## Files likely touched

- `apps/backend/internal/agent/nomad/destroy.go and tests (new)`
- `apps/backend/internal/agent/runtime/lifecycle/executor_nomad_cleanup.go and tests (new)`
- `apps/backend/internal/agent/nomad/integration/cleanup_test.go and process evidence fixture (new)`
- `scripts/nomad-native-e2e; docs/public/executors.md operator recovery`
- `Generic drain files from task 01 only for demonstrated matrix defects`

## Dependencies

[Task 06](task-06-admission-disconnect.md).

## Risks

Success is scoped to trusted managed groups. Terminal allocation, missing PID, closed port or ownership/shutdown response does not substitute for successful drain; runtime crash/daemonized escape remains quarantined.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/nomad-native-executor.md), using frontmatter IDs.
- [System design](../../specs/executors/system-design/nomad-native-executor.md), matching implementation boundary and current source.
- [Plan](plan.md), including strict real-worker runner/prerequisites.
- [ADR](../../decisions/2026-10-24-native-nomad-cleanup-evidence.md).
- Read applicable scoped `AGENTS.md` before editing code.

## Results

Complete (2026-10-26). Real-worker proof in `docs/evidence/nomad-wo07/`: run 7 passed `cleanup-positive` and `cleanup-negative` on build `ffc77bb52d` against a local throwaway Nomad fixture, after six earlier runs that each found a defect (listed there). The offline Verification block passes; on macOS run the `task/service` tests with `TMPDIR=/private/tmp`.

## Implementation notes (offline complete, live proof pending)

- Owner decisions: plain `session.stop` stays transport-only; archive, delete, reset and launch rollback run cleanup. The claim id is folded into the receipt digest only. Readmission is `POST /api/v1/executors/nomad/readmit` (admin only), moving a retained row to the terminal `operator_released` phase; it never writes `absent`, never changes eligibility or the marker.
- Cleanup order: checkpoint `cleanup_pending`, seal and drain with the recorded server identity, persist receipt digest, remove the staged bootstrap env file, stop the job with purge=false, confirm one terminal allocation on the original ready node, checkpoint `absent`, release the fence by owner compare, release inventory. Any failure leaves `cleanup_pending` and quarantines the node; recovery never drains again.
- Owner rows are retained by service teardown, sqlite environment and task delete guards, and the task delete cascade.
- Deferred: absent tombstones are re-resumed on each boot (idempotent, one SSH dial per row); a persisted released marker needs an `EnvironmentState` field.
- Live runner: `scripts/nomad-native-e2e run --require-real --create-fixture --scenario cleanup-positive|cleanup-negative --config "$KANDEV_NOMAD_TEST_CONFIG"`.
- Live run 2 (cleanup-positive) showed the task-tree handoff stops (`cascade archive`, `task tree archived`, any bare force) racing the durable cleanup job. `NomadExecutor.StopInstance` is now transport-only for those reasons; only the claim-holding cleanup job destroys, and it re-reads the inventory row after taking the claim. The failed-run path of the WO06/WO07 runner now stops its own backend and git daemon through their owned records and records `stopped_on_failure`; the Nomad job, node state and quarantine stay for `cleanup-fixture`.
- Live run 3 showed the archive cleanup job deleting the `executors_running` row after the `absent` checkpoint. Contract: the row is the absent tombstone and keeps the drain receipt digest and generation until the environment is deleted. `performTaskCleanup` skips Nomad inventory rows only for the archive job (`taskEnvironmentCleanup.keepNomadInventory`, set from `job.IsArchive()`); task deletion still removes the row with its owner, and SQLite guards refuse deleting a row in a retained phase. "Row gone" is never absence evidence. `task_environments.status` is a generic lifecycle field and stays `ready` after archive cleanup; the inventory phase, not that column, is the cleanup evidence, and no consumer may read it as such.
- Live run 4 (cleanup-negative) froze the worker leader before archive and showed the handoff stop blocking on the agentctl client's 60s default and shutdown's agent wait ignoring its deadline. The agentctl stop on a Nomad execution is now bounded by `nomadAgentctlStopTimeout` (10s), `StopAllAgents` returns at its context deadline, and the runner's failed-run stop escalates to SIGKILL of the recorded process group only after the registry verified the leader identity (`stop_owned_process`).
- Test environment: the `task/service` archive and cleanup tests that build real worktrees (for example `TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity`) fail on macOS with the default `TMPDIR` under `/var/folders` because the no-follow worktree open rejects the `/var` symlink (`not a directory`). They fail identically on `codex/nomad-executor` and pass with `TMPDIR=/private/tmp`; run them that way on macOS.
- Live run 5 passed every product step; the operator readmission step lost a race in `_default_kill_exact` when a tree member exited between the identity check and `kill -KILL`. A failed kill now re-checks the recorded identity: gone returns `already_gone`, the same identity still present raises, and a reused pid raises a mismatch. Kill order stays deepest first, leader last.
- Live run 6 showed operator readmission refused (500) while the archive cleanup job was in `retry_wait`, because the task cleanup barrier rejected any claim without that job's id. Contract: readmission only checkpoints `operator_released` and starts nothing, so its claim is admitted past the task cleanup barrier (`TaskEnvironmentRecoveryClaimRequest.OperatorReadmission`). Exclusion against a running cleanup attempt stays the unique per-environment claim, and a busy claim is reported as `nomad.ErrNomadReadmitBusy`, HTTP 409 with a retryable message. The retrying job then finds no destroyable row and finishes; the row stays `operator_released`. No automatic clearing is added: readmission remains an explicit admin request with an attestation.

