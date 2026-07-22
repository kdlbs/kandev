---
id: "07-http-health-mocks"
title: "Connection API, health, and mocks"
status: completed
wave: 5
depends_on: ["04-app-oauth-webhooks", "05-service-routing", "06-executor-credentials"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 07: Connection API, Health, And Mocks

## Inputs

- Spec: complete API Surface, Permissions, stable observable failures, and App capability status.
- Completed service, lifecycle, and broker contracts.
- Existing Gin controller/handler and E2E mock patterns.

## Acceptance

- Every specified status/configure/disconnect/start/callback/webhook endpoint is wired with mandatory
  workspace/current-user ownership, stable error codes, safe redirects, and no secret responses.
- The old token endpoint is a workspace-required, one-release deprecated alias; health reports
  workspace automation/personal capability state instead of one global boolean.
- E2E mocks independently model workspace principals, App permissions/expiry, callback state, and
  webhook transitions, and reset clears all new state.

## Files Likely Touched

- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/controller_connections.go` (new)
- `apps/backend/internal/github/controller_app.go` (new)
- `apps/backend/internal/github/controller_personal.go` (new)
- `apps/backend/internal/github/controller_webhook.go` (new)
- `apps/backend/internal/github/handlers.go`
- `apps/backend/internal/github/controller_test.go`
- `apps/backend/internal/github/mock_client.go`
- `apps/backend/internal/github/mock_controller.go`
- `apps/backend/internal/github/mock_controller_test.go`
- `apps/backend/internal/health/checks.go`
- `apps/backend/internal/health/checks_test.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/e2e_reset.go`

## Verification

```bash
cd apps/backend && rtk go test ./internal/github -run 'TestHttp|TestMock'
cd apps/backend && rtk go test ./internal/health ./internal/backendapp -run 'Test.*GitHub'
```

## Output Contract

Report final route/request/response shapes, auth/redirect handling, compatibility headers, mock
controls, tests run, files touched, blockers, and risks.
