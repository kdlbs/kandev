---
id: "16-workspace-app-lifecycle-api"
title: "Workspace App lifecycle API"
status: done
wave: 4
depends_on: ["14-runtime-registration-resolution", "15-registration-webhook-routing"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 16: Workspace App Lifecycle API

## Acceptance

1. Catalog list/import/create/rename/delete and workspace install/personal flows implement the spec's
   registration-aware routes, stable errors, authorization, and secret-free responses.
2. App install commits registration plus installation atomically, preserves old auth on failure,
   revokes old leases, and removes incompatible personal secrets on success.
3. Status, mock controller, E2E reset, callback redirects, guarded deletion, stale flows, and
   handler-to-real-store paths have focused tests.

## Verification

```bash
rtk go test ./internal/github -run 'Test.*(Registration|Installation|Personal|Controller|Status)'
rtk go test ./internal/backendapp -run 'Test.*(GitHub|Reset)'
```

Run from `apps/backend`.

## Files Likely Touched

- `apps/backend/internal/github/deployment_app_registration_service.go` (rename/generalize)
- `apps/backend/internal/github/deployment_app_registration_service_test.go`
- `apps/backend/internal/github/controller_app_registration.go` (rename/generalize)
- `apps/backend/internal/github/controller_app_registration_test.go`
- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/controller_auth.go`
- `apps/backend/internal/github/handlers.go`
- `apps/backend/internal/github/app_installation_service.go`
- `apps/backend/internal/github/app_installation_service_test.go`
- `apps/backend/internal/github/personal_auth_service.go`
- `apps/backend/internal/github/personal_auth_service_test.go`
- `apps/backend/internal/github/service_connections.go`
- `apps/backend/internal/github/workspace_settings_service.go`
- `apps/backend/internal/github/mock_controller.go`
- `apps/backend/internal/github/mock_controller_test.go`
- `apps/backend/internal/backendapp/e2e_reset.go`
- `apps/backend/internal/backendapp/e2e_reset_test.go`

## Dependencies

Tasks 14 and 15.

## Inputs

- Spec: complete **API Surface**, state machines, permissions, and failure modes.
- Runtime registry from Task 14 and webhook service from Task 15.

## Output Contract

Report endpoint/state transitions, tests run, files touched, blockers/risks, and update this task plus
`plan.md` to done.
