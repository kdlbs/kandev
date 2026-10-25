---
id: "04-native-health-ownership"
title: "Native health ownership"
status: done
wave: 2
depends_on: ["02-native-launcher-port-preflight"]
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 04: Native health ownership

## Acceptance

- Each native run/start managed invocation generates one fresh token, passes it through
  KANDEV_DESKTOP_HEALTH_TOKEN, and requires an exact matching
  X-Kandev-Desktop-Health-Token response header before printing backend ready.
- Missing or mismatched headers do not satisfy waitForHealth, while child exit, timeout, and
  cancellation behavior remains intact and token values never appear in diagnostics.
- The native supervisor manifest retains the token for backend restarts, and the existing backend
  health endpoint still echoes configured tokens without changing tokenless direct health calls.

## Verification

Use TDD for launcher health, environment/manifest, and backend route coverage:

~~~bash
cd apps/backend
go test -tags fts5 ./internal/launcher ./internal/backendapp
~~~

Run the complete backend verification after implementation:

~~~bash
make -C apps/backend test
~~~

## Files likely touched

- apps/backend/internal/launcher/health.go
- apps/backend/internal/launcher/env.go
- apps/backend/internal/launcher/run.go
- apps/backend/internal/launcher/start.go
- apps/backend/internal/launcher/supervisor.go
- apps/backend/internal/launcher/health_test.go
- apps/backend/internal/launcher/start_test.go
- apps/backend/internal/launcher/supervisor_test.go
- apps/backend/internal/backendapp/server_test.go

## Dependencies

Task 02 provides the native managed-launch context and explicit-port behavior.

## Parallelism

Sequential after Task 02. Parallel-safe with Task 03 after both preflight contracts are stable,
but the primary conversation executes sequentially by default.

## Inputs

- Spec sections: Backend readiness ownership and Issue #2372 scenarios.
- Plan sections: Native Go launcher / Owned health readiness and Go launcher/backend tests.
- Existing contract: backendapp health-token constants/echo behavior and the desktop launcher’s
  per-launch token generation pattern.

## Risks

- Generate the token once before the first backend launch and reuse the same environment for
  supervisor restarts.
- Keep the manifest allowlist narrow and preserve owner-only manifest permissions.
- Do not require a token from unrelated direct backend health requests.

## Completion

- Behavior: native launches generate one token before the first backend process, require the exact
  token on health success, and preserve it through supervisor restarts.
- Files: native launcher environment/health/supervisor, backend health handler, and test files
  listed above.
- Verification: launcher/backend race tests are green; the fixup adds coverage that cancellation
  invokes the health failure callback exactly once.
- Cross-platform: token matching uses ordinary HTTP headers and the existing manifest allowlist.
