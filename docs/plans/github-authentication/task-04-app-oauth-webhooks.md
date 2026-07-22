---
id: "04-app-oauth-webhooks"
title: "App installation and personal OAuth lifecycle"
status: completed
wave: 3
depends_on: ["01-persistence-migration", "02-app-token-primitives", "03-workspace-credential-resolver"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 04: App Installation And Personal OAuth Lifecycle

## Inputs

- Spec: App/personal API contracts, both state machines, permissions, callback/webhook scenarios.
- ADR 0047: verified association, PKCE, encrypted user tokens, webhook dedupe.
- Task 01 stores and Task 02 App primitives.

## Acceptance

- Setup and personal OAuth starts create expiring, single-use, workspace/user-bound state; callbacks
  reject expiry, replay, mismatch, spoofed installation, and unverified user association.
- Personal access/refresh secrets and expiry metadata update atomically, refresh before use, and
  never fall back across users; revocation removes effective personal auth.
- HMAC-verified, delivery-deduplicated webhooks apply installation suspend/unsuspend/delete,
  repository-access, and user-authorization transitions only to known bindings.

## Files Likely Touched

- `apps/backend/internal/github/oauth_flow.go` (new)
- `apps/backend/internal/github/oauth_flow_test.go` (new)
- `apps/backend/internal/github/app_installation_service.go` (new)
- `apps/backend/internal/github/app_installation_service_test.go` (new)
- `apps/backend/internal/github/personal_auth_service.go` (new)
- `apps/backend/internal/github/personal_auth_service_test.go` (new)
- `apps/backend/internal/github/webhook_service.go` (new)
- `apps/backend/internal/github/webhook_service_test.go` (new)

## Verification

```bash
cd apps/backend && rtk go test ./internal/github -run 'Test(OAuthFlow|AppInstallation|PersonalAuth|GitHubWebhook)'
```

## Output Contract

Report state TTLs, verification calls, refresh compensation, webhook transitions, tests run, files
touched, blockers, and risks. HTTP route wiring remains Task 07.
