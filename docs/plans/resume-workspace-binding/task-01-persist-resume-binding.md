---
id: "01-persist-resume-binding"
title: "Persist the final resume binding"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
---

# Task 01: Persist the final resume binding

## Summary

Save the session's final environment and workspace binding after resume materialization.
Keep the credential boundary and guarded startup ownership intact.

## In scope

- Add `TestResumeSessionPersistsMaterializedWorkspaceBinding` in a new focused executor test file.
- Cover agent resume and workspace-only resume after an initially empty raw path.
- Capture immutable write values and verify the raw path through repository integration.
- Save the final binding after `persistTaskEnvironment` and before success or process startup.
- Cover final-write error, cancellation, changed environment, and replaced startup attempt with controlled gates.
- Cover a same-attempt transition to `RUNNING` or `WAITING_FOR_INPUT` while `LaunchAgent` returns, without accepting a replaced attempt.
- Preserve pre-credential `STARTING`, credential snapshots, rollback, and owned execution cleanup.

## Out of scope

Git source eligibility changes, UI markup, database backfill, live runtime mutations,
new schemas, and new public contracts.

## Acceptance

- Both resume modes persist the canonical final binding independently of the effective session projection or mutable mock pointer.
- An unsuccessful or superseded final write prevents agent startup and cannot alter the successor session or its execution.
- Existing credential-boundary ordering and canonical workspace tests pass.

## Verification

Run the new regression before production edits and record the expected failure.
Run these commands from the repository root after implementation:

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator/executor -run 'TestResumeSession.*(MaterializedWorkspaceBinding|WorkspaceBindingPersistence|WorkspaceBindingSuperseded)|TestPersistTaskEnvironmentBindsCanonicalWorkspaceOnEverySuccessPath|TestResumeSession_PersistsStartingBeforeCredentialLease|TestResumeSession_PersistsStartingBeforeLaunch|TestResumeSession_PropagatesTaskEnvironmentPersistenceFailure' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator/executor ./internal/task/repository/sqlite -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go test -v ./internal/task/repository/sqlite -run '^TestPostgresUpdateTaskSession(WorkspaceBinding|ResumeState)IfCurrentAttempt$' -count=1)
git diff --check
```

The PostgreSQL tests use an isolated database and skip when
`KANDEV_TEST_POSTGRES_DSN` is not configured. Run them with an owned test
instance when available and report a skipped run separately from test coverage.
No new schema tag is expected.

## Files likely touched

- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_workspace_binding_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_mocks_test.go` (only if required for immutable write assertions)
- `apps/backend/internal/task/repository/sqlite/session_workspace_binding_test.go` (new integration evidence)
- `apps/backend/internal/task/repository/sqlite/session.go` (only if the existing guarded operation is insufficient)

## Dependencies

None.

## Risks

State projection can conceal a missing raw write. The mock stores mutable
session pointers, so assertions must use copied write snapshots or raw database reads.
Final persistence must retain startup-attempt and recovery-claim ownership.

## Parallelism

`sequential`

## Inputs

- [Canonical continuity requirement](../../specs/tasks/requirements/additional-session-workspace-reuse.md), criteria `002.1` and the existing admission boundaries.
- [System design](../../specs/tasks/system-design/additional-session-workspace-reuse.md), Persistence and projection.
- [Plan](plan.md), confirmed root cause and reproduction.
- Existing `executor_resume_test.go`, `executor_environment_workspace_test.go`, and guarded session repository writes.

## Results

Pre-implementation RED reproduced the missing raw path for agent resume and
workspace-only resume. The temporary reproduction was removed.

Passed:

- Focused executor race tests for materialized binding, superseded attempts,
  canonical environment binding, and credential ordering.
- Full orchestrator race suite, including cancellation, prompt acceptance,
  unarchive recovery, and idle-session focus coverage.
- Full executor and SQLite package race suites (`sqlite`: 259.492s), covering
  persistence failure, cancellation, and changed-environment rejection.
- Deterministic successor-attempt regressions for a rejected final binding with
  a transient diagnostic read error and for an ordinary final-write error.
- An owned-attempt state-advancement regression proves the final binding can
  retry from the observed active state while still rejecting terminal or
  replaced attempts.
- Attempt-fenced state and credential rollback, including preservation of
  successor fields and reservation ownership.
- Runnable PostgreSQL raw write/read-back tests for owned agent binding,
  workspace-only state guarding, replaced attempts, cancellation, rollback, and
  unrelated metadata preservation.
- `go run ./cmd/sqlguard ./internal`.
- Persistence store conformance race suite (`32.838s`).
- `make -C apps/backend build`.
- `git diff --check`.

The environment-gated PostgreSQL tests were invoked but skipped because no
owned test instance or `KANDEV_TEST_POSTGRES_DSN` was available. PostgreSQL
behavior therefore remains unexecuted locally. No schema change was required.
