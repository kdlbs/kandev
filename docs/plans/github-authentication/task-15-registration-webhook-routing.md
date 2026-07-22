---
id: "15-registration-webhook-routing"
title: "Registration webhook routing"
status: done
wave: 3
depends_on: ["13-registration-onboarding-protocol"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 15: Registration Webhook Routing

## Acceptance

1. The public webhook route selects one registration, validates its HMAC before parsing or claiming,
   and deduplicates by registration plus delivery ID.
2. Installation, repository, suspension, deletion, and user revocation events only mutate rows that
   match registration and external identity; health changes are registration-local.
3. Wrong route/signature, same delivery across Apps, unknown installation, replay, and concurrent
   delivery behavior have regression tests.

## Verification

```bash
rtk go test ./internal/github -run 'Test.*Webhook'
```

Run from `apps/backend`.

## Files Likely Touched

- `apps/backend/internal/github/webhook_service.go`
- `apps/backend/internal/github/webhook_service_test.go`
- `apps/backend/internal/github/controller_auth.go`
- `apps/backend/internal/github/controller_test.go`
- `apps/backend/internal/github/store.go`
- registration repository file created by Task 12

## Dependencies

Task 13.

## Inputs

- Spec: webhook API, `github_webhook_deliveries`, security, and failure scenarios.
- Task 12 composite delivery and registration lookup contracts.

## Output Contract

Report signature/dedupe ordering, tests run, files touched, blockers/risks, and update this task plus
`plan.md` to done.
