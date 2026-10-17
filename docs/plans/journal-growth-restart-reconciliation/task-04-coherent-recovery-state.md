---
id: "04-coherent-recovery-state"
title: "Commit and publish restored recovery state"
status: done
wave: 4
depends_on:
  - 03-automatic-reconciliation
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Task 04: Commit and publish restored recovery state

## Summary

Make explicit native Resume and automatic restoration leave the same coherent revisioned recovery projection. Commit matching block resolution and restored metadata together after verified native identity and retained journal retirement, publish that snapshot, and prove normal composer and follow-up admission on desktop and phone.

## Regression

Add `TestNativeResumeCommitsRestoredRecoveryMetadata` before the correction. The fixture must fail for the observed state mismatch or stale projection, then pass through the production recovery path.

## In scope

- Identity/revision guarded SQL settlement with newer restored metadata and matching error cleanup, including already-retired/already-resolved retries
- Existing session event publication with complete metadata and cleared block projection before reporting recovery success
- Real-store integration and browser regressions for the stale uncertain banner, reload, independent errors, and distinct follow-up delivery
- Explicit recovery admission when concurrent workspace readers fail in the shared infrastructure slot, with ordinary admission fences preserved
- Review public recovery guidance at implementation; add only necessary behavior corrections

## Out of scope

- Rendered layout, new controls, localization copy, broad frontend rewrites, or clearing unrelated recovery causes

## Acceptance

- The real pre-fix Resume fixture returns success yet retains uncertain metadata; after correction the newer restored snapshot/block settlement is atomic and published, and stale notifications cannot revive it.
- Desktop and phone reload show ordinary usable chat after restart or explicit Resume; the old prompt remains uncertain and non-replayable, while a distinct instruction is accepted through normal admission.
- Failed or stale restore commits preserve newer/independent errors and blocks. Repeated Resume does not duplicate native restoration or historical dispatch.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite ./internal/orchestrator -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --host e2e/tests/session/agent-survival-restart.spec.ts e2e/tests/session/durable-stream-recovery.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run --host e2e/tests/session/mobile-agent-survival-restart.spec.ts e2e/tests/session/mobile-durable-stream-recovery.spec.ts --project=mobile-chrome)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/recovery_launch_coalescing.go`
- `apps/backend/internal/agent/runtime/lifecycle/recovery_launch_coalescing_test.go`
- `apps/web/e2e/helpers/restart-reconciliation.ts`
- `apps/web/e2e/helpers/native-resume-notice.ts`
- `apps/web/e2e/tests/session/durable-reattachment.spec.ts`
- `apps/web/e2e/tests/session/mobile-durable-reattachment.spec.ts`

- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/session_delivery_recovery.go`
- `apps/backend/internal/task/repository/sqlite/silent_restore.go`
- `apps/backend/internal/task/repository/sqlite/session_continuity.go`
- `apps/backend/internal/orchestrator/session_recovery_integration_test.go (new)`
- `apps/web/e2e/helpers/silent-restart-recovery.ts`
- `apps/web/e2e/tests/session/agent-survival-restart.spec.ts`
- `apps/web/e2e/tests/session/mobile-agent-survival-restart.spec.ts`
- `apps/web/e2e/tests/session/durable-stream-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-durable-stream-recovery.spec.ts`

## Dependencies

03-automatic-reconciliation.

## Risks

The stores cannot atomically commit journal retirement and SQL settlement. Require retained retirement proof at the SQL boundary. Preserve old recovery identity so frontend revision/error matching remains correct.

## Parallelism

`sequential`. No delegation is authorized.

## Inputs

- Linked platform requirements and system designs.
- Plan evidence and existing test patterns.
- Existing journal shutdown, native retirement, and silent restore packages.

## Results

SQLite/PostgreSQL tests pass for atomic restored metadata and block settlement, rollback, exact resolution proof, and preservation of unrelated or newer errors. Desktop and phone explicit native Resume pass through reload and normal follow-up. A deterministic regression reproduced recovery inheriting a concurrent workspace reader's unresolved-admission failure; the corrected shared-slot handling now passes while normal admission and unrelated or own failures remain authoritative. Desktop and phone automatic recovery also pass through another backend restart without replay. Final backend regression and lint checks pass. Delivery continues through PR checks and merge.

After main introduced compact session listings, native Resume verification reads rich session
detail explicitly and requires a nonempty original native identity. Compact rows remain valid
for state and membership checks, while omitted metadata never counts as restoration evidence.

The user requires all further browser/E2E validation to run in CI only because local
runs exhaust the host. No further local E2E runs are permitted. Duplicate crash/restart
scenarios were removed from survival specs; desktop and phone reattachment specs retain
their single owning scenarios. Recovery launch retries are bounded and respect cancellation
before admitting shared work. Bound delivery blocks with missing recovery metadata remain
open until reconstruction succeeds. CI checks and merge remain pending.

CI shard 5 exposed a lifecycle launch guard projected as interactive recovery despite
its unresolved durable delivery reason. The backend publication regression requires that
specific guard to retain the delivery consumer; other native failures retain their existing
consumer. The existing desktop/phone live-block E2E cases validate the full surface in CI.

CI backend shard 2 exposed an older native Resume fixture that supplied a delivery
block and unknown submission without canonical recovery metadata. The fixture now
includes its native generation and matching delivery identity. Successful Resume
asserts restored metadata and matching block-resolution timestamps while preserving
the unknown outcome and payload; launch/acknowledgment failures retain uncertainty.
Focused race checks covering every shared fixture case, reconstruction, checkpoint
refresh, and native recovery guards pass. This follow-up changes tests only; absent
production recovery metadata still fails closed until guarded reconstruction succeeds.
