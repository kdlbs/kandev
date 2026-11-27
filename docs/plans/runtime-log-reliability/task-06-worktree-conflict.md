---
id: "06-worktree-conflict"
title: "Diagnose worktree ownership conflict"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
acceptance_criteria:
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.1
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.2
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.3
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.1
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.2
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.3
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 06: Diagnose worktree ownership conflict

## Summary

Identify the exact registration conflict and test the existing recovery boundary. Record a preview or expected refusal without applying any installation repair.

## In scope

- Recorded worktree `54202d4a-5b17-411a-82bb-91c7c0ac113d` under task `13f1a547-415a-47f6-9cc9-41ebeb17515e` as the historical evidence target.
- Read-only identity evidence from retained logs and exact Git registration, without installation database or marker changes.
- Disposable fixtures for stale branch, conflicting checkout, duplicate candidate, and active-consumer refusal.
- `worktree-evidence.md` with cause classification and exact later repair eligibility.

## Out of scope

- Running the offline repair utility against the active backend, forcing removal, changing refs/markers, and rewriting cleanup snapshots.

## Acceptance

- Evidence identifies saved and observed repository/path/branch/common-directory identities or explicitly records unavailable identity evidence.
- Fixtures retain ownership, active-consumer, dirty-checkout, and audit-history refusal behavior. Safe preview makes no changes.
- The report classifies the result as a supported explicit repair, correct ownership refusal, or a defect needing its own concrete package. No live application occurs.

## Verification

Run this block from the repository root after the implementation result exists.
New test names below are required planned regressions, not claims of existing coverage.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/inventoryrepair ./cmd/worktree-inventory-repair -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/worktree -run 'Test.*(Cleanup|Registration)' -count=1)
```

## Files likely touched

- `apps/backend/internal/task/inventoryrepair/inspect_test.go`
- `apps/backend/internal/task/inventoryrepair/cleanup_worker_test.go`
- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/manager_cleanup_audit.go`
- `docs/plans/runtime-log-reliability/worktree-evidence.md (new)`

## Dependencies

None. Follow the plan's sequential priority order.

## Risks

- The offline repair tool refuses a live backend. Do not restart or stop the current installation to obtain a preview.
- A competing registration can belong to valid work. Ambiguity requires refusal, not cleanup.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), especially evidence, contract ownership, and completion rules.
- [Design](../../specs/tasks/system-design/worktree-inventory-repair.md) and its linked requirements.

- Scoped backend/agentctl instructions for any touched package.
- Existing source and tests listed above. Preserve completed companion-package results.

## Results

Completed as an evidence-only investigation. See [worktree-evidence.md](worktree-evidence.md). One cleanup refusal was found in the retained backend logs; the planned second attempt and exact saved/observed identities were not available. Existing disposable preview, identity-refusal, competing-inventory, live-consumer, dirty-checkout, and cleanup-audit fixtures passed. Classification is unresolved historical identity with a correct fail-closed refusal under current evidence; no repair is eligible and no installation state was read or changed.

Both required race-enabled test commands passed.
