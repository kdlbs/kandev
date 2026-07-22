---
id: "14-runtime-registration-resolution"
title: "Runtime registration resolution"
status: done
wave: 3
depends_on: ["13-registration-onboarding-protocol"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 14: Runtime Registration Resolution

## Acceptance

1. App runtime clients are independently loaded, hot-added, invalidated, and resolved by
   registration ID/generation; no global active App or fallback remains.
2. Installation token cache, auth resolver, broker lease, principal, rate/singleflight keys include
   registration identity where required and preserve workspace/repository isolation.
3. Concurrent different registrations, intentional reuse, restart, and one-registration failure are
   covered without leaking root or personal secrets.

## Verification

```bash
rtk go test ./internal/github -run 'Test.*(AppAuth|AuthResolver|TokenCache|CredentialBroker|Registration)'
rtk go test ./internal/backendapp -run 'Test.*GitHub'
```

Run from `apps/backend`.

## Files Likely Touched

- `apps/backend/internal/github/service_app_auth.go`
- `apps/backend/internal/github/service_app_auth_test.go`
- `apps/backend/internal/github/auth_resolver.go`
- `apps/backend/internal/github/auth_resolver_test.go`
- `apps/backend/internal/github/app_token_cache.go`
- `apps/backend/internal/github/app_token_cache_test.go`
- `apps/backend/internal/github/app_credential_provider.go`
- `apps/backend/internal/github/credential_broker.go`
- `apps/backend/internal/github/credential_broker_test.go`
- `apps/backend/internal/github/auth_principal.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/github_deployment_app_test.go` (rename/generalize)

## Dependencies

Task 13.

## Inputs

- Spec: **Identity And Routing**, workspace automation state machine, persistence, and security.
- Task 12 repository contracts and Task 13 verified registration objects.

## Output Contract

Report runtime registry/cache boundaries, tests run, files touched, blockers/risks, and update this
task plus `plan.md` to done.
