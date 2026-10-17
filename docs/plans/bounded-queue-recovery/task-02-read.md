---
id: "02-read"
title: "Bounded queue recovery: read"
status: in_progress
wave: 2
depends_on: ["01-contract"]
plan: plan.md
spec: ../../specs/tasks/requirements/bounded-queue-recovery.md
---

# Work packet 2: read

## Scope
Implementation Step 2 in [plan](plan.md) defines owned files, dependency order, acceptance mapping and risks. Read REQ-TASKS-QUEUE-RECOVERY-001 and its [design](../../specs/tasks/system-design/bounded-queue-recovery.md). MAIN is sole writer.

## Acceptance
- Complete Step 2's mapped ACs with real evidence.
- Preserve exact scope and nonmutating bounded observation.
- Record missing/failed checks; PostgreSQL skips are not PASS.

## Verification
```sh
cd apps/backend && go test -count=1 ./internal/orchestrator/messagequeue -run TestRecoveryRead
cd apps/backend && go test -race -count=1 ./internal/orchestrator/messagequeue -run TestRecoveryRead
```

## Risks and results
Census ownership confirmed and serialized hooks granted. Source implementation/checks are recorded in plan.md. PostgreSQL execution and independent verdicts remain pending. No overlap edits before ownership confirmation.
