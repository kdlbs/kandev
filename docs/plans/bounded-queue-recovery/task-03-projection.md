---
id: "03-projection"
title: "Bounded queue recovery: projection"
status: in_progress
wave: 3
depends_on: ["02-read"]
plan: plan.md
spec: ../../specs/tasks/requirements/bounded-queue-recovery.md
---

# Work packet 3: projection

## Scope
Implementation Step 3 in [plan](plan.md) defines owned files, dependency order, acceptance mapping and risks. Read REQ-TASKS-QUEUE-RECOVERY-001 and its [design](../../specs/tasks/system-design/bounded-queue-recovery.md). MAIN is sole writer.

## Acceptance
- Complete Step 3's mapped ACs with real evidence.
- Preserve exact scope and nonmutating bounded observation.
- Record missing/failed checks; PostgreSQL skips are not PASS.

## Verification
```sh
cd apps/backend && go test -count=1 ./internal/orchestrator/messagequeue -run 'Test(RecoverySnapshot|RecoveryCensus)'
```

## Risks and results
Census ownership confirmed and serialized hooks granted. Source implementation/checks are recorded in plan.md. PostgreSQL execution and independent verdicts remain pending. No overlap edits before ownership confirmation.
