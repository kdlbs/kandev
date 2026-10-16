---
id: "01-retry-after-workspace-election"
title: "Retry after the workspace election"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 01: Retry after the workspace election

## Summary

Let a session binding that lost the materializer election wait for the elected
environment and retry once, for task-local and shared-group elections.

## In scope

- Add executor and repository tests before the change.
- Name the elected environment on the preparing refusal.
- Wait and retry once in session preparation.
- Update the workspace state matrix and launch admission design.

## Out of scope

The election itself, the materialization timeout, and other refusals.

## Acceptance

- A start that lost the election attaches after the elected environment becomes
  ready, also when the environment belongs to another shared-group member.
- A failed environment, a wait that ends without readiness, or an environment
  this session materializes keeps the original refusal, without creating a
  session or workspace.
- Other binding refusals are not retried.
- A session the winner created while the loser waited stays primary and keeps
  the first-session origin.
- Two retries for the same shared-group member elect one primary and consume
  the initial runtime seed once in the insertion transaction on SQLite and
  PostgreSQL. The caller follows committed ownership for primary notifications.
- Cancellation does not invoke the retry, and a committed retry does not
  overwrite a later manual primary selection.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator/executor -run 'WorkspacePreparing|RetryPreparedSession|PrepareSessionDoesNotRetryOtherBindingErrors' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'TestCreateTaskSessionWith' -count=1)
(cd apps/backend && go vet -trimpath ./internal/orchestrator/executor ./internal/task/...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/task/models/workspace_binding.go
- apps/backend/internal/task/repository/sqlite/session.go
- apps/backend/internal/task/repository/sqlite/task_environment_test.go
- apps/backend/internal/orchestrator/executor/executor_execute.go
- apps/backend/internal/orchestrator/executor/executor_workspace_preparing_retry_test.go
- docs/specs/tasks/requirements/additional-session-workspace-reuse.md
- docs/specs/tasks/system-design/additional-session-workspace-reuse.md

## Dependencies

None.

## Risks

A losing start waits up to the launch timeout before it returns.

## Parallelism

`sequential`

## Inputs

- REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001 and
  AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4.
- The linked additional session workspace reuse design.

## Results

- RED: without the change, the losing start returned `workspace_preparing`
  without a retry, and the refusals did not name the elected environment.
- GREEN: the new tests and the existing workspace binding tests pass.
- Review remediation RED: contained exact-source proofs exposed stale primary
  ownership in the transaction and caller, repeated post-insert promotion, and
  a cancelled ready wait that invoked its retry callback.
- Review remediation GREEN: those exact-source proofs pass with transaction
  ownership and caller/cancellation guards. Added hosted real repository
  concurrency tests for SQLite/PostgreSQL, full caller ownership tests, and
  shared-group selected-environment attachment assertions. Hosted CI provides
  the full package and end-to-end results; narrow proofs use explicit boundary
  fakes and do not substitute for those suites.
