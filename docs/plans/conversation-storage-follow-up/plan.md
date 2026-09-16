---
created: 2026-09-16
status: complete
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-002
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-007
system_design:
  - ../../specs/plugins/system-design/conversation-source-reconciliation.md
legacy_specs: []
---

# Implementation Plan: Conversation storage remaining gates

## Overview

Close the remaining delivery gates for the committed conversation storage replacement.
The user requested this handoff for another agent to implement later.
This package reuses approved requirements and design; it introduces no new architecture.
Implementation starts from `c0a048bc128f7ef9a1051caf95ed442627faf9df`.
The comparison base before replacement is `88c6c0fe0a6ae5d25332070d603b99f7d9241645`.

## Scope

### In scope

- Add and execute real PostgreSQL source, receipt, and cleanup coverage.
- Reproduce and resolve the Office migration and task-service package failures.
- Prove recovery on desktop and mobile after all remediation.
- Keep test evidence and companion plan statuses accurate.

### Out of scope

- Automatic startup VACUUM, content hashes, transcript copies, or a new replay system.
- Plugin extraction, publication, UI redesign, and production database operations.
- Repeating completed large SQLite benchmarks without a related code change.
- Creating Kandev tasks, starting agents, pushing, or creating a PR during planning.

## Completed baseline

The [large SQLite report](../conversation-storage-replacement/verification/large-sqlite-upgrade.md)
records separate backup, cleanup, startup, streaming, and manual compaction evidence.
A 2.95 GB synthetic legacy database retained its file size after logical cleanup.
Manual compaction reduced it to 477 MB. Source records remained intact.
Both streaming runs passed without write or health-probe failures.
The SQLite catalog and runtime worker audit found no active legacy payload copies or journal workers.
The follow-up PostgreSQL tests now provide direct source, receipt, and cleanup
evidence against PostgreSQL 16.15.

Implementation commit hooks passed, including Go lint and zero-warning web lint.
The former lint findings are resolved; they are not another pending work order.
See [the implementation hook receipt](implementation-commit-receipt.md).
Earlier desktop 6/6 and mobile 2/2 results predate the final remediation.
They are historical evidence, not proof of this final implementation.

## Technical approach

Task 01 adds PostgreSQL coverage using `internal/testutil.OpenIsolatedPostgres`.
Merely setting a DSN while running SQLite-only tests does not prove PostgreSQL behavior.
Task 02 isolates `TestMigrate_PriorityIdempotent` and its missing `description` column.
Task 03 identifies the exact task-service failure, which the previous summary did not retain.
Compare both failures against the recorded base before calling either pre-existing.
Fix the smallest demonstrated cause, without weakening assertions or skipping tests.
Task 04 closes the browser recovery matrix after all backend fixes.

Keep source reads authoritative, revision checks payload-free, and receipts transient.
Keep compatibility, authorization, and loaded-range reconciliation from the existing design.
No rendered UI changes are proposed. Desktop panel and native mobile navigation remain required.

## Tests

Task 01 maps criteria 006.1-2, 006.5, 006.10-11 and 007.1-3 to real PostgreSQL tests.
It covers transactions, concurrent writers, pagination, cleanup rollback, and source preservation.
Task 02 owns the Office migration fixture and repeat-initialization evidence for the backend upgrade gate.
Task 03 owns the task-service package failure and its focused regression evidence.
These two gates do not establish new Office or task-service product requirements.
Each work order contains exact commands and file ownership.

## E2E tests

Task 04 maps criteria 005.2-3 and 006.3-10, 006.12 to desktop and mobile recovery.
Use the named plugin, prompt-history, and stream-isolation Playwright files in that order.
Test lost final delivery, reconnect, authorization loss, binding renewal, complete turn hydration,
and zero history rereads for irrelevant streaming or equal revisions.
Use deterministic fixture controls and existing unit coverage where a browser cannot inject a condition.
Record the boundary used for each scenario; do not report a unit test as browser evidence.

## Work orders

Recommended execution: 01, 02, 03, then 04. Only Task 04 depends on all preceding results.
A wave does not authorize delegation. The user will arrange implementation separately.

- [x] [Task 01: PostgreSQL source and cleanup coverage](task-01-postgres-coverage.md)
- [x] [Task 02: Office migration failure](task-02-office-migration.md)
- [x] [Task 03: Task-service failure](task-03-task-service.md)
- [x] [Task 04: Desktop and mobile recovery](task-04-recovery-e2e.md)

## Verification results

Follow-up implementation gates completed on 2026-09-17 at code revision
`213492517315abb38697b765e91e6fe0ea5388c7`:

- Task 01 passed all three named PostgreSQL tests on disposable PostgreSQL 16.15,
  the conversation SQLite and PostgreSQL conformance checks, and SQL guard.
- Task 02 classified the Office failure as a stale `fts5` fixture and passed the
  corrected targeted and full Office repository race tests. The comparison base
  reproduces the missing `description` column failure.
- Task 03 passed the full task-service race package three times on the current
  tree and once on the comparison base. No task-service production fix was
  needed.
- Task 04 passed the final desktop 5/5 and mobile 5/5 guarded Playwright
  matrices, the 41 frontend reconciliation tests, TypeScript, gateway race
  tests, specification validation, and whitespace checks.
- The broad `make test` target was attempted. It still reports unrelated
  `agentctl/server/process/probe`, `common/config`, and `launcher` failures in
  this workspace, including home-config discovery against the existing
  `/root/.kandev/config.yaml`. The Office failure seen during that run was fixed
  by the fixture correction above. These unrelated failures are recorded and
  are not hidden as passed follow-up gates.
- No PostgreSQL test was skipped, no production database was changed, and the
  disposable PostgreSQL service was removed after verification.
- Final plan validators and diff checks were rerun after this synchronization.

## Risks

- PostgreSQL evidence depends on a disposable service and must never use operator data.
- The broad backend audit still has unrelated process-probe, home-config, and launcher failures in this workspace.
- Physical SQLite space is reclaimed only by explicit maintenance after backup.
