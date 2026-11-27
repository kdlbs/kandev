---
id: "04-workspace-grants"
title: "Grant Workspace secret use"
status: complete
wave: 4
depends_on:
  - "03-personal-bindings"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-AUTOMATION-CREDENTIALS-003
acceptance_criteria:
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.1
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.2
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.3
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.4
  - AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.5
system_design:
  - ../../specs/workspaces/system-design/automation-credentials.md
---

# Task 04 Grant Workspace secret use

## Summary

Give the current workspace owner a grant and revoke action for one existing repository binding on a saved automation.
Only that automation and its creator can exercise the grant.
Check the current grant at the point where runtime receives the secret.

## In scope

- Add grant provenance and revision checks to the credential choice store.
- Add the owner actions and safe status at the saved repository binding. Add scoped runtime resolution.
- Require the current workspace owner to own the Workspace secret in the selected binding.
- Fence revoke, transfer, source changes, queued runs, and retry recovery.
- Update Team Access, automation and executor docs, and the WebSocket reference.

## Out of scope

- Grants that follow a copied secret or repository membership.
- Workspace grants on profile fields.
- Grant management through configuration chat, plugins, or a reusable shared profile.
- Rotation or replacement of credentials already present in an active process.

## Acceptance

- The current workspace owner can grant use of a Workspace secret they own at a selected repository binding. The automation's creator can launch it.
- Revoked or stale grants block later admission after restart or retry. A different automation requires a separate grant.
- Desktop and phone show approval status at the binding and the actions allowed for the viewer. Shared history contains safe status, and exports omit the grant.

## ASCII UI preview

### UI-03 Workspace grant

Entry is a saved automation's repository binding in **Then**. See the [full preview](plan.md#ui-03-workspace-grant).
Covers `AC-WORKSPACES-AUTOMATION-CREDENTIALS-003.1`, `.3`, and `.5`.

```text
Desktop
Repository billing
SERVICE_KEY     [ Grant Workspace secret ]
After grant     Approved for Alex  [ Revoke grant ]

Phone
Repository billing / SERVICE_KEY
[ Grant Workspace secret ]
................ inset drawer .....................
Grant Workspace secret
For Alex on this automation
Repository billing / SERVICE_KEY
Billing key
The agent can read this secret during the run.
[ Grant use ]
```

Show the grant control when the workspace owner also owns the secret in the saved binding.
Check the handoff from the creator's disabled draft to owner approval and back to the creator to enable and run it.
Give the owner one confirmation with the creator, field, and runtime use. Revoke from the row and update its status on success.
The creator sees **Owner approval needed**, **Approved**, or **Grant revoked** at the binding as applicable. Keep the owner's **Stop** action in the existing Runs view.
Check both accounts on a phone, including the drawer scroll and focus return.

## Verification

In a fresh worktree, run `cd apps && pnpm install --frozen-lockfile` before the first frontend command.

Use `internal/automation` `TestAutomationWorkspaceSecretGrant` for owner/secret ownership, grant identity, safe projection, revoke resolution, repository-binding replacement, and stale revisions.
Use `internal/automation` `TestAutomationCredentialExport` for disabled and redacted portable exports.
Run the conformance suite with SQLite and a disposable PostgreSQL database. Use a temporary Git repository and a mock agent in the authenticated Playwright scenarios.

The runtime resolution, admission, and revocation tests exercise the shared authority checks used by secret resolution and task admission. SQLite and PostgreSQL store conformance passed with `-race`; PostgreSQL ran in a temporary isolated Docker container that was removed after the test.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/automation -run '^TestAutomation(WorkspaceSecretGrant|CredentialExport)$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/automation -run '^TestAutomationCredentialRunAccessProjection$' -count=1 -v)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && env -u KANDEV_TEST_POSTGRES_DSN go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL DSN}" && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
pnpm --dir apps/web exec vitest run components/automations/automation-credentials.test.tsx
pnpm --dir apps/web e2e:run --host --shards 1 --project auth tests/auth/automation-credentials.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/auth/mobile-automation-credentials.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/automation/{models,store,service,handlers}.go` and `service_credentials_test.go`
- `apps/backend/internal/secrets/{store,sqlite_store,user_visible_store}.go` and `automation_grant_test.go`
- `apps/backend/internal/orchestrator/event_handlers_automation.go` and `automation_credentials_launch_test.go`
- `apps/backend/internal/backendapp/{services,secret_references}.go`
- `apps/backend/internal/persistence/{requiredstores,storeconformance}/` and upgrade fixtures
- `apps/web/components/automations/`, automation API types, and the `automations` locale files
- `apps/web/e2e/tests/auth/{automation-credentials,mobile-automation-credentials}.spec.ts`
- `docs/public/{team-access,executors,automation-and-mcp,websocket-api}.md`

## Dependencies

Task 02 supplies protected run access. Task 03 supplies the credential choice store.

## Risks

- A Workspace copy gets a new secret ID. Preserve the old relationship as stale until the owner makes a new grant.
- Revocation must order with the last admission check across backend connections.
- Watch for overlap with the shared-profile work before changing common resolution code. [Draft PR #5184](https://github.com/kdlbs/kandev/pull/5184).

## Parallelism

`sequential`

## Inputs

- [Grant authority](../../specs/workspaces/system-design/automation-credentials.md#workspace-grant-authority)
- [Run and recovery](../../specs/workspaces/system-design/automation-credentials.md#run-and-recovery)
- [Automation credential authority ADR](../../decisions/2026-11-26-automation-credential-authority.md)

## Results

| Criterion | Result |
| --- | --- |
| 003.1 | `TestAutomationWorkspaceSecretGrant` denies non-owners and secrets owned by another account. The confirmation binds to the repository-binding revision shown in the dialog. |
| 003.2 | The service test resolves the granted value only for the matching automation, creator, run, and repository binding. Desktop and phone E2E grant, enable, and start the creator's run. |
| 003.3 | The grant test confirms revoke blocks later resolution; `TestAutomationCredentialRevocationClosesLivePromptAndRuntimeAccess` rejects new prompts and attachments while a run remains live. A run admitted after repository replacement can use its current grant even while the old repository grant remains stale. Retry/restart admission rechecks current authority. |
| 003.4 | `TestAutomationWorkspaceSecretGrant` covers stale confirmation, binding replacement, ownership moving away and back, and stale grant revisions. `TestCredentialAuthorityIsRecheckedAtRunCommit` and `TestCredentialAuthorityIsRecheckedAtRetryCommit` cover commit-time admission. |
| 003.5 | Desktop and phone E2E show creator-specific owner confirmation, current status, revoke, and owner Stop. The other workspace member receives safe run status without controls. Phone viewport and target-size assertions passed. |
