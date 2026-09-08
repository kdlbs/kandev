---
id: "05-authenticated-webhook-handler"
title: "Enforce authenticated webhooks"
status: done
wave: 2
depends_on: ["02-authenticated-webhook-manifest"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/voice-extraction-host.md"
---

# Task 05: Enforce Authenticated Webhooks

## Acceptance

- The existing webhook handler enforces each declared key's access mode and effective body limit before
  invoking the existing `HandleWebhook` RPC.
- Authenticated webhooks require Kandev identity and accepted Origin for cookie-authenticated browser
  calls; public webhooks retain their current unauthenticated behavior.
- Integration tests cover legacy 4 MiB behavior, authenticated denial/success, Origin rejection,
  10 MiB multipart success, >16 MiB rejection, cancellation, disabled plugin, and runtime failure.

## Verification

```bash
cd apps/backend && go test ./internal/plugins ./internal/auth/httpmw ./internal/backendapp
make -C apps/backend lint
```

Follow RED-GREEN-REFACTOR, beginning with direct handler and middleware-boundary security tests.

## Files Likely Touched

- `apps/backend/internal/plugins/handlers.go`
- `apps/backend/internal/plugins/invoke.go` and focused tests if cancellation/size wiring needs changes
- `apps/backend/internal/auth/httpmw/*`

## Inputs And Risks

- ADR conditional webhook access decision.
- Because the webhook path is currently public at middleware level, handler authentication must use
  the canonical auth service and fail closed before reading or relaying the request body.
