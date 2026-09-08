---
id: "05-agentctl-settings-propagation"
title: "Agentctl settings propagation"
status: done
wave: 4
depends_on: ["04-backend-capacity-and-service-settings"]
plan: "plan.md"
spec: "../../specs/platform/requirements/startup-configuration-parity.md"
---

# Task 05: Agentctl settings propagation

Resolve agentctl settings in the backend and pass them explicitly through every
managed agentctl launch path.

## Acceptance

- `agentctl.idleTimeout` configures idle session cleanup.
- `agentctl.idleReaperInterval` configures the reaper interval.
- `agentctl.notificationQueueCapacity` configures ACP notification buffering.
- `observability.otlpEndpoint` configures tracing for the backend and managed
  agentctl processes.
- Environment values override YAML and keep compatibility.
- Local, Worktree, Docker, Sprite, and SSH launches receive the same resolved
  values.
- Remote and container configuration uses an explicit child contract. It does
  not depend on inheriting the launcher's host environment.
- Missing or invalid required child configuration fails that launch clearly.
- Debug-only agentctl logging and retention variables remain excluded.

## Files likely touched

- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/config/config_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_queue_test.go`
- `apps/backend/internal/agentctl/tracing/otel.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites*.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh*.go`

## Dependencies

Task 04 completes the backend typed-consumer pattern. Task 01 provides the
agentctl and observability fields.

## TDD sequence

1. Add server and adapter tests for YAML-only resolved values. Run them RED.
2. Add launch contract tests that inspect Local, Docker, Sprite, and SSH child
   configuration. Include Worktree through its local launch path.
3. Replace direct stable environment reads in agentctl with explicit options.
4. Thread the resolved child contract through every executor.
5. Run focused tests GREEN. Run the complete agentctl and lifecycle suites.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/config ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/tracing -run '^Test.*(Config|Idle|Queue|OTLP)' -count=1
cd apps/backend && go test ./internal/agent/runtime/lifecycle -run '^Test.*(Agentctl|Standalone|Docker|Sprite|SSH).*Config' -count=1
cd apps/backend && go test ./internal/agentctl/... ./internal/agent/runtime/lifecycle -count=1
```

## Risks

- Container and remote script quoting can corrupt duration or endpoint values.
  Tests must assert the structured launch input before shell rendering and the
  rendered result where applicable.
- Utility and passthrough agentctl launches can use separate code paths. The
  launch audit must include them.
- Tracing already uses a standard OpenTelemetry environment name. YAML support
  must not change its environment override semantics.

## Output contract

Record RED and GREEN results, executor coverage, the child configuration shape,
direct-read removal, files changed, and remaining risks in `## Results`.

## Results

RED:

- Agentctl server and executor contract tests initially failed until the
  managed child configuration was represented in `common/config` and threaded
  through lifecycle requests.

GREEN:

- `go test ./internal/agent/runtime/agentctl/launcher ./internal/agent/runtime/lifecycle ./internal/agentctl/server/config ./internal/agentctl/server/adapter/transport/acp ./internal/common/config -run '^Test.*(Startup|EnvironmentWithOverrides|Queue|LoadWithStartup|ConfigurationCatalog|Agentctl)' -count=1` passed 148 tests across 5 packages.
- `go test ./internal/common/config ./internal/agentctl/server/config ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/tracing ./internal/agent/runtime/agentctl/launcher ./internal/agent/runtime/lifecycle ./internal/backendapp -run '^Test.*(Startup|Config|Queue|Agentctl|Explicit|Capacity|CredentialFilePath|TrustedProxies)' -count=1` passed 393 tests across 7 packages.
- The complete affected package suite passed in the 5,691-test backend run recorded in Task 03.

The backend resolves one validated `AgentctlStartupConfig` and passes it
through Local, Worktree, Docker, Sprite, and SSH launch paths. The private
contract carries idle timeout, reaper interval, notification queue capacity,
and OTLP endpoint values. Managed agentctl processes load that contract before
legacy standalone compatibility paths; malformed contracts fail launch rather
than silently falling back. Host environment inheritance is removed where it
could conflict with the managed contract, while direct standalone adapters
retain compatibility reads for existing callers and tests.

Files changed include the common contract, agentctl server/config/adapter and
tracing packages, lifecycle manager and executor launch paths, and the focused
launcher, adapter, configuration, and lifecycle tests.
