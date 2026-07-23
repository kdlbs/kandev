---
id: "09-backend-composition"
title: "Backend provider composition"
status: completed
wave: 3
depends_on:
  - "02-jira-provider"
  - "03-linear-provider"
  - "04-github-provider"
  - "05-gitlab-provider"
  - "06-azure-provider"
  - "07-sentry-provider"
  - "08-message-reference-metadata"
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 09: Backend Provider Composition

## Acceptance

- Backend registers built-in adapters through the same descriptor-driven registrar a future plugin bridge can use, without giving adapters control of registry identity, then mounts the workspace search route.
- Request workspace is validated before any provider call; nil/disconnected providers degrade safely.
- Handler-to-provider integration test proves mixed success/failure, deterministic caps, cancellation, and workspace isolation.

## Verification

```bash
cd apps/backend && go test ./internal/backendapp/... ./internal/mentions/...
```

## Files likely touched

- `apps/backend/internal/backendapp/types.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/mentions.go`
- `apps/backend/internal/backendapp/mentions_test.go`

## Dependencies

Tasks 02-08.

## Inputs

All provider constructors; existing integration service construction/route registration patterns; spec API order/status/plugin-compatibility contract; ADR 0043.

## Output contract

Report registry order and workspace validation, files changed, exact tests, blockers, risks, and mark task/plan done.
