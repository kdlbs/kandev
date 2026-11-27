---
id: "02-protect-run-access"
title: "Protect credential run access"
status: complete
wave: 2
depends_on:
  - "01-validate-credentials"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-AUTOMATION-CREDENTIALS-002
acceptance_criteria:
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.4
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.6
system_design:
  - ../../specs/workspaces/system-design/automation-credentials.md
---

# Task 02 Protect credential run access

## Summary

Store the protected creator ID on the existing automation run when it uses a credential choice.
Apply the check at the existing authorization boundaries before the credential selectors ship.
Use a server-side fixture to test the boundary independently of the editor.

## In scope

- Use the existing run association to check the creator for its task, session, and environment.
- Apply the creator check to executable definition edits, manual activation, and access to protected run details and runtime controls.
- Preserve safe run status for workspace readers and Stop for the current workspace owner.
- Cover direct access, live fan-out, recovery, and retained history.

## Out of scope

- Credential selection, storage of personal bindings, and grant operations.
- A general task creator ACL. The protected creator field applies to the run and its attached resources.
- Rendered controls. Tasks 03 and 04 cover the editor and Runs view with desktop and phone tests.

## Acceptance

- The creator can access protected run controls and output. Other members can read safe status, and the workspace owner can stop the run. Applies to `AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.4`.
- A different account cannot activate or edit executable settings on a protected definition. The rejected request leaves the creator, definition, and credential choice intact. Applies to `AC-WORKSPACES-AUTOMATION-CREDENTIALS-002.6`.
- Restart and retention preserve the protected run boundary. Ordinary task and session access still follows workspace collaboration.

## Verification

| Entry points | Executed evidence |
| --- | --- |
| Task, session, environment, prompt authorization, and ordinary task access | `internal/task/service` `TestAutomationCredentialRunAccess` |
| Shared run projection and safe-field redaction | `internal/automation` `TestAutomationCredentialRunAccessProjection` |
| Manual and queued credential admission; private choice projection for launch | `internal/orchestrator` `TestAutomationCredentialLaunch` and `TestAutomationCredentialRunLaunchKeepsPrivateChoices` |
| Creator launch, owner Stop, retained status, and desktop/phone role flow | Authenticated desktop and mobile automation credential E2E scenarios |
| Persistence schema and backend wiring | Affected Go packages plus SQLite and disposable PostgreSQL store-conformance suites |

The changed projections have focused tests for plugin history, workspace WebSocket broadcasts, and retained task authority after cleanup failure. HTTP, MCP, and process restart continue to use the shared task/run checks and were not each exercised through separate end-to-end transports in this work order.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run '^TestAutomationCredentialRunAccess$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/automation -run '^TestAutomationCredentialRunAccessProjection$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestAutomationCredential(Launch|RunLaunchKeepsPrivateChoices)$' -count=1 -v)
```

Run these schema checks with a disposable PostgreSQL database for the final command.

```bash
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && env -u KANDEV_TEST_POSTGRES_DSN go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL DSN}" && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
```

## Files likely touched

- `apps/backend/internal/task/service/service_access.go` and `automation_credential_access_test.go`
- `apps/backend/internal/automation/{models,service,store,handlers}.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go` and `automation_credential_access_test.go`
- `apps/backend/internal/backendapp/adapters_plugin_automation_history.go` and `automation_credential_access_test.go`
- `apps/backend/internal/gateway/websocket/`, `internal/mcp/handlers/`, and runtime access adapters

## Dependencies

Task 01 supplies the resolved creator and credential validation boundary.

## Risks

- Direct task and environment routes can use different scope checks. Verify each transport with the same protected fixture.
- An old admission or missing run association must fail closed for a protected launch.
- Projections must retain only safe run fields for members who lack runtime access.

## Parallelism

`sequential`

## Inputs

- [Run access design](../../specs/workspaces/system-design/automation-credentials.md#run-access)
- [Workspace collaboration](../../specs/tasks/system-design/workspace-collaboration.md)
- `apps/backend/internal/backendapp/editor_session_access_test.go`

## Results

| Criterion | Result |
| --- | --- |
| 002.4 | `TestAutomationCredentialRunAccess`, `TestAutomationCredentialRunAccessProjection`, `TestPluginAutomationHistoryRedactsProtectedRunForWorkspaceReaders`, and `TestTaskEventBroadcasterRedactsProtectedSessionStateFromPersistedAuthority` passed. `TestDeleteAutomationCleanupFailureRetainsProtectedTaskAuthority` verifies a surviving task remains protected after cleanup failure. Desktop and phone E2E verify safe member status and owner Stop without transcript access. |
| 002.4 | `TestAutomationCredentialRevocationClosesLivePromptAndRuntimeAccess` rejects new prompts and runtime attachments after revoke while retaining safe status and owner Stop. Retry start, recovery, run commit, and retry commit regressions passed. |
| 002.4 | `TestProtectedAutomationTaskCreatedIsRedactedBeforeRunBinding` verifies safe workspace output while the task has not yet been linked to its protected run. `TestAutomationCredentialRunCreatorFallbackProtectsUnboundTask` verifies pre-bind task authorization. `TestProtectedCreatorRemainsScopedWhileCleanupJobSurvivesDeletedRun` verifies the cleanup owner remains protected after run-row deletion. |
| 002.6 | `TestCredentialAuthorityIsRecheckedAtAutomationUpdateCommit` rejects executable edits after a concurrent first grant changes authority. `TestCredentialAuthorityIsRecheckedAtRunCommit` and `TestCredentialAuthorityIsRecheckedAtRetryCommit` protect admission commits. |
