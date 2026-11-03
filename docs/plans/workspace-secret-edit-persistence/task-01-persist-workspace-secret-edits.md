---
id: "01-persist-workspace-secret-edits"
title: "Persist workspace secret edits"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SECRETS-001.15
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
---

# Task 01: Persist workspace secret edits

## Summary

Make workspace-scoped secret edits persist and stop the settings list from showing a
stale revealed value across an edit.

## Implementation

- Add the `WorkspaceSecretUpdater` store surface and implement
  `UserVisibleStore.UpdateForWorkspace`, which authorizes the workspace read before
  delegating the write.
- Route `Service.UpdateWorkspaceSecret` and `Service.UpdateForWorkspace` writes through
  that surface and read the updated row back through the same workspace scope.
- Filter the edited secret's row from the settings list while its edit is open or in
  flight, so the edit form replaces the row and the row's local reveal state unmounts.
- Remove the now-unreachable `isEditing` prop from the secret row component.
- Split REQ-WORKSPACES-REPOSITORY-SECRETS-002 into
 `global-secret-metadata-lifetime.md` so the requirements file stays within the
 specification size limit after adding AC-WORKSPACES-REPOSITORY-SECRETS-001.15.

## Verification

- `go test ./internal/secrets/...`
- `apps/backend/internal/backendapp/secret_workspace_update_integration_test.go`
- `go test -trimpath -race ./internal/secrets -run TestRegisteredWorkspaceUpdate -count=1`
- `pnpm --filter @kandev/web exec vitest run components/settings/...`
- `pnpm e2e:run --project chromium tests/settings/secrets-edit.spec.ts --retries=0`
- `pnpm e2e:run --project mobile-chrome tests/settings/mobile-secrets-edit.spec.ts --retries=0`
