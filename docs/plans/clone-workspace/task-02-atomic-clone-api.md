---
id: "02-atomic-clone-api"
title: "Create an authorized atomic clone"
status: done
wave: 2
depends_on:
  - 01-configuration-graph
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-CLONE-001
  - REQ-WORKSPACES-CLONE-002
acceptance_criteria:
  - AC-WORKSPACES-CLONE-001.1
  - AC-WORKSPACES-CLONE-001.4
  - AC-WORKSPACES-CLONE-001.5
  - AC-WORKSPACES-CLONE-001.6
  - AC-WORKSPACES-CLONE-001.7
  - AC-WORKSPACES-CLONE-002.1
  - AC-WORKSPACES-CLONE-002.2
  - AC-WORKSPACES-CLONE-002.3
  - AC-WORKSPACES-CLONE-002.4
  - AC-WORKSPACES-CLONE-002.5
system_design:
  - ../../specs/workspaces/system-design/clone-workspace.md
---

# Task 02: Create an authorized atomic clone

## Summary

Expose the complete authorized creation operation. Compose graph and GitHub
storage plus encrypted PAT copying in one transaction, then return the standard
access-aware DTO and publish ordinary creation events after commit.

## In scope

- Add `Service.CloneWorkspace`, a name request, narrow persistence interface,
  source manage/credential authority, supported-source checks, ordinary owner,
  placement and default-selection admission. Extract only creation helpers
  necessary to share policy with ordinary creation.
- Add transaction-aware GitHub/secret copy participants using existing field
  mappings. Preserve nil/empty query semantics, saved default markers, host,
  registration/installation identity, inactive status, and action preset fallback.
- Compose participants in backendapp with the shared writer transaction. Creator
  membership participates; defaults do not replace source values or fill an
  absent source connection. Participants never call independently committing
  copy helpers or check out the writer again while the transaction is held.
- Add `POST /api/v1/workspaces/:id/clone`, HTTP 201 DTO, sanitized error/status
  handling and existing post-commit workspace/workflow events.
- Use `/tdd`; include full-store tests and failure injection at every participant.

## Out of scope

UI, personal OAuth, other integrations, general secret copying, new WS/MCP
commands, clone job state, alternate transaction framework, or global changes
to existing GitHub copy-helper callers.

## Acceptance

1. Full-store `TestWorkspaceCloneAtomicCreation` proves readback/reopen persistence,
   ordinary creation ownership/reach, empty history and post-commit event ordering;
   source edit/delete remains independent of destination configuration.
2. `TestWorkspaceCloneGitHubSources` covers every design matrix row, source PAT
   disconnect isolation, App registration/installation disambiguation, excluded
   personal state, and zero secret-bearing response/log/event fields.
3. `TestWorkspaceCloneHTTP` proves source/default/placement authorization,
   managed/unsupported-source and invalid-name rejection, while each failure or
   cancellation before commit leaves zero target rows/credentials/events.

## Verification

Run from the repository root. PostgreSQL integration cases follow the existing
`KANDEV_TEST_POSTGRES_DSN` fixture contract and report unavailable infrastructure.

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/backendapp ./internal/github ./internal/secrets -run 'TestWorkspaceClone|TestCopyWorkspace|TestService_CreateWorkspace|TestCreateWorkspace|TestCopySecret' -count=1)
(cd apps/backend && go test -trimpath -race -p=1 ./internal/task/service -run TestWorkspaceClone -count=1)
(cd apps/backend && go test -trimpath -race -p=1 ./internal/backendapp -run '^TestWorkspaceClone' -count=3)
(cd apps/backend && go test ./internal/db/sqlguard/... -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
git diff --check
```

The full-store test must detect a nested-writer deadlock with a bounded test
context; a fake transaction callback alone does not establish atomicity.

## Files likely touched

- `apps/backend/internal/task/service/service_workspace_clone.go` and `_test.go` (new).
- `apps/backend/internal/task/service/service_workspace_clone_defaults_test.go` (new).
- `apps/backend/internal/task/service/service.go`, `service_resources.go`,
  `service_unit_placement.go`, and `service_members.go` as needed for shared admission.
- `apps/backend/internal/task/handlers/workspace_handlers.go`.
- `apps/backend/internal/task/handlers/workspace_clone_test.go` (new).
- `apps/backend/internal/backendapp/workspace_clone.go` and `_test.go` (new).
- `apps/backend/internal/backendapp/services.go`.
- `apps/backend/internal/github/workspace_clone.go` and `_test.go` (new).
- `apps/backend/internal/github/copy.go`, `service_connections.go`,
  `store_connections.go`, and workspace-settings/action-preset store helpers.
- `apps/backend/internal/secrets/workspace_clone.go` and `_test.go` (new),
  `sqlite_store.go`, and `store.go` only for a narrow supplemental transaction capability.

## Dependencies

Task 01 graph mapping and transaction participants.

## Risks

The current connection copy commits a connection before writing PAT material;
the public clone must not inherit this partial failure. Source-only read access
does not authorize copying credentials. SQL secret encryption must remain with
the secret owner, and no transport field accepts a source ownership claim.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/clone-workspace.md), 001.4-.7 and 002.
- [Design](../../specs/workspaces/system-design/clone-workspace.md), API, admission, atomic persistence and connection matrix.
- Existing GitHub `copy_test.go`, `service_connections_test.go`, creation
  integration tests and secret-store tests.

## Results

Atomic creation, seven persistence failure points, admission/authorization, event ordering, GitHub source matrix, encrypted nonce isolation and PAT disconnect independence tests passed. HTTP 201/name validation tests passed. SQL guard command and tests passed; race storeconformance passed. The original local check skipped the PostgreSQL atomic fixture because KANDEV_TEST_POSTGRES_DSN was unset; real PostgreSQL fixup results follow below.

Final targeted backend run passed across eight packages after adding review-action
profile admission: unavailable reviewer profiles roll back the clone, while
eligible shared reviewer profiles retain their action configuration. Cancellation,
empty-workflow bootstrap, stale source and authenticated HTTP ownership tests pass.
Scoped Go lint reports zero issues.

PR review coverage: `TestWorkspaceCloneRejectsUnavailableDefaults` adds 13
service-boundary cases for missing/deleted/disabled executor defaults,
missing/deleted environment defaults, and missing/deleted/disabled/source-scoped
profiles for both ordinary and config agents. Every rejection leaves the
persistence participant uncalled, workspace count unchanged and events empty.
The same fixture's positive control preserves all four eligible defaults and
reaches persistence/publication. The race-enabled command above passed (1.530s).
A temporary Go overlay bypassing admission made all 13 rejection cases fail with
the expected missing configuration error; no production file was changed.

Existing environment reads already filter soft-deleted rows, and GitHub service
initialization retains its store when authentication is absent. Owner-specific
fresh timestamps do not weaken transaction atomicity. These dispositions do
not change the documented behavior or contract, so requirements/design and
public documentation need no amendment.


The current-head Backend Postgres job (run 37707946725, job 113090745544)
failed `TestWorkspaceClonePostgresAtomicCreation` with `task resource version
changed`. The same assertion failed locally on an isolated PostgreSQL 16
instance (1.363s). The fixture supplied its pre-storage creation object;
PostgreSQL stores microsecond precision, while clone admission compares the
persisted workspace version. Reloading the source through `GetWorkspace`, as
the service does, fixes the fixture without weakening the version check.
The second command above passed three repetitions of all eight coordinator
clone tests with the race detector and PostgreSQL enabled (17.406s).
