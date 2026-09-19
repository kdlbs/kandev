---
id: "03-backend-security-and-lifecycle-settings"
title: "Backend security and lifecycle settings"
status: done
wave: 3
depends_on: ["02-launcher-configuration-consumption"]
plan: "plan.md"
spec: "../../specs/platform/requirements/startup-configuration-parity.md"
---

# Task 03: Backend security and lifecycle settings

Move stable security and lifecycle consumers from direct environment access to
the resolved typed configuration.

## Acceptance

- `server.trustedProxies` configures Gin proxy trust with the existing
  fail-closed parsing and warning behavior.
- `tasks.preparationTimeout` controls setup and derived launch budgets without a
  package-initialization environment read.
- `credentials.file` controls the operator credential file path through typed
  startup configuration.
- Environment variables keep their existing behavior and override YAML.
- YAML type or value errors fail startup. Legacy invalid environment values keep
  their documented fallback behavior.
- Constructors or startup options receive resolved values. The affected
  packages do not gain mutable global configuration.
- Logs identify invalid setting names and sources without exposing secret data.

## Files likely touched

- `apps/backend/internal/backendapp/trustedproxies.go`
- `apps/backend/internal/backendapp/trustedproxies_test.go`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/common/constants/timeouts.go`
- `apps/backend/internal/common/constants/timeouts_test.go`
- Agent runtime constructors and tests that consume preparation timeouts

## Dependencies

Task 02 makes the launcher and backend agree on the selected configuration.

## TDD sequence

1. Add tests that set only YAML for trusted proxies, preparation timeout, and
   credentials file. Confirm current consumers ignore those values.
2. Add environment-over-YAML cases and invalid YAML cases.
3. Pass typed values into consumers and remove in-scope direct environment
   reads.
4. Run focused tests GREEN. Run backend application, constants, auth, and
   runtime launch regressions.

## Verification

```bash
cd apps/backend && go test ./internal/backendapp -run '^Test.*(TrustedProxies|CredentialsFile|StartupConfig)' -count=1
cd apps/backend && go test ./internal/common/constants ./internal/agent/runtime/lifecycle -run '^Test.*(Preparation|LaunchTimeout|Config)' -count=1
cd apps/backend && go test ./internal/backendapp/... ./internal/auth/... -count=1
```

## Risks

- Trusted proxy configuration affects client IP, login rate limiting, and stored
  session IP data. The secure no-trust default must remain explicit.
- Preparation timeouts currently exist as package-level values. Initialization
  order can preserve stale environment behavior unless the value becomes an
  explicit dependency.
- The credentials file can contain secrets even though the path itself is not a
  secret. Error logs must not dump file contents.

## Output contract

Record RED and GREEN results, environment compatibility evidence, constructor
changes, direct-read removal, files changed, and remaining risks in
`## Results`.

## Results

RED:

- The new typed trusted-proxy, credential-file, and preparation-timeout tests
  initially failed until the backend consumers accepted resolved startup
  values. The old package-initialization timeout read also had to be removed
  before the typed startup value could take effect.

GREEN:

- `go test ./internal/backendapp -run '^Test(ConfigureTrustedProxiesUsesTyped|CredentialFilePath)' -count=1` passed.
- `go test ./internal/common/constants -run '^TestApplyPreparation' -count=1` passed.
- The affected package suite passed as part of `go test ./internal/common/config ./internal/common/constants ./internal/common/subproc ./internal/launcher ./internal/backendapp ./internal/system/queuesettings ./internal/system ./internal/gateway/websocket ./internal/task/service ./internal/runs/scheduler ./internal/office/service ./internal/agentctl/server/config ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/tracing ./internal/agent/runtime/agentctl/launcher ./internal/agent/runtime/lifecycle`, with 5,691 tests across 16 packages.

Trusted proxy parsing, credential-file selection, and preparation timeout
application now consume typed configuration. The legacy environment aliases
retain their fallback behavior, and invalid YAML values are rejected during
configuration loading. Preparation timeout state is applied explicitly at
backend startup instead of being captured by package initialization.

Files changed:

- `apps/backend/internal/backendapp/trustedproxies.go`
- `apps/backend/internal/backendapp/trustedproxies_test.go`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/backendapp/agents_startup_config_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/common/constants/timeouts.go`
- `apps/backend/internal/common/constants/timeouts_test.go`
