---
id: "03-projection"
title: "Bounded queue recovery: projection"
status: done
wave: 3
depends_on: ["02-read"]
plan: plan.md
requirements:
  - REQ-TASKS-QUEUE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-QUEUE-RECOVERY-001.4
  - AC-TASKS-QUEUE-RECOVERY-001.5
  - AC-TASKS-QUEUE-RECOVERY-001.7
system_design:
  - ../../specs/tasks/system-design/bounded-queue-recovery.md
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
Terminal/child projection source passed focused memory, SQLite, and task-owned PostgreSQL 16.15 race checks at `56a5f0e4`; distinct Review and QA passed that exact candidate. A later PR correction handles queue shrinkage during cursor continuation and needs delta verification. Current-session census projection remains with task `ca015838`; release acceptance is pending.
