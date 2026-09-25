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
system_design:
  - ../../specs/tasks/system-design/guarded-exact-task-retirement.md
---

# Task 02: Add read-only exact-pair preview

## Summary

Expose a task-authorized preview for an exact old/replacement pair. It returns
fixed, redacted predicate receipts and fails closed where the later evidence
adapters do not yet exist.

## Acceptance

1. Both IDs are authorized before inventory disclosure and must be distinct,
   same-workspace tasks with matching expected generations.
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
`UNKNOWN`, so it cannot claim eligibility or initiate cleanup.

- `cd apps/backend && go test -count=1 ./internal/task/handlers -run 'TestHTTPPreviewExactRetirement'` - passed.
- `cd apps/backend && go test -count=1 ./internal/task/service -run 'TestPreviewExactRetirement'` - passed.
