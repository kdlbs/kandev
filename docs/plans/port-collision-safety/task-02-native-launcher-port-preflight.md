---
id: "02-native-launcher-port-preflight"
title: "Native launcher port preflight"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
---

# Task 02: Native launcher port preflight

## Acceptance

- Native run and start reject an occupied explicitly selected backend port before launching the
  managed backend child.
- The error identifies the numeric port and whether the value came from --port,
  KANDEV_BACKEND_PORT, or KANDEV_PORT; an explicit port is never silently substituted.
- With no explicit backend port, the existing preferred-port and fallback selection remains
  unchanged.

## Verification

Use TDD around resolvePorts, pickPorts, runInstalled, and runStart:

~~~bash
cd apps/backend
go test -tags fts5 ./internal/launcher
~~~

Run the full backend target after the implementation wave:

~~~bash
make -C apps/backend test
~~~

## Files likely touched

- apps/backend/internal/launcher/ports.go
- apps/backend/internal/launcher/run.go
- apps/backend/internal/launcher/start.go
- apps/backend/internal/launcher/ports_test.go
- apps/backend/internal/launcher/start_test.go

## Dependencies

None.

## Parallelism

Parallel-safe candidate with Tasks 01 and 05; the primary conversation executes sequentially by
default. Task 04 owns the same native managed-launch context and depends on this task.

## Inputs

- Spec sections: Explicit backend-port preflight and Issue #2370 scenarios.
- Plan sections: Native Go launcher / Explicit port preflight and Go launcher tests.
- Existing helpers: resolvePorts, envPort, pickPorts, pickAvailablePortExcept, and canBind.

## Risks

- Preserve the current precedence and parse errors while adding source metadata.
- Validate before child launch without changing runtime-bundle resolution or service behavior outside
  the reported run/start paths.
- The source-aware preflight must not claim ownership; Task 04 handles the remaining race.

## Completion

- Behavior: native run/start launchers preflight explicit backend ports and preserve their source
  metadata in configuration errors.
- Files: native launcher port-resolution, run/start wiring, and launcher test files listed above.
- Verification: launcher race tests are green in the fixup run; the focused test first failed until
  the source-aware preflight and collision wiring were implemented.
- Cross-platform: the Windows-specific bind classifier and targeted CI coverage are recorded in
  Task 05; the preflight itself remains platform-neutral.
