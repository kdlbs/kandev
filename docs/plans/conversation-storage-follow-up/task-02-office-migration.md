---
id: "02-office-migration"
title: "Resolve the Office migration test failure"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-007
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-007.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-007.2
system_design:
  - ../../specs/plugins/system-design/conversation-source-reconciliation.md
---

# Task 02: Resolve the Office migration test failure

## Summary

Reproduce `TestMigrate_PriorityIdempotent`, previously reported as missing the `description` column.
Determine whether the fixture is stale or the replacement changes upgrade behavior, then fix the cause.

## In scope

- Own the Office migration test and the smallest required migration/fixture correction.
- Capture the exact failure on the implementation head and comparison base using the same command.
  Use a temporary detached worktree for the base, leaving the active checkout intact.
- Preserve the test's repeated-initialization and priority assertions. Match the intended historical
  schema if fixing the fixture; do not add arbitrary columns merely to silence each failure.
- Add a failing regression first if production migration logic must change.

## Out of scope

Office behavior redesign, unrelated migration cleanup, task-service fixes, and test suppression.

## Acceptance

1. Evidence classifies the failure using both SHAs, or records why the base cannot reproduce it.
2. The targeted test and entire Office SQLite repository package pass with the race detector.
3. Initialization stays idempotent and original task values remain intact.

## Verification

Run this block at both the current head and comparison worktree root, retaining the failing baseline.
After remediation, run it again at the final implementation head.

```bash
(cd apps/backend && go test -race ./internal/office/repository/sqlite -run '^TestMigrate_PriorityIdempotent$' -count=1 -v)
(cd apps/backend && go test -race ./internal/office/repository/sqlite -count=1)
```

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/migrations_priority_test.go`
- Corresponding Office migration implementation only if reproduction proves a production defect.
- `apps/backend/internal/task/repository/sqlite/base_schema.go` only if the failure originates there.

## Risks

Changing a historical fixture can conceal an actual upgrade regression. Preserve the intended source
schema and distinguish test setup failure from migration failure in the results.

## Dependencies

None. Run sequentially in the recommended order.

## Parallelism

`sequential`

## Inputs

- [Host requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md) and [source reconciliation design](../../specs/plugins/system-design/conversation-source-reconciliation.md).
- [Architecture decision](../../decisions/2026-09-16-conversation-source-reconciliation.md).
- Implementation commit `c0a048bc128f7ef9a1051caf95ed442627faf9df`; comparison base `88c6c0fe0a6ae5d25332070d603b99f7d9241645`.
- [Original package](../conversation-storage-replacement/plan.md) and its recorded evidence.

## Results

Pending. Record exact commands, tested commit, environment, results, and skipped tests.
Update this package and the original package together; preserve historical results.
