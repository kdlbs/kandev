---
id: "01-contract"
title: "Bounded queue recovery: contract"
status: done
wave: 1
depends_on: []
plan: plan.md
spec: ../../specs/tasks/requirements/bounded-queue-recovery.md
---

# Work packet 1: contract

## Scope
Implementation Step 1 in [plan](plan.md) defines owned files, dependency order, acceptance mapping and risks. Read REQ-TASKS-QUEUE-RECOVERY-001 and its [design](../../specs/tasks/system-design/bounded-queue-recovery.md). MAIN is sole writer.

## Acceptance
- Complete Step 1's mapped ACs with real evidence.
- Preserve exact scope and nonmutating bounded observation.
- Record missing/failed checks; PostgreSQL skips are not PASS.

## Verification
```sh
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Risks and results
Contract/reuse package materialized, ownership confirmed, documentation/catalog/spec checks passed. PostgreSQL fixture delivery remains an execution prerequisite for parity, not a missing contract decision. No overlap edits before ownership confirmation.
