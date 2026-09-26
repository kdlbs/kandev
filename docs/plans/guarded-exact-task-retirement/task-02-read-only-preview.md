---
id: "02-read-only-preview"
title: "Add read-only exact-pair preview"
status: done
wave: 1
depends_on:
  - "01-record-contract"
plan: "plan.md"
requirements:
  - REQ-TASKS-EXACT-RETIREMENT-001
acceptance_criteria:
  - AC-TASKS-EXACT-RETIREMENT-001.1
  - AC-TASKS-EXACT-RETIREMENT-001.2
  - AC-TASKS-EXACT-RETIREMENT-001.3
  - AC-TASKS-EXACT-RETIREMENT-001.4
system_design:
  - ../../specs/tasks/system-design/guarded-exact-task-retirement.md
---

# Task 02: Add read-only exact-pair preview

## Summary

Expose a task-authorized preview for an exact old/replacement pair. It returns
fixed, redacted predicate receipts and fails closed where the later evidence
adapters do not yet exist.

## Acceptance

1. The caller has an admin identity and `task.write` on both tasks. Admin
   status does not grant workspace access. The workspace exists, and both IDs
   identify distinct tasks in that workspace with matching generations.
2. Each fixed predicate returns a receipt with status, reason, resource,
   observed generation, and digest.
3. Unavailable evidence returns `UNKNOWN`; preview never changes task, session,
   queue, or filesystem state.

## Verification

```bash
cd apps/backend && go test -count=1 ./internal/task/handlers ./internal/task/service
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Risks

This work intentionally has no commit route. W03 replaces unknown preservation,
queue, Git, and consumer predicates with independently verified adapters.

## Results

The preview route requires trusted authentication, authorizes both task IDs,
requires a distinct same-workspace pair with matching generations, and returns
a no-store receipt. Its fixed registry reports unavailable evidence as
`UNKNOWN`, so it cannot initiate cleanup. Eligibility is derived from the
complete non-empty receipt set and is true only when every predicate passes.
Read-only members are denied on the old task before the replacement lookup,
so their response does not reveal whether a supplied replacement exists.
The requirements and system design also record the admin identity, workspace
access, and synthetic-admin policy used by this route.

- `cd apps/backend && go test -count=1 ./internal/task/handlers ./internal/task/service -run 'Test(HTTPPreviewExactRetirement|PreviewExactRetirement|ExactRetirementReceiptsEligible)'` - passed.
- `cd apps/backend && go test -count=1 ./internal/task/handlers ./internal/task/service -run 'Test(HTTPPreviewExactRetirementRejectsAuthorizedEqualAndStaleGenerations|PreviewExactRetirementRejectsAuthorizedInvalidPairs)'` - passed.
- `python3 scripts/list-docs.py validate` - passed.
- `python3 scripts/lint-spec-files.py --all` - passed.
