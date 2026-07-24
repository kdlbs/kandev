---
id: "01-interim-settings-interlock"
title: "Interim settings interlock"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/agents.md"
---

# Task 01: Interim settings interlock

## Acceptance

- State-changing agent/settings routes reject a missing/wrong boot token and
  any bearer credential before handler side effects.
- Legitimate SPA requests attach the per-boot token and existing desktop/mobile
  settings saves continue to work.
- Tests and naming describe the control as a replayable interim interlock/CSRF
  token, never operator authentication.

## Verification

```bash
cd apps/backend && go test ./internal/common/httpmw ./internal/agent/settings/handlers ./internal/backendapp
cd apps && pnpm --filter @kandev/web test -- --run src/boot-payload.test.ts lib/api
```

## Files likely touched

- `apps/backend/internal/common/httpmw/`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/handlers/*_test.go`
- `apps/backend/internal/webapp/payload.go`
- `apps/web/src/boot-payload.ts`
- `apps/web/src/boot-payload.test.ts`
- `apps/web/lib/api/client.ts`
- `apps/web/app/actions/agents.ts`
- focused API/action tests
- this task file

## Inputs

- ADR sections “Decision” and “Consequences”.
- Existing CORS middleware and SPA boot-payload hydration.
- Existing agent settings route registration and action clients.

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed
files and entry points, ADR sections, risk tags, exact RED/GREEN verification
commands/results, uncertainties, and this task status set to `done`. Do not edit
`plan.md`.
