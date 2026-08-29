---
id: "03-launcher-contracts"
title: "Launcher contracts"
status: completed
wave: 2
depends_on: ["01-backend-log-sinks"]
plan: "plan.md"
spec: "../../specs/platform/requirements/diagnostic-logging.md"
---

# Task 03: Launcher contracts

## Acceptance

- Native and TypeScript normal/debug launch modes pass file `info`/`debug`
  thresholds while keeping stdout at `warn`; verbose passes `info` to both.
- Backend stdout is forwarded in every launch mode without losing the
  startup-failure capture used by health checks.
- Native and TypeScript help text describe `--debug` as file diagnostics and
  `--verbose` as stdout-info output.

## Verification

```bash
cd apps/backend
go test ./internal/launcher
```

```bash
cd apps
pnpm --filter @kandev/cli test -- src/shared.test.ts src/start.test.ts src/run.test.ts src/dev.test.ts
```

## Files likely touched

- `apps/backend/internal/launcher/start.go`
- `apps/backend/internal/launcher/start_test.go`
- `apps/backend/internal/launcher/env.go`
- `apps/backend/internal/launcher/process.go`
- `apps/backend/internal/launcher/process_test.go`
- `apps/backend/internal/launcher/supervisor.go`
- `apps/backend/internal/launcher/cli/help.go`
- `apps/cli/src/shared.ts`
- `apps/cli/src/shared.test.ts`
- `apps/cli/src/dev.ts`
- `apps/cli/src/start.ts`
- `apps/cli/src/start.test.ts`
- `apps/cli/src/run.ts`
- `apps/cli/src/run.test.ts`
- `apps/cli/src/cli.ts`

## Dependencies

- Task 01 defines the environment variables and backend sink behavior the
  launchers drive.

## Parallelism

Parallel-safe with Task 02 after Task 01. It owns launcher/CLI files only.

## Inputs

- Spec: What and normal/debug/verbose Scenarios.
- Plan: Launchers and CLI contracts.
- Existing patterns: Go `startProcess` captured output and TypeScript release
  `attachRingBuffer`.

## Risks

- TypeScript release mode must tee piped stdout without consuming or duplicating
  the health-failure buffer.
- Restart manifests must preserve the internal console-threshold environment.
- `KANDEV_LOG_LEVEL` remains an explicit file-level override.

## Output contract

Report the resolved matrix for every launcher mode, files changed, exact test
commands/results, blockers or risks, and update this task plus `plan.md` status
in the same conversation.
