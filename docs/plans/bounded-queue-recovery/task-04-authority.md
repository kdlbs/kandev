---
id: "04-authority"
title: "Bounded queue recovery: authority"
status: in_progress
wave: 4
depends_on: ["03-projection"]
plan: plan.md
requirements:
  - REQ-TASKS-QUEUE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-QUEUE-RECOVERY-001.2
  - AC-TASKS-QUEUE-RECOVERY-001.3
  - AC-TASKS-QUEUE-RECOVERY-001.5
  - AC-TASKS-QUEUE-RECOVERY-001.6
  - AC-TASKS-QUEUE-RECOVERY-001.8
system_design:
  - ../../specs/tasks/system-design/bounded-queue-recovery.md
---

# Work packet 4: authority

## Scope
Implementation Step 4 in [plan](plan.md) defines owned files, dependency order, acceptance mapping and risks. Read REQ-TASKS-QUEUE-RECOVERY-001 and its [design](../../specs/tasks/system-design/bounded-queue-recovery.md). MAIN is sole writer.

## Acceptance
- Complete Step 4's mapped ACs with real evidence.
- Preserve exact scope and nonmutating bounded observation.
- Record missing/failed checks; PostgreSQL skips are not PASS.

## Verification
```sh
cd apps/backend && go test -count=1 ./internal/mcp/handlers -run 'Test(QueueRecovery|TerminalQueueRecovery|ChildQueueRecovery|QueueCensus)'
```

## Risks and results
Census ownership confirmed and serialized hooks granted. Source implementation/checks are recorded in plan.md. PostgreSQL execution and independent verdicts remain pending. No overlap edits before ownership confirmation.
