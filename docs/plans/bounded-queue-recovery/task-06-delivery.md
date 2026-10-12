---
id: "06-delivery"
title: "Bounded queue recovery: delivery"
status: in_progress
wave: 6
depends_on: ["05-mcp"]
plan: plan.md
requirements:
  - REQ-TASKS-QUEUE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-QUEUE-RECOVERY-001.9
system_design:
  - ../../specs/tasks/system-design/bounded-queue-recovery.md
---

# Work packet 6: delivery

## Scope
Implementation Step 6 in [plan](plan.md) defines owned files, dependency order, acceptance mapping and risks. Read REQ-TASKS-QUEUE-RECOVERY-001 and its [design](../../specs/tasks/system-design/bounded-queue-recovery.md). MAIN is sole writer.

## Acceptance
- Complete Step 6's mapped ACs with real evidence.
- Preserve exact scope and nonmutating bounded observation.
- Record missing/failed checks; PostgreSQL skips are not PASS.

## Verification
```sh
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Risks and results
Census ownership confirmed and serialized hooks granted. Source implementation/checks are recorded in plan.md. PostgreSQL execution and independent verdicts remain pending. No overlap edits before ownership confirmation.
