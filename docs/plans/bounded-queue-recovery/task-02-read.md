---
id: "02-read"
title: "Bounded queue recovery: read"
status: done
wave: 2
depends_on: ["01-contract"]
plan: plan.md
requirements:
  - REQ-TASKS-QUEUE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-QUEUE-RECOVERY-001.3
  - AC-TASKS-QUEUE-RECOVERY-001.6
  - AC-TASKS-QUEUE-RECOVERY-001.7
  - AC-TASKS-QUEUE-RECOVERY-001.9
system_design:
  - ../../specs/tasks/system-design/bounded-queue-recovery.md
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
Bounded read source passed memory, SQLite, and task-owned PostgreSQL 16.15 focused race checks at `56a5f0e4`; distinct Review and QA passed that exact candidate. The current PR correction strengthens the no-write test fingerprint and needs delta verification. Census delivery remains with task `ca015838`; release acceptance is pending.
