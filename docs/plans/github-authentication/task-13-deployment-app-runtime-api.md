---
id: "13-deployment-app-runtime-api"
title: "Deployment App runtime and API"
status: done
wave: 9
depends_on: ["11-deployment-app-persistence", "12-app-manifest-protocol"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 13: Deployment App Runtime and API

## Inputs

- Tasks 11-12 outputs.
- Spec: deployment registration API/state machine, hot-load, environment read-only behavior,
  callback failure modes, and webhook health.
- Existing patterns: `service_app_auth.go`, `controller_auth.go`, `backendapp/services.go`, and
  `mock_controller.go`.

## Acceptance

1. Boot and manifest callback use one resolver to build and atomically swap a complete immutable App
   runtime generation without restart or mixed credentials.
2. Status/start/callback/delete routes enforce the spec's state, source, URL, compensation, binding,
   stable-error, and no-secret contracts; callback state is single-use and environment state is
   read-only.
3. Mock/E2E reset support all deployment registration and webhook-health states, and integration
   tests prove callback-to-workspace-App availability through real stores.

## Files Likely Touched

- `apps/backend/internal/github/deployment_app_registration_service.go` (new)
- `apps/backend/internal/github/deployment_app_registration_service_test.go` (new)
- `apps/backend/internal/github/service_app_auth.go`
- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/controller_app_registration.go` (new)
- `apps/backend/internal/github/controller_app_registration_test.go` (new)
- `apps/backend/internal/github/mock_controller.go`
- `apps/backend/internal/github/mock_controller_test.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/e2e_reset.go`

## Verification

```bash
rtk go test ./internal/github -run 'Test(DeploymentAppRegistration|HTTPDeploymentApp|MockDeploymentApp)' -count=1
rtk go test ./internal/backendapp -run 'Test.*GitHub.*(Boot|Reset|Registration)' -count=1
```

Run from `apps/backend`.

## Dependencies

Tasks 11 and 12.

## Output Contract

Report runtime swap/rollback semantics, routes and stable errors, real-store integration coverage,
files touched, commands run, blockers, and authorization/restart risks. Mark this task `done` and
update `plan.md` only after targeted tests pass.
