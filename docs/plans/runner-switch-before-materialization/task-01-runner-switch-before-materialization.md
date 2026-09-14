---
id: "01-runner-switch-before-materialization"
title: "Implement runner switch before materialization"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNNER-SWITCH-001
  - REQ-TASKS-RUNNER-SWITCH-002
  - REQ-TASKS-RUNNER-SWITCH-003
  - REQ-TASKS-RUNNER-SWITCH-004
acceptance_criteria:
  - AC-TASKS-RUNNER-SWITCH-001.1
  - AC-TASKS-RUNNER-SWITCH-002.3
  - AC-TASKS-RUNNER-SWITCH-003.1
  - AC-TASKS-RUNNER-SWITCH-004.1
system_design:
  - ../../specs/tasks/system-design/runner-switch-before-materialization.md
---

# Task 01: Runner switch before materialization

## Acceptance

- Task projections carry the server-derived runner mutability verdict and its
  closed reason vocabulary.
- The runner action rechecks mutability under the task lock, validates the
  target runner, and changes only the stored executor profile.
- Session preparation and every materialization writer observe the committed
  runner in a deterministic order.
- The task dialog applies a successful runner switch before later save calls,
  keeps the last confirmed value after a partial failure, and uses the same
  behavior on desktop and mobile.

## Verification

```bash
go test ./internal/orchestrator/executor ./internal/task/repository/sqlite -count=1
go test -race ./internal/orchestrator/executor -run 'TestPrepareSessionRetriesTaskRunnerChangedAfterReload|TestEnsureSessionForAgentRetriesTaskRunnerChangedAfterReload' -count=1
go test -race ./internal/task/repository/sqlite -run 'TestCreateTaskSessionRejectsStaleTaskRunnerResolution|TestSwitchTaskRunner_ConcurrentBaseBranchUpdateNeverBlendsWithSwitch|TestPostgresRepositoryWritersUseConsistentTaskThenLinkLockOrder' -count=1
cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-submit.test.tsx
```

## Implementation result

The backend stores transient runner-resolution metadata on a session and
retries task-derived preparation when a concurrent switch commits first. The
SQLite session transaction rejects stale resolutions. Comparison-target writes
use task-before-link locking and recheck ownership. The web dialog tracks the
last confirmed runner so a failed later save can be retried in either direction.

