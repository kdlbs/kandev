---
id: "01-cli-explicit-port-preflight"
title: "CLI explicit port preflight"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 01: CLI explicit port preflight

## Acceptance

- dev, start, and installed run reject an occupied explicitly selected backend port before
  starting the backend child or opening the browser.
- The failure identifies the numeric port and whether it came from a CLI flag,
  KANDEV_BACKEND_PORT, or KANDEV_PORT; an explicit port is never replaced with an automatic one.
- Omitted backend ports retain the current preferred-port and random-fallback behavior, and
  make dev PORT/WEB_PORT wiring from PR #2368 remains unchanged.

## Verification

Use TDD: add failing cases for each source and for the automatic fallback, implement the shared
assertion, then rerun:

~~~bash
cd apps
pnpm --filter kandev test -- src/args.test.ts src/ports.test.ts src/shared.test.ts
~~~

If the worktree has no dependencies:

~~~bash
cd apps
pnpm install --frozen-lockfile
~~~

## Files likely touched

- apps/cli/src/args.ts
- apps/cli/src/cli.ts
- apps/cli/src/ports.ts
- apps/cli/src/shared.ts
- apps/cli/src/dev.ts
- apps/cli/src/start.ts
- apps/cli/src/run.ts
- apps/cli/src/args.test.ts
- apps/cli/src/ports.test.ts
- apps/cli/src/shared.test.ts

## Dependencies

None.

## Parallelism

Parallel-safe candidate with Tasks 02 and 05; the primary conversation executes sequentially by
default. Task 03 owns the same CLI launch context and depends on this task.

## Inputs

- Spec sections: Explicit backend-port preflight and Issue #2370 scenarios.
- Plan sections: CLI launcher / Explicit port preflight and CLI tests.
- Existing helpers: resolvePorts, isPortAvailable, pickAvailablePort, pickPorts, and
  pickBackendPorts.

## Risks

- Keep the source metadata aligned with the existing CLI-over-environment precedence.
- Do not turn a probe failure into a random fallback for an explicitly requested port.
- The preflight does not eliminate the bind race; Task 03 supplies the ownership proof.

## Completion

- Behavior: explicit CLI backend ports are preflighted and fail with the configuration source;
  automatic web and agentctl selection excludes already selected service ports.
- Files: CLI argument, port-selection, launcher, and focused test files listed above.
- Verification: the focused CLI suite is green (`62 passed` in the fixup run); the original red
  case was the missing preflight/collision behavior.
- Caveat: preflight remains a check immediately before launch, while health-token ownership covers
  the remaining bind/readiness race.
