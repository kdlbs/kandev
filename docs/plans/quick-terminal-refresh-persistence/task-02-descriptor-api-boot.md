---
id: "02-descriptor-api-boot"
title: "Expose descriptor API and boot state"
status: done
wave: 2
depends_on: ["01-persist-descriptors"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 02: Expose descriptor API and boot state

## Acceptance

- Authenticated GET/POST/PATCH/DELETE descriptor routes implement the documented idempotency,
  workspace authorization, lifecycle validation, best-effort stop, and already-closed behavior.
- Quick Chat boot includes the resolved workspace's persisted terminal descriptors, reconciled with
  the same live login-PTY manager used by the API; task-backed conversation boot remains unchanged.
- The repository/service is wired into storage and route construction without changing the legacy
  host-shell or Agents login endpoints.

## Verification

```bash
(cd apps/backend && go test ./internal/quickterminal/...)
(cd apps/backend && go test ./internal/backendapp -run 'TestBootPayloadRestoresQuickChat|Test.*QuickTerminal')
```

## Files likely touched

- `apps/backend/internal/quickterminal/handler_test.go` (new)
- `apps/backend/internal/backendapp/storage.go`
- `apps/backend/internal/backendapp/types.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/backend/internal/backendapp/boot_state_routes_test.go` (if route wiring coverage belongs there)

## Dependencies

Task 01.

## Parallelism

Sequential. Route, boot, auth, and storage wiring must agree on one descriptor contract.

## Inputs

- Spec API surface and boot/resync requirements.
- Task 01 service/repository interfaces.
- Existing user/task workspace authorization and `userhandlers.RegisterRoutes` patterns.
- Existing Quick Chat boot fixtures in `apps/backend/internal/backendapp/helpers_test.go`.

## Output contract

Report routes, boot payload shape, auth/workspace isolation evidence, exact backend tests, and any
compatibility note for legacy clients. Synchronize task/plan status and results.

## Results

- Registered the authenticated descriptor GET/POST/PATCH/DELETE routes, wired SQLite storage and the shared login-PTY manager, and added boot-state restoration with stale-session reconciliation.
- `(cd apps/backend && go test ./internal/backendapp ./internal/quickterminal/... ./internal/agent/loginpty/... -count=1)` passed (279 test cases/subcases).
- `(cd apps/backend && make lint)` passed with 0 issues; legacy agent-login and host-shell routes remain covered by the same backend suite.
