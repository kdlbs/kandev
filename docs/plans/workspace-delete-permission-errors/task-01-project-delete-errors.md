---
id: "01-project-delete-errors"
title: "Project workspace deletion errors"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-DELETION-001
acceptance_criteria:
  - AC-WORKSPACES-DELETION-001.2
  - AC-WORKSPACES-DELETION-001.3
  - AC-WORKSPACES-DELETION-001.4
  - AC-WORKSPACES-DELETION-001.5
  - AC-WORKSPACES-DELETION-001.9
  - AC-WORKSPACES-DELETION-001.10
  - AC-WORKSPACES-DELETION-001.11
system_design:
  - ../../specs/workspaces/system-design/deletion.md
---

# Task 01: Project workspace deletion errors

## Summary

Restore the generic workspace deletion response contract for real role denials
on HTTP and WebSocket. Qualify only the existing error mappers, with causal
transport/service/repository regressions and unchanged deletion authorization.

## In scope

- Add the five test functions named in the plan, using the registered routes and
  dispatcher with a dedicated real-service/SQLite deletion fixture.
- Seed an owned workspace and actual `collaborator`/`viewer` membership rows.
  Assert `workspace.read` is present and `workspace.manage` absent through the
  real decision resolver. Include inherited unit membership using existing orgunit
  fixture patterns, without mocking the authorization result.
- Prove the denial for both empty workspaces and workspaces with a workflow,
  task, session and member row. Inspect retained rows and instrument cleanup
  admission/publication to prove that denial has no destructive aftermath.
  Wrap the real task inventory read and install canvas/secret cleanup recorders
  as specified in the plan. Assert zero calls and no new cleanup jobs on denial.
  Activate the same recorders in the authorized success control.
- Cover nonmember, admin nonmember, foreign-organization, and nonexistent IDs.
  Compare hidden and missing response bodies; exclude workspace names and IDs.
- Preserve malformed/empty confirmation validation and WS ID validation before
  repository access. Preserve reach/manage checks before a wrong nonempty name.
- Add owner/admin-owner success and internal/synthetic compatibility controls.
  Prove ordinary deletion and the existing event path still work.
- Cause a real cascade failure with a temporary SQLite trigger, including a
  message containing `not found`. Verify transaction rollback and internal errors.
- Add focused mapper tests for wrapped sentinels and unrelated message decoys.
  Update the two existing missing-workspace fake errors to typed absence.
- Change only the two mapper branches and their required import. Use typed
  workspace absence, `service.IsForbidden`, `ws.ErrorCodeForbidden`, and the
  fixed `service.ErrForbidden.Error()` message.

## Out of scope

No changes to access policy, auth middleware, role/scope grants, service methods,
shared `isNotFound`, SQL, cascades, or cleanup workers.
Keep side-table consumers, events, Office routes, web code, locale catalogs,
and runtime flags unchanged. No new
dependencies, generated contracts, lockfiles, migrations, or mapping framework.
Do not implement the draft task lifecycle Store API.

## Acceptance

1. AC 001.9/.10: registered HTTP/WS requests produce the specified denial and
   hidden/missing results from real role decisions. Dependent rows remain,
   cleanup is not admitted, and no deletion event publishes.
2. AC 001.2/.11: validation order and exact-name checks remain intact. Wrapped
   sentinels classify correctly, safe messages omit wrapper details, and actual
   storage failures stay internal regardless of permission/absence words.
3. AC 001.3/.4/.5: authorized empty-workspace deletion succeeds on both
   transports; existing cascade, cleanup, rollback and event regressions pass.
   The production diff stays within `workspace_handlers.go`.

## TDD sequence

1. Recheck current source, issue assignment and the dependency heads in the plan.
2. Add the integration and focused error tests before changing either mapper.
3. Run RED. Expect collaborator/viewer `403`/`FORBIDDEN` assertions to fail at
   the current `500`/`INTERNAL_ERROR` results. Expect the misleading storage-text
   case to fail at the current `404`/`NOT_FOUND` result.
4. Verify positive, privacy, ordinary storage and validation controls pass on
   that same baseline. Setup errors and absent identity are not causal RED.
5. Replace each deletion mapper's broad absence predicate with typed workspace
   absence. Add permission handling after absence and before the internal fallback.
6. Keep current validation, fixed not-found/internal messages, and success bodies.
7. Run GREEN and the remaining exact verification commands.
8. Record results and update this work order and its plan after all checks pass.

The old fake errors stop matching after typed classification. Change those two
fixtures with the mapper correction, rather than restoring substring matching.
Do not add new cases to the oversized service resources test file.

