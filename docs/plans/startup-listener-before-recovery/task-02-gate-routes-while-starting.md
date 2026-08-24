---
id: "02-gate-routes-while-starting"
title: "Gate application routes behind an explicit starting state"
status: done
wave: 2
depends_on: ["01-bind-before-startup"]
plan: "plan.md"
spec: "../../specs/startup-listener-before-recovery/spec.md"
parallelism: sequential
---

# Task 02: Gate Application Routes Behind an Explicit Starting State

## Intent

Task 01 makes the socket answer while the application is still starting. A
caller must be able to tell that apart from a started backend, and must get a
deterministic answer rather than a hang.

## Acceptance

- Non-liveness requests arriving before startup completes receive an explicit
  starting response with a stable status code (503 is the conventional choice)
  and a machine-readable body, not a hang, a reset, or a 404.
- Readiness is exposed distinctly from liveness so an operator can tell "bound,
  still starting" from "fully started". Prefer extending the existing `/health`
  payload with a field over adding a new endpoint.
- **The liveness status code does not change while starting.** If the path the
  launcher probes starts returning non-200 during startup, the crash loop
  returns and Task 01 is undone. Add an explicit test asserting the launcher's
  probe path returns 200 during the starting state.
- The starting state clears exactly when the real router takes over and the
  existing `ready` flag flips; there is no third state that can persist.
- No new user-facing web copy. If any is added later it goes through `t()` per
  the i18n rules.

## Regression test (write first, must fail)

Extend `apps/backend/internal/backendapp/binds_before_startup_test.go` or add
`apps/backend/internal/backendapp/starting_state_test.go`:

- While startup is blocked, assert an API route returns the starting status and
  a parseable body.
- In the same blocked state, assert the launcher's liveness path returns 200.
- After startup completes, assert the same API route serves normally and
  readiness reports ready.

## Files likely touched

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/helpers.go` (the `/health` payload,
  around `helpers.go:716`)
- the corresponding `*_test.go` files

## Validation

```bash
cd apps/backend
go test ./internal/backendapp/... -count=1
make -C . lint
```

## Notes

Check `docs/specs/health-endpoint-version/spec.md` before changing the
`/health` response shape; that spec owns it and may need the same amendment
treatment the WIP-limit spec received.
