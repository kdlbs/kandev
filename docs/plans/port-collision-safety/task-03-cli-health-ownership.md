---
id: "03-cli-health-ownership"
title: "CLI health ownership"
status: done
wave: 2
depends_on: ["01-cli-explicit-port-preflight"]
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 03: CLI health ownership

## Acceptance

- Each CLI dev/start/run invocation generates a fresh token, passes it to the backend as
  KANDEV_DESKTOP_HEALTH_TOKEN, and requires the matching
  X-Kandev-Desktop-Health-Token response header before declaring readiness.
- A 2xx response with a missing or mismatched token never triggers the ready message or browser
  open; normal child-exit and timeout diagnostics remain intact without printing token values.
- The token is allowlisted in the CLI supervisor manifest so a backend restart for the same
  invocation remains recognized, while direct tokenless health callers retain current behavior.

## Verification

Use TDD for the HTTP and manifest cases:

~~~bash
cd apps
pnpm --filter kandev test -- src/health.test.ts src/shared.test.ts src/supervisor/manifest.test.ts
~~~

Run the broader CLI focused set with Task 01:

~~~bash
pnpm --filter kandev test -- src/args.test.ts src/ports.test.ts src/shared.test.ts src/health.test.ts src/supervisor/manifest.test.ts
~~~

## Files likely touched

- apps/cli/src/health.ts
- apps/cli/src/shared.ts
- apps/cli/src/dev.ts
- apps/cli/src/start.ts
- apps/cli/src/run.ts
- apps/cli/src/supervisor/manifest.ts
- apps/cli/src/health.test.ts
- apps/cli/src/shared.test.ts
- apps/cli/src/supervisor/manifest.test.ts

## Dependencies

Task 01 provides the shared CLI launch context and source-aware preflight path.

## Parallelism

Sequential after Task 01. Parallel-safe with Task 04 after both native preflight contracts are
stable, but the primary conversation executes sequentially by default.

## Inputs

- Spec sections: Backend readiness ownership and Issue #2372 scenarios.
- Plan sections: CLI launcher / Owned health readiness and CLI tests.
- Existing contract: apps/backend/internal/backendapp/helpers.go and the desktop
  KANDEV_DESKTOP_HEALTH_TOKEN response-header flow.

## Risks

- Generate once per launcher invocation, not on each poll or supervisor restart.
- Override any stale inherited token rather than trusting process environment state.
- Keep the token out of logs, errors, ring buffers, and tests' diagnostic output.

## Completion

- Behavior: each CLI launch owns one health token, passes it through the backend environment, and
  requires an exact token match on a successful health response; the supervisor manifest retains
  the allowlisted token.
- Files: shared CLI environment/health/supervisor and launcher test files listed above.
- Verification: CLI health, environment, manifest, and launch tests are green; the fixup adds
  explicit-token coverage so token ownership is visible to future callers.
- Compatibility: direct backend health requests without an expected token remain supported.