## Verification

Run from the repository root. The first command is the RED/GREEN regression
gate. The remaining commands are required after the correction.

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/handlers -run '^TestWorkspaceDelete(AuthorizationIntegration|ValidationIntegration|SuccessIntegration|StorageFailureIntegration|ErrorProjection)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/handlers -run '^Test(HTTP|WS)DeleteWorkspace' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/service -run '^TestService_DeleteWorkspace' -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/handlers/workspace_handlers.go apps/backend/internal/task/handlers/workspace_handlers_test.go apps/backend/internal/task/handlers/workspace_delete_permissions_integration_test.go apps/backend/internal/task/handlers/workspace_delete_errors_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- apps/backend/internal/task/handlers docs/specs/workspaces docs/plans/workspace-delete-permission-errors
```

Use `gofmt` on the changed Go files before lint. Keep every test process handle
until completion. No frontend dependency install, build, Playwright suite, full
backend suite, or broad verification audit is required for this bounded change.
The plan records the repository PR-documentation coverage preflight separately.

The integration boundary injects a resolved identity. It does not prove browser
login, bearer verification, a real WS upgrade, or runtime-resource cleanup.
Keep those limits in the final implementation report.

## Files likely touched

- `apps/backend/internal/task/handlers/workspace_handlers.go` (sole production file).
- `apps/backend/internal/task/handlers/workspace_handlers_test.go` (typed absence fixtures).
- `apps/backend/internal/task/handlers/workspace_delete_permissions_integration_test.go` (new).
- `apps/backend/internal/task/handlers/workspace_delete_errors_test.go` (new).
- This work order and `plan.md` (status and exact results after implementation).

Read-only references include `service_resources.go`, `service_errors_authz.go`,
`service_access.go`, `repository/sqlite/workspace.go`, and `handlers/errors.go`.
Preserve all edits from other work in the shared repository.

## Dependencies

None. The current service already provides the required typed decision.
PR #5179 and PR #5184 are adjacent work, not prerequisites. Re-read any later
changes to these handlers or the relevant service path before implementing.

## Risks

- A fixture without a real owner or identity can silently exercise unscoped access.
- Authorization stubs can conceal missing service wiring; use the real resolver.
- An asynchronous event observer needs a deterministic drain before a negative assertion.
- A storage trigger must live only in the disposable database, with normal test cleanup.
- Matching error text can both leak data and hide storage errors. Keep classification typed.
- Generic post-commit failures do not imply rollback; retain existing committed cleanup behavior.

## Parallelism

`sequential`. This package grants no delegation or additional sessions.

## Inputs

- [Owning requirement and ACs](../../specs/workspaces/requirements/deletion.md).
- [Generic deletion error design](../../specs/workspaces/system-design/deletion.md#generic-deletion-error-projection).
- [Plan, evidence, compatibility and exclusions](plan.md).
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.
- `workspace_field_updates_integration_test.go`: registered HTTP/WS and SQLite fixture idiom.
- `service_team_access_test.go`: real unit/direct membership and resolver patterns.
- `workspace_handlers_test.go`: existing validation, cascade and missing-row controls.
- `executor_ownership_errors.go` and `errors.go:handleNotFound`: established permission mapping.

## Results

Completed on 2026-11-08. The production diff is confined to the two deletion
mappers in `workspace_handlers.go`. The existing missing-workspace fixtures now
use `repoerrors.ErrWorkspaceNotFound`. New coverage lives in
`workspace_delete_permissions_integration_test.go` and
`workspace_delete_errors_test.go`.

- Causal RED: a temporary overlay reverted only the two mapper branches. The
  five-test gate exited 1 with 13 expected failures: four projection cases, six
  HTTP/WS permission denials, the wrong-name permission-order case, and both
  storage-text cases. Success and privacy controls passed on that baseline.
- GREEN: the five named tests passed; `TestHTTPDeleteWorkspace*`,
  `TestWSDeleteWorkspace*`, and `TestService_DeleteWorkspace*` passed.
- Race variants of all three gates passed. Full `internal/task/handlers` and
  `internal/task/service` packages passed. `go build -trimpath ./...` passed.
- `scripts/lint-go-changed` reported 0 issues after `gofmt`.
- Document catalogue validation, specification lint, and all 36 linter tests
  passed. `git diff --check` reported no whitespace errors.

Real registered HTTP/WS handlers exercised the real service, resolver, and
SQLite rows. The integration boundary still injects a resolved identity, so live
login/bearer verification, a network WebSocket upgrade, and runtime-resource
cleanup remain outside this evidence.
