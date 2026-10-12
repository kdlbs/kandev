---
id: "03-automatic-reconciliation"
title: "Reconcile restart inconsistencies automatically"
status: done
wave: 3
depends_on:
  - 02-abandoned-dispatches
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-003
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 03: Reconcile restart inconsistencies automatically

## Summary

Extend the existing bounded restart reconciler to repair exact provable SQL/journal/execution inconsistencies at startup and confirmed local runtime replacement. Discover affected records even with missing/stale recovery metadata, then use existing restore-only checkpoints and owner admission to recover the native conversation.

## Regression

Add `TestRestartRecoveryRepairsDispatchStateDisagreement` before the correction. The fixture must fail for the observed state mismatch or stale projection, then pass through the production recovery path.

## In scope

- Candidate inventory, all-unresolved-record comparison, exact CAS normalization, and crash-resumable journal/SQL checkpoints
- Repair retired in-memory/runtime rows only after exact obsolete-owner proof, preserving native resume tokens and workspaces
- Share bounded worker ownership across startup/replacement triggers; cancellation and one blocked candidate must not strand other candidates
- Use existing record reconstruction and terminal projection instead of fabricating missing outcomes

## Out of scope

- Office/automation scheduler bypass, live remote process replacement, queue drain, or fresh conversations when native state is missing

## Acceptance

- Backend restart and confirmed shared-runtime replacement recover matching SQL unknown/peer dispatching and dead execution rows without browser interaction, replay, or duplicate native load.
- A mixed candidate set and mixed unresolved records keep conflicts blocked while independent good sessions recover; unknown liveness and changed ownership remain blocked.
- Crashes after journal classification/retirement and before SQL completion converge from the pinned checkpoint; completed restoration is not rediscovered as unresolved work.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite ./internal/orchestrator -run 'Test(RestartRecovery|SilentRestore|Interrupted|DeliveryRecord|RetrySessionDelivery)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run 'Test.*(RuntimeReplacement|Recovery|Retired|NativeResume)' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/agentctl_facade.go`

- `apps/backend/internal/backendapp/adapters_native_resume.go`
- `apps/backend/internal/backendapp/adapters_acp_session_test.go`
- `apps/backend/internal/orchestrator/restart_block_binding.go`
- `apps/backend/internal/task/repository/sqlite/restart_block_binding.go`
- `apps/backend/internal/task/repository/sqlite/restart_block_binding_test.go`

- `apps/backend/internal/orchestrator/silent_restart_recovery.go`
- `apps/backend/internal/orchestrator/agent_delivery_submission.go`
- `apps/backend/internal/orchestrator/reconcile_liveness.go`
- `apps/backend/internal/orchestrator/interrupted_session_restart_test.go`
- `apps/backend/internal/orchestrator/silent_restart_recovery_boundaries_test.go`
- `apps/backend/internal/task/repository/sqlite/silent_restore.go`
- `apps/backend/internal/task/repository/sqlite/silent_restore_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/runtime_replacement.go`
- `apps/backend/internal/agent/runtime/lifecycle/runtime_replacement_test.go`

## Dependencies

02-abandoned-dispatches.

## Risks

Never substitute a retired client error or reused port for original-process death evidence. Keep inventory bounded and use every unresolved record, not the first match.

## Parallelism

`sequential`. No delegation is authorized.

## Inputs

- Linked platform requirements and system designs.
- Plan evidence and existing test patterns.
- Existing journal shutdown, native retirement, and silent restore packages.

## Results

The exact host-crash regression passes with both stores dispatching, missing metadata, an empty block, and an already loaded replacement conversation. Source process birth proof, authenticated complete SQL/journal comparison, and guarded partial-block binding repair preserve independent live or unknown owners. Startup and replacement-triggered bounded sweeps restore original native continuity without replay. Desktop and phone automatic recovery pass, including a second backend restart and a distinct new instruction. Final SQLite/PostgreSQL recovery regressions and backend lint pass. Delivery continues through PR checks and merge.

## CI review remediation

Consecutive crashes in a preserved harness generation now use distinct incident
checkpoints, while legacy checkpoints remain reusable for their pinned submission.
Recovery checks retained retirement evidence for prior SQL unknown outcomes rather than
requiring them in the unresolved descriptor. Missing-metadata reconstruction also validates
those exact canonical snapshots under its SQL write lock. Regressions cover consecutive
incidents with and without metadata, conflicting retired history, changed/live historical
rows, and a lost dispatch compare-and-set. Focused backend regressions pass under the race detector. CI browser validation remains pending.

CI shard 8 provided a prepared checkpoint whose recovery revision and identity were
unchanged while an identical block's updated timestamp advanced. A SQLite/PostgreSQL
regression reproduced its permanent identity conflict. Unclaimed checkpoints now accept
that timestamp advance while rejecting same-revision notice changes, timestamp rollback,
foreign owners, and every change after candidate allocation.
