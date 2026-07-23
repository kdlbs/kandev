---
id: "07-sentry-provider"
title: "Sentry mention provider"
status: completed
wave: 2
depends_on: ["01-core-and-task-search"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 07: Sentry Mention Provider

## Acceptance

- Adapter searches every configured active-workspace Sentry instance through bounded organization discovery.
- Stable reference scope includes instance ID; result ID/title/URL never collide across self-hosted instances.
- Free text is escaped from Sentry syntax and provider calls preserve cancellation/caps/safe error mapping.
- Provider authorizer verifies workspace-owned instance ID and that the destination matches that instance origin.

## Verification

```bash
cd apps/backend && go test ./internal/sentry/... ./internal/mentions/...
```

## Files likely touched

- `apps/backend/internal/sentry/client.go`
- `apps/backend/internal/sentry/rest_client.go`
- `apps/backend/internal/sentry/service_mentions.go`
- `apps/backend/internal/sentry/service_mentions_test.go`
- `apps/backend/internal/sentry/rest_client_test.go`
- `apps/backend/internal/mentions/provider_sentry.go`
- `apps/backend/internal/mentions/provider_sentry_test.go`

## Dependencies

Task 01.

## Inputs

ADR 0030 multiple-instance rules; existing `ListInstances`, instance ownership checks, browse client, and `SentryIssue.ID`.

## Output contract

Report instance/org discovery bounds, escaping, files changed, exact tests, blockers, risks, and mark task/plan done.
