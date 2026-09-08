---
id: "04-backend-capacity-and-service-settings"
title: "Backend capacity and service settings"
status: done
wave: 3
depends_on: ["03-backend-security-and-lifecycle-settings"]
plan: "plan.md"
spec: "../../specs/platform/requirements/startup-configuration-parity.md"
---

# Task 04: Backend capacity and service settings

Move stable capacity and service timing settings behind explicit typed startup
configuration.

## Acceptance

- `limits.ghMaxConcurrent` and `limits.gitMaxConcurrent` configure subprocess
  admission without package-initialization environment reads.
- `limits.lspMaxConnections` configures LSP gateway capacity.
- `planning.coalesceWindowMs` configures plan update coalescing.
- `office.schedulerTickMs` configures the scheduler through one shared value and
  removes duplicate parsing.
- Environment variables override YAML. YAML overrides built-in defaults.
- Invalid YAML values fail startup with the key and selected file. Invalid
  legacy environment values keep current fallback behavior.
- Values are immutable for one backend process and require restart.
- Focused concurrency tests prove configured limits without timing-sensitive
  sleeps.

## Files likely touched

- `apps/backend/internal/common/subproc/shared.go`
- `apps/backend/internal/common/subproc/*_test.go`
- `apps/backend/internal/gateway/websocket/lsp_capacity.go`
- `apps/backend/internal/gateway/websocket/*_test.go`
- `apps/backend/internal/task/service/plan_service.go`
- `apps/backend/internal/task/service/*_test.go`
- `apps/backend/internal/runs/scheduler/scheduler.go`
- `apps/backend/internal/runs/scheduler/*_test.go`
- Office scheduler construction and tests
- Backend startup constructors that assemble these services

## Dependencies

Task 03 establishes the backend pattern for replacing direct stable environment
reads with explicit configuration.

## TDD sequence

1. Add focused YAML-only and environment-over-YAML tests for every setting. Run
   them RED.
2. Add constructor options or immutable startup setters at each owning package.
3. Remove duplicate environment parsing and package-initialization reads.
4. Run focused tests GREEN. Run package regressions and race-sensitive tests
   where those packages already support them.

## Verification

```bash
cd apps/backend && go test ./internal/common/subproc ./internal/gateway/websocket -run '^Test.*(Config|Capacity|Concurrent)' -count=1
cd apps/backend && go test ./internal/task/service ./internal/runs/scheduler -run '^Test.*(Config|Coalesce|SchedulerTick)' -count=1
cd apps/backend && go test ./internal/common/subproc ./internal/gateway/websocket ./internal/task/service ./internal/runs/scheduler -count=1
```

## Risks

- Shared subprocess gates are process-wide singletons. Tests must prove that
  startup constructs them once with the resolved limit.
- Two scheduler packages currently parse the same variable. Leaving either read
  in place would preserve executor-dependent behavior.
- Millisecond settings can create busy loops. Typed validation needs a positive
  lower bound that matches current safe behavior.

## Output contract

Record RED and GREEN results, the immutable ownership boundary for each value,
removed duplicate parsing, files changed, and remaining risks in `## Results`.

## Results

RED:

- Capacity and scheduler tests initially failed until typed startup options
  replaced package-initialization environment reads and the two scheduler
  packages shared one resolved tick interval.

GREEN:

- `go test ./internal/common/config ./internal/common/constants ./internal/common/subproc ./internal/gateway/websocket ./internal/task/service ./internal/runs/scheduler ./internal/office/service ./internal/backendapp -run '^Test.*(Config|Capacity|Concurrent|TrustedProxies|CredentialsFile|Preparation|LaunchTimeout|Coalesce|SchedulerTick|Startup)' -count=1` passed.
- The complete affected package suite passed as part of the 5,691-test,
  16-package backend run recorded in Task 03.

The backend now applies typed Git and GitHub admission caps, LSP capacity,
planning coalescing, and Office scheduler timing at startup. The shared
subprocess singleton is configured once, scheduler parsing is centralized,
and invalid legacy environment values retain their previous default fallback.
The configured values are immutable for the backend process and require a
restart.

Files changed:

- `apps/backend/internal/common/subproc/admission.go`
- `apps/backend/internal/common/subproc/shared.go`
- `apps/backend/internal/common/subproc/shared_test.go`
- `apps/backend/internal/common/subproc/throttle.go`
- `apps/backend/internal/gateway/websocket/lsp_handler.go`
- `apps/backend/internal/gateway/websocket/setup.go`
- `apps/backend/internal/gateway/websocket/lsp_capacity_test.go`
- `apps/backend/internal/task/service/plan_service.go`
- `apps/backend/internal/task/service/plan_service_test.go`
- `apps/backend/internal/runs/scheduler/scheduler.go`
- `apps/backend/internal/office/service/scheduler_integration.go`
- `apps/backend/internal/backendapp/main.go`
